// Package e2e contains the multi-process simulation test for the warden
// service: it builds the real binary, spawns a three-node cluster on
// localhost ports, and exercises leader election, leader death,
// re-election, death alerting, node rejoin, and recovery alerting.
//
// Run from the module root (the repository root of candacelabs/candace, and
// candace/ inside the monorepo it is exported from):
//
//	go test ./app/warden/e2e/ -v
//
// The test is skipped under -short.
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Timing profile for the test cluster: aggressive but safe multiples so
// the whole scenario completes in well under two minutes on a loaded box.
const (
	heartbeatInterval = "200ms"
	suspectAfter      = "1s"
	deadAfter         = "3s"
	electionMin       = "400ms"
	electionMax       = "800ms"
	rpcTimeout        = "250ms"
	cooldown          = "2s"

	convergeTimeout = 20 * time.Second
	alertTimeout    = 20 * time.Second
	pollEvery       = 250 * time.Millisecond
)

// statusView mirrors the parts of the dashboard /api/status response the
// test asserts on (kept intentionally minimal and decoupled).
type statusView struct {
	View struct {
		Self          string `json:"self"`
		Role          string `json:"role"`
		Term          uint64 `json:"term"`
		LeaderID      string `json:"leader_id"`
		Authoritative bool   `json:"authoritative"`
		Peers         []struct {
			Node struct {
				ID   string `json:"id"`
				Addr string `json:"addr"`
			} `json:"node"`
			Status string `json:"status"`
			Member string `json:"member"`
		} `json:"peers"`
		Membership struct {
			Version uint64 `json:"version"`
			Voters  []struct {
				ID string `json:"id"`
			} `json:"voters"`
		} `json:"membership"`
	} `json:"view"`
}

// alertLine mirrors the file notifier's JSONL incident records.
type alertLine struct {
	Type string `json:"type"`
	Peer struct {
		ID string `json:"id"`
	} `json:"peer"`
	ReportedBy string `json:"reported_by"`
}

type node struct {
	id        string
	addr      string
	dataDir   string
	alertFile string
	logFile   string
	cmd       *exec.Cmd
}

type cluster struct {
	t        *testing.T
	bin      string
	nodes    map[string]*node
	peers    string   // WARDEN_PEERS value
	extraEnv []string // appended to every node's environment (discovery knobs)
}

func TestClusterLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-process e2e test in -short mode")
	}

	moduleRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolving module root: %v", err)
	}

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "warden")
	build := exec.Command("go", "build", "-o", bin, "github.com/candacelabs/candace/app/warden/cmd")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building warden binary: %v\n%s", err, out)
	}

	c := &cluster{t: t, bin: bin, nodes: map[string]*node{}}
	ids := []string{"n1", "n2", "n3"}
	ports := []int{19701, 19702, 19703}
	var peerParts []string
	for i, id := range ids {
		addr := fmt.Sprintf("127.0.0.1:%d", ports[i])
		dir := filepath.Join(tmp, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		c.nodes[id] = &node{
			id:        id,
			addr:      addr,
			dataDir:   dir,
			alertFile: filepath.Join(dir, "alerts.jsonl"),
			logFile:   filepath.Join(dir, "warden.log"),
		}
		peerParts = append(peerParts, id+"="+addr)
	}
	c.peers = strings.Join(peerParts, ",")

	for _, id := range ids {
		c.start(id)
	}
	t.Cleanup(c.stopAll)

	// --- Phase 1: a single leader is elected and all nodes agree. ---
	leader1, term1 := c.awaitConsistentLeader(ids, "", 0, "initial election")
	t.Logf("initial leader %q at term %d", leader1, term1)

	// --- Phase 2: kill the leader; survivors elect a new leader at a
	// higher term. Immediately after the kill the survivors still report
	// the stale leader until timeouts elapse, so convergence here demands
	// a leader other than the killed node at a strictly higher term. ---
	c.kill(leader1)
	survivors := remove(ids, leader1)
	leader2, term2 := c.awaitConsistentLeader(survivors, leader1, term1, "re-election after leader death")
	if leader2 == leader1 {
		t.Fatalf("re-election chose the killed node %q", leader1)
	}
	if term2 <= term1 {
		t.Fatalf("term did not increase after re-election: %d -> %d", term1, term2)
	}
	t.Logf("new leader %q at term %d", leader2, term2)

	// --- Phase 3: the new leader declares the dead node dead and the
	// death alert is recorded exactly once (per episode). ---
	c.awaitAlert(leader2, "peer_dead", leader1)
	c.awaitPeerStatus(leader2, leader1, "dead")

	// --- Phase 4: restart the killed node; it rejoins as a follower,
	// the cluster converges with all peers alive, and a recovery alert
	// is recorded. ---
	c.start(leader1)
	leader3, _ := c.awaitConsistentLeader(ids, "", 0, "convergence after rejoin")
	c.awaitAllAlive(leader3, ids)

	if got := c.status(leader1); got.View.Role == "leader" {
		// Not fatal by itself (leadership can legally move), but the
		// rejoined node must at least agree on the consistent view,
		// which awaitConsistentLeader already proved. Log for insight.
		t.Logf("note: rejoined node %q ended up leader again", leader1)
	}

	// Recovery may be observed by whichever node led when the peer came
	// back; accept the alert from any node's file.
	c.awaitAlertAnywhere("peer_recovered", leader1)

	// The death alert for this episode must not have been duplicated.
	if n := c.countAlerts(leader2, "peer_dead", leader1); n > 1 {
		t.Fatalf("expected at most one peer_dead alert for %q from %q, got %d", leader1, leader2, n)
	}

	// Regression ledger (review M2): subsystem packages must emit the same
	// JSON log format as main. The election package's "starting election"
	// line is written via the shared core.Logger; if the JSON logger is not
	// installed globally before components are built, this line appears in
	// console format and fails to parse.
	c.assertSubsystemLogsJSON(leader2, `"warden: starting election"`)
}

// assertSubsystemLogsJSON asserts that a log line from a subsystem package
// (matched by substring) exists in the node's log and is valid JSON.
func (c *cluster) assertSubsystemLogsJSON(id, substr string) {
	data, err := os.ReadFile(c.nodes[id].logFile)
	if err != nil {
		c.t.Fatalf("reading %s log: %v", id, err)
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, substr) {
			continue
		}
		found = true
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			c.t.Fatalf("subsystem log line is not JSON (log-format propagation regression): %q: %v", line, err)
		}
	}
	if !found {
		c.t.Fatalf("no subsystem log line containing %s found in %s's log (searched for JSON-format proof)", substr, id)
	}
}

func (c *cluster) start(id string) {
	n := c.nodes[id]
	logf, err := os.OpenFile(n.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		c.t.Fatalf("opening log file for %s: %v", id, err)
	}
	cmd := exec.Command(c.bin)
	cmd.Env = append(os.Environ(),
		"WARDEN_NODE_ID="+n.id,
		"WARDEN_BIND="+n.addr,
		"WARDEN_DATA_DIR="+n.dataDir,
		"WARDEN_PEERS="+c.peers,
		"WARDEN_NOTIFY_MODE=file",
		"WARDEN_NOTIFY_FILE="+n.alertFile,
		"WARDEN_HEARTBEAT_INTERVAL="+heartbeatInterval,
		"WARDEN_SUSPECT_AFTER="+suspectAfter,
		"WARDEN_DEAD_AFTER="+deadAfter,
		"WARDEN_ELECTION_TIMEOUT_MIN="+electionMin,
		"WARDEN_ELECTION_TIMEOUT_MAX="+electionMax,
		"WARDEN_RPC_TIMEOUT="+rpcTimeout,
		"WARDEN_COOLDOWN="+cooldown,
		"WARDEN_LOG_LEVEL=debug",
	)
	cmd.Env = append(cmd.Env, c.extraEnv...)
	cmd.Env = append(cmd.Env, "WARDEN_ADVERTISE_ADDR="+n.addr)
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		logf.Close()
		c.t.Fatalf("starting %s: %v", id, err)
	}
	// The write side is inherited by the child; close the parent's copy
	// once started.
	logf.Close()
	n.cmd = cmd
	go cmd.Wait() // reap; exit status is irrelevant (nodes die by SIGKILL)
	c.t.Logf("started %s (pid %d) on %s", id, cmd.Process.Pid, n.addr)
}

func (c *cluster) kill(id string) {
	n := c.nodes[id]
	if n.cmd == nil || n.cmd.Process == nil {
		c.t.Fatalf("kill %s: not running", id)
	}
	if err := n.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		c.t.Fatalf("killing %s: %v", id, err)
	}
	c.t.Logf("killed %s (pid %d)", id, n.cmd.Process.Pid)
	n.cmd = nil
}

func (c *cluster) stopAll() {
	for id, n := range c.nodes {
		if n.cmd != nil && n.cmd.Process != nil {
			_ = n.cmd.Process.Signal(syscall.SIGKILL)
			n.cmd = nil
		}
		_ = id
	}
}

// status fetches /api/status from a node; fails the test on transport or
// decode errors only after the caller's own polling deadline handling, so
// it returns a zero statusView plus false on any error.
func (c *cluster) tryStatus(id string) (statusView, bool) {
	n := c.nodes[id]
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + n.addr + "/api/status")
	if err != nil {
		return statusView{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusView{}, false
	}
	var sv statusView
	if err := json.NewDecoder(resp.Body).Decode(&sv); err != nil {
		return statusView{}, false
	}
	return sv, true
}

func (c *cluster) status(id string) statusView {
	sv, ok := c.tryStatus(id)
	if !ok {
		c.t.Fatalf("fetching status from %s failed", id)
	}
	return sv
}

// awaitConsistentLeader polls the given nodes until they all report the
// same nonempty leader, exactly one of them claims the leader role (when
// the leader is among the polled set), and every polled node returns an
// authoritative view. When notLeader is nonempty the agreed leader must
// differ from it, and the agreed term must exceed minTerm — this rejects
// the stale pre-kill state survivors report right after a leader dies.
// Returns the agreed leader and its term.
func (c *cluster) awaitConsistentLeader(ids []string, notLeader string, minTerm uint64, phase string) (string, uint64) {
	deadline := time.Now().Add(convergeTimeout)
	var lastErr string
	for time.Now().Before(deadline) {
		leader, term, err := c.checkConsistent(ids)
		switch {
		case err != "":
			lastErr = err
		case notLeader != "" && leader == notLeader:
			lastErr = fmt.Sprintf("still reporting stale leader %s", notLeader)
		case term <= minTerm && minTerm > 0:
			lastErr = fmt.Sprintf("term %d has not advanced past %d", term, minTerm)
		default:
			return leader, term
		}
		time.Sleep(pollEvery)
	}
	c.dumpLogs()
	c.t.Fatalf("%s: cluster did not converge within %s: %s", phase, convergeTimeout, lastErr)
	return "", 0
}

func (c *cluster) checkConsistent(ids []string) (string, uint64, string) {
	leader := ""
	var term uint64
	leaders := 0
	for _, id := range ids {
		sv, ok := c.tryStatus(id)
		if !ok {
			return "", 0, "node " + id + " unreachable"
		}
		if sv.View.LeaderID == "" {
			return "", 0, "node " + id + " sees no leader"
		}
		if leader == "" {
			leader = sv.View.LeaderID
			term = sv.View.Term
		} else if sv.View.LeaderID != leader {
			return "", 0, fmt.Sprintf("disagreement: %s sees %s, others see %s", id, sv.View.LeaderID, leader)
		} else if sv.View.Term != term {
			return "", 0, fmt.Sprintf("term disagreement on %s: %d vs %d", id, sv.View.Term, term)
		}
		if sv.View.Role == "leader" {
			leaders++
			if sv.View.Self != sv.View.LeaderID {
				return "", 0, "node " + id + " is leader but reports another leader id"
			}
		}
		if !sv.View.Authoritative {
			return "", 0, "node " + id + " view not authoritative yet"
		}
	}
	wantLeaders := 0
	for _, id := range ids {
		if id == leader {
			wantLeaders = 1
		}
	}
	if leaders != wantLeaders {
		return "", 0, fmt.Sprintf("expected %d leader role(s) among polled nodes, saw %d", wantLeaders, leaders)
	}
	return leader, term, ""
}

// awaitPeerStatus polls until observer's view reports peer with status.
func (c *cluster) awaitPeerStatus(observer, peer, status string) {
	deadline := time.Now().Add(alertTimeout)
	for time.Now().Before(deadline) {
		if sv, ok := c.tryStatus(observer); ok {
			for _, p := range sv.View.Peers {
				if p.Node.ID == peer && p.Status == status {
					return
				}
			}
		}
		time.Sleep(pollEvery)
	}
	c.dumpLogs()
	c.t.Fatalf("%s never reported peer %s as %s within %s", observer, peer, status, alertTimeout)
}

// awaitAllAlive polls until observer's view reports every listed node alive.
func (c *cluster) awaitAllAlive(observer string, ids []string) {
	deadline := time.Now().Add(convergeTimeout)
	for time.Now().Before(deadline) {
		if sv, ok := c.tryStatus(observer); ok {
			alive := map[string]bool{}
			for _, p := range sv.View.Peers {
				if p.Status == "alive" {
					alive[p.Node.ID] = true
				}
			}
			all := true
			for _, id := range ids {
				if !alive[id] {
					all = false
					break
				}
			}
			if all {
				return
			}
		}
		time.Sleep(pollEvery)
	}
	c.dumpLogs()
	c.t.Fatalf("%s never reported all of %v alive within %s", observer, ids, convergeTimeout)
}

func (c *cluster) readAlerts(id string) []alertLine {
	data, err := os.ReadFile(c.nodes[id].alertFile)
	if err != nil {
		return nil
	}
	var out []alertLine
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var a alertLine
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			c.t.Errorf("malformed alert line in %s alerts file: %q: %v", id, line, err)
			continue
		}
		out = append(out, a)
	}
	return out
}

func (c *cluster) countAlerts(id, typ, peer string) int {
	n := 0
	for _, a := range c.readAlerts(id) {
		if a.Type == typ && a.Peer.ID == peer {
			n++
		}
	}
	return n
}

// awaitAlert waits for node id's alert file to record an alert of typ for peer.
func (c *cluster) awaitAlert(id, typ, peer string) {
	deadline := time.Now().Add(alertTimeout)
	for time.Now().Before(deadline) {
		if c.countAlerts(id, typ, peer) >= 1 {
			return
		}
		time.Sleep(pollEvery)
	}
	c.dumpLogs()
	c.t.Fatalf("no %s alert for %s recorded by %s within %s", typ, peer, id, alertTimeout)
}

// awaitAlertAnywhere waits for any node's alert file to record the alert.
func (c *cluster) awaitAlertAnywhere(typ, peer string) {
	deadline := time.Now().Add(alertTimeout)
	for time.Now().Before(deadline) {
		for id := range c.nodes {
			if c.countAlerts(id, typ, peer) >= 1 {
				return
			}
		}
		time.Sleep(pollEvery)
	}
	c.dumpLogs()
	c.t.Fatalf("no %s alert for %s recorded by any node within %s", typ, peer, alertTimeout)
}

// dumpLogs tails each node's log into the test output to make failures
// diagnosable.
func (c *cluster) dumpLogs() {
	for id, n := range c.nodes {
		data, err := os.ReadFile(n.logFile)
		if err != nil {
			c.t.Logf("--- %s: no log (%v)", id, err)
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		const tail = 40
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
		c.t.Logf("--- last %d log lines of %s ---\n%s", len(lines), id, strings.Join(lines, "\n"))
	}
}

func remove(ids []string, drop string) []string {
	var out []string
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}

// writeRoster atomically writes the file-discovery roster.
func writeRoster(t *testing.T, path string, ids map[string]string) {
	t.Helper()
	type n struct {
		ID   string `json:"id"`
		Addr string `json:"addr"`
	}
	var out struct {
		Nodes []n `json:"nodes"`
	}
	for id, addr := range ids {
		out.Nodes = append(out.Nodes, n{ID: id, Addr: addr})
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal roster: %v", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write roster: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename roster: %v", err)
	}
}

// TestClusterDiscoveryJoin exercises live membership growth: a 3-voter
// cluster in file-discovery mode discovers a 4th warden process (started
// with the same 3-node seed, itself absent — the joiner configuration),
// admits it observer-first, and every node converges on a 4-voter
// membership at a higher version — all without restarting anything.
// Afterwards the leader is killed to prove elections work at the new
// quorum (3 of 4).
func TestClusterDiscoveryJoin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-process e2e test in -short mode")
	}
	moduleRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolving module root: %v", err)
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "warden")
	build := exec.Command("go", "build", "-o", bin, "github.com/candacelabs/candace/app/warden/cmd")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building warden binary: %v\n%s", err, out)
	}

	roster := filepath.Join(tmp, "roster.json")
	c := &cluster{t: t, bin: bin, nodes: map[string]*node{}, extraEnv: []string{
		"WARDEN_DISCOVERY_MODE=file",
		"WARDEN_DISCOVERY_FILE=" + roster,
		"WARDEN_FILE_POLL_INTERVAL=200ms",
		"WARDEN_JOIN_STABILITY=1s",
	}}

	seeds := []string{"n1", "n2", "n3"}
	ports := map[string]int{"n1": 19711, "n2": 19712, "n3": 19713, "n4": 19714}
	addrs := map[string]string{}
	var peerParts []string
	for id, port := range ports {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		addrs[id] = addr
		dir := filepath.Join(tmp, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		c.nodes[id] = &node{id: id, addr: addr, dataDir: dir,
			alertFile: filepath.Join(dir, "alerts.jsonl"), logFile: filepath.Join(dir, "warden.log")}
	}
	for _, id := range seeds {
		peerParts = append(peerParts, id+"="+addrs[id])
	}
	c.peers = strings.Join(peerParts, ",") // 3-node seed for EVERY process, incl. the joiner

	writeRoster(t, roster, map[string]string{"n1": addrs["n1"], "n2": addrs["n2"], "n3": addrs["n3"]})
	for _, id := range seeds {
		c.start(id)
	}
	t.Cleanup(c.stopAll)

	leader1, _ := c.awaitConsistentLeader(seeds, "", 0, "initial election (discovery mode)")
	if sv := c.status(leader1); len(sv.View.Membership.Voters) != 3 {
		t.Fatalf("seed membership voters = %d, want 3", len(sv.View.Membership.Voters))
	}

	// The 4th node starts (same seed, self absent) and enters the roster.
	c.start("n4")
	writeRoster(t, roster, addrs)

	// Observer phase: some seed node reports n4 as observer.
	deadline := time.Now().Add(convergeTimeout)
	sawObserver := false
	for time.Now().Before(deadline) && !sawObserver {
		if sv, ok := c.tryStatus(leader1); ok {
			for _, p := range sv.View.Peers {
				if p.Node.ID == "n4" && p.Member == "observer" {
					sawObserver = true
				}
			}
		}
		time.Sleep(pollEvery)
	}
	if !sawObserver {
		c.dumpLogs()
		t.Fatalf("n4 never appeared as observer on the leader's view")
	}

	// Admission: every node (including n4) converges on 4 voters.
	all := []string{"n1", "n2", "n3", "n4"}
	deadline = time.Now().Add(convergeTimeout)
	for {
		if time.Now().After(deadline) {
			c.dumpLogs()
			t.Fatalf("cluster never converged on 4-voter membership")
		}
		okAll := true
		for _, id := range all {
			sv, ok := c.tryStatus(id)
			if !ok || len(sv.View.Membership.Voters) != 4 || sv.View.Membership.Version < 2 {
				okAll = false
				break
			}
			if id == "n4" && sv.View.Role != "follower" && sv.View.Role != "leader" {
				okAll = false
				break
			}
		}
		if okAll {
			break
		}
		time.Sleep(pollEvery)
	}

	// Elections still work at the new quorum (3 of 4): kill the leader.
	leader2, term2 := c.awaitConsistentLeader(all, "", 0, "post-join convergence")
	c.kill(leader2)
	leader3, term3 := c.awaitConsistentLeader(remove(all, leader2), leader2, term2, "re-election with 4-voter membership")
	if leader3 == leader2 || term3 <= term2 {
		t.Fatalf("re-election failed under dynamic membership: leader %q term %d", leader3, term3)
	}
}

package main

import (
	"context"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/candacelabs/candace/examples/widget/clusterheartbeats"
	"github.com/candacelabs/candace/examples/widget/nodestatus"
	"github.com/candacelabs/candace/pkg/gotth/live"
	"github.com/candacelabs/candace/pkg/gotth/live/livetest"
	"github.com/candacelabs/candace/pkg/widget"
)

// snapshot is one cluster delivery, as the wire carries it.
func snapshot(sequence uint64, leaderKnown bool, aliveVoters int) live.Event {
	return live.Event{
		Name: clusterheartbeats.ClusterHeartbeatsEventSnapshot,
		Fields: live.NewFields(map[string]string{
			"sequence":      strconv.FormatUint(sequence, 10),
			"connected":     "true",
			"authoritative": "true",
			"leader_known":  strconv.FormatBool(leaderKnown),
			"has_quorum":    "true",
			"term":          "7",
			"voters":        "3",
			"alive_voters":  strconv.Itoa(aliveVoters),
		}),
	}
}

var _ = Describe("The generated widgets' dirty declarations", func() {
	// A generated widget answers for itself whether a transition reached its
	// region, from its document's computed dirty projection. Over-declaring
	// costs a suppressed render; under-declaring is a correctness bug the
	// compiler cannot see — a region that stops updating for reasons nothing
	// explains — so it is asserted against the markup rather than against the
	// projection it came from.
	It("declare every transition that moves their markup", func() {
		config, configError := hostWidgets().LiveConfig(widget.MountOptions{
			Origins:      []string{"http://127.0.0.1:8080"},
			Authenticate: live.Anonymous,
			Authorize:    live.AllowAll,
			CSRF:         live.NoCSRFCheck,
		})
		Expect(configError).ToNot(HaveOccurred())

		initial, _, initError := config.Init(context.Background(), live.Session{})
		Expect(initError).ToNot(HaveOccurred())

		// One of everything that can move either widget: a tick, an election,
		// the pause control, the health check, and the two notices the runtime
		// mints rather than a browser sending them.
		// GinkgoTB rather than GinkgoT: the library's assertion takes a
		// testing.TB, which only the wrapper satisfies.
		livetest.AssertDirtyComplete(GinkgoTB(), config, initial, []live.Event{
			snapshot(1, true, 3),
			snapshot(2, false, 2),
			snapshot(3, true, 3),
			// Two deliveries apart only in the tick. Nothing the bindings read
			// has moved, so this is the transition that catches a dirty
			// declaration which forgot that the scene's own identity is the
			// tick — the whole reason the projection counts it as read.
			snapshot(4, true, 3),
			{Name: clusterheartbeats.ClusterHeartbeatsEventToggleMotion},
			{Name: clusterheartbeats.ClusterHeartbeatsEventToggleMotion},
			{
				Name:   nodestatus.NodeStatusEventHealth,
				Fields: live.NewFields(map[string]string{"reachable": "false"}),
			},
			{
				Name:   nodestatus.NodeStatusEventHealth,
				Fields: live.NewFields(map[string]string{"reachable": "true"}),
			},
			{Name: live.SlowClientEvent},
			{Name: live.ClientRecoveredEvent},
			// A counter never walks backwards, so this delivery changes
			// nothing and neither region may claim it did.
			snapshot(2, true, 3),
		})
	})
})

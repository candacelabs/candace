package livetest

import (
	"testing"

	"github.com/candacelabs/candace/pkg/gotth/internal/livebridge"
	"github.com/candacelabs/candace/pkg/gotth/live"
)

// NewSession returns the [live.Session] a spec needs in order to call an
// application's own Config hooks directly.
//
// [live.Config]'s Init, Authorize, Teardown and Execute all take a Session, and
// a Session's fields are unexported because identity is bound at the handshake
// and nothing downstream may mint one. The consequence for a test is sharper
// than it first looks: live.Session{} compiles anywhere — an empty composite
// literal names no field — but its ID() is all-zero and its Identity() is nil,
// and identity is the reason those hooks take a Session at all. So an
// application can construct a useless Session and cannot construct a useful
// one, and a hook that takes one is testable only through a running server.
// Every application that unit-tests a hook otherwise invents the same
// workaround: a second, unexported method taking the values the hook would
// have read, with the exported hook reduced to an adapter over it. This
// library's own counter example carried exactly that split, and its comment was
// a defect report.
//
// Both values are the caller's, deliberately. Deriving the identifier from the
// identity would be tempting and wrong: one subject holds many concurrent
// sessions — that is what Limits.MaxSessionsPerIdentity is about — so two tabs
// belonging to one user need two identifiers and one identity.
//
// A nil identity is a fatal test failure rather than a returned value. It is
// the trap the zero Session already sets, and scaffolding that reproduces the
// trap is not scaffolding.
//
// It takes a [testing.TB] first, matching ReplayN and AssertDirtyComplete, and
// that is a guard and not decoration: reaching this constructor from production
// code means importing a package that links testing and then fabricating a
// testing.TB, which is a visible and absurd act rather than an accident.
func NewSession(tb testing.TB, id live.ID, identity live.Identity) live.Session {
	tb.Helper()
	if identity == nil {
		tb.Fatalf("livetest.NewSession: identity is nil. A Session whose Identity() is nil is the " +
			"trap live.Session{} already sets; pass the identity the hook under test will read.")
		return live.Session{}
	}
	// The assertion cannot fail: live is the only package that assigns this,
	// and internal/arch asserts that it is the only one that can.
	return livebridge.NewSession(id, identity).(live.Session)
}

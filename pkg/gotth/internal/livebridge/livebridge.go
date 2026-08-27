// Package livebridge lets live/livetest construct a value only live can build.
//
// # Why this exists at all
//
// live.Session has unexported fields, so livetest — a different package —
// cannot build one. What anybody can build outside live is live.Session{},
// whose ID() is all-zero and whose Identity() is nil; identity is the reason
// Init, Authorize, Teardown and Execute take a Session at all, so a value
// without one is not a Session worth passing to a hook. The accurate statement
// is that an application can construct a useless Session and cannot construct a
// useful one, and livetest.NewSession answers the second half.
//
// # Why it is a package and not an exported constructor
//
// live could export NewSession and be done. That would put an identity
// constructor in the production package, reachable from any handler, which is a
// materially worse trade than the problem it solves: the whole point of binding
// identity at the handshake is that nothing downstream can mint one.
//
// So live assigns the constructor here at init and livetest reads it. The path
// gotth-live/internal/... is importable by gotth-live/live and
// gotth-live/live/livetest and by nothing outside this module, which is what
// makes the constructor unreachable to a consumer except through livetest —
// where the first parameter is a testing.TB, so calling it from production code
// means fabricating one.
//
// internal/arch asserts that this package's importers are exactly those two.
// The paragraph above is a claim about who imports this, and an unverified
// claim is how one quietly stops being true.
package livebridge

// Identity is live.Identity, redeclared because this package cannot import
// live: live imports this one, to assign NewSession.
//
// The duplication is safe in the direction that matters. Go interfaces are
// structural, so any live.Identity satisfies this and vice versa; if live.Identity
// ever grows a method, this one keeps compiling and keeps meaning less, which
// the assignment in live/livebridge.go would then have to widen deliberately.
type Identity interface {
	// Subject is the stable identifier for the authenticated principal, and
	// the only thing this package needs from an identity: it is what the
	// per-identity session cap counts and what a log record names.
	Subject() string
}

// NewSession builds a live.Session from a session identifier and an identity.
// It is set by live at init.
//
// The result is an any because live.Session is live's type and this package
// cannot name it. That is the whole cost of the indirection, it is paid once,
// and livetest asserts it back immediately — an assertion that cannot fail,
// because live is the only package that assigns this and the architecture test
// is what keeps that true.
//
// The id is a live.ID, which is a [16]byte.
var NewSession func(id [16]byte, identity Identity) any

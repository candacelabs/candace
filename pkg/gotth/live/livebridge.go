package live

import "github.com/candacelabs/candace/pkg/gotth/internal/livebridge"

// This is the whole of live's side of the livetest session constructor: one
// assignment, no exported identifier, and nothing a consumer can reach.
//
// It is an init rather than a var initialiser so that the reason is somewhere a
// reader will find it. A Session is the pair bound at the handshake, and the
// reason nothing downstream can mint one is that the fields are unexported —
// which is also why livetest, a different package, cannot build the Session a
// spec needs to drive Config.Init, Config.Authorize, Config.Teardown or
// Config.Execute directly. Exporting a constructor from this package would
// solve that by putting an identity constructor in every consumer's production
// import graph, which is the trade the handshake exists to refuse.
//
// See internal/livebridge for the containment argument, and internal/arch for
// the assertion that holds it.
func init() {
	livebridge.NewSession = func(id [16]byte, identity livebridge.IIdentity) any {
		return Session{id: ID(id), identity: identity}
	}
}

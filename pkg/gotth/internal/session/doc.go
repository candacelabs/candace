// Package session owns live session state, one goroutine at a time.
//
// The actor is the lock. Each session is a single goroutine and the state it
// holds is reachable from nowhere else, so there is no mutex guarding session
// state anywhere in this library. That goroutine selects over three typed,
// bounded inputs — a mailbox of events and effect results, a channel of
// client acknowledgements, and a heartbeat ticker — and exactly one function
// writes to the mailbox, which is what makes the per-event authorization hook
// impossible to route around.
//
// # One step
//
// A step stamps the event with wall time and any generated identifiers, so
// reducers never call a clock or a random source; reduces under a panic
// guard; marks the fragments the transition may have touched; renders those
// fragments; emits one patch through the outbound validation boundary and the
// framer; and finally hands any effects to the actor boundary, which runs them
// in goroutines scoped to the session context and returns their results as
// ordinary events. Effects never run inside a reducer.
//
// # Backpressure
//
// Patches in flight are bounded by an acknowledgement window that retains
// metadata, never frame bytes. When the window fills, the actor keeps reducing
// but stops rendering, then renders once from current state when an
// acknowledgement re-opens it. Because rendering is pure, the skipped frames
// were never needed — memory under a slow client is proportional to the number
// of fragments, not to the number of pending patches. Sustained pressure
// reaches the application as a synthesized event rather than as a transport
// call, so the reducer stays deterministic and replayable.
//
// # Panics
//
// Go has no supervision tree, so every goroutine this package starts is
// started through one helper that installs a recover, a counter, and the
// shutdown wait-group registration. A bare go statement in this library is a
// defect. A recovered panic contains to one session; a site that panics
// repeatedly closes that session and leaves every other session serving.
//
// # Status
//
// Implemented: the actor and its three bounded inputs, the mailbox and its
// pool, the acknowledgement window, the coalescing flush, the effect boundary,
// the panic guard, the rate budgets, and the provenance record.
package session

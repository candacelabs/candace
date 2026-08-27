package warden

import "time"

// Clock abstracts time so the election and watchdog state machines can be
// tested deterministically with a simulated clock (services/warden/testclock).
// Production code uses NewRealClock.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
	NewTimer(d time.Duration) Timer
	NewTicker(d time.Duration) Ticker
}

// Timer mirrors time.Timer behind an interface.
type Timer interface {
	C() <-chan time.Time
	// Stop prevents the timer from firing; it reports whether it stopped
	// a pending fire (same semantics as time.Timer.Stop).
	Stop() bool
	// Reset re-arms the timer with duration d (same semantics as
	// time.Timer.Reset: only call on stopped/drained timers).
	Reset(d time.Duration) bool
}

// Ticker mirrors time.Ticker behind an interface.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// NewRealClock returns a Clock backed by the time package.
func NewRealClock() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (realClock) NewTimer(d time.Duration) Timer         { return realTimer{time.NewTimer(d)} }
func (realClock) NewTicker(d time.Duration) Ticker       { return realTicker{time.NewTicker(d)} }

type realTimer struct{ t *time.Timer }

func (t realTimer) C() <-chan time.Time        { return t.t.C }
func (t realTimer) Stop() bool                 { return t.t.Stop() }
func (t realTimer) Reset(d time.Duration) bool { return t.t.Reset(d) }

type realTicker struct{ t *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.t.C }
func (t realTicker) Stop()               { t.t.Stop() }

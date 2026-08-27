// Package apptest is the compiled source for docs/guide/testing-your-app.md:
// a small application, and the specs that hold it to the library's contracts.
package apptest

import (
	"context"

	"github.com/a-h/templ"

	"github.com/candacelabs/candace/pkg/gotth/live"
)

const (
	EventInc   = "count.inc"
	EventReset = "count.reset"

	FragmentValue = "count.value"
	FragmentLabel = "count.label"
)

// State is one session's view.
type State struct {
	N      int
	Resets int
}

// Reduce is the pure state transition under test.
func Reduce(s State, ev live.Event) (State, []live.Effect) {
	switch ev.Name {
	case EventInc:
		s.N++
	case EventReset:
		s.N = 0
		s.Resets++
	}
	return s, nil
}

// Config is the application, built by a function so a spec can construct one
// without a running server.
func Config(origins []string) live.Config[State] {
	return live.Config[State]{
		Init:   func(context.Context, live.Session) (State, []live.Effect, error) { return State{}, nil, nil },
		Reduce: Reduce,
		Fragments: []live.Fragment[State]{
			{
				ID:     FragmentValue,
				Render: func(s State) templ.Component { return ValueRegion(s) },
				Dirty:  func(prev, next State) bool { return prev.N != next.N },
			},
			{
				ID:     FragmentLabel,
				Render: func(s State) templ.Component { return LabelRegion(s) },
				// Deliberately narrow, and correct: the label renders Resets
				// and nothing else. Widen it and AssertDirtyComplete still
				// passes — over-declaring is free in correctness. Narrow it to
				// nothing and AssertDirtyComplete fails, which is the point.
				Dirty: func(prev, next State) bool { return prev.Resets != next.Resets },
			},
		},
		Events:       []string{EventInc, EventReset},
		Origins:      origins,
		Authenticate: live.Anonymous,
		Authorize:    Authorize,
		CSRF:         live.NoCSRFCheck,
	}
}

// Authorize is a hook a spec can call directly, given a Session.
func Authorize(_ context.Context, sess live.Session, ev live.Event) error {
	if ev.Name == EventReset && sess.Identity().Subject() != "admin" {
		return &live.DenyError{Reason: "only an admin may reset"}
	}
	return nil
}

// Admin is an identity a spec can bind a session to.
type Admin struct{}

func (Admin) Subject() string { return "admin" }

// Guest is the other one.
type Guest struct{}

func (Guest) Subject() string { return "guest" }

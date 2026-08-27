package livetest

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/candacelabs/candace/pkg/gotth/live"
)

// ReplayN replays an event log against a reducer n times and fails unless the
// resulting state and the emitted effects are identical on every run.
//
// It is the determinism harness the library requires rather than suggests. A
// reducer that reads a clock, a random source, or the iteration order of a map
// fails it, and those three are the whole of what usually goes wrong: nothing
// else in a pure function of two values can differ between runs.
//
// The comparison is deep, so effects declared as data are compared by value —
// which is why effects must be plain values and not closures over live handles.
func ReplayN[S any](tb testing.TB, reduce live.Reducer[S], initial S, log []live.Event, n int) {
	tb.Helper()

	if n < 2 {
		tb.Fatalf("livetest.ReplayN needs at least 2 replays to compare, got %d", n)
	}
	if len(log) == 0 {
		tb.Fatal("livetest.ReplayN was given an empty event log: replaying nothing proves nothing")
	}

	wantState, wantEffects := fold(reduce, initial, log)

	for run := 2; run <= n; run++ {
		gotState, gotEffects := fold(reduce, initial, log)

		if !reflect.DeepEqual(wantState, gotState) {
			tb.Fatalf("livetest.ReplayN: replay %d produced different state than replay 1.\n"+
				"  replay 1: %#v\n  replay %d: %#v\n"+
				"A reducer must be a pure function of (state, event). The usual causes are a clock, "+
				"a random source, or ranging over a map.", run, wantState, run, gotState)
		}
		if !reflect.DeepEqual(wantEffects, gotEffects) {
			tb.Fatalf("livetest.ReplayN: replay %d produced different effects than replay 1.\n"+
				"  replay 1: %#v\n  replay %d: %#v\n"+
				"Effects must be declared as values a test can compare, never closures over live handles.",
				run, wantEffects, run, gotEffects)
		}
	}
}

func fold[S any](reduce live.Reducer[S], initial S, log []live.Event) (S, []live.Effect) {
	state := initial
	var effects []live.Effect
	for _, ev := range log {
		var produced []live.Effect
		state, produced = reduce(state, ev)
		effects = append(effects, produced...)
	}
	return state, effects
}

// AssertDirtyComplete replays a log against a configuration and fails if any
// fragment declared itself unchanged while its rendered bytes moved.
//
// Under-declaring is the one rendering mistake that produces a stale region in
// production and nothing at all in development, because the fragment is
// usually re-rendered by some other transition before anybody looks. This is
// what catches it before merge.
//
// Over-declaring is not a failure: a render whose bytes did not change is
// suppressed, so declaring too much costs a comparison and nothing else.
func AssertDirtyComplete[S any](tb testing.TB, cfg live.Config[S], initial S, log []live.Event) {
	tb.Helper()

	if len(cfg.Fragments) == 0 {
		tb.Fatal("livetest.AssertDirtyComplete: the configuration declares no fragments")
	}
	if cfg.Reduce == nil {
		tb.Fatal("livetest.AssertDirtyComplete: the configuration declares no reducer")
	}

	ctx := context.Background()
	previous := make([]string, len(cfg.Fragments))
	for i, f := range cfg.Fragments {
		html, err := renderFragment(ctx, f, initial)
		if err != nil {
			tb.Fatalf("livetest.AssertDirtyComplete: rendering fragment %q failed: %v", f.ID, err)
		}
		previous[i] = html
	}

	state := initial
	for step, ev := range log {
		prev := state
		state, _ = cfg.Reduce(state, ev)

		for i, f := range cfg.Fragments {
			html, err := renderFragment(ctx, f, state)
			if err != nil {
				tb.Fatalf("livetest.AssertDirtyComplete: rendering fragment %q failed: %v", f.ID, err)
			}
			changed := html != previous[i]
			previous[i] = html

			if !changed || f.Dirty == nil {
				continue
			}
			if !f.Dirty(prev, state) {
				tb.Fatalf("livetest.AssertDirtyComplete: fragment %q declared itself unchanged at event %d (%q), "+
					"but its markup moved.\n  before: %s\n  after:  %s\n"+
					"Widen the fragment's Dirty function, or set it to nil to re-render on every transition.",
					f.ID, step, ev.Name, previous[i], html)
			}
		}
	}
}

func renderFragment[S any](ctx context.Context, f live.Fragment[S], state S) (string, error) {
	component := f.Render(state)
	if component == nil {
		return "", fmt.Errorf("fragment %q rendered no component: its Render returned nil, and a live "+
			"region has to return a templ component for every state — return an empty one rather "+
			"than nil for the state that has nothing to show", f.ID)
	}
	var buf bytes.Buffer
	if err := component.Render(ctx, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

package livetest_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/candacelabs/candace/pkg/gotth/live"
	"github.com/candacelabs/candace/pkg/gotth/live/livetest"
)

type tester string

func (t tester) Subject() string { return string(t) }

var _ = Describe("NewSession", func() {
	// The gap this closes, stated as the thing that was actually wrong: a
	// Session an application can build itself has a nil Identity, and identity
	// is the entire reason the hooks take a Session.
	It("is the difference between a Session and a useful one", func() {
		zero := live.Session{}
		Expect(zero.Identity()).To(BeNil())
		Expect(zero.ID()).To(Equal(live.ID{}))

		s := livetest.NewSession(GinkgoTB(), live.ID{1, 2, 3}, tester("alice"))

		Expect(s.ID()).To(Equal(live.ID{1, 2, 3}))
		Expect(s.Identity().Subject()).To(Equal("alice"))
	})

	// One subject, many sessions — which is what Limits.MaxSessionsPerIdentity
	// is about, and why the identifier is the caller's rather than derived from
	// the identity. Two tabs belonging to one user is the first thing the chat
	// example needs.
	It("gives one identity as many distinct sessions as the caller asks for", func() {
		user := tester("alice")

		first := livetest.NewSession(GinkgoTB(), live.ID{0xa}, user)
		second := livetest.NewSession(GinkgoTB(), live.ID{0xb}, user)

		Expect(first.ID()).NotTo(Equal(second.ID()))
		Expect(first.Identity().Subject()).To(Equal(second.Identity().Subject()))
	})

	It("fails the test rather than handing back the trap it exists to avoid", func() {
		r := run(func(tb testing.TB) { livetest.NewSession(tb, live.ID{1}, nil) })

		Expect(r.failed).To(BeTrue())
		Expect(r.message).To(ContainSubstring("identity is nil"))
	})

	// The point of the helper: a Config hook that takes a Session becomes
	// callable from a spec, with no running server and no adapter invented by
	// the application to work around the gap.
	It("makes a Config hook callable without a server", func() {
		var seen live.Session
		cfg := live.Config[counter]{
			Init: func(_ context.Context, s live.Session) (counter, []live.IEffect, error) {
				seen = s
				return counter{Label: s.Identity().Subject()}, nil, nil
			},
		}

		state, effects, err := cfg.Init(context.Background(),
			livetest.NewSession(GinkgoTB(), live.ID{7}, tester("bob")))

		Expect(err).NotTo(HaveOccurred())
		Expect(effects).To(BeEmpty())
		Expect(state.Label).To(Equal("bob"))
		Expect(seen.ID()).To(Equal(live.ID{7}))
	})
})

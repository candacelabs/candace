package payments_test

import (
	"context"
	"errors"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/candacelabs/candace/pkg/gotth/docs/guide/_samples/payments"
	"github.com/candacelabs/candace/pkg/gotth/live"
)

// These specs are the executable half of
// docs/guide/effects-and-server-push.md's idempotency section. FR-77(b) names
// two distinct ways an application meets a double execution, and there is one
// Describe for each, plus one for the mistake that makes both of them charge
// twice.
//
// Every arm asserts on KeyedProvider.Made — how many charges the third party
// actually made — because that is the number the customer's statement shows,
// and every other assertion here is a proxy for it.

const (
	orderID     = "ord_8812"
	amountCents = 4_990
)

// open is the state a fresh session mounts with: whatever Init read back from
// the application's order store.
func open() payments.State {
	return payments.State{OrderID: orderID, AmountCents: amountCents, Status: payments.StatusOpen}
}

func payEvent(id uint64) live.Event {
	return live.Event{ID: id, Name: payments.EventPay}
}

// drain runs every effect a transition produced through the executor.
func drain(g *payments.Gateway, effects []live.Effect, emit live.Emitter) []error {
	var errs []error
	for _, effect := range effects {
		if err := g.Execute(context.Background(), live.Session{}, effect, emit); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// discard is an Emitter that accepts everything, standing in for a session with
// room in its mailbox.
func discard(live.Event) error { return nil }

var _ = Describe("a sender who genuinely sent twice", func() {
	// FR-77(b)'s first path. The frames are not duplicates the library
	// produced — the client has no queue and no resend, so a second frame is
	// always somebody's second intent — and the library does not collapse them.

	It("charges once for a double-click, and the in-session guard is what stops it", func() {
		provider := payments.NewKeyedProvider()
		gateway := &payments.Gateway{Provider: provider}

		charging, first := payments.Reduce(open(), payEvent(1))
		after, second := payments.Reduce(charging, payEvent(2))

		Expect(first).To(HaveLen(1), "the first click must schedule a charge")
		Expect(second).To(BeEmpty(),
			"the second click of a double-click saw StatusCharging and scheduled nothing")
		Expect(after.Status).To(Equal(payments.StatusCharging))

		Expect(drain(gateway, append(first, second...), discard)).To(BeEmpty())
		Expect(provider.Made()).To(Equal(1))
	})

	It("charges once when the two clicks are in two tabs, where the guard cannot help", func() {
		// The guard is per session and two tabs are two sessions with two
		// independent States, so both of them schedule a charge. This is the arm
		// that shows the guard is not the mechanism: only the key is.
		provider := payments.NewKeyedProvider()
		gateway := &payments.Gateway{Provider: provider}

		_, tabA := payments.Reduce(open(), payEvent(1))
		_, tabB := payments.Reduce(open(), payEvent(1))

		Expect(tabA).To(HaveLen(1))
		Expect(tabB).To(HaveLen(1))
		Expect(tabA).To(Equal(tabB), "both tabs derived the same key from the same order")

		Expect(drain(gateway, append(tabA, tabB...), discard)).To(BeEmpty())
		Expect(provider.Made()).To(Equal(1))
	})
})

var _ = Describe("an effect that committed while its patch never reached the client", func() {
	// FR-77(b)'s second path, and the one at-most-once delivery does not solve.
	// One intent, two executions, and nothing the library can do about it.

	It("charges once when the emit fails and the customer presses Pay again", func() {
		provider := payments.NewKeyedProvider()
		gateway := &payments.Gateway{Provider: provider}

		// The money moves and the session never learns: a full mailbox here, a
		// dropped connection in the field. Same shape either way.
		state, effects := payments.Reduce(open(), payEvent(1))
		errs := drain(gateway, effects, func(live.Event) error {
			return errors.New("mailbox full")
		})

		Expect(errs).To(HaveLen(1))
		Expect(live.IsRetryable(errs[0])).To(BeTrue(),
			"the executor may claim transience precisely because it passed a key")
		Expect(provider.Made()).To(Equal(1))
		Expect(state.Status).To(Equal(payments.StatusCharging),
			"the session is left believing a charge is in flight, which is the leak")

		// The customer reloads. The session died with its connection, so this is
		// a fresh mount, and Init rebuilt the order as still unpaid — because
		// nothing recorded the charge.
		retryState, retryEffects := payments.Reduce(open(), payEvent(2))
		Expect(retryEffects).To(Equal(effects),
			"a fresh session mints the same key from the same order")

		Expect(drain(gateway, retryEffects, discard)).To(BeEmpty())
		Expect(provider.Made()).To(Equal(1), "the provider answered with the charge it already made")
		Expect(retryState.Status).To(Equal(payments.StatusCharging))
	})

	It("charges once when a retryable failure re-schedules the charge", func() {
		provider := payments.NewKeyedProvider()
		gateway := &payments.Gateway{Provider: provider}

		state, effects := payments.Reduce(open(), payEvent(1))
		Expect(drain(gateway, effects, discard)).To(BeEmpty())

		failed := live.Event{
			ID:   2,
			Name: live.EffectFailedEvent,
			Fields: live.NewFields(map[string]string{
				live.EffectFailedSourceField:    payments.SourceCharge,
				live.EffectFailedRetryableField: "true",
				live.EffectFailedErrorField:     "the session did not learn",
			}),
		}
		_, retry := payments.Reduce(state, failed)

		Expect(retry).To(Equal(effects))
		Expect(drain(gateway, retry, discard)).To(BeEmpty())
		Expect(provider.Made()).To(Equal(1))
	})

	It("reopens the checkout on a terminal failure, which is safe for the same reason", func() {
		failed := live.Event{
			ID:   2,
			Name: live.EffectFailedEvent,
			Fields: live.NewFields(map[string]string{
				live.EffectFailedSourceField: payments.SourceCharge,
			}),
		}
		state, _ := payments.Reduce(open(), payEvent(1))
		reopened, effects := payments.Reduce(state, failed)

		Expect(effects).To(BeEmpty())
		Expect(reopened.Status).To(Equal(payments.StatusOpen))
	})
})

var _ = Describe("keying on the event instead of the order", func() {
	// The mutation that makes both paths false. It is here rather than in prose
	// because "do not do this" is worth exactly as much as the failure it is
	// shown to produce.

	It("charges twice, because two clicks are two events with two identifiers", func() {
		provider := payments.NewKeyedProvider()
		gateway := &payments.Gateway{Provider: provider}

		var effects []live.Effect
		for _, id := range []uint64{1, 2} {
			effects = append(effects, payments.ChargeEffect{
				OrderID:     orderID,
				AmountCents: amountCents,
				Key:         strconv.FormatUint(id, 10),
			})
		}

		Expect(drain(gateway, effects, discard)).To(BeEmpty())
		Expect(provider.Made()).To(Equal(2),
			"a key derived from the event is a different key per click, which is no key at all")
	})
})

var _ = Describe("the key itself", func() {
	It("is a function of the order and of nothing else", func() {
		Expect(payments.IdempotencyKey(orderID, amountCents)).
			To(Equal(payments.IdempotencyKey(orderID, amountCents)))
		Expect(payments.IdempotencyKey(orderID, amountCents)).
			NotTo(Equal(payments.IdempotencyKey(orderID, amountCents+1)))
		Expect(payments.IdempotencyKey(orderID, amountCents)).
			NotTo(Equal(payments.IdempotencyKey("ord_8813", amountCents)))
	})
})

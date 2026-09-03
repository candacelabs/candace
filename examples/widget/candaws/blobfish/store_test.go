package blobfish

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The specifications below run a real store: a bucket, a coordinator, three
// zone goroutines and a repairer, against real timers. They are paced far
// faster than the demo is, because everything here is a question about the
// quorum rather than about what a picture looks like.
const (
	specDelay    = 5 * time.Millisecond
	specCadence  = 25 * time.Millisecond
	specPatience = 15 * time.Second
)

// specConfig is the pace every specification in this file runs at. The slow
// zone is the parameter, because a store whose zones are all the same speed and
// one that has a zone the quorum does not wait for are the two states this
// engine exists to tell apart.
func specConfig(slowZone int) Config {
	return Config{
		StorageClass:   "Glacial",
		Cadence:        specCadence,
		ReplicaDelay:   specDelay,
		DelaySpread:    2 * specDelay,
		SlowZone:       slowZone,
		SlowFactor:     20,
		Patience:       12 * specDelay,
		RepairInterval: 4 * specCadence,
		Seed:           20260902,
	}
}

// stream is one subscriber, read only by the specification's own goroutine.
type stream struct {
	views <-chan StoreView
	seen  []StoreView
}

// await takes views until one matches, checking on the way that no view ever
// claimed a quorum wider than the zone set.
func (subscriber *stream) await(what string, match func(view StoreView) bool) StoreView {
	GinkgoHelper()
	deadline := time.After(specPatience)
	for {
		select {
		case view, open := <-subscriber.views:
			Expect(open).To(BeTrue(), "the store stopped before %s", what)
			Expect(view.WriteAcks).To(BeNumerically("<=", zoneCount))
			Expect(view.LaggingZones).To(BeNumerically("<", zoneCount),
				"every zone cannot be behind every zone")
			subscriber.seen = append(subscriber.seen, view)
			if match(view) {
				return view
			}
		case <-deadline:
			Fail("never saw " + what + " within " + specPatience.String())
			return StoreView{}
		}
	}
}

// storeUnder starts a store for the length of one specification, with one
// subscriber attached, and joins its goroutines on the way out.
func storeUnder(config Config) (*Store, *stream) {
	GinkgoHelper()

	store, buildError := NewStore(config)
	Expect(buildError).ToNot(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() { stopped <- store.Run(ctx) }()
	DeferCleanup(func() {
		cancel()
		Eventually(stopped, specPatience).Should(Receive())
	})

	views, subscribeError := store.Watch(ctx)
	Expect(subscribeError).ToNot(HaveOccurred())
	return store, &stream{views: views}
}

var _ = Describe("A store that is configured wrongly", func() {
	It("reports the fault rather than running and never acknowledging", func() {
		sound := specConfig(-1)
		for _, broken := range []struct {
			what   string
			change func(config *Config)
			fault  error
		}{
			{"no storage class", func(config *Config) { config.StorageClass = "" }, ErrStorageClass},
			{"no cadence", func(config *Config) { config.Cadence = 0 }, ErrCadence},
			{"no zone latency", func(config *Config) { config.ReplicaDelay = 0 }, ErrReplicaDelay},
			{"a slow zone that is not a zone",
				func(config *Config) { config.SlowZone = zoneCount }, ErrSlowZone},
			{"a slow zone that is not slower",
				func(config *Config) { config.SlowFactor = 0 }, ErrSlowFactor},
			{"a coordinator that gives up before a healthy zone can answer",
				func(config *Config) { config.Patience = time.Nanosecond }, ErrPatience},
			{"no repair sweep",
				func(config *Config) { config.RepairInterval = 0 }, ErrRepairInterval},
		} {
			config := sound
			broken.change(&config)
			_, buildError := NewStore(config)
			Expect(buildError).To(MatchError(broken.fault), "for %s", broken.what)
		}
	})

	It("refuses a second Run rather than minting two generation counters", func() {
		store, _ := storeUnder(specConfig(-1))
		Expect(store.Run(context.Background())).To(MatchError(ContainSubstring("already running")))
	})
})

var _ = Describe("A store whose zones are all the same speed", func() {
	It("acknowledges a write, serves a read, and finds nothing to repair", func() {
		_, subscriber := storeUnder(specConfig(-1))

		durable := subscriber.await("a durable write",
			func(view StoreView) bool { return view.Writable && view.Generation >= 2 })
		Expect(durable.Durable()).To(BeTrue())
		Expect(durable.Serving()).To(BeTrue())
		Expect(durable.StorageClass).To(Equal("Glacial"),
			"the storage class is a string the card renders and never computes")
		Expect(durable.Objects).To(BeNumerically(">=", 1))
	})
})

var _ = Describe("A store with a zone the quorum does not wait for", func() {
	It("replies at the quorum and stops counting", func() {
		// The third zone is twenty times slower than the coordinator's whole
		// patience allows for, so it cannot be one of the two that answered.
		// The write is durable anyway, and that is the entire point: the
		// coordinator replied and never un-replied.
		_, subscriber := storeUnder(specConfig(zoneCount - 1))

		durable := subscriber.await("a write acknowledged by a quorum",
			func(view StoreView) bool { return view.Writable })
		Expect(durable.WriteAcks).To(Equal(quorum),
			"stopping at the quorum is a select that stopped counting, not a timeout")
		Expect(durable.WriteAcks).To(BeNumerically("<", zoneCount))
	})

	It("finds the zone behind and repairs it forward", func() {
		_, subscriber := storeUnder(specConfig(zoneCount - 1))

		behind := subscriber.await("a repair sweep that found a zone behind",
			func(view StoreView) bool { return view.LaggingZones >= 1 })
		Expect(behind.LaggingZones).To(Equal(1),
			"one zone is slow and two are not, so exactly one is ever behind")

		// The generation the card ticks on only ever goes forward, whatever the
		// repairer is doing behind it.
		highest := uint64(0)
		for _, view := range subscriber.seen {
			Expect(view.Generation).To(BeNumerically(">=", highest),
				"a generation is never reused for a key, so it never walks backwards")
			highest = view.Generation
		}
	})
})

package patience

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestPatience(t *testing.T) {
	gomega.RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/patience suite")
}

// gomega is imported under its own name rather than dot-imported, which is the
// convention everywhere else in this module. It has to be here: this package
// declares Consistently itself, and a dot import would be a redeclaration.
// That collision is worth leaving in place rather than renaming around — the
// two names mean the same thing on purpose, and this package is the typed
// shell over that one.

// recordingReporter is an [IReporter] that records instead of aborting.
//
// This is the reason IReporter exists rather than the signatures taking
// testing.TB: testing.TB cannot be implemented outside the standard library,
// and a real *testing.T aborts the spec on the first Fatalf, so the failing
// half of this package — every message a reader will ever actually see —
// would have been untestable.
type recordingReporter struct {
	helped   int
	failures []string
}

func (reporter *recordingReporter) Helper() { reporter.helped++ }

func (reporter *recordingReporter) Fatalf(format string, arguments ...any) {
	reporter.failures = append(reporter.failures, fmt.Sprintf(format, arguments...))
}

// failed is the whole recorded failure text, which is what the specifications
// assert against: a message is only useful if the thing a reader needs is in
// it, wherever the engine chose to put it.
func (reporter *recordingReporter) failed() string {
	return fmt.Sprint(reporter.failures)
}

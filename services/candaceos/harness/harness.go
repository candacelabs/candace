// Package harness defines the public behavior boundary between CandaceOS Core
// and a compiled-in agent runtime implementation.
package harness

import (
	"context"
	"fmt"
	"strings"

	candaceosv1 "github.com/candacelabs/candace/proto/candace/candaceos/v1"
)

// BackendName projects the generated HarnessBackend enum into its stable
// configuration and diagnostic spelling.
func BackendName(backend candaceosv1.HarnessBackend) string {
	value := backend.Descriptor().Values().ByNumber(backend.Number())
	if value == nil {
		return fmt.Sprintf("unknown-%d", backend)
	}
	name := strings.TrimPrefix(string(value.Name()), "HARNESS_BACKEND_")
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// Factory constructs one harness runtime against Core-owned host services.
// Core passes an owned context snapshot and keeps Host valid until the returned
// runtime is closed; implementations must not mutate protobuf inputs.
type Factory interface {
	New(harnessContext *candaceosv1.HarnessContext, host Host) (*Instance, error)
}

// FactoryFunc adapts a function to Factory.
type FactoryFunc func(harnessContext *candaceosv1.HarnessContext, host Host) (*Instance, error)

// New implements Factory.
func (factory FactoryFunc) New(
	harnessContext *candaceosv1.HarnessContext,
	host Host,
) (*Instance, error) {
	return factory(harnessContext, host)
}

// Instance binds a runtime to the immutable identity recorded by Core.
type Instance struct {
	Runtime  Runtime
	Identity *candaceosv1.HarnessRuntimeIdentity
}

// Runtime owns one harness session and its turn lifecycle. Core calls Start
// once, then Activate once after a successful start, and serializes Send and
// Abort. Close may overlap an in-flight call, so implementations must
// synchronize provider resources, respect each call's context, and make Close
// idempotent, including after a failed Start or Activate.
type Runtime interface {
	Start(ctx context.Context) (*candaceosv1.HarnessSession, error)
	Activate(ctx context.Context) error
	Send(ctx context.Context, prompt *candaceosv1.HarnessPrompt) error
	Abort(ctx context.Context) error
	Close() error
}

// Host is the Core-owned capability surface available to a harness. Core
// retains fleet observation, approval, fencing, and reconciliation authority.
// Its methods are safe for concurrent provider callbacks, snapshot protobuf
// inputs before returning, and honor cancellation. Reconcile may remain blocked
// while Core waits for operator approval.
type Host interface {
	Publish(ctx context.Context, event *candaceosv1.HarnessEvent) error
	FleetStatus(ctx context.Context) (*candaceosv1.HarnessFleetStatus, error)
	Reconcile(
		ctx context.Context,
		request *candaceosv1.HarnessReconcileRequest,
	) (*candaceosv1.ReconcileEvidence, error)
}

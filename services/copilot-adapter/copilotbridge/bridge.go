// Package copilotbridge is the concrete ICopilotBridge over the Copilot Go SDK.
// It is the one place the adapter touches github.com/github/copilot-sdk/go and
// it cannot run without a Copilot CLI on the host, which is why the seam above
// it is what the specs exercise.
package copilotbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
	"github.com/google/uuid"

	copilotadapter "github.com/candacelabs/candace/services/copilot-adapter"
	api "github.com/candacelabs/candace/services/copilot-adapter/gen/api"
)

const (
	// systemMessageModeAppend is the SDK's SystemMessageConfig.Mode that adds
	// the adapter's instructions to the CLI's own system message.
	systemMessageModeAppend = "append"
	// messageModeImmediate is the SDK's MessageOptions.Mode that interrupts
	// the work in flight; it is what the contract calls steer.
	messageModeImmediate = "immediate"
	// permissionFallbackToolName is the tool name a permission request carries
	// when the SDK payload names none.
	permissionFallbackToolName copilotadapter.ToolName = "permission"
)

// permissionToolNameFields are the JSON field names a permission request may
// carry its tool name under, in the order they are read.
var permissionToolNameFields = []string{"toolName", "tool_name", "kind"}

var permissionToolCallIDFields = []string{"toolCallId", "tool_call_id"}

// Config is what the binary hands New.
type Config struct {
	GitHubToken      string
	WorkingDirectory string
	Logger           *slog.Logger
	// ShutdownTimeout bounds process-owner cleanup after the adapter has made
	// its per-session Disconnect attempts.
	ShutdownTimeout time.Duration
	// MCPServers are caller-owned tool transports shared by created and restored sessions.
	MCPServers map[string]copilot.MCPServerConfig
}

// CopilotBridge drives one Copilot CLI process for every adapter session.
type CopilotBridge struct {
	client             *copilot.Client
	mcpServers         map[string]copilot.MCPServerConfig
	logger             *slog.Logger
	shutdownTimeout    time.Duration
	forceStop          func()
	listModels         func(ctx context.Context, params *rpc.ModelsListRequest) (*rpc.ModelList, error)
	getSessionMetadata func(ctx context.Context, sessionID string) (*copilot.SessionMetadata, error)
	resumeSession      func(ctx context.Context, sessionID string, config *copilot.ResumeSessionConfig) (*copilot.Session, error)
	disconnects        *disconnectTracker
	closeOnce          sync.Once
	closeErr           error
}

// NewCopilotBridge starts the CLI client. The CLI is spawned headless over stdio by the SDK.
func NewCopilotBridge(ctx context.Context, config Config) (*CopilotBridge, error) {
	if config.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("copilot bridge: shutdown timeout must be positive")
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	client := copilot.NewClient(&copilot.ClientOptions{
		GitHubToken:      config.GitHubToken,
		WorkingDirectory: config.WorkingDirectory,
	})
	if err := client.Start(ctx); err != nil {
		return nil, fmt.Errorf("copilot bridge: start the CLI client: %w", err)
	}
	return &CopilotBridge{
		client: client, logger: logger, shutdownTimeout: config.ShutdownTimeout, mcpServers: config.MCPServers,
		listModels: client.RPC.Models.List, forceStop: client.ForceStop, getSessionMetadata: client.GetSessionMetadata,
		resumeSession: client.ResumeSession, disconnects: newDisconnectTracker(),
	}, nil
}

// Close stops the CLI process and gives every tracked Disconnect worker one
// final bounded drain window. Copilot SDK v1.0.11's graceful Client.Stop and
// Session.Disconnect both contain unbounded context.Background requests, so
// process ownership deliberately ends through ForceStop after the adapter has
// already attempted bounded per-session cleanup. If the SDK worker still does
// not return, Close reports the configured deadline; that worker remains
// counted by disconnectTracker until process exit instead of blocking shutdown.
func (bridge *CopilotBridge) Close() error {
	bridge.closeOnce.Do(func() {
		// ForceStop is the SDK's documented escape hatch, but it has no context
		// either. Count it beside Disconnect so even an SDK-internal lock bug
		// cannot make this process-owner method wait forever.
		if !bridge.disconnects.begin() {
			bridge.closeErr = errBridgeClosing
			return
		}
		go func() {
			defer bridge.disconnects.done()
			bridge.forceStop()
		}()
		drained := bridge.disconnects.stop()
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), bridge.shutdownTimeout)
		defer cancelShutdown()
		select {
		case <-drained:
		case <-shutdownContext.Done():
			bridge.closeErr = fmt.Errorf(
				"copilot bridge: %d SDK shutdown worker(s) did not drain after force stop: %w",
				bridge.disconnects.activeCount(), shutdownContext.Err(),
			)
		}
	})
	return bridge.closeErr
}

// ListModels reports the models the CLI can use.
func (bridge *CopilotBridge) ListModels(ctx context.Context) ([]copilotadapter.BridgeModel, error) {
	// The convenience ListModels caches its first success until disconnect.
	// Let the CLI own catalog caching so refresh can recover an early Auto-only response.
	catalog, err := bridge.listModels(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]copilotadapter.BridgeModel, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		result = append(result, copilotadapter.BridgeModel{
			ID: model.ID, DisplayName: model.Name, Capabilities: supportedCapabilities(model.Capabilities),
		})
	}
	return result, nil
}

// supportedCapabilities explicitly translates the pinned SDK's optional
// support flags into the independently owned OpenAPI vocabulary. New SDK
// fields remain private until the public contract deliberately adopts them.
func supportedCapabilities(capabilities rpc.ModelCapabilities) []api.ModelCapability {
	names := make([]api.ModelCapability, 0, 2)
	if capabilities.Supports == nil {
		return names
	}
	if capabilities.Supports.Vision != nil && *capabilities.Supports.Vision {
		names = append(names, api.Vision)
	}
	if capabilities.Supports.ReasoningEffort != nil && *capabilities.Supports.ReasoningEffort {
		names = append(names, api.Reasoning)
	}
	return names
}

// CreateSession opens a CLI session and returns the data-shaped handle the
// adapter drives it through.
func (bridge *CopilotBridge) CreateSession(ctx context.Context, spec copilotadapter.BridgeSessionSpec) (copilotadapter.BridgeSession, error) {
	events := make(chan copilotadapter.BridgeEvent, 256)
	turns := newTurnCorrelator()
	resolutions := newResolutionRegistry()
	aborts := &turnTerminationWaiter{}
	translator := newEventTranslator(events, turns, aborts.publish)
	translator.handlePermissions(spec.SessionID, resolutions)
	streaming := true
	configuration := &copilot.SessionConfig{
		SessionID:           spec.SessionID.String(),
		Model:               spec.Model,
		WorkingDirectory:    spec.WorkingDirectory,
		Streaming:           &streaming,
		OnPermissionRequest: permissionHandler(spec.PermissionPolicy),
		OnEvent:             translator.handle,
		MCPServers:          bridge.mcpServers,
	}
	if spec.SystemInstructions != "" {
		configuration.SystemMessage = &copilot.SystemMessageConfig{Mode: systemMessageModeAppend, Content: spec.SystemInstructions}
	}
	session, err := bridge.client.CreateSession(ctx, configuration)
	if err != nil {
		turns.stop()
		resolutions.stop()
		return copilotadapter.BridgeSession{}, fmt.Errorf("copilot bridge: create session: %w", err)
	}
	resolutions.bind(session.RPC.Permissions.HandlePendingPermissionRequest)
	return bridge.sessionHandle(session, events, turns, resolutions, aborts), nil
}

// ResumeSession reconnects a persisted, nonterminal adapter session after the
// process restarts. The SDK receives the same model, working directory,
// instructions, request callbacks, and event hook as a newly created session.
func (bridge *CopilotBridge) ResumeSession(ctx context.Context, spec copilotadapter.BridgeSessionSpec) (copilotadapter.BridgeSession, error) {
	missing, err := bridge.sessionMissing(ctx, spec.SessionID.String())
	if err != nil {
		return copilotadapter.BridgeSession{}, fmt.Errorf("copilot bridge: inspect session before resume: %w", err)
	}
	if missing {
		return copilotadapter.BridgeSession{}, fmt.Errorf("%w: %s", copilotadapter.ErrBridgeSessionMissing, spec.SessionID)
	}
	events := make(chan copilotadapter.BridgeEvent, 256)
	turns := newRestoredTurnCorrelator(spec.RestoredTurns)
	resolutions := newResolutionRegistry()
	aborts := &turnTerminationWaiter{}
	translator := newEventTranslator(events, turns, aborts.publish)
	translator.handlePermissions(spec.SessionID, resolutions)
	streaming := true
	// The pinned SDK cannot rehydrate an interrupted permission request. With
	// pending work enabled it resumes into a permanently blocked session while
	// emitting neither the request nor a terminal event. The adapter reconciles
	// every interrupted durable turn before this call, so explicitly discard
	// that unrecoverable SDK work and keep the session usable for new prompts.
	continuePending := false
	configuration := &copilot.ResumeSessionConfig{
		Model:               spec.Model,
		WorkingDirectory:    spec.WorkingDirectory,
		Streaming:           &streaming,
		OnPermissionRequest: permissionHandler(spec.PermissionPolicy),
		OnEvent:             translator.handle,
		MCPServers:          bridge.mcpServers,
		ContinuePendingWork: &continuePending,
	}
	if spec.SystemInstructions != "" {
		configuration.SystemMessage = &copilot.SystemMessageConfig{Mode: systemMessageModeAppend, Content: spec.SystemInstructions}
	}
	session, err := bridge.resumeSession(ctx, spec.SessionID.String(), configuration)
	if err != nil {
		turns.stop()
		resolutions.stop()
		resumeErr := fmt.Errorf("copilot bridge: resume session: %w", err)
		missing, metadataErr := bridge.sessionMissing(ctx, spec.SessionID.String())
		if metadataErr != nil {
			return copilotadapter.BridgeSession{}, errors.Join(
				resumeErr,
				fmt.Errorf("copilot bridge: confirm session after resume failure: %w", metadataErr),
			)
		}
		if missing {
			return copilotadapter.BridgeSession{}, errors.Join(
				fmt.Errorf("%w: %s", copilotadapter.ErrBridgeSessionMissing, spec.SessionID),
				resumeErr,
			)
		}
		return copilotadapter.BridgeSession{}, resumeErr
	}
	resolutions.bind(session.RPC.Permissions.HandlePendingPermissionRequest)
	return bridge.sessionHandle(session, events, turns, resolutions, aborts), nil
}

func (bridge *CopilotBridge) sessionMissing(ctx context.Context, sessionID string) (bool, error) {
	metadata, err := bridge.getSessionMetadata(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return metadata == nil, nil
}

func (bridge *CopilotBridge) sessionHandle(
	session *copilot.Session,
	events chan copilotadapter.BridgeEvent,
	turns *turnCorrelator,
	resolutions *resolutionRegistry,
	aborts *turnTerminationWaiter,
) copilotadapter.BridgeSession {
	disconnect := newDisconnectOperation(bridge.disconnects, session.Disconnect)
	sender := newTurnSender(turns, func(ctx context.Context, prompt copilotadapter.BridgePrompt) error {
		text := prompt.Text
		if prompt.Author != "" {
			text = prompt.Author + ": " + prompt.Text
		}
		options := copilot.MessageOptions{Prompt: text}
		if prompt.Mode == string(api.Steer) {
			options.Mode = messageModeImmediate
		}
		_, err := session.Send(ctx, options)
		return err
	})
	return copilotadapter.BridgeSession{
		Events: events,
		UsageHistory: func(ctx context.Context) ([]copilotadapter.BridgeEvent, error) {
			history, err := session.GetEvents(ctx)
			if err != nil {
				return nil, err
			}
			var usage []copilotadapter.BridgeEvent
			for _, source := range history {
				// Replay only measurements; never replay control events into the live correlator.
				switch source.Data.(type) {
				case *rpc.SessionUsageCheckpointData, *rpc.SessionShutdownData, *rpc.AssistantUsageData:
					translated := translateEvents(source, nil, nil)
					for index, event := range translated {
						if event.Kind != copilotadapter.BridgeEventUsage {
							continue
						}
						event.ID = translatedEventID(source.ID, event, index, len(translated))
						event.UsagePayload, _ = json.Marshal(source)
						usage = append(usage, event)
					}
				}
			}
			return usage, nil
		},
		Send:                sender.send,
		AcknowledgeDelivery: turns.acknowledgeDelivery,
		ActiveTurn:          turns.active,
		Abort: func(ctx context.Context, expectedTurnID uuid.UUID) (uuid.UUID, error) {
			return sender.abort(ctx, expectedTurnID, session.Abort, aborts)
		},
		SetModel: func(ctx context.Context, model string) error {
			return session.SetModel(ctx, model, nil)
		},
		Resolve: func(ctx context.Context, resolution copilotadapter.BridgeResolution) error {
			return resolutions.resolve(ctx, resolution)
		},
		AcknowledgeResolution: resolutions.acknowledge,
		AbandonResolution:     resolutions.abandon,
		Close: func(ctx context.Context) error {
			turns.stop()
			resolutions.stop()
			return disconnect.wait(ctx)
		},
	}
}

var errBridgeClosing = errors.New("copilot bridge: client shutdown already started")

// keepPermissionPending advertises the SDK permission-event capability while
// deliberately sending no decision. The SDK v1.0.11 wire config only requests
// permission events when this callback is non-nil; exact identity and delivery
// remain owned by PermissionRequestedData and HandlePendingPermissionRequest.
func keepPermissionPending(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
	return &rpc.PermissionDecisionNoResult{}, nil
}

func permissionHandler(policy copilotadapter.PermissionPolicy) copilot.PermissionHandlerFunc {
	if policy == copilotadapter.PermissionPolicyApproveAll {
		return func(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
			return &rpc.PermissionDecisionApproved{}, nil
		}
	}
	return keepPermissionPending
}

// disconnectTracker makes every compatibility goroutine visible to the
// process owner. It can reject new work and expose a drain channel without
// starting a waiter goroutine of its own.
type disconnectTracker struct {
	mutex    sync.Mutex
	active   int
	stopping bool
	drained  chan struct{}
}

func newDisconnectTracker() *disconnectTracker {
	drained := make(chan struct{})
	close(drained)
	return &disconnectTracker{drained: drained}
}

func (tracker *disconnectTracker) begin() bool {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	if tracker.stopping {
		return false
	}
	if tracker.active == 0 {
		tracker.drained = make(chan struct{})
	}
	tracker.active++
	return true
}

func (tracker *disconnectTracker) done() {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.active--
	if tracker.active == 0 {
		close(tracker.drained)
	}
}

func (tracker *disconnectTracker) stop() <-chan struct{} {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	tracker.stopping = true
	return tracker.drained
}

func (tracker *disconnectTracker) activeCount() int {
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return tracker.active
}

type disconnectOperation struct {
	tracker *disconnectTracker
	invoke  func() error
	once    sync.Once
	done    chan struct{}
	err     error
}

func newDisconnectOperation(tracker *disconnectTracker, invoke func() error) *disconnectOperation {
	return &disconnectOperation{tracker: tracker, invoke: invoke, done: make(chan struct{})}
}

func (operation *disconnectOperation) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	operation.once.Do(func() {
		if !operation.tracker.begin() {
			operation.err = errBridgeClosing
			close(operation.done)
			return
		}
		go func() {
			defer operation.tracker.done()
			operation.err = operation.invoke()
			close(operation.done)
		}()
	})
	select {
	case <-operation.done:
		return operation.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func permissionToolName(request copilot.PermissionRequest) copilotadapter.ToolName {
	body, err := json.Marshal(request)
	if err != nil {
		return permissionFallbackToolName
	}
	fields := map[string]any{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return permissionFallbackToolName
	}
	for _, key := range permissionToolNameFields {
		if value, ok := fields[key].(string); ok && value != "" {
			return copilotadapter.ToolName(value)
		}
	}
	return permissionFallbackToolName
}

func permissionToolCallID(request copilot.PermissionRequest) copilotadapter.ToolCallID {
	body, err := json.Marshal(request)
	if err != nil {
		return ""
	}
	fields := map[string]any{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return ""
	}
	for _, key := range permissionToolCallIDFields {
		if value, ok := fields[key].(string); ok && value != "" {
			return copilotadapter.ToolCallID(value)
		}
	}
	return ""
}

//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package graph

import (
	"context"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/log"
)

// EventEmitter is the interface for emitting events from within NodeFunc.
// It provides a convenient way for nodes to emit custom events, progress updates,
// and streaming text during execution.
type EventEmitter interface {
	// Emit sends a custom event to the event channel.
	// Returns an error if the event cannot be sent (e.g., channel closed or timeout).
	Emit(evt *event.Event) error

	// EmitCustom sends a custom event with the specified event type and payload.
	// The event will be automatically enriched with node context (NodeID, InvocationID, etc.).
	EmitCustom(eventType string, payload any) error

	// EmitProgress sends a progress event with the specified progress percentage and message.
	// Progress should be a value between 0 and 100.
	EmitProgress(progress float64, message string) error

	// EmitText sends a streaming text event.
	// This is useful for streaming intermediate text output from a node.
	EmitText(text string) error

	// Context returns the context associated with this emitter.
	Context() context.Context
}

// eventEmitter is the default implementation of EventEmitter.
type eventEmitter struct {
	ctx          context.Context
	eventChan    chan<- *event.Event
	nodeID       string
	invocationID string
	stepNumber   int
	branch       string
	timeout      time.Duration
}

// EventEmitterOption is a function that configures an eventEmitter.
type EventEmitterOption func(*eventEmitter)

// WithEmitterContext sets the context for the emitter.
func WithEmitterContext(ctx context.Context) EventEmitterOption {
	return func(e *eventEmitter) {
		e.ctx = ctx
	}
}

// WithEmitterNodeID sets the node ID for the emitter.
func WithEmitterNodeID(nodeID string) EventEmitterOption {
	return func(e *eventEmitter) {
		e.nodeID = nodeID
	}
}

// WithEmitterInvocationID sets the invocation ID for the emitter.
func WithEmitterInvocationID(invocationID string) EventEmitterOption {
	return func(e *eventEmitter) {
		e.invocationID = invocationID
	}
}

// WithEmitterStepNumber sets the step number for the emitter.
func WithEmitterStepNumber(stepNumber int) EventEmitterOption {
	return func(e *eventEmitter) {
		e.stepNumber = stepNumber
	}
}

// WithEmitterBranch sets the branch for the emitter.
func WithEmitterBranch(branch string) EventEmitterOption {
	return func(e *eventEmitter) {
		e.branch = branch
	}
}

// WithEmitterTimeout sets the timeout for emit operations.
func WithEmitterTimeout(timeout time.Duration) EventEmitterOption {
	return func(e *eventEmitter) {
		e.timeout = timeout
	}
}

// NewEventEmitter creates a new EventEmitter with the given event channel and options.
// If eventChan is nil, returns a no-op emitter that safely ignores all emit calls.
func NewEventEmitter(eventChan chan<- *event.Event, opts ...EventEmitterOption) EventEmitter {
	if eventChan == nil {
		return &noopEmitter{}
	}

	emitter := &eventEmitter{
		ctx:       context.Background(),
		eventChan: eventChan,
		timeout:   event.EmitWithoutTimeout,
	}

	for _, opt := range opts {
		opt(emitter)
	}

	return emitter
}

// Emit sends a custom event to the event channel.
func (e *eventEmitter) Emit(evt *event.Event) error {
	if evt == nil {
		return nil
	}

	// Inject context information if not already set
	if evt.InvocationID == "" {
		evt.InvocationID = e.invocationID
	}
	if evt.Author == "" {
		evt.Author = e.nodeID
	}
	if evt.Branch == "" && e.branch != "" {
		evt.Branch = e.branch
	}

	return e.emitWithRecover(evt)
}

// EmitCustom sends a custom event with the specified event type and payload.
func (e *eventEmitter) EmitCustom(eventType string, payload any) error {
	metadata := NodeCustomEventMetadata{
		EventType:    eventType,
		Category:     NodeCustomEventCategoryCustom,
		NodeID:       e.nodeID,
		InvocationID: e.invocationID,
		StepNumber:   e.stepNumber,
		Timestamp:    time.Now(),
		Payload:      payload,
	}

	evt := NewGraphEvent(
		e.invocationID,
		e.nodeID,
		ObjectTypeGraphNodeCustom,
		WithNodeCustomMetadata(metadata),
	)
	if e.branch != "" {
		evt.Branch = e.branch
	}

	return e.emitWithRecover(evt)
}

// EmitProgress sends a progress event with the specified progress percentage and message.
func (e *eventEmitter) EmitProgress(progress float64, message string) error {
	// Clamp progress to 0-100
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	metadata := NodeCustomEventMetadata{
		EventType:    "progress",
		Category:     NodeCustomEventCategoryProgress,
		NodeID:       e.nodeID,
		InvocationID: e.invocationID,
		StepNumber:   e.stepNumber,
		Timestamp:    time.Now(),
		Progress:     progress,
		Message:      message,
	}

	evt := NewGraphEvent(
		e.invocationID,
		e.nodeID,
		ObjectTypeGraphNodeCustom,
		WithNodeCustomMetadata(metadata),
	)
	if e.branch != "" {
		evt.Branch = e.branch
	}

	return e.emitWithRecover(evt)
}

// EmitText sends a streaming text event.
func (e *eventEmitter) EmitText(text string) error {
	metadata := NodeCustomEventMetadata{
		EventType:    "text",
		Category:     NodeCustomEventCategoryText,
		NodeID:       e.nodeID,
		InvocationID: e.invocationID,
		StepNumber:   e.stepNumber,
		Timestamp:    time.Now(),
		Message:      text,
	}

	evt := NewGraphEvent(
		e.invocationID,
		e.nodeID,
		ObjectTypeGraphNodeCustom,
		WithNodeCustomMetadata(metadata),
	)
	if e.branch != "" {
		evt.Branch = e.branch
	}

	return e.emitWithRecover(evt)
}

// Context returns the context associated with this emitter.
func (e *eventEmitter) Context() context.Context {
	return e.ctx
}

// emitGracePeriod is the grace period for emitting events after context cancellation.
// This allows events to be delivered even when the main context is cancelled,
// ensuring no events are lost during graph shutdown.
const emitGracePeriod = 5 * time.Second

// emitWithRecover sends an event to the channel with panic recovery.
// If the original context is cancelled, it will retry with a grace period context
// to ensure events are not lost during graph shutdown.
func (e *eventEmitter) emitWithRecover(evt *event.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("EventEmitter: recovered from panic while emitting event: %v", r)
			err = nil // Don't propagate panic as error
		}
	}()

	// First attempt with the original context.
	err = event.EmitEventWithTimeout(e.ctx, e.eventChan, evt, e.timeout)
	if err == nil {
		return nil
	}

	// If failed due to context cancellation, retry with a grace period context
	// to ensure the event can still be delivered.
	if e.ctx.Err() != nil {
		log.Tracef("EventEmitter: original context cancelled, retrying with grace period: %v", err)

		// Use a fresh context with a grace period to allow event delivery.
		graceCtx, cancel := context.WithTimeout(context.Background(), emitGracePeriod)
		defer cancel()

		// Retry with the grace context.
		gracePeriodTimeout := emitGracePeriod
		if e.timeout > 0 && e.timeout < emitGracePeriod {
			gracePeriodTimeout = e.timeout
		}
		err = event.EmitEventWithTimeout(graceCtx, e.eventChan, evt, gracePeriodTimeout)
		if err != nil {
			// If still failed, it means the channel is likely closed or full.
			log.Warnf("EventEmitter: failed to emit event during grace period: %v", err)
		}
		return nil // Don't propagate error to caller during shutdown.
	}

	return err
}

// noopEmitter is a no-op implementation of EventEmitter.
// It safely ignores all emit calls and is used when EventChan is unavailable.
type noopEmitter struct{}

// Emit does nothing and returns nil.
func (n *noopEmitter) Emit(evt *event.Event) error {
	return nil
}

// EmitCustom does nothing and returns nil.
func (n *noopEmitter) EmitCustom(eventType string, payload any) error {
	return nil
}

// EmitProgress does nothing and returns nil.
func (n *noopEmitter) EmitProgress(progress float64, message string) error {
	return nil
}

// EmitText does nothing and returns nil.
func (n *noopEmitter) EmitText(text string) error {
	return nil
}

// Context returns a background context.
func (n *noopEmitter) Context() context.Context {
	return context.Background()
}

// GetEventEmitter retrieves an EventEmitter from the given State.
// It extracts the ExecutionContext from the state and creates an EventEmitter
// with the appropriate context information.
// If the state does not contain a valid ExecutionContext or EventChan,
// returns a no-op emitter that safely ignores all emit calls.
func GetEventEmitter(state State) EventEmitter {
	return GetEventEmitterWithContext(context.Background(), state)
}

// GetEventEmitterWithContext retrieves an EventEmitter from the given State with a custom context.
func GetEventEmitterWithContext(ctx context.Context, state State) EventEmitter {
	if state == nil {
		return &noopEmitter{}
	}

	// Get ExecutionContext from state
	execCtx, ok := GetStateValue[*ExecutionContext](state, StateKeyExecContext)
	if !ok || execCtx == nil {
		return &noopEmitter{}
	}

	// Check if EventChan is available
	if execCtx.EventChan == nil {
		return &noopEmitter{}
	}

	// Get current node ID from state
	nodeID, _ := GetStateValue[string](state, StateKeyCurrentNodeID)

	// Create EventEmitter with context information
	return NewEventEmitter(
		execCtx.EventChan,
		WithEmitterContext(ctx),
		WithEmitterNodeID(nodeID),
		WithEmitterInvocationID(execCtx.InvocationID),
	)
}

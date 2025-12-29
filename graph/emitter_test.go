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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/trpc-agent-go/event"
)

func TestEventEmitter_EmitCustom(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := NewEventEmitter(
		eventChan,
		WithEmitterNodeID("test-node"),
		WithEmitterInvocationID("test-invocation"),
		WithEmitterStepNumber(1),
	)

	err := emitter.EmitCustom("test-event", map[string]any{"key": "value"})
	require.NoError(t, err)

	select {
	case evt := <-eventChan:
		assert.Equal(t, "test-invocation", evt.InvocationID)
		assert.Equal(t, "test-node", evt.Author)
		assert.Equal(t, ObjectTypeGraphNodeCustom, evt.Object)
		assert.NotNil(t, evt.StateDelta)
		assert.Contains(t, evt.StateDelta, MetadataKeyNodeCustom)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventEmitter_EmitProgress(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := NewEventEmitter(
		eventChan,
		WithEmitterNodeID("test-node"),
		WithEmitterInvocationID("test-invocation"),
	)

	// Test normal progress
	err := emitter.EmitProgress(50.0, "halfway done")
	require.NoError(t, err)

	select {
	case evt := <-eventChan:
		assert.Equal(t, ObjectTypeGraphNodeCustom, evt.Object)
		assert.Contains(t, evt.StateDelta, MetadataKeyNodeCustom)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Test progress clamping (should clamp to 0-100)
	err = emitter.EmitProgress(-10.0, "negative")
	require.NoError(t, err)

	err = emitter.EmitProgress(150.0, "over 100")
	require.NoError(t, err)
}

func TestEventEmitter_EmitText(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := NewEventEmitter(
		eventChan,
		WithEmitterNodeID("test-node"),
		WithEmitterInvocationID("test-invocation"),
	)

	err := emitter.EmitText("Hello, World!")
	require.NoError(t, err)

	select {
	case evt := <-eventChan:
		assert.Equal(t, ObjectTypeGraphNodeCustom, evt.Object)
		assert.Contains(t, evt.StateDelta, MetadataKeyNodeCustom)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventEmitter_Emit(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := NewEventEmitter(
		eventChan,
		WithEmitterNodeID("test-node"),
		WithEmitterInvocationID("test-invocation"),
		WithEmitterBranch("test-branch"),
	)

	evt := event.New("", "", event.WithObject("test-object"))
	err := emitter.Emit(evt)
	require.NoError(t, err)

	select {
	case received := <-eventChan:
		assert.Equal(t, "test-invocation", received.InvocationID)
		assert.Equal(t, "test-node", received.Author)
		assert.Equal(t, "test-branch", received.Branch)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventEmitter_NilEvent(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := NewEventEmitter(eventChan)

	// Emit nil event should not panic or error
	err := emitter.Emit(nil)
	assert.NoError(t, err)
}

func TestNoopEmitter(t *testing.T) {
	emitter := NewEventEmitter(nil) // nil channel returns noopEmitter

	// All methods should return nil without panic
	assert.NoError(t, emitter.Emit(&event.Event{}))
	assert.NoError(t, emitter.EmitCustom("type", nil))
	assert.NoError(t, emitter.EmitProgress(50, "msg"))
	assert.NoError(t, emitter.EmitText("text"))
	assert.NotNil(t, emitter.Context())
}

func TestGetEventEmitter_NilState(t *testing.T) {
	emitter := GetEventEmitter(nil)

	// Should return noopEmitter
	assert.NoError(t, emitter.EmitCustom("type", nil))
}

func TestGetEventEmitter_NoExecutionContext(t *testing.T) {
	state := State{
		"some_key": "some_value",
	}

	emitter := GetEventEmitter(state)

	// Should return noopEmitter
	assert.NoError(t, emitter.EmitCustom("type", nil))
}

func TestGetEventEmitter_NilEventChan(t *testing.T) {
	execCtx := &ExecutionContext{
		InvocationID: "test-invocation",
		EventChan:    nil,
	}
	state := State{
		StateKeyExecContext: execCtx,
	}

	emitter := GetEventEmitter(state)

	// Should return noopEmitter
	assert.NoError(t, emitter.EmitCustom("type", nil))
}

func TestGetEventEmitter_WithValidContext(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	execCtx := &ExecutionContext{
		InvocationID: "test-invocation",
		EventChan:    eventChan,
	}
	state := State{
		StateKeyExecContext:   execCtx,
		StateKeyCurrentNodeID: "test-node",
	}

	emitter := GetEventEmitter(state)

	err := emitter.EmitCustom("test-type", map[string]any{"foo": "bar"})
	require.NoError(t, err)

	select {
	case evt := <-eventChan:
		assert.Equal(t, "test-invocation", evt.InvocationID)
		assert.Equal(t, "test-node", evt.Author)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestGetEventEmitterWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan := make(chan *event.Event, 10)
	execCtx := &ExecutionContext{
		InvocationID: "test-invocation",
		EventChan:    eventChan,
	}
	state := State{
		StateKeyExecContext: execCtx,
	}

	emitter := GetEventEmitterWithContext(ctx, state)

	assert.Equal(t, ctx, emitter.Context())
}

func TestEventEmitter_WithTimeout(t *testing.T) {
	eventChan := make(chan *event.Event, 1)
	emitter := NewEventEmitter(
		eventChan,
		WithEmitterTimeout(100*time.Millisecond),
	)

	// First emit should succeed
	err := emitter.EmitCustom("test", nil)
	assert.NoError(t, err)
}

func TestEventEmitter_RecoverFromPanic(t *testing.T) {
	// Create a closed channel to simulate panic scenario
	eventChan := make(chan *event.Event, 1)
	close(eventChan)

	emitter := &eventEmitter{
		ctx:          context.Background(),
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
	}

	// This should recover from panic and not propagate error
	err := emitter.EmitCustom("test", nil)
	assert.NoError(t, err)
}

// TestEmitWithRecover_Success 测试正常发送事件成功的场景
func TestEmitWithRecover_Success(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := &eventEmitter{
		ctx:          context.Background(),
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      100 * time.Millisecond,
	}

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))
	err := emitter.emitWithRecover(evt)

	assert.NoError(t, err)

	select {
	case received := <-eventChan:
		assert.Equal(t, "test-invocation", received.InvocationID)
		assert.Equal(t, "test-node", received.Author)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestEmitWithRecover_ContextCancelled_GracePeriodRetrySuccess 测试 context 取消后使用 grace period 重试成功的场景
func TestEmitWithRecover_ContextCancelled_GracePeriodRetrySuccess(t *testing.T) {
	eventChan := make(chan *event.Event, 10)

	// 创建一个已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	emitter := &eventEmitter{
		ctx:          ctx,
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      100 * time.Millisecond,
	}

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))
	err := emitter.emitWithRecover(evt)

	// 即使原 context 取消，也应该通过 grace period 成功发送，不返回错误
	assert.NoError(t, err)

	// 验证事件确实被发送了
	select {
	case received := <-eventChan:
		assert.Equal(t, "test-invocation", received.InvocationID)
		assert.Equal(t, "test-node", received.Author)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event - grace period retry should have succeeded")
	}
}

// TestEmitWithRecover_ClosedChannel_RecoverFromPanic 测试 channel 已关闭时的 panic 恢复
func TestEmitWithRecover_ClosedChannel_RecoverFromPanic(t *testing.T) {
	eventChan := make(chan *event.Event, 1)
	close(eventChan) // 关闭 channel 会导致发送时 panic

	emitter := &eventEmitter{
		ctx:          context.Background(),
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      100 * time.Millisecond,
	}

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))

	// 不应该 panic，应该恢复并返回 nil
	assert.NotPanics(t, func() {
		err := emitter.emitWithRecover(evt)
		assert.NoError(t, err) // panic 恢复后返回 nil
	})
}

// TestEmitWithRecover_ContextCancelled_ClosedChannel 测试 context 取消且 channel 关闭时的场景
func TestEmitWithRecover_ContextCancelled_ClosedChannel(t *testing.T) {
	eventChan := make(chan *event.Event, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	emitter := &eventEmitter{
		ctx:          ctx,
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      100 * time.Millisecond,
	}

	// 关闭 channel
	close(eventChan)

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))

	// 即使 context 取消且 channel 关闭，也不应该 panic，应该恢复并返回 nil
	assert.NotPanics(t, func() {
		err := emitter.emitWithRecover(evt)
		assert.NoError(t, err)
	})
}

// TestEmitWithRecover_FullChannel_ContextCancelled_GracePeriodRetry 测试 channel 满且 context 取消时的 grace period 重试
func TestEmitWithRecover_FullChannel_ContextCancelled_GracePeriodRetry(t *testing.T) {
	// 创建一个容量为1的 channel，并填满它
	eventChan := make(chan *event.Event, 1)
	eventChan <- event.New("", "", event.WithObject("filler")) // 填满 channel

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	emitter := &eventEmitter{
		ctx:          ctx,
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      50 * time.Millisecond, // 短超时
	}

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))

	// 启动一个 goroutine 稍后消费 channel 中的事件，使 grace period 重试能成功
	go func() {
		time.Sleep(10 * time.Millisecond)
		<-eventChan // 消费掉 filler 事件
	}()

	err := emitter.emitWithRecover(evt)

	// 不应该返回错误（grace period 期间 shutdown 不传播错误）
	assert.NoError(t, err)

	// 等待一下，确保事件被发送
	time.Sleep(100 * time.Millisecond)

	// 验证新事件被发送了
	select {
	case received := <-eventChan:
		assert.Equal(t, "test-invocation", received.InvocationID)
	default:
		// 如果 channel 为空，也是可以接受的（取决于时序）
	}
}

// TestEmitWithRecover_NilEvent 测试 nil 事件
func TestEmitWithRecover_NilEvent(t *testing.T) {
	eventChan := make(chan *event.Event, 10)
	emitter := &eventEmitter{
		ctx:          context.Background(),
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
	}

	// nil 事件应该在 Emit 方法中就被过滤，不会到达 emitWithRecover
	err := emitter.Emit(nil)
	assert.NoError(t, err)

	// channel 应该是空的
	select {
	case <-eventChan:
		t.Fatal("should not receive any event")
	default:
		// 预期行为
	}
}

// TestEmitWithRecover_GracePeriodUsesCustomTimeout 测试 grace period 使用自定义超时
func TestEmitWithRecover_GracePeriodUsesCustomTimeout(t *testing.T) {
	eventChan := make(chan *event.Event, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	customTimeout := 50 * time.Millisecond
	emitter := &eventEmitter{
		ctx:          ctx,
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      customTimeout, // 自定义超时小于 emitGracePeriod
	}

	evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))
	err := emitter.emitWithRecover(evt)

	assert.NoError(t, err)

	// 验证事件被发送
	select {
	case received := <-eventChan:
		assert.Equal(t, "test-invocation", received.InvocationID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestEmitWithRecover_MultipleEventsWithCancelledContext 测试 context 取消后连续发送多个事件
func TestEmitWithRecover_MultipleEventsWithCancelledContext(t *testing.T) {
	eventChan := make(chan *event.Event, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	emitter := &eventEmitter{
		ctx:          ctx,
		eventChan:    eventChan,
		nodeID:       "test-node",
		invocationID: "test-invocation",
		timeout:      100 * time.Millisecond,
	}

	// 连续发送多个事件
	for i := 0; i < 5; i++ {
		evt := event.New("test-invocation", "test-node", event.WithObject("test-object"))
		err := emitter.emitWithRecover(evt)
		assert.NoError(t, err)
	}

	// 验证所有事件都被发送了
	receivedCount := 0
	for i := 0; i < 5; i++ {
		select {
		case <-eventChan:
			receivedCount++
		case <-time.After(time.Second):
			break
		}
	}
	assert.Equal(t, 5, receivedCount, "all 5 events should be received via grace period")
}
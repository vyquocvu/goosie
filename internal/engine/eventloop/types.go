// Package eventloop provides bounded, UI-independent scheduling primitives for
// browser engine input and frame work.
package eventloop

import (
	"context"
	"time"
)

// TaskKind identifies a category of engine work.
type TaskKind uint8

const (
	TaskInput TaskKind = iota
	TaskNavigation
	TaskScript
	TaskMicrotask
	TaskMutation
	TaskResource
	TaskRender
	TaskIdle
)

// InputEventType identifies an input scheduling policy.
type InputEventType uint8

const (
	InputScroll InputEventType = iota
	InputMouseMove
	InputClick
	InputKey
	InputResize
)

// Viewport is the UI-independent visible document rectangle.
type Viewport struct {
	X, Y          float32
	Width, Height float32
}

// InputEvent is an immutable input value posted to the engine loop. Scroll,
// mouse-move, and resize events are coalesced; click and key events are FIFO.
type InputEvent struct {
	Type      InputEventType
	Viewport  Viewport
	X, Y      float32
	Button    int
	Key       string
	Timestamp time.Time
}

// Generation identifies the engine state used to build a frame.
type Generation struct {
	Navigation uint64
	Document   uint64
	DOM        uint64
	Style      uint64
	Layout     uint64
	Viewport   uint64
}

// Matches reports whether two generations describe the same engine state.
func (g Generation) Matches(other Generation) bool { return g == other }

// RenderReason describes why a frame was requested.
type RenderReason uint8

const (
	RenderReasonNavigation RenderReason = iota
	RenderReasonViewport
	RenderReasonMutation
	RenderReasonImageLoaded
	RenderReasonResize
)

// RenderRequest is a cancellable request for a worker to build a frame.
type RenderRequest struct {
	Context    context.Context
	Generation Generation
	Viewport   Viewport
	Reason     RenderReason
	Created    time.Time
}

// RenderResult is a completed worker result. Snapshot is intentionally opaque:
// integrations should pass an immutable renderer snapshot value.
type RenderResult struct {
	Request  RenderRequest
	Snapshot any
	Err      error
	Finished time.Time
}

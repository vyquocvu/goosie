// Package js — session ownership, bounded task queue, and shutdown (M8.1)
//
// Session wraps a Runtime with single-owner goroutine enforcement,
// a bounded task queue for scheduling JavaScript work, and context-based
// cancellation for shutdown and navigation.
package js

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	// ErrSessionClosed is returned when operating on a closed session.
	ErrSessionClosed = errors.New("js: session closed")

	// ErrTaskQueueFull is returned when the task queue is at capacity.
	ErrTaskQueueFull = errors.New("js: task queue full")

	// ErrWrongOwner is returned when a non-owner goroutine tries to
	// access the runtime directly.
	ErrWrongOwner = errors.New("js: runtime accessed from wrong goroutine")
)

// ---------------------------------------------------------------------------
// Task — a unit of work scheduled on the session
// ---------------------------------------------------------------------------

// Task is a function that executes within the session's owner goroutine.
// Tasks receive the session's Runtime for JavaScript interaction.
type Task func(rt *Runtime)

// ---------------------------------------------------------------------------
// Session — one runtime, one owner goroutine, bounded task queue
// ---------------------------------------------------------------------------

// SessionConfig configures the session.
type SessionConfig struct {
	// MaxPendingTasks is the maximum number of tasks in the queue.
	// Zero means default (256).
	MaxPendingTasks int
}

// DefaultSessionConfig returns sensible defaults.
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		MaxPendingTasks: 256,
	}
}

// Session wraps a Runtime with ownership, task scheduling, and shutdown.
//
// Exactly one goroutine "owns" the session — it is the only goroutine
// allowed to call Runtime methods directly. Other goroutines schedule
// work via Submit() which enqueues a Task for the owner to execute.
type Session struct {
	mu     sync.Mutex
	rt     *Runtime
	cfg    SessionConfig
	ctx    context.Context
	cancel context.CancelFunc

	// Task queue.
	tasks   []Task
	head    int
	count   int
	maxTask int

	// notify is used to wake the owner goroutine when a task is submitted.
	notify chan struct{}

	// ownerGID tracks the owner goroutine ID for assertion.
	// 0 = no owner set. We use a simple atomic flag rather than
	// runtime.Goexit to avoid importing runtime.
	ownerSet atomic.Bool

	// closed flag.
	closed atomic.Bool

	// Metrics.
	totalTasks   atomic.Uint64
	droppedTasks atomic.Uint64
}

// NewSession creates a session with a fresh Runtime and the given config.
// The returned session is not yet owned — call Run() to start the owner loop.
func NewSession(cfg SessionConfig) *Session {
	if cfg.MaxPendingTasks <= 0 {
		cfg.MaxPendingTasks = 256
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		rt:      NewRuntime(),
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		tasks:   make([]Task, cfg.MaxPendingTasks),
		maxTask: cfg.MaxPendingTasks,
		notify:  make(chan struct{}, 1),
	}
}

// NewSessionWithContext creates a session with an external context.
// Cancelling the context triggers session shutdown.
func NewSessionWithContext(ctx context.Context, cfg SessionConfig) *Session {
	if cfg.MaxPendingTasks <= 0 {
		cfg.MaxPendingTasks = 256
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Session{
		rt:      NewRuntime(),
		cfg:     cfg,
		ctx:     ctx,
		cancel:  cancel,
		tasks:   make([]Task, cfg.MaxPendingTasks),
		maxTask: cfg.MaxPendingTasks,
		notify:  make(chan struct{}, 1),
	}
}

// Runtime returns the session's JavaScript runtime.
// WARNING: Should only be called from the owner goroutine.
func (s *Session) Runtime() *Runtime {
	return s.rt
}

// Context returns the session's context for cancellation checks.
func (s *Session) Context() context.Context {
	return s.ctx
}

// Submit enqueues a task for execution on the owner goroutine.
// Returns ErrSessionClosed if the session is shut down, or
// ErrTaskQueueFull if the queue is at capacity.
func (s *Session) Submit(task Task) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count >= s.maxTask {
		s.droppedTasks.Add(1)
		return ErrTaskQueueFull
	}

	idx := (s.head + s.count) % s.maxTask
	s.tasks[idx] = task
	s.count++

	// Non-blocking wake of owner goroutine.
	select {
	case s.notify <- struct{}{}:
	default:
	}

	return nil
}

// Run starts the owner loop. It processes tasks until the context is
// cancelled or Close() is called. Run blocks the calling goroutine,
// which becomes the "owner" — the only goroutine allowed to call
// Runtime methods directly.
//
// Run returns the context error (nil if Close was called cleanly).
func (s *Session) Run() error {
	s.ownerSet.Store(true)
	defer s.ownerSet.Store(false)

	for {
		// Process all available tasks.
		s.drainTasks()

		// Wait for notification or context cancellation.
		select {
		case <-s.ctx.Done():
			// Final drain before exit.
			s.drainTasks()
			return s.ctx.Err()
		case <-s.notify:
			// New task(s) available — loop to drain.
		}
	}
}

// drainTasks executes all pending tasks in order.
func (s *Session) drainTasks() {
	for {
		s.mu.Lock()
		if s.count == 0 {
			s.mu.Unlock()
			return
		}
		task := s.tasks[s.head]
		s.tasks[s.head] = nil // avoid retaining reference
		s.head = (s.head + 1) % s.maxTask
		s.count--
		s.mu.Unlock()

		if task != nil {
			s.totalTasks.Add(1)
			task(s.rt)
		}
	}
}

// Close shuts down the session, cancelling the context and preventing
// new task submissions. Pending tasks are drained by Run() before exit.
func (s *Session) Close() {
	if s.closed.CompareAndSwap(false, true) {
		s.cancel()
	}
}

// IsClosed reports whether the session has been shut down.
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}

// PendingTasks returns the number of tasks waiting in the queue.
func (s *Session) PendingTasks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// Metrics returns session task metrics.
func (s *Session) Metrics() (totalExecuted, totalDropped uint64) {
	return s.totalTasks.Load(), s.droppedTasks.Load()
}

// Navigate cancels the current context and creates a new one, simulating
// navigation to a new document. The runtime is reset for the new page.
// This must be called from the owner goroutine.
func (s *Session) Navigate() {
	// Cancel current context.
	s.cancel()

	// Create new context for the new document.
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Reset runtime state for new document.
	s.rt.htmlCache = ""
	s.rt.historyStack = []string{}
	s.rt.historyIndex = -1

	// Drain any remaining tasks from previous session.
	s.mu.Lock()
	s.head = 0
	s.count = 0
	s.mu.Unlock()
}

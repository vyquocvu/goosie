package js

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// EventLoop — explicit task/microtask ordering with timer integration
// ---------------------------------------------------------------------------

// TaskCallback is a function executed by the event loop.
type TaskCallback func()

// EventLoopConfig configures the event loop.
type EventLoopConfig struct {
	// MaxTasks is the maximum pending macrotasks.
	MaxTasks int
	// MaxMicrotasks is the maximum pending microtasks.
	MaxMicrotasks int
	// MaxTimers is the maximum active timers.
	MaxTimers int
}

// DefaultEventLoopConfig returns sensible defaults.
func DefaultEventLoopConfig() EventLoopConfig {
	return EventLoopConfig{
		MaxTasks:      256,
		MaxMicrotasks: 512,
		MaxTimers:     128,
	}
}

// EventLoop implements the HTML event loop processing model:
// 1. Run the oldest macrotask
// 2. Run all microtasks (which may enqueue more microtasks)
// 3. Check timers
// 4. Repeat
//
// The event loop also batches DOM mutations and triggers a single
// style/layout update per mutation batch.
type EventLoop struct {
	mu sync.Mutex

	// Macrotask queue (ring buffer).
	tasks    []TaskCallback
	taskHead int
	taskCnt  int
	maxTasks int

	// Microtask queue (ring buffer).
	micros    []TaskCallback
	microHead int
	microCnt  int
	maxMicros int

	// Timer tracking.
	timers   map[int]*eventTimer
	timerSeq int
	maxTimer int

	// DOM mutation batching.
	pendingMutations     int
	onFlushMutations     func() // called after each task to flush DOM mutations
	onFlushMutationBatch func([]DOMMutation)

	// Context for shutdown.
	ctx    context.Context
	cancel context.CancelFunc

	// State.
	running atomic.Bool
	closed  atomic.Bool

	// Metrics.
	tasksExecuted   atomic.Uint64
	microsExecuted  atomic.Uint64
	mutationBatches atomic.Uint64
}

// eventTimer represents a scheduled timer in the event loop.
type eventTimer struct {
	id       int
	callback TaskCallback
	deadline time.Time
	interval time.Duration // 0 for one-shot
	repeat   bool
}

// NewEventLoop creates an event loop with the given config.
func NewEventLoop(cfg EventLoopConfig) *EventLoop {
	if cfg.MaxTasks <= 0 {
		cfg.MaxTasks = 256
	}
	if cfg.MaxMicrotasks <= 0 {
		cfg.MaxMicrotasks = 512
	}
	if cfg.MaxTimers <= 0 {
		cfg.MaxTimers = 128
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &EventLoop{
		tasks:     make([]TaskCallback, cfg.MaxTasks),
		micros:    make([]TaskCallback, cfg.MaxMicrotasks),
		maxTasks:  cfg.MaxTasks,
		maxMicros: cfg.MaxMicrotasks,
		timers:    make(map[int]*eventTimer, cfg.MaxTimers),
		maxTimer:  cfg.MaxTimers,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// SetMutationFlush sets the callback invoked after each macrotask to
// flush batched DOM mutations (triggering style/layout update).
func (el *EventLoop) SetMutationFlush(fn func()) {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.onFlushMutations = fn
}

func (el *EventLoop) SetMutationBatchFlush(fn func([]DOMMutation)) {
	el.mu.Lock()
	defer el.mu.Unlock()
	el.onFlushMutationBatch = fn
}

// QueueTask enqueues a macrotask. Returns false if the queue is full.
func (el *EventLoop) QueueTask(fn TaskCallback) bool {
	el.mu.Lock()
	defer el.mu.Unlock()
	if el.taskCnt >= el.maxTasks {
		return false
	}
	idx := (el.taskHead + el.taskCnt) % el.maxTasks
	el.tasks[idx] = fn
	el.taskCnt++
	return true
}

// QueueMicrotask enqueues a microtask. Returns false if the queue is full.
func (el *EventLoop) QueueMicrotask(fn TaskCallback) bool {
	el.mu.Lock()
	defer el.mu.Unlock()
	if el.microCnt >= el.maxMicros {
		return false
	}
	idx := (el.microHead + el.microCnt) % el.maxMicros
	el.micros[idx] = fn
	el.microCnt++
	return true
}

// RecordMutation records a pending DOM mutation. The mutation will be
// flushed after the current macrotask completes.
func (el *EventLoop) RecordMutation() {
	el.mu.Lock()
	el.pendingMutations++
	el.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Timer scheduling
// ---------------------------------------------------------------------------

// SetTimeout schedules a one-shot timer. Returns the timer ID.
func (el *EventLoop) SetTimeout(fn TaskCallback, delay time.Duration) int {
	el.mu.Lock()
	defer el.mu.Unlock()
	if len(el.timers) >= el.maxTimer {
		return -1
	}
	el.timerSeq++
	id := el.timerSeq
	el.timers[id] = &eventTimer{
		id:       id,
		callback: fn,
		deadline: time.Now().Add(delay),
		repeat:   false,
	}
	return id
}

// SetInterval schedules a repeating timer. Returns the timer ID.
func (el *EventLoop) SetInterval(fn TaskCallback, interval time.Duration) int {
	el.mu.Lock()
	defer el.mu.Unlock()
	if len(el.timers) >= el.maxTimer {
		return -1
	}
	el.timerSeq++
	id := el.timerSeq
	el.timers[id] = &eventTimer{
		id:       id,
		callback: fn,
		deadline: time.Now().Add(interval),
		interval: interval,
		repeat:   true,
	}
	return id
}

// ClearTimer cancels a timer by ID. Returns true if found.
func (el *EventLoop) ClearTimer(id int) bool {
	el.mu.Lock()
	defer el.mu.Unlock()
	_, ok := el.timers[id]
	if ok {
		delete(el.timers, id)
	}
	return ok
}

// ---------------------------------------------------------------------------
// Event loop execution
// ---------------------------------------------------------------------------

// RunOnce processes one iteration of the event loop:
// 1. Execute the oldest macrotask (if any)
// 2. Drain all microtasks
// 3. Fire ready timers (which enqueue more macrotasks)
// 4. Flush DOM mutations
//
// Returns true if any work was done.
func (el *EventLoop) RunOnce() bool {
	didWork := false

	// 1. Execute one macrotask.
	el.mu.Lock()
	var task TaskCallback
	if el.taskCnt > 0 {
		task = el.tasks[el.taskHead]
		el.tasks[el.taskHead] = nil
		el.taskHead = (el.taskHead + 1) % el.maxTasks
		el.taskCnt--
	}
	el.mu.Unlock()

	if task != nil {
		task()
		el.tasksExecuted.Add(1)
		didWork = true
	}

	// 2. Drain all microtasks (including those enqueued by microtasks).
	for {
		el.mu.Lock()
		var micro TaskCallback
		if el.microCnt > 0 {
			micro = el.micros[el.microHead]
			el.micros[el.microHead] = nil
			el.microHead = (el.microHead + 1) % el.maxMicros
			el.microCnt--
		}
		el.mu.Unlock()

		if micro == nil {
			break
		}
		micro()
		el.microsExecuted.Add(1)
		didWork = true
	}

	// 3. Fire ready timers.
	el.fireReadyTimers()

	// 4. Flush DOM mutations.
	el.mu.Lock()
	hasMutations := el.pendingMutations > 0
	flushFn := el.onFlushMutations
	batchFn := el.onFlushMutationBatch
	mutationCount := el.pendingMutations
	if hasMutations {
		el.pendingMutations = 0
		el.mutationBatches.Add(1)
	}
	el.mu.Unlock()

	if hasMutations {
		if flushFn != nil {
			flushFn()
		}
		if batchFn != nil {
			batchFn([]DOMMutation{{Kind: MutationBatch, Count: mutationCount}})
		}
	}

	return didWork
}

// fireReadyTimers moves expired timers' callbacks into the macrotask queue.
func (el *EventLoop) fireReadyTimers() {
	now := time.Now()
	el.mu.Lock()
	var ready []*eventTimer
	for id, t := range el.timers {
		if !t.deadline.After(now) {
			ready = append(ready, t)
			if t.repeat {
				t.deadline = now.Add(t.interval)
			} else {
				delete(el.timers, id)
			}
		}
	}
	el.mu.Unlock()

	// Enqueue ready timers as macrotasks.
	for _, t := range ready {
		el.QueueTask(t.callback)
	}
}

// Run processes event loop iterations until the context is cancelled
// or no work remains and the loop is stopped.
func (el *EventLoop) Run() {
	el.running.Store(true)
	defer el.running.Store(false)

	for !el.closed.Load() {
		if !el.RunOnce() {
			// No work — wait briefly or until context done.
			select {
			case <-el.ctx.Done():
				return
			case <-time.After(time.Millisecond):
			}
		}
	}
}

// Stop signals the event loop to stop after draining pending work.
func (el *EventLoop) Stop() {
	if el.closed.CompareAndSwap(false, true) {
		el.cancel()
	}
}

// IsRunning reports whether the event loop is currently in Run().
func (el *EventLoop) IsRunning() bool {
	return el.running.Load()
}

// PendingTasks returns the number of pending macrotasks.
func (el *EventLoop) PendingTasks() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return el.taskCnt
}

// PendingMicrotasks returns the number of pending microtasks.
func (el *EventLoop) PendingMicrotasks() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return el.microCnt
}

// ActiveTimers returns the number of active timers.
func (el *EventLoop) ActiveTimers() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return len(el.timers)
}

// PendingMutations returns the number of unflushed DOM mutations.
func (el *EventLoop) PendingMutations() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return el.pendingMutations
}

// Metrics returns event loop execution metrics.
func (el *EventLoop) Metrics() (tasksExecuted, microsExecuted, mutationBatches uint64) {
	return el.tasksExecuted.Load(), el.microsExecuted.Load(), el.mutationBatches.Load()
}

package js

import (
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// EventLoop — construction
// ---------------------------------------------------------------------------

func TestNewEventLoop(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())
	if el.PendingTasks() != 0 {
		t.Errorf("PendingTasks = %d, want 0", el.PendingTasks())
	}
	if el.PendingMicrotasks() != 0 {
		t.Errorf("PendingMicrotasks = %d, want 0", el.PendingMicrotasks())
	}
	if el.ActiveTimers() != 0 {
		t.Errorf("ActiveTimers = %d, want 0", el.ActiveTimers())
	}
}

func TestDefaultEventLoopConfig(t *testing.T) {
	cfg := DefaultEventLoopConfig()
	if cfg.MaxTasks != 256 {
		t.Errorf("MaxTasks = %d, want 256", cfg.MaxTasks)
	}
	if cfg.MaxMicrotasks != 512 {
		t.Errorf("MaxMicrotasks = %d, want 512", cfg.MaxMicrotasks)
	}
	if cfg.MaxTimers != 128 {
		t.Errorf("MaxTimers = %d, want 128", cfg.MaxTimers)
	}
}

// ---------------------------------------------------------------------------
// Task ordering — macrotasks execute FIFO
// ---------------------------------------------------------------------------

func TestEventLoop_TaskOrder(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	order := make([]int, 0, 3)
	el.QueueTask(func() { order = append(order, 1) })
	el.QueueTask(func() { order = append(order, 2) })
	el.QueueTask(func() { order = append(order, 3) })

	el.RunOnce()
	el.RunOnce()
	el.RunOnce()

	if len(order) != 3 {
		t.Fatalf("order len = %d, want 3", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("order[%d] = %d, want %d", i, v, i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Microtask draining — all microtasks run after each macrotask
// ---------------------------------------------------------------------------

func TestEventLoop_MicrotaskDrain(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var microOrder []int
	el.QueueTask(func() {
		// Enqueue microtasks from within a macrotask.
		el.QueueMicrotask(func() { microOrder = append(microOrder, 1) })
		el.QueueMicrotask(func() { microOrder = append(microOrder, 2) })
	})

	el.RunOnce()

	if len(microOrder) != 2 {
		t.Fatalf("microOrder len = %d, want 2", len(microOrder))
	}
	if microOrder[0] != 1 || microOrder[1] != 2 {
		t.Errorf("microOrder = %v, want [1, 2]", microOrder)
	}
}

func TestEventLoop_MicrotaskEnqueuesMicrotask(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var count atomic.Int32
	el.QueueTask(func() {
		el.QueueMicrotask(func() {
			count.Add(1)
			el.QueueMicrotask(func() {
				count.Add(1)
			})
		})
	})

	el.RunOnce()

	if count.Load() != 2 {
		t.Errorf("count = %d, want 2 (microtask should enqueue another)", count.Load())
	}
}

// ---------------------------------------------------------------------------
// Task/microtask ordering — microtasks run before next macrotask
// ---------------------------------------------------------------------------

func TestEventLoop_MicrotaskBeforeNextTask(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var order []string
	el.QueueTask(func() {
		order = append(order, "task1")
		el.QueueMicrotask(func() { order = append(order, "micro1") })
	})
	el.QueueTask(func() {
		order = append(order, "task2")
	})

	el.RunOnce() // task1 + micro1
	el.RunOnce() // task2

	if len(order) != 3 {
		t.Fatalf("order len = %d, want 3", len(order))
	}
	if order[0] != "task1" || order[1] != "micro1" || order[2] != "task2" {
		t.Errorf("order = %v, want [task1, micro1, task2]", order)
	}
}

// ---------------------------------------------------------------------------
// Timer integration
// ---------------------------------------------------------------------------

func TestEventLoop_SetTimeout(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var fired atomic.Bool
	el.SetTimeout(func() { fired.Store(true) }, time.Millisecond)

	// Wait for timer to expire.
	time.Sleep(5 * time.Millisecond)
	el.RunOnce() // fires timer → enqueues macrotask
	el.RunOnce() // executes macrotask

	if !fired.Load() {
		t.Error("setTimeout callback should have fired")
	}
}

func TestEventLoop_SetInterval(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var count atomic.Int32
	id := el.SetInterval(func() { count.Add(1) }, time.Millisecond)

	// Fire multiple times.
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		el.RunOnce() // fire timer
		el.RunOnce() // execute task
	}

	el.ClearTimer(id)

	if count.Load() < 2 {
		t.Errorf("interval fired %d times, want >= 2", count.Load())
	}
}

func TestEventLoop_ClearTimer(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var fired atomic.Bool
	id := el.SetTimeout(func() { fired.Store(true) }, time.Hour)

	if !el.ClearTimer(id) {
		t.Error("ClearTimer should return true for existing timer")
	}
	if el.ClearTimer(id) {
		t.Error("ClearTimer should return false for already cleared timer")
	}
	if el.ActiveTimers() != 0 {
		t.Errorf("ActiveTimers = %d, want 0", el.ActiveTimers())
	}
}

func TestEventLoop_TimerBounded(t *testing.T) {
	cfg := EventLoopConfig{MaxTimers: 2, MaxTasks: 10, MaxMicrotasks: 10}
	el := NewEventLoop(cfg)

	id1 := el.SetTimeout(func() {}, time.Hour)
	id2 := el.SetTimeout(func() {}, time.Hour)
	if id1 < 0 || id2 < 0 {
		t.Fatal("first two timers should succeed")
	}

	id3 := el.SetTimeout(func() {}, time.Hour)
	if id3 != -1 {
		t.Error("third timer should fail (max timers reached)")
	}
}

// ---------------------------------------------------------------------------
// DOM mutation batching
// ---------------------------------------------------------------------------

func TestEventLoop_MutationBatching(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var flushCount atomic.Int32
	el.SetMutationFlush(func() { flushCount.Add(1) })

	// Record mutations during a task.
	el.QueueTask(func() {
		el.RecordMutation()
		el.RecordMutation()
		el.RecordMutation()
	})

	el.RunOnce()

	if el.PendingMutations() != 0 {
		t.Errorf("PendingMutations = %d, want 0 (should be flushed)", el.PendingMutations())
	}
	if flushCount.Load() != 1 {
		t.Errorf("flushCount = %d, want 1 (one batch per task)", flushCount.Load())
	}
}

func TestEventLoop_TypedMutationBatch(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var got []DOMMutation
	el.SetMutationBatchFlush(func(batch []DOMMutation) {
		got = batch
	})
	el.QueueTask(func() {
		el.RecordMutation()
		el.RecordMutation()
	})

	el.RunOnce()

	if len(got) != 1 {
		t.Fatalf("batch length = %d, want 1", len(got))
	}
	if got[0].Kind != MutationBatch || got[0].Count != 2 {
		t.Fatalf("batch = %#v, want kind=%v count=2", got[0], MutationBatch)
	}
}
func TestEventLoop_NoFlushWithoutMutations(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	var flushCount atomic.Int32
	el.SetMutationFlush(func() { flushCount.Add(1) })

	el.QueueTask(func() {
		// No mutations recorded.
	})

	el.RunOnce()

	if flushCount.Load() != 0 {
		t.Errorf("flushCount = %d, want 0 (no mutations)", flushCount.Load())
	}
}

// ---------------------------------------------------------------------------
// Queue bounds
// ---------------------------------------------------------------------------

func TestEventLoop_TaskQueueFull(t *testing.T) {
	cfg := EventLoopConfig{MaxTasks: 2, MaxMicrotasks: 10, MaxTimers: 10}
	el := NewEventLoop(cfg)

	if !el.QueueTask(func() {}) {
		t.Error("first task should succeed")
	}
	if !el.QueueTask(func() {}) {
		t.Error("second task should succeed")
	}
	if el.QueueTask(func() {}) {
		t.Error("third task should fail (queue full)")
	}
}

func TestEventLoop_MicrotaskQueueFull(t *testing.T) {
	cfg := EventLoopConfig{MaxTasks: 10, MaxMicrotasks: 2, MaxTimers: 10}
	el := NewEventLoop(cfg)

	if !el.QueueMicrotask(func() {}) {
		t.Error("first microtask should succeed")
	}
	if !el.QueueMicrotask(func() {}) {
		t.Error("second microtask should succeed")
	}
	if el.QueueMicrotask(func() {}) {
		t.Error("third microtask should fail (queue full)")
	}
}

// ---------------------------------------------------------------------------
// RunOnce returns false when idle
// ---------------------------------------------------------------------------

func TestEventLoop_RunOnceIdle(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())
	if el.RunOnce() {
		t.Error("RunOnce on empty loop should return false")
	}
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

func TestEventLoop_Metrics(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	el.QueueTask(func() {
		el.QueueMicrotask(func() {})
	})
	el.RunOnce()

	tasks, micros, batches := el.Metrics()
	if tasks != 1 {
		t.Errorf("tasks = %d, want 1", tasks)
	}
	if micros != 1 {
		t.Errorf("micros = %d, want 1", micros)
	}
	_ = batches
}

// ---------------------------------------------------------------------------
// Stop
// ---------------------------------------------------------------------------

func TestEventLoop_Stop(t *testing.T) {
	el := NewEventLoop(DefaultEventLoopConfig())

	go func() {
		time.Sleep(20 * time.Millisecond)
		el.Stop()
	}()

	el.Run()

	if el.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkEventLoop_QueueAndRun(b *testing.B) {
	el := NewEventLoop(DefaultEventLoopConfig())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		el.QueueTask(func() {})
		el.RunOnce()
	}
}

func BenchmarkEventLoop_MicrotaskDrain(b *testing.B) {
	el := NewEventLoop(DefaultEventLoopConfig())
	for i := 0; i < 100; i++ {
		el.QueueMicrotask(func() {})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		el.QueueTask(func() {})
		el.RunOnce()
	}
}

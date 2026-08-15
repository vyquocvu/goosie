package js

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Session — construction
// ---------------------------------------------------------------------------

func TestNewSession(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	defer s.Close()

	if s.Runtime() == nil {
		t.Fatal("Runtime should not be nil")
	}
	if s.IsClosed() {
		t.Error("new session should not be closed")
	}
	if s.PendingTasks() != 0 {
		t.Errorf("PendingTasks = %d, want 0", s.PendingTasks())
	}
}

func TestNewSession_DefaultConfig(t *testing.T) {
	cfg := DefaultSessionConfig()
	if cfg.MaxPendingTasks != 256 {
		t.Errorf("MaxPendingTasks = %d, want 256", cfg.MaxPendingTasks)
	}
}

func TestSession_NewSessionWithRuntime(t *testing.T) {
	rt := NewRuntime()
	s := NewSessionWithRuntime(rt, DefaultSessionConfig())
	defer s.Close()

	if s.Runtime() != rt {
		t.Fatal("session should own the provided runtime")
	}
	if rt.enqueueTask == nil {
		t.Error("async-callback routing (enqueueTask) should be wired to the session")
	}
}

func TestNewSessionWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewSessionWithContext(ctx, DefaultSessionConfig())
	defer s.Close()

	if s.Context() == nil {
		t.Fatal("Context should not be nil")
	}
}

// ---------------------------------------------------------------------------
// Session — task submission and execution
// ---------------------------------------------------------------------------

func TestSession_SubmitAndRun(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	var executed atomic.Bool
	s.Submit(func(rt *Runtime) {
		executed.Store(true)
	})

	// Run in background, close after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Close()
	}()

	err := s.Run()
	if err != nil && err != context.Canceled {
		t.Errorf("Run returned unexpected error: %v", err)
	}

	if !executed.Load() {
		t.Error("task should have been executed")
	}
}

func TestSession_MultipleTasks(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	var counter atomic.Int64
	for i := 0; i < 10; i++ {
		s.Submit(func(rt *Runtime) {
			counter.Add(1)
		})
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Close()
	}()

	s.Run()

	if counter.Load() != 10 {
		t.Errorf("counter = %d, want 10", counter.Load())
	}
}

func TestSession_TaskOrder(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	order := make([]int, 0, 5)
	var mu sync.Mutex
	for i := 0; i < 5; i++ {
		val := i
		s.Submit(func(rt *Runtime) {
			mu.Lock()
			order = append(order, val)
			mu.Unlock()
		})
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Close()
	}()

	s.Run()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 5 {
		t.Fatalf("order len = %d, want 5", len(order))
	}
	for i, v := range order {
		if v != i {
			t.Errorf("order[%d] = %d, want %d", i, v, i)
		}
	}
}

// ---------------------------------------------------------------------------
// Session — bounded queue
// ---------------------------------------------------------------------------

func TestSession_QueueFull(t *testing.T) {
	cfg := SessionConfig{MaxPendingTasks: 2}
	s := NewSession(cfg)

	// Fill the queue.
	s.Submit(func(rt *Runtime) {})
	s.Submit(func(rt *Runtime) {})

	// Third should fail.
	err := s.Submit(func(rt *Runtime) {})
	if err != ErrTaskQueueFull {
		t.Errorf("expected ErrTaskQueueFull, got %v", err)
	}

	_, dropped := s.Metrics()
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
	s.Close()
}

func TestSession_SubmitAfterClose(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	s.Close()

	err := s.Submit(func(rt *Runtime) {})
	if err != ErrSessionClosed {
		t.Errorf("expected ErrSessionClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session — SubmitAndWait and Eval (blocking owner execution)
// ---------------------------------------------------------------------------

func TestSession_SubmitAndWait(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	var ran atomic.Bool
	if err := s.SubmitAndWait(func(rt *Runtime) { ran.Store(true) }); err != nil {
		t.Fatalf("SubmitAndWait: %v", err)
	}
	if !ran.Load() {
		t.Error("task should have run before SubmitAndWait returned")
	}

	s.Close()
	if err := <-done; err != context.Canceled {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
}

func TestSession_SubmitAndWait_Ordered(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	order := make([]int, 0, 3)
	var mu sync.Mutex
	for i := 0; i < 3; i++ {
		val := i
		if err := s.SubmitAndWait(func(rt *Runtime) {
			mu.Lock()
			order = append(order, val)
			mu.Unlock()
		}); err != nil {
			t.Fatalf("SubmitAndWait #%d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for i, v := range order {
		if v != i {
			t.Errorf("order[%d] = %d, want %d", i, v, i)
		}
	}
	s.Close()
	<-done
}

func TestSession_SubmitAndWait_Closed(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	s.Close()

	err := s.SubmitAndWait(func(rt *Runtime) {})
	if err != ErrSessionClosed {
		t.Errorf("expected ErrSessionClosed, got %v", err)
	}
}

func TestSession_Eval(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	v, err := s.Eval("1 + 2")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v.String() != "3" {
		t.Errorf("Eval result = %s, want 3", v.String())
	}

	s.Close()
	<-done
}

func TestSession_Eval_Error(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	if _, err := s.Eval("this is not valid js {"); err == nil {
		t.Error("expected a syntax error from Eval")
	}

	s.Close()
	<-done
}

func TestSession_Eval_Closed(t *testing.T) {
	s := NewSession(DefaultSessionConfig())
	s.Close()

	if _, err := s.Eval("1"); err != ErrSessionClosed {
		t.Errorf("expected ErrSessionClosed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Session — Close and cancellation
// ---------------------------------------------------------------------------

func TestSession_Close(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	go func() {
		time.Sleep(20 * time.Millisecond)
		s.Close()
	}()

	err := s.Run()
	if err != context.Canceled {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
	if !s.IsClosed() {
		t.Error("session should be closed after Run exits")
	}
}

func TestSession_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSessionWithContext(ctx, DefaultSessionConfig())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := s.Run()
	if err != context.Canceled {
		t.Errorf("Run error = %v, want context.Canceled", err)
	}
}

// ---------------------------------------------------------------------------
// Session — metrics
// ---------------------------------------------------------------------------

func TestSession_Metrics(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	for i := 0; i < 5; i++ {
		s.Submit(func(rt *Runtime) {})
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Close()
	}()

	s.Run()

	executed, dropped := s.Metrics()
	if executed != 5 {
		t.Errorf("executed = %d, want 5", executed)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
}

// ---------------------------------------------------------------------------
// Session — Navigate resets state
// ---------------------------------------------------------------------------

func TestSession_Navigate(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	// Submit some tasks.
	s.Submit(func(rt *Runtime) {})
	s.Submit(func(rt *Runtime) {})

	// Navigate (must be called from owner goroutine).
	// We simulate by calling it before Run.
	s.Navigate()

	if s.PendingTasks() != 0 {
		t.Errorf("PendingTasks after Navigate = %d, want 0", s.PendingTasks())
	}

	// Context should be fresh.
	if s.Context().Err() != nil {
		t.Error("new context should not be cancelled")
	}

	s.Close()
}

// ---------------------------------------------------------------------------
// Session — concurrent submit from multiple goroutines
// ---------------------------------------------------------------------------

func TestSession_ConcurrentSubmit(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	var wg sync.WaitGroup
	var submitted atomic.Int64
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				err := s.Submit(func(rt *Runtime) {})
				if err == nil {
					submitted.Add(1)
				}
			}
		}()
	}

	// Run owner in background.
	done := make(chan error, 1)
	go func() {
		done <- s.Run()
	}()

	wg.Wait()
	time.Sleep(100 * time.Millisecond) // let tasks drain
	s.Close()
	<-done

	executed, _ := s.Metrics()
	if executed == 0 {
		t.Error("some tasks should have been executed")
	}
}

// ---------------------------------------------------------------------------
// Session — task receives runtime
// ---------------------------------------------------------------------------

func TestSession_TaskReceivesRuntime(t *testing.T) {
	s := NewSession(DefaultSessionConfig())

	var gotRT *Runtime
	s.Submit(func(rt *Runtime) {
		gotRT = rt
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.Close()
	}()

	s.Run()

	if gotRT == nil {
		t.Fatal("task should receive non-nil runtime")
	}
	if gotRT != s.Runtime() {
		t.Error("task should receive the session's runtime")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkSession_Submit(b *testing.B) {
	s := NewSession(DefaultSessionConfig())
	defer s.Close()

	// Start owner in background.
	go func() {
		for !s.IsClosed() {
			s.drainTasks()
			time.Sleep(time.Microsecond)
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.Submit(func(rt *Runtime) {})
	}
}

func BenchmarkSession_TaskExecution(b *testing.B) {
	s := NewSession(DefaultSessionConfig())

	// Pre-submit all tasks.
	for i := 0; i < b.N; i++ {
		s.Submit(func(rt *Runtime) {})
	}

	b.ResetTimer()
	b.ReportAllocs()
	s.drainTasks()
}

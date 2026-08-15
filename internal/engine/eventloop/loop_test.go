package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestScrollEventsCoalesceLatestWins(t *testing.T) {
	loop := New(Config{})
	for i := 0; i < 100; i++ {
		if err := loop.PostInput(InputEvent{Type: InputScroll, Viewport: Viewport{Y: float32(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	events := loop.DrainInput()
	if len(events) != 1 || events[0].Viewport.Y != 99 {
		t.Fatalf("events = %#v, want latest scroll", events)
	}
	if got := loop.Metrics().CoalescedScrollEvents; got != 99 {
		t.Fatalf("coalesced scroll = %d, want 99", got)
	}
}

func TestMouseMoveEventsCoalesceLatestWins(t *testing.T) {
	loop := New(Config{})
	for i := 0; i < 50; i++ {
		if err := loop.PostInput(InputEvent{Type: InputMouseMove, X: float32(i), Y: float32(i * 2)}); err != nil {
			t.Fatal(err)
		}
	}
	events := loop.DrainInput()
	if len(events) != 1 || events[0].X != 49 || events[0].Y != 98 {
		t.Fatalf("events = %#v, want latest mouse move", events)
	}
	if got := loop.Metrics().CoalescedMouseMoves; got != 49 {
		t.Fatalf("coalesced mouse = %d, want 49", got)
	}
}

func TestClickAndKeyEventsPreserveOrdering(t *testing.T) {
	loop := New(Config{InputQueueSize: 4})
	want := []InputEvent{
		{Type: InputClick, Button: 1},
		{Type: InputKey, Key: "a"},
		{Type: InputClick, Button: 2},
		{Type: InputKey, Key: "b"},
	}
	for _, event := range want {
		if err := loop.PostInput(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := loop.PostInput(InputEvent{Type: InputClick}); !errors.Is(err, ErrInputQueueFull) {
		t.Fatalf("overflow error = %v, want ErrInputQueueFull", err)
	}
	got := loop.DrainInput()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Button != want[i].Button || got[i].Key != want[i].Key {
			t.Fatalf("event %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	if got := loop.Metrics().InputSignalsDropped; got != 1 {
		t.Fatalf("dropped input = %d, want 1", got)
	}
}

func TestRenderQueueKeepsLatestRequest(t *testing.T) {
	loop := New(Config{})
	first, err := loop.ScheduleRender(context.Background(), RenderRequest{Generation: Generation{Viewport: 1}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := loop.ScheduleRender(context.Background(), RenderRequest{Generation: Generation{Viewport: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Context.Err() == nil {
		t.Fatal("superseded request context was not cancelled")
	}
	select {
	case got := <-loop.RenderRequests():
		if !got.Generation.Matches(second.Generation) {
			t.Fatalf("generation = %#v, want %#v", got.Generation, second.Generation)
		}
	default:
		t.Fatal("render request queue is empty")
	}
	metrics := loop.Metrics()
	if metrics.RenderRequestsCreated != 2 || metrics.RenderRequestsDropped != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestCurrentRenderResultIsPresented(t *testing.T) {
	presented := make(chan RenderResult, 1)
	gen := Generation{Layout: 7}
	loop := New(Config{Present: func(result RenderResult) { presented <- result }})
	loop.SetGeneration(gen)
	req, err := loop.ScheduleRender(context.Background(), RenderRequest{Generation: gen})
	if err != nil {
		t.Fatal(err)
	}
	<-loop.RenderRequests()

	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(runCtx) }()
	if err := loop.SubmitRenderResult(RenderResult{Request: req, Snapshot: "frame"}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-presented:
		if result.Snapshot != "frame" {
			t.Fatalf("snapshot = %v, want frame", result.Snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("current frame was not presented")
	}
	stop()
	<-done
	if got := loop.Metrics().FramesPresented; got != 1 {
		t.Fatalf("frames presented = %d, want 1", got)
	}
}

func TestStaleRenderResultDroppedByGeneration(t *testing.T) {
	presented := make(chan RenderResult, 1)
	loop := New(Config{Present: func(result RenderResult) { presented <- result }})
	loop.SetGeneration(Generation{Navigation: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loop.SubmitRenderResult(RenderResult{Request: RenderRequest{
		Context: ctx, Generation: Generation{Navigation: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(runCtx) }()
	waitForMetric(t, loop, func(m Metrics) bool { return m.StaleFramesDropped == 1 })
	select {
	case <-presented:
		t.Fatal("stale frame was presented")
	default:
	}
	stop()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
}

func TestCancellationPreventsPresentation(t *testing.T) {
	var presented atomic.Uint64
	loop := New(Config{Present: func(RenderResult) { presented.Add(1) }})
	gen := Generation{Document: 1}
	loop.SetGeneration(gen)
	req, err := loop.ScheduleRender(context.Background(), RenderRequest{Generation: gen})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	cancelRun()
	loop.mu.Lock()
	loop.runCtx = runCtx
	loop.mu.Unlock()
	loop.handleRenderResult(RenderResult{Request: req})
	if got := presented.Load(); got != 0 {
		t.Fatalf("presented = %d, want 0", got)
	}
	if got := loop.Metrics().StaleFramesDropped; got != 1 {
		t.Fatalf("stale frames = %d, want 1", got)
	}
	loop.Close()
}

func TestMetricsCountCoalescedInputAndStaleFrames(t *testing.T) {
	loop := New(Config{})
	for i := 0; i < 3; i++ {
		_ = loop.PostInput(InputEvent{Type: InputScroll})
		_ = loop.PostInput(InputEvent{Type: InputMouseMove})
	}
	loop.SetGeneration(Generation{DOM: 2})
	loop.handleRenderResult(RenderResult{Request: RenderRequest{Generation: Generation{DOM: 1}}})
	metrics := loop.Metrics()
	if metrics.InputEventsReceived != 6 || metrics.CoalescedScrollEvents != 2 || metrics.CoalescedMouseMoves != 2 || metrics.StaleFramesDropped != 1 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestProcessPendingResultsNonBlocking(t *testing.T) {
	var presented atomic.Uint64
	gen := Generation{Layout: 3}
	loop := New(Config{Present: func(RenderResult) { presented.Add(1) }})
	loop.SetGeneration(gen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// SubmitRenderResult keeps only the latest completion (capacity 1), so
	// two of the three submits replace older results.
	for i := 0; i < 3; i++ {
		if err := loop.SubmitRenderResult(RenderResult{Request: RenderRequest{
			Context: ctx, Generation: gen,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	// The surviving result is processed without blocking; the two replaced
	// completions were counted as stale at submit time.
	if got := loop.ProcessPendingResults(); got != 1 {
		t.Fatalf("processed = %d, want 1", got)
	}
	if got := presented.Load(); got != 1 {
		t.Fatalf("presented = %d, want 1", got)
	}
	if got := loop.Metrics().StaleFramesDropped; got != 2 {
		t.Fatalf("replaced completions = %d, want 2", got)
	}
	// Second call is a non-blocking no-op: nothing new to process.
	if got := loop.ProcessPendingResults(); got != 0 {
		t.Fatalf("empty processed = %d, want 0", got)
	}
}

func TestProcessPendingResultsDropsStaleByGeneration(t *testing.T) {
	var presented atomic.Uint64
	loop := New(Config{Present: func(RenderResult) { presented.Add(1) }})
	loop.SetGeneration(Generation{Navigation: 2})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// A result built for generation 1 arrives after the engine moved to
	// generation 2: it must be dropped, not presented.
	if err := loop.SubmitRenderResult(RenderResult{Request: RenderRequest{
		Context: ctx, Generation: Generation{Navigation: 1},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := loop.ProcessPendingResults(); got != 1 {
		t.Fatalf("processed = %d, want 1", got)
	}
	if got := presented.Load(); got != 0 {
		t.Fatalf("stale frame was presented")
	}
	if got := loop.Metrics().StaleFramesDropped; got != 1 {
		t.Fatalf("stale frames = %d, want 1", got)
	}
}

func TestLoopShutsDownCleanly(t *testing.T) {
	loop := New(Config{})
	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	loop.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("loop did not shut down")
	}
	if err := loop.PostInput(InputEvent{Type: InputClick}); !errors.Is(err, ErrClosed) {
		t.Fatalf("PostInput error = %v, want ErrClosed", err)
	}
}

func TestFrameBudget(t *testing.T) {
	budget := NewFrameBudget(10 * time.Millisecond)
	start := time.Unix(0, 0)
	if got := budget.Remaining(start, start.Add(4*time.Millisecond)); got != 6*time.Millisecond {
		t.Fatalf("remaining = %v", got)
	}
	if !budget.Exhausted(start, start.Add(10*time.Millisecond)) {
		t.Fatal("budget should be exhausted")
	}
	if got := budget.Slice(start, start.Add(time.Millisecond), 3*time.Millisecond); got != 3*time.Millisecond {
		t.Fatalf("slice = %v", got)
	}
}

func waitForMetric(t *testing.T, loop *Loop, ready func(Metrics) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ready(loop.Metrics()) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("metric condition not reached: %#v", loop.Metrics())
}

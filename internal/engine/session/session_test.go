package session

import (
	"context"
	"errors"
	"net/http/cookiejar"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

// testTimeout is used in concurrent tests to prevent permanent blocking.
const testTimeout = 5 * time.Second

func TestNewSessionIsCreated(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.State() != StateCreated {
		t.Fatalf("initial state = %v, want %v", s.State(), StateCreated)
	}
	if s.NavID() != navigation.Invalid {
		t.Fatalf("initial NavID = %d, want invalid", s.NavID())
	}
	if s.ActiveURL() != "" {
		t.Fatalf("initial ActiveURL = %q, want empty", s.ActiveURL())
	}
	if s.NavigationErr() != nil {
		t.Fatalf("initial NavigationErr = %v, want nil", s.NavigationErr())
	}
}

func TestSessionNavigateTransitionsToNavigating(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")
	if !load.ID.IsValid() {
		t.Fatal("Navigate returned invalid load ID")
	}
	if load.URL != "https://example.com" {
		t.Fatalf("load URL = %q, want %q", load.URL, "https://example.com")
	}
	if ctx == nil {
		t.Fatal("Navigate returned nil context")
	}
	if s.State() != StateNavigating {
		t.Fatalf("state = %v, want %v", s.State(), StateNavigating)
	}
	if s.NavID() != load.ID {
		t.Fatalf("NavID = %d, want %d", s.NavID(), load.ID)
	}
	if s.ActiveURL() != "https://example.com" {
		t.Fatalf("ActiveURL = %q, want %q", s.ActiveURL(), "https://example.com")
	}
}

func TestSessionNavigateReturnsRecorderInContext(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")
	rec := metrics.RecorderFromContext(ctx)
	if rec == nil {
		t.Fatal("context missing metrics recorder")
	}
	snap := rec.Snapshot()
	if snap.NavID != uint64(load.ID) {
		t.Fatalf("recorder NavID = %d, want %d", snap.NavID, load.ID)
	}
	if snap.URL != "https://example.com" {
		t.Fatalf("recorder URL = %q, want %q", snap.URL, "https://example.com")
	}
}

func TestSessionCancelTransitionsToCancelled(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")

	s.Cancel()

	if s.State() != StateCancelled {
		t.Fatalf("state after Cancel = %v, want %v", s.State(), StateCancelled)
	}
	if s.IsActive(load.ID) {
		t.Fatal("Cancel navigation should not be active")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context was not cancelled by Cancel")
	}
}

func TestSessionCompleteTransitionsToComplete(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Complete()

	if s.State() != StateComplete {
		t.Fatalf("state after Complete = %v, want %v", s.State(), StateComplete)
	}
}

func TestSessionFailTransitionsToFailed(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")

	wantErr := errors.New("connection refused")
	s.Fail(wantErr)

	if s.State() != StateFailed {
		t.Fatalf("state after Fail = %v, want %v", s.State(), StateFailed)
	}
	if !errors.Is(s.NavigationErr(), wantErr) {
		t.Fatalf("NavigationErr = %v, want %v", s.NavigationErr(), wantErr)
	}
}

func TestSessionFailWithNilError(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Fail(nil)

	if s.State() != StateFailed {
		t.Fatalf("state after Fail(nil) = %v, want %v", s.State(), StateFailed)
	}
	if s.NavigationErr() == nil {
		t.Fatal("NavigationErr should not be nil after Fail(nil)")
	}
}

func TestSessionCloseTransitionsToClosed(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Close()

	if s.State() != StateClosed {
		t.Fatalf("state after Close = %v, want %v", s.State(), StateClosed)
	}
}

func TestSessionCloseReturnsZeroLoad(t *testing.T) {
	s := New()
	s.Close()

	load, ctx := s.Navigate(context.Background(), "https://example.com")
	if load.ID.IsValid() {
		t.Fatal("Navigate after Close should return invalid load")
	}
	if ctx != nil {
		t.Fatal("Navigate after Close should return nil context")
	}
}

func TestSessionCloseIsIdempotent(t *testing.T) {
	s := New()
	s.Close()
	s.Close() // must not panic
	if s.State() != StateClosed {
		t.Fatalf("state = %v, want %v", s.State(), StateClosed)
	}
}

func TestSessionCloseCancelsActiveNavigation(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")
	if !s.IsActive(load.ID) {
		t.Fatal("navigation should be active before Close")
	}

	s.Close()

	if s.IsActive(load.ID) {
		t.Fatal("navigation should not be active after Close")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context was not cancelled by Close")
	}
}

func TestSessionRepeatedNavigationAssignsNewIDs(t *testing.T) {
	s := New()
	first, _ := s.Navigate(context.Background(), "https://example.com/first")
	s.Complete()

	second, _ := s.Navigate(context.Background(), "https://example.com/second")

	if first.ID == second.ID {
		t.Fatalf("repeated navigation reused ID %d", first.ID)
	}
	if s.NavID() != second.ID {
		t.Fatalf("NavID = %d, want %d", s.NavID(), second.ID)
	}
	if s.State() != StateNavigating {
		t.Fatalf("state after second Navigate = %v, want %v", s.State(), StateNavigating)
	}
}

func TestSessionNavigateCancelsPreviousNavigation(t *testing.T) {
	s := New()
	first, firstCtx := s.Navigate(context.Background(), "https://example.com/first")

	second, secondCtx := s.Navigate(context.Background(), "https://example.com/second")

	// First context should be cancelled by second navigation
	select {
	case <-firstCtx.Done():
	default:
		t.Fatal("first navigation context was not cancelled by second navigation")
	}

	// First navigation should no longer be active
	if s.IsActive(first.ID) {
		t.Fatal("first navigation should not be active after second Navigate")
	}

	// Second context should still be active
	if !s.IsActive(second.ID) {
		t.Fatal("second navigation should be active")
	}
	select {
	case <-secondCtx.Done():
		t.Fatal("second navigation context should not be done")
	default:
	}
}

func TestSessionIsActiveRejectsStaleID(t *testing.T) {
	s := New()
	stale, _ := s.Navigate(context.Background(), "https://example.com/stale")
	current, _ := s.Navigate(context.Background(), "https://example.com/current")

	if !s.IsActive(current.ID) {
		t.Fatalf("current ID %d should be active", current.ID)
	}
	if s.IsActive(stale.ID) {
		t.Fatalf("stale ID %d should not be active", stale.ID)
	}
}

func TestSessionIsActiveAfterComplete(t *testing.T) {
	s := New()
	load, _ := s.Navigate(context.Background(), "https://example.com")
	s.Complete()

	if !s.IsActive(load.ID) {
		t.Fatal("completed navigation should be considered active (no superseding nav)")
	}
}

func TestSessionIsActiveAfterFailed(t *testing.T) {
	s := New()
	load, _ := s.Navigate(context.Background(), "https://example.com")
	s.Fail(errors.New("error"))

	if s.IsActive(load.ID) {
		t.Fatal("failed navigation should not be active")
	}
}

func TestSessionEventCallbackFiredOnNavigate(t *testing.T) {
	s := New()
	events := make([]Event, 0, 8)
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	load, _ := s.Navigate(context.Background(), "https://example.com")

	mu.Lock()
	if len(events) != 1 {
		mu.Unlock()
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].State != StateNavigating {
		t.Fatalf("event state = %v, want %v", events[0].State, StateNavigating)
	}
	if events[0].NavID != load.ID {
		t.Fatalf("event NavID = %d, want %d", events[0].NavID, load.ID)
	}
	if events[0].URL != "https://example.com" {
		t.Fatalf("event URL = %q, want %q", events[0].URL, "https://example.com")
	}
	if events[0].Err != nil {
		t.Fatalf("event Err = %v, want nil", events[0].Err)
	}
	mu.Unlock()
}

func TestSessionEventCallbackFiredOnAllTransitions(t *testing.T) {
	s := New()
	events := make([]Event, 0, 8)
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	s.Navigate(context.Background(), "https://example.com")
	s.Parsing()
	s.Interactive()
	s.Complete()

	mu.Lock()
	want := []State{StateNavigating, StateParsing, StateInteractive, StateComplete}
	if len(events) != len(want) {
		mu.Unlock()
		t.Fatalf("got %d events, want %d", len(events), len(want))
	}
	for i, ev := range events {
		if ev.State != want[i] {
			t.Errorf("event[%d] state = %v, want %v", i, ev.State, want[i])
		}
	}
	mu.Unlock()
}

func TestSessionEventCallbackFiredOnCancel(t *testing.T) {
	s := New()
	var got Event
	s.SetEventCallback(func(ev Event) {
		got = ev
	})

	s.Navigate(context.Background(), "https://example.com")
	s.Cancel()

	if got.State != StateCancelled {
		t.Fatalf("event state = %v, want %v", got.State, StateCancelled)
	}
}

func TestSessionEventCallbackFiredOnFail(t *testing.T) {
	s := New()
	var got Event
	s.SetEventCallback(func(ev Event) {
		got = ev
	})

	wantErr := errors.New("timeout")
	s.Navigate(context.Background(), "https://example.com")
	s.Fail(wantErr)

	if got.State != StateFailed {
		t.Fatalf("event state = %v, want %v", got.State, StateFailed)
	}
	if !errors.Is(got.Err, wantErr) {
		t.Fatalf("event Err = %v, want %v", got.Err, wantErr)
	}
}

func TestSessionEventCallbackRemovedWithNil(t *testing.T) {
	s := New()
	s.SetEventCallback(func(ev Event) {
		t.Fatal("callback should not be called after being removed")
	})
	s.SetEventCallback(nil)

	s.Navigate(context.Background(), "https://example.com")
}

func TestSessionLifecycleFullSequence(t *testing.T) {
	s := New()

	// Created -> Navigating -> Parsing -> Interactive -> Complete
	if s.State() != StateCreated {
		t.Fatalf("want created, got %v", s.State())
	}

	load, ctx := s.Navigate(context.Background(), "https://example.com")
	if s.State() != StateNavigating {
		t.Fatalf("want navigating, got %v", s.State())
	}
	if !s.IsActive(load.ID) {
		t.Fatal("navigation should be active")
	}

	s.Parsing()
	if s.State() != StateParsing {
		t.Fatalf("want parsing, got %v", s.State())
	}
	if !s.IsActive(load.ID) {
		t.Fatal("navigation should still be active during parsing")
	}

	s.Interactive()
	if s.State() != StateInteractive {
		t.Fatalf("want interactive, got %v", s.State())
	}

	s.Complete()
	if s.State() != StateComplete {
		t.Fatalf("want complete, got %v", s.State())
	}

	select {
	case <-ctx.Done():
		t.Fatal("context should remain alive on successful completion")
	default:
	}
}

func TestSessionLifecycleCancelledDuringParsing(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")
	s.Parsing()

	s.Cancel()

	if s.State() != StateCancelled {
		t.Fatalf("want cancelled, got %v", s.State())
	}
	if s.IsActive(load.ID) {
		t.Fatal("cancelled navigation should not be active")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context was not cancelled")
	}
}

func TestSessionLifecycleFailedDuringNavigation(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")

	err := errors.New("DNS resolution failed")
	s.Fail(err)

	if s.State() != StateFailed {
		t.Fatalf("want failed, got %v", s.State())
	}
	if s.IsActive(load.ID) {
		t.Fatal("failed navigation should not be active")
	}
	if !errors.Is(s.NavigationErr(), err) {
		t.Fatalf("NavigationErr = %v, want %v", s.NavigationErr(), err)
	}
	// Context should NOT be cancelled for explicit failure (caller handles it)
	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled by Fail")
	default:
	}
}

func TestSessionStateStrings(t *testing.T) {
	tests := []struct {
		s    State
		want string
	}{
		{StateCreated, "created"},
		{StateNavigating, "navigating"},
		{StateParsing, "parsing"},
		{StateInteractive, "interactive"},
		{StateComplete, "complete"},
		{StateCancelled, "cancelled"},
		{StateFailed, "failed"},
		{StateClosed, "closed"},
		{State(99), "state_99"},
	}
	for _, tt := range tests {
		got := tt.s.String()
		if got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tt.s), got, tt.want)
		}
	}
}

func TestSessionConcurrentNavigateSafe(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ctx := s.Navigate(context.Background(), "https://example.com/page")
			if ctx != nil {
				// Context will be cancelled by a subsequent Navigate call,
				// or by the session Close at test end.
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
		}()
	}
	// Give goroutines time to start and potentially get cancelled.
	time.Sleep(100 * time.Millisecond)
	s.Close()
	wg.Wait()
}

func TestSessionConcurrentStateAccessSafe(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.State()
			_ = s.NavID()
			_ = s.ActiveURL()
			_ = s.IsActive(navigation.ID(1))
		}()
	}
	wg.Wait()
}

func TestSessionConcurrentTransitionsSafe(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")

	var wg sync.WaitGroup
	transitions := []func(){
		func() { s.Parsing() },
		func() { s.Interactive() },
		func() { s.Complete() },
		func() { s.Cancel() },
		func() { s.Fail(errors.New("err")) },
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn := transitions[i%len(transitions)]
			fn()
		}()
	}
	wg.Wait()
	// Must not panic or deadlock
}

func TestSessionConcurrentEventCallbackSafe(t *testing.T) {
	s := New()
	var mu sync.Mutex
	count := 0
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Navigate(context.Background(), "https://example.com")
			s.Complete()
		}()
	}
	wg.Wait()
	// Must not deadlock
}

func TestSessionNavigateAfterFailed(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com/first")
	s.Fail(errors.New("error"))

	second, ctx := s.Navigate(context.Background(), "https://example.com/second")
	if !second.ID.IsValid() {
		t.Fatal("Navigate after Fail should return valid load")
	}
	if ctx == nil {
		t.Fatal("Navigate after Fail should return non-nil context")
	}
	if s.State() != StateNavigating {
		t.Fatalf("state = %v, want %v", s.State(), StateNavigating)
	}
	if s.NavID() != second.ID {
		t.Fatalf("NavID = %d, want %d", s.NavID(), second.ID)
	}
	// Previous error should be cleared
	if s.NavigationErr() != nil {
		t.Fatalf("NavigationErr should be nil after new Navigate, got %v", s.NavigationErr())
	}
}

func TestSessionNavigateAfterCancelled(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com/first")
	s.Cancel()

	second, ctx := s.Navigate(context.Background(), "https://example.com/second")
	if !second.ID.IsValid() {
		t.Fatal("Navigate after Cancel should return valid load")
	}
	if ctx == nil {
		t.Fatal("Navigate after Cancel should return non-nil context")
	}
	if s.State() != StateNavigating {
		t.Fatalf("state = %v, want %v", s.State(), StateNavigating)
	}
}

func TestSessionContextCarriesNavigationID(t *testing.T) {
	s := New()
	load, ctx := s.Navigate(context.Background(), "https://example.com")

	id, ok := navigation.IDFromContext(ctx)
	if !ok {
		t.Fatal("context missing navigation ID")
	}
	if id != load.ID {
		t.Fatalf("context ID = %d, load ID = %d", id, load.ID)
	}
}

func TestSessionHonoursParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	s := New()
	load, ctx := s.Navigate(parent, "https://example.com")
	if !load.ID.IsValid() {
		t.Fatal("Navigate with cancelled parent should still assign valid ID")
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("navigation context was not cancelled when parent is cancelled")
	}
}

func TestSessionNoOpsAfterClose(t *testing.T) {
	s := New()
	s.Close()

	// All these must be no-ops (no panic, no state change)
	s.Navigate(context.Background(), "https://example.com")
	s.Parsing()
	s.Interactive()
	s.Complete()
	s.Cancel()
	s.Fail(errors.New("err"))

	if s.State() != StateClosed {
		t.Fatalf("state after Close = %v, want %v", s.State(), StateClosed)
	}
}

func TestSessionMultipleCompleteIsSafe(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Complete()
	s.Complete() // second call should be safe
	if s.State() != StateComplete {
		t.Fatalf("state = %v, want %v", s.State(), StateComplete)
	}
}

func TestSessionMultipleCancelIsSafe(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Cancel()
	s.Cancel() // second call should be safe
	if s.State() != StateCancelled {
		t.Fatalf("state = %v, want %v", s.State(), StateCancelled)
	}
}

func TestSessionStateAfterCloseDoesNotFireTransitions(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Close()

	// After close, callback should not be fired for state transitions
	var called bool
	s.SetEventCallback(func(ev Event) {
		called = true
	})

	s.Parsing()
	s.Interactive()
	s.Complete()

	if called {
		t.Fatal("event callback was called after Close")
	}
}

func TestSessionRecorderPhasesWorkThroughContext(t *testing.T) {
	s := New()
	_, ctx := s.Navigate(context.Background(), "https://example.com")

	rec := metrics.RecorderFromContext(ctx)
	if rec == nil {
		t.Fatal("no recorder in context")
	}

	// Simulate phase recording as the renderer does
	rec.BeginPhase(metrics.PhaseParse)
	rec.BeginPhase(metrics.PhaseLayout)
	rec.EndPhase(metrics.PhaseParse)
	rec.EndPhase(metrics.PhaseLayout)
	rec.AddCounters(metrics.Counters{
		NodeCount:     42,
		RuleCount:     5,
		SelectorCount: 7,
	})

	s.Complete()

	snap := rec.Snapshot()
	if snap.Counters.NodeCount != 42 {
		t.Fatalf("NodeCount = %d, want 42", snap.Counters.NodeCount)
	}
	if snap.Counters.RuleCount != 5 {
		t.Fatalf("RuleCount = %d, want 5", snap.Counters.RuleCount)
	}
}

func TestSessionFinalizeRecorder(t *testing.T) {
	s := New()
	_, ctx := s.Navigate(context.Background(), "https://example.com")

	rec := metrics.RecorderFromContext(ctx)
	rec.BeginPhase(metrics.PhaseParse)
	rec.EndPhase(metrics.PhaseParse)

	m := rec.Finalize()

	if len(m.Timings) != 1 {
		t.Fatalf("got %d timings, want 1", len(m.Timings))
	}
	if m.Timings[0].Phase != metrics.PhaseParse {
		t.Fatalf("phase = %v, want %v", m.Timings[0].Phase, metrics.PhaseParse)
	}
}

func TestFromContext(t *testing.T) {
	// Verify the utility function compiles and works
	ctx := context.Background()
	rec := RecorderFromContext(ctx)
	if rec != nil {
		t.Fatal("expected nil recorder from background context")
	}
}

// --- Transport tests ---

func TestNewSessionHasTransport(t *testing.T) {
	s := New()
	if s.Transport() == nil {
		t.Fatal("New() session has nil transport")
	}
}

func TestSessionTransportDefaults(t *testing.T) {
	s := New()
	tr := s.Transport()
	if tr.MaxIdleConns != 100 {
		t.Fatalf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 6 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 6", tr.MaxIdleConnsPerHost)
	}
	if tr.MaxConnsPerHost != 6 {
		t.Fatalf("MaxConnsPerHost = %d, want 6", tr.MaxConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 should be true")
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
}

func TestSessionHTTPClientUsesSharedTransport(t *testing.T) {
	s := New()
	c1 := s.HTTPClient()
	c2 := s.HTTPClient()

	want := s.Transport()
	if c1.Transport != want {
		t.Fatal("HTTPClient().Transport is not the session's transport")
	}
	if c2.Transport != want {
		t.Fatal("second HTTPClient().Transport is not the session's transport")
	}
	if c1 == c2 {
		t.Fatal("HTTPClient() should return a new client each call")
	}
}

func TestSessionHTTPClientHasTimeout(t *testing.T) {
	s := New()
	c := s.HTTPClient()
	if c.Timeout != 30*time.Second {
		t.Fatalf("HTTPClient().Timeout = %v, want 30s", c.Timeout)
	}
}

func TestSessionHTTPClientHasCookieJar(t *testing.T) {
	s := New()
	c := s.HTTPClient()
	if c.Jar == nil {
		t.Fatal("HTTPClient() missing cookie jar")
	}
	_, ok := c.Jar.(*cookiejar.Jar)
	if !ok {
		t.Fatalf("HTTPClient() Jar = %T, want *cookiejar.Jar", c.Jar)
	}
}

func TestSessionCloseClosesTransport(t *testing.T) {
	s := New()
	tr := s.Transport()

	// Close once — should close idle connections without panic.
	s.Close()

	// Transport should still be the same object (not nil'd) but idle
	// connections are closed. Calling CloseIdleConnections again is safe.
	tr.CloseIdleConnections()
	if s.Transport() == nil {
		t.Fatal("Transport() should not return nil after Close")
	}
}

func TestSessionCloseIdempotentTransport(t *testing.T) {
	s := New()
	s.Close()
	s.Close()         // must not panic
	_ = s.Transport() // must not panic
}

func TestSessionTransportSurvivesNavigationCancel(t *testing.T) {
	s := New()
	tr := s.Transport()

	// Navigate and cancel repeatedly — transport should remain intact.
	for i := 0; i < 10; i++ {
		s.Navigate(context.Background(), "https://example.com")
		s.Cancel()
		if s.Transport() == nil {
			t.Fatal("transport became nil after Cancel")
		}
		if s.Transport() != tr {
			t.Fatal("transport was replaced after Cancel")
		}
	}

	// Transport should still be functional.
	client := s.HTTPClient()
	if client.Transport != tr {
		t.Fatal("HTTPClient() uses a different transport after cancel")
	}
}

func TestSessionTransportSameAcrossNavigations(t *testing.T) {
	s := New()
	tr := s.Transport()

	// Run several complete navigation cycles.
	for i := 0; i < 5; i++ {
		s.Navigate(context.Background(), "https://example.com")
		s.Complete()
		if s.Transport() != tr {
			t.Fatal("transport was replaced after navigation")
		}
	}
}

func TestSessionTransportHTTPClientConcurrentSafe(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := s.HTTPClient()
			if c.Transport != s.Transport() {
				t.Error("HTTPClient() returned client with wrong transport")
			}
		}()
	}
	wg.Wait()
}

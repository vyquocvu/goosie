package session

import (
	"context"
	"errors"
	"net/http/cookiejar"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	engineNet "github.com/vyquocvu/goosie/internal/net"
)

// testTimeout is used in concurrent tests to prevent permanent blocking.
const testTimeout = 5 * time.Second

func TestNewSessionIsCreated(t *testing.T) {
	s := New()
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
	s.Navigate(context.Background(), "https://example.com")
	s.Complete()

	if s.State() != StateComplete {
		t.Fatalf("state after Complete = %v, want %v", s.State(), StateComplete)
	}
}

func TestSessionFailTransitionsToFailed(t *testing.T) {
	s := New()
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
	load, _ := s.Navigate(context.Background(), "https://example.com")
	s.Complete()

	if !s.IsActive(load.ID) {
		t.Fatal("completed navigation should be considered active (no superseding nav)")
	}
}

func TestSessionIsActiveAfterFailed(t *testing.T) {
	s := New()
	defer s.Close()
	load, _ := s.Navigate(context.Background(), "https://example.com")
	s.Fail(errors.New("error"))

	if s.IsActive(load.ID) {
		t.Fatal("failed navigation should not be active")
	}
}

func TestSessionEventCallbackFiredOnNavigate(t *testing.T) {
	s := New()
	defer s.Close()
	events := make([]Event, 0, 8)
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	load, _ := s.Navigate(context.Background(), "https://example.com")
	s.FlushEvents()

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
	defer s.Close()
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
	s.FlushEvents()

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
	defer s.Close()
	var got Event
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		got = ev
		mu.Unlock()
	})

	s.Navigate(context.Background(), "https://example.com")
	s.Cancel()
	s.FlushEvents()

	mu.Lock()
	if got.State != StateCancelled {
		t.Fatalf("event state = %v, want %v", got.State, StateCancelled)
	}
	mu.Unlock()
}

func TestSessionEventCallbackFiredOnFail(t *testing.T) {
	s := New()
	defer s.Close()
	events := make([]Event, 0, 8)
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	})

	wantErr := errors.New("timeout")
	s.Navigate(context.Background(), "https://example.com")
	s.Fail(wantErr)
	s.FlushEvents()

	mu.Lock()
	// Fail triggers StateChange (StateFailed) and EventError
	if len(events) < 2 {
		t.Fatalf("got %d events, want at least 2", len(events))
	}
	stateEv := events[len(events)-2]
	errEv := events[len(events)-1]

	if stateEv.State != StateFailed {
		t.Fatalf("event state = %v, want %v", stateEv.State, StateFailed)
	}
	if !errors.Is(stateEv.Err, wantErr) {
		t.Fatalf("event Err = %v, want %v", stateEv.Err, wantErr)
	}
	if errEv.Type != EventError {
		t.Fatalf("event type = %v, want %v", errEv.Type, EventError)
	}
	if !errors.Is(errEv.Err, wantErr) {
		t.Fatalf("event Err = %v, want %v", errEv.Err, wantErr)
	}
	mu.Unlock()
}

func TestSessionEventCallbackRemovedWithNil(t *testing.T) {
	s := New()
	defer s.Close()
	s.SetEventCallback(func(ev Event) {
		t.Fatal("callback should not be called after being removed")
	})
	s.SetEventCallback(nil)

	s.Navigate(context.Background(), "https://example.com")
	s.FlushEvents()
}

func TestSessionLifecycleFullSequence(t *testing.T) {
	s := New()
	defer s.Close()

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
	defer s.Close()
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
	defer s.Close()
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
				select {
				case <-ctx.Done():
				case <-time.After(time.Second):
				}
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	s.Close()
	wg.Wait()
}

func TestSessionConcurrentStateAccessSafe(t *testing.T) {
	s := New()
	defer s.Close()
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
	defer s.Close()
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
}

func TestSessionConcurrentEventCallbackSafe(t *testing.T) {
	s := New()
	defer s.Close()
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
}

func TestSessionNavigateAfterFailed(t *testing.T) {
	s := New()
	defer s.Close()
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
	if s.NavigationErr() != nil {
		t.Fatalf("NavigationErr should be nil after new Navigate, got %v", s.NavigationErr())
	}
}

func TestSessionNavigateAfterCancelled(t *testing.T) {
	s := New()
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
	s.Navigate(context.Background(), "https://example.com")
	s.Complete()
	s.Complete()
	if s.State() != StateComplete {
		t.Fatalf("state = %v, want %v", s.State(), StateComplete)
	}
}

func TestSessionMultipleCancelIsSafe(t *testing.T) {
	s := New()
	defer s.Close()
	s.Navigate(context.Background(), "https://example.com")
	s.Cancel()
	s.Cancel()
	if s.State() != StateCancelled {
		t.Fatalf("state = %v, want %v", s.State(), StateCancelled)
	}
}

func TestSessionStateAfterCloseDoesNotFireTransitions(t *testing.T) {
	s := New()
	s.Navigate(context.Background(), "https://example.com")
	s.Close()

	var called bool
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	s.Parsing()
	s.Interactive()
	s.Complete()
	s.FlushEvents()

	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Fatal("event callback was called after Close")
	}
}

func TestSessionRecorderPhasesWorkThroughContext(t *testing.T) {
	s := New()
	defer s.Close()
	_, ctx := s.Navigate(context.Background(), "https://example.com")

	rec := metrics.RecorderFromContext(ctx)
	if rec == nil {
		t.Fatal("no recorder in context")
	}

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
	defer s.Close()
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
	ctx := context.Background()
	rec := RecorderFromContext(ctx)
	if rec != nil {
		t.Fatal("expected nil recorder from background context")
	}
}

func TestNewSessionHasTransport(t *testing.T) {
	s := New()
	defer s.Close()
	if s.Transport() == nil {
		t.Fatal("New() session has nil transport")
	}
}

func TestSessionTransportDefaults(t *testing.T) {
	s := New()
	defer s.Close()
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
	defer s.Close()
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
	defer s.Close()
	c := s.HTTPClient()
	if c.Timeout != 30*time.Second {
		t.Fatalf("HTTPClient().Timeout = %v, want 30s", c.Timeout)
	}
}

func TestSessionHTTPClientHasCookieJar(t *testing.T) {
	s := New()
	defer s.Close()
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
	s.Close()

	tr.CloseIdleConnections()
	if s.Transport() == nil {
		t.Fatal("Transport() should not return nil after Close")
	}
}

func TestSessionCloseIdempotentTransport(t *testing.T) {
	s := New()
	s.Close()
	s.Close()
	_ = s.Transport()
}

func TestSessionTransportSurvivesNavigationCancel(t *testing.T) {
	s := New()
	defer s.Close()
	tr := s.Transport()

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

	client := s.HTTPClient()
	if client.Transport != tr {
		t.Fatal("HTTPClient() uses a different transport after cancel")
	}
}

func TestSessionTransportSameAcrossNavigations(t *testing.T) {
	s := New()
	defer s.Close()
	tr := s.Transport()

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
	defer s.Close()
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

// --- Formalized Event Queue and New Events Tests ---

func TestSessionEventQueueOldestDropped(t *testing.T) {
	s := New()
	defer s.Close()

	// Register a slow callback that blocks on a channel so the dispatch loop blocks
	blockChan := make(chan struct{})
	processedChan := make(chan struct{})
	eventsReceived := make([]Event, 0)
	var mu sync.Mutex

	var once sync.Once
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		eventsReceived = append(eventsReceived, ev)
		mu.Unlock()
		once.Do(func() {
			processedChan <- struct{}{}
			<-blockChan
		})
	})

	// Send an event to block the callback
	s.Title("Initial Block Event")

	// Wait until the dispatcher receives the first event and starts blocking
	select {
	case <-processedChan:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for block callback to start")
	}

	// Now fill the queue completely.
	// The queue has capacity 256. We'll send 260 events.
	// Since the dispatcher is blocked, these events will sit in the queue.
	// The queue will overflow, and we should drop the oldest of the overflowed ones.
	for i := 1; i <= 260; i++ {
		s.Progress(float64(i))
	}

	// Unblock the callback
	close(blockChan)

	// Wait for the dispatcher loop to process all remaining queued events.
	// We'll queue a sync event and block on it.
	s.FlushEvents()

	mu.Lock()
	defer mu.Unlock()

	// Initial event is 1. The remaining capacity in queue is 256.
	// When we wrote 260 progress events:
	// - Event 1 is the block event (already dispatched/read from queue)
	// - The queue can hold 256 events.
	// - Writing 260 progress events means 4 of the earliest progress events must have been dropped.
	// So we expect to have received:
	// - 1 blocker event
	// - 256 progress events (the last ones, i.e., progress values 5 to 260)
	// Total events received should be exactly 257.
	if len(eventsReceived) != 257 {
		t.Fatalf("got %d events, want %d", len(eventsReceived), 257)
	}

	// The first progress event received should be 5.0 (since 1.0, 2.0, 3.0, 4.0 were dropped)
	if eventsReceived[1].Progress != 5.0 {
		t.Fatalf("first progress received was %f, want %f", eventsReceived[1].Progress, 5.0)
	}
	// The last progress event received should be 260.0
	if eventsReceived[256].Progress != 260.0 {
		t.Fatalf("last progress received was %f, want %f", eventsReceived[256].Progress, 260.0)
	}
}

func TestSessionEventImmutability(t *testing.T) {
	s := New()
	defer s.Close()

	var received []Event
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		received = append(received, ev)
		mu.Unlock()
	})

	sec := engineNet.SecuritySummary{
		URL:     "https://secure.com",
		Scheme:  "https",
		Secure:  true,
		Subject: "CN=secure.com",
		Issuer:  "CN=CA",
	}
	dl := engineNet.DownloadRecord{
		URL:        "https://secure.com/file.zip",
		TargetPath: "/tmp/file.zip",
		Status:     engineNet.DownloadStatusRunning,
	}

	s.Navigate(context.Background(), "https://secure.com")
	s.Security(sec)
	s.Download(dl)
	s.FlushEvents()

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 3 {
		t.Fatalf("got %d events, want at least 3", len(received))
	}

	// Check SecuritySummary event details
	var foundSec, foundDl bool
	for _, ev := range received {
		if ev.Type == EventSecuritySummary {
			foundSec = true
			if ev.SecuritySummary.URL != sec.URL || ev.SecuritySummary.Secure != sec.Secure {
				t.Fatalf("SecuritySummary event mismatch: %+v", ev.SecuritySummary)
			}
		}
		if ev.Type == EventDownload {
			foundDl = true
			if ev.Download.URL != dl.URL || ev.Download.Status != dl.Status {
				t.Fatalf("Download event mismatch: %+v", ev.Download)
			}
		}
	}
	if !foundSec {
		t.Fatal("missing SecuritySummary event")
	}
	if !foundDl {
		t.Fatal("missing Download event")
	}
}

func TestSessionGoroutineLeakCheck(t *testing.T) {
	// Count active goroutines before creating sessions
	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()

	sessions := make([]*Session, 10)
	for i := 0; i < 10; i++ {
		sessions[i] = New()
	}

	// Active goroutines should be higher now
	activeGoroutines := runtime.NumGoroutine()
	if activeGoroutines <= initialGoroutines {
		t.Fatalf("expected goroutine count to increase after creating sessions, got initial=%d, active=%d", initialGoroutines, activeGoroutines)
	}

	// Close all sessions
	for i := 0; i < 10; i++ {
		sessions[i].Close()
	}

	// Give background threads a brief moment to exit and run GC
	time.Sleep(50 * time.Millisecond)
	runtime.GC()

	finalGoroutines := runtime.NumGoroutine()
	// Allow slight variance but should be close to initial
	if finalGoroutines > initialGoroutines+2 {
		t.Fatalf("goroutine leak detected: initial=%d, active=%d, final=%d", initialGoroutines, activeGoroutines, finalGoroutines)
	}
}

func TestSessionNewEvents(t *testing.T) {
	s := New()
	defer s.Close()

	events := make(map[EventType][]Event)
	var mu sync.Mutex
	s.SetEventCallback(func(ev Event) {
		mu.Lock()
		events[ev.Type] = append(events[ev.Type], ev)
		mu.Unlock()
	})

	s.Title("Hello World")
	s.URL("https://example.com/new")
	s.FirstPaint()
	s.Progress(0.75)
	s.FlushEvents()

	mu.Lock()
	defer mu.Unlock()

	if len(events[EventTitleChange]) != 1 || events[EventTitleChange][0].Title != "Hello World" {
		t.Fatalf("incorrect TitleChange event: %+v", events[EventTitleChange])
	}
	if len(events[EventURLChange]) != 1 || events[EventURLChange][0].URL != "https://example.com/new" {
		t.Fatalf("incorrect URLChange event: %+v", events[EventURLChange])
	}
	if len(events[EventFirstPaint]) != 1 {
		t.Fatalf("incorrect FirstPaint event: %+v", events[EventFirstPaint])
	}
	if len(events[EventProgress]) != 1 || events[EventProgress][0].Progress != 0.75 {
		t.Fatalf("incorrect Progress event: %+v", events[EventProgress])
	}
}

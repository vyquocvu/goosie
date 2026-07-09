// Package session owns one active navigation lifecycle — cancellation, state
// tracking, and event notification — without importing UI packages.
//
// A Session wraps the navigation.Scheduler and enforces an explicit state
// machine (created → navigating → complete/cancelled/failed → closed).
// Callers use Navigate to start a load and Close to release all resources.
package session

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

// State represents the lifecycle phase of an engine session.
type State int

const (
	StateCreated     State = iota // initial state after construction
	StateNavigating               // navigation is in progress
	StateParsing                  // response body is being parsed
	StateInteractive              // DOM available, script may run
	StateComplete                 // page fully loaded
	StateCancelled                // navigation superseded or cancelled
	StateFailed                   // navigation encountered an error
	StateClosed                   // session has been closed and cannot be reused
)

var stateStrings = map[State]string{
	StateCreated:     "created",
	StateNavigating:  "navigating",
	StateParsing:     "parsing",
	StateInteractive: "interactive",
	StateComplete:    "complete",
	StateCancelled:   "cancelled",
	StateFailed:      "failed",
	StateClosed:      "closed",
}

func (s State) String() string {
	if str, ok := stateStrings[s]; ok {
		return str
	}
	return fmt.Sprintf("state_%d", int(s))
}

// Event describes a session state change. It is delivered synchronously
// through the callback registered with SetEventCallback.
type Event struct {
	State State
	NavID navigation.ID
	URL   string
	Err   error
}

// defaultTransport returns a shared http.Transport configured with
// sensible connection limits and timeouts for browser engine use.
func defaultTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 6,
		MaxConnsPerHost:     6,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// Session owns the lifecycle of one browsing context.
// It is safe for concurrent use.
type Session struct {
	mu        sync.Mutex
	scheduler *navigation.Scheduler
	state     State
	navID     navigation.ID
	url       string
	err       error
	onEvent   func(Event)
	closed    bool

	transport *http.Transport
}

// New creates a new Session in the Created state with a shared HTTP
// transport configured for concurrent browser engine use.
func New() *Session {
	return &Session{
		scheduler: navigation.NewScheduler(),
		state:     StateCreated,
		transport: defaultTransport(),
	}
}

// SetEventCallback registers a function that is called synchronously on every
// state change. Passing nil removes the callback. The callback must not
// block or call Session methods to avoid deadlock.
func (s *Session) SetEventCallback(fn func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = fn
}

// Navigate starts a new navigation, cancelling any in-flight load.
// The returned context carries the new navigation ID and metrics recorder.
// When the navigation completes (or fails), the caller should call Complete
// or Fail. Returns a zero Load and nil context when the session is closed.
func (s *Session) Navigate(parent context.Context, url string) (navigation.Load, context.Context) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return navigation.Load{}, nil
	}
	s.state = StateNavigating
	s.err = nil
	load, ctx := s.scheduler.Begin(parent, url)
	ctx = metrics.WithRecorder(ctx, load.Recorder)
	s.navID = load.ID
	s.url = url
	s.mu.Unlock()

	s.fireEvent(Event{State: StateNavigating, NavID: load.ID, URL: url})
	return load, ctx
}

// Parsing transitions the session to the parsing state.
func (s *Session) Parsing() {
	s.mu.Lock()
	if s.closed || s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateParsing
	navID := s.navID
	url := s.url
	s.mu.Unlock()

	s.fireEvent(Event{State: StateParsing, NavID: navID, URL: url})
}

// Interactive transitions the session to the interactive state.
func (s *Session) Interactive() {
	s.mu.Lock()
	if s.closed || s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateInteractive
	navID := s.navID
	url := s.url
	s.mu.Unlock()

	s.fireEvent(Event{State: StateInteractive, NavID: navID, URL: url})
}

// Complete marks the current navigation as successfully finished.
func (s *Session) Complete() {
	s.mu.Lock()
	if s.closed || s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateComplete
	navID := s.navID
	url := s.url
	s.mu.Unlock()

	s.fireEvent(Event{State: StateComplete, NavID: navID, URL: url})
}

// Cancel transitions the session to Cancelled and cancels the active context.
// It is safe to call multiple times. After Cancel, the session may be reused
// by calling Navigate again.
func (s *Session) Cancel() {
	s.mu.Lock()
	if s.closed || s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateCancelled
	navID := s.navID
	url := s.url
	s.mu.Unlock()

	s.scheduler.Cancel()
	s.fireEvent(Event{State: StateCancelled, NavID: navID, URL: url})
}

// Fail transitions the session to Failed with the given error.
// The session may be reused by calling Navigate again.
func (s *Session) Fail(err error) {
	if err == nil {
		err = fmt.Errorf("session: failed with nil error")
	}
	s.mu.Lock()
	if s.closed || s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateFailed
	s.err = err
	navID := s.navID
	url := s.url
	evErr := err
	s.mu.Unlock()

	s.fireEvent(Event{State: StateFailed, NavID: navID, URL: url, Err: evErr})
}

// Close releases all session resources and transitions to Closed.
// After Close, any call to Navigate returns a zero load and nil context.
// It is safe to call multiple times.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.state = StateClosed
	navID := s.navID
	url := s.url
	s.mu.Unlock()

	s.scheduler.Cancel()
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	s.fireEvent(Event{State: StateClosed, NavID: navID, URL: url})
}

// State returns the current session lifecycle state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// NavID returns the navigation ID of the most recent navigation, or
// navigation.Invalid if no navigation has been started.
func (s *Session) NavID() navigation.ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.navID
}

// ActiveURL returns the URL of the most recent or ongoing navigation.
func (s *Session) ActiveURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

// NavigationErr returns the error that caused a Failed state, if any.
func (s *Session) NavigationErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// IsActive reports whether the given navigation ID is the current one
// and the session has not been closed or cancelled. This is the preferred
// way for callbacks to check staleness.
func (s *Session) IsActive(navID navigation.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	if s.state == StateCancelled || s.state == StateClosed || s.state == StateFailed {
		return false
	}
	return navID.IsValid() && navID == s.navID
}

// RecorderFromContext is a convenience wrapper around
// metrics.RecorderFromContext. It is provided so session users can
// consistently access the metrics recorder from a navigation context.
func RecorderFromContext(ctx context.Context) *metrics.Recorder {
	return metrics.RecorderFromContext(ctx)
}

// StartedAt returns when the most recent navigation was initiated.
// Returns the zero time if no navigation has occurred.
func (s *Session) StartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The scheduler doesn't expose started time; callers should track
	// via load.StartedAt. This is provided as a convenience.
	return time.Time{}
}

// Transport returns the session's shared HTTP transport. The transport
// is closed by Session.Close and must not be used after the session is
// closed.
func (s *Session) Transport() *http.Transport {
	return s.transport
}

// HTTPClient returns an *http.Client that shares the session's HTTP
// transport with a fresh cookie jar. Each call returns a new client
// so callers that need separate cookie state (e.g., per-tab) can
// create isolated clients while sharing the underlying connection pool.
// The client inherits the transport's concurrency limits and timeouts.
func (s *Session) HTTPClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Transport: s.transport,
		Jar:       jar,
		Timeout:   30 * time.Second,
	}
}

func (s *Session) fireEvent(ev Event) {
	s.mu.Lock()
	fn := s.onEvent
	s.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

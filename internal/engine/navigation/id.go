// Package navigation assigns monotonic navigation IDs and manages
// cancellable navigation contexts without depending on UI packages.
package navigation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
)

// ID uniquely identifies one page load within a browser session.
type ID uint64

// Invalid is the zero navigation ID and is never assigned to a load.
const Invalid ID = 0

// IsValid reports whether the ID was assigned to a navigation.
func (id ID) IsValid() bool {
	return id != Invalid
}

// String returns a decimal representation for logs and diagnostics.
func (id ID) String() string {
	return fmt.Sprintf("%d", uint64(id))
}

// IDGenerator produces monotonically increasing navigation IDs.
type IDGenerator struct {
	mu   sync.Mutex
	next ID
}

// NewIDGenerator creates a generator whose first ID is 1.
func NewIDGenerator() *IDGenerator {
	return &IDGenerator{}
}

// Next returns a new unique navigation ID.
func (g *IDGenerator) Next() ID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next
}

type idContextKey struct{}

// WithID returns a child context that carries navID.
func WithID(ctx context.Context, navID ID) context.Context {
	return context.WithValue(ctx, idContextKey{}, navID)
}

// IDFromContext returns the navigation ID stored in ctx, if present.
func IDFromContext(ctx context.Context) (ID, bool) {
	navID, ok := ctx.Value(idContextKey{}).(ID)
	if !ok || !navID.IsValid() {
		return Invalid, false
	}
	return navID, true
}

// Load describes one navigation request.
type Load struct {
	ID        ID
	URL       string
	Priority  Priority
	StartedAt time.Time
	Recorder  *metrics.Recorder
}

// Scheduler assigns navigation IDs and owns the active load context.
type Scheduler struct {
	generator       *IDGenerator
	mu              sync.Mutex
	activeID        ID
	activeCancel    context.CancelFunc
	pending         map[ID]Load               // all in-flight loads keyed by ID
	resourceCancels map[ID]context.CancelFunc // sub-resource cancel funcs
}

// NewScheduler creates a scheduler ready to assign navigation IDs.
func NewScheduler() *Scheduler {
	return &Scheduler{
		generator: NewIDGenerator(),
		pending:   make(map[ID]Load),
	}
}

// startedAtNow returns the current wall-clock time. Extracted so that
// BeginWithPriority can share the same timestamp source.
func startedAtNow() time.Time { return time.Now() }

// newRecorder returns a metrics.Recorder wired to the given navigation.
func newRecorder(navID ID, url string) *metrics.Recorder {
	return metrics.NewRecorder(uint64(navID), url)
}

// Begin cancels any in-flight navigation, assigns a new ID, and returns
// load metadata plus a cancellable context that carries the navigation ID.
// The load is assigned PriorityDocument by default.
func (s *Scheduler) Begin(parent context.Context, url string) (Load, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancelPreviousLocked()

	navID := s.generator.Next()
	load := Load{
		ID:        navID,
		URL:       url,
		Priority:  PriorityDocument,
		StartedAt: startedAtNow(),
		Recorder:  newRecorder(navID, url),
	}
	s.activeID = load.ID
	s.pending[navID] = load

	ctx, cancel := context.WithCancel(parent)
	s.activeCancel = cancel
	ctx = WithID(ctx, load.ID)
	ctx = WithPriority(ctx, PriorityDocument)

	return load, ctx
}

// cancelPreviousLocked cancels the active navigation and all associated
// sub-resource contexts. The caller must hold s.mu.
func (s *Scheduler) cancelPreviousLocked() {
	if s.activeCancel != nil {
		s.activeCancel()
		s.activeCancel = nil
	}
	if s.activeID.IsValid() {
		delete(s.pending, s.activeID)
	}
	for id, cancel := range s.resourceCancels {
		cancel()
		delete(s.resourceCancels, id)
		delete(s.pending, id)
	}
}

// ActiveID returns the currently active navigation ID, or Invalid if none.
func (s *Scheduler) ActiveID() ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeID
}

// IsActive reports whether navID is the currently active navigation.
func (s *Scheduler) IsActive(navID ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return navID.IsValid() && navID == s.activeID
}

// Cancel stops the active navigation and clears the active ID.
// All pending loads (including sub-resources) are removed and their
// contexts are cancelled.
func (s *Scheduler) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancelPreviousLocked()
	s.activeID = Invalid
	s.pending = make(map[ID]Load)
}

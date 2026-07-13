package navigation

import (
	"context"
	"fmt"
	"sort"
)

// Priority describes the loading urgency of a resource within a navigation.
// Lower numeric values indicate higher priority. The scheduler uses priorities
// to order pending loads; downstream network layers may use them to bound
// concurrent requests per origin and globally.
type Priority uint8

// Resource priority levels ordered from highest (Document) to lowest
// (Speculative). Values are stable and may be persisted or compared.
const (
	PriorityDocument      Priority = iota + 1 // main document navigation
	PriorityBlockingCSS                       // render-blocking stylesheet
	PriorityVisibleImage                      // image in or near the viewport
	PriorityScript                            // synchronous or async script
	PriorityDeferredImage                     // below-fold or lazy image
	PrioritySpeculative                       // prefetch, prerender, dns-prefetch
)

var priorityNames = [...]string{
	PriorityDocument:      "document",
	PriorityBlockingCSS:   "blocking_css",
	PriorityVisibleImage:  "visible_image",
	PriorityScript:        "script",
	PriorityDeferredImage: "deferred_image",
	PrioritySpeculative:   "speculative",
}

// String returns a stable, human-readable label for logging and diagnostics.
func (p Priority) String() string {
	if int(p) < len(priorityNames) && priorityNames[p] != "" {
		return priorityNames[p]
	}
	return fmt.Sprintf("unknown_priority_%d", p)
}

type priorityContextKey struct{}

// WithPriority returns a child context that carries prio.
func WithPriority(ctx context.Context, prio Priority) context.Context {
	return context.WithValue(ctx, priorityContextKey{}, prio)
}

// PriorityFromContext returns the Priority stored in ctx, if present.
func PriorityFromContext(ctx context.Context) (Priority, bool) {
	p, ok := ctx.Value(priorityContextKey{}).(Priority)
	return p, ok
}

// BeginWithPriority starts a main-document navigation with an explicit
// resource priority. It behaves identically to Begin otherwise: the previous
// navigation is cancelled, a new ID is assigned, and the returned context
// carries both the navigation ID and the priority.
func (s *Scheduler) BeginWithPriority(parent context.Context, url string, prio Priority) (Load, context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel any in-flight navigation and remove its pending entry.
	s.cancelPreviousLocked()

	navID := s.generator.Next()
	load := Load{
		ID:        navID,
		URL:       url,
		Priority:  prio,
		StartedAt: startedAtNow(),
		Recorder:  newRecorder(navID, url),
	}
	s.activeID = load.ID
	s.pending[navID] = load

	ctx, cancel := context.WithCancel(parent)
	s.activeCancel = cancel
	ctx = WithID(ctx, load.ID)
	ctx = WithPriority(ctx, prio)

	return load, ctx
}

// AddResource registers a sub-resource load (stylesheet, image, script, etc.)
// under the active navigation. Unlike Begin/BeginWithPriority, it does not
// cancel the main navigation. Multiple sub-resources may coexist. The returned
// context is derived from parent and carries the resource's ID and priority.
// If no active navigation exists, the resource is still tracked but its
// context is derived solely from parent.
//
// If the scheduler has a RateLimiter, Acquire is called before registering the
// resource. If the context is cancelled while waiting, a zero Load and the
// cancelled context are returned without registering the resource.
func (s *Scheduler) AddResource(parent context.Context, rawURL string, prio Priority) (Load, context.Context) {
	origin, _ := ParseOrigin(rawURL)
	originKey := origin.Host()

	// Admit through rate limiter before taking scheduler lock.
	if s.limiter != nil {
		if err := s.limiter.Acquire(parent, originKey, prio); err != nil {
			// Context cancelled while waiting — do not register.
			return Load{}, parent
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	resID := s.generator.Next()
	load := Load{
		ID:        resID,
		URL:       rawURL,
		Origin:    origin,
		Priority:  prio,
		StartedAt: startedAtNow(),
	}
	s.pending[resID] = load

	ctx, cancel := context.WithCancel(parent)
	// Store the cancel func keyed by resource ID so RemoveResource can clean up.
	if s.resourceCancels == nil {
		s.resourceCancels = make(map[ID]context.CancelFunc)
	}
	s.resourceCancels[resID] = cancel
	ctx = WithID(ctx, resID)
	ctx = WithPriority(ctx, prio)

	return load, ctx
}

// RemoveResource marks a sub-resource as finished and releases its resources.
// If the scheduler has a RateLimiter, Release is called for the resource's origin.
// It is safe to call for IDs that have already been removed.
func (s *Scheduler) RemoveResource(id ID) {
	s.mu.Lock()
	load, ok := s.pending[id]
	delete(s.pending, id)
	if cancel, ok2 := s.resourceCancels[id]; ok2 {
		cancel()
		delete(s.resourceCancels, id)
	}
	s.mu.Unlock()

	if ok && s.limiter != nil && load.Origin.IsValid() {
		s.limiter.Release(load.Origin.Host())
	}
}

// PendingLoads returns a snapshot of all pending loads (main navigation and
// sub-resources) sorted by priority (highest priority first, i.e. lowest
// Priority value first). Ties are broken by ID. The returned slice is a copy.
func (s *Scheduler) PendingLoads() []Load {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) == 0 {
		return nil
	}

	loads := make([]Load, 0, len(s.pending))
	for _, l := range s.pending {
		loads = append(loads, l)
	}
	sort.Slice(loads, func(i, j int) bool {
		if loads[i].Priority != loads[j].Priority {
			return loads[i].Priority < loads[j].Priority
		}
		return loads[i].ID < loads[j].ID // stable tie-break by ID
	})
	return loads
}

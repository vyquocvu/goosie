package mcpserver

import (
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	mu         sync.Mutex
	capacity   int
	refillRate float64 // tokens per second
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter with the given capacity and refill rate.
// capacity is the maximum burst size.
// refillRate is tokens per second.
func NewRateLimiter(capacity int, refillRate float64) *RateLimiter {
	return &RateLimiter{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     float64(capacity),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request can proceed under the rate limit.
// Returns true if allowed, false if rate limited.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()
	r.lastRefill = now

	// Refill tokens
	r.tokens += elapsed * r.refillRate
	if r.tokens > float64(r.capacity) {
		r.tokens = float64(r.capacity)
	}

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// Tokens returns the current number of tokens (approximate).
func (r *RateLimiter) Tokens() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens
}

// QuotaTracker tracks resource usage against configured limits.
type QuotaTracker struct {
	mu sync.Mutex

	// Per-context counters
	ctxMemory   map[string]int64 // bytes allocated per context
	ctxRequests map[string]int   // request count per context
	ctxScreenshots map[string]int // screenshot count per context
	ctxNavigations map[string]int // navigation count per context

	// Limits
	maxMemoryPerContext    int64
	maxRequestsPerContext  int
	maxScreenshotsPerContext int
	maxNavigationsPerContext int

	// Global counters
	totalContextsCreated atomic.Int64
}

// QuotaLimits configures the quota tracker.
type QuotaLimits struct {
	MaxMemoryPerContext      int64 // bytes
	MaxRequestsPerContext    int
	MaxScreenshotsPerContext int
	MaxNavigationsPerContext int
}

// DefaultQuotaLimits returns sensible defaults.
func DefaultQuotaLimits() QuotaLimits {
	return QuotaLimits{
		MaxMemoryPerContext:      100 * 1024 * 1024, // 100MB
		MaxRequestsPerContext:    10000,
		MaxScreenshotsPerContext: 100,
		MaxNavigationsPerContext: 1000,
	}
}

// QuotaUsage reports current quota usage for a context.
type QuotaUsage struct {
	MemoryBytes    int64 `json:"memoryBytes"`
	RequestCount   int   `json:"requestCount"`
	ScreenshotCount int  `json:"screenshotCount"`
	NavigationCount int  `json:"navigationCount"`
}

// NewQuotaTracker creates a new quota tracker.
func NewQuotaTracker(limits QuotaLimits) *QuotaTracker {
	return &QuotaTracker{
		ctxMemory:        make(map[string]int64),
		ctxRequests:      make(map[string]int),
		ctxScreenshots:   make(map[string]int),
		ctxNavigations:   make(map[string]int),
		maxMemoryPerContext:      limits.MaxMemoryPerContext,
		maxRequestsPerContext:    limits.MaxRequestsPerContext,
		maxScreenshotsPerContext: limits.MaxScreenshotsPerContext,
		maxNavigationsPerContext: limits.MaxNavigationsPerContext,
	}
}

// CheckRequest checks if a request can proceed for the given context.
func (q *QuotaTracker) CheckRequest(ctxID string) (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.maxRequestsPerContext > 0 {
		count := q.ctxRequests[ctxID]
		if count >= q.maxRequestsPerContext {
			return false, "request quota exceeded for context"
		}
	}
	return true, ""
}

// RecordRequest increments the request counter for a context.
func (q *QuotaTracker) RecordRequest(ctxID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ctxRequests[ctxID]++
}

// CheckNavigation checks if navigation can proceed for the given context.
func (q *QuotaTracker) CheckNavigation(ctxID string) (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.maxNavigationsPerContext > 0 {
		count := q.ctxNavigations[ctxID]
		if count >= q.maxNavigationsPerContext {
			return false, "navigation quota exceeded for context"
		}
	}
	return true, ""
}

// RecordNavigation increments the navigation counter.
func (q *QuotaTracker) RecordNavigation(ctxID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ctxNavigations[ctxID]++
}

// CheckScreenshot checks if a screenshot can be captured.
func (q *QuotaTracker) CheckScreenshot(ctxID string) (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.maxScreenshotsPerContext > 0 {
		count := q.ctxScreenshots[ctxID]
		if count >= q.maxScreenshotsPerContext {
			return false, "screenshot quota exceeded for context"
		}
	}
	return true, ""
}

// RecordScreenshot increments the screenshot counter.
func (q *QuotaTracker) RecordScreenshot(ctxID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ctxScreenshots[ctxID]++
}

// AddMemory records memory allocation for a context.
func (q *QuotaTracker) AddMemory(ctxID string, bytes int64) (bool, string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.ctxMemory[ctxID] + bytes
	if q.maxMemoryPerContext > 0 && current > q.maxMemoryPerContext {
		return false, "memory quota exceeded for context"
	}
	q.ctxMemory[ctxID] = current
	return true, ""
}

// ReleaseContext frees all quota tracking for a closed context.
func (q *QuotaTracker) ReleaseContext(ctxID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.ctxMemory, ctxID)
	delete(q.ctxRequests, ctxID)
	delete(q.ctxScreenshots, ctxID)
	delete(q.ctxNavigations, ctxID)
}

// Usage returns the current usage for a context.
func (q *QuotaTracker) Usage(ctxID string) QuotaUsage {
	q.mu.Lock()
	defer q.mu.Unlock()
	return QuotaUsage{
		MemoryBytes:     q.ctxMemory[ctxID],
		RequestCount:    q.ctxRequests[ctxID],
		ScreenshotCount: q.ctxScreenshots[ctxID],
		NavigationCount: q.ctxNavigations[ctxID],
	}
}

// TotalContextsCreated returns the total number of contexts ever created.
func (q *QuotaTracker) TotalContextsCreated() int64 {
	return q.totalContextsCreated.Load()
}

// RecordContextCreated increments the total context creation counter.
func (q *QuotaTracker) RecordContextCreated() {
	q.totalContextsCreated.Add(1)
}

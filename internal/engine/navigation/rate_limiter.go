package navigation

import (
	"container/heap"
	"context"
	"sync"
)

// originState tracks the number of active requests for a single origin.
type originState struct {
	active int
}

// waiter represents a blocked Acquire call waiting for admission.
type waiter struct {
	priority Priority
	origin   string
	ch       chan struct{} // buffered(1); signalled when admitted
	index    int           // position in the heap
}

// waitQueue is a min-heap of waiters ordered by priority (lower = higher priority).
type waitQueue []*waiter

func (q waitQueue) Len() int            { return len(q) }
func (q waitQueue) Less(i, j int) bool  { return q[i].priority < q[j].priority }
func (q waitQueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i]; q[i].index = i; q[j].index = j }
func (q *waitQueue) Push(x interface{}) { w := x.(*waiter); w.index = len(*q); *q = append(*q, w) }
func (q *waitQueue) Pop() interface{} {
	n := len(*q)
	w := (*q)[n-1]
	(*q)[n-1] = nil // avoid memory leak
	*q = (*q)[:n-1]
	w.index = -1
	return w
}

// RateLimiter bounds concurrent in-flight requests per origin and globally.
// A zero-value RateLimiter is unlimited (Acquire never blocks).
type RateLimiter struct {
	maxPerOrigin int
	maxGlobal    int

	mu            sync.Mutex
	origins       map[string]*originState
	globalActive  int
	globalWaiters waitQueue
}

// NewRateLimiter creates a limiter with the given per-origin and global limits.
// A limit of zero means unlimited for that dimension.
func NewRateLimiter(maxPerOrigin, maxGlobal int) *RateLimiter {
	return &RateLimiter{
		maxPerOrigin: maxPerOrigin,
		maxGlobal:    maxGlobal,
		origins:      make(map[string]*originState),
	}
}

// canProceedLocked reports whether a request for origin can be admitted now.
// Caller must hold rl.mu.
func (rl *RateLimiter) canProceedLocked(origin string) bool {
	if rl.maxPerOrigin > 0 {
		if os, ok := rl.origins[origin]; ok && os.active >= rl.maxPerOrigin {
			return false
		}
	}
	if rl.maxGlobal > 0 && rl.globalActive >= rl.maxGlobal {
		return false
	}
	return true
}

// originCanProceedLocked checks only the per-origin constraint.
// Caller must hold rl.mu.
func (rl *RateLimiter) originCanProceedLocked(origin string) bool {
	if rl.maxPerOrigin > 0 {
		if os, ok := rl.origins[origin]; ok && os.active >= rl.maxPerOrigin {
			return false
		}
	}
	return true
}

// globalCanProceedLocked checks only the global constraint.
// Caller must hold rl.mu.
func (rl *RateLimiter) globalCanProceedLocked() bool {
	return rl.maxGlobal == 0 || rl.globalActive < rl.maxGlobal
}

// admitLocked increments per-origin and global counters for origin.
// Caller must hold rl.mu.
func (rl *RateLimiter) admitLocked(origin string) {
	if rl.origins == nil {
		rl.origins = make(map[string]*originState)
	}
	os, ok := rl.origins[origin]
	if !ok {
		os = &originState{}
		rl.origins[origin] = os
	}
	os.active++
	rl.globalActive++
}

// releaseLocked decrements per-origin and global counters for origin,
// removing the origin map entry when its count reaches zero.
// Caller must hold rl.mu.
func (rl *RateLimiter) releaseLocked(origin string) {
	if os, ok := rl.origins[origin]; ok {
		os.active--
		if os.active <= 0 {
			delete(rl.origins, origin)
		}
	}
	if rl.globalActive > 0 {
		rl.globalActive--
	}
}

// wakeWaitersLocked scans the heap and admits the highest-priority waiters
// that can proceed under both per-origin and global limits.
// Caller must hold rl.mu.
func (rl *RateLimiter) wakeWaitersLocked() {
	var skipped []*waiter
	for rl.globalWaiters.Len() > 0 && rl.globalCanProceedLocked() {
		w := heap.Pop(&rl.globalWaiters).(*waiter)
		if !rl.originCanProceedLocked(w.origin) {
			skipped = append(skipped, w)
			continue
		}
		rl.admitLocked(w.origin)
		// Signal the waiter (non-blocking send on buffered channel).
		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
	for _, w := range skipped {
		heap.Push(&rl.globalWaiters, w)
	}
}

// Acquire blocks until a slot is available for origin at the given priority,
// or until ctx is cancelled. Returns nil on success or ctx.Err() on cancellation.
// A zero-value RateLimiter (maxPerOrigin=0, maxGlobal=0) never blocks.
func (rl *RateLimiter) Acquire(ctx context.Context, origin string, prio Priority) error {
	if rl == nil || (rl.maxPerOrigin == 0 && rl.maxGlobal == 0) {
		return nil
	}

	rl.mu.Lock()
	if rl.canProceedLocked(origin) {
		rl.admitLocked(origin)
		rl.mu.Unlock()
		return nil
	}

	w := &waiter{
		priority: prio,
		origin:   origin,
		ch:       make(chan struct{}, 1),
	}
	heap.Push(&rl.globalWaiters, w)
	rl.mu.Unlock()

	select {
	case <-w.ch:
		// Admitted by Release.
		return nil
	case <-ctx.Done():
		rl.mu.Lock()
		// Check whether we were admitted concurrently (Release signalled ch).
		select {
		case <-w.ch:
			// Admitted but caller is cancelling — undo admission and wake next.
			rl.releaseLocked(origin)
			rl.wakeWaitersLocked()
			rl.mu.Unlock()
			return ctx.Err()
		default:
			// Not yet admitted — remove from heap if identity matches.
			if w.index >= 0 && w.index < len(rl.globalWaiters) && rl.globalWaiters[w.index] == w {
				heap.Remove(&rl.globalWaiters, w.index)
			}
			rl.mu.Unlock()
			return ctx.Err()
		}
	}
}

// Release frees one slot for origin and wakes the highest-priority waiter
// that can now proceed. It is safe to call for origins that were never acquired.
func (rl *RateLimiter) Release(origin string) {
	if rl == nil || (rl.maxPerOrigin == 0 && rl.maxGlobal == 0) {
		return
	}

	rl.mu.Lock()
	rl.releaseLocked(origin)
	rl.wakeWaitersLocked()
	rl.mu.Unlock()
}

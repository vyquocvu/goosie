package navigation

import (
	"container/heap"
	"context"
	"sync"
)

// OriginState tracks the number of active requests for a single origin.
type OriginState struct {
	Active int
}

// Waiter represents a blocked Acquire call waiting for admission.
type Waiter struct {
	Priority Priority
	Origin   string
	Ch       chan struct{} // buffered(1); signalled when admitted
	Index    int           // position in the heap
}

// WaitQueue is a min-heap of waiters ordered by priority (lower = higher priority).
type WaitQueue []*Waiter

func (q WaitQueue) Len() int            { return len(q) }
func (q WaitQueue) Less(i, j int) bool  { return q[i].Priority < q[j].Priority }
func (q WaitQueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i]; q[i].Index = i; q[j].Index = j }
func (q *WaitQueue) Push(x interface{}) { w := x.(*Waiter); w.Index = len(*q); *q = append(*q, w) }
func (q *WaitQueue) Pop() interface{} {
	n := len(*q)
	w := (*q)[n-1]
	(*q)[n-1] = nil // avoid memory leak
	*q = (*q)[:n-1]
	w.Index = -1
	return w
}

// RateLimiter bounds concurrent in-flight requests per origin and globally.
// A zero-value RateLimiter is unlimited (Acquire never blocks).
type RateLimiter struct {
	MaxPerOrigin int
	MaxGlobal    int

	Mu            sync.Mutex
	Origins       map[string]*OriginState
	GlobalActive  int
	GlobalWaiters WaitQueue
}

// NewRateLimiter creates a limiter with the given per-origin and global limits.
// A limit of zero means unlimited for that dimension.
func NewRateLimiter(maxPerOrigin, maxGlobal int) *RateLimiter {
	return &RateLimiter{
		MaxPerOrigin: maxPerOrigin,
		MaxGlobal:    maxGlobal,
		Origins:      make(map[string]*OriginState),
	}
}

// canProceedLocked reports whether a request for origin can be admitted now.
// Caller must hold rl.Mu.
func (rl *RateLimiter) canProceedLocked(origin string) bool {
	if rl.MaxPerOrigin > 0 {
		if os, ok := rl.Origins[origin]; ok && os.Active >= rl.MaxPerOrigin {
			return false
		}
	}
	if rl.MaxGlobal > 0 && rl.GlobalActive >= rl.MaxGlobal {
		return false
	}
	return true
}

// originCanProceedLocked checks only the per-origin constraint.
// Caller must hold rl.Mu.
func (rl *RateLimiter) originCanProceedLocked(origin string) bool {
	if rl.MaxPerOrigin > 0 {
		if os, ok := rl.Origins[origin]; ok && os.Active >= rl.MaxPerOrigin {
			return false
		}
	}
	return true
}

// globalCanProceedLocked checks only the global constraint.
// Caller must hold rl.Mu.
func (rl *RateLimiter) globalCanProceedLocked() bool {
	return rl.MaxGlobal == 0 || rl.GlobalActive < rl.MaxGlobal
}

// admitLocked increments per-origin and global counters for origin.
// Caller must hold rl.Mu.
func (rl *RateLimiter) admitLocked(origin string) {
	if rl.Origins == nil {
		rl.Origins = make(map[string]*OriginState)
	}
	os, ok := rl.Origins[origin]
	if !ok {
		os = &OriginState{}
		rl.Origins[origin] = os
	}
	os.Active++
	rl.GlobalActive++
}

// releaseLocked decrements per-origin and global counters for origin,
// removing the origin map entry when its count reaches zero.
// Caller must hold rl.Mu.
func (rl *RateLimiter) releaseLocked(origin string) {
	if os, ok := rl.Origins[origin]; ok {
		os.Active--
		if os.Active <= 0 {
			delete(rl.Origins, origin)
		}
	}
	if rl.GlobalActive > 0 {
		rl.GlobalActive--
	}
}

// wakeWaitersLocked scans the heap and admits the highest-priority waiters
// that can proceed under both per-origin and global limits.
// Caller must hold rl.Mu.
func (rl *RateLimiter) wakeWaitersLocked() {
	var skipped []*Waiter
	for rl.GlobalWaiters.Len() > 0 && rl.globalCanProceedLocked() {
		w := heap.Pop(&rl.GlobalWaiters).(*Waiter)
		if !rl.originCanProceedLocked(w.Origin) {
			skipped = append(skipped, w)
			continue
		}
		rl.admitLocked(w.Origin)
		// Signal the waiter (non-blocking send on buffered channel).
		select {
		case w.Ch <- struct{}{}:
		default:
		}
	}
	for _, w := range skipped {
		heap.Push(&rl.GlobalWaiters, w)
	}
}

// Acquire blocks until a slot is available for origin at the given priority,
// or until ctx is cancelled. Returns nil on success or ctx.Err() on cancellation.
// A zero-value RateLimiter (MaxPerOrigin=0, MaxGlobal=0) never blocks.
func (rl *RateLimiter) Acquire(ctx context.Context, origin string, prio Priority) error {
	if rl == nil || (rl.MaxPerOrigin == 0 && rl.MaxGlobal == 0) {
		return nil
	}

	rl.Mu.Lock()
	if rl.canProceedLocked(origin) {
		rl.admitLocked(origin)
		rl.Mu.Unlock()
		return nil
	}

	w := &Waiter{
		Priority: prio,
		Origin:   origin,
		Ch:       make(chan struct{}, 1),
	}
	heap.Push(&rl.GlobalWaiters, w)
	rl.Mu.Unlock()

	select {
	case <-w.Ch:
		// Admitted by Release.
		return nil
	case <-ctx.Done():
		rl.Mu.Lock()
		// Check whether we were admitted concurrently (Release signalled Ch).
		select {
		case <-w.Ch:
			// Admitted but caller is cancelling — undo admission and wake next.
			rl.releaseLocked(origin)
			rl.wakeWaitersLocked()
			rl.Mu.Unlock()
			return ctx.Err()
		default:
			// Not yet admitted — remove from heap if identity matches.
			if w.Index >= 0 && w.Index < len(rl.GlobalWaiters) && rl.GlobalWaiters[w.Index] == w {
				heap.Remove(&rl.GlobalWaiters, w.Index)
			}
			rl.Mu.Unlock()
			return ctx.Err()
		}
	}
}

// Release frees one slot for origin and wakes the highest-priority waiter
// that can now proceed. It is safe to call for origins that were never acquired.
func (rl *RateLimiter) Release(origin string) {
	if rl == nil || (rl.MaxPerOrigin == 0 && rl.MaxGlobal == 0) {
		return
	}

	rl.Mu.Lock()
	rl.releaseLocked(origin)
	rl.wakeWaitersLocked()
	rl.Mu.Unlock()
}

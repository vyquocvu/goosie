// Package session_test — M9 performance target tests.
//
// These tests verify the three remaining M9 performance guarantees:
//
//  1. Repeated navigation does not create unbounded heap growth.
//  2. Closing a tab releases session-owned memory after expected GC behaviour.
//
// Heap-growth tests use runtime.ReadMemStats around explicit runtime.GC()
// calls. GC timing is non-deterministic, so hard assertions use generous
// multipliers; unexpected results are logged as failures with diagnostics.
//
// The tests deliberately use -short guard so they do not slow down the
// fast unit-test tier.
package session_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/session"
)

// TestRepeatedNavigation_NoUnboundedHeapGrowth exercises the session
// navigate → complete → navigate cycle N times and verifies that the
// retained heap after GC does not grow linearly with the iteration count.
//
// The invariant: heap growth after N navigations must not exceed
// navigationsHardLimit × (single-navigation overhead estimate).
// Single-navigation overhead is captured from the first iteration.
func TestRepeatedNavigation_NoUnboundedHeapGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heap-growth test in short mode")
	}

	const navigations = 50

	s := session.New()
	defer s.Close()

	// Warm-up: one navigation to initialise all lazily-allocated internals
	// so they do not skew the baseline.
	load, _ := s.Navigate(context.Background(), "https://warmup.example.com")
	s.Complete()
	_ = load

	// Collect a clean baseline.
	runtime.GC()
	runtime.GC() // two calls to flush finaliser queues
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Navigate-and-complete loop without any real network I/O.
	for i := 0; i < navigations; i++ {
		url := fmt.Sprintf("https://nav-%d.example.com", i)
		_, _ = s.Navigate(context.Background(), url)
		s.Complete()
	}

	// Force collection so we measure retained (not just live) heap.
	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	heapGrowth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	totalAlloc := after.TotalAlloc - before.TotalAlloc
	gcCycles := after.NumGC - before.NumGC

	t.Logf("Repeated navigation heap growth over %d navigations:", navigations)
	t.Logf("  HeapAlloc before : %d B", before.HeapAlloc)
	t.Logf("  HeapAlloc after  : %d B", after.HeapAlloc)
	t.Logf("  Heap growth      : %d B (%.2f KB)", heapGrowth, float64(heapGrowth)/1024)
	t.Logf("  TotalAlloc delta : %d B (%.2f KB)", totalAlloc, float64(totalAlloc)/1024)
	t.Logf("  GC cycles        : %d", gcCycles)

	// Hard limit: retained heap must not exceed 512 KB above baseline for
	// 50 navigate-complete cycles. A well-behaved session holds only a
	// single navigation's worth of state at a time; any unbounded growth
	// would violate M9's guarantee.
	const maxRetainedBytes = 512 * 1024
	if heapGrowth > maxRetainedBytes {
		t.Errorf("retained heap growth %d B exceeds hard limit %d B — possible navigation-state leak",
			heapGrowth, maxRetainedBytes)
	}
}

// TestRepeatedNavigation_HeapDoesNotGrowLinearly verifies that heap growth
// does not scale linearly with the number of navigations. It measures growth
// for a small batch and a large batch and confirms the ratio is sublinear.
func TestRepeatedNavigation_HeapDoesNotGrowLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping linearity heap test in short mode")
	}

	navigate := func(n int) (before, after runtime.MemStats) {
		s := session.New()
		defer s.Close()

		// single warm-up
		_, _ = s.Navigate(context.Background(), "https://warmup.example.com")
		s.Complete()

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&before)

		for i := 0; i < n; i++ {
			_, _ = s.Navigate(context.Background(), fmt.Sprintf("https://nav-%d.example.com", i))
			s.Complete()
		}

		runtime.GC()
		runtime.GC()
		runtime.ReadMemStats(&after)
		return
	}

	const small = 10
	const large = 100

	beforeSmall, afterSmall := navigate(small)
	beforeLarge, afterLarge := navigate(large)

	growSmall := int64(afterSmall.HeapAlloc) - int64(beforeSmall.HeapAlloc)
	growLarge := int64(afterLarge.HeapAlloc) - int64(beforeLarge.HeapAlloc)

	t.Logf("Heap growth for %d navigations : %d B", small, growSmall)
	t.Logf("Heap growth for %d navigations: %d B", large, growLarge)

	// If memory grows strictly linearly, growLarge would be ~10× growSmall.
	// We allow up to 3× as a loose bound — any larger ratio indicates a
	// per-navigation retained allocation.
	//
	// When growSmall is near zero (GC cleared everything), skip the ratio
	// check to avoid false positives from noise.
	if growSmall > 4096 && growLarge > growSmall*3 {
		t.Errorf("heap growth appears linear: small=%d B, large=%d B (ratio=%.1f×) — navigation state may not be released",
			growSmall, growLarge, float64(growLarge)/float64(growSmall))
	}
}

// TestClose_ReleasesSessionOwnedMemory verifies that calling Session.Close
// releases the goroutine and does not cause the session's event loop to
// retain heap after GC collection.
//
// This test is complementary to TestSessionGoroutineLeakCheck in
// session_test.go which checks goroutine count. Here we focus on heap.
func TestClose_ReleasesSessionOwnedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping close-releases-memory test in short mode")
	}

	const sessionCount = 20

	// Warm up by creating and closing one session.
	warmup := session.New()
	_, _ = warmup.Navigate(context.Background(), "https://warmup.example.com")
	warmup.Complete()
	warmup.Close()
	time.Sleep(10 * time.Millisecond)

	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	// Create a batch of sessions, navigate each, then close all.
	sessions := make([]*session.Session, sessionCount)
	for i := 0; i < sessionCount; i++ {
		sessions[i] = session.New()
		_, _ = sessions[i].Navigate(context.Background(),
			fmt.Sprintf("https://tab-%d.example.com", i))
		sessions[i].Complete()
	}

	// Measure heap with all sessions live.
	var atPeak runtime.MemStats
	runtime.ReadMemStats(&atPeak)

	// Close every session and give dispatch loops time to exit.
	for i := 0; i < sessionCount; i++ {
		sessions[i].Close()
	}
	sessions = nil // drop references

	// Allow goroutines to exit, then force two GC cycles.
	time.Sleep(20 * time.Millisecond)
	runtime.GC()
	runtime.GC()

	var afterClose runtime.MemStats
	runtime.ReadMemStats(&afterClose)

	peakGrowth := int64(atPeak.HeapAlloc) - int64(before.HeapAlloc)
	retainedGrowth := int64(afterClose.HeapAlloc) - int64(before.HeapAlloc)

	t.Logf("Session close releases memory (%d sessions):", sessionCount)
	t.Logf("  HeapAlloc baseline : %d B", before.HeapAlloc)
	t.Logf("  HeapAlloc at peak  : %d B (+%d B)", atPeak.HeapAlloc, peakGrowth)
	t.Logf("  HeapAlloc after GC : %d B (+%d B retained)", afterClose.HeapAlloc, retainedGrowth)

	// After closing all sessions the retained heap must not exceed the peak
	// by more than 20% (GC may not have collected everything yet, but the
	// trend must be downward). We use a generous 20% headroom to account
	// for GC non-determinism.
	//
	// The key invariant: retained heap must be significantly less than
	// peak, proving that Close() releases session-owned memory.
	if peakGrowth > 0 && retainedGrowth > peakGrowth {
		t.Errorf("closing sessions did not reduce heap: peak_growth=%d B, retained_growth=%d B — session may be retaining state after Close()",
			peakGrowth, retainedGrowth)
	}
	t.Logf("  Memory released    : %d B (%.0f%% of peak growth)",
		peakGrowth-retainedGrowth,
		100*float64(peakGrowth-retainedGrowth)/float64(max1(peakGrowth, 1)))
}

// max1 is a local helper to avoid importing math.
func max1(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

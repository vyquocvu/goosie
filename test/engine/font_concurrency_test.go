package engine_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/raster"
)

// TestLoadFontsFetchesConcurrently guards the @font-face fetch path. Real
// pages declare several families and weights; fetched one after another the
// per-fetch latency multiplies into the whole wall-clock budget (iana.org
// measured 6 fonts × ~250ms = 1.5s of blank screen). A slow fetcher that
// records its own peak in-flight count proves the loads overlap.
func TestLoadFontsFetchesConcurrently(t *testing.T) {
	const (
		count      = 8
		perFetch   = 100 * time.Millisecond
		minOverlap = 4 // a serial walk would never exceed 1
	)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	var inFlight, peak, fetched int64
	var css strings.Builder
	fetcher := func(base, url string) ([]byte, error) {
		cur := atomic.AddInt64(&inFlight, 1)
		for {
			old := atomic.LoadInt64(&peak)
			if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
				break
			}
		}
		time.Sleep(perFetch)
		atomic.AddInt64(&inFlight, -1)
		atomic.AddInt64(&fetched, 1)
		// Garbage bytes: the fetch is what is timed; registration fails and
		// the loop moves on, exactly as a corrupt download would.
		return []byte{1, 2, 3}, nil
	}
	for i := 0; i < count; i++ {
		fmt.Fprintf(&css, "@font-face { font-family: F%d; src: url(\"/f%d.woff\"); }\n", i, i)
	}

	start := time.Now()
	if _, err := engine.NewSession("<html><body>hi</body></html>", []string{css.String()}, 800,
		engine.WithMetrics(fonts),
		engine.WithCustomFontLoading("https://example.com/", fetcher, fonts)); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if got := atomic.LoadInt64(&fetched); got != int64(count) {
		t.Fatalf("fetched %d fonts, want %d", got, count)
	}
	if p := atomic.LoadInt64(&peak); p < minOverlap {
		t.Fatalf("peak concurrent fetches = %d, want >= %d (loads are serialising)", p, minOverlap)
	}
	// Serialising 8 fetches at 100ms each needs ~0.8s; the pool must finish
	// well inside that bound.
	if serial := time.Duration(count) * perFetch; elapsed >= serial {
		t.Fatalf("elapsed %v >= serial bound %v: fetches did not overlap", elapsed, serial)
	}
}

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

func TestLoadFontsFetchesConcurrently(t *testing.T) {
	const (
		count      = 8
		perFetch   = 100 * time.Millisecond
		minOverlap = 4
	)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	var inFlight, peak, fetched int64
	var css strings.Builder
	fetcher := func(base, url string) (engine.FontResult, error) {
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
		return engine.FontResult{Data: []byte{1, 2, 3}}, nil
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
	if serial := time.Duration(count) * perFetch; elapsed >= serial {
		t.Fatalf("elapsed %v >= serial bound %v: fetches did not overlap", elapsed, serial)
	}
}

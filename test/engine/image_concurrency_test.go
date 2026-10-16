package engine_test

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestLoadImagesFetchesConcurrently guards the image-fetch worker pool. A page
// can carry dozens of images; when they are fetched one after another the fixed
// wall-clock budget is spent on the first handful and the rest paint nothing.
// A slow fetcher that records its own peak in-flight count proves the loads
// overlap instead of serialising.
func TestLoadImagesFetchesConcurrently(t *testing.T) {
	const (
		count      = 24
		perFetch   = 80 * time.Millisecond
		minOverlap = 4 // a serial walk would never exceed 1
	)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	var inFlight, peak, fetched int64
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
		return pngBytes, nil
	}

	var sb []byte
	sb = append(sb, []byte("<html><body>")...)
	for i := 0; i < count; i++ {
		sb = append(sb, []byte(fmt.Sprintf(`<img src="/i%d.png">`, i))...)
	}
	sb = append(sb, []byte("</body></html>")...)

	start := time.Now()
	if _, err := engine.NewSession(string(sb), nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher)); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if got := atomic.LoadInt64(&fetched); got != int64(count) {
		t.Fatalf("fetched %d images, want %d", got, count)
	}
	if p := atomic.LoadInt64(&peak); p < minOverlap {
		t.Fatalf("peak concurrent fetches = %d, want >= %d (loads are serialising)", p, minOverlap)
	}
	// Serialising 24 fetches at 80ms each needs ~1.9s; the pool should finish
	// well inside that, so a large elapsed time means the pool is not working.
	if serial := time.Duration(count) * perFetch; elapsed >= serial {
		t.Fatalf("elapsed %v >= serial bound %v: fetches did not overlap", elapsed, serial)
	}
}

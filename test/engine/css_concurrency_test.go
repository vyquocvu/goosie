package engine_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestCheckedSheetsFetchConcurrently guards the linked-stylesheet fetch path.
// A page can link a dozen stylesheets; fetched one after another with no
// deadline, one slow host stalls the whole document behind the client's 30s
// timeout. A slow fetcher recording peak in-flight count proves the loads
// overlap.
func TestCheckedSheetsFetchConcurrently(t *testing.T) {
	const (
		count      = 8
		perFetch   = 100 * time.Millisecond
		minOverlap = 4
	)
	var inFlight, peak, fetched int64
	linker := func(base, href string) (string, error) {
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
		return fmt.Sprintf(".c%d { color: rgb(%d, 0, 0); }", 0, 0), nil
	}
	var head, body strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&head, `<link rel="stylesheet" href="/s%d.css">`, i)
		fmt.Fprintf(&body, `<p class="c%d">x</p>`, i)
	}

	start := time.Now()
	if _, err := engine.NewSession("<html><head>"+head.String()+"</head><body>"+body.String()+"</body></html>", nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithLinkedCSS("https://example.com/", linker)); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if got := atomic.LoadInt64(&fetched); got != int64(count) {
		t.Fatalf("fetched %d sheets, want %d", got, count)
	}
	if p := atomic.LoadInt64(&peak); p < minOverlap {
		t.Fatalf("peak concurrent fetches = %d, want >= %d (fetches are serialising)", p, minOverlap)
	}
	if serial := time.Duration(count) * perFetch; elapsed >= serial {
		t.Fatalf("elapsed %v >= serial bound %v: fetches did not overlap", elapsed, serial)
	}
}

// TestCheckedSheetsCascadeOrderPreserved pins that parallel fetching does not
// shuffle the cascade: with two sheets setting the same property, the one
// linked later in the document must win.
func TestCheckedSheetsCascadeOrderPreserved(t *testing.T) {
	linker := func(base, href string) (string, error) {
		if strings.HasSuffix(href, "/a.css") {
			return "p { color: #010000; }", nil
		}
		return "p { color: #020000; }", nil
	}
	sess, err := engine.NewSession(`<html><head><link rel="stylesheet" href="/a.css"><link rel="stylesheet" href="/b.css"></head><body><p>x</p></body></html>`, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithLinkedCSS("https://example.com/", linker))
	if err != nil {
		t.Fatal(err)
	}
	ps := sess.Doc.ElementsByTagName("p")
	if len(ps) != 1 {
		t.Fatalf("found %d p nodes, want 1", len(ps))
	}
	st, ok := sess.Styles[ps[0].ID]
	if !ok {
		t.Fatal("p has no resolved style")
	}
	if st.Color.R != 2 || st.Color.G != 0 {
		t.Fatalf("p color = %v, want the later sheet (2,0,0) to win", st.Color)
	}
}

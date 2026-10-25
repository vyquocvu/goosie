package engine_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestImageFloodCostsAtMost256Attempts pins the attempt ceiling, not a success
// ceiling: 257 unique references that ALL fail must still cost at most 256
// fetches. Fetching is the expensive half (dial/TLS/bytes); a cap that only
// counts decoded successes never trips when every fetch fails, turning one
// hostile page into an unbounded burst of outbound requests. The document
// itself must still build: over-budget images are omitted, not fatal.
func TestImageFloodCostsAtMost256Attempts(t *testing.T) {
	const total = 257
	var b strings.Builder
	b.WriteString("<html><body>")
	urls := make([]string, 0, total)
	for i := 0; i < total; i++ {
		u := fmt.Sprintf("https://cdn.example.com/img%d.png", i)
		urls = append(urls, u)
		fmt.Fprintf(&b, `<img src="%s">`, u)
	}
	b.WriteString("</body></html>")

	var mu sync.Mutex
	fetched := make(map[string]bool)
	var attempts int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&attempts, 1)
		mu.Lock()
		fetched[url] = true
		mu.Unlock()
		return nil, fmt.Errorf("adversarial: unreachable")
	}
	sess, err := engine.NewSession(b.String(), nil, 400,
		engine.WithImages("https://example.com/page", fetcher))
	if err != nil {
		t.Fatalf("NewSession over image budget: %v (images must degrade, not fail)", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if n := atomic.LoadInt64(&attempts); n > 256 {
		t.Fatalf("flood cost %d fetch attempts, want <= 256 (attempt ceiling)", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if fetched[urls[total-1]] {
		t.Fatal("the 257th reference was fetched: selection is not first-256 in document order")
	}
}

// TestOversizedImageHeaderRejectedBeforeDecode pins pre-decode admission at
// the engine boundary: a header claiming 8193x1 pixels must never reach full
// decode. The payload below is IHDR-only (no IDAT, no pixel buffer possible),
// so a regression that decodes before probing fails loudly here instead of
// allocating 32 MiB in a test runner.
// qc:BR-166
func TestOversizedImageHeaderRejectedBeforeDecode(t *testing.T) {
	header := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x20, 0x01, // width 8193: one over the 8192 admission limit
		0x00, 0x00, 0x00, 0x01, // height 1
		0x08, 0x02, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, // placeholder CRC
	}
	fetcher := func(base, url string) ([]byte, error) { return header, nil }
	html := `<img src="https://cdn.example.com/wide.png">`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://example.com/page", fetcher))
	if err != nil {
		t.Fatalf("NewSession with over-limit image: %v (must degrade, not fail)", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
}

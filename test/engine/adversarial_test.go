package engine_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
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
// the engine boundary: a valid PNG claiming 8193x1 pixels must never reach
// full decode, while a valid 1x1 PNG must be attached to its layout object.
// qc:BR-166
func TestOversizedImageHeaderRejectedBeforeDecode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		width     int
		wantImage bool
	}{
		{name: "within admission limit", width: 1, wantImage: true},
		{name: "over dimension limit", width: 8193, wantImage: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded := encodePNG(t, tc.width, 1)
			fetcher := func(base, url string) ([]byte, error) { return encoded, nil }
			html := `<img src="https://cdn.example.com/wide.png">`
			sess, err := engine.NewSession(html, nil, 400,
				engine.WithImages("https://example.com/page", fetcher))
			if err != nil {
				t.Fatalf("NewSession with image %dx1: %v (image failures must degrade, not fail)", tc.width, err)
			}
			if sess == nil {
				t.Fatal("session is nil")
			}
			defer sess.Close()

			img := imageObject(sess)
			if img == nil {
				t.Fatal("image element has no layout object")
			}
			if got := img.Image != nil; got != tc.wantImage {
				t.Fatalf("decoded image attached = %t, want %t", got, tc.wantImage)
			}
		})
	}
}

func encodePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode %dx%d PNG: %v", width, height, err)
	}
	return encoded.Bytes()
}

func imageObject(sess *engine.Session) *layout.Object {
	if sess == nil || sess.Arena == nil {
		return nil
	}
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == "img" {
			return obj
		}
	}
	return nil
}

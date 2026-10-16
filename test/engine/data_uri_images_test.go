package engine_test

import (
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestDataURIImagesDecodeWithoutFetcher guards document-inline images: data:
// payloads decode locally under the byte cap, so sessions without a network
// fetcher (headless, tests) still render them. Before, data: srcs never
// became refs at all and every such image collapsed to a zero box - which is
// what blanked the aside-figure parity fixture's placeholders.
func TestDataURIImagesDecodeWithoutFetcher(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString(pngOf(t, 40, 30))
	svgPct := url.PathEscape(`<svg xmlns="http://www.w3.org/2000/svg" width="60" height="20"><rect fill="red" width="60" height="20"/></svg>`)
	html := `<html><body>` +
		`<img id="pix" src="data:image/png;base64,` + pngB64 + `">` +
		`<img id="vec" src="data:image/svg+xml,` + svgPct + `">` +
		`<img id="txt" src="data:text/plain,hello">` +
		`</body></html>`
	// No WithImages: there is deliberately no fetcher to call.
	s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatal(err)
	}
	box := func(id string) (float32, float32) {
		t.Helper()
		n := s.Doc.ElementByID(id)
		if n == nil {
			t.Fatalf("img %q not parsed", id)
		}
		oid, ok := s.Arena.ForNode(n.ID)
		if !ok {
			t.Fatalf("img %q not laid out", id)
		}
		b := s.Arena.Get(oid)
		return b.W, b.H
	}
	if w, h := box("pix"); w != 40 || h != 30 {
		t.Errorf("png box = %vx%v, want 40x30 (intrinsic, no fetcher)", w, h)
	}
	if w, h := box("vec"); w != 60 || h != 20 {
		t.Errorf("svg box = %vx%v, want 60x20 (intrinsic, no fetcher)", w, h)
	}
	if w, h := box("txt"); w != 0 || h != 0 {
		t.Errorf("text box = %vx%v, want 0x0 (not an image)", w, h)
	}
}

// TestResponsiveImageKeepsAspectRatio guards imgContentSize: with a specified
// width and `height: auto` the natural ratio derives the height. Taking the
// natural height as specified instead pinned every responsive image to it
// and shoved the page below down.
func TestResponsiveImageKeepsAspectRatio(t *testing.T) {
	svgPct := url.PathEscape(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400"><rect fill="red" width="600" height="400"/></svg>`)
	html := `<html><body style="margin: 0;"><div style="width: 505px;">` +
		`<img id="pic" style="width: 100%; height: auto;" src="data:image/svg+xml,` + svgPct + `">` +
		`</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatal(err)
	}
	n := s.Doc.ElementByID("pic")
	oid, ok := s.Arena.ForNode(n.ID)
	if !ok {
		t.Fatal("img not laid out")
	}
	b := s.Arena.Get(oid)
	if b.W != 505 {
		t.Errorf("img.W = %v, want 505 (100%% of 505)", b.W)
	}
	// 505 * 400/600 = 336.67.
	if b.H < 336 || b.H > 337.5 {
		t.Errorf("img.H = %v, want ~336.67 (aspect from width)", b.H)
	}
}

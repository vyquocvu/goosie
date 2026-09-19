package engine_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestResponsiveImageClampsToContainer guards the `img { max-width: 100% }`
// sizing path. A picture wider than its column must shrink to fit and keep its
// aspect ratio; without the clamp the box keeps its intrinsic width and
// overflows into its neighbours.
func TestResponsiveImageClampsToContainer(t *testing.T) {
	// A 640x292 intrinsic image inside a 200px column: the used width must be
	// 200 and the height 200 * 292/640 = 91.25.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 640, 292))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()
	fetcher := func(base, url string) ([]byte, error) { return pngBytes, nil }

	html := `<html><head><style>
		.box { width: 200px; }
		img { max-width: 100%; display: block; }
	</style></head><body><div class="box"><img id="big" src="/big.png"></div></body></html>`

	s, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}

	img := s.Doc.ElementByID("big")
	if img == nil {
		t.Fatal("img element missing")
	}
	id, ok := s.Arena.ForNode(img.ID)
	if !ok {
		t.Fatal("img not laid out")
	}
	got := s.Arena.Get(id)
	if diff := got.W - 200; diff > 0.5 || diff < -0.5 {
		t.Fatalf("clamped width = %v, want 200 (image overflowed its column)", got.W)
	}
	wantH := float32(200.0 * 292.0 / 640.0)
	if diff := got.H - wantH; diff > 0.5 || diff < -0.5 {
		t.Fatalf("aspect height = %v, want %v", got.H, wantH)
	}
}

package engine_test

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// TestObjectFitContainStaysInBox guards where a fitted image lands. `contain`
// shrinks the picture to fit the content box and centres the result *in that
// box*; the letterbox offsets are relative to the box, not to the page. Grid by
// Example sets `img{object-fit:contain}` on every example thumbnail, and adding
// the offsets to the page origin painted all eighteen of them stacked over the
// header while their cards stayed empty.
func TestObjectFitContainStaysInBox(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 640, 292))); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()
	fetcher := func(base, url string) ([]byte, error) { return pngBytes, nil }

	html := `<html><head><style>
		body { margin: 0; }
		.spacer { height: 600px; }
		img { width: 200px; height: 400px; object-fit: contain; display: block; }
	</style></head><body><div class="spacer"></div><img id="fit" src="/p.png"></body></html>`

	s, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	img := s.Doc.ElementByID("fit")
	if img == nil {
		t.Fatal("img element missing")
	}
	id, ok := s.Arena.ForNode(img.ID)
	if !ok {
		t.Fatal("img not laid out")
	}
	box := s.Arena.Get(id)
	bx0, by0, _, by1 := box.ContentRect()

	var cmd *paint.DisplayCmd
	all := s.Paint(1).All()
	for i := range all {
		if all[i].Kind == paint.CmdImage {
			cmd = &all[i]
		}
	}
	if cmd == nil {
		t.Fatal("no image command emitted")
	}
	// 640x292 contained into 200x400 keeps the width and centres vertically.
	wantW := float64(200)
	wantH := wantW * 292.0 / 640.0
	wantX0, wantY0 := float64(bx0), float64(by0)+(float64(by1-by0)-wantH)/2
	r := cmd.Rect
	if d := math.Abs(float64(r.X0) - wantX0); d > 1 {
		t.Errorf("image rect X0 = %v, want %v (within 1px): the fit offset was not added to the box", r.X0, wantX0)
	}
	if d := math.Abs(float64(r.Y0) - wantY0); d > 1 {
		t.Errorf("image rect Y0 = %v, want %v (within 1px): the fit offset was not added to the box", r.Y0, wantY0)
	}
	if d := math.Abs(float64(r.X1-r.X0) - wantW); d > 1 {
		t.Errorf("image width = %v, want %v (within 1px)", r.X1-r.X0, wantW)
	}
	if d := math.Abs(float64(r.Y1-r.Y0) - wantH); d > 1 {
		t.Errorf("image height = %v, want %v (within 1px)", r.Y1-r.Y0, wantH)
	}
}

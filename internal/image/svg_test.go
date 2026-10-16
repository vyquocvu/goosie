package image

import (
	"strings"
	"testing"
)

// TestSVGRectFill guards the static-SVG path end to end: dimensions resolve
// from width/height and a flat rect paints its color. This is the shape the
// aside-figure parity fixture's data-URI placeholders are built from.
func TestSVGRectFill(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="400"><rect fill="#3498db" width="600" height="400"/></svg>`)
	cfg, err := Probe(data)
	if err != nil {
		t.Fatalf("Probe(svg) = %v", err)
	}
	if cfg.Width != 600 || cfg.Height != 400 {
		t.Fatalf("Probe(svg) = %dx%d, want 600x400", cfg.Width, cfg.Height)
	}
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode(svg) = %v", err)
	}
	if b := img.Bounds(); b.Dx() != 600 || b.Dy() != 400 {
		t.Fatalf("Decode(svg) bounds = %v, want 600x400", b)
	}
	r, g, b, a := img.At(300, 200).RGBA()
	if r>>8 != 0x34 || g>>8 != 0x98 || b>>8 != 0xDB || a>>8 != 0xFF {
		t.Errorf("center = (%d,%d,%d,%d), want (52,152,219,255)", r>>8, g>>8, b>>8, a>>8)
	}
}

// TestSVGViewBoxScalesContent guards viewBox mapping: a 100-unit square in a
// 200-unit viewBox rasterized at 200x200 fills the whole output.
func TestSVGViewBoxScalesContent(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 100 100"><rect fill="red" width="100" height="100"/></svg>`)
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode(svg) = %v", err)
	}
	r, g, b, _ := img.At(199, 199).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("corner = (%d,%d,%d), want red (viewBox scaled up)", r>>8, g>>8, b>>8)
	}
}

// TestSVGShapesPaintInOrder guards circle+line+polygon coverage and
// painter's-order compositing: the last shape wins its pixels.
func TestSVGShapesPaintInOrder(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">` +
		`<rect fill="white" width="100" height="100"/>` +
		`<circle fill="red" cx="50" cy="50" r="40"/>` +
		`<rect fill="blue" x="50" y="50" width="50" height="50"/>` +
		`<line stroke="lime" stroke-width="10" x1="0" y1="0" x2="100" y2="0"/>` +
		`</svg>`)
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode(svg) = %v", err)
	}
	r, g, b, _ := img.At(50, 20).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("circle interior = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(75, 75).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 255 {
		t.Errorf("overlap = (%d,%d,%d), want blue on top", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(50, 2).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Errorf("top line = (%d,%d,%d), want lime", r>>8, g>>8, b>>8)
	}
}

// TestSVGSkipsUnsupportedSubtrees guards graceful degradation: text, paths,
// scripts and gradients never paint and never fail the document. Script in
// particular must be inert - there is no JS here by construction.
func TestSVGSkipsUnsupportedSubtrees(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="50" height="50">` +
		`<rect fill="white" width="50" height="50"/>` +
		`<script>alert(1)</script>` +
		`<defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs>` +
		`<rect fill="url(#g)" x="0" y="0" width="25" height="25"/>` +
		`<path d="M0,0 L50,50" fill="red"/>` +
		`<text x="0" y="10">hi</text>` +
		`</svg>`)
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode(svg with unsupported parts) = %v", err)
	}
	r, g, b, _ := img.At(10, 10).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("gradient rect = (%d,%d,%d), want the white underneath (url() unsupported)", r>>8, g>>8, b>>8)
	}
}

// TestSVGRefusesEntityAttackSurface guards the billion-laughs shape: any
// DOCTYPE or ENTITY declaration fails before parsing.
func TestSVGRefusesEntityAttackSurface(t *testing.T) {
	for _, doc := range []string{
		`<!DOCTYPE svg [<!ENTITY x "y">]><svg width="10" height="10"></svg>`,
		`<svg width="10" height="10"><!ENTITY x "y"></svg>`,
	} {
		if _, err := Probe([]byte(doc)); err == nil {
			t.Errorf("Probe accepted entity-bearing SVG: %q", doc)
		}
		if _, err := Decode([]byte(doc)); err == nil {
			t.Errorf("Decode accepted entity-bearing SVG: %q", doc)
		}
	}
}

// TestSVGElementCap guards decompression-style bombs: thousands of elements
// fail closed instead of rasterizing for minutes.
func TestSVGElementCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">`)
	for i := 0; i < maxSVGElements+10; i++ {
		sb.WriteString(`<rect width="1" height="1"/>`)
	}
	sb.WriteString(`</svg>`)
	if _, err := Decode([]byte(sb.String())); err == nil {
		t.Error("Decode accepted an SVG past the element cap")
	}
}

// TestNonSVGDataUntouched guards the sniff: PNG bytes never route to SVG.
func TestNonSVGDataUntouched(t *testing.T) {
	if isSVGData(smallPNG(t)) {
		t.Error("PNG bytes sniffed as SVG")
	}
	if isSVGData([]byte(`<html><body>hi</body></html>`)) {
		t.Error("HTML sniffed as SVG")
	}
	if !isSVGData([]byte("  <?xml version=\"1.0\"?><!-- c --><svg></svg>")) {
		t.Error("prolog/comment-prefixed SVG not recognized")
	}
}

package image

import (
	"testing"
)

func TestSVGIntrinsicKind(t *testing.T) {
	for _, c := range []struct {
		doc         string
		dims, ratio bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="20"></svg>`, true, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="20" viewBox="0 0 5 5"></svg>`, true, true},
		{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 5 5"></svg>`, false, true},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="50%" height="50%"></svg>`, false, false},
		{`<svg xmlns="http://www.w3.org/2000/svg"></svg>`, false, false},
		{`<html></html>`, false, false},
	} {
		dims, ratio := SVGIntrinsicKind([]byte(c.doc))
		if dims != c.dims || ratio != c.ratio {
			t.Errorf("SVGIntrinsicKind(%q) = (%v,%v), want (%v,%v)", c.doc, dims, ratio, c.dims, c.ratio)
		}
	}
}

func TestDecodeSVGAtPercentShapes(t *testing.T) {
	doc := `<svg xmlns="http://www.w3.org/2000/svg" height="8px" viewBox="0 0 1 2147483647" preserveAspectRatio="none"><rect y="0" width="100%" height="100%" fill="lime"/></svg>`
	if !SVGPreserveNone([]byte(doc)) {
		t.Fatal("preserveAspectRatio=none not detected")
	}
	img, err := DecodeSVGAt([]byte(doc), 258, 770)
	if err != nil {
		t.Fatalf("DecodeSVGAt: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 258 || b.Dy() != 770 {
		t.Fatalf("bounds = %v, want 258x770", b)
	}
	r, g, b, _ := img.At(128, 100).RGBA()
	if r>>8 != 0 || g>>8 != 255 || b>>8 != 0 {
		t.Errorf("interior = (%d,%d,%d), want lime", r>>8, g>>8, b>>8)
	}
}

func TestSVGZeroViewBoxFailsClosed(t *testing.T) {
	for _, vb := range []string{"0 0 0 8", "0 0 8 0", "0 0 -4 8", "0 0 0 0"} {
		doc := `<svg xmlns="http://www.w3.org/2000/svg" width="8px" viewBox="` + vb + `" preserveAspectRatio="none"><rect width="100%" height="100%" fill="lime"/></svg>`
		if _, err := Probe([]byte(doc)); err == nil {
			t.Errorf("Probe(viewBox %q) succeeded, want rejection", vb)
		}
	}
}

func TestSVGDefersBothSides(t *testing.T) {
	for _, c := range []struct {
		doc  string
		want bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg"></svg>`, true},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="50%" height="50%"></svg>`, true},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="8px"></svg>`, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" height="32px"></svg>`, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="8px" height="32px"></svg>`, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 64"></svg>`, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" width="50%" viewBox="0 0 4 64"></svg>`, false},
		{`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 8"></svg>`, true},
		{`<html></html>`, false},
	} {
		if got := SVGDefersBothSides([]byte(c.doc)); got != c.want {
			t.Errorf("SVGDefersBothSides(%q) = %v, want %v", c.doc, got, c.want)
		}
	}
}

func TestSVGRatio(t *testing.T) {
	rw, rh, ok := SVGRatio([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 64"></svg>`))
	if !ok || rw != 4 || rh != 64 {
		t.Errorf("SVGRatio(viewBox) = (%v,%v,%v), want (4,64,true)", rw, rh, ok)
	}
	if _, _, ok := SVGRatio([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 0 8"></svg>`)); ok {
		t.Error("SVGRatio(zero viewBox) ok, want false")
	}
	if _, _, ok := SVGRatio([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8px"></svg>`)); ok {
		t.Error("SVGRatio(no viewBox) ok, want false")
	}
	if _, _, ok := SVGRatio([]byte(`<html></html>`)); ok {
		t.Error("SVGRatio(non-svg) ok, want false")
	}
}

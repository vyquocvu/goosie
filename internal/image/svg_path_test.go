package image

import (
	"os"
	"testing"
)

// The menu icon (three bars) is pure paths: without <path> support every
// icon-shaped SVG decodes to a blank transparent tile, and masked UI icons
// vanish from pages.
func TestSVGPathFill(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><g fill="#000"><path d="M1 3h18v2H1zm0 6h18v2H1zm0 6h18v2H1z"/></g></svg>`)
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	opaque := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0x8000 {
				opaque++
			}
		}
	}
	// Three 18x2 bars = 108 opaque pixels (give or take AA).
	if opaque < 80 {
		t.Errorf("opaque pixels = %d, want >= 80", opaque)
	}
}

func TestSVGPathRelative(t *testing.T) {
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><path d="M1 1l8 0 0 8 -8 0z" fill="red"/></svg>`)
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	r, g, b, a := img.At(5, 5).RGBA()
	if a < 0x8000 || r < 0x8000 || g > 0x8000 || b > 0x8000 {
		t.Errorf("center = (%d,%d,%d,%d), want opaque red", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestSVGMenuIconFile(t *testing.T) {
	data, err := os.ReadFile("/tmp/dbg/menu.svg")
	if err != nil {
		t.Skip(err)
	}
	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	opaque := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0x8000 {
				opaque++
			}
		}
	}
	if opaque < 80 {
		t.Errorf("menu opaque pixels = %d, want >= 80", opaque)
	}
}

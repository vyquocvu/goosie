package tmpsvg

import (
	"os"
	"testing"

	"github.com/vyquocvu/goosie/internal/image"
)

func TestEnwikiDecode(t *testing.T) {
	data, err := os.ReadFile("/tmp/dbg/enwiki.svg")
	if err != nil {
		t.Skip(err)
	}
	img, err := image.Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := img.Bounds()
	t.Logf("bounds=%v", b)
	// Sample colors: puzzle logo should be colorful/gray, not solid black.
	black, total := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			r, g, bl, a := img.At(x, y).RGBA()
			total++
			if r < 0x1111 && g < 0x1111 && bl < 0x1111 && a > 0x8888 {
				black++
			}
		}
	}
	t.Logf("opaque-black=%d/%d (%.1f%%)", black, total, 100*float64(black)/float64(total))
}

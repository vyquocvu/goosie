package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

// TestBGTileVectorNoRatio pins tile negotiation for sources without an
// intrinsic ratio: cover and contain fill the positioning area, auto keeps
// the natural (fallback) size the decoder baked the mapping into, explicit
// lengths resolve per axis. The engine rasterizes these at the same tile,
// so both sides of the boundary must agree.
func TestBGTileVectorNoRatio(t *testing.T) {
	mk := func(size style.BgSize, w, h float32, wpct, hpct bool) *style.ComputedStyle {
		return &style.ComputedStyle{BackgroundSize: size, BgSizeW: w, BgSizeH: h, BgSizeWPct: wpct, BgSizeHPct: hpct}
	}
	for _, c := range []struct {
		name  string
		style *style.ComputedStyle
		natW  float32
		natH  float32
		tw    float32
		th    float32
	}{
		{"auto", mk(style.BgSizeAuto, 0, 0, false, false), 300, 150, 300, 150},
		{"cover", mk(style.BgSizeCover, 0, 0, false, false), 300, 150, 256, 768},
		{"contain", mk(style.BgSizeContain, 0, 0, false, false), 300, 150, 256, 768},
		{"length", mk(style.BgSizeLength, 100, 50, false, false), 300, 150, 100, 50},
		{"mixed", mk(style.BgSizeLength, 100, -1, false, false), 300, 150, 100, 768},
	} {
		tw, th := paint.BGTileSize(c.style, 256, 768, c.natW, c.natH, true)
		if tw != c.tw || th != c.th {
			t.Errorf("%s: tile = %vx%v, want %vx%v", c.name, tw, th, c.tw, c.th)
		}
	}
}

// TestBGTileRatioKept pins the ratio path: with a ratio, cover/contain
// scale by it, an auto width resolves from the declared height, and auto
// keeps nature. preserveAspectRatio=none never reaches here as noRatio:
// the viewBox ratio still sizes the tile.
func TestBGTileRatioKept(t *testing.T) {
	mk := func(size style.BgSize, w, h float32, wpct, hpct bool) *style.ComputedStyle {
		return &style.ComputedStyle{BackgroundSize: size, BgSizeW: w, BgSizeH: h, BgSizeWPct: wpct, BgSizeHPct: hpct}
	}
	tw, th := paint.BGTileSize(mk(style.BgSizeAuto, 0, 0, false, false), 256, 768, 100, 200, false)
	if tw != 100 || th != 200 {
		t.Errorf("auto = %vx%v, want 100x200", tw, th)
	}
	tw, th = paint.BGTileSize(mk(style.BgSizeCover, 0, 0, false, false), 256, 768, 100, 200, false)
	if tw != 384 || th != 768 {
		t.Errorf("cover = %vx%v, want 384x768", tw, th)
	}
	tw, th = paint.BGTileSize(mk(style.BgSizeLength, -1, 32, false, false), 256, 768, 8, 32, false)
	if tw != 8 || th != 32 {
		t.Errorf("auto 32px = %vx%v, want 8x32", tw, th)
	}
	tw, th = paint.BGTileSize(mk(style.BgSizeLength, -1, 32, false, false), 256, 768, 4, 64, false)
	if tw != 2 || th != 32 {
		t.Errorf("auto 32px ratio = %vx%v, want 2x32", tw, th)
	}
}

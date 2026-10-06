package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/style"
)

// TestBGTileVectorNoIntrinsic pins tile negotiation for vector images
// without intrinsic dimensions or ratio: automatic axes are the positioning
// area, explicit lengths resolve per axis. The engine rasterizes these at
// the same tile, so both sides of the boundary must agree.
func TestBGTileVectorNoIntrinsic(t *testing.T) {
	mk := func(size style.BgSize, w, h float32, wpct, hpct bool) *style.ComputedStyle {
		return &style.ComputedStyle{BackgroundSize: size, BgSizeW: w, BgSizeH: h, BgSizeWPct: wpct, BgSizeHPct: hpct}
	}
	for _, c := range []struct {
		name  string
		style *style.ComputedStyle
		tw    float32
		th    float32
	}{
		{"auto", mk(style.BgSizeAuto, 0, 0, false, false), 256, 768},
		{"cover", mk(style.BgSizeCover, 0, 0, false, false), 256, 768},
		{"contain", mk(style.BgSizeContain, 0, 0, false, false), 256, 768},
		{"length", mk(style.BgSizeLength, 100, 50, false, false), 100, 50},
		{"mixed", mk(style.BgSizeLength, 100, -1, false, false), 100, 768},
	} {
		tw, th := paint.BGTileSize(c.style, 256, 768, 0, 0, true, false)
		if tw != c.tw || th != c.th {
			t.Errorf("%s: tile = %vx%v, want %vx%v", c.name, tw, th, c.tw, c.th)
		}
	}
}

// TestBGTileRasterUnchanged pins the standard path the extraction must not
// alter: contain/cover scale by ratio, lengths resolve, auto keeps nature.
func TestBGTileRasterUnchanged(t *testing.T) {
	mk := func(size style.BgSize, w, h float32, wpct, hpct bool) *style.ComputedStyle {
		return &style.ComputedStyle{BackgroundSize: size, BgSizeW: w, BgSizeH: h, BgSizeWPct: wpct, BgSizeHPct: hpct}
	}
	tw, th := paint.BGTileSize(mk(style.BgSizeAuto, 0, 0, false, false), 256, 768, 100, 200, false, false)
	if tw != 100 || th != 200 {
		t.Errorf("auto = %vx%v, want 100x200", tw, th)
	}
	tw, th = paint.BGTileSize(mk(style.BgSizeCover, 0, 0, false, false), 256, 768, 100, 200, false, false)
	if tw != 384 || th != 768 {
		t.Errorf("cover = %vx%v, want 384x768", tw, th)
	}
}

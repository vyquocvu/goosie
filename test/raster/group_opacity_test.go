package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestGroupOpacityOverlapComposite(t *testing.T) {
	blue := frame.RGBA(0, 0, 255, 128)
	l := paint.NewList(3)
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: blue})
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: blue})
	l.AddGroup(0.5, 2)
	dl := l.Build(1).Publish()

	out := frame.NewBitmap(120, 80)
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 120, 80), out, nil, nil, frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(15, 40); !pixClose(got, 191, 191, 255, 0) {
		t.Errorf("non-overlap pixel = (%d,%d,%d), want (191,191,255)", got.R(), got.G(), got.B())
	}
	if got := out.At(50, 40); !pixClose(got, 158, 158, 255, 2) {
		t.Errorf("overlap pixel = (%d,%d,%d), want (158,158,255) +-\u22642 (the double-darkening defect sat near 143)", got.R(), got.G(), got.B())
	}
}

func pixClose(c frame.Color, r, g, b, tol int) bool {
	d := func(x, y int) int {
		if v := x - y; v < 0 {
			return -v
		} else {
			return v
		}
	}
	return d(int(c.R()), r) <= tol && d(int(c.G()), g) <= tol && d(int(c.B()), b) <= tol
}

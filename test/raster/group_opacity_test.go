package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// GOOSIE-COMPOSITE-001 measured oracle (Chromium, 300x120, DPR 1, white):
// a container opacity:0.5 holding two rgba(0,0,255,0.5) 60x60 boxes overlapping
// 30px. Non-overlap pixel = (191,191,255); overlap pixel = (158,158,255). Before
// offscreen compositing Goosie produced (143,143,255) in the overlap (each box
// pre-faded to 0.25, then 0.25-over-0.25 = 0.4375).
func TestGroupOpacityOverlapComposite(t *testing.T) {
	blue := frame.RGBA(0, 0, 255, 128) // rgba(0,0,255,0.5) premultiplied
	l := paint.NewList(3)
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: blue})
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: blue})
	l.AddGroup(0.5, 2)
	dl := l.Build(1).Publish()

	out := frame.NewBitmap(120, 80)
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 120, 80), out, nil, nil, frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	// Non-overlap: one box faded once, exact in our integer path and Chromium's.
	if got := out.At(15, 40); !pixClose(got, 191, 191, 255, 0) {
		t.Errorf("non-overlap pixel = (%d,%d,%d), want (191,191,255)", got.R(), got.G(), got.B())
	}
	// Overlap: two boxes composited inside the group, then the group faded once.
	// Chromium rounds this asymmetrically to (158,158,255); our premultiplied
	// integer path lands on 159. The tolerance is 2 so the oracle accepts the
	// rounding yet still rejects the pre-fix per-command double-darkening value
	// (~143), which is 15 away - the defect this closes.
	if got := out.At(50, 40); !pixClose(got, 158, 158, 255, 2) {
		t.Errorf("overlap pixel = (%d,%d,%d), want (158,158,255) +-\u22642 (the double-darkening defect sat near 143)", got.R(), got.G(), got.B())
	}
}

// pixClose reports whether c's RGB channels each sit within tol of (r,g,b).
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

package toolbar_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func badgeColor() frame.Color { return frame.RGB(232, 240, 254) }

func badgeInk(buf *frame.Bitmap) (int, frame.Rect) {
	bg := badgeColor()
	count := 0
	box := frame.Rect{}
	for y := 0; y < buf.H; y++ {
		for x := 0; x < buf.W; x++ {
			if buf.At(x, y) != bg {
				continue
			}
			if count == 0 {
				box = frame.Rect4(int32(x), int32(y), int32(x+1), int32(y+1))
			} else {
				if int32(x) < box.X0 {
					box.X0 = int32(x)
				}
				if int32(x+1) > box.X1 {
					box.X1 = int32(x + 1)
				}
				if int32(y) < box.Y0 {
					box.Y0 = int32(y)
				}
				if int32(y+1) > box.Y1 {
					box.Y1 = int32(y + 1)
				}
			}
			count++
		}
	}
	return count, box
}

func drawRow(tb *toolbar.State, sc, logicalW int32) *frame.Bitmap {
	buf := frame.NewBitmap(int(logicalW*sc), int(toolbar.ToolbarHeight*sc))
	tb.Draw(buf, 0)
	return buf
}

// qc:BR-129
func TestZoomBadgeExposesTheBadgeTheToolbarPaints(t *testing.T) {
	const logicalW = 800
	for _, sc := range []int32{1, 2} {
		tb := toolbar.NewState(logicalW, nil)
		tb.SetScale(sc)
		tb.SetZoom(1.25)
		label, rect, shown := tb.ZoomBadge(logicalW)
		if !shown {
			t.Fatalf("sc=%d: ZoomBadge shown = false at 125%%, want the badge the toolbar paints", sc)
		}
		if label != "125%" || label != tb.ZoomPercent() {
			t.Fatalf("sc=%d: ZoomBadge label = %q, want the same whole-percent ZoomPercent reports (%q)", sc, label, tb.ZoomPercent())
		}
		if rect.X0 >= rect.X1 || rect.Y0 >= rect.Y1 {
			t.Fatalf("sc=%d: ZoomBadge rect %+v is empty", sc, rect)
		}
		bar := toolbar.AddressBarRect(logicalW)
		if rect.X0 < bar.X0 || rect.X1 > bar.X1 || rect.Y0 < bar.Y0 || rect.Y1 > bar.Y1 {
			t.Fatalf("sc=%d: ZoomBadge rect %+v escapes the address bar %+v", sc, rect, bar)
		}

		count, ink := badgeInk(drawRow(tb, sc, logicalW))
		if count == 0 {
			t.Fatalf("sc=%d: the toolbar painted no badge pixels while ZoomBadge reported %+v shown", sc, rect)
		}
		want := frame.Rect4(rect.X0*sc, rect.Y0*sc, rect.X1*sc, rect.Y1*sc)
		if ink != want {
			t.Fatalf("sc=%d: painted badge bbox %+v != the advertised rect scaled to device pixels %+v", sc, ink, want)
		}
		if area := int(rect.W()) * int(rect.H()); count*2 < area {
			t.Fatalf("sc=%d: badge ink covers %d pixels of a %d pixel box, want a filled pill", sc, count, area)
		}
	}
}

// qc:BR-131
func TestZoomBadgeHiddenExactlyWhenTheBadgeIsNotPainted(t *testing.T) {
	const logicalW = 800

	done := toolbar.NewState(logicalW, nil)
	done.SetZoom(1.5)
	done.ResetZoom()
	if label, rect, shown := done.ZoomBadge(logicalW); shown {
		t.Fatalf("ZoomBadge shown = true at 100%% after the grace, want hidden; label=%q rect=%+v", label, rect)
	}
	if count, ink := badgeInk(drawRow(done, 1, logicalW)); count != 0 {
		t.Fatalf("toolbar painted %d badge pixels at 100%% after the grace, bbox %+v, want none", count, ink)
	}

	grace := toolbar.NewState(logicalW, nil)
	grace.SetZoom(1.0)
	if label, _, shown := grace.ZoomBadge(logicalW); !shown {
		t.Fatalf("ZoomBadge shown = false at 100%% inside the grace, want exposed; label=%q", label)
	}
	if count, _ := badgeInk(drawRow(grace, 1, logicalW)); count == 0 {
		t.Fatal("toolbar painted no badge pixels at 100% inside the grace, want the exposed badge")
	}

	find := toolbar.NewState(logicalW, nil)
	find.SetZoom(1.25)
	find.OpenFind()
	if _, _, shown := find.ZoomBadge(logicalW); shown {
		t.Fatal("ZoomBadge shown = true while find is active, want hidden")
	}
	if count, ink := badgeInk(drawRow(find, 1, logicalW)); count != 0 {
		t.Fatalf("toolbar painted %d badge pixels while find was active, bbox %+v, want none", count, ink)
	}

	broken := toolbar.NewState(logicalW, nil)
	broken.SetZoom(1.25)
	broken.Zoom = 0
	if _, _, shown := broken.ZoomBadge(logicalW); shown {
		t.Fatal("ZoomBadge shown = true with zoom 0, want hidden")
	}
	if count, ink := badgeInk(drawRow(broken, 1, logicalW)); count != 0 {
		t.Fatalf("toolbar painted %d badge pixels with zoom 0, bbox %+v, want none", count, ink)
	}
}

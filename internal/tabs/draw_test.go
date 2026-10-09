package tabs

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestDrawTabTextWithGlyphs(t *testing.T) {
	buf := frame.NewBitmap(200, 36)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("fonts not available: %v", err)
	}

	tabRect := frame.Rect4(0, 0, 150, 36)
	drawTabText(buf, "Test Tab", tabRect, frame.RGB(30, 30, 30), fonts, 1)

	hasPixels := false
	for i := 0; i < len(buf.RGBA); i += 4 {
		if buf.RGBA[i+3] > 0 {
			hasPixels = true
			break
		}
	}

	if !hasPixels {
		t.Error("drawTabText should render glyph pixels")
	}
}

func TestDrawTabTextTruncation(t *testing.T) {
	buf := frame.NewBitmap(200, 36)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("fonts not available: %v", err)
	}

	longText := "This is a very long tab title that should be truncated"
	tabRect := frame.Rect4(0, 0, 80, 36)
	drawTabText(buf, longText, tabRect, frame.RGB(30, 30, 30), fonts, 1)

	hasPixels := false
	for i := 0; i < len(buf.RGBA); i += 4 {
		if buf.RGBA[i+3] > 0 {
			hasPixels = true
			break
		}
	}

	if !hasPixels {
		t.Error("drawTabText should render truncated text")
	}
}

func TestDrawTabTextNilFonts(t *testing.T) {
	buf := frame.NewBitmap(200, 36)
	tabRect := frame.Rect4(0, 0, 150, 36)
	// Should not panic with nil fonts
	drawTabText(buf, "Test Tab", tabRect, frame.RGB(30, 30, 30), nil, 1)
}

// chevronMarkCount counts dark pixels in the strip's right 56px at the given
// offset: chevron glyphs are the only thing ever drawn there (at max offset
// the tabs end left of the zone), so the count is the control.
func chevronMarkCount(t *testing.T, mgr *TabManager, width, offset int32) int {
	t.Helper()
	buf := frame.NewBitmap(int(width), TabBarHeight)
	DrawTabBar(buf, mgr, offset, nil, 1)
	n := 0
	for y := int32(0); y < TabBarHeight; y++ {
		for x := width - 56; x < width; x++ {
			c := buf.At(int(x), int(y)).RGBA()
			if (int(c.R)+int(c.G)+int(c.B))/3 < 200 {
				n++
			}
		}
	}
	return n
}

// TestDrawTabBarChevrons pins chevron visibility end to end: an overflowing
// strip draws the controls, a fitting one leaves the zone clean background.
func TestDrawTabBarChevrons(t *testing.T) {
	full := NewManager(nil)
	for i := 0; i < 40; i++ {
		full.NewTab()
	}
	if n := chevronMarkCount(t, full, 1000, StripMaxOffset(40, 1000)); n < 20 {
		t.Fatalf("chevron zone has %d dark pixels with 40 tabs, want controls (>= 20)", n)
	}
	fitting := NewManager(nil)
	for i := 0; i < 3; i++ {
		fitting.NewTab()
	}
	if n := chevronMarkCount(t, fitting, 1000, 0); n != 0 {
		t.Fatalf("chevron zone has %d dark pixels with 3 tabs, want none", n)
	}
}

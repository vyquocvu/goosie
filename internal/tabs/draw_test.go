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

package tabs

import (
	"image"
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

// faviconBand counts non-background pixels in the icon slot of tab 0
// (logical x 16..32, tab 0 starts at LeftMargin=8, icon inset 8).
func faviconBand(t *testing.T, mgr *TabManager) (red, dark int) {
	t.Helper()
	buf := frame.NewBitmap(400, TabBarHeight)
	DrawTabBar(buf, mgr, 0, nil, 1)
	for y := int32(TabBarHeight/2 - 8); y < TabBarHeight/2+8; y++ {
		for x := int32(LeftMargin) + 8; x < int32(LeftMargin)+8+tabIconSize; x++ {
			c := buf.At(int(x), int(y)).RGBA()
			r, g, b := int(c.R), int(c.G), int(c.B)
			if r > 180 && g < 100 && b < 100 {
				red++
			}
			if (r+g+b)/3 < 180 {
				dark++
			}
		}
	}
	return red, dark
}

// TestDrawTabFaviconImage pins that a stored favicon is resampled into the
// icon slot.
func TestDrawTabFaviconImage(t *testing.T) {
	mgr := NewManager(nil)
	tab := mgr.NewTab()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	tab.SetFavicon(img)

	red, _ := faviconBand(t, mgr)
	if red == 0 {
		t.Fatal("favicon slot has no red pixels; the stored image was not drawn")
	}
}

// TestDrawTabFallbackIcon pins the placeholder: a tab with no favicon still
// shows a visible glyph so the strip has a consistent icon column.
func TestDrawTabFallbackIcon(t *testing.T) {
	mgr := NewManager(nil)
	mgr.NewTab()

	if _, dark := faviconBand(t, mgr); dark < 8 {
		t.Fatalf("fallback icon slot has %d dark pixels, want a visible glyph (>= 8)", dark)
	}
}

// TestDrawTabFaviconIgnoresEmptyImage guards the draw path against a decoded
// image with a zero-sized (or fully empty) bounds rect.
func TestDrawTabFaviconIgnoresEmptyImage(t *testing.T) {
	mgr := NewManager(nil)
	tab := mgr.NewTab()
	tab.SetFavicon(image.NewRGBA(image.Rect(0, 0, 0, 0)))
	buf := frame.NewBitmap(400, TabBarHeight)
	DrawTabBar(buf, mgr, 0, nil, 1)
	if _, dark := faviconBand(t, mgr); dark < 8 {
		t.Fatalf("empty favicon should fall back to the glyph, got %d dark pixels", dark)
	}
}

package toolbar

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestLoadingIndicator(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(800, fonts)
	state.SetLoading(true)

	buf := frame.NewBitmap(800, 40)
	state.Draw(buf, 0)

	hasBluePixels := false
	for y := 37; y < 40; y++ {
		for x := 0; x < 800; x++ {
			idx := y*buf.Stride + x*4
			if idx+3 < len(buf.RGBA) {
				r, g, b := buf.RGBA[idx], buf.RGBA[idx+1], buf.RGBA[idx+2]
				if r == 70 && g == 140 && b == 220 {
					hasBluePixels = true
					break
				}
			}
		}
		if hasBluePixels {
			break
		}
	}

	if !hasBluePixels {
		t.Error("loading indicator should draw blue progress line at bottom")
	}
}

func TestNoLoadingIndicatorWhenNotLoading(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(800, fonts)
	state.SetLoading(false)

	buf := frame.NewBitmap(800, 40)
	state.Draw(buf, 0)

	hasBlueLine := false
	for y := 37; y < 40; y++ {
		for x := 100; x < 700; x++ {
			idx := y*buf.Stride + x*4
			if idx+3 < len(buf.RGBA) {
				r, g, b := buf.RGBA[idx], buf.RGBA[idx+1], buf.RGBA[idx+2]
				if r == 70 && g == 140 && b == 220 {
					hasBlueLine = true
					break
				}
			}
		}
		if hasBlueLine {
			break
		}
	}

	if hasBlueLine {
		t.Error("loading indicator should not appear when not loading")
	}
}

func TestErrorIndicator(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(800, fonts)
	state.URL = "https://example.com"
	state.Error = "Connection failed"

	buf := frame.NewBitmap(800, 40)
	state.Draw(buf, 0)

	hasRedPixels := false
	for y := 0; y < 40; y++ {
		for x := 100; x < 700; x++ {
			idx := y*buf.Stride + x*4
			if idx+3 < len(buf.RGBA) {
				r, g, b := buf.RGBA[idx], buf.RGBA[idx+1], buf.RGBA[idx+2]
				if r == 200 && g == 50 && b == 50 {
					hasRedPixels = true
					break
				}
			}
		}
		if hasRedPixels {
			break
		}
	}

	if !hasRedPixels {
		t.Error("error indicator should draw red error text in address bar")
	}
}

func TestNoErrorIndicatorWhenFocused(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(800, fonts)
	state.URL = "https://example.com"
	state.Input = "editing..."
	state.Error = "Connection failed"
	state.Focus = FocusAddress

	buf := frame.NewBitmap(800, 40)
	state.Draw(buf, 0)

	hasRedPixels := false
	for y := 0; y < 40; y++ {
		for x := 100; x < 700; x++ {
			idx := y*buf.Stride + x*4
			if idx+3 < len(buf.RGBA) {
				r, g, b := buf.RGBA[idx], buf.RGBA[idx+1], buf.RGBA[idx+2]
				if r == 200 && g == 50 && b == 50 {
					hasRedPixels = true
					break
				}
			}
		}
		if hasRedPixels {
			break
		}
	}

	if hasRedPixels {
		t.Error("error indicator should not appear when address bar is focused")
	}
}

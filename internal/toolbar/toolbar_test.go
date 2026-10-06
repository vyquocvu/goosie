package toolbar

import (
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
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

// TestConcurrentChromeAccessIsRaceFree hammers the three-thread sharing the
// browser relies on: typing/clicks (event pump), loading flags and tab sync
// (navigation drain), and Draw (UI loop). Before the State mutex this trips
// the race detector; the private-field writes it also performs would not
// even compile now, which is the point.
func TestConcurrentChromeAccessIsRaceFree(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(800, fonts)
	state.Navigate("https://example.com/very/long/address/that/wraps/around")
	done := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buf := frame.NewBitmap(800, 40)
			for j := 0; j < 50; j++ {
				select {
				case <-done:
					return
				default:
				}
				switch i {
				case 0:
					state.HandleKeyEvent('x'+rune(j%26), 0)
					state.HandleClick(frame.Point{X: int32(100 + j), Y: 10}, surface.ButtonLeft)
				case 1:
					state.SetLoading(j%2 == 0)
					state.SetError("boom")
					state.SetError("")
				case 2:
					state.Draw(buf, 0)
					_ = state.CursorAt(frame.Point{X: 100, Y: 10})
				case 3:
					h := NewHistory()
					h.Push("https://example.com/")
					state.SyncFromTab("https://example.com/", true, "", h)
					_, _, _ = state.FindCounts()
				}
			}
		}(i)
	}
	wg.Wait()
	close(done)
}

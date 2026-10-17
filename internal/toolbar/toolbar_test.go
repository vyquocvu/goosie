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

func TestToolbarButtonDisabledState(t *testing.T) {
	state := NewState(800, nil)
	var traverseDelta int
	state.OnTraverse = func(delta int) {
		traverseDelta = delta
	}

	// Initially empty history: neither back nor forward should fire
	backRect := ButtonRect(ButtonBack, 800)
	clickPt := frame.Point{X: backRect.X0 + 5, Y: backRect.Y0 + 5}
	state.HandleClick(clickPt, surface.ButtonLeft)
	if traverseDelta != 0 {
		t.Fatalf("traversed on empty history back: got delta %d", traverseDelta)
	}

	forwardRect := ButtonRect(ButtonForward, 800)
	clickPt = frame.Point{X: forwardRect.X0 + 5, Y: forwardRect.Y0 + 5}
	state.HandleClick(clickPt, surface.ButtonLeft)
	if traverseDelta != 0 {
		t.Fatalf("traversed on empty history forward: got delta %d", traverseDelta)
	}

	// Push 2 pages: back enabled, forward disabled
	state.History.Push("https://a/")
	state.History.Push("https://b/")

	clickPt = frame.Point{X: backRect.X0 + 5, Y: backRect.Y0 + 5}
	state.HandleClick(clickPt, surface.ButtonLeft)
	if traverseDelta != -1 {
		t.Fatalf("expected traverse -1, got %d", traverseDelta)
	}

	traverseDelta = 0
	clickPt = frame.Point{X: forwardRect.X0 + 5, Y: forwardRect.Y0 + 5}
	state.HandleClick(clickPt, surface.ButtonLeft)
	if traverseDelta != 0 {
		t.Fatalf("traversed on forward at head of history: got delta %d", traverseDelta)
	}

	// Move cursor back: back disabled, forward enabled
	state.History.Back()
	clickPt = frame.Point{X: forwardRect.X0 + 5, Y: forwardRect.Y0 + 5}
	state.HandleClick(clickPt, surface.ButtonLeft)
	if traverseDelta != 1 {
		t.Fatalf("expected traverse 1, got %d", traverseDelta)
	}
}

func TestToolbarNilHistorySafe(t *testing.T) {
	state := NewState(800, nil)
	state.History = nil
	var traversed bool
	state.OnTraverse = func(delta int) { traversed = true }

	// Should not panic on Navigate with nil history
	state.Navigate("https://example.com")

	// Should not panic on HandleClick for Back/Forward
	backRect := ButtonRect(ButtonBack, 800)
	state.HandleClick(frame.Point{X: backRect.X0 + 5, Y: backRect.Y0 + 5}, surface.ButtonLeft)
	if traversed {
		t.Fatal("traversed should not fire with nil History")
	}

	forwardRect := ButtonRect(ButtonForward, 800)
	state.HandleClick(frame.Point{X: forwardRect.X0 + 5, Y: forwardRect.Y0 + 5}, surface.ButtonLeft)
	if traversed {
		t.Fatal("traversed should not fire with nil History")
	}

	// Should not panic on Draw
	buf := frame.NewBitmap(800, 60)
	state.Draw(buf, 0)
}

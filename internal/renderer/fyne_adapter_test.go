package renderer

import (
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

func TestFyneAdapterCreation(t *testing.T) {
	a := NewFyneAdapter()
	if a == nil {
		t.Fatal("NewFyneAdapter returned nil")
	}
	if a.Content() == nil {
		t.Error("Content() should not be nil")
	}
	if a.CurrentFrame() != nil {
		t.Error("CurrentFrame() should be nil initially")
	}
}

func TestFyneAdapterPresentFrame(t *testing.T) {
	a := NewFyneAdapter()
	fb := raster.NewFrameBuffer(200, 100)

	// Fill with a color to verify.
	img := fb.Image()
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	a.PresentFrame(fb)

	if a.CurrentFrame() != fb {
		t.Error("CurrentFrame should match presented frame")
	}
}

func TestFyneAdapterContentStable(t *testing.T) {
	a := NewFyneAdapter()
	c1 := a.Content()
	c2 := a.Content()
	if c1 != c2 {
		t.Error("Content() should return the same stable object")
	}
}

func TestFyneAdapterScrollNoRebuild(t *testing.T) {
	a := NewFyneAdapter()
	content := a.Content()

	// Scroll should not change the content object.
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(0, 100)
	a.SetViewport(vp)

	if a.Content() != content {
		t.Error("Content() should remain stable after scroll")
	}

	gotVP := a.Viewport()
	if gotVP.ScrollY != 100 {
		t.Errorf("Viewport ScrollY = %f, want 100", gotVP.ScrollY)
	}
}

func TestFyneAdapterMultipleFrames(t *testing.T) {
	a := NewFyneAdapter()

	fb1 := raster.NewFrameBuffer(100, 100)
	fb2 := raster.NewFrameBuffer(200, 200)

	a.PresentFrame(fb1)
	if a.CurrentFrame() != fb1 {
		t.Error("first frame not set")
	}

	a.PresentFrame(fb2)
	if a.CurrentFrame() != fb2 {
		t.Error("second frame should replace first")
	}
}

func TestFyneAdapterResize(t *testing.T) {
	a := NewFyneAdapter()
	// Resize should not panic.
	a.Resize(a.Content().MinSize())
}

func TestFyneAdapterViewportThreadSafe(t *testing.T) {
	a := NewFyneAdapter()
	done := make(chan struct{})

	// Concurrent viewport updates.
	go func() {
		for i := 0; i < 100; i++ {
			a.SetViewport(frame.NewViewport(800, 600, frame.PixelScaleDefault).WithScroll(0, float32(i)))
		}
		close(done)
	}()

	// Concurrent reads.
	for i := 0; i < 100; i++ {
		_ = a.Viewport()
	}
	<-done
}

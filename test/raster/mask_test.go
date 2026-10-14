package raster

import (
	"image"
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// A masked fill shows the colour only where the mask says so: alpha masks
// gate on alpha, alpha-less images fall back to luminance (match-source).
func TestMaskedFillAlpha(t *testing.T) {
	mask := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			a := uint8(0)
			if x < 5 {
				a = 255
			}
			mask.SetRGBA(x, y, color.RGBA{0, 0, 0, a})
		}
	}
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(0, 0, 10, 10),
		Color: frame.RGB(255, 0, 0),
		Mask:  mask,
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(2, 5); got != frame.RGB(255, 0, 0) {
		t.Errorf("masked-in pixel = %v, want red", got)
	}
	if got := out.At(7, 5); got == frame.RGB(255, 0, 0) {
		t.Errorf("masked-out pixel = %v, want untouched", got)
	}
}

func TestMaskedFillLuminance(t *testing.T) {
	// Opaque image without alpha: white half passes, black half blocks.
	mask := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			v := uint8(0)
			if x >= 5 {
				v = 255
			}
			mask.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(0, 0, 10, 10),
		Color: frame.RGB(0, 0, 255),
		Mask:  mask,
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(7, 5); got != frame.RGB(0, 0, 255) {
		t.Errorf("luminant pixel = %v, want blue", got)
	}
	if got := out.At(2, 5); got == frame.RGB(0, 0, 255) {
		t.Errorf("dark pixel = %v, want untouched", got)
	}
}

func TestMaskedFillStretches(t *testing.T) {
	// A 2px mask stretches over the fill rect.
	mask := image.NewRGBA(image.Rect(0, 0, 2, 1))
	mask.SetRGBA(0, 0, color.RGBA{0, 0, 0, 255})
	mask.SetRGBA(1, 0, color.RGBA{0, 0, 0, 0})
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(0, 0, 10, 10),
		Color: frame.RGB(0, 255, 0),
		Mask:  mask,
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(2, 5); got != frame.RGB(0, 255, 0) {
		t.Errorf("left pixel = %v, want green", got)
	}
	if got := out.At(7, 5); got == frame.RGB(0, 255, 0) {
		t.Errorf("right pixel = %v, want untouched", got)
	}
}

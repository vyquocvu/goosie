package frame_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

func alphaMask(w, h int, at func(x, y int) uint8) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Pix[y*m.Stride+x] = at(x, y)
		}
	}
	return m
}

func TestBlitMaskAppliesCoverage(t *testing.T) {
	b := frame.NewBitmap(4, 2)
	black := frame.RGB(0, 0, 0)
	// Half coverage has to produce half the alpha and none of the color: a
	// premultiplied source at 50% is (0,0,0,128), not (0,0,0,255) at 50% blend.
	m := alphaMask(2, 2, func(x, y int) uint8 {
		if x == 0 {
			return 128
		}
		return 255
	})
	var writes int64
	b.BlitMask(m, frame.Point{X: 1, Y: 0}, black, b.Bounds(), &writes)
	if got := b.At(0, 0); got != frame.TransparentBlack {
		t.Fatalf("pixel left of the mask = %v, want untouched", got)
	}
	if got := b.At(1, 0); got.A() < 120 || got.A() > 135 {
		t.Fatalf("half-covered pixel alpha = %d, want about 128", got.A())
	}
	if got := b.At(2, 0); got != black {
		t.Fatalf("fully covered pixel = %v, want black", got)
	}
	if got := b.At(3, 1); got != frame.TransparentBlack {
		t.Fatalf("pixel right of the mask = %v, want untouched", got)
	}
	if writes != 4 {
		t.Fatalf("writes = %d, want 4 (the 2x2 mask area)", writes)
	}
}

func TestBlitMaskClipsAndIgnoresEmptySources(t *testing.T) {
	b := frame.NewBitmap(4, 4)
	m := alphaMask(4, 4, func(x, y int) uint8 { return 255 })
	b.BlitMask(m, frame.Point{X: 2, Y: 2}, frame.RGB(255, 0, 0), b.Bounds(), nil)
	if got := b.At(3, 3); got != frame.RGB(255, 0, 0) {
		t.Fatalf("clipped corner = %v, want red", got)
	}
	if got := b.At(1, 1); got != frame.TransparentBlack {
		t.Fatalf("pixel outside the mask = %v, want untouched", got)
	}
	b.BlitMask(nil, frame.Point{}, frame.RGB(0, 255, 0), b.Bounds(), nil)
	b.BlitMask(m, frame.Point{}, frame.TransparentBlack, b.Bounds(), nil)
	b.BlitMask(m, frame.Point{X: 100, Y: 100}, frame.RGB(0, 0, 255), b.Bounds(), nil)
	if got := b.At(0, 0); got != frame.TransparentBlack {
		t.Fatalf("transparent paint changed a pixel: %v", got)
	}
	if got := b.At(2, 2); got != frame.RGB(255, 0, 0) {
		t.Fatalf("an out-of-bounds blit modified the buffer: %v", got)
	}
}

func TestScaleOverStaysInsideDestination(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	src.Set(3, 3, color.RGBA{B: 255, A: 255})

	b := frame.NewBitmap(16, 16)
	dst := frame.Rect4(4, 4, 12, 12)
	var writes int64
	b.ScaleOver(src, frame.Rect4(0, 0, 4, 4), dst, b.Bounds(), 255, &writes)
	if writes != 64 {
		t.Fatalf("writes = %d, want 64 (the destination area, once)", writes)
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			inside := x >= 4 && x < 12 && y >= 4 && y < 12
			if !inside && b.At(x, y) != frame.TransparentBlack {
				t.Fatalf("scale escaped its rect at (%d,%d): %v", x, y, b.At(x, y))
			}
		}
	}
	if got := b.At(11, 11); got.B() < 200 {
		t.Fatalf("bottom-right texel = %v, want the blue source pixel to reach here", got)
	}
}

func TestScaleOverReadsSubImageByItsOwnOrigin(t *testing.T) {
	// A sub-image has a nonzero Rect.Min over a shared buffer. Sampling it as if
	// the origin were (0,0) reads the wrong texels, so this pins the offset math.
	full := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range full.Pix {
		full.Pix[i] = 0
	}
	for y := 2; y < 4; y++ {
		for x := 2; x < 4; x++ {
			full.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	sub := full.SubImage(image.Rect(2, 2, 4, 4)).(*image.RGBA)

	b := frame.NewBitmap(4, 4)
	b.ScaleOver(sub, frame.Rect4(2, 2, 4, 4), frame.Rect4(0, 0, 4, 4), b.Bounds(), 255, nil)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got := b.At(x, y); got != frame.RGB(255, 0, 0) {
				t.Fatalf("sub-image pixel (%d,%d) = %v, want the red block it covers", x, y, got)
			}
		}
	}
}

func TestScaleOverHonoursCoverageAndClip(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	b := frame.NewBitmap(8, 8)
	b.FillRect(b.Bounds(), frame.RGB(0, 0, 0), nil)
	// Half coverage over black leaves half-white pixels, and a clip smaller than
	// the destination rect leaves the rest black.
	b.ScaleOver(src, frame.Rect4(0, 0, 2, 2), frame.Rect4(0, 0, 8, 8), frame.Rect4(0, 0, 4, 8), 128, nil)
	if got := b.At(1, 1); got.R() < 120 || got.R() > 135 {
		t.Fatalf("half-covered pixel = %v, want roughly half white", got)
	}
	if got := b.At(6, 1); got != frame.RGB(0, 0, 0) {
		t.Fatalf("pixel outside the clip = %v, want it untouched", got)
	}
	b.ScaleOver(src, frame.Rect4(0, 0, 2, 2), frame.Rect4(0, 0, 8, 8), b.Bounds(), 0, nil)
	if got := b.At(1, 1); got.R() < 120 {
		t.Fatalf("zero coverage erased a pixel it should not have touched: %v", got)
	}
}

func TestBlitMaskIsAllocationFree(t *testing.T) {
	b := frame.NewBitmap(64, 64)
	m := alphaMask(16, 16, func(x, y int) uint8 { return uint8(x * 16) })
	black := frame.RGB(0, 0, 0)
	if n := testing.AllocsPerRun(200, func() {
		b.BlitMask(m, frame.Point{X: 8, Y: 8}, black, b.Bounds(), nil)
	}); n != 0 {
		t.Fatalf("BlitMask allocated %v times per call", n)
	}
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if n := testing.AllocsPerRun(200, func() {
		b.ScaleOver(src, frame.Rect4(0, 0, 8, 8), frame.Rect4(0, 0, 32, 32), b.Bounds(), 255, nil)
	}); n != 0 {
		t.Fatalf("ScaleOver allocated %v times per call", n)
	}
}

// TestScaleOverSamplesCorrectSliceWhenClipped pins that a scaled image spanning
// several raster tiles shows its matching slice in each tile, not the whole
// image rescaled into every tile. A background wider than one tile used to
// repeat across every tile it spanned because the source step was derived from
// the clipped rect rather than the full destination box.
func TestScaleOverSamplesCorrectSliceWhenClipped(t *testing.T) {
	// 8x4 source: left half red, right half green.
	src := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			if x < 4 {
				src.Set(x, y, color.RGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{G: 255, A: 255})
			}
		}
	}

	// Draw into an 8-wide destination but clip to the LEFT 4 columns, as the
	// tile rasterizer does for the tile holding the left half. The correct slice
	// is red throughout; the old bug would rescale the whole source into the 4px
	// window and bleed green in at the right edge.
	left := frame.NewBitmap(8, 4)
	left.ScaleOver(src, frame.Rect4(0, 0, 8, 4), frame.Rect4(0, 0, 8, 4), frame.Rect4(0, 0, 4, 4), 255, nil)
	if got := left.At(1, 2); got.G() > 40 || got.R() < 200 {
		t.Fatalf("left tile at x=1 = %v, want red (source slice [0,4)), not a rescaled whole image", got)
	}

	// The RIGHT 4 columns must sample the green half, proving the offset tracks
	// the tile's position within the full destination box.
	right := frame.NewBitmap(8, 4)
	right.ScaleOver(src, frame.Rect4(0, 0, 8, 4), frame.Rect4(0, 0, 8, 4), frame.Rect4(4, 0, 8, 4), 255, nil)
	if got := right.At(6, 2); got.R() > 40 || got.G() < 200 {
		t.Fatalf("right tile at x=6 = %v, want green (source slice [4,8)), not a rescaled whole image", got)
	}
}

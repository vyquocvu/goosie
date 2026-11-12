package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// qc:BR-266
//
// A faded group must not lose the part of a glyph that lies below the baseline.
// The offscreen pass covers the region the members actually painted, and a text
// command's recorded bounds end at the baseline (paint.TextRun.Bounds sets
// Y1 = glyph Y), so a region read off the group's bounds would clip the tail off
// a faded 'g' and leave the descender rows as plain background. This compares a
// grouped render against the same text rendered ungrouped: the ink extent has to
// be identical, and it has to reach below the baseline, or the test would be
// comparing two empty rectangles.
func TestGroupOpacityFadedTextKeepsDescenderInk(t *testing.T) {
	f, g := mustFonts(t)
	bg := frame.RGB(255, 255, 255)
	const baseline = 96 // the pen's y; ink at or below this row is descender
	text := paint.DisplayCmd{
		Kind: paint.CmdText,
		Text: paint.TextRun{
			Glyphs: []paint.GlyphRun{{Rune: 'g', X: 40, Y: baseline, Size: 64}},
			Color:  frame.RGB(0, 0, 0),
		},
	}

	plain := newTile()
	if err := raster.RasterizeTile(build(t, text), frame.Rect4(0, 0, 256, 256), plain, f, g, bg); err != nil {
		t.Fatalf("ungrouped rasterize: %v", err)
	}
	plainInk := inkRect(plain, bg)
	if plainInk.Y1 <= baseline {
		t.Fatalf("the fixture proves nothing: ungrouped ink ends at row %d, want a descender past the baseline %d",
			plainInk.Y1, baseline)
	}

	l := paint.NewList(1)
	l.Append(text)
	l.AddGroup(0.5, 1)
	dl := l.Build(1).Publish()
	faded := newTile()
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), faded, f, g, bg); err != nil {
		t.Fatalf("grouped rasterize: %v", err)
	}

	// Same extent: grouping fades the ink, it does not move or crop it.
	if got := inkRect(faded, bg); got != plainInk {
		t.Errorf("faded ink extent = %v, want the ungrouped extent %v", got, plainInk)
	}

	// And the descender carries the half-faded glyph, not background and not the
	// unfaded one. Black over white: halving a pixel's coverage maps its value v
	// to (255+v)/2 whatever the antialiasing, so the ungrouped value at the
	// darkest descender pixel predicts the faded one exactly.
	px, py := darkestAtOrBelow(plain, baseline)
	if px < 0 {
		t.Fatalf("no dark pixel at or below row %d, want the glyph's descender to be there", baseline)
	}
	unfaded := int(plain.At(px, py).R())
	if got := int(faded.At(px, py).R()); got-(255+unfaded)/2 > 2 || (255+unfaded)/2-got > 2 {
		t.Errorf("descender pixel (%d,%d) fades to %d, want %d±2 (ungrouped it was %d)",
			px, py, got, (255+unfaded)/2, unfaded)
	}
	if far := faded.At(0, 0); far != bg {
		t.Errorf("pixel far from the glyph = %v, want the untouched background %v", far, bg)
	}
}

// inkRect bounds every pixel of b that differs from bg; empty when none does.
func inkRect(b *frame.Bitmap, bg frame.Color) frame.Rect {
	var r frame.Rect
	for y := 0; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			if b.At(x, y) == bg {
				continue
			}
			r = r.Union(frame.Rect4(int32(x), int32(y), int32(x+1), int32(y+1)))
		}
	}
	return r
}

// darkestAtOrBelow is the darkest pixel at or below row baseline: the glyph's
// tail, which is the ink a bounds-sized fade region would have cut off.
func darkestAtOrBelow(b *frame.Bitmap, baseline int) (int, int) {
	best, bx, by := 256, -1, -1
	for y := baseline; y < b.H; y++ {
		for x := 0; x < b.W; x++ {
			if v := int(b.At(x, y).R()); v < best {
				best, bx, by = v, x, y
			}
		}
	}
	if bx < 0 {
		return -1, -1 // no ink at all below the baseline; the caller rejects it
	}
	return bx, by
}

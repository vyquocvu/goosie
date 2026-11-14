package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// qc:BR-087
func TestDefaultIgnorableControlsCostNothingAndInkNothing(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []rune{
		'\u00AD',
		'\u200B',
		'\u200C',
		'\u200D',
		'\u200E',
		'\u200F',
		'\uFEFF',
		'\uFE0F',
	} {
		for _, slot := range fontSlots {
			if adv := f.GlyphAdvanceFixed(16, r, slot); adv != 0 {
				t.Errorf("U+%04X advance = %d in slot %+v, want 0", r, adv, slot)
			}
			g := f.Glyph(16, r, slot)
			if !g.Ok {
				t.Errorf("U+%04X Glyph.Ok = false in slot %+v, want an ignorable (empty) glyph", r, slot)
			}
			if g.Bounds.X1 != g.Bounds.X0 || g.Bounds.Y1 != g.Bounds.Y0 {
				t.Errorf("U+%04X bounds = %+v in slot %+v, want empty: an inked control draws a phantom box over its neighbours", r, g.Bounds, slot)
			}
			if g.Mask != nil && len(g.Mask.Pix) > 0 {
				t.Errorf("U+%04X mask has %d pixels in slot %+v, want none", r, len(g.Mask.Pix), slot)
			}
		}
	}
}

// qc:BR-087
func TestCombiningMarksStillInkAfterTheIgnorableRule(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	slot := frame.FontSlot{Family: frame.FontTimes}
	for _, r := range []rune{'\u0302', '\u0301', '\u0303', '\u031B'} {
		if adv := f.GlyphAdvanceFixed(16, r, slot); adv != 0 {
			t.Errorf("U+%04X advance = %d, want 0: a combining mark must not shift the pen", r, adv)
		}
		g := f.Glyph(16, r, slot)
		if g.Bounds.X1 <= g.Bounds.X0 || g.Bounds.Y1 <= g.Bounds.Y0 {
			t.Errorf("U+%04X bounds = %+v, want accent ink with area", r, g.Bounds)
		}
	}
}

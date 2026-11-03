package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// Chromium 1.63 measured (scripts/verify-br087-text-parity.js, 2026-11-04):
// "a" is 8.906px wide and "a" plus any of these controls is still 8.906px, and
// a lone control occupies 0px. A default-ignorable code point therefore costs
// no advance and carries no ink, even on a host whose faces do map it - which
// is exactly where Goosie used to differ: the host drew U+200D and U+200F as
// phantom boxes and charged U+00AD a 5.328px hyphen.
// qc:BR-087
func TestDefaultIgnorableControlsCostNothingAndInkNothing(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []rune{
		'\u00AD', // soft hyphen
		'\u200B', // zero-width space
		'\u200C', // zero-width non-joiner
		'\u200D', // zero-width joiner
		'\u200E', // left-to-right mark
		'\u200F', // right-to-left mark
		'\uFEFF', // zero-width no-break space (BOM)
		'\uFE0F', // variation selector-16
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

// Combining marks are the opposite case and must not be swept up by the rule:
// Chromium advances them 0 yet inks the accent, so ế keeps its tone mark.
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

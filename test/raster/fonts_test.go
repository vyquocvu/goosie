package raster

import (
	"bytes"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

// fontSlots is one representative of every face a resolved style can ask for.
var fontSlots = []frame.FontSlot{
	{},
	{Bold: true},
	{Italic: true},
	{Bold: true, Italic: true},
	{Family: frame.FontTimes},
	{Family: frame.FontTimes, Bold: true},
	{Family: frame.FontTimes, Italic: true},
	{Family: frame.FontTimes, Bold: true, Italic: true},
	{Family: frame.FontArial},
	{Family: frame.FontArial, Bold: true},
	{Family: frame.FontCourier},
	{Family: frame.FontCourier, Italic: true},
	{Family: frame.FontGeorgia},
	{Family: frame.FontGeorgia, Bold: true, Italic: true},
	{Family: frame.FontVerdana},
	{Family: frame.FontVerdana, Bold: true},
}

// TestFontsWithoutSystemFontsStillDraw is the determinism contract: a host with
// no system fonts - which is what a Linux CI runner is - must still construct a
// font set and serve every slot, because a slot falls back to the embedded face
// rather than failing the page.
func TestFontsWithoutSystemFontsStillDraw(t *testing.T) {
	f, err := raster.NewEmbeddedFonts()
	if err != nil {
		t.Fatalf("NewEmbeddedFonts: %v", err)
	}
	for _, slot := range fontSlots {
		g := f.Glyph(16, 'A', slot)
		if !g.Ok || g.Mask == nil {
			t.Fatalf("slot %+v draws no glyph", slot)
		}
		if f.GlyphAdvance(16, 'A', slot) <= 0 {
			t.Errorf("slot %+v reports no advance", slot)
		}
	}
}

// TestSystemFontFlagFallsBackPerSlot checks the fallback is per slot rather than
// per run: with the host fonts off, every family draws the embedded face at the
// slot's own weight.
func TestSystemFontFlagFallsBackPerSlot(t *testing.T) {
	t.Setenv("GOOSIE_SYSTEM_FONTS", "off")
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	for _, slot := range fontSlots {
		want := f.Glyph(16, 'A', frame.FontSlot{Bold: slot.Bold})
		got := f.Glyph(16, 'A', slot)
		if want.Mask == nil || got.Mask == nil {
			t.Fatalf("slot %+v: no mask", slot)
		}
		if !bytes.Equal(got.Mask.Pix, want.Mask.Pix) || got.Bounds != want.Bounds {
			t.Errorf("slot %+v did not fall back to the embedded face", slot)
		}
	}
}

// TestSlotsSharingAFaceShareItsGlyphs: a fallback is a face, not a copy. With
// the host fonts off every family is served by the embedded one, and a glyph
// rasterized for Times is the same bitmap Georgia asks for - which is also what
// keeps measurement and paint from ever disagreeing.
func TestSlotsSharingAFaceShareItsGlyphs(t *testing.T) {
	t.Setenv("GOOSIE_SYSTEM_FONTS", "off")
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	times := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontTimes})
	georgia := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontGeorgia})
	if times.Mask == nil || georgia.Mask == nil {
		t.Fatal("no glyph mask")
	}
	if times.Mask != georgia.Mask {
		t.Error("two slots served by one face rasterized it twice")
	}
	if bold := f.Glyph(16, 'a', frame.FontSlot{Family: frame.FontTimes, Bold: true}); bold.Mask == times.Mask {
		t.Error("a bold slot reused the regular face")
	}
}

// TestSlotsNeverCoverLessThanTheEmbeddedFace: a text family has no arrow, no
// summation sign and no check mark, so those fall back to the embedded face
// rather than disappearing from the page. This holds on a host with the fonts
// and on one without them, which is why it compares against the embedded slot
// instead of a fixed answer.
func TestSlotsNeverCoverLessThanTheEmbeddedFace(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	for _, r := range []rune{'←', '→', '∑', '√', '✓', '★', '♠', '£', '∞'} {
		for _, slot := range fontSlots {
			if embedded := f.Glyph(16, r, frame.FontSlot{Bold: slot.Bold}); embedded.Ok {
				if got := f.Glyph(16, r, slot); !got.Ok || got.Mask == nil {
					t.Errorf("slot %+v dropped %q that the embedded face has", slot, r)
				}
			}
		}
	}
}

// TestGlyphAtlasKeysSlotsApart: the atlas caches per requested slot, so a page
// that draws one glyph in two weights never reads back the other's mask.
func TestGlyphAtlasKeysSlotsApart(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	g := raster.NewGlyphAtlas(1<<20, f)
	regular := g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes})
	bold := g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes, Bold: true})
	if regular.Mask == nil || bold.Mask == nil {
		t.Fatal("no glyph mask")
	}
	if regular.Mask == bold.Mask {
		t.Error("the atlas served a bold glyph for a regular request")
	}
	if g.Get('a', 16, frame.FontSlot{Family: frame.FontTimes}).Mask != regular.Mask {
		t.Error("the same slot re-rasterized instead of hitting the cache")
	}
}

// TestGlyphAdvanceFixedIsTheUnroundedAdvance pins the fractional-measurement
// contract: GlyphAdvanceFixed carries the face's raw 26.6 advance, and the
// integer GlyphAdvance is exactly that value rounded once. A run measured by
// summing the fixed values must disagree with summing the rounded ones when
// the advance has a fraction, which is the whole reason the fixed API exists.
func TestGlyphAdvanceFixedIsTheUnroundedAdvance(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	const size = int32(16)
	for _, slot := range fontSlots {
		fixed := f.GlyphAdvanceFixed(size, 'e', slot)
		if fixed <= 0 {
			t.Fatalf("slot %+v: GlyphAdvanceFixed('e') = %d, want a positive advance", slot, fixed)
		}
		if got, want := f.GlyphAdvance(size, 'e', slot), (fixed+32)>>6; got != want {
			t.Errorf("slot %+v: GlyphAdvance = %d, want %d (fixed %d rounded once)", slot, got, want, fixed)
		}
	}
}

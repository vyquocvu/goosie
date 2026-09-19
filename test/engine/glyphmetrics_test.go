package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// TestGlyphAdvancesFromMetrics pins the anti-ghosting contract: with a metrics
// source wired in, paint spaces glyphs by the font's real advance, not by the
// len*0.5em estimate. "WWWW" is the sharpest probe because W is one of the
// widest glyphs in every family this engine loads, so the estimate under-spaces
// it by nearly half and used to draw visibly overlapping glyphs.
func TestGlyphAdvancesFromMetrics(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(
		`<html><body style="margin: 0"><p>WWWW</p></body></html>`,
		nil, 400, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatal(err)
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	// A document that names no family draws in the standard font, and the
	// advance below has to come from that same face or the run's own spacing
	// contradicts the box layout measured.
	slot := frame.FontSlot{Family: frame.FontTimes}
	wantFixed := fonts.GlyphAdvanceFixed(16, 'W', slot)
	if wantFixed <= 0 {
		t.Fatal("font reports no advance for W")
	}
	found := false
	for _, c := range dl.All() {
		if c.Kind != paint.CmdText || len(c.Text.Glyphs) != 4 {
			continue
		}
		g := c.Text.Glyphs
		if g[0].Rune != 'W' {
			continue
		}
		if g[0].Slot != slot {
			t.Errorf("glyph slot = %+v, want %+v (the face layout measured with)", g[0].Slot, slot)
		}
		found = true
		for i := 1; i < len(g); i++ {
			// Positions accumulate the fractional advance and round only at
			// each glyph, so the spacing between any two glyphs is not constant.
			want := (int32(i)*wantFixed + 32) >> 6
			if got := g[i].X - g[0].X; got != want {
				t.Errorf("glyph %d offset = %d, want %d (fractional advance accumulation)", i, got, want)
			}
		}
	}
	if !found {
		t.Fatal("no 4-glyph W text command in the display list")
	}
}

// TestLayoutWordWidthFromMetrics checks the same metrics source reaches the
// inline pass: a word object's width must be the sum of real glyph advances,
// not len*0.5em. Layout and paint must agree or words overlap their neighbors
// even when each word's own glyphs are spaced correctly.
func TestLayoutWordWidthFromMetrics(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(
		`<html><body style="margin: 0"><p>WWWW</p></body></html>`,
		nil, 400, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatal(err)
	}

	var word *layout.Object
	for i := range sess.Arena.Objects {
		o := &sess.Arena.Objects[i]
		if o.Node != nil && o.Node.Type == dom.NodeText && o.Node.DataContent == "WWWW" {
			word = o
		}
	}
	if word == nil {
		t.Fatal("word object for WWWW not found in arena")
	}
	want := float32(4*fonts.GlyphAdvanceFixed(16, 'W', frame.FontSlot{Family: frame.FontTimes})) / 64
	if diff := word.W - want; diff < -0.5 || diff > 0.5 {
		t.Errorf("word.W = %v, want %v (sum of real advances)", word.W, want)
	}
}

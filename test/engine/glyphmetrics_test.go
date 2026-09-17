package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// TestGlyphAdvancesFromMetrics pins the anti-ghosting contract: with a metrics
// source wired in, paint spaces glyphs by the font's real advance, not by the
// len*0.5em estimate. "WWWW" is the sharpest probe because W is one of the
// widest glyphs in Go Regular, so the estimate under-spaces it by nearly half
// and used to draw visibly overlapping glyphs.
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
	want := fonts.GlyphAdvance(16, 'W')
	if want <= 0 {
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
		found = true
		for i := 1; i < len(g); i++ {
			if got := g[i].X - g[i-1].X; got != want {
				t.Errorf("glyph %d advance = %d, want %d (real font advance)", i, got, want)
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
	want := float32(fonts.GlyphAdvance(16, 'W')) * 4
	if diff := word.W - want; diff < -0.5 || diff > 0.5 {
		t.Errorf("word.W = %v, want %v (sum of real advances)", word.W, want)
	}
}

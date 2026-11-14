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

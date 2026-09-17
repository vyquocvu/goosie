package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// A one-glyph run must claim ink width, not one pixel: tile selection drops any
// command whose bounds miss the tile, so an under-reported first glyph loses its
// ink in every tile right of its origin pixel.
func TestTextRunBoundsCoverSingleGlyph(t *testing.T) {
	run := paint.TextRun{
		Glyphs: []paint.GlyphRun{{Rune: 0x2014, X: 240, Y: 100, Size: 32}},
		Color:  frame.RGB(0, 0, 0),
	}
	r := run.Bounds(1)
	if r.X1 < 240+32 {
		t.Fatalf("single-glyph bounds X1 = %d, want at least %d to cover the glyph's ink", r.X1, 240+32)
	}
}

func TestDisplayCmdTextBoundsCoverSingleGlyph(t *testing.T) {
	c := paint.DisplayCmd{
		Kind: paint.CmdText,
		Text: paint.TextRun{
			Glyphs: []paint.GlyphRun{{Rune: '8', X: 2036, Y: 253, Size: 32}},
			Color:  frame.RGB(0, 0, 0),
		},
	}
	b, ok := c.Bounds()
	if !ok {
		t.Fatal("Bounds() = false, want true")
	}
	if b.X1 < 2036+32 {
		t.Fatalf("text command bounds X1 = %d, want at least %d", b.X1, 2036+32)
	}
}

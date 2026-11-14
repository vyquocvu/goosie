package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestRasterizeTileGlyphInkCrossingTileEdge(t *testing.T) {
	f, g := mustFonts(t)
	dl := build(t, paint.DisplayCmd{
		Kind: paint.CmdText,
		Text: paint.TextRun{
			Glyphs: []paint.GlyphRun{{Rune: 0x2014, X: 240, Y: 100, Size: 32}},
			Color:  frame.RGB(0, 0, 0),
		},
	})

	left := newTile()
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), left, f, g, frame.TransparentBlack); err != nil {
		t.Fatalf("left tile: %v", err)
	}
	if inkPixels(left) == 0 {
		t.Fatal("left tile has no dash ink, want the dash's first pixels")
	}

	right := newTile()
	if err := raster.RasterizeTile(dl, frame.Rect4(256, 0, 512, 256), right, f, g, frame.TransparentBlack); err != nil {
		t.Fatalf("right tile: %v", err)
	}
	if inkPixels(right) == 0 {
		t.Fatal("right tile has no dash ink: the glyph's ink crossing x=256 was clipped")
	}
}

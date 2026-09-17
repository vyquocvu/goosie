package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// A glyph whose ink straddles a tile boundary must appear in both tiles. Tile
// selection consults the command's bounds, so an under-reported run loses the
// part of the glyph right of its origin pixel — the visible form of this bug
// was an em dash rendered at half width.
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
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), left, f, g); err != nil {
		t.Fatalf("left tile: %v", err)
	}
	if inkPixels(left) == 0 {
		t.Fatal("left tile has no dash ink, want the dash's first pixels")
	}

	right := newTile()
	if err := raster.RasterizeTile(dl, frame.Rect4(256, 0, 512, 256), right, f, g); err != nil {
		t.Fatalf("right tile: %v", err)
	}
	if inkPixels(right) == 0 {
		t.Fatal("right tile has no dash ink: the glyph's ink crossing x=256 was clipped")
	}
}

package raster

import (
	"hash/fnv"
	"image"
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

func newTile() *frame.Bitmap { return frame.NewBitmap(int(frame.TileSize), int(frame.TileSize)) }

func build(t *testing.T, cmds ...paint.DisplayCmd) *paint.LayerDL {
	t.Helper()
	l := paint.NewList(len(cmds))
	for _, c := range cmds {
		l.Append(c)
	}
	return l.Build(1).Publish()
}

func mustFonts(t *testing.T) (*raster.Fonts, *raster.GlyphAtlas) {
	t.Helper()
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	return f, raster.NewGlyphAtlas(1<<20, f)
}

func hashTile(b *frame.Bitmap) uint64 {
	h := fnv.New64a()
	h.Write(b.RGBA)
	return h.Sum64()
}

func inkPixels(b *frame.Bitmap) int {
	n := 0
	for i := 3; i < len(b.RGBA); i += 4 {
		if b.RGBA[i] != 0 {
			n++
		}
	}
	return n
}

func TestRasterizeTileFillInside(t *testing.T) {
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(10, 20, 30, 40),
		Color: frame.RGB(255, 0, 0),
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(10, 20); got != frame.RGB(255, 0, 0) {
		t.Fatalf("top-left of fill = %v, want red", got)
	}
	if got := out.At(29, 39); got != frame.RGB(255, 0, 0) {
		t.Fatalf("bottom-right of fill = %v, want red", got)
	}
	if got := out.At(30, 40); got != frame.TransparentBlack {
		t.Fatalf("one past the fill = %v, want untouched", got)
	}
	if got := out.At(9, 20); got != frame.TransparentBlack {
		t.Fatalf("one before the fill = %v, want untouched", got)
	}
}

func TestRasterizeTileFillOutsideWritesNothing(t *testing.T) {
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(1000, 1000, 1100, 1100),
		Color: frame.RGB(0, 255, 0),
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if n := inkPixels(out); n != 0 {
		t.Fatalf("a command a full tile away painted %d pixels", n)
	}
}

func TestRasterizeTileFillAtTileEdgeIsClipped(t *testing.T) {
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(250, 250, 400, 400),
		Color: frame.RGB(0, 0, 255),
	})
	// The tile covers content [0,256); the command covers [250,400). Only the
	// 6x6 corner belongs to this tile, and the rest must land in the neighbours.
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(255, 255); got != frame.RGB(0, 0, 255) {
		t.Fatalf("tile corner = %v, want blue", got)
	}
	if got := out.At(249, 255); got != frame.TransparentBlack {
		t.Fatalf("pixel left of the fill = %v, want untouched", got)
	}
}

func TestRasterizeTileNegativeOrigin(t *testing.T) {
	out := newTile()
	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(-10, -10, 10, 10),
		Color: frame.RGB(9, 9, 9),
	})
	// The tile spans content [-256, 0); the command's right half is inside it.
	if err := raster.RasterizeTile(dl, frame.Rect4(-256, -256, 0, 0), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(255, 255); got != frame.RGB(9, 9, 9) {
		t.Fatalf("bottom-right corner = %v, want the fill color", got)
	}
	if got := out.At(246, 246); got != frame.RGB(9, 9, 9) {
		t.Fatalf("just inside the fill = %v, want the fill color", got)
	}
	if got := out.At(245, 245); got != frame.TransparentBlack {
		t.Fatalf("just outside the fill = %v, want untouched", got)
	}
}

func TestRasterizeTileBorderPerSideWidths(t *testing.T) {
	out := newTile()
	red := frame.RGB(255, 0, 0)
	blue := frame.RGB(0, 0, 255)
	dl := build(t, paint.DisplayCmd{
		Kind: paint.CmdBorder,
		Rect: frame.Rect4(16, 16, 116, 116),
		Border: paint.BorderSpec{
			Top:    paint.SideSpec{Width: 4, Color: red},
			Left:   paint.SideSpec{Width: 2, Color: blue},
			Bottom: paint.SideSpec{Width: 10, Color: red},
		},
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	checks := []struct {
		x, y int
		want frame.Color
		what string
	}{
		{20, 16, red, "top strip"},
		{20, 19, red, "top strip's last row"},
		{20, 20, frame.TransparentBlack, "just inside the top strip"},
		{16, 20, blue, "left strip"},
		{17, 20, blue, "left strip's last column"},
		{18, 22, frame.TransparentBlack, "inside the box"},
		{20, 115, red, "bottom strip"},
		{20, 106, red, "bottom strip's inner edge"},
		{20, 105, frame.TransparentBlack, "just above the bottom strip"},
	}
	for _, c := range checks {
		if got := out.At(c.x, c.y); got != c.want {
			t.Fatalf("%s at (%d,%d) = %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

func TestRasterizeTileTextDrawsInk(t *testing.T) {
	out := newTile()
	f, g := mustFonts(t)
	dl := build(t, textCmd("Goosie", 40, 60, 16))
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if n := inkPixels(out); n == 0 {
		t.Fatal("text produced no pixels at all")
	}
}

func TestRasterizeTileTextIsDeterministicAcrossWorkers(t *testing.T) {
	f, g := mustFonts(t)
	dl := build(t, textCmd("Goosie", 40, 60, 16))

	render := func() uint64 {
		out := newTile()
		if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
			t.Fatalf("RasterizeTile: %v", err)
		}
		return hashTile(out)
	}
	want := render()
	// A shared opentype.Face reuses scratch buffers between calls, so any race
	// between concurrent rasters shows up here as a different hash long before it
	// shows up on screen.
	for _, workers := range []int{1, 2, 8} {
		results := make(chan uint64, workers)
		for i := 0; i < workers; i++ {
			go func() { results <- render() }()
		}
		for i := 0; i < workers; i++ {
			if got := <-results; got != want {
				t.Fatalf("hash with %d workers = %d, want %d: glyph raster is order-dependent",
					workers, got, want)
			}
		}
	}
}

func TestRasterizeTileGlyphPlacedRelativeToPen(t *testing.T) {
	f, g := mustFonts(t)
	at := func(x, y int32) uint64 {
		out := newTile()
		dl := build(t, textCmd("A", x, y, 16))
		if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
			t.Fatalf("RasterizeTile: %v", err)
		}
		return hashTile(out)
	}
	if at(40, 60) == at(80, 60) {
		t.Fatal("moving the pen changed nothing: glyphs are not positioned per run")
	}
}

func TestRasterizeTileImageScalesIntoRect(t *testing.T) {
	out := newTile()
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	// One green texel so the scaling is observable rather than a solid fill.
	src.Set(0, 0, color.RGBA{G: 255, A: 255})

	dl := build(t, paint.DisplayCmd{
		Kind:  paint.CmdImage,
		Rect:  frame.Rect4(8, 8, 40, 40),
		Image: paint.ImageSpec{Src: src},
	})
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(8, 8); got.G() < 150 || got.G() <= got.R() {
		t.Fatalf("top-left texel = %v, want the green source pixel to dominate here", got)
	}
	if got := out.At(39, 39); got != frame.RGB(255, 0, 0) {
		t.Fatalf("bottom-right = %v, want red", got)
	}
	// Nothing may escape the destination rect.
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			if x >= 8 && x < 40 && y >= 8 && y < 40 {
				continue
			}
			if got := out.At(x, y); got != frame.TransparentBlack {
				t.Fatalf("image wrote outside its rect at (%d,%d): %v", x, y, got)
			}
		}
	}
}

func TestRasterizeTileZeroAllocAfterWarmup(t *testing.T) {
	f, g := mustFonts(t)
	dl := build(t,
		paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(0, 0, 256, 256), Color: frame.RGB(255, 255, 255)},
		paint.DisplayCmd{Kind: paint.CmdBorder, Rect: frame.Rect4(20, 20, 220, 120),
			Border: paint.BorderSpec{Top: paint.SideSpec{Width: 3, Color: frame.RGB(0, 0, 0)}}},
		textCmd("The quick brown fox jumps over the lazy dog", 30, 60, 16),
		textCmd("Pack my box with five dozen liquor jugs", 30, 90, 16),
	)
	out := newTile()
	// Warm every glyph in the run into the atlas, then measure.
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile warmup: %v", err)
	}
	if inkPixels(out) == 0 {
		t.Fatal("the warmup pass drew nothing; the measurement below would be meaningless")
	}
	if n := testing.AllocsPerRun(500, func() {
		if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
			t.Errorf("RasterizeTile: %v", err)
		}
	}); n != 0 {
		t.Fatalf("a warm tile raster allocated %v times per frame; invariant 6 wants 0", n)
	}
}

func TestGlyphCacheReusesFaceAcrossDPR(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	g := raster.NewGlyphAtlas(1<<20, f)
	// GlyphRun.Size is already a device-pixel size, so a 16px glyph at DPR 1 and
	// a 16px glyph at DPR 2 are the same bitmap and must be rasterized once.
	first := g.Get('a', 16, frame.FontSlot{})
	second := g.Get('a', 16, frame.FontSlot{})
	if !first.Ok || !second.Ok {
		t.Fatal("glyph lookup failed")
	}
	if first.Mask != second.Mask {
		t.Fatal("the same device size re-rasterized; the face is not shared across DPR")
	}
	if got := g.Stats().Misses; got != 1 {
		t.Fatalf("Misses = %d, want 1 for two lookups of one glyph at one size", got)
	}
	// A different device size is a different bitmap, not a rescale.
	if big := g.Get('a', 32, frame.FontSlot{}); big.Mask == first.Mask {
		t.Fatal("32px reused the 16px mask; glyphs must be rasterized at their own size")
	}
}

func TestGlyphAtlasEvictsToBudget(t *testing.T) {
	f, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("NewFonts: %v", err)
	}
	one := f.Glyph(16, 'a', frame.FontSlot{}).Bytes()
	if one == 0 {
		t.Fatal("a 16px glyph mask is empty; the budget below would not bind")
	}
	g := raster.NewGlyphAtlas(one*2, f)
	for _, r := range []rune{'a', 'b', 'c', 'd', 'e'} {
		g.Get(r, 16, frame.FontSlot{})
	}
	st := g.Stats()
	if st.Bytes > st.Budget {
		t.Fatalf("Bytes = %d over Budget = %d", st.Bytes, st.Budget)
	}
	if st.Evictions == 0 {
		t.Fatal("no evictions with a two-glyph budget and five glyphs")
	}
}

func TestRasterizeTileNilDestination(t *testing.T) {
	if err := raster.RasterizeTile(nil, frame.Rect4(0, 0, 256, 256), nil, nil, nil, frame.TransparentBlack); err == nil {
		t.Fatal("nil destination accepted")
	}
}

func TestRasterizeTileEmptyListClears(t *testing.T) {
	out := frame.NewBitmap(16, 16)
	out.FillRect(out.Bounds(), frame.RGB(1, 2, 3), nil)
	dl := build(t)
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 16, 16), out, nil, nil, frame.TransparentBlack); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if n := inkPixels(out); n != 0 {
		t.Fatalf("an empty tile left %d pixels of the previous content", n)
	}
}

func TestRasterizeTileConcurrentSameAtlas(t *testing.T) {
	f, g := mustFonts(t)
	dl := build(t, textCmd("Concurrent rasters must agree", 10, 30, 16))
	const n = 8
	ch := make(chan uint64, n)
	for i := 0; i < n; i++ {
		go func() {
			out := newTile()
			if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 256, 256), out, f, g, frame.TransparentBlack); err != nil {
				t.Errorf("RasterizeTile: %v", err)
			}
			ch <- hashTile(out)
		}()
	}
	want := <-ch
	for i := 1; i < n; i++ {
		if got := <-ch; got != want {
			t.Fatalf("concurrent rasters disagree: %d vs %d", got, want)
		}
	}
}

func textCmd(s string, x, y, size int32) paint.DisplayCmd {
	glyphs := make([]paint.GlyphRun, 0, len(s))
	pen := x
	for _, r := range s {
		glyphs = append(glyphs, paint.GlyphRun{Rune: r, X: pen, Y: y, Size: size})
		pen += size / 2 // a fixed advance: the producer owns positioning
	}
	return paint.DisplayCmd{
		Kind: paint.CmdText,
		Text: paint.TextRun{Glyphs: glyphs, Color: frame.RGB(0, 0, 0)},
	}
}

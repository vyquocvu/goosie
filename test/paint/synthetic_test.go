package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// The synthetic scene is the M1 gate's page: a document with no DOM, no CSS, and no
// layout pass behind it. Its only job is to contain enough tiles, enough commands per
// tile, and enough distinct glyphs that a frame path cannot pass the gate by doing
// nothing. These tests are the reason it can stand in for real content, so they
// assert the properties the gate's claims rest on rather than how a pixel looks.

// gateSpec is the document the M1 criteria are stated against: wide enough for a
// 1440x900 viewport at DPR 2 and tall enough for 600 frames of a 100px scroll.
func gateSpec() paint.SceneSpec {
	return paint.SceneSpec{
		Cols:         12,
		DocHeight:    600*100 + 1800,
		TextRuns:     1200,
		Seed:         7,
		Checkerboard: true,
		BudgetTiles:  256,
	}
}

// TestSceneDeterministic is what makes a golden meaningful: the same spec has to
// produce the same commands, glyph for glyph, or a hash comparison is comparing
// noise. A scene whose content came from the wall clock or from map iteration order
// would pass every other test in this file and still be useless as a baseline.
func TestSceneDeterministic(t *testing.T) {
	spec := gateSpec()
	a, layerA := paint.BuildLayer(spec)
	b, layerB := paint.BuildLayer(spec)

	if got, want := paint.SceneHash(a), paint.SceneHash(b); got != want {
		t.Fatalf("two builds of one spec hashed %016x and %016x", got, want)
	}
	if len(a.All()) != len(b.All()) {
		t.Fatalf("two builds of one spec produced %d and %d commands", len(a.All()), len(b.All()))
	}
	if layerA.Bounds != layerB.Bounds {
		t.Fatalf("two builds of one spec produced bounds %v and %v", layerA.Bounds, layerB.Bounds)
	}

	// The seed is the only thing that may change the content, and a hash that ignored
	// it would be a hash of the geometry rather than of the scene.
	other := spec
	other.Seed++
	if paint.SceneHash(a) == paint.SceneHash(buildOf(t, other)) {
		t.Fatal("changing Seed changed nothing: the scene's content is not seed-derived")
	}

	// Nor may two specs that differ only in size collide, since the gate compares
	// hashes across runs whose documents differ deliberately.
	taller := spec
	taller.DocHeight += 4 * frame.TileSize
	if paint.SceneHash(a) == paint.SceneHash(buildOf(t, taller)) {
		t.Fatal("a taller document hashed identically")
	}

	fewer := spec
	fewer.TextRuns = 600
	if paint.SceneHash(a) == paint.SceneHash(buildOf(t, fewer)) {
		t.Fatal("half the text runs hashed identically")
	}

	plain := spec
	plain.Checkerboard = false
	if paint.SceneHash(a) == paint.SceneHash(buildOf(t, plain)) {
		t.Fatal("Checkerboard off hashed identically to it on")
	}
}

func buildOf(t *testing.T, spec paint.SceneSpec) *paint.LayerDL {
	t.Helper()
	dl, _ := paint.BuildLayer(spec)
	return dl
}

// TestSceneBoundsCoverGrid is the plan's name for the one property a tile-based
// rasterizer needs of its test document: nothing may be painted outside the extent
// the tile grid was built over. A command that leaked past the bounds would be
// silently unclipped by no one - the grid never makes a tile for it, so it would
// simply never appear, and a gate that measured "did the tiles get drawn" would be
// measuring a document with a hole in it.
func TestSceneBoundsCoverGrid(t *testing.T) {
	for _, name := range []string{"gate spec", "zero spec"} {
		spec := gateSpec()
		if name == "zero spec" {
			spec = paint.SceneSpec{}
		}
		t.Run(name, func(t *testing.T) {
			dl, l := paint.BuildLayer(spec)
			if l.Bounds.W() <= 0 || l.Bounds.H() <= 0 {
				t.Fatalf("layer bounds %v are degenerate", l.Bounds)
			}
			if got := l.Grid.Extent(); got != l.Bounds {
				t.Fatalf("grid extent %v does not cover the layer bounds %v", got, l.Bounds)
			}
			if got := dl.Extent(); !l.Bounds.Canon().Contains(got.Origin()) || !l.Bounds.Canon().Contains(frame.Point{X: got.X1 - 1, Y: got.Y1 - 1}) {
				t.Fatalf("display list extent %v is not inside %v", got, l.Bounds)
			}
			for i, c := range dl.All() {
				b, ok := c.Bounds()
				if !ok {
					// A command with no bounds paints nothing, so it cannot stick out. It is
					// still worth knowing that the generator emits one: an empty text run is
					// how a layout bug would look here.
					t.Errorf("command %d (%v) bounds to nothing", i, c.Kind)
					continue
				}
				if !l.Bounds.Canon().Contains(b.Origin()) || !l.Bounds.Canon().Contains(frame.Point{X: b.X1 - 1, Y: b.Y1 - 1}) {
					t.Fatalf("command %d (%v) bounds %v lie outside the layer %v", i, c.Kind, b, l.Bounds)
				}
				if b.W() > l.Bounds.W() || b.H() > l.Bounds.H() {
					t.Fatalf("command %d (%v) bounds %v exceed the layer in one axis", i, c.Kind, b)
				}
			}
		})
	}
}

// TestSceneCarriesTheContentTheGateDependsOn states, as counts, what the synthetic
// page is claimed to contain. Each of these is a different way the frame path could
// cheat: no text means no glyph atlas, one text size means one atlas row, no image
// means the scaler is never exercised, no borders means a tile costs one memset.
func TestSceneCarriesTheContentTheGateDependsOn(t *testing.T) {
	dl, l := paint.BuildLayer(gateSpec())
	if !dl.Frozen() {
		t.Fatal("BuildLayer handed back an unfrozen list; a worker would be reading it while a builder could still append")
	}
	if dl.Version() != 1 {
		t.Fatalf("Version() = %d, want the scene's first content version to be 1", dl.Version())
	}

	var fills, borders, texts, images, glyphs int
	var sizes = map[int32]bool{}
	var colors = map[frame.Color]bool{}
	for _, c := range dl.All() {
		switch c.Kind {
		case paint.CmdFill:
			fills++
			colors[c.Color] = true
		case paint.CmdBorder:
			borders++
		case paint.CmdText:
			texts++
			glyphs += len(c.Text.Glyphs)
			for _, g := range c.Text.Glyphs {
				sizes[g.Size] = true
				if g.Rune == 0 {
					t.Fatal("a glyph run carried a zero rune")
				}
			}
		case paint.CmdImage:
			images++
			if c.Image.Src == nil {
				t.Fatal("an image command carried no source")
			}
			if c.Image.Src.Bounds().Empty() {
				t.Fatal("the fake image has no pixels")
			}
		}
	}
	if texts < 1200 {
		t.Fatalf("%d text runs, want at least 1,200: the glyph atlas is the point", texts)
	}
	if len(sizes) < 3 {
		t.Fatalf("text appears at %d sizes, want at least 3 so the atlas holds more than one face", len(sizes))
	}
	if glyphs/texts < 4 {
		t.Fatalf("%d glyphs across %d runs: a run of one or two glyphs is not a text layout", glyphs, texts)
	}
	if images == 0 {
		t.Fatal("no image command: the rasterizer's scaler would never run")
	}
	if borders == 0 {
		t.Fatal("no borders: every tile would cost one fill")
	}
	if len(colors) < 2 {
		t.Fatalf("Checkerboard on produced %d fill colors, want the cells to alternate", len(colors))
	}

	// The budget is the layer's, in the units the memory criterion is stated in.
	if got, want := l.Grid.Stats().Budget, int64(256)*frame.TileSizeBytes(); got != want {
		t.Fatalf("Budget = %d, want %d (256 tiles)", got, want)
	}
}

// TestSceneFlatCellsAreFlat checks the other half of the Checkerboard flag: it
// changes colour, not structure, so a gate can compare command counts across it.
func TestSceneFlatCellsAreFlat(t *testing.T) {
	checker := buildOf(t, gateSpec())
	plain := buildOf(t, func() paint.SceneSpec {
		s := gateSpec()
		s.Checkerboard = false
		return s
	}())
	if len(checker.All()) != len(plain.All()) {
		t.Fatalf("Checkerboard changed the command count: %d vs %d", len(checker.All()), len(plain.All()))
	}
	seen := map[frame.Color]bool{}
	for _, c := range plain.All() {
		if c.Kind == paint.CmdFill {
			seen[c.Color] = true
		}
	}
	if len(seen) > 2 { // the page background and one cell colour, at most
		t.Fatalf("Checkerboard off still produced %d distinct fill colours", len(seen))
	}
}

// TestSceneDefaultsReachTheGatesScrollDistance is why the defaults are the defaults:
// a caller that writes SceneSpec{} and scrolls 600 times must never hit the clamp,
// because a frame path that has stopped moving is a frame path that stopped being
// measured.
func TestSceneDefaultsReachTheGatesScrollDistance(t *testing.T) {
	dl, l := paint.BuildLayer(paint.SceneSpec{})
	if dl.Len() == 0 {
		t.Fatal("the zero-value spec built an empty document")
	}
	const scrollDistance = 600 * 100
	if got := l.Bounds.H(); got < scrollDistance {
		t.Fatalf("default document is %dpx tall, want at least the gate's %dpx of scrolling", got, scrollDistance)
	}
	if got := l.Bounds.W(); got < 12*frame.TileSize {
		t.Fatalf("default document is %dpx wide, want at least %d to cover a 1440px viewport at DPR 2",
			got, 12*frame.TileSize)
	}
	if got := l.Grid.Stats().Budget; got < int64(96)*frame.TileSizeBytes() {
		t.Fatalf("default budget is %d tiles, want room for a 12x8 tile viewport", got/frame.TileSizeBytes())
	}
	// Rows follow DocHeight, so the two cannot disagree about how tall the document is.
	tall := paint.SceneSpec{Rows: 4}
	if _, l := paint.BuildLayer(tall); l.Bounds.H() != 4*frame.TileSize {
		t.Fatalf("Rows:4 gave a %dpx document, want %d", l.Bounds.H(), 4*frame.TileSize)
	}
	if _, l := paint.BuildLayer(paint.SceneSpec{Rows: 4, DocHeight: 3 * frame.TileSize}); l.Bounds.H() != 3*frame.TileSize {
		t.Fatal("DocHeight did not win over Rows")
	}
}

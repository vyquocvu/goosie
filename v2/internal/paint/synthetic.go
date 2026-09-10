package paint

import (
	"hash/fnv"
	"image"
	"math"
	"math/rand"

	"github.com/vyquocvu/goosie/v2/internal/frame"
)

// The synthetic scene is the frame path's test document: a page with no DOM, no CSS,
// and no layout pass behind it. M1's criteria are about tiles, budgets, versions, and
// pacing, and every one of them is measurable against a page whose contents are known
// analytically. Building it from a seed rather than from a fixture file is what lets a
// golden hash mean something two years from now on a machine with no fonts installed.
//
// It lives in paint because a scene *is* a display list. Nothing here knows about
// tiles, threads, or windows; the grid that caches these commands and the workers that
// rasterize them are the layers above.

// SceneCellSize is the edge of one checkerboard cell, in device pixels. It equals the
// tile edge on purpose: the cell grid and the tile grid are then the same grid, so a
// test that says "one tile of content" says "one cell of content" and cannot drift.
const SceneCellSize = frame.TileSize

const (
	// DefaultCols is wide enough for a 1440 CSS-pixel viewport at DPR 2.
	DefaultCols int32 = 12
	// DefaultDocHeight is the gate's scroll distance - 600 frames of 100px - plus two
	// screens, so a measured run never presses against the end of the document.
	DefaultDocHeight int32 = 600*100 + 2*1800
	// DefaultTextRuns is the spec's glyph-atlas stress: enough positioned runs that the
	// atlas holds hundreds of distinct (rune, size) entries.
	DefaultTextRuns int32 = 1200
	// DefaultBudgetTiles is the layer's tile budget in buffers, 16 MiB at TileSize,
	// which is room for a 12x8 tile viewport plus several screens of prefetch.
	DefaultBudgetTiles int64 = 256
	// SceneVersion is the content version a freshly built scene depicts. It is 1 rather
	// than 0 because 0 is the frame path's "no version yet".
	SceneVersion uint64 = 1

	// sceneLayerID identifies the one layer a scene owns.
	sceneLayerID = frame.LayerID(1)
	// sceneMargin keeps the outermost commands off the document's edge, so a bounds
	// assertion is never decided by a half pixel.
	sceneMargin = int32(8)
	// sceneMinGlyphs and sceneMaxGlyphExtra bound a word's glyph count. Four is the
	// floor because a run of one or two glyphs is not a text layout.
	sceneMinGlyphs     = int32(4)
	sceneMaxGlyphExtra = int32(12)
)

// textSizes are the device-pixel sizes the scene draws glyphs at. Five of them is the
// point: a page with one font size exercises one atlas row, and an atlas that only
// ever sees one row is an atlas nobody has tested.
var textSizes = []int32{12, 16, 20, 24, 32}

// sceneLinePitch is the baseline-to-baseline distance: the largest size plus room for a
// descender, so no two lines of the scene overlap and every glyph box is distinct.
const sceneLinePitch = int32(40)

// textGlyphs is the glyph pool. It is mostly ASCII because Go Regular covers it, and
// the handful of accented and non-Latin code points are there to put more than a
// hundred distinct entries into the atlas rather than a hundred in one block. It is a
// []rune rather than a string because the non-ASCII entries are multi-byte and an index
// into the string would put raw UTF-8 bytes into a GlyphRun.
var textGlyphs = []rune("abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.,;:'-!?()éüßΩ£")

// SceneSpec describes a synthetic document. Every field has a usable zero value, and
// no field can produce an invalid scene: out-of-range values are clamped rather than
// rejected, because the caller is a test or a benchmark that wants a page, not an
// error to handle.
type SceneSpec struct {
	// Cols is how many cells wide the document is. A cell is SceneCellSize across, so
	// Cols is also the document's tile-column count. Zero means DefaultCols.
	Cols int32
	// Rows is how many cells tall the document is, when DocHeight is not given. Zero
	// means the document is DefaultDocHeight tall.
	Rows int32
	// DocHeight, when positive, is the document height in device pixels and wins over
	// Rows, rounding up to a whole number of cells. It is the field a gate sets when it
	// needs a specific scroll distance.
	DocHeight int32
	// TextRuns is how many positioned runs to lay out, up to what the document has room
	// for. Zero means DefaultTextRuns.
	TextRuns int32
	// Seed makes the scene reproducible. It is a seed, not a flag: 0 is a valid one and
	// hashes like any other.
	Seed int64
	// Checkerboard alternates two derived cell colours by cell parity. When false every
	// cell gets one colour, and the command count is unchanged.
	Checkerboard bool
	// BudgetTiles is the layer's tile-buffer budget, the knob the byte-budget criteria
	// are stated in. Zero or less means DefaultBudgetTiles.
	BudgetTiles int64
	// PreallocTiles is how many tile buffers the layer's pool creates up front instead of
	// on first acquire. Zero - the default - means none, which is right for a page: the
	// pool fills as the document is scrolled. A gate that intends to measure a cold
	// document without the pool's first-touch allocations in the numbers sets it.
	PreallocTiles int32
}

func (s SceneSpec) cols() int32 {
	if s.Cols > 0 {
		return s.Cols
	}
	return DefaultCols
}

// height returns the document height in device pixels, rounded up to whole cells so
// the last row of cells is never clipped to a stub.
func (s SceneSpec) height() int32 {
	h := s.DocHeight
	if h <= 0 {
		if s.Rows > 0 {
			h = s.Rows * SceneCellSize
		} else {
			h = DefaultDocHeight
		}
	}
	rows := (h + SceneCellSize - 1) / SceneCellSize
	if rows < 1 {
		rows = 1
	}
	return rows * SceneCellSize
}

func (s SceneSpec) rows() int32 { return s.height() / SceneCellSize }

func (s SceneSpec) runs() int32 {
	if s.TextRuns > 0 {
		return s.TextRuns
	}
	return DefaultTextRuns
}

// budgetTiles returns the layer's tile-buffer budget as a whole number of buffers,
// rounded up and never below one, since a grid that cannot hold a tile is raised to
// one tile by NewGrid anyway.
func (s SceneSpec) budgetTiles() int64 {
	n := s.BudgetTiles
	if n <= 0 {
		n = DefaultBudgetTiles
	}
	return n
}

// budget returns the layer's tile byte budget, which the grid clamps up to at least
// one tile.
func (s SceneSpec) budget() int64 {
	return s.budgetTiles() * frame.TileSizeBytes()
}

// BuildLayer returns a frozen display list and the layer that caches its tiles.
//
// The list is in document order - top to bottom, and within one row of cells the cells
// first and then the text that sits on that row. That ordering is load bearing rather
// than cosmetic: LayerDL.Intersecting returns a contiguous span of the list, so a list
// in document order lets a tile walk a slice of the page, while a list with all the
// text gathered at the end would make every tile in the lower half scan the whole
// document and the frame path would be measured against a page nobody renders that way.
//
// The scene has no stacking contexts, so every command is Z 0 and list order is paint
// order. Z exists for a producer that resolves CSS z-index; a flat page must not sort.
//
// No command reaches outside the layer bounds. A scene that did would be a scene whose
// overflow is silently never drawn, and the frame path would be measured against a
// document with a hole in it.
func BuildLayer(spec SceneSpec) (*LayerDL, *frame.Layer) {
	b := &sceneBuilder{
		rng:   rand.New(rand.NewSource(spec.Seed)),
		w:     spec.cols() * SceneCellSize,
		h:     spec.height(),
		rows:  spec.rows(),
		cols:  spec.cols(),
		limit: spec.runs(),
		page:  frame.RGB(248, 248, 248),
	}
	b.cellA, b.cellB = b.color(), b.color()
	if !spec.Checkerboard {
		b.cellB = b.cellA
	}
	b.list = NewList(int(b.rows*b.cols)*2 + int(b.limit)*3 + 16)
	b.list.Append(fill(frame.Rect4(0, 0, b.w, b.h), b.page))

	text := &textFlow{b: b, x: sceneMargin, y: sceneMargin + textSizes[len(textSizes)-1]}
	imgRow := b.rows / 3
	for row := int32(0); row < b.rows; row++ {
		for col := int32(0); col < b.cols; col++ {
			box := frame.Rect4(col*SceneCellSize, row*SceneCellSize,
				(col+1)*SceneCellSize, (row+1)*SceneCellSize)
			color := b.cellA
			if spec.Checkerboard && (col+row)%2 == 1 {
				color = b.cellB
			}
			b.list.Append(fill(box, color))
			b.list.Append(cellBorder(box, b))
		}
		if row == imgRow {
			b.list.Append(b.image(imgRow))
		}
		for text.liveAbove((row + 1) * SceneCellSize) {
			b.list.Append(text.take())
		}
	}
	dl := b.list.Build(SceneVersion).Publish()

	// The pool's free list has to be able to hold every buffer the layer's budget can
	// pay for. A list that is shorter than the budget drops the releases it cannot keep,
	// and the next acquire then allocates - which breaks invariant 6 through a constant
	// nobody intended as a limit. maxIdle is a ceiling on retention, not on creation, so
	// sizing it to the budget costs nothing until tiles actually exist.
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(spec.budgetTiles()))
	if n := spec.PreallocTiles; n > 0 {
		pool.Prealloc(int(n))
	}
	l := frame.NewLayer(sceneLayerID, frame.Rect4(0, 0, b.w, b.h), spec.budget(), pool)
	l.SetContent(dl)
	return dl, l
}

// sceneBuilder holds what the passes share: one random stream, the document size, and
// the list being filled. The single stream drives all of it in a fixed order, so the
// same seed always yields the same page.
type sceneBuilder struct {
	rng   *rand.Rand
	list  *List
	w, h  int32
	rows  int32
	cols  int32
	limit int32
	page  frame.Color
	cellA frame.Color
	cellB frame.Color
}

// color returns a deterministic opaque colour. Alpha stays at 255 because a scene whose
// colours are translucent would be testing blending rather than painting, and blending
// has its own tests.
func (b *sceneBuilder) color() frame.Color {
	return frame.RGB(uint8(32+b.intn(224)), uint8(32+b.intn(224)), uint8(32+b.intn(224)))
}

func (b *sceneBuilder) intn(n int32) int32 {
	if n <= 0 {
		return 0
	}
	return b.rng.Int31n(n)
}

func fill(r frame.Rect, c frame.Color) DisplayCmd {
	return DisplayCmd{Kind: CmdFill, Rect: r, Color: c}
}

// cellBorder gives a cell one to four edges of one to three pixels, with a colour of
// its own. A cell that costs one memset is a cell that tests nothing; the four clipped
// strips a border becomes are what make a tile raster do real work.
func cellBorder(r frame.Rect, b *sceneBuilder) DisplayCmd {
	width := func() int32 { return 1 + b.intn(3) }
	edge := func() SideSpec { return SideSpec{Width: width(), Color: b.color()} }
	return DisplayCmd{
		Kind:   CmdBorder,
		Rect:   r,
		Border: BorderSpec{Top: edge(), Right: edge(), Bottom: edge(), Left: edge()},
	}
}

// image draws a generated bitmap into a box in the document, scaled up from a much
// smaller source so the scaler is actually scaling rather than copying.
func (b *sceneBuilder) image(row int32) DisplayCmd {
	const srcW, srcH = 96, 64
	src := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			o := src.PixOffset(x, y)
			src.Pix[o], src.Pix[o+1], src.Pix[o+2], src.Pix[o+3] =
				uint8(x*3), uint8(y*4), uint8(b.intn(256)), 255
		}
	}
	x0 := sceneMargin
	y0 := row*SceneCellSize + sceneMargin
	w := min32(b.w-2*sceneMargin, srcW*4)
	h := min32(b.h-y0-sceneMargin, srcH*2)
	return DisplayCmd{
		Kind:  CmdImage,
		Rect:  frame.Rect4(x0, y0, x0+w, y0+h),
		Image: ImageSpec{Src: src, SrcBox: image.Rect(0, 0, srcW, srcH)},
	}
}

// textFlow places runs top to bottom and left to right, wrapping at the document's
// right edge and stepping down by a fixed line pitch. The pen starts one largest-size
// below the top margin, because a glyph box reaches its size above the baseline and the
// first line must still be inside the document.
//
// It stops when the document has no room left, which is why a scene's run count is "as
// many as were asked for, up to what fits" rather than a promise: the alternative is
// text clipped to nothing, and clipped text is the failure this generator would be
// least likely to notice. The gate's document fits a great deal more than it asks for.
type textFlow struct {
	b       *sceneBuilder
	x, y    int32
	placed  int32
	pending DisplayCmd
	have    bool
	done    bool
}

// arm places the next run into pending and reports whether one is ready. A run is
// produced whole or not at all, so no command is ever clipped by the document edge.
func (t *textFlow) arm() bool {
	if t.have || t.done {
		return t.have
	}
	if t.placed >= t.b.limit {
		t.done = true
		return false
	}
	const attempts = 8
	for range attempts {
		size := textSizes[t.b.intn(int32(len(textSizes)))]
		advance := size*2/3 + 1
		// A word can be no wider than the text column, and the box a command reports is
		// estimated as X+Size for its last glyph, so both are bounded here rather than
		// discovered later by a bounds check.
		column := t.b.w - 2*sceneMargin
		glyphs := min32(sceneMinGlyphs+t.b.intn(sceneMaxGlyphExtra), column/advance)
		if glyphs < sceneMinGlyphs {
			continue
		}
		width := (glyphs-1)*advance + size
		if t.x+width > t.b.w-sceneMargin {
			t.x = sceneMargin
			t.y += sceneLinePitch
		}
		if t.y > t.b.h-sceneMargin {
			break
		}
		run := TextRun{Color: t.b.color(), Glyphs: make([]GlyphRun, glyphs)}
		x := t.x
		for i := range glyphs {
			r := textGlyphs[t.b.intn(int32(len(textGlyphs)))]
			run.Glyphs[i] = GlyphRun{Rune: r, X: x, Y: t.y, Size: size}
			x += advance
		}
		t.pending = DisplayCmd{
			Kind: CmdText,
			Rect: frame.Rect4(t.x, t.y-size, t.x+width, t.y),
			Text: run,
		}
		t.x = x
		t.have = true
		return true
	}
	t.done = true
	return false
}

// liveAbove reports whether the next run lies above y, which is how the merge in
// BuildLayer keeps the list in document order. The whole run, baseline included, has to
// be above the boundary for it to belong to the row being emitted.
func (t *textFlow) liveAbove(y int32) bool {
	return t.arm() && t.pending.Rect.Y1 < y
}

// take hands over the run liveAbove just reported.
func (t *textFlow) take() DisplayCmd {
	cmd := t.pending
	t.pending = DisplayCmd{}
	t.have = false
	t.placed++
	return cmd
}

// min32 is the local form of the builtin: these arguments are int32 coordinates, and
// writing min(x, y) over int32 works on the toolchain v2 requires. It is spelled out
// because frame.min32 is unexported and paint must not reach into frame's internals.
func min32(a, b int32) int32 {
	if b < a {
		return b
	}
	return a
}

// SceneHash returns a hash of a frozen list's visible content: every command's kind,
// box, colour, and detail, plus sampled pixels of any image. Two builds of one spec
// hash the same, which is what makes a golden frame meaningful, and anything that would
// change what a user sees changes the hash.
//
// It is a build-time and test-time tool, not a frame-path one: a per-frame hash of a
// whole page would cost more than the frames it was meant to be checking.
func SceneHash(d *LayerDL) uint64 {
	if d == nil {
		return 0
	}
	h := fnv.New64a()
	var scratch [8]byte
	add := func(v uint64) {
		for i := range scratch {
			scratch[i] = byte(v >> (8 * i))
		}
		_, _ = h.Write(scratch[:])
	}
	add(uint64(d.Version()))
	add(uint64(d.Len()))
	for _, c := range d.All() {
		add(uint64(c.Kind))
		add(uint64(c.Z))
		add(uint64(math.Float32bits(c.Opacity)))
		add(uint64(c.Rect.X0))
		add(uint64(c.Rect.Y0))
		add(uint64(c.Rect.X1))
		add(uint64(c.Rect.Y1))
		add(uint64(c.Color))
		for _, s := range [4]SideSpec{c.Border.Top, c.Border.Right, c.Border.Bottom, c.Border.Left} {
			add(uint64(s.Width))
			add(uint64(s.Color))
		}
		add(uint64(c.Text.Color))
		add(uint64(len(c.Text.Glyphs)))
		for _, g := range c.Text.Glyphs {
			add(uint64(g.Rune))
			add(uint64(g.X))
			add(uint64(g.Y))
			add(uint64(g.Size))
		}
		if src := c.Image.Src; src != nil {
			box := src.Bounds()
			add(uint64(box.Dx()))
			add(uint64(box.Dy()))
			add(uint64(c.Image.SrcBox.Dx()))
			add(uint64(c.Image.SrcBox.Dy()))
			// Nine samples on a 3x3 grid: enough that a gradient which drifted with the
			// seed changes the hash, without reading every pixel of every image.
			for dy := 0; dy < 3; dy++ {
				for dx := 0; dx < 3; dx++ {
					r, g, b, a := src.At(box.Min.X+dx*box.Dx()/3, box.Min.Y+dy*box.Dy()/3).RGBA()
					add(uint64(r))
					add(uint64(g))
					add(uint64(b))
					add(uint64(a))
				}
			}
		}
	}
	return h.Sum64()
}

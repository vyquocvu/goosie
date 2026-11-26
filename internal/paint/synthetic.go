package paint

import (
	"hash/fnv"
	"image"
	"math"
	"math/rand"

	"github.com/vyquocvu/goosie/internal/frame"
)

const SceneCellSize = frame.TileSize

const (
	DefaultCols        int32  = 12
	DefaultDocHeight   int32  = 600*100 + 2*1800
	DefaultTextRuns    int32  = 1200
	DefaultBudgetTiles int64  = 256
	SceneVersion       uint64 = 1

	sceneLayerID       = frame.LayerID(1)
	sceneMargin        = int32(8)
	sceneMinGlyphs     = int32(4)
	sceneMaxGlyphExtra = int32(12)
)

var textSizes = []int32{12, 16, 20, 24, 32}

const sceneLinePitch = int32(40)

var textGlyphs = []rune("abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.,;:'-!?()éüßΩ£")

type SceneSpec struct {
	Cols          int32
	Rows          int32
	DocHeight     int32
	TextRuns      int32
	Seed          int64
	Checkerboard  bool
	BudgetTiles   int64
	PreallocTiles int32
}

func (s SceneSpec) cols() int32 {
	if s.Cols > 0 {
		return s.Cols
	}
	return DefaultCols
}

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

func (s SceneSpec) budgetTiles() int64 {
	n := s.BudgetTiles
	if n <= 0 {
		n = DefaultBudgetTiles
	}
	return n
}

func (s SceneSpec) budget() int64 {
	return s.budgetTiles() * frame.TileSizeBytes()
}

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

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, int(spec.budgetTiles()))
	if n := spec.PreallocTiles; n > 0 {
		pool.Prealloc(int(n))
	}
	l := frame.NewLayer(sceneLayerID, frame.Rect4(0, 0, b.w, b.h), spec.budget(), pool)
	l.SetContent(dl)
	return dl, l
}

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

func cellBorder(r frame.Rect, b *sceneBuilder) DisplayCmd {
	width := func() int32 { return 1 + b.intn(3) }
	edge := func() SideSpec { return SideSpec{Width: width(), Color: b.color()} }
	return DisplayCmd{
		Kind:   CmdBorder,
		Rect:   r,
		Border: BorderSpec{Top: edge(), Right: edge(), Bottom: edge(), Left: edge()},
	}
}

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

type textFlow struct {
	b       *sceneBuilder
	x, y    int32
	placed  int32
	pending DisplayCmd
	have    bool
	done    bool
}

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

func (t *textFlow) liveAbove(y int32) bool {
	return t.arm() && t.pending.Rect.Y1 < y
}

func (t *textFlow) take() DisplayCmd {
	cmd := t.pending
	t.pending = DisplayCmd{}
	t.have = false
	t.placed++
	return cmd
}

func min32(a, b int32) int32 {
	if b < a {
		return b
	}
	return a
}

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
		add(uint64(len(c.ZKey)))
		for i := 0; i < len(c.ZKey); i++ {
			add(uint64(c.ZKey[i]))
		}
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

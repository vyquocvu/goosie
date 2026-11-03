package raster

import (
	"context"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

// BR-074's frame-path half: an idle page must not move a pixel, and the pixels
// a scrolled page shows must not depend on how the viewport got there. The
// reference behaviour is measured by scripts/verify-br074-color-idle-scroll.js
// (Chromium 1.63, this host, 2026-11-04):
//
//	IDLE   three captures 300ms apart on an unattended page: byte-identical
//	SCROLL scrollY 160 reached in one jump, in eight steps of 20, and after
//	       going to 320 and back: three byte-identical captures; returning to
//	       the top reproduces the very first capture byte for byte
//
// and the colour of a document point is the same at every offset:
//
//	SCROLL-PIXELS at scrollY 0   tall@10px=rgb(1,168,0) tall@100px=rgb(10,159,0) tall@250px=rgb(26,143,0) tall@390px=rgb(41,128,0)
//	SCROLL-PIXELS at scrollY 160 the same four values
//
// This file drives the same path the shipped browser does - a real display list
// from Session.PaintChecked, the real tile rasterizer, the real worker pool, the
// damage-blit composer - because flicker is a property of that path, not of the
// painter alone: a scroll memmoves backing-store rows and reuses tiles, and
// either can leave a pixel drawn for a different offset or a different content
// version. The fixture is the one test/paint/color_idle_scroll_test.go samples
// against Chromium, so a pixel checked here is the same pixel scored there.

// flkFixture: flat fills, one faded group, a hard-stop band whose stop positions
// use CSS's unitless zero, and a 1600px ramp that spans every scroll offset
// these tests visit.
const flkFixture = `<!doctype html><html><head><style>
html,body{margin:0;padding:0;background:#fff}
.box{position:absolute}
#solid{left:10px;top:10px;width:100px;height:100px;background:#e7004c}
#backdrop{left:120px;top:10px;width:100px;height:100px;background:#00f}
#over{position:absolute;left:20px;top:20px;width:60px;height:60px;background:#f00;opacity:.5}
#hard{left:10px;top:130px;width:200px;height:40px;background:linear-gradient(#111 0 50%,#eee 50% 100%)}
#tall{left:0;top:200px;width:800px;height:1600px;background:linear-gradient(180deg,#0a0 0,#a00 100%)}
</style></head><body>
<div id="solid" class="box"></div>
<div id="backdrop" class="box"><div id="over"></div></div>
<div id="hard" class="box"></div>
<div id="tall" class="box"></div>
</body></html>`

const (
	flkVPH = 600
	flkVW  = 800
)

// flkPages is the page background the composer paints behind the layer - the
// same white the anchor's captures and test/paint's rasters start from.
var flkPages = frame.RGB(255, 255, 255)

// flkHarness is one frame path over a real page, driven the way surface.Loop
// drives its scheduler: BeginFrame, Submit, ComposeInto.
type flkHarness struct {
	t      *testing.T
	s      *raster.Scheduler
	l      *frame.Layer
	pool   *raster.Pool
	c      *surface.Composer
	fonts  *raster.Fonts
	damage []frame.Rect
}

func flkPage(t *testing.T, html string) *flkHarness {
	t.Helper()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("raster.NewFonts: %v", err)
	}
	sess, err := engine.NewSession(html, nil, flkVW, engine.WithMetrics(fonts), engine.WithViewportH(flkVPH))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	dl, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	content := dl.Build(1).Publish()
	// The layer starts at the document's own origin, so a viewport offset of y
	// puts document row y on backing row 0 and the two stay comparable.
	ext := content.Extent()
	budget := int64(64) * frame.TileSizeBytes()
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, 4096)
	l := frame.NewLayer(frame.LayerID(1), frame.Rect4(0, 0, ext.X1, ext.Y1), budget, pool)
	l.SetContent(content)

	wp := raster.New(2, 64, raster.DefaultRaster(fonts, raster.NewGlyphAtlas(4<<20, fonts)))
	wp.Start(context.Background())
	t.Cleanup(func() { _ = wp.Close() })

	size := frame.Size{W: flkVW, H: flkVPH}
	c := surface.NewComposer(size, frame.NewBitmapPool(size, 8))
	s := raster.NewScheduler(l, wp, frame.Viewport{Size: size}, 1, raster.Pref{})
	s.SetPlan(frame.FramePlan{Serial: 1, Layers: []*frame.Layer{l}, Background: flkPages})
	return &flkHarness{t: t, s: s, l: l, pool: wp, c: c, fonts: fonts}
}

// frame draws one vsync, scrolling by dy device pixels, and keeps the damage
// the composer reported for that frame.
func (h *flkHarness) frame(dy int32) surface.FrameWork {
	h.t.Helper()
	plan, needed := h.s.BeginFrame(surface.Event{Kind: surface.EvVsync,
		Delta: frame.Point{Y: dy}, At: time.Now()})
	if plan == nil {
		h.t.Fatal("BeginFrame returned no plan after SetPlan")
	}
	h.c.Resize(plan.Viewport.Size)
	work := h.s.Submit(needed)
	h.damage = h.s.ComposeInto(h.c.Backing, h.c, plan, needed)
	return work
}

// settle leaves a surface that actually shows the document. Submit only puts
// tiles in the grid; ComposeInto is what writes them onto the backing store, so
// a snapshot taken after a pump-only wait can be missing rows that settled
// happily in the cache. A frame that wants no tile and reports no damage is the
// frame an idle browser draws, so that is where settle stops.
func (h *flkHarness) settle() {
	h.t.Helper()
	if !pollUntil(func() bool {
		w := h.frame(0)
		return w.Needed == 0 && len(h.damage) == 0 && h.l.Grid.Stats().PaintingBytes == 0
	}) {
		h.t.Fatalf("the surface never settled and composited: layer %+v", h.l.Stats())
	}
}

// scrollTo lands exactly on a document offset. It fails rather than clamping:
// a stop that came up short would compare the pixels of two different offsets
// and call the difference a scroll artefact.
func (h *flkHarness) scrollTo(y int32) {
	h.t.Helper()
	if dy := y - h.s.Viewport().Offset.Y; dy != 0 {
		h.frame(dy)
		if got := h.s.Viewport().Offset.Y; got != y {
			h.t.Fatalf("scrollTo(%d) landed at %d", y, got)
		}
	}
	h.settle()
}

// snapshot copies the composited surface, which the next frame rewrites in
// place.
func (h *flkHarness) snapshot() *frame.Bitmap {
	cp := frame.NewBitmap(int(h.c.Backing.W), int(h.c.Backing.H))
	copy(cp.RGBA, h.c.Backing.RGBA)
	return cp
}

func (h *flkHarness) rasterized() int64 { return h.s.Stats().Rasterized }

// flkChanged lists the pixels two surfaces disagree about, capped so a failure
// names a few rather than flooding the log.
func flkChanged(a, b *frame.Bitmap) [][2]int {
	var out [][2]int
	for y := 0; y < int(b.H); y++ {
		for x := 0; x < int(b.W); x++ {
			if a.At(x, y) != b.At(x, y) {
				if out = append(out, [2]int{x, y}); len(out) >= 8 {
					return out
				}
			}
		}
	}
	return out
}

// The idle half of "colour does not flicker": frames that change nothing must
// ask for no tile, report no damage, and leave no pixel moved.
//
// qc:BR-074
func TestIdleFramesMoveNoColourAndDrawNoTile(t *testing.T) {
	h := flkPage(t, flkFixture)
	h.settle()
	first := h.snapshot()
	before := h.rasterized()

	for i := 0; i < 5; i++ {
		w := h.frame(0)
		if w.Needed != 0 {
			t.Errorf("idle frame %d wanted %d tiles, want none", i, w.Needed)
		}
		if len(h.damage) != 0 {
			t.Errorf("idle frame %d reported %d damage rects, want none: an idle vsync must cost no pixel work", i, len(h.damage))
		}
	}
	if got := h.rasterized() - before; got != 0 {
		t.Errorf("%d tiles rasterized across 5 idle frames, want 0", got)
	}
	if bad := flkChanged(first, h.snapshot()); len(bad) != 0 {
		t.Errorf("5 idle frames moved %d pixels, e.g. %v: colour must not change when nothing happened", len(bad), bad)
	}
}

// The scroll half, as the anchor states it: the surface at an offset is the
// same surface however the viewport arrived, and the top of the page survives a
// round trip through the rows a scroll memmoved and the tiles the cache dropped
// on the way down.
//
// qc:BR-074
func TestTheSameOffsetLooksTheSameHoweverTheScrollGotThere(t *testing.T) {
	const y = 160
	h := flkPage(t, flkFixture)
	h.settle()
	top := h.snapshot()

	h.scrollTo(y)
	jump := h.snapshot()

	// Eight steps of 20: every intermediate row is composited, and the
	// destination is reached by memmoves rather than by one big shift.
	h.scrollTo(0)
	for cur := int32(20); cur <= y; cur += 20 {
		h.scrollTo(cur)
	}
	stepped := h.snapshot()

	// Past it and back: the tiles that were on screen at 160 have been off
	// screen, in another offset's company, and must come back as themselves.
	h.scrollTo(320)
	h.scrollTo(y)
	there := h.snapshot()

	h.scrollTo(0)
	back := h.snapshot()

	if bad := flkChanged(jump, stepped); len(bad) != 0 {
		t.Errorf("scrollY %d reached in one jump and in eight steps of 20 differ in %d pixels, e.g. %v", y, len(bad), bad)
	}
	if bad := flkChanged(jump, there); len(bad) != 0 {
		t.Errorf("scrollY %d reached directly and after going to 320 and back differ in %d pixels, e.g. %v", y, len(bad), bad)
	}
	if bad := flkChanged(top, back); len(bad) != 0 {
		t.Errorf("the top of the page after a down-and-up round trip differs in %d pixels, e.g. %v: the first frame's colour must survive the journey", len(bad), bad)
	}
}

// A gradient's colour belongs to the document, not to the viewport: the ramp
// must answer the same value at a document point whatever the scroll offset
// paints it at, which is what the anchor's SCROLL-PIXELS pair measures and
// what a tile whose ramp was computed against its own rect would break.
//
// qc:BR-074
func TestGradientColourIsAnchoredToTheDocumentNotTheViewport(t *testing.T) {
	const y = 160
	h := flkPage(t, flkFixture)
	h.settle()

	// Document points inside the 1600px ramp, with the values the reference
	// browser prints at the same points.
	pts := []struct {
		docY    int
		r, g, b int
	}{
		{210, 1, 168, 0},
		{300, 10, 159, 0},
		{450, 26, 143, 0},
		{590, 41, 128, 0},
	}
	atZero := h.snapshot()
	h.scrollTo(y)
	atY := h.snapshot()

	for _, p := range pts {
		c0 := atZero.At(400, p.docY)
		if int(c0.R()) != p.r || int(c0.G()) != p.g || int(c0.B()) != p.b {
			t.Errorf("document (%d,%d) at scrollY 0 reads rgb(%d,%d,%d), want the rgb(%d,%d,%d) Chromium measures there",
				400, p.docY, c0.R(), c0.G(), c0.B(), p.r, p.g, p.b)
		}
		c1 := atY.At(400, p.docY-y)
		if c1 != c0 {
			t.Errorf("document (400,%d) reads rgb(%d,%d,%d) at scrollY %d and rgb(%d,%d,%d) at scrollY 0: the ramp moved with the viewport",
				p.docY, c1.R(), c1.G(), c1.B(), y, c0.R(), c0.G(), c0.B())
		}
	}

	// The hard-stop band keeps its edge too, and it is the syntax that painted
	// nothing before this slice read the unitless zero as a position.
	h.scrollTo(0)
	before := h.snapshot()
	if c := before.At(100, 145); c != frame.RGB(17, 17, 17) {
		t.Errorf("the flat #111 band reads rgb(%d,%d,%d) at document (100,145), want rgb(17,17,17)", c.R(), c.G(), c.B())
	}
	if c := before.At(100, 155); c != frame.RGB(238, 238, 238) {
		t.Errorf("the flat #eee band reads rgb(%d,%d,%d) at document (100,155), want rgb(238,238,238)", c.R(), c.G(), c.B())
	}
	h.scrollTo(60)
	after := h.snapshot()
	if c := after.At(100, 85); c != frame.RGB(17, 17, 17) {
		t.Errorf("after a 60px scroll the #111 band reads rgb(%d,%d,%d), want the rgb(17,17,17) it read at scrollY 0", c.R(), c.G(), c.B())
	}
	if c := after.At(100, 95); c != frame.RGB(238, 238, 238) {
		t.Errorf("after a 60px scroll the #eee band reads rgb(%d,%d,%d), want the rgb(238,238,238) it read at scrollY 0", c.R(), c.G(), c.B())
	}
}

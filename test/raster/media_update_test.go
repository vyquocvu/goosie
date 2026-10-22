package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// These tests are about the one thing UpdateLayer exists for: a producer thread
// changing what a layer displays while the frame thread is drawing it. A page that
// repaints on a timer is the ordinary case, and a playing video is that thirty
// times a second, so the swap has to be handed over rather than written in place —
// and it has to wait for the tiles it would dirty, because Invalidate deliberately
// leaves a painting tile alone and a completion arriving after the swap would
// install pixels from a display list nothing is showing any more.

// gatedRaster holds every job until the test releases it, which is how a tile is
// kept in flight deterministically rather than by hoping to win a race against a
// worker.
type gatedRaster struct {
	release chan struct{}
	entered chan struct{}
}

func newGatedRaster() *gatedRaster {
	return &gatedRaster{release: make(chan struct{}), entered: make(chan struct{}, 16)}
}

func (g *gatedRaster) fn(j raster.Job) error {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-g.release
	return schedRaster(j)
}

// flatDL is a whole-extent fill at content version 1, which is what a repaint that
// reuses a layer produces.
func flatDL(bounds frame.Rect, c frame.Color) *paint.LayerDL {
	list := paint.NewList(1)
	list.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: bounds, Color: c})
	return list.Build(1).Publish()
}

func TestUpdateLayerWaitsForAnInFlightTile(t *testing.T) {
	g := newGatedRaster()
	h := newSchedRaster(t, 2, 2, 8, frame.Viewport{Size: frame.Size{W: 512, H: 512}}, raster.Pref{}, g.fn)

	h.frame(t, vsync(0))
	<-g.entered
	if h.l.Grid.Stats().PaintingBytes == 0 {
		t.Fatal("no tile was in flight, so the test cannot prove the swap waits")
	}

	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)
	dirty := frame.Rect4(0, 0, frame.TileSize, frame.TileSize)
	next := flatDL(bounds, frame.RGB(11, 222, 33))
	if !h.s.UpdateLayer(h.l, next, dirty) {
		t.Fatal("UpdateLayer refused a swap for the layer being drawn")
	}

	// A frame planned while the dirty tile is still painting must leave the old
	// content in place rather than swap underneath it.
	h.pump()
	if h.l.Content == frame.Content(next) {
		t.Fatal("the content swapped while a tile it would dirty was still in flight")
	}

	close(g.release)
	h.untilQuiet(t)
	// untilQuiet stops at the first frame that wants nothing, and a swap deferred
	// behind an in-flight tile is applied at the top of the frame after that one.
	// The real loop plans a frame on every vsync whether or not anything is
	// needed, so this is one vsync of latency rather than a stall — but it is
	// worth pinning, because a swap that waited for something else to trigger a
	// frame would never reach a paused video.
	h.pump()
	if h.l.Content != frame.Content(next) {
		t.Fatal("the content never swapped once the in-flight tile finished")
	}

	// Damage stayed scoped: the swap named one tile's rectangle, so a tile
	// diagonally opposite it — which the new content does not change — must not
	// have been staled along with it.
	out := frame.CoordFor(frame.Point{X: frame.TileSize + 10, Y: frame.TileSize + 10}, frame.TileSize)
	if h.l.Needs(out) {
		t.Fatal("a tile outside the damage was staled: the swap escaped its dirty rect")
	}
}

func TestUpdateLayerKeepsOnlyTheNewest(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)

	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)
	dirty := frame.Rect4(0, 0, frame.TileSize, frame.TileSize)
	middle := flatDL(bounds, frame.RGB(1, 2, 3))
	last := flatDL(bounds, frame.RGB(4, 5, 6))

	// A producer running faster than the display is the normal case for a video:
	// frames are dropped rather than queued, because a queued frame is one the
	// user is shown late and out of order.
	h.s.UpdateLayer(h.l, middle, dirty)
	h.s.UpdateLayer(h.l, last, dirty)
	h.pump()

	if h.l.Content != frame.Content(last) {
		t.Fatal("an older content update won over a newer one")
	}
	if h.l.Content == frame.Content(middle) {
		t.Fatal("the superseded update was applied")
	}
}

func TestUpdateLayerForAnotherLayerIsDropped(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)

	other, _ := schedDoc(t, 2, 2, 8)
	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)
	stray := flatDL(bounds, frame.RGB(200, 0, 0))
	h.s.UpdateLayer(other, stray, bounds)

	before := h.l.Content
	h.pump()
	if h.l.Content != before {
		t.Fatal("an update for a layer that is not being drawn changed the one that is")
	}
	// The stale update must be gone rather than applied later, once a navigation
	// happens to make that layer current again.
	h.s.SetPlan(frame.FramePlan{Serial: 2, Layers: []*frame.Layer{other}, Background: testBackground})
	h.pump()
	if other.Content == frame.Content(stray) {
		t.Fatal("a dropped update was applied when its layer later became current")
	}
}

func TestUpdateLayerRejectsAnEmptySwap(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)
	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)

	if h.s.UpdateLayer(nil, flatDL(bounds, frame.RGB(1, 1, 1)), bounds) {
		t.Fatal("UpdateLayer accepted a nil layer")
	}
	if h.s.UpdateLayer(h.l, nil, bounds) {
		t.Fatal("UpdateLayer accepted nil content")
	}
	before := h.l.Content
	h.pump()
	if h.l.Content != before {
		t.Fatal("a rejected update changed the layer")
	}
}

// TestUpdateLayerAppliesWithoutAPlanPublish is the case a video repaint actually
// takes: the layer and its extent are unchanged, so no plan is republished, and the
// swap still has to reach the screen.
func TestUpdateLayerAppliesWithoutAPlanPublish(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)
	published := h.s.PlanStats().Published

	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)
	next := flatDL(bounds, frame.RGB(9, 200, 9))
	rasterizedBefore := h.s.Stats().Rasterized

	h.s.UpdateLayer(h.l, next, bounds)
	h.frame(t, vsync(0))
	h.untilQuiet(t)

	if got := h.s.PlanStats().Published; got != published {
		t.Fatalf("a content swap published %d new plans, want none", got-published)
	}
	if h.l.Content != frame.Content(next) {
		t.Fatal("the swap never reached the layer")
	}
	// Holding new content is not the same as showing it: the tiles the swap
	// dirtied have to be rasterized again from the new list. The harness's worker
	// paints a tile's identity rather than the display list's colours, so this is
	// measured in tiles redrawn rather than in pixels read back.
	if got := h.s.Stats().Rasterized - rasterizedBefore; got == 0 {
		t.Fatal("a content swap re-rasterized nothing: the tiles kept pixels from the old list")
	}
}

package raster

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

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

	h.pump()
	if h.l.Content == frame.Content(next) {
		t.Fatal("the content swapped while a tile it would dirty was still in flight")
	}

	close(g.release)
	h.untilQuiet(t)
	h.pump()
	if h.l.Content != frame.Content(next) {
		t.Fatal("the content never swapped once the in-flight tile finished")
	}

	out := frame.CoordFor(frame.Point{X: frame.TileSize + 10, Y: frame.TileSize + 10}, frame.TileSize)
	if h.l.Needs(out) {
		t.Fatal("a tile outside the damage was staled: the swap escaped its dirty rect")
	}
}

func TestUpdateLayerAutomaticDamageWaitsForInFlightTile(t *testing.T) {
	g := newGatedRaster()
	h := newSchedRaster(t, 2, 2, 8, frame.Viewport{Size: frame.Size{W: 512, H: 512}}, raster.Pref{}, g.fn)
	h.frame(t, vsync(0))
	<-g.entered
	next := flatDL(frame.Rect4(0, 0, 512, 512), frame.RGB(11, 222, 33))
	h.s.UpdateLayer(h.l, next, frame.Rect{})
	h.pump()
	premature := h.l.Content == frame.Content(next)
	close(g.release)
	if premature {
		t.Fatal("automatic damage swapped content underneath an in-flight raster")
	}
	h.untilQuiet(t)
	h.pump()
	h.untilQuiet(t)
	if h.l.Content != frame.Content(next) {
		t.Fatal("automatic damage never applied after the worker completed")
	}
}

func TestUpdateLayerKeepsOnlyTheNewest(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)

	bounds := frame.Rect4(0, 0, 2*frame.TileSize, 2*frame.TileSize)
	dirty := frame.Rect4(0, 0, frame.TileSize, frame.TileSize)
	middle := flatDL(bounds, frame.RGB(1, 2, 3))
	last := flatDL(bounds, frame.RGB(4, 5, 6))

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

func TestUpdateLayerCoalescesDamage(t *testing.T) {
	h := newSched(t, 2, 2, 8, frame.Size{W: 512, H: 512}, raster.Pref{})
	h.untilQuiet(t)
	bounds := frame.Rect4(0, 0, 512, 512)
	h.s.UpdateLayer(h.l, flatDL(bounds, frame.RGB(1, 2, 3)), frame.Rect4(0, 0, 256, 256))
	last := flatDL(bounds, frame.RGB(4, 5, 6))
	h.s.UpdateLayer(h.l, last, frame.Rect4(256, 256, 512, 512))
	h.pump()
	if h.l.Content != frame.Content(last) {
		t.Fatal("newest update was not applied")
	}
	if !h.l.Needs(frame.TileCoord{Col: 0, Row: 0}) {
		t.Fatal("coalescing lost damage from the superseded update")
	}
}

// qc:BR-171
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
	if got := h.s.Stats().Rasterized - rasterizedBefore; got == 0 {
		t.Fatal("a content swap re-rasterized nothing: the tiles kept pixels from the old list")
	}
}

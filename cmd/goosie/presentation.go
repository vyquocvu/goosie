package main

import (
	"fmt"
	"math"
	"os"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
)

// handleResize reflows and repaints the active tab after the window is resized.
func (f *framePath) handleResize(contentW, contentH int) {
	f.config.width = contentW
	f.config.height = contentH

	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}

	reflowW := float32(contentW)
	if f.emulationPreset != nil {
		reflowW = float32(f.emulationPreset.Width)
	}

	if err := tab.Session().Reflow(reflowW); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		return
	}

	f.repaintTab(tab, frame.Rect{})
}

// pumpJS runs one event-loop turn for the active tab and repaints on change.
// Background tabs stay frozen (and cheap) until shown. It runs on the chrome
// pump thread like every input callback; see the wiring at openWindow.
func (f *framePath) pumpJS() {
	if f.tabMgr == nil {
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	sess := tab.Session()
	if sess == nil {
		return
	}
	if !sess.PumpJS(float32(f.config.width)) {
		return
	}
	f.repaintTab(tab, frame.Rect{})
}

// selectionDamage unions the previous selection bounds with the current
// ones, scaled to device pixels: a highlight change damages exactly where
// ink appeared or disappeared. Either side may be absent (a collapsed caret
// paints nothing, a cleared highlight leaves nothing); both absent yields an
// empty rect, which repaintTab resolves by comparing display lists.
func (f *framePath) selectionDamage(tab *tabs.Tab, oldR frame.Rect, oldOK bool) frame.Rect {
	scale := float32(f.config.dpr) * float32(f.zoom)
	var x0, y0, x1, y1 int32
	first := true
	add := func(r frame.Rect) {
		dx0 := int32(math.Floor(float64(r.X0) * float64(scale)))
		dy0 := int32(math.Floor(float64(r.Y0) * float64(scale)))
		dx1 := int32(math.Ceil(float64(r.X1) * float64(scale)))
		dy1 := int32(math.Ceil(float64(r.Y1) * float64(scale)))
		if first || dx0 < x0 {
			x0 = dx0
		}
		if first || dy0 < y0 {
			y0 = dy0
		}
		if first || dx1 > x1 {
			x1 = dx1
		}
		if first || dy1 > y1 {
			y1 = dy1
		}
		first = false
	}
	if oldOK {
		add(oldR)
	}
	if newR, ok := tab.Session().SelectionBounds(); ok {
		add(newR)
	}
	if first {
		return frame.Rect{}
	}
	return frame.Rect{X0: x0, Y0: y0, X1: x1, Y1: y1}
}

// repaintTab rebuilds the active tab's layer from its session and hands the
// plan to the scheduler. dirty is the device-pixel damage in layer space;
// an empty rect derives damage from the old and new display lists.
//
// When the tab already holds a layer over the same extent and budget, the
// layer (and its tile cache and pool) is reused: the new content is
// attached and only dirty tiles go stale via InvalidateRect, and no plan is
// republished — the loop re-reads the shared layer pointer every frame and
// re-rasterizes exactly what went stale. A new layer (and full plan) happens
// only when geometry demands it.
//
// Versions deliberately do not advance on reuse: bumping the content version
// would skew any frame already rasterizing the previous list (its completion
// arrives stamped with the old display-list version and would be dropped as
// stale forever, requeued every frame). Invalidating dirty tiles at a fixed
// version converges by construction instead.
func (f *framePath) repaintTab(tab *tabs.Tab, dirty frame.Rect) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	if f.emulationPreset != nil {
		effectiveDPR = f.emulationPreset.DPR * float32(f.zoom)
	}
	list, err := tab.Session().PaintChecked(effectiveDPR)

	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: paint:", err)
		return
	}

	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: budget:", err)
		return
	}
	layerBounds := extent.Canon()
	if layerBounds.X0 < 0 {
		layerBounds.X0 = 0
	}
	if layerBounds.Y0 < 0 {
		layerBounds.Y0 = 0
	}

	if lyr := tab.Layer(); lyr != nil && lyr.Bounds == layerBounds && lyr.Grid.Budget() == budgetBytes {
		// Reuse: keep the tile cache and pool. How the new content is attached
		// depends on whether the frame thread is reading this layer.
		if f.tabMgr.Active() == tab {
			// Handed to the scheduler rather than written here, because this runs
			// on the chrome pump thread and SetContent/InvalidateRect are
			// frame-thread operations. Writing them directly races BeginFrame's
			// read of the same fields, and since Invalidate leaves a painting tile
			// alone, a swap made while one of the damaged tiles is in flight can
			// leave it showing content nothing displays any more. A repaint driven
			// by a video frame is that case thirty times a second rather than once
			// in a while. The scheduler applies the swap on its own thread once no
			// rasterization is in flight for a tile the damage covers.
			f.sched.UpdateLayer(lyr, dl, dirty)
		} else {
			// A background tab's layer is not the one being drawn, so no frame
			// thread is reading it and attaching directly cannot race. Doing it
			// here is also what keeps the tab current for when it comes forward.
			if dirty.Empty() {
				dirty = paint.ContentDamage(lyr.Content, dl)
			}
			lyr.SetContent(dl)
			lyr.InvalidateRect(dirty)
		}
		f.publishAccessibility()
		return
	}

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)

	tab.SetLayer(layer)
	// A background tab's repaint must not steal the screen; switchTab
	// presents the fresh layer when the tab comes forward.
	if f.tabMgr.Active() == tab {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     tab.Nav.Serial,
			Layers:     []*frame.Layer{layer},
			Background: tab.BGColor(),
		})
	}
	f.publishAccessibility()
}

// publishAccessibility hands the active document's accessibility tree to a
// window that can publish it. The tree's coordinates are document CSS pixels;
// the platform wants window content coordinates, so every rect is shifted by
// the scroll offset before it leaves. A backend with no accessibility story,
// or a tab with no session, publishes nothing - clearing what a previous
// document left behind.
func (f *framePath) publishAccessibility() {
	aw, ok := f.window.(surface.AccessibilityWindow)
	if !ok {
		return
	}
	if f.tabMgr == nil {
		aw.SetAccessibility(nil)
		return
	}
	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		aw.SetAccessibility(nil)
		return
	}
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	vp := f.sched.Viewport()
	aw.SetAccessibility(shiftAx(tab.Session().AccessibilityTree(),
		float32(vp.Offset.X)/effectiveDPR, float32(vp.Offset.Y)/effectiveDPR))
}

// shiftAx copies the tree with every rect translated by (dx, dy).
func shiftAx(nodes []ax.Node, dx, dy float32) []ax.Node {
	out := make([]ax.Node, len(nodes))
	for i, n := range nodes {
		n.X0 -= dx
		n.Y0 -= dy
		n.X1 -= dx
		n.Y1 -= dy
		n.Children = shiftAx(n.Children, dx, dy)
		out[i] = n
	}
	return out
}

// docPoint converts a content-area point to document coordinates through the
// active zoom and scroll offset.
func (f *framePath) docPoint(contentX, contentY int32) (float32, float32) {
	effectiveDPR := float32(f.config.dpr) * float32(f.zoom)
	vp := f.sched.Viewport()
	return float32(contentX)/effectiveDPR + float32(vp.Offset.X)/effectiveDPR,
		float32(contentY)/effectiveDPR + float32(vp.Offset.Y)/effectiveDPR
}

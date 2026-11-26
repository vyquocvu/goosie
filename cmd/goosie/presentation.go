package main

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func (f *framePath) contentWidth() int {
	f.sizeMu.RLock()
	defer f.sizeMu.RUnlock()
	return f.config.width
}

func (f *framePath) contentHeight() int {
	f.sizeMu.RLock()
	defer f.sizeMu.RUnlock()
	return f.config.height
}

func (f *framePath) devSize() frame.Size {
	f.sizeMu.RLock()
	defer f.sizeMu.RUnlock()
	return f.config.devSize()
}

func (f *framePath) documentViewport() frame.Size {
	if f.sched != nil {
		if size := f.sched.Viewport().Size; !size.Empty() {
			return size
		}
	}
	return f.devSize()
}

func (f *framePath) setContentSize(w, h int) {
	f.sizeMu.Lock()
	f.config.width = w
	f.config.height = h
	f.sizeMu.Unlock()
}

func (f *framePath) handleResize(contentW, contentH int) {
	f.setContentSize(contentW, contentH)
	if f.sched != nil {
		vp := f.sched.Viewport()
		vp.Size = f.devSize()
		f.sched.SetViewport(vp)
	}

	tab := f.tabMgr.Active()
	if tab == nil || tab.Session() == nil {
		return
	}

	reflowW := float32(contentW)
	reflowH := float32(contentH)
	canHover, finePointer := true, true
	if f.emulationPreset != nil {
		reflowW = float32(f.emulationPreset.Width)
		reflowH = float32(f.emulationPreset.Height)
		canHover = !f.emulationPreset.Mobile
		finePointer = !f.emulationPreset.Mobile
	}

	tab.Session().SetViewportSize(reflowW, reflowH)
	if err := tab.Session().Reflow(reflowW); err != nil {
		fmt.Fprintln(os.Stderr, "goosie: reflow:", err)
		return
	}
	tab.Session().SetDevicePixelRatio(f.effectiveScale())
	tab.Session().SetPointerCapabilities(canHover, finePointer)
	tab.Session().FireResizeEvent()

	f.repaintTab(tab, frame.Rect{})
}

func (f *framePath) syncViewportScroll(tab *tabs.Tab) bool {
	f.scrollMu.Lock()
	defer f.scrollMu.Unlock()
	return f.syncViewportScrollLocked(tab)
}

func (f *framePath) syncViewportScrollLocked(tab *tabs.Tab) bool {
	if f.sched == nil || tab == nil || tab.Session() == nil || f.tabMgr == nil {
		return false
	}
	if f.tabMgr.Active() != tab {
		return false
	}
	vp := f.sched.Viewport()
	tabY := tab.ScrollY()
	if tabY == vp.Offset.Y {
		f.lastSyncedScrollY = vp.Offset.Y
		return false
	}
	if f.sched.ScrollOffset().Y == tabY {
		return false
	}
	if vp.Offset.Y == f.lastSyncedScrollY {
		vp.Offset.Y = tabY
		f.sched.SetViewport(vp)
		tab.Session().SyncViewportScroll(float64(vp.Offset.X), float64(tabY))
		f.lastSyncedScrollY = tabY
		return true
	}
	tab.SetScrollY(vp.Offset.Y)
	f.sched.SetViewport(vp)
	tab.Session().SyncViewportScroll(float64(vp.Offset.X), float64(vp.Offset.Y))
	f.lastSyncedScrollY = vp.Offset.Y
	return true
}

func (f *framePath) pumpJS() bool {
	if f.tabMgr == nil {
		return f.pumpPaced()
	}
	f.scrollMu.Lock()
	tab := f.tabMgr.Active()
	var changed bool
	if tab != nil {
		changed = f.syncViewportScrollLocked(tab)
	}
	f.scrollMu.Unlock()
	if tab == nil {
		return false
	}
	sess := tab.Session()
	if sess == nil {
		return false
	}
	focusedBefore := sess.Focused()
	if f.windowContexts != nil {
		changed = f.windowContexts.flushAll() || changed
	}
	changed = sess.PumpJS(float32(f.contentWidth())) || changed
	if sess.Focused() != focusedBefore {
		f.syncContentFocus(tab)
		changed = true
	}
	changed = f.serviceFilePicker(tab) || changed
	changed = f.drainSessionNavigation(tab) || changed
	if !changed {
		return false
	}
	if sess.HasScriptTimeout() {
		tab.Nav.Mu.Lock()
		serial := tab.Nav.Serial
		tab.Nav.Mu.Unlock()
		if f.installUnresponsivePrompt(tab, tab.URL(), sess.ScriptTimeoutCount(), serial, sess) {
			return true
		}
	}
	f.repaintTab(tab, frame.Rect{})
	return true
}

func (f *framePath) pendingReveal() (time.Duration, bool) {
	sess := f.pacedSession
	if f.tabMgr != nil {
		if tab := f.tabMgr.Active(); tab != nil {
			sess = tab.Session()
		} else {
			sess = nil
		}
	}
	if sess == nil {
		return 0, false
	}
	return sess.PendingLoadTimerDelay()
}

func (f *framePath) pumpPaced() bool {
	sess := f.pacedSession
	if sess == nil {
		return false
	}
	if !sess.PumpJS(float32(f.contentWidth())) {
		return false
	}
	return f.repaintPaced(sess)
}

func (f *framePath) repaintPaced(sess *engine.Session) bool {
	scale := f.effectiveScale()
	list, err := sess.PaintChecked(scale)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: paint:", err)
		return false
	}
	dl := list.Build(1)
	extent, err := sess.DocumentLayerBounds(dl.Extent(), scale, f.documentViewport())
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: bounds:", err)
		return false
	}
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: budget:", err)
		return false
	}
	bg := sess.BackgroundColor()
	if lyr := f.layer; lyr != nil && lyr.Bounds == extent && lyr.Grid.Budget() == budgetBytes {
		f.sched.UpdateLayer(lyr, dl, frame.Rect{})
		if bg != f.pacedBG {
			f.pacedBG = bg
			f.sched.SetPlan(frame.FramePlan{
				Serial:     1,
				Layers:     []*frame.Layer{lyr},
				Background: bg,
			})
		}
		return true
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	f.layer = layer
	f.pacedBG = bg
	f.sched.SetPlan(frame.FramePlan{
		Serial:     1,
		Layers:     []*frame.Layer{layer},
		Background: bg,
	})
	return true
}

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
	extent, err := tab.Session().DocumentLayerBounds(dl.Extent(), effectiveDPR, f.documentViewport())
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: bounds:", err)
		return
	}
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goosie: budget:", err)
		return
	}

	bg := tab.Session().BackgroundColor()
	previousBG := tab.BGColor()
	tab.SetBGColor(bg)

	if lyr := tab.Layer(); lyr != nil && lyr.Bounds == extent && lyr.Grid.Budget() == budgetBytes {
		if f.tabMgr.Active() == tab {
			f.sched.UpdateLayer(lyr, dl, dirty)
			if bg != previousBG {
				f.sched.SetPlan(frame.FramePlan{
					Serial:     tab.Nav.Serial,
					Layers:     []*frame.Layer{lyr},
					Background: bg,
				})
			}
		} else {
			if dirty.Empty() {
				dirty = paint.ContentDamage(lyr.ContentSnapshot(), dl)
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
	if f.tabMgr.Active() == tab {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     tab.Nav.Serial,
			Layers:     []*frame.Layer{layer},
			Background: bg,
		})
	}
	f.publishAccessibility()
}

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

func (f *framePath) scheduleInspectRepaint(tab *tabs.Tab) {
	if tab == nil {
		return
	}
	f.inspectRepaintMu.Lock()
	for _, t := range f.inspectRepaintPending {
		if t == tab {
			f.inspectRepaintMu.Unlock()
			return
		}
	}
	f.inspectRepaintPending = append(f.inspectRepaintPending, tab)
	f.inspectRepaintMu.Unlock()
}

func (f *framePath) drainInspectRepaints() {
	f.inspectRepaintMu.Lock()
	pending := f.inspectRepaintPending
	f.inspectRepaintPending = nil
	f.inspectRepaintMu.Unlock()
	if f.tabMgr == nil {
		return
	}
	for _, tab := range pending {
		if tab == nil || f.tabMgr.TabByID(tab.ID()) != tab || tab.Session() == nil {
			continue
		}
		f.repaintTab(tab, frame.Rect{})
	}
}

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

func (f *framePath) docPoint(contentX, contentY int32) (float32, float32) {
	dpr := float32(f.config.dpr)
	if dpr <= 0 {
		dpr = 1
	}
	zoom := float32(f.zoom)
	if zoom <= 0 {
		zoom = 1
	}
	effectiveDPR := dpr * zoom
	var offset frame.Point
	if f.sched != nil {
		offset = f.sched.Viewport().Offset
	}
	return float32(contentX+offset.X) / effectiveDPR,
		float32(contentY+offset.Y) / effectiveDPR
}

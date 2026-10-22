package main

import (
	"bytes"
	"strconv"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const (
	totalChromeHeight  = tabs.TabBarHeight + toolbar.ToolbarHeight
	bookmarksBarHeight = 28
)

type chromeWindow struct {
	surface.Window
	toolbar           *toolbar.State
	tabMgr            *tabs.TabManager
	scale             int32
	events            chan surface.Event
	onSwitch          func(uint64)
	onNewTab          func()
	onNewIncognitoTab func()
	onCloseTab        func(uint64)
	onReopenTab       func()
	onResize          func(contentW, contentH int)
	onZoom            func(delta float64)
	onLinkClick       func(href string)
	onBookmark        func()
	onInspect         func()
	onViewSource      func()
	onDuplicateTab    func()
	onReaderMode      func()
	onFocusClick      func(contentX, contentY int32) bool
	onContentKey      func(key rune, mods surface.KeyMod) bool
	onDragSelect      func(action surface.PointerAction, contentX, contentY int32) bool
	onIME             func(ev surface.Event) bool
	onCopy            func() bool
	hitTestLink       func(contentX, contentY int32) string
	onHoverCursor     func(contentX, contentY int32) surface.Cursor
	chromeBitmap      *frame.Bitmap
	outBitmap         *frame.Bitmap
	lastChromeH       int32
	fonts             *raster.Fonts
	hover             surface.Cursor // last shape sent to the platform, for change detection

	// Bookmarks bar
	bookmarks        *bookmarks.Store
	showBookmarksBar bool
	hoverBookmark    int

	// Context menu
	contextMenu             *ContextMenu
	onContextMenu           func(contentX, contentY, winX, winY int32)
	onContextMenuAction     func(action, data string)
	onPrintPDF              func()
	onAccessibilitySetValue func(contentX, contentY int32, value string) bool
	onAccessibilityChanged  func()

	// Full screen mode & sizing
	fullscreen          bool
	lastWinW            int32
	lastWinH            int32
	onFullscreenChanged func(fullscreen bool)
	onDeviceEmulation   func()

	// pressTab/pressX/pressing track a press that may become a tab drag.
	// Pump-thread only, like everything else in intercept.
	pressTab uint64
	pressX   int32
	pressing bool

	// tickInterval paces the onTick hook (the host's JS and media pumps); zero
	// disables it, which keeps every literal construction (tests especially)
	// exactly as before. Both are read by the pump under tickMu, because the host
	// installs them after constructing the window.
	tickMu       sync.Mutex
	tickInterval time.Duration
	onTick       func()

	// onMediaControl is consulted on a left press before links and focus, so a
	// click on a video's painted controls reaches the player instead of starting a
	// text selection over it. It reports whether it handled the press.
	onMediaControl func(contentX, contentY int32) bool
}

// setTick installs the host's per-vsync hook. It is safe after construction,
// which is how the host wires it: the pump reads the configuration each time
// around its loop rather than snapshotting it once at startup.
//
// Snapshotting was a race the pump usually lost by luck. The constructor starts
// the pump, the host assigns the hook on the next line, and whichever ran first
// decided whether the browser ever ticked at all — no timers, no fetch
// completions, and no video frames. Reading under a mutex makes the order
// irrelevant instead of fortunate.
func (cw *chromeWindow) setTick(interval time.Duration, fn func()) {
	cw.tickMu.Lock()
	cw.tickInterval = interval
	cw.onTick = fn
	cw.tickMu.Unlock()
}

func (cw *chromeWindow) tickConfig() (time.Duration, func()) {
	cw.tickMu.Lock()
	defer cw.tickMu.Unlock()
	return cw.tickInterval, cw.onTick
}

func newChromeWindow(w surface.Window, tb *toolbar.State, mgr *tabs.TabManager,
	onSwitch func(uint64), onNewTab func(), onCloseTab func(uint64), onReopenTab func(),
	onResize func(contentW, contentH int), onZoom func(delta float64),
	onLinkClick func(href string), onBookmark func(),
	onFocusClick func(contentX, contentY int32) bool,
	onContentKey func(key rune, mods surface.KeyMod) bool,
	onDragSelect func(action surface.PointerAction, contentX, contentY int32) bool,
	onIME func(ev surface.Event) bool,
	onCopy func() bool,
	hitTestLink func(contentX, contentY int32) string,
	onHoverCursor func(contentX, contentY int32) surface.Cursor,
	fonts *raster.Fonts, scale int32) *chromeWindow {
	if scale < 1 {
		scale = 1
	}
	cw := &chromeWindow{
		Window:        w,
		toolbar:       tb,
		tabMgr:        mgr,
		scale:         scale,
		events:        make(chan surface.Event, 64),
		onSwitch:      onSwitch,
		onNewTab:      onNewTab,
		onCloseTab:    onCloseTab,
		onReopenTab:   onReopenTab,
		onResize:      onResize,
		onZoom:        onZoom,
		onLinkClick:   onLinkClick,
		onBookmark:    onBookmark,
		onFocusClick:  onFocusClick,
		onContentKey:  onContentKey,
		onDragSelect:  onDragSelect,
		onIME:         onIME,
		onCopy:        onCopy,
		hitTestLink:   hitTestLink,
		onHoverCursor: onHoverCursor,
		fonts:         fonts,
		hoverBookmark: -1,
		contextMenu:   NewContextMenu(),
	}
	go cw.pump()
	return cw
}

func (cw *chromeWindow) sc() int32 {
	if cw.scale < 1 {
		return 1
	}
	return cw.scale
}

// SetAccessibility preserves the document tree while adding the browser's
// own tabs and toolbar controls. chromeWindow embeds surface.Window, which
// otherwise hides optional capabilities from framePath's type assertion.
func (cw *chromeWindow) SetAccessibility(nodes []ax.Node) {
	aw, ok := cw.Window.(surface.AccessibilityWindow)
	if !ok {
		return
	}
	if offset := cw.chromeHeight() / cw.sc(); offset > 0 {
		nodes = shiftAccessibilityY(nodes, float32(offset))
	}
	combined := cw.accessibilityChrome()
	combined = append(combined, nodes...)
	aw.SetAccessibility(combined)
}

func shiftAccessibilityY(nodes []ax.Node, dy float32) []ax.Node {
	out := make([]ax.Node, len(nodes))
	for i, node := range nodes {
		node.Y0 += dy
		node.Y1 += dy
		node.Children = shiftAccessibilityY(node.Children, dy)
		out[i] = node
	}
	return out
}

func (cw *chromeWindow) accessibilityChrome() []ax.Node {
	if cw.fullscreen || cw.isStandalone() || cw.toolbar == nil || cw.tabMgr == nil {
		return nil
	}
	width := cw.toolbar.BoundsWidth() / cw.sc()
	if width <= 0 {
		return nil
	}
	var roots []ax.Node
	tabList := cw.tabMgr.Tabs()
	count := int32(len(tabList))
	scroll := cw.tabMgr.TabScroll()
	visibleRight := width
	if tabs.ChevronsShown(count, width) {
		visibleRight -= tabs.ChevronReserve
	}
	clip := func(r frame.Rect, right int32) (frame.Rect, bool) {
		if r.X0 < 0 {
			r.X0 = 0
		}
		if r.X1 > right {
			r.X1 = right
		}
		return r, r.X0 < r.X1 && r.Y0 < r.Y1
	}
	toNode := func(role ax.Role, id, label, value string, r frame.Rect) ax.Node {
		return ax.Node{Role: role, ID: id, Label: label, Value: value,
			X0: float32(r.X0), Y0: float32(r.Y0), X1: float32(r.X1), Y1: float32(r.Y1)}
	}
	activeID := uint64(0)
	if active := cw.tabMgr.Active(); active != nil {
		activeID = active.ID()
	}
	tabNodes := make([]ax.Node, 0, len(tabList)+3)
	for i, tab := range tabList {
		r, ok := clip(tabs.TabRect(i, scroll, count, width), visibleRight)
		if !ok {
			continue
		}
		label := tab.Title()
		if label == "" || label == "New Tab" {
			label = tab.URL()
		}
		if label == "" {
			label = "New Tab"
		}
		id := strconv.FormatUint(tab.ID(), 10)
		node := toNode(ax.RoleTab, "goosie.tab."+id, label, "", r)
		node.Selected = tab.ID() == activeID
		if !tab.Pinned() {
			closeRect, shown := clip(tabs.CloseButtonRect(tabs.TabRect(i, scroll, count, width)), visibleRight)
			if shown {
				close := toNode(ax.RoleButton, "goosie.tab."+id+".close", "Close tab "+label, "", closeRect)
				node.Children = append(node.Children, close)
			}
		}
		tabNodes = append(tabNodes, node)
	}
	newTabRect, shown := clip(tabs.NewTabButtonRect(count, scroll, width), visibleRight)
	if shown {
		tabNodes = append(tabNodes, toNode(ax.RoleButton, "goosie.new-tab", "New tab", "", newTabRect))
	}
	if left, right, ok := tabs.ChevronRects(count, width); ok {
		if r, visible := clip(left, width); visible {
			tabNodes = append(tabNodes, toNode(ax.RoleButton, "goosie.tabs.scroll-left", "Scroll tabs left", "", r))
		}
		if r, visible := clip(right, width); visible {
			tabNodes = append(tabNodes, toNode(ax.RoleButton, "goosie.tabs.scroll-right", "Scroll tabs right", "", r))
		}
	}
	tabGroup := ax.Node{Role: ax.RoleTabGroup, ID: "goosie.tabs", Label: "Tabs",
		X0: 0, Y0: 0, X1: float32(width), Y1: tabs.TabBarHeight, Children: tabNodes}
	roots = append(roots, tabGroup)

	value, addressFocused, findActive, loading, canBack, canForward, activeDownloads := cw.toolbar.AccessibilitySnapshot()
	toolY := int32(tabs.TabBarHeight)
	toolChildren := make([]ax.Node, 0, 5)
	appendTool := func(id, label, value string, r frame.Rect, disabled, addressBar bool) {
		r, visible := clip(translateAxRect(r, toolY), width)
		if !visible {
			return
		}
		node := toNode(ax.RoleButton, id, label, value, r)
		node.Disabled = disabled
		node.AddressBar = addressBar
		toolChildren = append(toolChildren, node)
	}
	appendTool("goosie.back", "Back", "", toolbar.ButtonRect(toolbar.ButtonBack, width), !canBack, false)
	appendTool("goosie.forward", "Forward", "", toolbar.ButtonRect(toolbar.ButtonForward, width), !canForward, false)
	reloadLabel := "Reload"
	if loading {
		reloadLabel = "Stop loading"
	}
	appendTool("goosie.reload", reloadLabel, "", toolbar.ButtonRect(toolbar.ButtonReload, width), false, false)
	if addressRect, visible := clip(translateAxRect(toolbar.AddressBarRect(width), toolY), width); visible {
		label := "Address and search"
		if findActive {
			label = "Find in page"
		}
		address := toNode(ax.RoleTextField, "goosie.address", label, value, addressRect)
		address.AddressBar = true
		address.Focused = addressFocused
		toolChildren = append(toolChildren, address)
	}
	downloadValue := ""
	if activeDownloads > 0 {
		downloadValue = strconv.Itoa(activeDownloads) + " active"
	}
	appendTool("goosie.downloads", "Downloads", downloadValue, toolbar.ButtonRect(toolbar.ButtonDownloads, width), false, false)
	roots = append(roots, ax.Node{Role: ax.RoleGroup, ID: "goosie.toolbar", Label: "Browser toolbar",
		X0: 0, Y0: float32(toolY), X1: float32(width), Y1: float32(toolY + toolbar.ToolbarHeight), Children: toolChildren})
	return roots
}

func translateAxRect(r frame.Rect, dy int32) frame.Rect {
	r.Y0 += dy
	r.Y1 += dy
	return r
}

func (cw *chromeWindow) isStandalone() bool {
	if cw.tabMgr != nil {
		active := cw.tabMgr.Active()
		if active != nil && active.IsStandaloneApp() {
			return true
		}
	}
	return false
}

func (cw *chromeWindow) chromeHeight() int32 {
	if cw.fullscreen || cw.isStandalone() {
		return 0
	}
	h := int32(totalChromeHeight)
	if cw.showBookmarksBar {
		h += bookmarksBarHeight
	}
	return h * cw.sc()
}

func (cw *chromeWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	chromeH := cw.chromeHeight()
	if cw.fullscreen || cw.isStandalone() {
		if cw.outBitmap == nil || cw.outBitmap.W != buf.W || cw.outBitmap.H != buf.H {
			cw.outBitmap = frame.NewBitmap(buf.W, buf.H)
		}
		cw.lastWinW = int32(buf.W)
		cw.lastWinH = int32(buf.H)
		copy(cw.outBitmap.RGBA, buf.RGBA)
		merged := damage
		if cw.contextMenu != nil && cw.contextMenu.Visible {
			cw.contextMenu.Draw(cw.outBitmap, cw.fonts, cw.sc())
			mRect := cw.contextMenu.Rect()
			if !mRect.Empty() {
				merged = append(make([]frame.Rect, 0, len(damage)+1), damage...)
				merged = append(merged, mRect)
			}
		}
		return cw.Window.Present(cw.outBitmap, merged)
	}

	if cw.chromeBitmap == nil || cw.chromeBitmap.W != buf.W || int32(cw.chromeBitmap.H) != chromeH {
		cw.chromeBitmap = frame.NewBitmap(buf.W, int(chromeH))
	}

	cw.chromeBitmap.FillRect(frame.Rect4(0, 0, int32(cw.chromeBitmap.W), chromeH), frame.RGB(255, 255, 255), nil)
	// Draw and hit-testing share one clamped strip offset: TabScrollFor keeps
	// the value inside the legal range for the painted width, so a resize
	// cannot strand the strip past its content between wheel events.
	n := int32(cw.tabMgr.Count())
	stripW := int32(cw.chromeBitmap.W) / cw.sc()
	stripOff := cw.tabMgr.TabScroll()
	if max := tabs.StripMaxOffset(n, stripW); stripOff > max {
		stripOff = max
	}
	tabs.DrawTabBar(cw.chromeBitmap, cw.tabMgr, stripOff, cw.fonts, cw.sc())
	cw.toolbar.Draw(cw.chromeBitmap, int32(tabs.TabBarHeight)*cw.sc())
	if cw.showBookmarksBar && cw.bookmarks != nil {
		cw.drawBookmarksBar(cw.chromeBitmap)
	}

	// The loop's backing store is the content viewport, so buf is window-minus-
	// chrome tall. The window surface is the whole window, so the presented
	// bitmap has to carry the chrome rows on top and the content rows shifted
	// down by chromeH; presenting buf directly would show a short frame and let
	// the chrome paint over the top of the document.
	outH := int32(buf.H) + chromeH
	outResized := cw.outBitmap == nil || int32(cw.outBitmap.W) != int32(buf.W) || int32(cw.outBitmap.H) != outH
	if outResized {
		cw.outBitmap = frame.NewBitmap(buf.W, int(outH))
	}
	cw.lastWinW = int32(buf.W)
	cw.lastWinH = outH
	out := cw.outBitmap
	chromeHeightChanged := cw.lastChromeH != chromeH
	chromeChanged := outResized || chromeHeightChanged || !bitmapRegionEqual(cw.chromeBitmap, out, chromeH)
	fullCopy := outResized || chromeHeightChanged

	if fullCopy || chromeChanged {
		for y := int32(0); y < chromeH; y++ {
			copy(out.RGBA[int(y)*out.Stride:int(y)*out.Stride+out.Stride], cw.chromeBitmap.RGBA[int(y)*cw.chromeBitmap.Stride:int(y)*cw.chromeBitmap.Stride+cw.chromeBitmap.Stride])
		}
	}
	if fullCopy {
		copyBitmapRegion(out, buf, buf.Bounds(), chromeH)
	} else {
		// The loop's backing bitmap is updated in place, but out is the coherent
		// window image retained by the platform. Copy only pixels the composer
		// says changed; copying the entire document here on every vsync made an
		// idle page a full-surface memory copy at display refresh rate.
		for _, r := range damage {
			copyBitmapRegion(out, buf, r, chromeH)
		}
	}

	merged := make([]frame.Rect, 0, len(damage)+3)
	if outResized || chromeHeightChanged {
		merged = append(merged, out.Bounds())
	} else if chromeChanged {
		if chromeH > 0 {
			merged = append(merged, frame.Rect4(0, 0, int32(out.W), chromeH))
		}
	}
	if !fullCopy {
		for _, r := range damage {
			if r.Empty() {
				continue
			}
			merged = append(merged, frame.Rect4(r.X0, r.Y0+chromeH, r.X1, r.Y1+chromeH))
		}
	}

	if cw.toolbar != nil && cw.toolbar.HasSuggestions() {
		cw.toolbar.DrawSuggestions(out, chromeH)
		sugRect := cw.toolbar.SuggestionsRect(chromeH)
		if !sugRect.Empty() {
			merged = append(merged, sugRect)
		}
	}
	if cw.contextMenu != nil && cw.contextMenu.Visible {
		cw.contextMenu.Draw(out, cw.fonts, cw.sc())
		mRect := cw.contextMenu.Rect()
		if !mRect.Empty() {
			merged = append(merged, mRect)
		}
	}
	cw.lastChromeH = chromeH
	return cw.Window.Present(out, merged)
}

// bitmapRegionEqual checks the chrome rows against the last composed window
// image. The chrome is redrawn into chromeBitmap each frame, but unchanged pixels
// do not need to be uploaded to the native surface or recommitted to Core Animation.
func bitmapRegionEqual(a, b *frame.Bitmap, rows int32) bool {
	if a == nil || b == nil || a.W != b.W || rows < 0 || rows > int32(a.H) || rows > int32(b.H) {
		return false
	}
	rowBytes := int(a.W) * 4
	for y := 0; y < int(rows); y++ {
		aStart := y * a.Stride
		bStart := y * b.Stride
		if !bytes.Equal(a.RGBA[aStart:aStart+rowBytes], b.RGBA[bStart:bStart+rowBytes]) {
			return false
		}
	}
	return true
}

// copyBitmapRegion updates only a source rectangle in the retained window image.
// dy maps content coordinates below the browser chrome; it is zero for chrome-free
// surfaces. Both rectangles are clipped so a damaged edge can safely touch a bitmap
// boundary.
func copyBitmapRegion(dst, src *frame.Bitmap, r frame.Rect, dy int32) {
	if dst == nil || src == nil || r.Empty() {
		return
	}
	x0, y0 := r.X0, r.Y0
	x1, y1 := r.X1, r.Y1
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > int32(src.W) {
		x1 = int32(src.W)
	}
	if x1 > int32(dst.W) {
		x1 = int32(dst.W)
	}
	if y1 > int32(src.H) {
		y1 = int32(src.H)
	}
	if y0+dy < 0 {
		y0 = -dy
	}
	if y1+dy > int32(dst.H) {
		y1 = int32(dst.H) - dy
	}
	if x1 <= x0 || y1 <= y0 {
		return
	}
	rowBytes := int(x1-x0) * 4
	for y := y0; y < y1; y++ {
		srcStart := int(y)*src.Stride + int(x0)*4
		dstStart := int(y+dy)*dst.Stride + int(x0)*4
		copy(dst.RGBA[dstStart:dstStart+rowBytes], src.RGBA[srcStart:srcStart+rowBytes])
	}
}

func (cw *chromeWindow) Events() <-chan surface.Event {
	return cw.events
}

func (cw *chromeWindow) Name() string {
	if n, ok := cw.Window.(interface{ Name() string }); ok {
		return n.Name()
	}
	return ""
}

func (cw *chromeWindow) Run() {
	if r, ok := cw.Window.(platform.Runner); ok {
		r.Run()
	}
}

func (cw *chromeWindow) pump() {
	defer close(cw.events)
	// The tick shares the pump select so the host's pumps run on the same thread
	// as every input callback: sessions never see two threads at once from this
	// side. No extra lifecycle: the ticker dies with the pump, which dies when the
	// window's events run out.
	//
	// The ticker is built on the first iteration that finds a hook installed
	// rather than before the loop, so a host that configures the window a moment
	// after constructing it still gets its ticks.
	var tick <-chan time.Time
	var tickFn func()
	var ticker *time.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	for {
		if tickFn == nil {
			if interval, fn := cw.tickConfig(); fn != nil && interval > 0 {
				ticker = time.NewTicker(interval)
				tick, tickFn = ticker.C, fn
			}
		}
		select {
		case ev, ok := <-cw.Window.Events():
			if !ok {
				return
			}
			if cw.intercept(ev) {
				continue
			}
			cw.events <- ev
		case <-tick:
			tickFn()
		}
	}
}

func (cw *chromeWindow) intercept(ev surface.Event) bool {
	switch ev.Kind {
	case surface.EvAXSetValue:
		if ev.AXTarget == surface.AXTargetAddressBar {
			if !cw.fullscreen && !cw.isStandalone() && cw.toolbar != nil {
				cw.toolbar.SetAccessibilityValue(ev.Text)
				cw.SetIME(false)
				if cw.onAccessibilityChanged != nil {
					cw.onAccessibilityChanged()
				}
			}
			return true
		}
		contentY := ev.Pos.Y - cw.chromeHeight()
		if contentY >= 0 && cw.onAccessibilitySetValue != nil {
			cw.onAccessibilitySetValue(ev.Pos.X, contentY, ev.Text)
		}
		return true
	case surface.EvPointer:
		if cw.contextMenu != nil && cw.contextMenu.Visible {
			if ev.Action == surface.PointerMotion {
				cw.contextMenu.UpdateHover(ev.Pos, cw.sc())
				mRect := cw.contextMenu.Rect()
				if ev.Pos.X >= mRect.X0 && ev.Pos.X < mRect.X1 && ev.Pos.Y >= mRect.Y0 && ev.Pos.Y < mRect.Y1 {
					return true
				}
			}
			if ev.Action == surface.PointerPress {
				if it, ok := cw.contextMenu.HitItem(ev.Pos, cw.sc()); ok {
					cw.contextMenu.Hide()
					if cw.onContextMenuAction != nil {
						cw.onContextMenuAction(it.Action, it.Data)
					}
					return true
				}
				cw.contextMenu.Hide()
				if ev.Button == surface.ButtonLeft {
					return true
				}
			}
		}

		if ev.Action == surface.PointerPress && ev.Button == surface.ButtonRight {
			if ev.Pos.Y >= cw.chromeHeight() {
				contentX := ev.Pos.X
				contentY := ev.Pos.Y - cw.chromeHeight()
				if cw.onContextMenu != nil {
					cw.onContextMenu(contentX, contentY, ev.Pos.X, ev.Pos.Y)
					return true
				}
			}
		}

		tabH := int32(tabs.TabBarHeight) * cw.sc()
		toolH := int32(toolbar.ToolbarHeight) * cw.sc()
		if ev.Action == surface.PointerMotion && ev.Button == surface.ButtonNone {
			cw.updateHoverCursor(ev.Pos, cw.chromeHeight())
		}
		if !cw.fullscreen && !cw.isStandalone() {
			if ev.Action == surface.PointerRelease {
				if _, _, dragging := cw.tabMgr.Dragging(); dragging {
					if ev.Button == surface.ButtonRight {
						cw.tabMgr.EndDrag()
					} else {
						cw.handleTabBarRelease(ev.Pos)
					}
					return true
				}
			}
			if ev.Pos.Y < tabH {
				switch {
				case ev.Action == surface.PointerPress && ev.Button == surface.ButtonLeft:
					cw.handleTabBarPress(ev.Pos)
					return true
				case ev.Action == surface.PointerMotion && ev.Button == surface.ButtonLeft:
					cw.handleTabBarDrag(ev.Pos)
					return true
				default:
					return true
				}
			}
			toolbarY := ev.Pos.Y - tabH
			if toolbarY >= 0 && toolbarY < toolH {
				// Every phase reaches the toolbar so a press can start a drag
				// selection, motion extends it, and release ends it. Non-left
				// presses are ignored inside; hover motion is a no-op there.
				adjusted := ev.Pos
				adjusted.Y = toolbarY
				cw.toolbar.HandlePointer(adjusted, ev.Button, ev.Action)
				if cw.toolbar.GetFocus() == toolbar.FocusAddress {
					cw.SetIME(false)
				}
				if ev.Action == surface.PointerPress {
					cw.notifyAccessibilityChanged()
				}
				return true
			}
			if cw.toolbar != nil && cw.toolbar.HasSuggestions() {
				sugRect := cw.toolbar.SuggestionsRect(cw.chromeHeight())
				if !sugRect.Empty() && ev.Pos.X >= sugRect.X0 && ev.Pos.X < sugRect.X1 && ev.Pos.Y >= sugRect.Y0 && ev.Pos.Y < sugRect.Y1 {
					if ev.Action == surface.PointerPress && ev.Button == surface.ButtonLeft {
						if sug, ok := cw.toolbar.HitSuggestion(ev.Pos, cw.chromeHeight()); ok {
							if cw.onLinkClick != nil {
								cw.onLinkClick(sug.URL)
							}
							return true
						}
					}
					if ev.Action == surface.PointerMotion {
						cw.toolbar.HoverSuggestion(ev.Pos, cw.chromeHeight())
						return true
					}
				}
			}
			if cw.showBookmarksBar && cw.bookmarks != nil {
				barY := int32(totalChromeHeight) * cw.sc()
				barH := int32(bookmarksBarHeight) * cw.sc()
				if ev.Pos.Y >= barY && ev.Pos.Y < barY+barH {
					if ev.Action == surface.PointerPress && ev.Button == surface.ButtonLeft {
						if entry, ok := cw.hitBookmark(ev.Pos); ok {
							if cw.onLinkClick != nil {
								cw.onLinkClick(entry.URL)
							}
							return true
						}
					}
					if ev.Action == surface.PointerMotion {
						cw.updateBookmarkHover(ev.Pos)
						return true
					}
					return true
				}
			}
			if cw.toolbar.GetFocus() == toolbar.FocusAddress {
				// Presses and releases outside defocus the bar; motion extends
				// an active drag so it clamps to the nearer end like a normal
				// input instead of freezing at the bar's edge.
				if ev.Action == surface.PointerPress || ev.Action == surface.PointerRelease || ev.Action == surface.PointerMotion {
					adjusted := ev.Pos
					adjusted.Y = toolbarY
					cw.toolbar.HandlePointer(adjusted, ev.Button, ev.Action)
				}
			}
		}
		// Link and focus handling is a press concern; a drag's motion and
		// release must not re-trigger them. Whatever a press leaves behind goes
		// to the drag selector, which decides per action whether to consume it.
		contentX := ev.Pos.X
		contentY := ev.Pos.Y - cw.chromeHeight()
		press := ev.Action == surface.PointerPress && ev.Button == surface.ButtonLeft
		if press {
			// Media controls come first. A video's painted buttons sit inside the
			// document, so without this a press on the play button would fall
			// through to focus and then to the drag selector and start a text
			// selection over the thing the user was aiming at.
			if cw.onMediaControl != nil && cw.onMediaControl(contentX, contentY) {
				return true
			}
			if cw.hitTestLink != nil {
				if href := cw.hitTestLink(contentX, contentY); href != "" {
					if cw.onLinkClick != nil {
						cw.onLinkClick(href)
					}
					return true
				}
			}
			if cw.onFocusClick != nil && cw.onFocusClick(contentX, contentY) {
				return true
			}
		}
		if cw.onDragSelect != nil && cw.onDragSelect(ev.Action, contentX, contentY) {
			return true
		}
		return false
	case surface.EvKey:
		if cw.contextMenu != nil && cw.contextMenu.Visible {
			if ev.Key == 0x1B { // Escape
				cw.contextMenu.Hide()
				return true
			}
		}
		if cw.fullscreen && ev.Key == 0x1B { // Escape exits fullscreen
			cw.toggleFullscreen()
			return true
		}
		if ev.Key == 0xF70E || ev.Key == 0xFFC8 { // F11 toggles fullscreen
			cw.toggleFullscreen()
			return true
		}
		// Address-bar edit shortcuts win over browser shortcuts while the
		// bar is focused: Cmd+C there copies the URL selection, it must not
		// fall through to the content copy below.
		if cw.toolbar.GetFocus() == toolbar.FocusAddress && addressEditKey(ev.Key, ev.Mods) {
			cw.toolbar.HandleKeyEvent(ev.Key, ev.Mods)
			cw.notifyAccessibilityChanged()
			return true
		}
		if cw.handleTabShortcut(ev.Key, ev.Mods) {
			return true
		}
		if cw.toolbar.GetFocus() == toolbar.FocusAddress {
			cw.toolbar.HandleKeyEvent(ev.Key, ev.Mods)
			cw.notifyAccessibilityChanged()
			return true
		}
		if cw.onContentKey != nil && cw.onContentKey(ev.Key, ev.Mods) {
			return true
		}
		return false

	case surface.EvIME:
		if cw.onIME != nil {
			return cw.onIME(ev)
		}
		return false
	case surface.EvScroll:
		// A scroll over the tab bar moves the strip when tabs overflow; anywhere
		// else, or when everything fits, it belongs to the document and falls
		// through to the loop. Both axes feed the strip, so a vertical wheel
		// over the tabs behaves like every other browser's.
		tabH := int32(tabs.TabBarHeight) * cw.sc()
		if !cw.fullscreen && !cw.isStandalone() && ev.Pos.Y < tabH && cw.toolbar != nil {
			stripW := cw.toolbar.BoundsWidth() / cw.sc()
			n := int32(cw.tabMgr.Count())
			if tabs.StripMaxOffset(n, stripW) > 0 {
				d := ev.Delta.X + ev.Delta.Y
				cw.tabMgr.SetTabScroll(cw.tabMgr.TabScroll()+d/cw.sc(), n, stripW)
				cw.notifyAccessibilityChanged()
				return true
			}
		}
		return false
	case surface.EvResize:
		cw.lastWinW = ev.Size.W
		cw.lastWinH = ev.Size.H
		cw.toolbar.SetBounds(ev.Size.W)
		cw.notifyAccessibilityChanged()
		contentDevH := ev.Size.H - cw.chromeHeight()

		if contentDevH < 0 {
			contentDevH = 0
		}
		if cw.onResize != nil {
			// The shim reports device pixels; the host keeps logical sizes and
			// multiplies by the DPR itself.
			sc := int(cw.sc())
			cw.onResize(int(ev.Size.W)/sc, int(contentDevH)/sc)
		}
		// The loop's surface is the content viewport, not the whole window, so
		// it must learn the content size: forwarding the window size makes the
		// scheduler size a buffer this method then grows again by the chrome
		// band, and the platform stretches that over-long bitmap to fit -
		// putting every pointer event's hit below the pixels it names. The
		// rewritten event ends the pump's read chain, so the resize is consumed
		// here rather than returned to the pump.
		cw.events <- surface.Event{Kind: surface.EvResize, Size: frame.Size{W: ev.Size.W, H: contentDevH}, Scale: ev.Scale}
		return true
	default:
		return false
	}
}

// updateHoverCursor resolves the pointer shape for buttonless motion: the
// toolbar hit tests its own buttons and bar, the document area defers to the
// host's resolver, and everywhere else asks for the default arrow. The shape
// is only sent to the platform when it changes, because motion arrives per
// pixel and each call crosses into platform code.
func (cw *chromeWindow) updateHoverCursor(pos frame.Point, chromeH int32) {
	c := surface.CursorDefault
	switch {
	case pos.Y >= chromeH && cw.onHoverCursor != nil:
		c = cw.onHoverCursor(pos.X, pos.Y-cw.chromeHeight())
	case cw.showBookmarksBar && pos.Y >= int32(totalChromeHeight)*cw.sc() && pos.Y < chromeH:
		if cw.hoverBookmark >= 0 {
			c = surface.CursorPointer
		} else {
			c = surface.CursorDefault
		}
	case pos.Y >= int32(tabs.TabBarHeight)*cw.sc() && pos.Y < int32(totalChromeHeight)*cw.sc():
		adjusted := pos
		adjusted.Y -= int32(tabs.TabBarHeight) * cw.sc()
		c = cw.toolbar.CursorAt(adjusted)
	}
	if c == cw.hover {
		return
	}
	cw.hover = c
	if cw.Window != nil {
		cw.Window.SetCursor(c)
	}
}

// stripGeom returns the tab strip's tab list, count, logical width and
// clamped scroll offset. A non-positive width means nothing is painted yet
// and there is nothing to hit.
func (cw *chromeWindow) stripGeom() (tabList []*tabs.Tab, n, stripW, stripOff int32) {
	if cw.toolbar != nil {
		stripW = cw.toolbar.BoundsWidth() / cw.sc()
	}
	if stripW <= 0 {
		return nil, 0, 0, 0
	}
	tabList = cw.tabMgr.Tabs()
	n = int32(len(tabList))
	stripOff = cw.tabMgr.TabScroll()
	if max := tabs.StripMaxOffset(n, stripW); stripOff > max {
		stripOff = max
	}
	return tabList, n, stripW, stripOff
}

func (cw *chromeWindow) handleTabBarPress(pos frame.Point) {
	lp := frame.Point{X: pos.X / cw.sc(), Y: pos.Y / cw.sc()}
	// The strip is as wide as the window: the toolbar bounds carry the same
	// device width DrawTabBar painted at, so clicks map onto the tabs the
	// pixels show. Without painted geometry yet there is nothing to hit.
	tabList, n, stripW, stripOff := cw.stripGeom()
	if stripW <= 0 {
		return
	}
	// The scroll chevrons overlay the strip's right end, so they win ties
	// against the tabs sliding underneath. Same rects DrawTabBar painted.
	if left, right, ok := tabs.ChevronRects(n, stripW); ok {
		if lp.X >= left.X0 && lp.X < left.X1 && lp.Y >= left.Y0 && lp.Y < left.Y1 {
			cw.tabMgr.SetTabScroll(stripOff-tabs.TabScrollPage(n, stripW), n, stripW)
			cw.notifyAccessibilityChanged()
			return
		}
		if lp.X >= right.X0 && lp.X < right.X1 && lp.Y >= right.Y0 && lp.Y < right.Y1 {
			cw.tabMgr.SetTabScroll(stripOff+tabs.TabScrollPage(n, stripW), n, stripW)
			cw.notifyAccessibilityChanged()
			return
		}
	}
	for i := range tabList {
		r := tabs.TabRect(i, stripOff, n, stripW)
		if lp.X >= r.X0 && lp.X < r.X1 && lp.Y >= r.Y0 && lp.Y < r.Y1 {
			if !tabList[i].Pinned() {
				closeR := tabs.CloseButtonRect(r)
				if lp.X >= closeR.X0 && lp.X < closeR.X1 && lp.Y >= closeR.Y0 && lp.Y < closeR.Y1 {
					if cw.onCloseTab != nil {
						cw.onCloseTab(tabList[i].ID())
					}
					return
				}
			}
			if cw.onSwitch != nil {
				cw.onSwitch(tabList[i].ID())
			}
			cw.pressing = true
			cw.pressTab = tabList[i].ID()
			cw.pressX = lp.X
			return
		}
	}
	btnR := tabs.NewTabButtonRect(n, stripOff, stripW)
	if lp.X >= btnR.X0 && lp.X < btnR.X1 && lp.Y >= btnR.Y0 && lp.Y < btnR.Y1 {
		if cw.onNewTab != nil {
			cw.onNewTab()
		}
	}
}

// handleTabBarDrag extends a press into a reorder once it travels past the
// threshold; smaller motions stay clicks. The dragged tab follows the
// pointer through the manager state DrawTabBar reads.
func (cw *chromeWindow) handleTabBarDrag(pos frame.Point) {
	if !cw.pressing {
		return
	}
	lp := frame.Point{X: pos.X / cw.sc(), Y: pos.Y / cw.sc()}
	_, n, stripW, _ := cw.stripGeom()
	if stripW <= 0 || n <= 0 {
		return
	}
	dx := lp.X - cw.pressX
	if dx < 0 {
		dx = -dx
	}
	if _, _, dragging := cw.tabMgr.Dragging(); !dragging {
		if dx < tabs.DragThreshold {
			return
		}
		cw.tabMgr.StartDrag(cw.pressTab, lp.X)
		return
	}
	cw.tabMgr.MoveDrag(lp.X)
}

// handleTabBarRelease commits a drag to its drop slot and ends the gesture.
// A release without a drag is a no-op: the press already switched, closed
// or opened whatever it hit.
func (cw *chromeWindow) handleTabBarRelease(pos frame.Point) {
	cw.pressing = false
	id, x, dragging := cw.tabMgr.Dragging()
	if !dragging {
		return
	}
	cw.tabMgr.EndDrag()
	_, n, stripW, stripOff := cw.stripGeom()
	if stripW <= 0 || n <= 0 {
		return
	}
	if cw.tabMgr.MoveTab(id, int(tabs.DropIndexAt(x, n, stripW, stripOff))) {
		// The moved tab is active (press switched first): keep it visible
		// the same way a keyboard switch does.
		cw.tabMgr.SetTabScroll(tabs.EnsureTabVisible(cw.tabMgr.ActiveIndex(), n, stripW, cw.tabMgr.TabScroll()), n, stripW)
	}
}

func (cw *chromeWindow) handleTabShortcut(key rune, mods surface.KeyMod) bool {
	cmd := mods&surface.ModCommand != 0
	if !cmd {
		return false
	}
	shift := mods&surface.ModShift != 0

	switch {
	case key == 'l' || key == 'L':
		if cw.toolbar != nil {
			cw.toolbar.FocusURL()
			cw.SetIME(false)
			cw.notifyAccessibilityChanged()
			return true
		}
	case key == 't' || key == 'T':
		if shift {
			if cw.onReopenTab != nil {
				cw.onReopenTab()
				return true
			}
		} else if cw.onNewTab != nil {
			cw.onNewTab()
			return true
		}
	case (key == 'n' || key == 'N') && shift:
		if cw.onNewIncognitoTab != nil {
			cw.onNewIncognitoTab()
			return true
		}
	case key == 'p' || key == 'P':
		if shift {
			if cw.onNewIncognitoTab != nil {
				cw.onNewIncognitoTab()
				return true
			}
		} else {
			if cw.onPrintPDF != nil {
				cw.onPrintPDF()
				return true
			}
		}
	case (key == 'b' || key == 'B') && shift:
		cw.toggleBookmarksBar()
		return true
	case key == 'f' || key == 'F':
		if shift {
			cw.toggleFullscreen()
			return true
		}
		if cw.toolbar != nil {
			cw.toolbar.OpenFind()
			cw.SetIME(false)
			cw.notifyAccessibilityChanged()
			return true
		}
	case (key == 'm' || key == 'M') && shift:
		if cw.onDeviceEmulation != nil {
			cw.onDeviceEmulation()
			return true
		}

	case key == 'd' || key == 'D':
		if shift {
			if cw.onDuplicateTab != nil {
				cw.onDuplicateTab()
				return true
			}
		} else if cw.onBookmark != nil {
			cw.onBookmark()
			return true
		}
	case key == 'r' || key == 'R':
		if shift {
			if cw.onReaderMode != nil {
				cw.onReaderMode()
				return true
			}
		}
	case (key == 'i' || key == 'I') && (mods&surface.ModOption != 0):
		if cw.onInspect != nil {
			cw.onInspect()
			return true
		}
	case (key == 'u' || key == 'U') && (mods&surface.ModOption != 0):
		if cw.onViewSource != nil {
			cw.onViewSource()
			return true
		}
	case key == 'c' || key == 'C':
		if cw.onCopy != nil && cw.onCopy() {
			return true
		}
	case key == 'w' || key == 'W':
		active := cw.tabMgr.Active()
		if active != nil && cw.onCloseTab != nil {
			cw.onCloseTab(active.ID())
			return true
		}
	case key == '\t':
		tabList := cw.tabMgr.Tabs()
		if len(tabList) <= 1 {
			return true
		}
		idx := cw.tabMgr.ActiveIndex()
		if shift {
			idx = (idx - 1 + len(tabList)) % len(tabList)
		} else {
			idx = (idx + 1) % len(tabList)
		}
		if cw.onSwitch != nil {
			cw.onSwitch(tabList[idx].ID())
		}
		return true
	case key >= '1' && key <= '9':
		tabList := cw.tabMgr.Tabs()
		n := int(key - '1')
		if n < len(tabList) && cw.onSwitch != nil {
			cw.onSwitch(tabList[n].ID())
			return true
		}
	case key == '=' || key == '+':
		if cw.onZoom != nil {
			cw.onZoom(0.1)
			return true
		}
	case key == '-':
		if cw.onZoom != nil {
			cw.onZoom(-0.1)
			return true
		}
	case key == '0':
		if cw.onZoom != nil {
			cw.onZoom(0)
			return true
		}
	}
	return false
}

// addressEditKey reports whether a key event is an address-bar edit shortcut
// that must reach the focused URL bar rather than a browser shortcut. Cmd+C
// with the bar focused copies the URL selection; without this priority the
// content copy in handleTabShortcut would steal it.
func addressEditKey(key rune, mods surface.KeyMod) bool {
	if mods&(surface.ModCommand|surface.ModControl) == 0 {
		return false
	}
	switch key {
	case 'a', 'A', 'c', 'C', 'v', 'V', 'x', 'X', 'z', 'Z', 'y', 'Y':
		return true
	}
	return false
}

func (cw *chromeWindow) drawBookmarksBar(buf *frame.Bitmap) {
	scale := cw.sc()
	barY := int32(totalChromeHeight) * scale
	barH := int32(bookmarksBarHeight) * scale
	barW := int32(buf.W)

	// Background and 1px separator line at bottom
	buf.FillRect(frame.Rect4(0, barY, barW, barY+barH), frame.RGB(248, 249, 250), nil)
	buf.FillRect(frame.Rect4(0, barY+barH-scale, barW, barY+barH), frame.RGB(218, 220, 224), nil)

	pills := cw.bookmarkPills()
	fontSize := int32(11 * scale)
	textCol := frame.RGB(60, 64, 67)
	hoverBg := frame.RGB(232, 240, 254)
	hoverText := frame.RGB(26, 115, 232)

	for i, p := range pills {
		col := textCol
		if i == cw.hoverBookmark {
			buf.FillRounded(p.rect, pillCorners(float32(4*scale)), hoverBg, nil)
			col = hoverText
		}
		textX := p.rect.X0 + 6*scale
		textY := p.rect.Y0 + (p.rect.H()-fontSize)/2 + fontSize*4/5
		drawMenuText(buf, cw.fonts, p.label, textX, textY, fontSize, col)
	}
}

func pillCorners(r float32) frame.Corners {
	rad := frame.Radius{X: r, Y: r}
	return frame.Corners{TL: rad, TR: rad, BR: rad, BL: rad}
}

type bookmarkPill struct {
	entry bookmarks.Entry
	label string
	rect  frame.Rect
}

func (cw *chromeWindow) bookmarkPills() []bookmarkPill {
	if cw.bookmarks == nil {
		return nil
	}
	scale := cw.sc()
	barY := int32(totalChromeHeight) * scale
	barH := int32(bookmarksBarHeight) * scale
	pillH := int32(20) * scale
	pillY := barY + (barH-pillH)/2
	pillX := int32(8) * scale
	fontSize := int32(11 * scale)

	entries := cw.bookmarks.List()
	pills := make([]bookmarkPill, 0, len(entries))

	maxW := int32(1440) * scale
	if cw.toolbar != nil && cw.toolbar.BoundsWidth() > 0 {
		maxW = cw.toolbar.BoundsWidth()
	}
	if cw.chromeBitmap != nil {
		maxW = int32(cw.chromeBitmap.W)
	}

	for _, bm := range entries {
		label := bm.Title
		if label == "" {
			label = bm.URL
		}
		if len(label) > 20 {
			label = label[:18] + "..."
		}
		label = "★ " + label
		textW := cw.measureTextWidth(label, fontSize)
		pillW := textW + 12*scale
		if pillX+pillW > maxW-8*scale {
			break
		}
		rect := frame.Rect4(pillX, pillY, pillX+pillW, pillY+pillH)
		pills = append(pills, bookmarkPill{
			entry: bm,
			label: label,
			rect:  rect,
		})
		pillX += pillW + 4*scale
	}
	return pills
}

func (cw *chromeWindow) hitBookmark(pos frame.Point) (bookmarks.Entry, bool) {
	pills := cw.bookmarkPills()
	for _, p := range pills {
		if pos.X >= p.rect.X0 && pos.X < p.rect.X1 && pos.Y >= p.rect.Y0 && pos.Y < p.rect.Y1 {
			return p.entry, true
		}
	}
	return bookmarks.Entry{}, false
}

func (cw *chromeWindow) updateBookmarkHover(pos frame.Point) bool {
	pills := cw.bookmarkPills()
	oldHover := cw.hoverBookmark
	cw.hoverBookmark = -1
	for i, p := range pills {
		if pos.X >= p.rect.X0 && pos.X < p.rect.X1 && pos.Y >= p.rect.Y0 && pos.Y < p.rect.Y1 {
			cw.hoverBookmark = i
			break
		}
	}
	return cw.hoverBookmark != oldHover
}

func (cw *chromeWindow) measureTextWidth(text string, size int32) int32 {
	if cw.fonts == nil {
		return int32(len(text)) * size / 2
	}
	var penFixed int32
	for _, rn := range text {
		penFixed += cw.fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
	}
	return (penFixed + 32) >> 6
}

func (cw *chromeWindow) notifyContentResize() {
	if cw.Window == nil {
		return
	}
	var winW, winH int32
	if cw.lastWinW > 0 && cw.lastWinH > 0 {
		winW = cw.lastWinW
		winH = cw.lastWinH
	} else if cw.outBitmap != nil {
		winW = int32(cw.outBitmap.W)
		winH = int32(cw.outBitmap.H)
	} else if cw.chromeBitmap != nil {
		winW = int32(cw.chromeBitmap.W)
		winH = int32(cw.chromeBitmap.W) * 3 / 4
	} else {
		winW = 800 * cw.sc()
		winH = 600 * cw.sc()
	}

	contentDevH := winH - cw.chromeHeight()
	if contentDevH < 0 {
		contentDevH = 0
	}
	sc := int(cw.sc())
	if cw.onResize != nil {
		cw.onResize(int(winW)/sc, int(contentDevH)/sc)
	}
	if cw.events != nil {
		cw.events <- surface.Event{
			Kind:  surface.EvResize,
			Size:  frame.Size{W: winW, H: contentDevH},
			Scale: float32(cw.sc()),
		}
	}
}

func (cw *chromeWindow) toggleBookmarksBar() {
	cw.showBookmarksBar = !cw.showBookmarksBar
	cw.notifyContentResize()
	cw.notifyAccessibilityChanged()
}

func (cw *chromeWindow) toggleFullscreen() {
	cw.fullscreen = !cw.fullscreen
	cw.notifyContentResize()
	cw.notifyAccessibilityChanged()
	if cw.onFullscreenChanged != nil {
		cw.onFullscreenChanged(cw.fullscreen)
	}
}

func (cw *chromeWindow) notifyAccessibilityChanged() {
	if cw.onAccessibilityChanged != nil {
		cw.onAccessibilityChanged()
	}
}

func (cw *chromeWindow) IsFullscreen() bool {
	return cw.fullscreen
}

func (cw *chromeWindow) SetFullscreen(fs bool) {
	if cw.fullscreen != fs {
		cw.toggleFullscreen()
	}
}

var _ surface.Window = (*chromeWindow)(nil)
var _ surface.AccessibilityWindow = (*chromeWindow)(nil)

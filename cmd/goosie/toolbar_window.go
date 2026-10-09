package main

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const totalChromeHeight = tabs.TabBarHeight + toolbar.ToolbarHeight

type chromeWindow struct {
	surface.Window
	toolbar       *toolbar.State
	tabMgr        *tabs.TabManager
	scale         int32
	events        chan surface.Event
	onSwitch      func(uint64)
	onNewTab      func()
	onCloseTab    func(uint64)
	onResize      func(contentW, contentH int)
	onZoom        func(delta float64)
	onLinkClick   func(href string)
	onBookmark    func()
	onFocusClick  func(contentX, contentY int32) bool
	onContentKey  func(key rune, mods surface.KeyMod) bool
	onDragSelect  func(action surface.PointerAction, contentX, contentY int32) bool
	onIME         func(ev surface.Event) bool
	onCopy        func() bool
	hitTestLink   func(contentX, contentY int32) string
	onHoverCursor func(contentX, contentY int32) surface.Cursor
	chromeBitmap  *frame.Bitmap
	outBitmap     *frame.Bitmap
	fonts         *raster.Fonts
	hover         surface.Cursor // last shape sent to the platform, for change detection
}

func newChromeWindow(w surface.Window, tb *toolbar.State, mgr *tabs.TabManager,
	onSwitch func(uint64), onNewTab func(), onCloseTab func(uint64),
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

func (cw *chromeWindow) chromeHeight() int32 {
	return int32(totalChromeHeight) * cw.sc()
}

func (cw *chromeWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	chromeH := cw.chromeHeight()
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

	// The loop's backing store is the content viewport, so buf is window-minus-
	// chrome tall. The window surface is the whole window, so the presented
	// bitmap has to carry the chrome rows on top and the content rows shifted
	// down by chromeH; presenting buf directly would show a short frame and let
	// the chrome paint over the top of the document.
	outH := int32(buf.H) + chromeH
	if cw.outBitmap == nil || int32(cw.outBitmap.W) != int32(buf.W) || int32(cw.outBitmap.H) != outH {
		cw.outBitmap = frame.NewBitmap(buf.W, int(outH))
	}
	out := cw.outBitmap

	for y := int32(0); y < chromeH; y++ {
		copy(out.RGBA[int(y)*out.Stride:int(y)*out.Stride+out.Stride], cw.chromeBitmap.RGBA[int(y)*cw.chromeBitmap.Stride:int(y)*cw.chromeBitmap.Stride+cw.chromeBitmap.Stride])
	}
	for y := int32(0); y < int32(buf.H); y++ {
		oy := int(y+chromeH) * out.Stride
		sy := int(y) * buf.Stride
		copy(out.RGBA[oy:oy+out.Stride], buf.RGBA[sy:sy+buf.Stride])
	}

	merged := make([]frame.Rect, 0, len(damage)+1)
	merged = append(merged, frame.Rect4(0, 0, int32(out.W), chromeH))
	for _, r := range damage {
		if r.Empty() {
			continue
		}
		merged = append(merged, frame.Rect4(r.X0, r.Y0+chromeH, r.X1, r.Y1+chromeH))
	}
	return cw.Window.Present(out, merged)
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
	for ev := range cw.Window.Events() {
		if cw.intercept(ev) {
			continue
		}
		cw.events <- ev
	}
}

func (cw *chromeWindow) intercept(ev surface.Event) bool {
	switch ev.Kind {
	case surface.EvPointer:
		tabH := int32(tabs.TabBarHeight) * cw.sc()
		toolH := int32(toolbar.ToolbarHeight) * cw.sc()
		if ev.Action == surface.PointerMotion && ev.Button == surface.ButtonNone {
			cw.updateHoverCursor(ev.Pos, tabH+toolH)
		}
		if ev.Pos.Y < tabH && ev.Button == surface.ButtonLeft {
			cw.handleTabBarClick(ev.Pos)
			return true
		}
		if ev.Pos.Y < tabH {
			return true
		}
		toolbarY := ev.Pos.Y - tabH
		if toolbarY >= 0 && toolbarY < toolH && ev.Button == surface.ButtonLeft {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
			if cw.toolbar.GetFocus() == toolbar.FocusAddress {
				cw.SetIME(false)
			}
			return true
		}
		if toolbarY >= 0 && toolbarY < toolH {
			return true
		}
		if cw.toolbar.GetFocus() == toolbar.FocusAddress {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
		}
		// Link and focus handling is a press concern; a drag's motion and
		// release must not re-trigger them. Whatever a press leaves behind goes
		// to the drag selector, which decides per action whether to consume it.
		contentX := ev.Pos.X
		contentY := ev.Pos.Y - cw.chromeHeight()
		press := ev.Action == surface.PointerPress && ev.Button == surface.ButtonLeft
		if press {
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
		if cw.handleTabShortcut(ev.Key, ev.Mods) {
			return true
		}
		if cw.toolbar.GetFocus() == toolbar.FocusAddress {
			cw.toolbar.HandleKeyEvent(ev.Key, ev.Mods)
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
		if ev.Pos.Y < tabH && cw.toolbar != nil {
			stripW := cw.toolbar.BoundsWidth() / cw.sc()
			n := int32(cw.tabMgr.Count())
			if tabs.StripMaxOffset(n, stripW) > 0 {
				d := ev.Delta.X + ev.Delta.Y
				cw.tabMgr.SetTabScroll(cw.tabMgr.TabScroll()+d/cw.sc(), n, stripW)
				return true
			}
		}
		return false
	case surface.EvResize:
		cw.toolbar.SetBounds(ev.Size.W)
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
	case pos.Y >= int32(tabs.TabBarHeight)*cw.sc() && pos.Y < chromeH:
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

func (cw *chromeWindow) handleTabBarClick(pos frame.Point) {
	lp := frame.Point{X: pos.X / cw.sc(), Y: pos.Y / cw.sc()}
	// The strip is as wide as the window: the toolbar bounds carry the same
	// device width DrawTabBar painted at, so clicks map onto the tabs the
	// pixels show. Without painted geometry yet there is nothing to hit.
	var stripW int32
	if cw.toolbar != nil {
		stripW = cw.toolbar.BoundsWidth() / cw.sc()
	}
	if stripW <= 0 {
		return
	}
	tabList := cw.tabMgr.Tabs()
	n := int32(len(tabList))
	stripOff := cw.tabMgr.TabScroll()
	if max := tabs.StripMaxOffset(n, stripW); stripOff > max {
		stripOff = max
	}
	// The scroll chevrons overlay the strip's right end, so they win ties
	// against the tabs sliding underneath. Same rects DrawTabBar painted.
	if left, right, ok := tabs.ChevronRects(n, stripW); ok {
		if lp.X >= left.X0 && lp.X < left.X1 && lp.Y >= left.Y0 && lp.Y < left.Y1 {
			cw.tabMgr.SetTabScroll(stripOff-tabs.TabScrollPage(n, stripW), n, stripW)
			return
		}
		if lp.X >= right.X0 && lp.X < right.X1 && lp.Y >= right.Y0 && lp.Y < right.Y1 {
			cw.tabMgr.SetTabScroll(stripOff+tabs.TabScrollPage(n, stripW), n, stripW)
			return
		}
	}
	for i := range tabList {
		r := tabs.TabRect(i, stripOff, n, stripW)
		if lp.X >= r.X0 && lp.X < r.X1 && lp.Y >= r.Y0 && lp.Y < r.Y1 {
			closeR := tabs.CloseButtonRect(r)
			if lp.X >= closeR.X0 && lp.X < closeR.X1 && lp.Y >= closeR.Y0 && lp.Y < closeR.Y1 {
				if cw.onCloseTab != nil {
					cw.onCloseTab(tabList[i].ID())
				}
				return
			}
			if cw.onSwitch != nil {
				cw.onSwitch(tabList[i].ID())
			}
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

func (cw *chromeWindow) handleTabShortcut(key rune, mods surface.KeyMod) bool {
	cmd := mods&surface.ModCommand != 0
	if !cmd {
		return false
	}
	shift := mods&surface.ModShift != 0

	switch {
	case key == 't' || key == 'T':
		if cw.onNewTab != nil {
			cw.onNewTab()
			return true
		}
	case key == 'f' || key == 'F':
		if cw.toolbar != nil {
			cw.toolbar.OpenFind()
			return true
		}
	case key == 'd' || key == 'D':
		if cw.onBookmark != nil {
			cw.onBookmark()
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

var _ surface.Window = (*chromeWindow)(nil)

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
	toolbar      *toolbar.State
	tabMgr       *tabs.TabManager
	scale        int32
	events       chan surface.Event
	onSwitch     func(uint64)
	onNewTab     func()
	onCloseTab   func(uint64)
	onResize     func(contentW, contentH int)
	onZoom       func(delta float64)
	onLinkClick  func(href string)
	onBookmark   func()
	onFocusClick func(contentX, contentY int32) bool
	onContentKey func(key rune, mods surface.KeyMod) bool
	onDragSelect func(action surface.PointerAction, contentX, contentY int32) bool
	onIME        func(ev surface.Event) bool
	onCopy       func() bool
	hitTestLink  func(contentX, contentY int32) string
	chromeBitmap *frame.Bitmap
	fonts        *raster.Fonts
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
	fonts *raster.Fonts, scale int32) *chromeWindow {
	if scale < 1 {
		scale = 1
	}
	cw := &chromeWindow{
		Window:       w,
		toolbar:      tb,
		tabMgr:       mgr,
		scale:        scale,
		events:       make(chan surface.Event, 64),
		onSwitch:     onSwitch,
		onNewTab:     onNewTab,
		onCloseTab:   onCloseTab,
		onResize:     onResize,
		onZoom:       onZoom,
		onLinkClick:  onLinkClick,
		onBookmark:   onBookmark,
		onFocusClick: onFocusClick,
		onContentKey: onContentKey,
		onDragSelect: onDragSelect,
		onIME:        onIME,
		onCopy:       onCopy,
		hitTestLink:  hitTestLink,
		fonts:        fonts,
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
	tabs.DrawTabBar(cw.chromeBitmap, cw.tabMgr, 0, cw.fonts, cw.sc())
	cw.toolbar.Draw(cw.chromeBitmap, int32(tabs.TabBarHeight)*cw.sc())

	for y := int32(0); y < chromeH && y < int32(buf.H); y++ {
		copy(buf.RGBA[int(y)*buf.Stride:int(y)*buf.Stride+buf.Stride], cw.chromeBitmap.RGBA[int(y)*cw.chromeBitmap.Stride:int(y)*cw.chromeBitmap.Stride+cw.chromeBitmap.Stride])
	}

	chromeRect := frame.Rect4(0, 0, int32(buf.W), chromeH)
	merged := append(damage[:len(damage):len(damage)], chromeRect)
	return cw.Window.Present(buf, merged)
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
			if cw.toolbar.Focus == toolbar.FocusAddress {
				cw.SetIME(false)
			}
			return true
		}
		if toolbarY >= 0 && toolbarY < toolH {
			return true
		}
		if cw.toolbar.Focus == toolbar.FocusAddress {
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
		if cw.toolbar.Focus == toolbar.FocusAddress {
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
	case surface.EvResize:
		cw.toolbar.SetBounds(ev.Size.W)
		if cw.onResize != nil {
			// The shim reports device pixels; the host keeps logical sizes and
			// multiplies by the DPR itself.
			sc := int(cw.sc())
			contentH := int(ev.Size.H) - int(cw.chromeHeight())
			if contentH < 0 {
				contentH = 0
			}
			cw.onResize(int(ev.Size.W)/sc, contentH/sc)
		}
		return false
	default:
		return false
	}
}

func (cw *chromeWindow) handleTabBarClick(pos frame.Point) {
	lp := frame.Point{X: pos.X / cw.sc(), Y: pos.Y / cw.sc()}
	tabList := cw.tabMgr.Tabs()
	for i := range tabList {
		r := tabs.TabRect(i, 0, int32(len(tabList)), lp.X+100)
		if lp.X >= r.X0 && lp.X < r.X1 && lp.Y >= r.Y0 && lp.Y < r.Y1 {
			closeR := tabs.CloseButtonRect(r)
			if lp.X >= closeR.X0 && lp.X < closeR.X1 && lp.Y >= closeR.Y0 && lp.Y < closeR.Y1 {
				if cw.onCloseTab != nil {
					cw.onCloseTab(tabList[i].ID)
				}
				return
			}
			if cw.onSwitch != nil {
				cw.onSwitch(tabList[i].ID)
			}
			return
		}
	}
	btnR := tabs.NewTabButtonRect(int32(len(tabList)), 0, lp.X+100)
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
			cw.onCloseTab(active.ID)
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
			cw.onSwitch(tabList[idx].ID)
		}
		return true
	case key >= '1' && key <= '9':
		tabList := cw.tabMgr.Tabs()
		n := int(key - '1')
		if n < len(tabList) && cw.onSwitch != nil {
			cw.onSwitch(tabList[n].ID)
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

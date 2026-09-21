package main

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

const totalChromeHeight = tabs.TabBarHeight + toolbar.ToolbarHeight

type chromeWindow struct {
	surface.Window
	toolbar    *toolbar.State
	tabMgr     *tabs.TabManager
	events     chan surface.Event
	onSwitch   func(uint64)
	onNewTab   func()
	onCloseTab func(uint64)
}

func newChromeWindow(w surface.Window, tb *toolbar.State, mgr *tabs.TabManager,
	onSwitch func(uint64), onNewTab func(), onCloseTab func(uint64)) *chromeWindow {
	cw := &chromeWindow{
		Window:     w,
		toolbar:    tb,
		tabMgr:     mgr,
		events:     make(chan surface.Event, 64),
		onSwitch:   onSwitch,
		onNewTab:   onNewTab,
		onCloseTab: onCloseTab,
	}
	go cw.pump()
	return cw
}

func (cw *chromeWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
	tabs.DrawTabBar(buf, cw.tabMgr, 0)
	cw.toolbar.Draw(buf, tabs.TabBarHeight)
	chromeRect := frame.Rect4(0, 0, int32(buf.W), totalChromeHeight)
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
		if ev.Pos.Y < int32(tabs.TabBarHeight) && ev.Button == surface.ButtonLeft {
			cw.handleTabBarClick(ev.Pos)
			return true
		}
		if ev.Pos.Y < int32(tabs.TabBarHeight) {
			return true
		}
		toolbarY := ev.Pos.Y - int32(tabs.TabBarHeight)
		if toolbarY >= 0 && toolbarY < toolbar.ToolbarHeight && ev.Button == surface.ButtonLeft {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
			return true
		}
		if toolbarY >= 0 && toolbarY < toolbar.ToolbarHeight {
			return true
		}
		if cw.toolbar.Focus == toolbar.FocusAddress {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
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
		return false
	case surface.EvResize:
		cw.toolbar.SetBounds(ev.Size.W)
		return false
	default:
		return false
	}
}

func (cw *chromeWindow) handleTabBarClick(pos frame.Point) {
	tabList := cw.tabMgr.Tabs()
	for i := range tabList {
		r := tabs.TabRect(i, 0, int32(len(tabList)), pos.X+100)
		if pos.X >= r.X0 && pos.X < r.X1 && pos.Y >= r.Y0 && pos.Y < r.Y1 {
			closeR := tabs.CloseButtonRect(r)
			if pos.X >= closeR.X0 && pos.X < closeR.X1 && pos.Y >= closeR.Y0 && pos.Y < closeR.Y1 {
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
	btnR := tabs.NewTabButtonRect(int32(len(tabList)), 0, pos.X+100)
	if pos.X >= btnR.X0 && pos.X < btnR.X1 && pos.Y >= btnR.Y0 && pos.Y < btnR.Y1 {
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
	}
	return false
}

var _ surface.Window = (*chromeWindow)(nil)

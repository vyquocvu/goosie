# Independent Tab/Document Lifecycle Design

**Date:** 2026-09-23  
**Status:** Proposed  
**Gate:** Roadmap v2 Gate 2

## Problem Statement

The current tab implementation has three critical issues that prevent race-free, independent tab navigation:

**R3 - Tabs are not independently navigable:**
- Navigation cancellation (`navCancel`) is window-scoped, not per-tab
- Navigating one tab cancels all other tabs' in-flight requests
- Tab switching increments global `navSerial`, invalidating other tabs' navigation
- `newTab()` creates a tab but doesn't activate it (only first tab becomes active)

**R4 - Mutable UI state has multiple owners:**
- Navigation goroutines mutate tab fields and toolbar state directly
- `Present()` reads toolbar state while navigation goroutines write it
- `syncTabToToolbar()` replaces `Input` without resetting `Cursor`/selection
- Collection locking in `TabManager` doesn't protect mutable fields on returned tabs

**R5 - Chrome and document presentation are not separated:**
- `Present()` paints chrome directly over the composed document buffer
- Scroll fast path shifts retained backing pixels, including chrome
- No reserved content viewport for the 76-device-pixel chrome overlay
- Chrome can contaminate subsequent document output

## Solution Overview

Implement per-tab navigation controllers, event-based state updates, and chrome/content separation to establish single-owner state and true tab independence.

## Design

### 1. Per-Tab Navigation Controller

Each `Tab` owns its own `navController` with independent context and cancellation.

```go
type navController struct {
    mu      sync.Mutex
    cancel  context.CancelFunc
    serial  uint64
    loading bool
}

type Tab struct {
    ID      uint64
    URL     string
    Title   string
    History *toolbar.History
    ScrollY int32
    Layer   *frame.Layer
    BGColor frame.Color
    Error   string
    
    nav navController  // Per-tab navigation state
}
```

**Key changes:**
- `navController` moves from `framePath` to `Tab`
- Each tab has independent context/cancellation
- Navigating tab A doesn't affect tab B's in-flight requests
- Tab switching doesn't invalidate other tabs' navigation

**Navigation flow:**
```go
func (f *framePath) navigateTab(rawURL string) {
    tab := f.tabMgr.Active()
    if tab == nil {
        return
    }
    
    // Cancel only THIS tab's navigation
    tab.nav.cancel()
    ctx, cancel := context.WithCancel(context.Background())
    tab.nav.cancel = cancel
    serial := tab.nav.serial + 1
    tab.nav.serial = serial
    tab.nav.loading = true
    
    go func() {
        layer, _, bgColor, err := loadURLCtx(ctx, ...)
        // Send result to event channel, don't mutate directly
        f.navResults <- navResult{
            tabID: tab.ID,
            serial: serial,
            layer: layer,
            bgColor: bgColor,
            err: err,
        }
    }()
}
```

### 2. Event-Based State Updates

Navigation results flow through a channel to the UI thread, eliminating races between goroutines and `Present`.

```go
type navResult struct {
    tabID   uint64
    serial  uint64
    layer   *frame.Layer
    bgColor frame.Color
    err     error
    url     string
}

type framePath struct {
    // ... existing fields ...
    navResults chan navResult  // Navigation results from goroutines
}
```

**UI thread event loop:**
```go
func (f *framePath) run() {
    for {
        select {
        case result := <-f.navResults:
            f.applyNavResult(result)
        case ev := <-f.window.Events():
            f.handleEvent(ev)
        }
    }
}

func (f *framePath) applyNavResult(result navResult) {
    tab := f.tabMgr.TabByID(result.tabID)
    if tab == nil || tab.nav.serial != result.serial {
        return  // Stale result, discard
    }
    
    // Update tab state on UI thread only
    tab.nav.loading = false
    if result.err != nil {
        tab.Error = result.err.Error()
        return
    }
    
    tab.Layer = result.layer
    tab.BGColor = result.bgColor
    tab.URL = result.url
    tab.Title = result.url
    tab.History.Push(result.url)
    
    // Only update display if this is the active tab
    if f.tabMgr.Active() == tab {
        f.syncTabToToolbar()
        f.sched.SetPlan(frame.FramePlan{
            Serial:     result.serial,
            Layers:     []*frame.Layer{result.layer},
            Background: result.bgColor,
        })
    }
}
```

**Benefits:**
- Single writer for tab/toolbar state (UI thread)
- `Present` reads state that's only modified on its own thread
- No locks needed for tab/toolbar access during rendering
- Stale results are discarded via serial check

### 3. Chrome/Content Separation

Chrome and content use separate bitmaps, composited in `Present`. This prevents chrome pixels from contaminating document output during scroll.

```go
type chromeWindow struct {
    surface.Window
    toolbar    *toolbar.State
    tabMgr     *tabs.TabManager
    events     chan surface.Event
    onSwitch   func(uint64)
    onNewTab   func()
    onCloseTab func(uint64)
    
    chromeBitmap *frame.Bitmap  // Separate chrome layer
    contentBitmap *frame.Bitmap // Document layer
}

func (cw *chromeWindow) Present(contentBuf *frame.Bitmap, damage []frame.Rect) error {
    // Resize bitmaps if needed
    if cw.chromeBitmap == nil || cw.chromeBitmap.W != contentBuf.W {
        cw.chromeBitmap = frame.NewBitmap(contentBuf.W, totalChromeHeight)
    }
    
    // Draw chrome to separate bitmap
    cw.chromeBitmap.FillRect(frame.Rect4(0, 0, int32(cw.chromeBitmap.W), int32(cw.chromeBitmap.H)), 
        frame.RGB(255, 255, 255), nil)
    tabs.DrawTabBar(cw.chromeBitmap, cw.tabMgr, 0)
    cw.toolbar.Draw(cw.chromeBitmap, tabs.TabBarHeight)
    
    // Composite: chrome on top, content below
    compositeBuf := contentBuf
    if contentBuf.H > totalChromeHeight {
        // Blit chrome
        for y := int32(0); y < totalChromeHeight; y++ {
            copy(compositeBuf.Row(y), cw.chromeBitmap.Row(y))
        }
    }
    
    // Mark chrome region as damaged
    chromeRect := frame.Rect4(0, 0, int32(contentBuf.W), totalChromeHeight)
    merged := append(damage[:len(damage):len(damage)], chromeRect)
    return cw.Window.Present(compositeBuf, merged)
}
```

**Scheduler viewport adjustment:**
```go
// In framePath.build(), adjust viewport to account for chrome
contentHeight := dev.H - totalChromeHeight
f.sched = raster.NewScheduler(layer, wp, frame.Viewport{
    Size: frame.Size{W: dev.W, H: contentHeight},
}, scale, raster.Pref{})
```

**Benefits:**
- Chrome pixels never enter the document tile cache
- Scroll operations shift only document pixels
- Chrome redraws independently of document scroll
- Clean separation matches the compositor architecture

### 4. Tab Activation and Presentation

Tab switching coordinates between the tab manager, navigation state, and display. The active tab's layer is displayed, but other tabs continue loading independently.

```go
func (f *framePath) switchTab(id uint64) {
    // Save current tab's scroll
    cur := f.tabMgr.Active()
    if cur != nil {
        vp := f.sched.Viewport()
        cur.ScrollY = vp.Offset.Y
    }
    
    // Switch active tab
    f.tabMgr.SwitchTo(id)
    newTab := f.tabMgr.Active()
    if newTab == nil {
        return
    }
    
    // Update toolbar to reflect new active tab
    f.syncTabToToolbar()
    
    // Display new tab's layer if it has one
    if newTab.Layer != nil {
        f.sched.SetPlan(frame.FramePlan{
            Serial:     newTab.nav.serial,
            Layers:     []*frame.Layer{newTab.Layer},
            Background: newTab.BGColor,
        })
    }
    
    // Restore new tab's scroll position
    f.sched.SetViewport(frame.Viewport{
        Offset: frame.Point{Y: newTab.ScrollY},
        Size:   f.config.devSize(),
    })
}

func (f *framePath) newTab() {
    if f.tabMgr == nil {
        return
    }
    
    // Save current tab's scroll
    cur := f.tabMgr.Active()
    if cur != nil {
        vp := f.sched.Viewport()
        cur.ScrollY = vp.Offset.Y
    }
    
    // Create and activate new tab
    tab := f.tabMgr.NewTab()
    f.tabMgr.SwitchTo(tab.ID)  // Explicitly activate
    
    // Initialize with blank layer
    spec := paint.SceneSpec{DocHeight: f.config.devSize().H}
    _, layer := paint.BuildLayer(spec)
    tab.Layer = layer
    tab.BGColor = frame.RGB(255, 255, 255)
    
    f.syncTabToToolbar()
    f.sched.SetPlan(frame.FramePlan{
        Serial:     tab.nav.serial,
        Layers:     []*frame.Layer{layer},
        Background: tab.BGColor,
    })
    f.sched.SetViewport(frame.Viewport{
        Offset: frame.Point{Y: 0},
        Size:   f.config.devSize(),
    })
}
```

**Key fixes:**
- `newTab` explicitly calls `SwitchTo` to activate the new tab
- Tab switching uses the tab's own serial, not a global one
- Each tab's scroll is preserved independently
- Navigation continues in background tabs without affecting display

**Toolbar cursor reset:**
```go
func (f *framePath) syncTabToToolbar() {
    if f.toolbar == nil || f.tabMgr == nil {
        return
    }
    tab := f.tabMgr.Active()
    if tab == nil {
        return
    }
    f.toolbar.URL = tab.URL
    f.toolbar.Input = tab.URL
    f.toolbar.Cursor = len([]rune(tab.URL))  // Reset cursor to end
    f.toolbar.SelectionEnd = -1              // Clear selection
    f.toolbar.History = tab.History
    f.toolbar.SetLoading(tab.nav.loading)
    f.toolbar.Error = tab.Error
}
```

## Testing Strategy

**Race detection:**
- All tests must pass with `-race` flag
- Test concurrent navigation across multiple tabs
- Test tab switching during active navigation
- Test rapid navigation on same tab

**Integration tests:**
- Navigate tab A, switch to tab B, verify tab A continues loading
- Navigate tab A, navigate tab A again, verify first navigation cancels
- Switch tabs during navigation, verify correct tab displays
- Close tab with in-flight navigation, verify cleanup

**Visual verification:**
- Verify chrome doesn't shift during document scroll
- Verify tab bar renders correctly with multiple tabs
- Verify toolbar input cursor behavior on tab switch

## Acceptance Criteria

- [ ] Each tab has independent navigation context and cancellation
- [ ] Navigating one tab doesn't cancel other tabs' requests
- [ ] Tab switching doesn't invalidate in-flight navigation
- [ ] New tabs are properly activated
- [ ] All tab/toolbar state mutations happen on UI thread
- [ ] No data races under `-race` detector
- [ ] Chrome pixels don't contaminate document scroll
- [ ] Toolbar cursor resets properly on tab switch
- [ ] Background tabs continue loading independently
- [ ] All existing tests pass
- [ ] New integration tests cover the above scenarios

## Deferred

This design addresses R3, R4, and R5 from the roadmap. The following remain for future gates:

- Aggregate fetch/font budgets and cancellation (Gate 4)
- Native buffer/thread/lifetime hardening (Gate 4)
- Origin, mixed-content, CSP rules (Gate 4)
- Links/forms/selection/accessibility (Gate 5)
- JavaScript runtime (Gate 6)

## Implementation Order

1. Add `navController` to `Tab` struct
2. Add `navResults` channel to `framePath`
3. Refactor navigation methods to use per-tab controllers
4. Implement event loop and `applyNavResult`
5. Add chrome/content bitmap separation
6. Fix `newTab` to explicitly activate
7. Reset toolbar cursor in `syncTabToToolbar`
8. Add integration tests under `-race`
9. Visual verification of scroll behavior

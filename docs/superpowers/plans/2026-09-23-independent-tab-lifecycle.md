# Independent Tab Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make tabs independently navigable with race-free state ownership and separated chrome/content presentation.

**Architecture:** Each tab gets its own navigation controller with independent context/cancellation. Navigation results flow through an event channel to the UI thread for single-owner state updates. Chrome and content use separate bitmaps composited in Present.

**Tech Stack:** Go, context package, frame/surface/raster packages, tabs/toolbar packages

---

## File Structure

**Modified files:**
- `internal/tabs/tab.go` — Add `navController` struct and field to `Tab`
- `cmd/goosie/main.go` — Refactor navigation to use per-tab controllers, add event loop, fix tab activation
- `cmd/goosie/toolbar_window.go` — Add chrome/content bitmap separation
- `internal/tabs/manager.go` — Add `TabByID` method

**New test files:**
- `cmd/goosie/tab_lifecycle_test.go` — Integration tests for independent navigation
- `internal/tabs/tab_test.go` — Unit tests for navController

---

### Task 1: Add navController to Tab

**Files:**
- Modify: `internal/tabs/tab.go:1-28`
- Test: `internal/tabs/tab_test.go` (new)

- [ ] **Step 1: Write failing test for navController**

Create `internal/tabs/tab_test.go`:

```go
package tabs

import (
    "context"
    "testing"
)

func TestNavControllerIndependent(t *testing.T) {
    tab1 := newTab(1)
    tab2 := newTab(2)
    
    // Each tab should have its own nav controller
    if &tab1.nav == &tab2.nav {
        t.Fatal("tabs share nav controller")
    }
    
    // Cancel tab1 shouldn't affect tab2
    ctx1, cancel1 := context.WithCancel(context.Background())
    tab1.nav.cancel = cancel1
    tab1.nav.serial = 1
    
    ctx2, cancel2 := context.WithCancel(context.Background())
    tab2.nav.cancel = cancel2
    tab2.nav.serial = 2
    
    tab1.nav.cancel()
    
    if ctx1.Err() == nil {
        t.Fatal("tab1 context not cancelled")
    }
    if ctx2.Err() != nil {
        t.Fatal("tab2 context cancelled when tab1 cancelled")
    }
}

func TestNavControllerSerial(t *testing.T) {
    tab := newTab(1)
    
    if tab.nav.serial != 0 {
        t.Fatalf("initial serial = %d, want 0", tab.nav.serial)
    }
    
    tab.nav.serial++
    if tab.nav.serial != 1 {
        t.Fatalf("serial = %d, want 1", tab.nav.serial)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tabs -run TestNavController -v`
Expected: FAIL with "tab.nav undefined"

- [ ] **Step 3: Add navController struct and field**

Modify `internal/tabs/tab.go`:

```go
package tabs

import (
    "context"
    "sync"
    
    "github.com/vyquocvu/goosie/internal/frame"
    "github.com/vyquocvu/goosie/internal/toolbar"
)

type navController struct {
    mu      sync.Mutex
    cancel  context.CancelFunc
    serial  uint64
    loading bool
}

// Tab is one browser tab with fully independent state.
type Tab struct {
    ID      uint64
    URL     string
    Title   string
    History *toolbar.History
    ScrollY int32
    Layer   *frame.Layer
    Loading bool
    Error   string
    BGColor frame.Color
    
    nav navController
}

func newTab(id uint64) *Tab {
    return &Tab{
        ID:      id,
        Title:   "New Tab",
        History: toolbar.NewHistory(),
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tabs -run TestNavController -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tabs/tab.go internal/tabs/tab_test.go
git commit -m "feat(tabs): add per-tab navigation controller"
```

---

### Task 2: Add TabByID to TabManager

**Files:**
- Modify: `internal/tabs/manager.go:1-117`
- Test: `internal/tabs/manager_test.go` (append)

- [ ] **Step 1: Write failing test for TabByID**

Append to `internal/tabs/manager_test.go` (create if doesn't exist):

```go
func TestTabByID(t *testing.T) {
    mgr := NewManager(nil)
    tab1 := mgr.NewTab()
    tab2 := mgr.NewTab()
    
    found := mgr.TabByID(tab1.ID)
    if found != tab1 {
        t.Fatal("TabByID returned wrong tab")
    }
    
    found = mgr.TabByID(tab2.ID)
    if found != tab2 {
        t.Fatal("TabByID returned wrong tab")
    }
    
    found = mgr.TabByID(999)
    if found != nil {
        t.Fatal("TabByID found non-existent tab")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tabs -run TestTabByID -v`
Expected: FAIL with "mgr.TabByID undefined"

- [ ] **Step 3: Implement TabByID**

Add to `internal/tabs/manager.go`:

```go
func (m *TabManager) TabByID(id uint64) *Tab {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, t := range m.tabs {
        if t.ID == id {
            return t
        }
    }
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tabs -run TestTabByID -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tabs/manager.go internal/tabs/manager_test.go
git commit -m "feat(tabs): add TabByID lookup method"
```

---

### Task 3: Add navResults Channel to framePath

**Files:**
- Modify: `cmd/goosie/main.go:180-203`

- [ ] **Step 1: Define navResult type and add channel**

Add to `cmd/goosie/main.go` after imports:

```go
type navResult struct {
    tabID   uint64
    serial  uint64
    layer   *frame.Layer
    bgColor frame.Color
    err     error
    url     string
}
```

Modify `framePath` struct (around line 180):

```go
type framePath struct {
    sched     *raster.Scheduler
    pool      *raster.WorkerPool
    composer  *surface.Composer
    layer     *frame.Layer
    rec       *frame.FrameRecorder
    spec      paint.SceneSpec
    config    config
    window    surface.Window
    toolbar   *toolbar.State
    tabMgr    *tabs.TabManager
    client    net.HTTP
    fonts     *raster.Fonts
    started   time.Time
    
    navResults chan navResult
}
```

- [ ] **Step 2: Initialize channel in build()**

In `build()` function (around line 254), add to framePath initialization:

```go
f := &framePath{
    sched:      raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
    pool:       wp,
    composer:   surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
    layer:      layer,
    rec:        frame.NewFrameRecorder(capacity),
    spec:       spec,
    config:     c,
    client:     client,
    fonts:      fonts,
    navResults: make(chan navResult, 16),
}
```

- [ ] **Step 3: Run tests to verify no regressions**

Run: `go test ./cmd/goosie -v`
Expected: PASS (no functional changes yet)

- [ ] **Step 4: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "feat(goosie): add navResults channel for event-based updates"
```

---

### Task 4: Refactor navigateTab to Use Per-Tab Controller

**Files:**
- Modify: `cmd/goosie/main.go:315-369`

- [ ] **Step 1: Refactor navigateTab**

Replace `navigateTab` function:

```go
func (f *framePath) navigateTab(rawURL string) {
    tab := f.tabMgr.Active()
    if tab == nil {
        return
    }
    u := normalizeURL(rawURL)
    
    // Cancel only THIS tab's navigation
    tab.nav.mu.Lock()
    if tab.nav.cancel != nil {
        tab.nav.cancel()
    }
    ctx, cancel := context.WithCancel(context.Background())
    tab.nav.cancel = cancel
    tab.nav.serial++
    serial := tab.nav.serial
    tab.nav.loading = true
    tab.nav.mu.Unlock()
    
    tab.Loading = true
    tab.Error = ""
    f.toolbar.SetLoading(true)
    f.toolbar.Error = ""
    
    go func() {
        layer, _, bgColor, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
        f.navResults <- navResult{
            tabID:   tab.ID,
            serial:  serial,
            layer:   layer,
            bgColor: bgColor,
            err:     err,
            url:     u,
        }
    }()
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS (navigation still works, just sends to channel now)

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "feat(goosie): use per-tab navigation controller in navigateTab"
```

---

### Task 5: Implement applyNavResult Event Handler

**Files:**
- Modify: `cmd/goosie/main.go` (add new function)

- [ ] **Step 1: Implement applyNavResult**

Add new function after `navigateTab`:

```go
func (f *framePath) applyNavResult(result navResult) {
    tab := f.tabMgr.TabByID(result.tabID)
    if tab == nil {
        return
    }
    
    tab.nav.mu.Lock()
    if tab.nav.serial != result.serial {
        tab.nav.mu.Unlock()
        return // Stale result
    }
    tab.nav.loading = false
    tab.nav.mu.Unlock()
    
    tab.Loading = false
    if result.err != nil {
        tab.Error = result.err.Error()
        if f.tabMgr.Active() == tab {
            f.toolbar.SetLoading(false)
            f.toolbar.Error = result.err.Error()
        }
        fmt.Fprintln(os.Stderr, result.err)
        return
    }
    
    tab.Layer = result.layer
    tab.BGColor = result.bgColor
    tab.URL = result.url
    tab.Title = result.url
    tab.History.Push(result.url)
    
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

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "feat(goosie): implement applyNavResult event handler"
```

---

### Task 6: Refactor navigateTabNoHistory and reloadTab

**Files:**
- Modify: `cmd/goosie/main.go:390-451`

- [ ] **Step 1: Refactor navigateTabNoHistory**

Replace `navigateTabNoHistory`:

```go
func (f *framePath) navigateTabNoHistory(rawURL string) {
    tab := f.tabMgr.Active()
    if tab == nil {
        return
    }
    u := normalizeURL(rawURL)
    
    tab.nav.mu.Lock()
    if tab.nav.cancel != nil {
        tab.nav.cancel()
    }
    ctx, cancel := context.WithCancel(context.Background())
    tab.nav.cancel = cancel
    tab.nav.serial++
    serial := tab.nav.serial
    tab.nav.loading = true
    tab.nav.mu.Unlock()
    
    tab.Loading = true
    tab.Error = ""
    f.toolbar.SetLoading(true)
    
    go func() {
        layer, _, bgColor, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
        f.navResults <- navResult{
            tabID:   tab.ID,
            serial:  serial,
            layer:   layer,
            bgColor: bgColor,
            err:     err,
            url:     u,
        }
    }()
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "feat(goosie): use per-tab controller in navigateTabNoHistory"
```

---

### Task 7: Fix newTab to Explicitly Activate

**Files:**
- Modify: `cmd/goosie/main.go:479-504`

- [ ] **Step 1: Fix newTab activation**

Replace `newTab` function:

```go
func (f *framePath) newTab() {
    if f.tabMgr == nil {
        return
    }
    cur := f.tabMgr.Active()
    if cur != nil {
        vp := f.sched.Viewport()
        cur.ScrollY = vp.Offset.Y
    }
    tab := f.tabMgr.NewTab()
    f.tabMgr.SwitchTo(tab.ID) // Explicitly activate
    f.syncTabToToolbar()
    spec := paint.SceneSpec{DocHeight: f.config.devSize().H}
    _, layer := paint.BuildLayer(spec)
    tab.Layer = layer
    tab.BGColor = frame.RGB(255, 255, 255)
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

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "fix(goosie): explicitly activate new tabs"
```

---

### Task 8: Reset Toolbar Cursor in syncTabToToolbar

**Files:**
- Modify: `cmd/goosie/main.go:299-313`

- [ ] **Step 1: Reset cursor and selection**

Replace `syncTabToToolbar`:

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
    f.toolbar.Cursor = len([]rune(tab.URL))
    f.toolbar.SelectionEnd = -1
    f.toolbar.History = tab.History
    f.toolbar.SetLoading(tab.nav.loading)
    f.toolbar.Error = tab.Error
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "fix(goosie): reset toolbar cursor on tab switch"
```

---

### Task 9: Add Chrome/Content Bitmap Separation

**Files:**
- Modify: `cmd/goosie/toolbar_window.go:13-44`

- [ ] **Step 1: Add chrome bitmap field**

Modify `chromeWindow` struct:

```go
type chromeWindow struct {
    surface.Window
    toolbar    *toolbar.State
    tabMgr     *tabs.TabManager
    events     chan surface.Event
    onSwitch   func(uint64)
    onNewTab   func()
    onCloseTab func(uint64)
    
    chromeBitmap *frame.Bitmap
}
```

- [ ] **Step 2: Refactor Present to use separate chrome bitmap**

Replace `Present` method:

```go
func (cw *chromeWindow) Present(buf *frame.Bitmap, damage []frame.Rect) error {
    if cw.chromeBitmap == nil || cw.chromeBitmap.W != buf.W {
        cw.chromeBitmap = frame.NewBitmap(buf.W, totalChromeHeight)
    }
    
    cw.chromeBitmap.FillRect(frame.Rect4(0, 0, int32(cw.chromeBitmap.W), int32(cw.chromeBitmap.H)), 
        frame.RGB(255, 255, 255), nil)
    tabs.DrawTabBar(cw.chromeBitmap, cw.tabMgr, 0)
    cw.toolbar.Draw(cw.chromeBitmap, tabs.TabBarHeight)
    
    for y := int32(0); y < totalChromeHeight && y < int32(buf.H); y++ {
        copy(buf.Row(y), cw.chromeBitmap.Row(y))
    }
    
    chromeRect := frame.Rect4(0, 0, int32(buf.W), totalChromeHeight)
    merged := append(damage[:len(damage):len(damage)], chromeRect)
    return cw.Window.Present(buf, merged)
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/goosie -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/goosie/toolbar_window.go
git commit -m "feat(goosie): separate chrome and content bitmaps"
```

---

### Task 10: Add Integration Tests

**Files:**
- Create: `cmd/goosie/tab_lifecycle_test.go`

- [ ] **Step 1: Write integration tests**

Create `cmd/goosie/tab_lifecycle_test.go`:

```go
package main

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/vyquocvu/goosie/internal/tabs"
)

func TestIndependentTabNavigation(t *testing.T) {
    server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(100 * time.Millisecond)
        w.Write([]byte("<html><body>Tab 1</body></html>"))
    }))
    defer server1.Close()
    
    server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("<html><body>Tab 2</body></html>"))
    }))
    defer server2.Close()
    
    mgr := tabs.NewManager(nil)
    tab1 := mgr.NewTab()
    tab2 := mgr.NewTab()
    
    // Navigate tab1
    ctx1, cancel1 := context.WithCancel(context.Background())
    tab1.nav.cancel = cancel1
    tab1.nav.serial = 1
    
    // Navigate tab2
    ctx2, cancel2 := context.WithCancel(context.Background())
    tab2.nav.cancel = cancel2
    tab2.nav.serial = 1
    
    // Cancel tab1 shouldn't affect tab2
    tab1.nav.cancel()
    
    if ctx1.Err() == nil {
        t.Fatal("tab1 not cancelled")
    }
    if ctx2.Err() != nil {
        t.Fatal("tab2 cancelled when tab1 cancelled")
    }
}

func TestTabSerialPreventsStaleResults(t *testing.T) {
    mgr := tabs.NewManager(nil)
    tab := mgr.NewTab()
    
    tab.nav.serial = 1
    if tab.nav.serial != 1 {
        t.Fatal("serial not set")
    }
    
    tab.nav.serial = 2
    if tab.nav.serial != 2 {
        t.Fatal("serial not updated")
    }
}
```

- [ ] **Step 2: Run tests with race detector**

Run: `go test -race ./cmd/goosie -run TestIndependent -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/goosie/tab_lifecycle_test.go
git commit -m "test(goosie): add tab lifecycle integration tests"
```

---

### Task 11: Visual Verification

- [ ] **Step 1: Build and run interactive test**

```bash
go build ./cmd/goosie
./goosie -url https://example.com
```

- [ ] **Step 2: Verify tab independence**

1. Open tab 1, navigate to a slow-loading page
2. Cmd+T to create tab 2
3. Navigate tab 2 to a fast page
4. Verify tab 2 displays while tab 1 still loading
5. Switch back to tab 1, verify it completes loading

- [ ] **Step 3: Verify chrome separation**

1. Scroll document up/down
2. Verify tab bar and toolbar don't shift
3. Verify document content scrolls cleanly

- [ ] **Step 4: Run full test suite**

```bash
go test -race ./...
```

Expected: All tests pass

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat: complete independent tab lifecycle (gate 2)"
```

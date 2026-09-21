# Tabbed Browser GUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add tabbed browsing to Goosie with full tab independence — each tab has its own URL, scroll position, history, and loading state.

**Architecture:** A `TabManager` in `internal/tabs/` owns all tab state. The existing `framePath` in `main.go` delegates navigation and display to the active tab. A new tab bar is drawn above the existing toolbar by expanding `toolbarWindow` to handle both chrome layers. Tab bar layout, drawing, and input routing live in `internal/tabs/`.

**Tech Stack:** Go, existing `frame`/`paint`/`raster`/`surface`/`toolbar` packages. No new dependencies.

---

## File Structure

**New files:**
- `internal/tabs/tab.go` — Tab struct with per-tab state (URL, history, scroll, layer, loading)
- `internal/tabs/manager.go` — TabManager: create, close, switch, query tabs
- `internal/tabs/layout.go` — Tab bar geometry: tab rects, close button rects, new-tab button, overflow
- `internal/tabs/draw.go` — Tab bar rendering onto a `frame.Bitmap`
- `test/tabs/manager_test.go` — TabManager unit tests
- `test/tabs/layout_test.go` — Layout calculation tests
- `test/tabs/draw_test.go` — Draw smoke tests (no panics, correct regions)

**Modified files:**
- `cmd/goosie/main.go` — Replace single `toolbar.State` with `tabs.TabManager`; navigate/traverse/reload operate on active tab
- `cmd/goosie/toolbar_window.go` — Expand to handle tab bar input (total chrome = 76px); rename to `chromeWindow`
- `internal/toolbar/toolbar.go` — Accept external `History` pointer so tab switching swaps the history

---

### Task 1: Tab data model

**Files:**
- Create: `internal/tabs/tab.go`
- Test: `test/tabs/manager_test.go`

- [ ] **Step 1: Write the failing test**

Create `test/tabs/manager_test.go`:

```go
package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestNewTabHasCorrectInitialState(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	if tab.ID == 0 {
		t.Fatal("new tab should have non-zero ID")
	}
	if tab.URL != "" {
		t.Fatalf("new tab URL should be empty, got %q", tab.URL)
	}
	if tab.Title != "New Tab" {
		t.Fatalf("new tab title should be 'New Tab', got %q", tab.Title)
	}
	if tab.Loading {
		t.Fatal("new tab should not be loading")
	}
	if tab.ScrollY != 0 {
		t.Fatalf("new tab scroll should be 0, got %d", tab.ScrollY)
	}
	if tab.History == nil {
		t.Fatal("new tab should have a history")
	}
}

func TestTabIDsAreUniqueAndMonotonic(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()
	t3 := mgr.NewTab()

	if t1.ID >= t2.ID || t2.ID >= t3.ID {
		t.Fatalf("IDs should be monotonically increasing: %d, %d, %d", t1.ID, t2.ID, t3.ID)
	}
}

func TestActiveReturnsCorrectTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	if mgr.Active() != t1 {
		t.Fatal("first tab should be active after creation")
	}
}

func TestNewTabStartsWithOwnHistory(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	if t1.History == t2.History {
		t.Fatal("each tab should have its own history instance")
	}
	t1.History.Push("https://example.com")
	if t2.History.Current() != "" {
		t.Fatal("pushing to t1 history should not affect t2")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/tabs/ -run TestNewTab -v`
Expected: FAIL — `internal/tabs` package does not exist

- [ ] **Step 3: Write minimal implementation**

Create `internal/tabs/tab.go`:

```go
package tabs

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

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
}

func newTab(id uint64) *Tab {
	return &Tab{
		ID:      id,
		Title:   "New Tab",
		History: toolbar.NewHistory(),
	}
}
```

- [ ] **Step 4: Create TabManager stub so tests compile**

Create `internal/tabs/manager.go`:

```go
package tabs

import "sync"

// TabManager owns all tabs and tracks which is active.
type TabManager struct {
	mu          sync.Mutex
	tabs        []*Tab
	activeIdx   int
	nextID      uint64
	onCloseLast func()
	OnChange    func()
}

func NewManager(onCloseLast func()) *TabManager {
	return &TabManager{
		onCloseLast: onCloseLast,
	}
}

func (m *TabManager) NewTab() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	tab := newTab(m.nextID)
	m.tabs = append(m.tabs, tab)
	if len(m.tabs) == 1 {
		m.activeIdx = 0
	}
	m.fireChange()
	return tab
}

func (m *TabManager) Active() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tabs) == 0 {
		return nil
	}
	return m.tabs[m.activeIdx]
}

func (m *TabManager) fireChange() {
	if m.OnChange != nil {
		m.OnChange()
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./test/tabs/ -v`
Expected: PASS — all 4 tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/tabs/tab.go internal/tabs/manager.go test/tabs/manager_test.go
git commit -m "feat(tabs): add Tab data model and TabManager.NewTab"
```

---

### Task 2: TabManager close and switch

**Files:**
- Modify: `internal/tabs/manager.go`
- Modify: `test/tabs/manager_test.go`

- [ ] **Step 1: Write failing tests for CloseTab and SwitchTo**

Append to `test/tabs/manager_test.go`:

```go
func TestCloseTabRemovesTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	mgr.CloseTab(t1.ID)

	if mgr.Count() != 1 {
		t.Fatalf("expected 1 tab, got %d", mgr.Count())
	}
	if mgr.Active().ID == t1.ID {
		t.Fatal("closed tab should not be active")
	}
}

func TestCloseLastTabCallsOnCloseLast(t *testing.T) {
	called := false
	mgr := tabs.NewManager(func() { called = true })
	t1 := mgr.NewTab()

	mgr.CloseTab(t1.ID)

	if !called {
		t.Fatal("onCloseLast should be called when last tab closes")
	}
	if mgr.Count() != 0 {
		t.Fatal("tab count should be 0")
	}
}

func TestSwitchToChangesActiveTab(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	if mgr.Active() != t1 {
		t.Fatal("t1 should be active initially")
	}

	mgr.SwitchTo(t2.ID)
	if mgr.Active() != t2 {
		t.Fatal("t2 should be active after SwitchTo")
	}

	mgr.SwitchTo(t1.ID)
	if mgr.Active() != t1 {
		t.Fatal("t1 should be active after switching back")
	}
}

func TestSwitchToFiresOnChange(t *testing.T) {
	changes := 0
	mgr := tabs.NewManager(nil)
	mgr.OnChange = func() { changes++ }
	t1 := mgr.NewTab()
	_ = mgr.NewTab()

	before := changes
	mgr.SwitchTo(t1.ID) // switching to already-active should not fire
	if changes != before {
		t.Fatal("switching to already-active tab should not fire OnChange")
	}

	t2ID := mgr.tabs[1].ID
	mgr.SwitchTo(t2ID)
	if changes != before+1 {
		t.Fatal("switching to different tab should fire OnChange once")
	}
}

func TestCount(t *testing.T) {
	mgr := tabs.NewManager(nil)
	if mgr.Count() != 0 {
		t.Fatal("empty manager should have count 0")
	}
	mgr.NewTab()
	mgr.NewTab()
	if mgr.Count() != 2 {
		t.Fatalf("expected 2, got %d", mgr.Count())
	}
}

func TestTabsReturnsAllTabs(t *testing.T) {
	mgr := tabs.NewManager(nil)
	t1 := mgr.NewTab()
	t2 := mgr.NewTab()

	all := mgr.Tabs()
	if len(all) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(all))
	}
	if all[0] != t1 || all[1] != t2 {
		t.Fatal("Tabs should return tabs in order")
	}
}
```

Note: The `TestSwitchToFiresOnChange` test accesses `mgr.tabs` directly. Since `tabs` is unexported, we need to add a `TabID(idx int) uint64` helper or use `Tabs()[1].ID`. Let me fix that:

Replace `t2ID := mgr.tabs[1].ID` with:
```go
	t2ID := mgr.Tabs()[1].ID
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./test/tabs/ -run "TestCloseTab|TestCloseLast|TestSwitchTo|TestCount|TestTabs" -v`
Expected: FAIL — methods don't exist yet

- [ ] **Step 3: Implement CloseTab, SwitchTo, Count, Tabs**

Replace `internal/tabs/manager.go` with:

```go
package tabs

import "sync"

type TabManager struct {
	mu          sync.Mutex
	tabs        []*Tab
	activeIdx   int
	nextID      uint64
	onCloseLast func()
	OnChange    func()
}

func NewManager(onCloseLast func()) *TabManager {
	return &TabManager{
		onCloseLast: onCloseLast,
	}
}

func (m *TabManager) NewTab() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	tab := newTab(m.nextID)
	m.tabs = append(m.tabs, tab)
	if len(m.tabs) == 1 {
		m.activeIdx = 0
	}
	m.fireChange()
	return tab
}

func (m *TabManager) Active() *Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tabs) == 0 {
		return nil
	}
	return m.tabs[m.activeIdx]
}

func (m *TabManager) CloseTab(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := -1
	for i, t := range m.tabs {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	if len(m.tabs) == 0 {
		m.activeIdx = 0
		m.fireChange()
		if m.onCloseLast != nil {
			go m.onCloseLast()
		}
		return
	}
	if m.activeIdx >= len(m.tabs) {
		m.activeIdx = len(m.tabs) - 1
	} else if m.activeIdx > idx {
		m.activeIdx--
	} else if m.activeIdx == idx {
		// Active tab was closed; new active is at same index (next tab slid in).
		// If we were at the end, activeIdx was already adjusted above.
	}
	m.fireChange()
}

func (m *TabManager) SwitchTo(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.tabs {
		if t.ID == id {
			if i == m.activeIdx {
				return
			}
			m.activeIdx = i
			m.fireChange()
			return
		}
	}
}

func (m *TabManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tabs)
}

func (m *TabManager) Tabs() []*Tab {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Tab, len(m.tabs))
	copy(out, m.tabs)
	return out
}

func (m *TabManager) ActiveIndex() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeIdx
}

func (m *TabManager) fireChange() {
	if m.OnChange != nil {
		m.OnChange()
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/tabs/ -v`
Expected: PASS — all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/tabs/manager.go test/tabs/manager_test.go
git commit -m "feat(tabs): add CloseTab, SwitchTo, Count, Tabs to TabManager"
```

---

### Task 3: Tab bar layout

**Files:**
- Create: `internal/tabs/layout.go`
- Test: `test/tabs/layout_test.go`

- [ ] **Step 1: Write failing layout tests**

Create `test/tabs/layout_test.go`:

```go
package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestTabRectSingleTab(t *testing.T) {
	r := tabs.TabRect(0, 0, 1, 1000)
	if r.X0 != tabs.LeftMargin {
		t.Fatalf("first tab X0 should be LeftMargin (%d), got %d", tabs.LeftMargin, r.X0)
	}
	if r.W() != tabs.TabWidth {
		t.Fatalf("tab width should be %d, got %d", tabs.TabWidth, r.W())
	}
	if r.Y0 != 0 || r.Y1 != tabs.TabBarHeight {
		t.Fatalf("tab should span full tab bar height, got Y0=%d Y1=%d", r.Y0, r.Y1)
	}
}

func TestTabRectMultipleTabs(t *testing.T) {
	r0 := tabs.TabRect(0, 0, 3, 1000)
	r1 := tabs.TabRect(1, 0, 3, 1000)
	r2 := tabs.TabRect(2, 0, 3, 1000)

	expectedGap := int32(tabs.LeftMargin)
	if r0.X0 != expectedGap {
		t.Fatalf("tab 0 X0 should be %d, got %d", expectedGap, r0.X0)
	}
	if r1.X0 != r0.X1+tabs.TabGap {
		t.Fatalf("tab 1 should start after tab 0 + gap, got %d", r1.X0)
	}
	if r2.X0 != r1.X1+tabs.TabGap {
		t.Fatalf("tab 2 should start after tab 1 + gap, got %d", r2.X0)
	}
}

func TestNewTabButtonRect(t *testing.T) {
	// With 2 tabs at 200px each + left margin + gaps
	tabCount := 2
	contentWidth := int32(1000)
	r := tabs.NewTabButtonRect(tabCount, 0, contentWidth)
	if r.W() != tabs.NewTabBtnSize {
		t.Fatalf("new tab button width should be %d, got %d", tabs.NewTabBtnSize, r.W())
	}
	// Should be positioned right after the last tab
	lastTab := tabs.TabRect(tabCount-1, 0, tabCount, contentWidth)
	if r.X0 != lastTab.X1+tabs.TabGap {
		t.Fatalf("new tab button should be after last tab + gap, got X0=%d expected %d", r.X0, lastTab.X1+tabs.TabGap)
	}
}

func TestCloseButtonRect(t *testing.T) {
	tabR := tabs.TabRect(0, 0, 1, 1000)
	closeR := tabs.CloseButtonRect(tabR)
	if closeR.W() != tabs.CloseBtnSize || closeR.H() != tabs.CloseBtnSize {
		t.Fatalf("close button should be %dx%d, got %dx%d", tabs.CloseBtnSize, tabs.CloseBtnSize, closeR.W(), closeR.H())
	}
	// Close button should be on the right side of the tab
	if closeR.X1 > tabR.X1-tabs.CloseBtnMargin {
		t.Fatal("close button should be inside the tab with margin")
	}
}

func TestTotalChromeHeight(t *testing.T) {
	expected := tabs.TabBarHeight + 40 // TabBarHeight + toolbar height
	if expected != 76 {
		t.Fatalf("total chrome should be 76px, got %d", expected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./test/tabs/ -run "TestTabRect|TestNewTab|TestClose|TestTotalChrome" -v`
Expected: FAIL — `internal/tabs` has no layout functions

- [ ] **Step 3: Implement layout functions**

Create `internal/tabs/layout.go`:

```go
package tabs

import "github.com/vyquocvu/goosie/internal/frame"

const (
	TabBarHeight   = 36
	TabWidth       = 200
	TabGap         = 1
	LeftMargin     = 8
	NewTabBtnSize  = 32
	CloseBtnSize   = 16
	CloseBtnMargin = 8
)

// TabRect returns the bounding rect for the tab at index idx given scrollOffset
// and total tabCount. contentWidth is the window width (unused for basic layout).
func TabRect(idx int, scrollOffset, tabCount, contentWidth int32) frame.Rect {
	x := int32(LeftMargin) + int32(idx)*(TabWidth+TabGap) - scrollOffset
	return frame.Rect4(x, 0, x+TabWidth, TabBarHeight)
}

// NewTabButtonRect returns the "+" button rect positioned after the last tab.
func NewTabButtonRect(tabCount int, scrollOffset, contentWidth int32) frame.Rect {
	lastX := int32(LeftMargin) + int32(tabCount)*(TabWidth+TabGap) - TabGap - scrollOffset
	x := lastX + TabGap
	y := (TabBarHeight - NewTabBtnSize) / 2
	return frame.Rect4(x, y, x+NewTabBtnSize, y+NewTabBtnSize)
}

// CloseButtonRect returns the close "×" button rect within a tab.
func CloseButtonRect(tabRect frame.Rect) frame.Rect {
	x := tabRect.X1 - CloseBtnSize - CloseBtnMargin
	y := (TabBarHeight - CloseBtnSize) / 2
	return frame.Rect4(x, y, x+CloseBtnSize, y+CloseBtnSize)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/tabs/ -run "TestTabRect|TestNewTab|TestClose|TestTotalChrome" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tabs/layout.go test/tabs/layout_test.go
git commit -m "feat(tabs): add tab bar layout calculations"
```

---

### Task 4: Tab bar drawing

**Files:**
- Create: `internal/tabs/draw.go`
- Test: `test/tabs/draw_test.go`

- [ ] **Step 1: Write failing draw test**

Create `test/tabs/draw_test.go`:

```go
package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestDrawDoesNotPanicOnEmptyManager(t *testing.T) {
	mgr := tabs.NewManager(nil)
	buf := frame.NewBitmap(frame.Size{W: 800, H: 76})
	// Should not panic with zero tabs
	tabs.DrawTabBar(buf, mgr, 0)
}

func TestDrawDoesNotPanicWithTabs(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	mgr.NewTab()
	buf := frame.NewBitmap(frame.Size{W: 800, H: 76})
	tabs.DrawTabBar(buf, mgr, 0)
}

func TestDrawDoesNotPanicOnNilBitmap(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()
	// Should not panic with nil bitmap
	tabs.DrawTabBar(nil, mgr, 0)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./test/tabs/ -run TestDraw -v`
Expected: FAIL — `DrawTabBar` does not exist

- [ ] **Step 3: Implement DrawTabBar**

Create `internal/tabs/draw.go`:

```go
package tabs

import (
	"github.com/vyquocvu/goosie/internal/frame"
)

var (
	tabBarBg       = frame.RGB(220, 220, 220)
	activeTabBg    = frame.RGB(255, 255, 255)
	inactiveTabBg  = frame.RGB(232, 232, 232)
	tabBorderColor = frame.RGB(208, 208, 208)
	tabTextColor   = frame.RGB(60, 60, 60)
	newTabBtnColor = frame.RGB(160, 160, 160)
)

// DrawTabBar renders the tab bar onto the top of buf. scrollOffset shifts tabs
// horizontally for overflow. The caller provides a bitmap that is at least
// TabBarHeight pixels tall.
func DrawTabBar(buf *frame.Bitmap, mgr *TabManager, scrollOffset int32) {
	if buf == nil || buf.Empty() {
		return
	}
	w := int32(buf.W)
	barRect := frame.Rect4(0, 0, w, TabBarHeight)
	buf.FillRect(barRect, tabBarBg, nil)

	tabsList := mgr.Tabs()
	activeIdx := mgr.ActiveIndex()

	for i, tab := range tabsList {
		r := TabRect(i, scrollOffset, len(tabsList), w)
		// Skip tabs entirely off-screen
		if r.X1 <= 0 || r.X0 >= w {
			continue
		}
		bg := inactiveTabBg
		if i == activeIdx {
			bg = activeTabBg
		}
		buf.FillRect(r, bg, nil)
		// Border on top and sides
		buf.FillRect(frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+1), tabBorderColor, nil)
		buf.FillRect(frame.Rect4(r.X0, r.Y0, r.X0+1, r.Y1), tabBorderColor, nil)
		buf.FillRect(frame.Rect4(r.X1-1, r.Y0, r.X1, r.Y1), tabBorderColor, nil)

		// Draw title text (simplified: just fill a small rect as placeholder)
		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		drawTabText(buf, title, r)

		// Draw close button
		closeR := CloseButtonRect(r)
		drawCloseButton(buf, closeR, tabTextColor)
	}

	// Draw "+" new tab button
	if len(tabsList) > 0 {
		btnR := NewTabButtonRect(len(tabsList), scrollOffset, w)
		if btnR.X0 < w {
			drawNewTabButton(buf, btnR, newTabBtnColor)
		}
	}
}

func drawTabText(buf *frame.Bitmap, text string, tabRect frame.Rect) {
	// Simplified: draw a small filled rect to represent text.
	// Real text rendering will use fonts when integrated.
	textX := tabRect.X0 + 10
	textY := tabRect.Y0 + (TabBarHeight-8)/2
	maxW := tabRect.W() - CloseBtnSize - CloseBtnMargin - 20
	if maxW < 0 {
		maxW = 0
	}
	textW := int32(len(text)) * 6
	if textW > maxW {
		textW = maxW
	}
	if textW > 0 {
		buf.FillRect(frame.Rect4(textX, textY, textX+textW, textY+8), tabTextColor, nil)
	}
}

func drawCloseButton(buf *frame.Bitmap, r frame.Rect, c frame.Color) {
	// Draw an "×" shape using two diagonal lines of pixels
	size := r.W()
	for i := int32(0); i < size; i++ {
		// Top-left to bottom-right
		buf.FillRect(frame.Rect4(r.X0+i, r.Y0+i, r.X0+i+1, r.Y0+i+1), c, nil)
		// Top-right to bottom-left
		buf.FillRect(frame.Rect4(r.X1-1-i, r.Y0+i, r.X1-i, r.Y0+i+1), c, nil)
	}
}

func drawNewTabButton(buf *frame.Bitmap, r frame.Rect, c frame.Color) {
	// Draw a "+" centered in the rect
	cx := (r.X0 + r.X1) / 2
	cy := (r.Y0 + r.Y1) / 2
	half := int32(6)
	// Horizontal bar
	buf.FillRect(frame.Rect4(cx-half, cy, cx+half+1, cy+1), c, nil)
	// Vertical bar
	buf.FillRect(frame.Rect4(cx, cy-half, cx+1, cy+half+1), c, nil)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/tabs/ -run TestDraw -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tabs/draw.go test/tabs/draw_test.go
git commit -m "feat(tabs): add tab bar drawing"
```

---

### Task 5: Toolbar accepts external History

**Files:**
- Modify: `internal/toolbar/toolbar.go`
- Test: existing toolbar tests (verify no regressions)

- [ ] **Step 1: Write failing test for external history**

Create `test/tabs/toolbar_history_test.go`:

```go
package tabs_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestToolbarAcceptsExternalHistory(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	external := toolbar.NewHistory()
	external.Push("https://example.com")

	tb.SetHistory(external)

	if tb.History.Current() != "https://example.com" {
		t.Fatalf("toolbar should show external history URL, got %q", tb.History.Current())
	}
}

func TestToolbarNavigatePushesToCurrentHistory(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	h := toolbar.NewHistory()
	tb.SetHistory(h)

	tb.Navigate("https://test.com")

	if h.Current() != "https://test.com" {
		t.Fatalf("navigate should push to current history, got %q", h.Current())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/tabs/ -run "TestToolbarAccepts|TestToolbarNavigate" -v`
Expected: FAIL — `SetHistory` does not exist

- [ ] **Step 3: Add SetHistory to toolbar.State**

In `internal/toolbar/toolbar.go`, add this method after `NewState`:

```go
func (s *State) SetHistory(h *History) {
	s.History = h
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./test/tabs/ -run "TestToolbar" -v`
Expected: PASS

Also run existing toolbar tests to verify no regressions:
Run: `go test ./internal/toolbar/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/toolbar/toolbar.go test/tabs/toolbar_history_test.go
git commit -m "feat(toolbar): add SetHistory for per-tab history support"
```

---

### Task 6: Integrate TabManager into framePath

**Files:**
- Modify: `cmd/goosie/main.go`

- [ ] **Step 1: Add TabManager to framePath struct**

In `cmd/goosie/main.go`, add `tabs` to the imports:

```go
import (
	// ... existing imports ...
	"github.com/vyquocvu/goosie/internal/tabs"
)
```

Add `tabMgr` field to the `framePath` struct:

```go
type framePath struct {
	window   surface.Window
	loop     *surface.Loop
	sched    *raster.Scheduler
	pool     *raster.Pool
	composer *surface.Composer
	layer    *frame.Layer
	rec      *frame.FrameRecorder
	spec     paint.SceneSpec
	config   config
	toolbar  *toolbar.State
	tabMgr   *tabs.TabManager
	client   net.HTTP
	fonts    *raster.Fonts
	started  time.Time

	navMu     sync.Mutex
	navCancel context.CancelFunc
	navSerial atomic.Uint64
}
```

- [ ] **Step 2: Initialize TabManager in build()**

In the `build()` function, after the toolbar is created (around line 266), add TabManager initialization:

```go
	if !c.paced() {
		f.toolbar = toolbar.NewState(dev.W, fonts)
		f.tabMgr = tabs.NewManager(func() {
			// onCloseLast: close the window
			if f.window != nil {
				_ = f.window.Close()
			}
		})
		f.tabMgr.OnChange = func() {
			f.syncTabToToolbar()
		}
		// Create the first tab
		firstTab := f.tabMgr.NewTab()
		if c.url != "" {
			firstTab.URL = normalizeURL(c.url)
			firstTab.Title = firstTab.URL
		}
		f.syncTabToToolbar()

		f.toolbar.OnNavigate = func(rawURL string) {
			f.navigateTab(rawURL)
		}
		f.toolbar.OnTraverse = func(delta int) {
			f.traverseTab(delta)
		}
		f.toolbar.OnReload = func() {
			f.reloadTab()
		}
		if cb := platform.NewClipboard(); cb != nil {
			f.toolbar.Clipboard = cb
		}
	}
```

- [ ] **Step 3: Add tab-aware navigation methods**

Add these methods to `framePath` in `main.go`:

```go
// syncTabToToolbar copies the active tab's state into the toolbar for display.
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
	f.toolbar.History = tab.History
	f.toolbar.SetLoading(tab.Loading)
	f.toolbar.Error = tab.Error
}

// navigateTab loads a URL on the active tab.
func (f *framePath) navigateTab(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	u := normalizeURL(rawURL)

	f.navMu.Lock()
	if f.navCancel != nil {
		f.navCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.navCancel = cancel
	f.navMu.Unlock()

	serial := f.navSerial.Add(1)
	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)
	f.toolbar.Error = ""

	go func() {
		layer, _, bgColor, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if f.navSerial.Load() != serial {
				return
			}
			tab.Loading = false
			tab.Error = err.Error()
			f.toolbar.SetLoading(false)
			f.toolbar.Error = err.Error()
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if f.navSerial.Load() != serial {
			return
		}
		tab.Layer = layer
		tab.Loading = false
		tab.URL = u
		tab.Title = u
		tab.BGColor = bgColor
		tab.History.Push(u)
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     serial,
			Layers:     []*frame.Layer{layer},
			Background: bgColor,
		})
	}()
}

// traverseTab handles back/forward on the active tab.
func (f *framePath) traverseTab(delta int) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	var url string
	var ok bool
	if delta < 0 {
		url, ok = tab.History.Back()
	} else if delta > 0 {
		url, ok = tab.History.Forward()
	}
	if !ok || url == "" {
		return
	}
	f.navigateTabNoHistory(url)
}

// navigateTabNoHistory loads a URL without pushing to history.
func (f *framePath) navigateTabNoHistory(rawURL string) {
	tab := f.tabMgr.Active()
	if tab == nil {
		return
	}
	u := normalizeURL(rawURL)

	f.navMu.Lock()
	if f.navCancel != nil {
		f.navCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.navCancel = cancel
	f.navMu.Unlock()

	serial := f.navSerial.Add(1)
	tab.Loading = true
	tab.Error = ""
	f.toolbar.SetLoading(true)

	go func() {
		layer, _, bgColor, err := loadURLCtx(ctx, f.client, f.fonts, u, f.config.width, f.config.height, float32(f.config.dpr))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if f.navSerial.Load() != serial {
				return
			}
			tab.Loading = false
			tab.Error = err.Error()
			f.toolbar.SetLoading(false)
			f.toolbar.Error = err.Error()
			fmt.Fprintln(os.Stderr, err)
			return
		}
		if f.navSerial.Load() != serial {
			return
		}
		tab.Layer = layer
		tab.Loading = false
		tab.URL = u
		tab.Title = u
		tab.BGColor = bgColor
		f.syncTabToToolbar()
		f.sched.SetPlan(frame.FramePlan{
			Serial:     serial,
			Layers:     []*frame.Layer{layer},
			Background: bgColor,
		})
	}()
}

// reloadTab re-fetches the active tab's URL.
func (f *framePath) reloadTab() {
	tab := f.tabMgr.Active()
	if tab == nil || tab.URL == "" {
		return
	}
	f.navigateTabNoHistory(tab.URL)
}

// switchTab saves the current tab's scroll and switches to the given tab.
func (f *framePath) switchTab(id uint64) {
	// Save current scroll
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	f.tabMgr.SwitchTo(id)
	// Restore new tab's state
	newTab := f.tabMgr.Active()
	if newTab == nil {
		return
	}
	f.syncTabToToolbar()
	if newTab.Layer != nil {
		f.sched.SetPlan(frame.FramePlan{
			Serial:     f.navSerial.Add(1),
			Layers:     []*frame.Layer{newTab.Layer},
			Background: newTab.BGColor,
		})
	}
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: newTab.ScrollY},
		Size:   f.config.devSize(),
	})
}
```

- [ ] **Step 4: Verify build compiles**

Run: `go build ./cmd/goosie/`
Expected: Compiles successfully (may have unused warnings for old navigate/traverse/reload — those will be removed in Task 8)

- [ ] **Step 5: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "feat(main): integrate TabManager into framePath with tab-aware navigation"
```

---

### Task 7: Expand toolbarWindow to handle tab bar

**Files:**
- Modify: `cmd/goosie/toolbar_window.go`

- [ ] **Step 1: Rename toolbarWindow to chromeWindow and add tab bar support**

Replace `cmd/goosie/toolbar_window.go` with:

```go
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
	toolbar *toolbar.State
	tabMgr  *tabs.TabManager
	events  chan surface.Event
	onSwitch func(uint64)
	onNewTab func()
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
	// Draw tab bar first (top 36px), then toolbar (next 40px)
	tabs.DrawTabBar(buf, cw.tabMgr, 0)
	cw.toolbar.Draw(buf)
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
		if ev.Pos.Y < tabs.TabBarHeight && ev.Button == surface.ButtonLeft {
			cw.handleTabBarClick(ev.Pos)
			return true
		}
		if ev.Pos.Y < tabs.TabBarHeight {
			return true
		}
		// Toolbar area (between tab bar and content)
		toolbarY := ev.Pos.Y - tabs.TabBarHeight
		if toolbarY >= 0 && toolbarY < toolbar.ToolbarHeight && ev.Button == surface.ButtonLeft {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
			return true
		}
		if toolbarY >= 0 && toolbarY < toolbar.ToolbarHeight {
			return true
		}
		// Click in content area — defocus address bar if needed
		if cw.toolbar.Focus == toolbar.FocusAddress {
			adjusted := ev.Pos
			adjusted.Y = toolbarY
			cw.toolbar.HandleClick(adjusted, ev.Button)
		}
		return false
	case surface.EvKey:
		// Check tab shortcuts before address bar
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
		r := tabs.TabRect(i, 0, len(tabList), pos.X+100)
		if pos.X >= r.X0 && pos.X < r.X1 && pos.Y >= r.Y0 && pos.Y < r.Y1 {
			// Check close button first
			closeR := tabs.CloseButtonRect(r)
			if pos.X >= closeR.X0 && pos.X < closeR.X1 && pos.Y >= closeR.Y0 && pos.Y < closeR.Y1 {
				if cw.onCloseTab != nil {
					cw.onCloseTab(tabList[i].ID)
				}
				return
			}
			// Switch to this tab
			if cw.onSwitch != nil {
				cw.onSwitch(tabList[i].ID)
			}
			return
		}
	}
	// Check new tab button
	btnR := tabs.NewTabButtonRect(len(tabList), 0, pos.X+100)
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
		if !shift && cw.onNewTab != nil {
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
```

- [ ] **Step 2: Update openWindow in main.go to use chromeWindow**

In `cmd/goosie/main.go`, update `openWindow()`:

```go
func (f *framePath) openWindow() error {
	w, err := platform.Select(platform.Options{
		Backend:     f.config.backend,
		Size:        f.config.devSize(),
		Scale:       float32(f.config.dpr),
		VsyncPeriod: f.config.vsyncPeriod(),
		Title:       "Goosie",
	})
	if err != nil {
		return fmt.Errorf("goosie: %w", err)
	}
	if f.toolbar != nil && f.tabMgr != nil {
		f.window = newChromeWindow(w, f.toolbar, f.tabMgr,
			func(id uint64) { f.switchTab(id) },
			func() { f.newTab() },
			func(id uint64) { f.closeTab(id) },
		)
	} else {
		f.window = w
	}
	f.loop = surface.NewLoop(f.window, f.sched, f.composer, f.rec)
	return nil
}
```

- [ ] **Step 3: Add newTab and closeTab methods to framePath**

Add to `main.go`:

```go
func (f *framePath) newTab() {
	if f.tabMgr == nil {
		return
	}
	// Save current scroll
	cur := f.tabMgr.Active()
	if cur != nil {
		vp := f.sched.Viewport()
		cur.ScrollY = vp.Offset.Y
	}
	tab := f.tabMgr.NewTab()
	f.syncTabToToolbar()
	// Show blank layer for new tab
	spec := paint.SceneSpec{DocHeight: f.config.devSize().H}
	_, layer := paint.BuildLayer(spec)
	tab.Layer = layer
	tab.BGColor = frame.RGB(255, 255, 255)
	f.sched.SetPlan(frame.FramePlan{
		Serial:     f.navSerial.Add(1),
		Layers:     []*frame.Layer{layer},
		Background: tab.BGColor,
	})
	f.sched.SetViewport(frame.Viewport{
		Offset: frame.Point{Y: 0},
		Size:   f.config.devSize(),
	})
}

func (f *framePath) closeTab(id uint64) {
	if f.tabMgr == nil {
		return
	}
	f.tabMgr.CloseTab(id)
	// If there's still a tab, switch to it
	if f.tabMgr.Count() > 0 {
		tab := f.tabMgr.Active()
		f.syncTabToToolbar()
		if tab.Layer != nil {
			f.sched.SetPlan(frame.FramePlan{
				Serial:     f.navSerial.Add(1),
				Layers:     []*frame.Layer{tab.Layer},
				Background: tab.BGColor,
			})
		}
		f.sched.SetViewport(frame.Viewport{
			Offset: frame.Point{Y: tab.ScrollY},
			Size:   f.config.devSize(),
		})
	}
}
```

- [ ] **Step 4: Verify build compiles**

Run: `go build ./cmd/goosie/`
Expected: Compiles successfully

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS — no regressions

- [ ] **Step 6: Commit**

```bash
git add cmd/goosie/toolbar_window.go cmd/goosie/main.go
git commit -m "feat(gui): add chromeWindow with tab bar input handling and keyboard shortcuts"
```

---

### Task 8: Clean up old single-tab navigation

**Files:**
- Modify: `cmd/goosie/main.go`

- [ ] **Step 1: Remove old navigate, traverse, reload, navigateNoHistory methods**

The old methods (`navigate`, `traverse`, `navigateNoHistory`, `reload`) are replaced by `navigateTab`, `traverseTab`, `navigateTabNoHistory`, `reloadTab`. Remove the old ones.

Also remove the old toolbar callback setup in `build()` that references the old methods. The new setup (added in Task 6) already uses `navigateTab`, `traverseTab`, `reloadTab`.

- [ ] **Step 2: Verify build compiles**

Run: `go build ./cmd/goosie/`
Expected: Compiles successfully with no undefined references

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/goosie/main.go
git commit -m "refactor(main): remove old single-tab navigation methods"
```

---

### Task 9: Manual verification

- [ ] **Step 1: Build and run the browser**

```bash
go build -o goosie ./cmd/goosie/ && ./goosie
```

- [ ] **Step 2: Verify tab bar appears above toolbar**

The tab bar (36px) should be visible above the toolbar (40px). Total chrome: 76px.

- [ ] **Step 3: Test new tab creation**

- Click the "+" button → new tab appears
- Press Cmd+T → new tab appears
- Each tab shows "New Tab" as title

- [ ] **Step 4: Test tab switching**

- Click on different tabs → active tab highlights white
- Press Cmd+Tab / Cmd+Shift+Tab → cycles through tabs
- Press Cmd+1, Cmd+2, etc. → switches to tab by number

- [ ] **Step 5: Test tab closing**

- Click "×" on a tab → tab closes
- Press Cmd+W → active tab closes
- Close last tab → window closes

- [ ] **Step 6: Test navigation independence**

- Navigate to a URL in tab 1
- Switch to tab 2, navigate to different URL
- Switch back to tab 1 → URL and content from step 1 are preserved

- [ ] **Step 7: Test scroll preservation**

- Scroll down in tab 1
- Switch to tab 2
- Switch back to tab 1 → scroll position is preserved

- [ ] **Step 8: Final commit if any fixes needed**

```bash
git add -A && git commit -m "fix(gui): tab bar manual verification fixes"
```

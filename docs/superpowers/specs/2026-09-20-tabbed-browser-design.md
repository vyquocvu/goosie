# Tabbed Browser GUI Design

**Date:** 2026-09-20  
**Status:** Approved for implementation  
**Branch:** feat/v2

## Overview

Add tabbed browsing to Goosie with full tab independence. Each tab maintains its own URL, scroll position, history stack, and loading state. The tab bar appears above the existing toolbar in a traditional browser layout.

## Design Decisions

### Tab State Model
**Decision:** Full independence  
Each tab has its own:
- URL and document layer
- Scroll position
- History stack (back/forward)
- Loading state and error messages

Switching tabs instantly displays that tab's exact state.

### Tab Bar Placement
**Decision:** Above the toolbar (traditional layout)  
```
┌─────────────────────────────────────────────────────────┐
│ Tab bar (36px)                                          │
├─────────────────────────────────────────────────────────┤
│ Toolbar (40px)                                          │
├─────────────────────────────────────────────────────────┤
│ Content area                                            │
└─────────────────────────────────────────────────────────┘
```

### Tab Overflow
**Decision:** Fixed tab width with scrolling  
- Tab width: 200px fixed
- When tabs exceed window width, left/right arrows appear
- Active tab is always visible (auto-scroll to keep it in view)

### New Tab Behavior
**Decision:** New tab page  
- Synthetic scene (not HTML) for simplicity
- Clean white background with centered "Goosie" text
- Can be upgraded to HTML later

### Last Tab Close
**Decision:** Close the window  
When the user closes the last open tab, the entire browser window closes.

### Keyboard Shortcuts
**Decision:** Essential only  
- Cmd+T: New tab
- Cmd+W: Close active tab
- Cmd+Shift+T: Reopen last closed tab (deferred)
- Cmd+1-9: Switch to tab by number
- Cmd+Tab / Cmd+Shift+Tab: Next/previous tab

Shortcuts are intercepted before address bar focus checks, so they work even when typing in the URL bar.

## Architecture

### Data Model

```go
// Tab is one browser tab with fully independent state.
type Tab struct {
    ID        uint64
    URL       string
    Title     string        // from <title> or URL fallback
    History   *History      // existing toolbar.History, per-tab
    ScrollY   int32         // vertical scroll position in device pixels
    Layer     *frame.Layer  // the rendered document, nil if not yet loaded
    Loading   bool
    Error     string
    BGColor   frame.Color   // document background color
}

// TabManager owns all tabs and tracks which is active.
type TabManager struct {
    tabs      []*Tab
    activeIdx int
    nextID    uint64
    onCloseLast func()  // called when last tab closes → closes window
    
    // Callbacks for UI updates
    OnChange  func()    // tab switched, added, removed, or reordered
}
```

### Integration with Existing Code

**Current architecture:**
```
main.go:framePath
  ├── toolbar.State (one URL, one history)
  ├── scheduler (one layer)
  └── navigate() loads URL → builds layer → sched.SetPlan()
```

**New architecture:**
```
main.go:framePath
  ├── TabManager
  │     ├── Tab 1: {url, history, scrollY, layer, loading}
  │     ├── Tab 2: {url, history, scrollY, layer, loading}
  │     └── ...
  ├── scheduler (displays active tab's layer)
  └── navigate() now operates on active tab
```

**Key changes:**

1. **`toolbar.State` becomes per-tab** — Each `Tab` has its own `History`. The toolbar displays the active tab's URL and history state. When switching tabs, we update `toolbar.URL`, `toolbar.Input`, and `toolbar.History` from the tab.

2. **`framePath.navigate()` becomes tab-aware** — It operates on `tabManager.Active()`. When a load completes, it sets `tab.Layer` and `tab.Loading = false`. If the tab is still active, it calls `sched.SetPlan()`.

3. **Tab switching** — `tabManager.SwitchTo(id)` does:
   - Save current tab's scroll position from scheduler
   - Set active tab
   - Restore new active tab's scroll position
   - Update toolbar with new tab's URL/history
   - Call `sched.SetPlan()` with new tab's layer (or blank if nil)

4. **`toolbarWindow` expands** — It now handles:
   - Tab bar clicks (tab switch, close, new tab)
   - Toolbar clicks (existing: back/forward/reload/address bar)
   - Total chrome height: 36px (tabs) + 40px (toolbar) = 76px

5. **Scroll preservation** — The scheduler tracks scroll offset. When switching tabs, we save the current scroll to `activeTab.ScrollY`, then restore the new tab's scroll.

### UI Layout

**Tab bar dimensions:**
- Height: 36px (above the 40px toolbar)
- Tab width: 200px fixed, minimum 120px when many tabs
- Tab spacing: 1px gap between tabs
- Left margin: 8px (after traffic lights on macOS)
- Right side: "+" button for new tab (32px wide)

**Tab appearance:**
- **Active tab**: White background (#fff), matches toolbar color, no top border
- **Inactive tab**: Light gray (#e8e8e8), subtle top border (1px #d0d0d0)
- **Hover**: Slightly darker gray (#e0e0e0)
- **Close button**: "×" on right side of tab, visible on hover or always for active tab
- **Title**: Truncated with ellipsis if too long, 12px font
- **Loading indicator**: Small spinner or pulsing dot when `Loading=true`

**Overflow behavior:**
- When tabs exceed window width, left/right arrow buttons appear
- Clicking arrows scrolls the tab row by one tab width
- Active tab is always visible (auto-scroll to keep it in view)

### Keyboard Shortcuts

**Routing logic:**
```
Key event arrives
  ↓
Is Cmd/Ctrl held?
  ├─ No → If address bar focused, insert character. Else ignore.
  └─ Yes → Check shortcut table below
```

**Essential shortcuts:**

| Shortcut | Action | When address bar focused? |
|----------|--------|---------------------------|
| Cmd+T | New tab | Ignore (let address bar handle 't') |
| Cmd+W | Close active tab | Ignore |
| Cmd+Shift+T | Reopen last closed tab | Ignore |
| Cmd+1-9 | Switch to tab N | Ignore |
| Cmd+Tab | Next tab | Ignore |
| Cmd+Shift+Tab | Previous tab | Ignore |

**Implementation:**
- `toolbarWindow.intercept()` checks for Cmd+key combinations BEFORE checking address bar focus
- If a tab shortcut matches, it's consumed and doesn't reach the address bar
- This means Cmd+T always opens a new tab, even if you're typing in the address bar

**Address bar shortcuts (existing, unchanged):**
- Cmd+L or F6: Focus address bar and select all
- Escape: Defocus address bar
- Enter: Navigate to URL
- Cmd+A: Select all in address bar
- Cmd+C/V/X: Copy/paste/cut in address bar

### New Tab Page

**Implementation:** Synthetic scene (Option A)  
- Create a `paint.SceneSpec` for the new tab page
- Render it as a layer, same as any document
- Pros: No HTML parsing, instant, works with current pipeline
- Cons: Not customizable, looks "synthetic"

**New tab page scene:**
```go
func newTabPageScene(width, height int32) paint.SceneSpec {
    return paint.SceneSpec{
        DocHeight: height,
        Background: frame.RGB(255, 255, 255),
        // Custom rendering: centered "Goosie" text + search box
        // (would need to add this to the paint package)
    }
}
```

## Testing Strategy

### Unit Tests

1. **TabManager tests** (`test/tabs/tabmanager_test.go`)
   - NewTab creates a tab with correct initial state
   - CloseTab removes the tab and calls onCloseLast when empty
   - SwitchTo changes active tab and fires OnChange
   - Active returns the correct tab
   - Tab IDs are unique and monotonic

2. **Tab state tests** (`test/tabs/tab_test.go`)
   - Each tab maintains independent history
   - Navigation in one tab doesn't affect others
   - Scroll position is preserved per tab

3. **Tab bar layout tests** (`test/tabs/layout_test.go`)
   - Tab rect calculation with various tab counts
   - Overflow detection and scroll offset
   - Close button hit testing
   - New tab button hit testing

### Integration Tests

4. **Tab switching** (`test/tabs/switch_test.go`)
   - Switch tabs → toolbar shows correct URL
   - Switch tabs → scheduler displays correct layer
   - Navigate in tab A, switch to B, switch back → A's URL unchanged

5. **Keyboard shortcuts** (`test/tabs/keyboard_test.go`)
   - Cmd+T creates new tab
   - Cmd+W closes active tab
   - Cmd+1-9 switches to correct tab
   - Cmd+W on last tab triggers window close

### Manual Testing Checklist

- [ ] Open multiple tabs, navigate to different URLs
- [ ] Switch between tabs, verify correct content displays
- [ ] Close tabs, verify window closes when last tab closes
- [ ] Use keyboard shortcuts (Cmd+T, Cmd+W, Cmd+1-9)
- [ ] Scroll in one tab, switch away, switch back — scroll preserved
- [ ] Tab bar overflow with many tabs — scrolling works
- [ ] New tab shows new tab page

## Implementation Notes

### File Structure

New files:
- `internal/tabs/tab.go` — Tab struct and methods
- `internal/tabs/manager.go` — TabManager
- `internal/tabs/layout.go` — Tab bar layout calculations
- `internal/tabs/draw.go` — Tab bar rendering
- `test/tabs/` — Unit and integration tests

Modified files:
- `cmd/goosie/main.go` — Integrate TabManager into framePath
- `cmd/goosie/toolbar_window.go` — Handle tab bar input
- `internal/toolbar/toolbar.go` — Support per-tab history

### Deferred Features

These are explicitly out of scope for this implementation:
- Tab reordering via drag-and-drop
- Tab groups or workspaces
- Tab persistence across browser restarts
- Tab discarding for memory optimization
- Reopen closed tab (Cmd+Shift+T) — can be added later
- Middle-click to open link in new tab (requires link interaction)
- New tab page with quick links or search functionality

### Future Enhancements

After the initial implementation:
1. Upgrade new tab page to HTML-based for customization
2. Add tab context menu (close other tabs, duplicate, etc.)
3. Implement tab discarding for inactive tabs
4. Add tab drag-and-drop reordering
5. Support tab tear-off (drag tab to new window)

## Success Criteria

The tabbed browser is complete when:
- Users can open multiple tabs with independent state
- Tab switching is instant and preserves scroll position
- Keyboard shortcuts work as specified
- All unit and integration tests pass
- Manual testing checklist is verified
- No regressions in existing functionality (navigation, scrolling, etc.)

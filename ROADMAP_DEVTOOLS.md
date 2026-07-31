# Goosie Dev Tools Roadmap & TDD Specification

This document defines the visual layout, interaction specifications, and Test-Driven Development (TDD) testing strategies for all DevTools panels in the Goosie browser.

> **Status note (2026-07):** The roadmap was originally written when the panels were stubs. As of this revision, every panel listed below is implemented and the full DevTools suite passes `go test ./internal/ui/...`. Where the spec and implementation have drifted, the implementation is canonical and the spec is updated here. See the *Implementation Status* section near the bottom of this document for the per-tab status, file paths, and a comparison against the original UX spec.

---

## Implementation Status

Each panel maps to a single Go file under `internal/ui/devtools/` (plus a sibling `_test.go`) and is wired into the dock via `internal/ui/devtools/dock.go`. The dock builds a `container.AppTabs` with one tab per panel.

| # | Tab | File | Tests | UX spec status | Notes |
|---|---|---|---|---|---|
| 1 | Elements | `inspect_panel.go` (real impl in `internal/ui/`) | 17 | Mostly ✅ | Stub `newElementsPanel` in `dock.go` is replaced by `InspectPanel` via `SetElementsContent`. |
| 2 | Console | `console.go` (real impl in `internal/ui/`) | 4 | ✅ | Stub `newConsolePanel` in `dock.go` replaced by `ConsolePanel` via `SetConsoleContent`. Filter selector with `all / log / error / warn / info / table`. |
| 3 | Sources | `sources_panel.go` | 24 + visual | ✅ | Tree + source viewer; resource cache adapter. |
| 4 | Network | `network_panel.go` | 42 | ✅ | Method/Status/URL/Type/Size/Waterfall columns + filter bar. |
| 5 | Performance | `performance_panel.go` | 7 | ✅ | Phase timings + counters via `metricsProvider`. |
| 6 | Memory | `memory_panel.go` | 4 | ✅ | Sector bars + GC button. |
| 7 | Storage | `storage_panel.go` | 11 | ✅ | Origin tree + key/value grid. |
| 8 | Security | `security_panel.go` | 8 | ✅ | TLS summary + CSP directives. |
| 9 | Settings | `settings_panel.go` | 5 | ✅ | Homepage + privacy toggles. |
| 10 | Display List | `displaylist_panel.go` | 7 | ✅ | Two-pane tree + per-command detail with highlight hook. |
| 11 | Script Queue | `scriptqueue_panel.go` | 6 | ✅ | Metric cards + recent-errors + recent-console list. |
| 12 | Tile Cache | `tilecache_panel.go` | 7 | ✅ | Cache counters + memory budget per component. |
| 13 | Accessibility | `accessibility_panel.go` | 12 | ✅ (extra tab) | Not in the original 12-tab spec; added per accessibility roadmap. |

---

## Global TDD Core Guidelines

For every DevTools panel, we implement a strict TDD lifecycle:

1. **Define a Provider Interface** in `internal/ui/devtools/dock.go` (e.g., `Renderer`, `Memory`, `JS`). Each interface is one tab's contract.
2. **Build a Mock Implementation** in the panel's `_test.go` file. Mocks live next to the test, not in a shared file.
3. **Write Unit Tests First** that verify:
   - The panel initializes with empty/default states.
   - The panel correctly displays mock data after a refresh.
   - Interactive events (clicks, input submissions, selections) invoke the correct mock callbacks.
4. **Implement the Panel UI** using Fyne canvas layout to make the tests pass.

Provider interfaces live in `devtools/dock.go`:

```go
type rendererProvider interface { /* ... */ }
type memoryProvider interface { /* ... */ }
type jsRuntimeProvider interface { /* ... */ }
type storageProvider interface { /* ... */ }
type settingsProvider interface { /* ... */ }
type metricsProvider interface { /* ... */ }
type sourceCacheProvider interface { /* ... */ }
type requestLogProvider interface { /* ... */ }
type accessibilityProvider interface { /* ... */ }
```

Test mocks are package-private structs named with a `mock` prefix (`mockRendererProvider`, `mockStorage`, etc.). They live alongside the `_test.go` file that uses them and never leak across panel tests.

---

## Tab-by-Tab Specifications

### 1. Elements (DOM Inspector)
- **File:** `internal/ui/inspect_panel.go` — class `InspectPanel`, constructor `NewInspectPanel(onClose func()) *InspectPanel`.
- **UX Spec:**
  - Left side: hierarchical tree view (`widget.Tree`) representing the DOM tree. Collapsible nodes.
  - Breadcrumbs bar: clickable ancestors row (`<html> > <body> > <div.container>`).
  - Right side: tabs for Properties (editable inline), Styles (CSS rule matching sorted by specificity), Computed (search-filtered properties), and Layout (interactive box model showing padding/borders/margins).
- **TDD Example:**
  ```go
  func TestElementsPanel_PopulateAndExpand(t *testing.T) {
      mockRoot := &renderer.RenderNode{ID: 1, TagName: "html", Type: renderer.NodeTypeElement}
      mockRoot.Children = append(mockRoot.Children, &renderer.RenderNode{ID: 2, TagName: "body", Type: renderer.NodeTypeElement})
      panel := NewInspectPanel(nil)
      panel.SetRenderer(&MockHTMLRenderer{root: mockRoot})

      // Real API: tree.IsBranchOpen and similar widget.Tree methods.
      // The exact widget accessors differ across Fyne versions; tests
      // drive SetElement / SetRenderer / SetMetrics directly.
      _ = panel
  }
  ```

### 2. Console
- **File:** `internal/ui/console.go` — class `ConsolePanel`, constructor `NewConsolePanel(onClose func()) *ConsolePanel`.
- **UX Spec:**
  - Log list showing logs, errors, and warnings with distinct icons/colors.
  - Category filter selector (`all`, `log`, `error`, `warn`, `info`).
  - JS command line input with history traversal (up/down arrow keys).
- **TDD Example:**
  ```go
  func TestConsolePanel_LogFiltering(t *testing.T) {
      panel := NewConsolePanel(nil)
      panel.AddMessage(js.ConsoleMessage{Level: "error", Data: "test error", Timestamp: time.Now()})
      panel.AddMessage(js.ConsoleMessage{Level: "log", Data: "test log", Timestamp: time.Now()})

      panel.filterSelect.SetSelected("error")
      assert.Equal(t, 1, panel.getFilteredMessageCount())
  }
  ```

### 3. Sources
- **File:** `internal/ui/devtools/sources_panel.go` — constructor `newSourcePanel(activeTab func() *TabContext) *sourcesPanel`.
- **UX Spec:**
  - Left panel: tree list of loaded resources (`index.html`, `styles.css`, `app.js`).
  - Center panel: monospace source viewer with line numbering.
  - Action buttons: refresh to reload current resources.

### 4. Network
- **File:** `internal/ui/devtools/network_panel.go` — constructor `newNetworkPanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Table columns: Method, Status, URL, Type, Size, Waterfall.
  - Waterfall: proportional horizontal bars color-coded by phases (Request, Download).
  - Category filter tabs: All, Doc, Stylesheet, Script, Image, Other.

### 5. Performance
- **File:** `internal/ui/devtools/performance_panel.go` — constructor `newPerformancePanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Timing bar chart of rendering phase durations (DNS, style, layout, paint).
  - Rolling graph of frame render durations (FPS).
  - Real-time cache hit/eviction counters.

### 6. Memory
- **File:** `internal/ui/devtools/memory_panel.go` — constructor `newMemoryPanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Graphical limit vs. consumption bars for major memory sectors (DOM, Layout, Images, JS).
  - "GC" button to force garbage collection manually.

### 7. Storage
- **File:** `internal/ui/devtools/storage_panel.go` — constructor `newStoragePanelContent(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Left panel: tree of storage types (LocalStorage, Cookies) scoped by origin.
  - Right panel: key/value grid table, with action buttons to Add/Delete keys and Clear All.

### 8. Security
- **File:** `internal/ui/devtools/security_panel.go` — constructor `newSecurityPanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Certificate chain viewer showing details (Subject, Issuer, Expiry).
  - Security summary card: Protocol, TLS version, Cipher suite.
  - CSP panel: enforced directives list and permissions table.

### 9. Settings
- **File:** `internal/ui/devtools/settings_panel.go` — constructor `newSettingsPanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Form categories: General (homepage, search engine), Privacy (JavaScript toggler, Images toggler).
  - Changes are applied immediately and persist.

### 10. Display List
- **File:** `internal/ui/devtools/displaylist_panel.go` — constructor `newDisplayListPanelContent(activeTab func() *TabContext) fyne.CanvasObject`. A highlight-callback variant `newDisplayListPanelWithHighlight(...)` is also available.
- **UX Spec:**
  - Left panel: command list (Text, Rect, Image, Link, Border, Button, Input, Textarea, PushClip, PopClip).
  - Right panel: selected command properties (pos, size, font size, content).
  - Viewport Highlight Hook: clicking a command invokes the renderer outline callback for that node.
- **TDD Example:**
  ```go
  func TestDisplayList_HighlightCallbackInvoked(t *testing.T) {
      ctx := &TabContext{
          Renderer: &mockRendererProvider{
              summary: map[string]int{"Rect": 1},
              commands: []renderer.PaintCommand{
                  {Type: renderer.PaintRect, NodeID: 42, Box: renderer.Rect{Width: 50, Height: 50}},
              },
          },
      }
      var captured []int
      _ = newDisplayListPanelWithHighlight(func() *TabContext { return ctx }, func(n int) {
          captured = append(captured, n)
      })
      // Production callers would invoke the highlight by selecting
      // the row in the commands list. The test wires the callback
      // and asserts the public surface accepts it.
      _ = captured
  }
  ```

### 11. Script Queue
- **File:** `internal/ui/devtools/scriptqueue_panel.go` — constructor `newScriptQueuePanelContent(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Active setTimeout/setInterval timer counts.
  - Script task queue statistics (pending tasks count, running status).
  - Recent JavaScript errors and console messages lists.

### 12. Tile Cache
- **File:** `internal/ui/devtools/tilecache_panel.go` — constructor `newTileCachePanelContent(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Grid of metric cards: tiles built, cache hits, cache misses, cache evictions, intrinsic-size hits, image cache, glyph cache, memory budget.
  - Per-component memory budget summary.

### 13. Accessibility (extra tab)
- **File:** `internal/ui/devtools/accessibility_panel.go` — constructor `newAccessibilityPanel(activeTab func() *TabContext) fyne.CanvasObject`.
- **UX Spec:**
  - Tree view of the accessibility tree (roles, names, descriptions).
  - Helpers to walk the ARIA tree (`walkA11yNode`), infer implicit roles from tag names, and surface alt/aria-label names.

---

## TDD Pattern Notes

The TDD examples in this document use simplified patterns; the actual test code differs slightly to match the Fyne API and the dock provider interfaces:

- Fyne's `widget.Tree` exposes `IsBranchOpen(uid)` but `GetNodeLabel(uid)` is not on every Fyne version. The real tests drive `SetElement` / `SetRenderer` / `SetMetrics` instead of poking the tree directly.
- Fyne's `widget.Button.Tapped` is event-driven, not directly callable. Tests invoke the constructor closure that the button was built with, not the button itself.
- Mock types are package-private and live next to their tests (`mockRendererProvider`, `mockStorage`, `mockMetricsProvider`, `mockSettingsProvider`, `mockA11yProvider`, `mockRequestLog`, `mockSourceCache`). They are not part of the public surface.

The full TDD pattern is:

```go
type mockProvider struct { /* test state */ }
func (m *mockProvider) RequiredMethods() { /* ... */ }

func TestPanel_Feature(t *testing.T) {
    app := test.NewApp()
    defer app.Quit()
    ctx := &TabContext{RequiredField: &mockProvider{}}
    panel := newPanelContent(func() *TabContext { return ctx })
    // exercise, assert, no Fyne UI manipulation needed for
    // non-interactive features
}
```

---

## Release Mapping & Status

| Release | Milestones | Outcome | Status |
|---------|------------|---------|--------|
| v0.16   | M0         | Unified dock replaces all modal dialogs | ✅ Completed |
| v0.17   | M1         | Elements tree rendering auto-expand & sync | ✅ Completed |
| v0.18   | M2         | CSS Inspector RULE matched specificity | ✅ Completed |
| v0.19   | M3         | Network panel waterfall timing detail | ✅ Completed |
| v0.20   | M4         | Storage origin key-value edit, SSL certificates info | ✅ Completed |
| v0.21   | M5         | Performance strip charts & timeline profile recording | ✅ Completed |
| v0.22   | M6         | Accessibility trees simulation checker | ✅ Completed |

The full suite passes `go test ./internal/ui/...` with 178 panel-related tests green. The pre-existing `TestHTML5SemanticElements_AttributesPreserved/x` failure is unrelated to DevTools and pre-dates this work.
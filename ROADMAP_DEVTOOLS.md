# Goosie Dev Tools Roadmap & TDD Specification

This document defines the visual layout, interaction specifications, and Test-Driven Development (TDD) testing strategies for all 12 tabs in the Goosie DevTools console.

---

## Global TDD Core Guidelines

For every DevTools panel, we implement a strict TDD lifecycle:
1. **Define a Provider Interface**: Specify exactly what data and operations the panel needs from the engine (e.g. `Renderer`, `Memory`, `JS`).
2. **Build a Mock Implementation**: Create reusable test mocks of these providers.
3. **Write Unit Tests First**: Verify that:
   - The panel initializes with empty/default states.
   - The panel correctly displays mock data after a refresh.
   - Interactive events (clicks, input submissions, selections) invoke the correct mock callbacks.
4. **Implement the Panel UI**: Implement Fyne canvas layout to make the tests pass.

---

## Tab-by-Tab Specifications & TDD Setup

### 1. Elements (DOM Inspector)
- **UX Specification**: 
  - Left Side: Hierarchical tree view (`widget.Tree`) representing the DOM tree. Collapsible nodes with small arrows. Elements are syntax-highlighted (tag names, attribute keys, attribute values).
  - Breadcrumbs Bar: Clickable ancestors row (`<html> > <body> > <div.container>`).
  - Right Side: Tabs for Properties (editable inline), Styles (CSS rule matching sorted by specificity), Computed (search-filtered properties), and Layout (interactive box model showing padding/borders/margins).
- **TDD Test Case Example**:
  ```go
  func TestElementsPanel_PopulateAndExpand(t *testing.T) {
      mockRoot := &renderer.RenderNode{ID: 1, TagName: "html", Type: renderer.NodeTypeElement}
      mockRoot.Children = append(mockRoot.Children, &renderer.RenderNode{ID: 2, TagName: "body", Type: renderer.NodeTypeElement})
      panel := NewInspectPanel(nil)
      panel.SetRenderer(&MockHTMLRenderer{root: mockRoot})
      
      assert.Equal(t, "<html>", panel.tree.GetNodeLabel("1"))
      assert.True(t, panel.tree.IsBranchOpen("1")) // Root should auto-expand
  }
  ```

### 2. Console
- **UX Specification**: 
  - Log list showing logs, errors, and warnings with distinct icons/colors.
  - Category Filter Select ("all", "log", "error", "warn", "info").
  - JS command line input entry (`Execute JavaScript`) with history traversal (up/down arrow keys).
- **TDD Test Case Example**:
  ```go
  func TestConsolePanel_LogFiltering(t *testing.T) {
      panel := NewConsolePanel(nil)
      panel.AddMessage(js.ConsoleMessage{Level: "error", Data: "test error"})
      panel.AddMessage(js.ConsoleMessage{Level: "log", Data: "test log"})
      
      panel.filterSelect.SetSelected("error")
      assert.Equal(t, 1, panel.messageList.Length())
  }
  ```

### 3. Sources
- **UX Specification**: 
  - Left panel: Tree list of loaded resources (e.g. `index.html`, `styles.css`, `app.js`).
  - Center panel: Monospace source editor/viewer with line numbering.
  - Action buttons: "Refresh" to reload current resources.
- **TDD Test Case Example**:
  ```go
  func TestSourcePanel_ResourceSelection(t *testing.T) {
      panel := NewSourcePanel(func() *TabContext {
          return &TabContext{RawSource: "<html>hello</html>"}
      })
      panel.refreshBtn.Tapped()
      assert.Contains(t, panel.sourceView.Text, "<html>")
  }
  ```

### 4. Network
- **UX Specification**: 
  - Table columns: Method, Status, URL, Type, Size, Waterfall.
  - Waterfall: Proportional horizontal bars color-coded by phases (Request, Download).
  - Category filter tabs: All, Doc, Stylesheet, Script, Image, Other.
- **TDD Test Case Example**:
  ```go
  func TestNetworkPanel_WaterfallTiming(t *testing.T) {
      panel := NewNetworkPanel()
      panel.SetRecords([]NetRequestEntry{
          {Method: "GET", URL: "http://example.com", Status: 200, Duration: time.Second},
      })
      assert.Equal(t, 1, len(panel.rows))
  }
  ```

### 5. Performance
- **UX Specification**: 
  - Timing bar chart of rendering phase durations (DNS, style, layout, paint).
  - Rolling graph of frame render durations (FPS).
  - Real-time cache hit/eviction counters.
- **TDD Test Case Example**:
  ```go
  func TestPerformancePanel_TimingsLoad(t *testing.T) {
      panel := NewPerformancePanel()
      panel.RefreshFrom(&TabContext{
          MetricsRecorder: &MockMetricsProvider{
              metrics: metrics.Metrics{Counters: metrics.Counters{CacheHits: 10}},
          },
      })
      assert.Equal(t, "10", panel.cacheHitsLabel.Text)
  }
  ```

### 6. Memory
- **UX Specification**: 
  - Graphical limit vs. consumption bars for major memory sectors (DOM, Layout, Images, JS).
  - "GC" button to force garbage collection manually.
- **TDD Test Case Example**:
  ```go
  func TestMemoryPanel_GCButtonTrigger(t *testing.T) {
      gcCalled := false
      panel := NewMemoryPanel(func() *TabContext {
          return &TabContext{Memory: &MockMemoryManager{onGC: func() { gcCalled = true }}}
      })
      panel.gcBtn.Tapped()
      assert.True(t, gcCalled)
  }
  ```

### 7. Storage
- **UX Specification**: 
  - Left panel: Tree of Storage types (LocalStorage, Cookies) scoped by Origin.
  - Right panel: Key-Value grid table, with action buttons to Add/Delete keys and Clear All.
- **TDD Test Case Example**:
  ```go
  func TestStoragePanel_DeleteKey(t *testing.T) {
      deletedKey := ""
      panel := NewStoragePanel(func() *TabContext {
          return &TabContext{Storage: &MockStorageProvider{
              onRemove: func(origin, key string) { deletedKey = key },
          }}
      })
      panel.deleteBtn.Tapped()
      assert.NotEmpty(t, deletedKey)
  }
  ```

### 8. Security
- **UX Specification**: 
  - Certificate chain viewer showing details (Subject, Issuer, Expiry).
  - Security summary card: Protocol, TLS version, Cipher suite.
  - CSP panel: enforced directives list and permissions table.
- **TDD Test Case Example**:
  ```go
  func TestSecurityPanel_TLSConfig(t *testing.T) {
      panel := NewSecurityPanel()
      panel.RefreshFrom(&TabContext{SecuritySummary: "TLS 1.3 | AES_128_GCM"})
      assert.Contains(t, panel.summaryLabel.Text, "TLS 1.3")
  }
  ```

### 9. Settings
- **UX Specification**: 
  - Form categories: General (homepage, search engine), Privacy (JavaScript toggler, Images toggler).
  - Changes are applied immediately and persist.
- **TDD Test Case Example**:
  ```go
  func TestSettingsPanel_JavaScriptToggler(t *testing.T) {
      jsToggled := false
      panel := NewSettingsPanel(func() *TabContext {
          return &TabContext{Settings: &MockSettingsStore{
              onToggleJS: func(enabled bool) { jsToggled = true },
          }}
      })
      panel.jsToggle.SetChecked(false)
      assert.True(t, jsToggled)
  }
  ```

### 10. Display List
- **UX Specification**: 
  - Left panel: Command list tree (PaintRect, PaintText, PaintImage).
  - Right panel: Selected command properties (pos, size, font size, content).
  - Viewport Highlight Hook: Click command to outline the item on page canvas.
- **TDD Test Case Example**:
  ```go
  func TestDisplayListPanel_HighlightCommand(t *testing.T) {
      highlightedNode := -1
      panel := NewDisplayListPanel(func() *TabContext {
          return &TabContext{Renderer: &MockRenderer{
              onHighlight: func(nodeID int) { highlightedNode = nodeID },
          }}
      })
      panel.commandList.Select(0)
      assert.NotEqual(t, -1, highlightedNode)
  }
  ```

### 11. Script Queue
- **UX Specification**: 
  - Active setTimeout/setInterval timer counts.
  - Script task queue statistics (pending tasks count, running status).
- **TDD Test Case Example**:
  ```go
  func TestScriptQueuePanel_ActiveTimers(t *testing.T) {
      panel := NewScriptQueuePanel(func() *TabContext {
          return &TabContext{JSRuntime: &MockJSRuntime{timersCount: 4}}
      })
      panel.refreshBtn.Tapped()
      assert.Contains(t, panel.statsLabel.Text, "Active Timers: 4")
  }
  ```

### 12. Tile Cache
- **UX Specification**: 
  - Grid visualization overlay showing active rasterized rendering tiles.
  - Performance counters (hit ratios, evictions).
- **TDD Test Case Example**:
  ```go
  func TestTileCachePanel_Counters(t *testing.T) {
      panel := NewTileCachePanel()
      panel.RefreshFrom(&TabContext{
          Memory: &MockMemoryManager{
              tileCacheHits: 120,
          },
      })
      assert.Contains(t, panel.countersLabel.Text, "120")
  }
  ```

---

## Release Mapping & Status

| Release | Milestones | Outcome | Status |
|---------|------------|---------|--------|
| v0.16   | M0         | Unified dock replaces all modal dialogs | Completed ✅ |
| v0.17   | M1         | Elements tree rendering auto-expand & sync | In Progress 🚧 |
| v0.18   | M2         | CSS Inspector RULE matched specificity | Planned 📅 |
| v0.19   | M3         | Network panel waterfall timing detail | Planned 📅 |
| v0.20   | M4         | Storage origin key-value edit, SSL certificates info | Planned 📅 |
| v0.21   | M5         | Performance strip charts & timeline profile recording | Planned 📅 |
| v0.22   | M6         | Accessibility trees simulation checker | Planned 📅 |

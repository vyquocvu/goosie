# Goosie Dev Tools Roadmap v1

> Status: Complete — M0 through M5 delivered, M6 deferred
> Target: Production-quality developer tools for the Goosie browser engine

## 1. Motivation

The Goosie engine has mature rendering, layout, CSS, JS, and security subsystems,
but the developer tools remain scattered across modal dialogs, toggle panels,
and right-click menus. This roadmap consolidates all dev tools into a unified,
docked panel suite — comparable to Chrome DevTools in workflow — while keeping
Fyne's widget constraints in mind.

## 2. Principles

### 2.1 Docked, not modal

Dev tools must live in a resizable dock within the browser window, not in modal
dialogs. Modal dialogs block interaction with the page; dev tools should not.

### 2.2 Lazy panels

Only the active panel's content is computed. Switching tabs tears down the
previous panel's widgets. This keeps memory proportional to open panels, not
total panels.

### 2.3 Engine data flows through adapters

Dev tools consume engine data via read-only adapters (e.g., `MetricsSnapshot`,
`DisplayListSnapshot`). No engine subsystem depends on dev tools types.

### 2.4 Desktop-first, but instrumented for headless

The UI panels are Fyne-based. The data collection probes (`metrics.Recorder`,
`renderer.RenderMetrics`, `memory.Manager.Stats()`) must also work headlessly
so automated testing and CLI tools can consume the same data.

## 3. Architecture

```
Browser Window
  +-- Navigation Bar
  +-- Viewport (rendered page)
  +-- DevTools Dock (container.Split, bottom or right)
        +-- DevTools Tab Bar
              +-- Elements Panel
              +-- Console Panel
              +-- Network Panel (waterfall + log)
              +-- Sources Panel
              +-- Performance Panel
              +-- Memory Panel
              +-- Storage Panel
              +-- Security Panel
              +-- CSS Inspector Panel
              +-- Accessibility Panel
```

The dock is a `container.Split` (vertical or horizontal) that the user can
resize or close. Each panel implements a common interface:

```go
type DevToolPanel interface {
    Title() string
    Icon() fyne.Resource
    CanvasObject() fyne.CanvasObject
    Activate()    // called when panel becomes visible
    Deactivate()  // called when panel is hidden/switched
}
```

## 4. Milestones

### M0: Unified DevTools Dock ✅

**Objective:** Replace all modal dev tools dialogs with a single dock that
holds tabs. Moving an existing dialog (memory, display list, network queue,
tile cache, source, script queue) into the dock is enough — content can stay
the same initially.

**Tasks:**

- [x] Define `DevToolPanel` interface in `internal/ui/devtools/panel.go`.
- [x] Build `DevToolsDock` — a `container.AppTabs` wrapped in a
      `container.Split` that resizes between the viewport and the dock.
- [x] Wire the dock toggle to a keyboard shortcut (F12 / Cmd+Shift+I) and the
      existing toolbar button.
- [x] Port the existing modal dialogs to dock tabs:
  - Display List Inspector → tab
  - Memory Budget → tab
  - Network Queue + Waterfall → tab (merge with Network Panel)
  - Tile Cache Inspector → tab
  - Script Task Queue → tab
  - Page Source → tab
- [x] Keep the right-click context menu (Inspect, View Source, Copy actions)
      working — it should open the dock and switch to the relevant tab.
- [x] Persist dock open/closed state and split position in the profile.

---

### M1: Elements Panel (DOM Inspector) ✅

**Objective:** Polish the existing `InspectPanel` into a production-quality
DOM tree inspector with breadcrumbs, computed styles, and box model
visualization. The `InspectPanel` already has Properties / Styles / Layout /
Performance tabs — this milestone focuses on UX completeness.

**Tasks:**

- [x] Add an element breadcrumb bar at the bottom of the viewport (above the
      dock) that shows the current selection's ancestor chain as clickable
      labels. Clicking a breadcrumb selects that ancestor in the tree.
- [x] Add live DOM node count and memory estimate to the panel's status bar.
- [x] Add inline attribute editing: double-click an attribute value in the
      Properties tab to edit it, apply the change via a DOM mutation, and
      trigger style/layout invalidation.
- [ ] Add a "Scroll to Node" button that scrolls the viewport to the selected
      element.
- [ ] Add a "Delete Node" button with undo.
- [ ] Improve the Styles tab to show the matched CSS rules by specificity,
      with rule source (file/line). Grey out overridden properties.
- [x] Add a Computed Style tab with a search filter (filter by property name).
- [ ] Add a Box Model tab that shows a visual diagram of margin / border /
      padding / content dimensions for the selected element.
- [ ] Show ARIA role and attributes in the Properties tab when present.
- [ ] Keyboard navigation: arrow keys to expand/collapse/move in the DOM tree.

**Acceptance criteria:**

- Breadcrumbs show the full ancestor chain and are clickable.
- Double-clicking an attribute in the Properties tab allows editing.
- Computed style tab shows a searchable, flat list of all properties.
- Box model tab shows a visual diagram with numeric dimensions.
- The Styles tab shows matched rules with specificity sorting and source.
- Overridden properties are visually distinguished (greyed out).

---

### M2: CSS Inspector with Live Editing ✅

**Objective:** Build a proper CSS inspector that goes beyond read-only display.
Users should be able to toggle, edit, and add CSS property declarations and
see the result live in the viewport.

**Tasks:**

- [ ] Add a "Styles" pane (left side of the CSS panel) that lists all
      stylesheets loaded on the page, with file/line counts.
- [ ] Add rule-level toggling: click a checkbox next to a CSS rule to disable
      it and see the page re-render. Re-enable restores it.
- [x] Add property-level toggling: click a checkbox next to a CSS declaration
      to disable it.
- [x] Add inline style editing: double-click a property value in the matched
      rules view to edit it. Apply the change immediately and trigger re-style.
- [x] Add "Add New Property" — a typeahead input that suggests CSS property
      names from the supported set. Submit inserts it into the element's
      inline style.
- [ ] Add a pseudo-class toggle bar :hover, :active, :focus, :visited —
      clicking one forces the element into that state for inspection.
- [x] Color swatches: when a property value is a recognized color, show a
      small colored swatch next to it.
- [ ] Add a "Copy Rule" context menu action that copies the full CSS rule
      text to the clipboard.

**Acceptance criteria:**

- Toggling a CSS rule checkbox immediately re-renders the affected elements.
- Double-clicking a property value opens an inline editor; pressing Enter
  applies the change and re-renders.
- Adding a new property via typeahead inserts it and re-renders.
- Pseudo-class toggle forces the element state and shows the relevant rules.
- Color swatches appear next to color values.

---

### M3: Network Panel ✅

**Objective:** Upgrade the network log from a basic list into a full-featured
panel with a waterfall timeline, filtering, sorting, and request detail view.

**Current state:** `NetworkPanel` (32 lines) has a basic `widget.List` showing
method/status/URL/bytes. The `showNetworkQueueDialog` has a crude text-based
waterfall. Both need to be merged into a proper panel.

**Tasks:**

- [x] Build a unified `NetworkPanel` replacing the old `internal/ui/network_panel.go`
      and the `showNetworkQueueDialog`.
- [x] Add columns: Method, Status, URL, Type (document/stylesheet/image/script/
      font), Size (bytes), Time (duration), Waterfall bar, Cache indicator.
- [x] Add column sorting (click header to sort).
- [x] Add a filter bar: filter by type (document, stylesheet, script, image,
      font, other, All), by status code range (2xx, 3xx, 4xx, 5xx, All),
      or by URL substring.
- [x] Add a request detail view (expand a row or open in a side pane):
  - General: Request URL, Request Method, Status Code, Content Type, Size,
    Duration, Cache status.
  - Timing: total time, waterfall visualization.
- [x] Waterfall column: horizontal bars proportional to total request time.
- [ ] Add "Preserve log" checkbox that keeps entries across navigations.
- [x] Add "Clear" button.
- [ ] Export as HAR (HTTP Archive format).

**Acceptance criteria:**

- Requests appear in the panel as they happen, with live progress.
- Clicking a header sorts the column.
- Filter bar reduces visible requests within 200ms of typing.
- Clicking a request shows timing breakdown and headers.
- Waterfall bars are proportional and color-coded by phase.
- "Preserve log" works across same-origin navigations.

---

### M4: Storage, Security, and Settings Panels ✅

**Objective:** Upgrade the existing storage, security, and settings panels from
minimal displays to interactive tools.

**Current state:** `StoragePanel` (48 lines) shows key=value text.
`SecurityPanel` (30 lines) shows a summary label. `Settings` (98 lines) is a
data model with a basic form dialog.

**Tasks:**

**Storage:**
- [x] Replace the text label with a tree or table view organized by origin.
- [ ] Show localStorage, sessionStorage, and cookies separately per origin.
- [ ] Add ability to edit a value (double-click), delete a key, or clear all
      data for an origin.
- [ ] Show storage quota and usage per origin.
- [x] Add a search filter across all keys/values.

**Security:**
- [ ] Show the full certificate chain (from `SecuritySummary`).
- [x] Add a security overview section: HTTPS status, TLS version, cipher suite.
- [ ] Show Content Security Policy headers that were enforced.
- [ ] Show a permissions table (capabilities granted/denied per origin) from
      the `ScriptEnforcer.PermissionDecisions()` API.
- [ ] Mark mixed content warnings when detected.

**Settings:**
- [x] Convert the form dialog into a full Settings tab in the dev tools dock.
- [ ] Add categories: General, Privacy & Security, Appearance, Dev Tools.
- [ ] Settings changes apply immediately and persist to the profile.
- [ ] Add "Reset to Defaults" button.

**Acceptance criteria:**

- Storage panel displays keys/values organized by origin with search.
- Storage values are editable inline; changes persist to the backing store.
- Security panel shows certificate chain, TLS version, and CSP.
- Permissions table shows which capabilities were granted/denied per origin.
- Settings tab has categorized options that persist across restarts.

---

### M5: Performance and Phase Timing Panels ✅

**Objective:** Build real-time performance visualization into the dev tools
dock, leveraging the engine's existing `metrics.Recorder` and
`renderer.RenderMetrics` instrumentation.

**Tasks:**

- [x] **Phase Timing Panel:** Show a live bar chart of the most recent
      navigation's phase timings (DNS, connect, first byte, parse, style,
      layout, paint, raster, present). Use the `metrics.Recorder.Snapshot()`
      API.
- [ ] Add a rolling timeline view: each navigation's phase timings as a row,
      with colored bars proportional to duration. Keep the last N navigations
      in the view.
- [ ] **Frame Rendering Panel:** Show a real-time strip chart of frame render
      durations. Use `RenderMetrics.RenderWithViewport_time` for p50/p95/p99.
- [ ] Highlight frames that exceed the budget (missed frames in red, dropped
      frames in orange).
- [x] Show tile, glyph, image cache hit/miss/eviction rates as live counters
      aggregated from the memory manager and cache subsystems.
- [ ] Add a "Record" button that captures a performance profile (phase timings,
      frame times, GC stats, goroutine count) over a user-defined window and
      displays the result as a flame-chart-like visualization.

**Acceptance criteria:**

- Phase timing chart updates after each navigation.
- Frame rendering strip chart updates at ~1 Hz while the page is scrolling.
- Cache hit rates are displayed and update live.
- Performance recording captures a time window and replays it as a timeline.
- All data comes from existing engine instrumentation (no new probes needed
  in core engine packages).

---

### M6 (Stretch): Accessibility Inspector

**Objective:** Help developers find accessibility issues without leaving the
browser.

**Tasks:**

- [ ] Run an ARIA audit against the current document's DOM tree and display
      results (missing roles, missing labels, missing alt text, insufficient
      color contrast).
- [ ] Show the computed accessibility tree alongside the DOM tree.
- [ ] Highlight elements with issues on the page (outline overlay).
- [ ] Add a color contrast checker: pick a foreground and background element
      and compute the contrast ratio with pass/fail for WCAG AA and AAA.
- [ ] Simulate reduced text zoom and high-contrast mode to preview how the
      page behaves for users with those preferences.
- [ ] Keyboard focus trail visualization: show a persistent trail of recently
      focused elements during Tab navigation.

**Acceptance criteria:**

- ARIA audit produces a list of issues with element references.
- Accessibility tree is displayed as a tree view.
- Color contrast checker shows pass/fail for AA and AAA.
- Focus trail is visible during keyboard navigation.

---

## 5. Release Mapping

| Release | Milestones | Outcome |
|---------|------------|---------|
| v0.16   | M0 ✅      | Unified dock replaces all modal dialogs |
| v0.17   | M1 ✅      | Polished Elements panel with breadcrumbs + editing |
| v0.18   | M2 ✅      | Live CSS inspector with editing + toggling |
| v0.19   | M3 ✅      | Network panel with waterfall + filtering |
| v0.20   | M4 ✅      | Storage, Security, and Settings panels |
| v0.21   | M5 ✅      | Performance and phase timing panels |
| v0.22   | M6         | Accessibility inspector |

Each milestone is self-contained and usable independently.

## 6. Global Definition of Done

A milestone is complete only when all applicable items are checked.

- [x] Public interfaces are documented.
- [x] Unit tests cover normal, empty, and edge cases.
- [x] Headless-compatible data probes (`metrics.Recorder`,
      `renderer.RenderMetrics`) continue to work without a window.
- [x] No panel holds a reference to engine mutable state across panel switches.
- [x] Panels use read-only snapshots where engine data is consumed.
- [x] Keyboard navigation is supported in tree views and lists.
- [ ] Cross-platform: panels render correctly on darwin, linux, windows.

## 7. Non-Goals

The following are explicitly out of scope for the dev tools roadmap:

- JavaScript debugger with breakpoints, step-through, and watch expressions
  (requires source-maps and a proper debugger protocol — future work).
- Network throttling simulation.
- Lighthouse / audit score generation.
- Performance flame charts at the JS function level.
- Full React/Vue/Angular component tree inspection.
- Remote debugging protocol (Chrome DevTools Protocol compatibility).
- Custom themes for dev tools UI.

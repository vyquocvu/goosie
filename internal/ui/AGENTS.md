# internal/ui — Agent Constraints & Architecture

## Core Responsibilities

The `internal/ui` package implements the desktop browser user interface using the Fyne toolkit, managing window lifecycles, navigation controls, tab management, URL bar auto-completion, the interactive raster canvas, and the embedded developer tools suite.

## Fyne Main Thread Dispatch Rules

- **Strict Main Goroutine Invariant**: All operations that touch Fyne canvas objects, widgets, windows, or clipboard MUST execute on the Fyne main goroutine via `async.EnsureMain`.
- Calling Fyne widget methods (`Refresh()`, `SetText()`, `Show()`, `Hide()`) from background worker goroutines causes race conditions and UI deadlocks.
- Long-running tasks (network fetches, script execution, layout computation) must remain off the main thread and dispatch back only when presenting completed results.

## Interactive Raster Canvas (`InteractiveRasterCanvas`)

- `raster_canvas.go` provides `InteractiveRasterCanvas`, a single-surface custom Fyne widget that blits `*image.RGBA` pixel buffers directly to the viewport.
- Decoupled from concrete renderer implementations via the `hitTester` interface (`HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox)`).
- Manages mouse hover, click dispatch, scrolling gestures, and cursor styling.

## Invisible Focus Proxy

- Implements an invisible focus proxy (`focusProxy *widget.Entry`) overlaid on the canvas.
- Captures keyboard key chords, text typing, clipboard shortcuts, and IME (Input Method Editor) composition events natively without requiring custom platform-specific IME implementations.

## Window Resize Debouncing (`windowResizeWatcher`)

- Window resizing triggers `windowResizeWatcher` in `browser.go`.
- Resize events are debounced with a 100ms `time.AfterFunc` timer before triggering viewport recalculations and document re-layouts, preventing layout thrashing during continuous window dragging.

## Developer Tools Architecture (`internal/ui/devtools/`)

The DevTools subsystem provides 11 dedicated inspection panels:
1. `accessibility`: Inspects accessible node trees, ARIA attributes, and roles.
2. `displaylist`: Visualizes hierarchical display list drawing commands.
3. `memory`: Displays live memory gauges, component quotas, and eviction telemetry.
4. `network`: Renders waterfall HTTP request timelines, status codes, headers, and payloads.
5. `performance`: Tracks frame presentation FPS, layout timing, and event loop metrics.
6. `scriptqueue`: Inspects active JavaScript macrotasks, microtasks, and timer queues.
7. `security`: Audits TLS certificates, cipher suites, mixed-content warnings, and CSP violations.
8. `settings`: Configures browser flags, user-agent strings, and developer options.
9. `sources`: Provides an embedded source code viewer and script inspector.
10. `storage`: Manages cookies, localStorage entries, and profile caches.
11. `tilecache`: Visualizes raster tile boundaries and cached tile hit ratios.

## Context Menus & Formatting

- `dev_tools_context_menu.go` provides right-click inspect actions using package-level `attrReplacer strings.Replacer` to eliminate per-invocation string allocation overhead.

## Testing & Verification

All UI tests reside in `test/internal/ui/...` and `test/internal/ui/devtools/...`.

Run the UI test suites:
```bash
go test ./test/internal/ui/...
go test ./test/internal/ui/devtools/...
```

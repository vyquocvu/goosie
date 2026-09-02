# Stream E: GUI Layer Optimization

## Status
**IMPLEMENTED**

## Goal
Reduce UI thread blocking and unnecessary widget rebuilds in the Fyne GUI layer.

## Scope
- `internal/ui/browser.go` — main browser UI
- `internal/ui/dev_tools_context_menu.go` — escapeAttr
- `internal/ui/` — devtools panels and views (`console.go`, `style_view.go`, `inspect_panel.go`)

## Constraints
- Zero behavior change (same UI, same functionality)
- All existing tests pass
- No new dependencies

## Optimizations

### E1: escapeAttr package-level replacer
**Problem:** `escapeAttr` in `dev_tools_context_menu.go` creates a new `strings.Replacer` on every call. This allocates a replacer struct per call.

**Solution:** Move the replacer to a package-level variable. `strings.Replacer` is safe for concurrent use.

**Implementation:** 
```go
var attrReplacer = strings.NewReplacer(
    `&`, "&amp;",
    `"` , "&quot;",
    `<`, "&lt;",
    `>`, "&gt;",
)

func escapeAttr(s string) string {
    return attrReplacer.Replace(s)
}
```

### E2: Window resize debounce
**Problem:** Window resize triggers immediate full re-render. During continuous resize (user dragging the window edge), this causes multiple expensive re-renders per second.

**Solution:** Debounce resize events with a 100ms timer. Only trigger re-render after resize events stop for 100ms.

**Implementation:** Add a `resizeTimer` field to the browser UI struct. On resize event, cancel the existing timer and start a new one. When the timer fires, trigger the re-render.

**Trade-off:** Adds 100ms latency to resize, but this is imperceptible and eliminates jank.

### E3: ConsolePanel filter optimization
**Problem:** ConsolePanel's filter logic re-filters the entire console message list on every keystroke. With hundreds of messages, this is O(n) per keystroke.

**Solution:** Pre-compute a filtered list and update it incrementally. Or, debounce the filter input with a 200ms timer.

**Implementation:** Add a debounce timer to the filter input. Only re-filter after the user stops typing for 200ms.

**Alternative:** Use a separate goroutine for filtering, but this adds complexity.

## Out of Scope
- Virtualizing inspector panels (larger refactor)
- Splitting browser.go monolith (architectural, separate effort)
- Raster canvas scroll callback (requires deeper integration)

## Testing
- Existing UI tests
- Manual verification: resize smoothness, console filter responsiveness
- Pixel hash unchanged (GUI layer doesn't affect headless rendering)

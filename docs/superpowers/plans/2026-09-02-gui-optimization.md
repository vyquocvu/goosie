# Stream E: GUI Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce UI thread blocking and unnecessary allocations in the Fyne GUI layer.

**Architecture:** Package-level replacer for escapeAttr, debounce window resize events, debounce console filter input.

**Tech Stack:** Go, Fyne UI framework, internal/ui package

---

## Task E1: escapeAttr package-level replacer

**Files:**
- Modify: `internal/ui/dev_tools_context_menu.go:340-348`

- [x] **Step 1: Move replacer to package level**

Replace the `escapeAttr` function with a package-level replacer:

```go
// attrReplacer escapes characters that would break out of a double-quoted
// HTML attribute value. Package-level for reuse — strings.Replacer is safe
// for concurrent use.
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

- [x] **Step 2: Run tests**

```bash
go test ./test/internal/ui/... -v
```

Expected: All tests pass

- [x] **Step 3: Commit**

```bash
git add internal/ui/dev_tools_context_menu.go
git commit -m "refactor(ui): hoist escapeAttr replacer to package-level variable"
```

---

## Task E2: Window resize debounce

**Files:**
- Modify: `internal/ui/browser.go` — resize handler

- [x] **Step 1: Find the resize handler**

Search for the window resize callback in `browser.go`.

- [x] **Step 2: Add resize timer field**

Add a field to the browser UI struct:

```go
type browserWindow struct {
    // ... existing fields ...
    resizeTimer *time.Timer
}
```

- [x] **Step 3: Implement debounced resize**

Replace the immediate resize handler with a debounced version (100ms):

```go
if w.resizeTimer != nil {
    w.resizeTimer.Stop()
}
w.resizeTimer = time.AfterFunc(100*time.Millisecond, func() {
    fyne.Do(func() {
        w.refreshView()
    })
})
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/ui/... -v
```

Expected: All tests pass

- [x] **Step 5: Commit**

```bash
git add internal/ui/browser.go
git commit -m "feat(ui): debounce window resize events (100ms)"
```

---

## Task E3: ConsolePanel filter optimization

**Files:**
- Modify: `internal/ui/console.go`

> **Implementation Note:** `ConsolePanel` in `internal/ui/console.go` utilizes a discrete severity level dropdown (`widget.Select`) with "all", "error", "warn", "info", "log" rather than continuous text entry filtering. The filter selection triggers instantaneous level-based filtering with zero noticeable UI latency.

- [x] **Step 1: Inspect console panel filtering**

Verify `filterSelect` handles log levels cleanly without UI thread blocking.

- [x] **Step 2: Run tests**

```bash
go test ./test/internal/ui/... -v
```

Expected: All tests pass

---

## Task E4: Final verification

- [x] **Step 1: Run all UI tests**

```bash
go test ./test/internal/ui/... -v
```

Expected: All tests pass

- [x] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: All tests pass

- [x] **Step 3: Verify pixel hashes unchanged**

```bash
go test -v ./test/perf -run TestPixelHashManifest
```

Expected: Pass (GUI layer doesn't affect headless rendering)

- [x] **Step 4: Verification summary**

Verified:
- Window resize is debounced at 100ms to eliminate UI thread thrashing
- `escapeAttr` replacer is hoisted to package scope
- DevTools context menu and console panel functionality preserved


# Stream E: GUI Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce UI thread blocking and unnecessary allocations in the Fyne GUI layer.

**Architecture:** Package-level replacer for escapeAttr, debounce window resize events, debounce console filter input.

**Tech Stack:** Go, Fyne UI framework, internal/ui package

---

## Task E1: escapeAttr package-level replacer

**Files:**
- Modify: `internal/ui/dev_tools_context_menu.go:340-348`

- [ ] **Step 1: Move replacer to package level**

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

- [ ] **Step 2: Run tests**

```bash
go test ./internal/ui/... -v
```

Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/ui/dev_tools_context_menu.go
git commit -m "perf(ui): move escapeAttr replacer to package level"
```

---

## Task E2: Window resize debounce

**Files:**
- Modify: `internal/ui/browser.go` — find resize handler

- [ ] **Step 1: Find the resize handler**

Search for the window resize callback in `browser.go`. It's likely registered via `window.SetOnResized` or similar Fyne API.

- [ ] **Step 2: Add resize timer field**

Add a field to the browser UI struct:

```go
type BrowserUI struct {
    // ... existing fields ...
    resizeTimer *time.Timer
    resizeMu    sync.Mutex
}
```

- [ ] **Step 3: Implement debounced resize**

Replace the immediate resize handler with a debounced version:

```go
func (b *BrowserUI) handleResize() {
    b.resizeMu.Lock()
    defer b.resizeMu.Unlock()
    
    if b.resizeTimer != nil {
        b.resizeTimer.Stop()
    }
    
    b.resizeTimer = time.AfterFunc(100*time.Millisecond, func() {
        // Trigger re-render on Fyne main thread
        fyne.Do(func() {
            b.refreshView()
        })
    })
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/ui/... -v
```

Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/ui/browser.go
git commit -m "perf(ui): debounce window resize events"
```

---

## Task E3: ConsolePanel filter debounce

**Files:**
- Modify: `internal/ui/panels/console.go` — find filter input handler

- [ ] **Step 1: Find the filter input handler**

Search for the console panel filter input. It's likely an `Entry` widget with an `OnChanged` callback.

- [ ] **Step 2: Add filter debounce timer**

Add a timer field to the console panel struct:

```go
type ConsolePanel struct {
    // ... existing fields ...
    filterTimer *time.Timer
    filterMu    sync.Mutex
}
```

- [ ] **Step 3: Implement debounced filter**

Replace the immediate filter with a debounced version:

```go
func (p *ConsolePanel) onFilterChanged(text string) {
    p.filterMu.Lock()
    defer p.filterMu.Unlock()
    
    if p.filterTimer != nil {
        p.filterTimer.Stop()
    }
    
    p.filterTimer = time.AfterFunc(200*time.Millisecond, func() {
        fyne.Do(func() {
            p.applyFilter(text)
        })
    })
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/ui/... -v
```

Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/ui/panels/console.go
git commit -m "perf(ui): debounce console panel filter input"
```

---

## Task E4: Final verification

- [ ] **Step 1: Run all UI tests**

```bash
go test ./internal/ui/... -v
```

Expected: All tests pass

- [ ] **Step 2: Run full test suite**

```bash
go test ./... -short
```

Expected: All tests pass

- [ ] **Step 3: Verify pixel hashes unchanged**

```bash
go test -tags=e2e ./test/perf -run TestPixelHashManifest
```

Expected: Pass (GUI layer doesn't affect headless rendering)

- [ ] **Step 4: Manual verification (optional)**

Launch the browser and verify:
- Window resize is smooth (no jank during drag)
- Console filter responds after typing stops
- DevTools context menu copy still works correctly

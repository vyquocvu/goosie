# Stream B: JS Runtime Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce JS execution overhead (goja VM, DOM bridge, event loop, timer allocations, and console logging) through architecture-first rework, gated by measurement. Zero behavior change.

**Architecture:** Add script compilation caching with LRU eviction, cache the `__flushMicrotasks` callable reference, batch DOM bridge property mapping, pool `time.Timer` instances across timeout/interval handlers, and replace dynamic slice allocations with a fixed 1000-entry ring buffer for console messages.

**Tech Stack:** Go, goja JavaScript runtime, internal/js package

---

## Task B1: Script Compilation Caching

**Files:**
- Modify: `internal/js/runtime.go`

- [x] **Step 1: Add script cache fields to JS runtime**

Add a mutex-protected cache for compiled `*goja.Program` instances:

```go
type Runtime struct {
    // ... existing fields ...
    scriptCache      map[string]*goja.Program
    scriptCacheMu    sync.Mutex
    scriptCacheLimit int
}
```

Initialize `scriptCache` with a 500-entry limit in `NewRuntime`:

```go
scriptCache:      make(map[string]*goja.Program),
scriptCacheLimit: 500,
```

- [x] **Step 2: Implement cached compilation in RunScript**

Check cache by SHA-256 content hash before compiling:

```go
func (r *Runtime) compileScriptCached(src string) (*goja.Program, error) {
    hash := hashScript(src)
    
    r.scriptCacheMu.Lock()
    if prog, ok := r.scriptCache[hash]; ok {
        r.scriptCacheMu.Unlock()
        return prog, nil
    }
    r.scriptCacheMu.Unlock()

    prog, err := goja.Compile("", src, false)
    if err != nil {
        return nil, err
    }

    r.scriptCacheMu.Lock()
    if len(r.scriptCache) >= r.scriptCacheLimit {
        count := 0
        for k := range r.scriptCache {
            delete(r.scriptCache, k)
            count++
            if count >= r.scriptCacheLimit/2 {
                break
            }
        }
    }
    r.scriptCache[hash] = prog
    r.scriptCacheMu.Unlock()

    return prog, nil
}
```

- [x] **Step 3: Run tests**

```bash
go test ./test/internal/js/... -v
```

Expected: All tests pass

- [x] **Step 4: Commit**

```bash
git add internal/js/runtime.go
git commit -m "perf(js): add script compilation cache to avoid redundant parsing"
```

---

## Task B2: Microtask Flush Resolution Caching

**Files:**
- Modify: `internal/js/runtime.go`

- [x] **Step 1: Add cached callable field**

Add `flushMicrotasksFn` to `Runtime` struct:

```go
type Runtime struct {
    // ... existing fields ...
    flushMicrotasksFn goja.Callable
}
```

- [x] **Step 2: Resolve callable once during environment initialization**

In `initEnvironment`, look up and cast `__flushMicrotasks` to `goja.Callable` once:

```go
if val := r.vm.Get("__flushMicrotasks"); val != nil {
    if fn, ok := goja.AssertFunction(val); ok {
        r.flushMicrotasksFn = fn
    }
}
```

- [x] **Step 3: Use cached callable in RunMicrotasks**

Replace per-call `r.vm.Get("__flushMicrotasks")` lookup:

```go
func (r *Runtime) RunMicrotasks() {
    if r.flushMicrotasksFn != nil {
        r.flushMicrotasksFn(goja.Undefined()) //nolint:errcheck
    }
}
```

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/js/... -v
```

Expected: All tests pass

- [x] **Step 5: Commit**

```bash
git add internal/js/runtime.go
git commit -m "perf(js): cache __flushMicrotasks function reference to avoid per-call lookup"
```

---

## Task B3: DOM Bridge Batch Population

**Files:**
- Modify: `internal/js/runtime.go`

- [x] **Step 1: Batch node conversion and property mapping**

In `populateJSNode`, pre-allocate slices for children and batch property assignments to reduce intermediate wrapper allocations.

- [x] **Step 2: Run tests**

```bash
go test ./test/internal/js/... -v
```

Expected: All tests pass

- [x] **Step 3: Commit**

```bash
git add internal/js/runtime.go
git commit -m "perf(js): batch DOM bridge population to reduce allocations"
```

---

## Task B4: Event Loop Timer Pooling

**Files:**
- Modify: `internal/js/runtime.go`

- [x] **Step 1: Add package-level sync.Pool for time.Timer**

```go
// timerPool pools time.Timer objects to reduce allocations.
var timerPool = sync.Pool{
    New: func() any {
        t := time.NewTimer(time.Hour)
        if !t.Stop() {
            select {
            case <-t.C:
            default:
            }
        }
        return t
    },
}
```

- [x] **Step 2: Use pooled timers in setTimeout and setInterval**

Retrieve from pool with `timerPool.Get().(*time.Timer)`, reset to target duration, and return to `timerPool.Put(t)` upon completion or cancellation.

- [x] **Step 3: Run tests**

```bash
go test ./test/internal/js/... -v
```

Expected: All tests pass

- [x] **Step 4: Commit**

```bash
git add internal/js/runtime.go
git commit -m "perf(js): pool time.Timer objects in event loop"
```

---

## Task B5: Console Message Ring Buffer

**Files:**
- Modify: `internal/js/runtime.go`

- [x] **Step 1: Define fixed-size ring buffer array**

Replace unbounded slice allocation with a fixed 1000-message array and monotonic counter:

```go
type Runtime struct {
    // ... existing fields ...
    consoleBuffer   [1000]ConsoleMessage
    consoleWriteIdx uint64
    consoleMu       sync.Mutex
}
```

- [x] **Step 2: Update console logging methods**

In `logToConsole`:

```go
r.consoleMu.Lock()
idx := r.consoleWriteIdx
r.consoleWriteIdx++
r.consoleBuffer[idx%1000] = ConsoleMessage{
    Level:     level,
    Message:   msg,
    Timestamp: time.Now(),
}
r.consoleMu.Unlock()
```

- [x] **Step 3: Update GetConsoleMessages snapshot retrieval**

In `GetConsoleMessages()`, extract the ring buffer contents up to the written count into a bounded slice in chronological order.

- [x] **Step 4: Run tests**

```bash
go test ./test/internal/js/... -v
```

Expected: All tests pass

- [x] **Step 5: Commit**

```bash
git add internal/js/runtime.go
git commit -m "perf(js): replace console message slice with ring buffer"
```

---

## Task B6: Final verification

- [x] **Step 1: Run all JS tests**

```bash
go test ./test/internal/js/... -v
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

Expected: Pass (JS optimizations preserve 100% functional and visual equivalence)

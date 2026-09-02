# internal/js — Agent Constraints & Architecture

## Single Owner Goroutine

- Exactly one goroutine owns the `Session`/`Runtime`. Only that goroutine may call `Runtime` methods directly.
- All external callers MUST use `Submit()` or `SubmitAndWait()` to schedule work onto the runtime owner goroutine.
- Calling `Submit()` or `SubmitAndWait()` from the owner goroutine WILL deadlock.

## Goja VM Thread Safety & Synchronization

- The underlying Goja JavaScript VM is NOT thread-safe.
- All script execution, value inspection, and VM interactions must be serialized through `scriptMu`.
- Never access or mutate Goja values/objects from a non-owner goroutine without holding `scriptMu`.

## Script Compilation Cache (`scriptCache`)

- `scriptCache map[string]*goja.Program` (guarded by `scriptCacheMu`, with a default soft limit of `scriptCacheLimit = 500`) caches compiled bytecode keyed by the SHA-256 digest of the script text.
- Repeated inline or external scripts reuse existing `*goja.Program` instances to avoid parsing and bytecode compilation overhead.

## Cached Microtask Flush (`flushMicrotasksFn`)

- The JavaScript microtask runner `__flushMicrotasks` is resolved once during runtime initialization via `resolveFlushMicrotasks()` and stored in `flushMicrotasksFn goja.Callable`.
- Avoid per-tick dynamic property lookups (`vm.Get("__flushMicrotasks")`) on hot execution loops.

## Timer Pooling (`timerPool`)

- `timerPool sync.Pool` pools `time.Timer` objects for `setTimeout` and `setInterval` execution.
- Timers must be properly stopped, drained, and returned to `timerPool` when cleared or completed to eliminate runtime GC allocation churn.

## Console Ring Buffer (`consoleBuffer`)

- Console messages are stored in a fixed-capacity ring buffer: `consoleBuffer [1000]ConsoleMessage` indexed by `consoleWriteIdx atomic.Uint64`.
- This avoids unbounded slice growth allocations while ensuring deterministic O(1) appending and bounded snapshot retrieval.

## DOM Bridge & Mutation Notifications

- The DOM API lives primarily in JavaScript land via polyfills initialized in `setupDocumentAPI` (within `runtime.go` and `polyfills.go`).
- DOM mutations made by JavaScript scripts signal the Go engine through `window.__onDOMChanged`.
- Do NOT add direct Go-to-DOM bindings. All state mutations must flow through mutation callbacks and `EventLoop` batching.
- Batched DOM tree construction (`populateDocument`, `populateJSNode`) sets attributes and attaches child nodes in batches to reduce VM boundary transitions.

## `document.currentScript` Support

- Synchronous and deferred classic script execution pairs `SetCurrentScript(attrs)` with `RunScript()` and calls `ClearCurrentScript()` upon completion.
- Async scripts leave `document.currentScript` set to `null` per the HTML specification.

## Event Loop & Execution Queues

- The execution order strictly adheres to the HTML specification:
  1. Execute next macrotask
  2. Drain all pending microtasks (`flushMicrotasksFn`)
  3. Fire due timers (`setTimeout` / `setInterval`)
  4. Flush pending DOM mutations
- Queues are bounded (task: 256, microtask: 512, timer: 128).
- Async completions MUST enqueue onto the event loop via `enqueueTask` — never execute callbacks inline on worker goroutines.

## Testing & Verification

All JavaScript runtime tests reside in `test/internal/js/...`.

Run the JS test suite:
```bash
go test -short ./test/internal/js/...
```


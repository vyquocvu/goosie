# Stream B: JS Runtime Optimization

## Status
**APPROVED**

## Goal
Reduce JS execution overhead (goja VM, DOM bridge, event loop, frame scheduler) through architecture-first rework, gated by measurement. Zero behavior change.

## Success Criteria
- Measurable improvement in JS-related benchmarks (benchstat).
- Zero failures in `go test ./...`.
- Pixel-hash manifest unchanged.

## Scope
**In scope:** `internal/js/runtime.go`, `eventloop.go`, `framescheduler.go`, `session.go`, `polyfills.go`
**Out of scope:** Streams C, D, E

## Phases

### Phase 0 — Measurement
- Add JS stage timings to perf-review
- Micro-benchmarks for RunScript, populateDocument, EventLoop
- Baseline capture

### Phase 1 — Architecture-first rework
1. **Script compilation cache**: Cache `goja.Compile()` by content hash, bounded LRU (500 entries)
2. **Cached microtask flush**: Resolve `__flushMicrotasks` once at init, eliminate per-call `vm.Get`
3. **DOM bridge batch population**: Pre-allocate arrays, batch `vm.ToValue` in `populateJSNode`
4. **Event loop timer pooling**: `sync.Pool` for `time.Timer` in SetTimeout/SetInterval
5. **Console message ring buffer**: Fixed-size (1000) ring buffer, atomic index

### Phase 2 — Evidence-driven micro-tuning
- Profile-guided fixes only

### Phase 3 — Verification
- benchstat, full test suite, pixel-hash manifest

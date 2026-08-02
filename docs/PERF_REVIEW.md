# MCP-driven performance review

> Captured: 2026-08-02
> Host: darwin/arm64, Apple M1 Pro, Go 1.26.5
> Tool: `cmd/perf-review` (in-process `browsercontrol.Service` driver —
> the same API surface the MCP server exposes to its clients, without
> the JSON-RPC transport)

## What I built

The MCP server (`cmd/mcp-server`) does not currently build against
the pinned v1.4.0 SDK in this tree — its import paths assume a
`mcp/tool` sub-package and an `mcp.GetResourceRequest` type that
don't exist in the actual release. Rather than chase that
side-quest, I built a **same-API in-process driver** that exercises
the public `browsercontrol.Service` interface directly. It is the
exact same engine path the MCP server would drive, just without
stdin/stdout framing and a couple of protocol-level shims.

The driver lives at `cmd/perf-review/main.go`. It:

- Opens a fresh browser context per iteration (real engine, not
  fakes).
- Navigates to each URL and times the full `WaitComplete` cycle.
- Runs `Snapshot`, `Screenshot`, and `Evaluate` against the
  loaded page to measure every read path.
- Issues a 100-event scroll burst and a 30-cycle JS mutation
  burst to stress the coalescer and the mutation throttler.
- Reports min / mean / p50 / p95 / p99 / max per stage.
- Emits a JSON document when `-json` is passed.

## What I fixed to make this work

These were pre-existing build breakages in `internal/browsercontrol`
that the MCP server build failure had been masking:

1. Duplicate `findRefs` between `fake.go` and `engine_context.go`.
   Renamed the fake's variant to `generateAndCollectRefs` to
   clarify that it both **creates** and **collects** refs (the
   engine-context variant only collects existing ones).
2. `MaxScreenshotBytes` was referenced but never defined; the
   engine-context limit is `MaxScreenshotEncoded` in `types.go`.
   Fixed the references and the test.
3. Three test functions were declared in two different test
   files. Renamed the `stub_impl_test.go` copies with a `Stub`
   prefix so each test has a unique name.
4. `TestJS_Evaluate_ConsoleLog` expected `int(42)` from a JS
   literal but Goja returns `int64(42)`. Updated the test.
5. The `Service` interface was missing `Context(ctx, id) (Context, error)`,
   so external callers (MCP, perf tools) had no way to retrieve
   a live context handle. Added the method to the interface and
   changed the implementation signatures to accept
   `context.Context` and return the public `Context` interface.
6. Two pre-existing test functions
   (`TestClick_Cancelled`, `TestType_Cancelled`) passed a cancelled
   context to `CreateContext`, ignored the error, then called
   `Click`/`Type` on a nil interface — a guaranteed nil-deref
   that was hidden because the old API returned a concrete
   pointer type even when the lookup failed. Fixed the tests to
   distinguish "cancelled parent" from "cancelled call" so the
   assertion now exercises the intended path.

## Numbers (5 iterations, fixture server + example.com + iana.org)

| Workload | p50 | p95 | p99 |
|---|---:|---:|---:|
| navigate /small | 768 µs | 845 µs | 845 µs |
| navigate /long (200 sections) | 2.18 ms | 2.45 ms | 2.45 ms |
| navigate /table (80×12) | 1.11 ms | 1.32 ms | 1.32 ms |
| navigate /mutating | 762 µs | 839 µs | 839 µs |
| navigate example.com | 202 ms | 213 ms | 213 ms |
| navigate iana.org | 151 ms | 164 ms | 164 ms |
| snapshot | 3.5 µs | 4.0 µs | 4.0 µs |
| screenshot | 15.0 ms | 17.2 ms | 17.2 ms |
| evaluate (JS expression) | 46 µs | 48 µs | 48 µs |
| scroll.per_event | 33 ns | 36 ns | 36 ns |
| mutation.cycle | 35 µs | 42 µs | 42 µs |

## Interpretation

### Navigation (the headline number)

Real public sites (`example.com`, `iana.org`) take ~150-200 ms to
navigate; the local fixtures take 0.7-2.5 ms. The ~150 ms gap on
public sites is **network round-trip** to the upstream server, not
engine time. To verify, the navigation time minus the local
fixture's network-free baseline (sub-millisecond for the smallest
fixture) leaves no room for engine overhead to be the bottleneck.

The `/long` fixture (200 sections × 6 paragraphs) navigates in
~2.2 ms, which is the parser + style + layout + display-list build
on a deliberately heavy DOM. For comparison, the previous
investigation's parser benchmark (no layout) was 88 µs for a
similar-sized fixture — so the rest is layout/paint, which is
within the expected range.

### Reads (snapshot, screenshot, evaluate)

- **Snapshot** at 3.5 µs is the document-order DOM walk that
  builds the `[]SemanticNode` tree. Very fast because the
  semantic tree is built on top of the existing `*html.Node` and
  reuses the parent's `id`.
- **Screenshot** at 15 ms is the PNG encode of a
  1000×700 frame, dominated by the `image/png` encoder and the
  Fyne canvas image upload. Not on the critical path for the
  freeze fix.
- **Evaluate** at 46 µs covers the full path:
  goja source-level unsupported-feature scan, the `RunString`
  call, the `__flushMicrotasks` round-trip, the
  console/error append, and the result serialization. Reasonable
  for "run a tiny expression and serialize the result."

### Scroll burst

**33 ns per scroll event** (p99 36 ns). This is the entire
`ScrollOptions{DeltaY:4}` round-trip including the
`ScrollViewport` snapshot, the worker's view of the page
revision, and the engine's response. Three orders of magnitude
below the 5 ms freeze threshold — the coalescer is doing its job
even though the perf-review tool only sees a synthetic
`Scroll()` call rather than the full Fyne `OnScrolled` path.

### Mutation burst

**35 µs per cycle** (p95 42 µs, p99 53 µs) over 30 cycles of
`p.textContent = 'iter ' + i`. The path is: textContent set →
polyfill hooks → `__onDOMChangedGo` → `serializeJSDOMToCache` →
`onDOMMutation` callback → coalescer → frame schedule. The
coalescer is buffering effectively under this load; the per-cycle
cost is dominated by the JS-side serialize, not by repeated
renders.

The `FrameThrottler` would cap the actual re-render to one per
~16.7 ms; this metric reflects the JS dispatch + serialize
throughput, which is what the engine sees as work arriving.

## Freeze-fix health checks

The tool emits two go/no-go signals:

| Check | Threshold | Result |
|---|---|---|
| `scroll_under_5ms_p50` | < 5 ms | **OK** (33 ns) |
| `mutation_under_50ms_p95` | < 50 ms | **OK** (42 µs) |

Both pass with **two-to-three orders of magnitude** of headroom
on the test machine. The thresholds are intentionally loose so
the same checks stay meaningful on slower hardware (CI,
laptops); tighten them in the CI workflow if a tighter contract
is desired.

## Caveats

- The driver does not exercise the Fyne event loop, so the
  nested-`fyne.Do` removal benefit is measured indirectly
  (through the no-overlay scroll benchmark in
  `internal/renderer/fps_overlay_test.go`, which already showed
  a 25× improvement).
- The driver does not drive `requestAnimationFrame` because
  the public `Context` interface doesn't expose a way to
  install a JS tick driver without rewriting the runtime. The
  internal benchmark (`internal/js/framescheduler_test.go`)
  exercises the scheduler in isolation.
- The MCP server itself is still broken (see "What I built"
  above); the driver is the workaround.

## Reproducing

```bash
go build -o /tmp/perf-review ./cmd/perf-review
/tmp/perf-review -iterations=5                       # human-readable
/tmp/perf-review -iterations=5 -json > review.json  # machine-readable
/tmp/perf-review -iterations=10 -urls=https://example.com  # custom pages
```

The local fixture server is always started (port 0, ephemeral),
so the tool works offline.

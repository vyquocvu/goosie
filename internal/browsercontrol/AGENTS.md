# internal/browsercontrol — Agent Constraints & Architecture

## Core Responsibilities

The `internal/browsercontrol` package provides the protocol-agnostic headless browser automation, orchestration, and page interaction engine for Goosie, serving automated test suites, CLI runners, and Model Context Protocol (MCP) server endpoints.

## Core Interfaces (`interfaces.go`)

- `Service`: Top-level manager for browser context lifecycles (`CreateContext`, `Context`, `ListContexts`, `CloseContext`).
- `Context`: Represents an individual tab or page session, exposing:
  - `Navigate(ctx, url, waitUntil, timeoutMs)`
  - `Wait(ctx, opts)`
  - `Snapshot(ctx, opts)`
  - `Screenshot(ctx, opts)`
  - `Query(ctx, locator)`
  - `Click(ctx, ref, opts)`
  - `Type(ctx, ref, text, opts)`
  - `PressKey(ctx, key, modifiers)`
  - `Scroll(ctx, opts)`
  - `SetViewport(ctx, vp)`
  - `Evaluate(ctx, source, opts)`
  - `Console(ctx, cursor, limit)`
  - `Network(ctx, cursor, limit)`
  - `Security(ctx)`

## Navigation Synchronization & Wait Conditions

- Navigation calls accept explicit `WaitCondition` criteria:
  - `WaitCommit`: Waits for the initial response headers and first byte commit.
  - `WaitInteractive`: Waits until DOM is parsed and interactive scripts begin.
  - `WaitComplete`: Waits until document parsing and resource loading are complete.
- Element polling (`WaitForSelector`, `WaitForCondition`) checks conditions with exponential backoff up to context deadline.

## Semantic Snapshots & Opaque References

- `Snapshot()` produces a bounded `PageSnapshot` containing accessible roles, names, and opaque `ElementRef` handles (e.g. `ref_1`, `ref_2`).
- Actions (`Click`, `Type`) resolve opaque refs back to live DOM nodes in `EngineContext`, avoiding brittle CSS selector re-evaluation across DOM mutations.

## Headless Rendering & Screenshots

- `Screenshot()` drives the renderer headlessly off-screen to produce clean PNG image bytes without requiring a physical window display.
- Supports full-page raster capture as well as clipped element-bounding-box captures.

## Error Isolation & Concurrency

- Contexts are strictly isolated. A crash or script infinite loop in one `Context` is bounded by context cancellation and will not affect sibling contexts.
- All automation methods accept `context.Context` for deadline and cancellation propagation.

## Testing & Verification

All browsercontrol tests reside in `test/internal/browsercontrol/...`.

Run the test suite with the race detector:
```bash
go test -race -short ./test/internal/browsercontrol/...
```

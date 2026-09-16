# Project: Goosie Browser — Frame Path

## Architecture

Goosie is a Go browser engine built around a fixed 256px tile frame path. The frame path draws synthetic documents through a CPU tile rasterizer with a worker pool, a damage-blit composer, and a vsync-paced UI loop. There is no DOM, no CSS cascade, no style or layout pass.

## Code Layout

- `cmd/goosie/` — the binary: flags, pipeline assembly, run report
- `internal/frame/` — geometry, bitmaps, tile grid, plans, frame recording
- `internal/paint/` — display lists and synthetic scene generator
- `internal/surface/` — UI-thread loop and composer
- `internal/raster/` — glyph atlas, tile rasterizer, worker pool, per-vsync scheduler
- `internal/platform/` — backend selection; `headless/` and `darwin/` are backends
- `internal/archtest/` — import boundary and cgo enforcement
- `test/` — one package per unit under test, plus `test/gate`

## Import Rules

Imports point down: `frame` <- `paint` <- `surface` <- `raster` <- {`platform/*`, `cmd/*`}. `surface` depends on no platform code. cgo appears in exactly one directory: `internal/platform/darwin`. Enforced by `go test ./internal/archtest/`.

## Interface Contracts

### `surface.Window`
Platform backends implement `surface.Window`. The surface package knows nothing about platforms; a backend satisfies the interface.

### `raster.Scheduler`
The scheduler reports the content version its tiles were painted at. A version bump retries a tile that failed under the old one.

### `frame.Recorder`
Records frame plans and reports counters (style passes, layout passes, allocs) that the gate suite asserts on.

## Testing

```bash
go test ./...                  # everything
go test -race ./...            # race detector
go test ./test/gate -run TestGate -v   # gate suite
```

All tests pass on a machine with no display.

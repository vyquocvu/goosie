# Goosie

Goosie is a modern, lightweight browser written in Go — a Go browser engine built around a fixed 256px tile frame path. Vision: a gọn nhẹ, native-first browser; current status: experimental static-document engine (see `docs/roadmap-v2.md`). It implements its own web front end — a real DOM, CSS cascade, style resolution, and layout (see PROJECT.md) — and renders through a CPU tile rasterizer with a worker pool, a damage-blit composer, and a vsync-paced UI loop. The engine pipeline is Parse → Style → Layout → Paint, wired in `internal/engine/engine.go`.

## Quick start

Requires Go 1.25 or newer. On macOS, `CGO_ENABLED=1` (the default) for a real window.

```bash
git clone https://github.com/vyquocvu/goosie.git
cd goosie

# a window on this machine, scrollable with a trackpad. Ctrl-C ends it.
go run ./cmd/goosie

# no window: works over ssh, in CI, on Linux
go run ./cmd/goosie -backend headless

# a different document, viewport, or ratio
go run ./cmd/goosie -scene plain -width 1024 -height 640 -dpr 1
```

Build a binary with:

```bash
make build
./bin/goosie
```

## Scenes

`-scene checkerboard` and `-scene plain` are the same document — a page of bordered cells, one image, and 1,200 positioned glyph runs — except that `plain` gives every cell one colour. The alternation is what makes a damaged tile differ from its neighbour, so `checkerboard` is what a blit-correctness run should use and `plain` is what isolates the text layout below it.

## Measuring

```bash
# throughput: as many frames as the clock allows, headless
go run ./cmd/goosie -bench -frames 2000

# pacing: 600 scripted scroll frames at real vsync
go run ./cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json

# scroll budgets
go test -run='^$' -bench='BenchmarkWarmScroll|BenchmarkColdScrollBurst' \
  -benchmem -benchtime=200x -count=3 ./test/gate
```

## Testing

```bash
go test ./...                  # everything
go test -race ./...            # race detector
go test ./test/gate -run TestGate -v   # gate suite
```

All tests pass on a machine with no display.

## Architecture

```
cmd/goosie            the binary: flags, pipeline assembly, run report
internal/dom          HTML parser, DOM tree, events
internal/css          tokenizer, stylesheet parser
internal/style        cascade, computed styles
internal/layout       box arena: block, inline, float, flex, grid, table, positioning
internal/engine       Session pipeline: Parse → Style → Layout → Paint (+JS, fonts, images)
internal/js           per-document goja runtime bound to the DOM
internal/frame        geometry, bitmaps, tile grid, plans, frame recording
internal/paint        display lists and synthetic scene generator
internal/surface      UI-thread loop and composer
internal/raster       glyph atlas, tile rasterizer, worker pool, per-vsync scheduler
internal/platform     backend selection; headless/ and darwin/ are the backends
internal/archtest     import and cgo boundary enforcement
test/                 one package per unit under test, plus test/gate
```

Imports point down: `frame` <- `paint` <- `surface` <- `raster` <- {`platform/*`, `cmd/*`}. Enforced, not documented: `go test ./internal/archtest/`.

## License

[MIT](LICENSE)

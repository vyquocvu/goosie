<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->


## Architecture

Goosie is a Go browser engine built around a fixed 256px tile frame path. There is no DOM, no CSS cascade, and no style or layout pass. The frame path draws synthetic documents through a CPU tile rasterizer with a worker pool, a damage-blit composer, and a vsync-paced UI loop.

### Code Layout

- `cmd/goosie/` — the binary: flags, pipeline assembly, run report
- `internal/frame/` — geometry, bitmaps, tile grid, plans, frame recording
- `internal/paint/` — display lists and synthetic scene generator
- `internal/surface/` — UI-thread loop and composer
- `internal/raster/` — glyph atlas, tile rasterizer, worker pool, per-vsync scheduler
- `internal/platform/` — backend selection; `headless/` and `darwin/` are the backends
- `internal/archtest/` — import boundary and cgo enforcement
- `test/` — one package per unit under test, plus `test/gate`

### Import Rules

Imports point down: `frame` <- `paint` <- `surface` <- `raster` <- {`platform/*`, `cmd/*`}. `surface` depends on no platform code. cgo appears in exactly one directory: `internal/platform/darwin`. All enforced by `go test ./internal/archtest/`.

## Testing

```bash
go test ./...                  # everything
go test -race ./...            # race detector
go test ./test/gate -run TestGate -v   # gate suite
```

All tests must pass on a machine with no display.

## Visual Verification

Pure backend changes with no user-visible output require the automated tests above. When UI features are added, visual verification through headless comparison is required before the work can be considered complete.

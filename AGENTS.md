<!-- CODEGRAPH_START -->
## CodeGraph

In repositories indexed by CodeGraph (a `.codegraph/` directory exists at the repo root), reach for it BEFORE grep/find or reading files when you need to understand or locate code:

- **MCP tool** (when available): `codegraph_explore` answers most code questions in one call — the relevant symbols' verbatim source plus the call paths between them, including dynamic-dispatch hops grep can't follow. Name a file or symbol in the query to read its current line-numbered source. If it's listed but deferred, load it by name via tool search.
- **Shell** (always works): `codegraph explore "<symbol names or question>"` prints the same output.

If there is no `.codegraph/` directory, skip CodeGraph entirely — indexing is the user's decision.
<!-- CODEGRAPH_END -->


## Architecture

Goosie is a browser engine written in Go. It implements its own web front end — a real DOM, a CSS cascade, style resolution, and layout — and renders through a fixed 256px tile frame path: a CPU tile rasterizer with a worker pool, a damage-blit composer, and a vsync-paced UI loop.

The engine pipeline is **Parse → Style → Layout → Paint**, wired in `internal/engine/engine.go`. `engine.NewSession` (`internal/engine/engine.go:422`) runs parse, scripts, stylesheet collection, fonts, cascade, images, and layout; `Session.PaintChecked` (`internal/engine/paint.go:13`) validates geometry and emits a frozen per-layer display list; `raster.Scheduler` and `surface.Loop` take it from there.

These are implemented subsets, not complete conformance. `docs/roadmap-v2.md` is the current production-readiness review and the ordered release gates — read it before planning new feature work, and do not re-plan things it records as already implemented. `PROJECT.md` has the full package inventory.

### Code Layout

Front end: `internal/dom` (HTML parser, DOM tree, events) → `internal/css` (tokenizer, stylesheet parser) → `internal/style` (cascade, computed styles) → `internal/layout` (box arena: block, inline, float, flex, grid, table, positioning). `internal/js` is a per-document goja runtime bound to that DOM. `internal/engine` wires all of it together and owns every admission limit.

Frame path: `internal/frame` (geometry, bitmaps, tiles, plans — the dependency leaf) ← `internal/paint` (display lists) ← `internal/surface` (window contract, composer, UI loop) ← `internal/raster` (glyph atlas, tile rasterizer, worker pool, scheduler) ← `internal/platform/{headless,darwin}`.

Chrome and app: `internal/tabs`, `internal/toolbar`, `internal/net`, `internal/image`, `internal/download`, `internal/bookmarks`, `internal/history`, `internal/session`, `internal/ax`. Binaries live in `cmd/` — `goosie` (browser), `goosie-headless` (HTML → PNG), `wpt-runner`, plus diagnostics.

`internal/archtest` enforces the rules below and is imported by no shipped binary. `test/` holds one package per unit under test.

### Import Rules

The allowed-imports table is `internal/archtest/rules.go:20-52`. Anything absent from it is a violation, and an internal package with no entry has no engine imports allowed at all — a new package must be added to the table deliberately. `test/` and `cmd/` are free.

Two chains run through the table: `frame` ← `paint` ← `surface` ← `raster` ← {`platform/*`, `toolbar`, `cmd/*`}, and `dom` ← `css` ← `style` ← `layout` → `paint`/`engine`. `surface` depends on no platform code and reaches `raster` only through an interface. cgo appears in exactly one directory: `internal/platform/darwin`, including via `//go:build` cgo tags. `internal/` also has no mutable package-level globals, and no banned GUI toolkit (`fyne.io`) may be linked.

All enforced by `go test ./internal/archtest/`.

### Untrusted Input

Binaries that touch untrusted input must call `Session.PaintChecked`, never `Paint`, and must admit input before parsing it. Per-document caps live in `internal/engine/limits.go:23-50`; image decode caps in `internal/image/decode.go:16-20` — probe encoded size, dimensions, and pixel count *before* decoding. `test/entrypoints/limits_test.go` guards this contract.

### Source Comments

First-party Go carries no comments: 32,893 comment-only lines and 683 trailing comments were removed on 2026-11-14 (both counted off the diff — deleted lines whose first token is `//`, and deleted lines whose trailing `//` was cut), and new code follows the same rule. Put the reasoning in the identifier, or in `docs/` when it is longer than a name can hold — `docs/engine-compatibility-review.md`, `docs/roadmap-v2.md` and the defect registry are where intent lives now. Two kinds of `//` line are functional and stay:

- `// qc:BR-NNN` — harvested by `test/qc/traceability_test.go` from the comment group directly above a `Test`/`Fuzz` func, counted by `test/qc/traceability.golden.json`, and swept by `scripts/rc-sweep.sh`. A tag dropped or separated from its func by a blank line is a CI failure, so keep the group adjacent.
- `//go:build` — a constraint is code, not comment.

`/* */` survives only as the cgo preamble in `internal/platform/darwin`, where the text is C. `//` inside a string literal is that language's source, not ours: `internal/userscript/script.go:53` parses `// ==UserScript==` metadata out of a userscript, so those lines are data the engine reads. `third_party/goja` keeps its upstream comments and licence headers.

With the source silent, docs are the only place a claim about code can be checked, so a `file.go:NNN` citation must name a declaration or a test function — never a bare line inside a function body. An interior number goes stale the moment anything above it moves and no gate notices; `GOOSIE-DOCSCITE-001` measures how many already have.

## Testing

```bash
go test ./...                            # everything
go test -race ./...                      # race detector
go vet -all ./...                        # must stay clean
go test ./internal/archtest/             # import, cgo, gofmt, globals rules
go test ./test/gate -run TestGate -v     # gate suite
make gate bench wpt-pilot                # see Makefile
```

All tests must pass on a machine with no display — gates and surface tests run against `platform/headless`, which exercises the real loop, scheduler, composer, and `Present` path. Timing budgets are gated only on the macOS nightly (`scripts/v2-gate-check.sh`), not on CI, so do not add duration assertions to the gate suite: it asserts on counters (style passes, layout passes, allocs, tiles rasterized vs reused).

## Visual Verification

Pure backend changes with no user-visible output require the automated tests above. When UI features are added, visual verification through headless comparison against a reference browser is required before the work can be considered complete. `testdata/render/` is the fixture corpus scored against Chromium/Playwright by `testdata/parity.py`; `testdata/interaction-parity/` holds event fixtures with a Chromium `reference-events.json`; `scripts/verify-phase*.js` are the per-phase Playwright checks.

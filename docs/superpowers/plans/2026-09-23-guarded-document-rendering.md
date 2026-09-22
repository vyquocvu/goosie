# Guarded Document Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make both real-document executables reject unsafe viewport/document geometry and enforce tile-cache and decoded-raster-image admission budgets before the corresponding allocations.

**Architecture:** Reuse `engine.ValidateViewport`, strengthen the signed tile-span calculation behind `ValidateExtent`, and require `PaintChecked` at both document entrypoints. Image metadata is checked before decoding; per-document reservations cover concurrent decodes and retained images. Keep networking, rendering, and native responsibilities in their existing packages; no new runtime dependencies or architecture-rule changes.

**Tech Stack:** Go 1.26.5 for the reviewed baseline, standard-library image codecs/testing/httptest, existing engine/frame/raster packages, macOS and headless backends.

---

## Status, scope and assumptions

Proposed implementation plan, based on `feat/v2` at `ba66721`, reviewed 2026-09-23. No implementation is included in this document. Obtain approval before executing it; do not commit, publish, change CI/permissions, or alter parity fixtures/references without the relevant authorization.

This is the first deliverable of [the production readiness roadmap](../../roadmap-v2.md), not the whole production-browser program. It closes specific allocation-admission gaps. It does not establish a total-process RSS/CPU bound, decoder isolation, correct tabs, complete resource-fetch budgets, or production readiness.

Three implementation approaches were considered:

1. **Reuse checked engine APIs and add image reservations — recommended.** Small, testable changes at existing boundaries; preserves supported content below the limits.
2. **Add checks only at CLI parsing.** Less code, but direct loader calls, decoded images, and document-derived tile allocation remain unguarded. Reject.
3. **Build a sandboxed document process first.** Important later, but substantially broader than this allocation-admission deliverable; process isolation also does not replace input budgets. Defer to its own design.

### Verified starting evidence

- `cmd/goosie/main.go:582-590` and `cmd/goosie-headless/main.go:118-130` call unchecked `Paint` and make cache budgets proportional to document area.
- `internal/engine/engine.go:445-465` already supplies `PaintChecked` and requires it for untrusted input.
- `internal/engine/limits.go:74-115` supplies viewport/extent validation. Its width-based tile calculation undercounts some negative, nonaligned coordinates; `frame.CoordFor` already implements signed floor division (`internal/frame/tile.go:23-30`).
- `cmd/goosie-headless/main.go:97-103` reads the complete input and performs an unbounded preliminary parse before the bounded engine parser. `extractStyleSheets` is used only there; `engine.NewSession` already collects embedded sheets (`internal/engine/engine.go:139-174`).
- `internal/image/decode.go:16-32` fully decodes before checking image dimensions. `internal/engine/engine.go:330-367` caps completed successes rather than reservations or attempts.
- `BitmapPool.maxIdle` limits idle retention, not all acquisitions (`internal/frame/pool.go:25-33,60-72`). Do not describe a 64 MiB cache as a 64 MiB process limit.

## Admission contract

Existing constants remain authoritative; do not increase them to make a regression pass.

| Resource | Limit | Enforcement point |
| --- | ---: | --- |
| Main document bytes | 8 MiB | Before headless parsing; existing HTTP/file response boundary in browser loader |
| DPR | Finite, greater than zero, at most 8 | Before conversion/allocation |
| Viewport dimensions | At most 16,384 device pixels each | `ValidateViewport` |
| Viewport area | At most 16,777,216 device pixels | `ValidateViewport` |
| Document geometry | Existing engine geometry bounds | Arena and extent validation |
| Document tile coordinates | At most 65,536 | Signed coordinate-span admission |
| Tile-cache allowance | At most 64 MiB / 256 tiles | Layer budget and positive idle-pool capacity |
| Encoded raster image | At most 8 MiB | Metadata/decode boundary, including direct decoder callers |
| Raster width/height | At most 8,192 pixels each | Metadata before full decode |
| One raster image | At most 16,777,216 decoded pixels | Metadata before full decode |
| One document's raster reservations | At most 33,554,432 decoded pixels | Atomic reservation before each full decode |
| Unique image fetch attempts | At most 256 per document | Select first unique URLs in document order before scheduling |
| Image workers | At most 12 | Retain current concurrency limit |

The raster caps are proposed policy choices, not measured peak-memory guarantees. Sixteen-bit PNG data, encoded responses, decoder scratch, fonts, display lists, tile pools, surfaces and GC retention have additional costs. The existing six-second image scheduling deadline is retained but is **not** described as an in-flight cancellation deadline.

Images that cannot be admitted follow the existing nonfatal image-failure behavior: no decoded image is installed, and declared layout dimensions remain available. Invalid document/viewport geometry rejects the document; it must not create/publish a replacement layer. This plan does not add an error-page UI.

## File responsibilities

| File | Planned responsibility/change |
| --- | --- |
| `internal/engine/limits.go` | One checked signed document-tile calculation; exported bounded cache sizing |
| `test/engine/viewport_test.go` | Allocation-free viewport/extent/cache boundary regressions |
| `internal/image/decode.go` | Encoded-length and metadata admission shared by all three decode entrypoints |
| `internal/image/decode_test.go` — new | Codec and pre-decode admission tests with an instance-local decoder spy |
| `internal/engine/engine.go` | Strict attempt ceiling, shared image/background reservations, failure release |
| `internal/engine/image_budget_test.go` — new | Deterministic concurrent reservation and deduplication tests |
| `cmd/goosie/main.go` | Early viewport checks, checked painting, bounded cache, error propagation |
| `cmd/goosie/main_test.go` — new | Exercise unexported `parse`, `build`, and `loadURLCtx` paths |
| `cmd/goosie-headless/main.go` | Bounded input read, remove preliminary parse, validate viewport, checked painting/cache |
| `test/entrypoints/document_limits_test.go` — new | Build and execute both binaries against temporary local inputs |

No separate policy framework or generic resource-manager package is needed. New test files are necessary because the executable packages currently have no tests. `cmd/` and `test/` are already exempt from internal import restrictions (`internal/archtest/rules.go:54-56`).

## Task 1 — Make tile admission and cache sizing exact

**Files:** `internal/engine/limits.go`, `test/engine/viewport_test.go`.

- [ ] Add an allocation-free regression using the actual extent API. Import `frame` in the existing external test package.

```go
func TestValidateExtentCountsSignedTileSpans(t *testing.T) {
    cases := []struct {
        name string
        rect frame.Rect
        valid bool
    }{
        {"aligned limit", frame.Rect4(0, 0, 65536, 65536), true},
        {"one extra column", frame.Rect4(0, 0, 65537, 65536), false},
        {"negative partial tiles", frame.Rect4(-1, -1, 65535, 65535), false},
        {"one negative tile", frame.Rect4(-256, -256, 0, 0), true},
        {"empty", frame.Rect4(0, 0, 0, 0), true},
        {"inverted", frame.Rect4(1, 0, 0, 1), false},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            if err := engine.ValidateExtent(tc.rect); (err == nil) != tc.valid {
                t.Fatalf("ValidateExtent(%v) = %v; valid=%v", tc.rect, err, tc.valid)
            }
        })
    }
}
```

- [ ] Run `go test ./test/engine -run '^TestValidateExtentCountsSignedTileSpans$' -count=1 -v`. The current negative-partial-tiles case must fail because 257×257 coordinates exceed 65,536.
- [ ] Move the geometry and tile-count logic into a private checked helper shared by `ValidateExtent` and the proposed `TileCacheBudget(extent frame.Rect) (tiles int, bytes int64, err error)`. Validate inversion/range before treating empty geometry as zero. Include the origin conservatively without changing either caller's actual layer bounds.
- [ ] Use signed floor tile coordinates, not rounded rectangle width. After validation and the empty check, the calculation is:

```go
lo := frame.CoordFor(frame.Point{X: min(extent.X0, 0), Y: min(extent.Y0, 0)}, frame.TileSize)
hi := frame.CoordFor(frame.Point{X: max(extent.X1, 0) - 1, Y: max(extent.Y1, 0) - 1}, frame.TileSize)
cols := int64(hi.Col) - int64(lo.Col) + 1
rows := int64(hi.Row) - int64(lo.Row) + 1
if cols <= 0 || rows <= 0 || cols > MaxDocumentTiles || rows > MaxDocumentTiles/cols {
    return 0, fmt.Errorf("document tile metadata limit exceeded (%d)", MaxDocumentTiles)
}
return cols * rows, nil
```

- [ ] `TileCacheBudget` must reject invalid extents before sizing. Given the validated nonnegative `documentTiles`, calculate `cacheTiles := min(documentTiles+8, MaxTileCacheBytes/frame.TileSizeBytes())`; return `int(cacheTiles)` and `cacheTiles*frame.TileSizeBytes()`. This is always positive, including empty content; never pass zero as the idle retention cap.
- [ ] Add table cases proving small documents retain headroom, long valid documents cap at 256 tiles/64 MiB, empty content gets a positive cap, and invalid extents return an error with zero usable capacity. Keep layer IDs and clipping independent of cache capacity.
- [ ] Run `go test ./test/engine ./test/frame -count=1`; inspect the diff. Do not change geometry tolerances or frame-grid behavior to hide a failing case.

## Task 2 — Admit raster metadata before decoding

**Files:** `internal/image/decode.go`, new `internal/image/decode_test.go`.

- [ ] Introduce tests for metadata admission and prove full decoding is never entered for rejected data. Use an instance-local helper accepting a decoder function so a spy fails safely; do not install mutable package-global hooks or actually decode huge adversarial images during a failing test.
- [ ] Define the following constants in `internal/image`, keeping its existing `gimage` alias for the standard image package:

```go
const (
    MaxEncodedImageBytes = 8 << 20
    MaxImageDimension = 8192
    MaxDecodedImagePixels int64 = 16 * 1024 * 1024
)
```

- [ ] Add `Probe(data []byte) (gimage.Config, error)` for engine preflight. `Probe` must check encoded length, call `gimage.DecodeConfig`, and then check positive dimensions and caps with division before multiplication:

```go
if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
    return gimage.Config{}, fmt.Errorf("image dimensions exceed admission limits")
}
if int64(cfg.Width) > MaxDecodedImagePixels/int64(cfg.Height) {
    return gimage.Config{}, fmt.Errorf("image pixel limit exceeded")
}
```

- [ ] Make `Decode` use the same admission before `gimage.Decode`. Keep `DecodePNG` and `DecodeJPEG` format-specific: bounded-read their readers, use the corresponding metadata decoder, then the corresponding full decoder. Reader buffering must use `io.LimitReader(r, MaxEncodedImageBytes+1)` and reject the extra byte. Preserve codec/read error causes with `%w`.
- [ ] Check decoded dimensions against the admitted reservation before returning. Preserve existing image representations and first-frame GIF behavior; GIF's first image may be smaller than or offset within its admitted logical screen. Do not require bounds equality or introduce an RGBA copy.
- [ ] Test valid small PNG/JPEG/GIF, transparency, 16-bit PNG, malformed/truncated input, wrong-format wrappers, exactly-at/over encoded byte limits, 8,193×1 rejection, 8,192×2,048 metadata acceptance, and 8,192×2,049 rejection. Metadata-only boundary tests must not invoke full image allocation.
- [ ] Run the new focused tests first and confirm the unsafe path is exposed, then implement admission and run `go test ./internal/image ./test/engine -count=1`. Existing ordinary image-rendering expectations must remain unchanged.

## Task 3 — Reserve document pixels before concurrent decode

**Files:** `internal/engine/engine.go`, new `internal/engine/image_budget_test.go`.

- [ ] Add a private per-load reservation object/helper with a fixed production limit of 33,554,432 pixels and a small injectable limit for package tests. Keep it local to a session construction/load, not global and not a user-facing configuration option.
- [ ] Add channel-controlled tests with a decode callback: hold admitted workers in decode; assert their total reservations never exceed the limit and a rejected image never enters full decode. Release a failed worker, then admit a subsequent image to verify rollback. Test exact equality, one-pixel overflow, and no double-release.
- [ ] Keep the existing document-order URL collection and image/background deduplication (`engine.go:292-328`). Select no more than the first 256 unique URLs before starting workers. Failed requests do not replenish this attempt allowance; duplicate references share one fetch/decode/reservation.
- [ ] Within each worker: fetch bounded bytes; call `imgdec.Probe`; compute validated `int64(width)*int64(height)`; reserve under the same lock used for reservation accounting; only then enter `imgdec.Decode`. Reprobing in the standalone decode function is acceptable and preserves that function's independent safety contract.

The atomic accounting operation, under the owning mutex, is:

```go
if pixels > limit-reserved {
    return false
}
reserved += pixels
return true
```

- [ ] Release a failed reservation exactly once. Retain successful charges for the batch lifetime because decoded pixels remain referenced by `byURL`, session maps, layout objects and display commands. Worker completion does not imply image release. Do not change existing image failure semantics into a fatal page error.
- [ ] Test that one URL used by several `<img>` elements and a CSS background is charged once, two documents have independent budgets, 257 failing unique references cause at most 256 fetch attempts, and the worker ceiling remains twelve. Over-budget pages may omit images; below-budget pages must keep their prior output.
- [ ] Run `go test ./internal/engine ./test/engine -count=1` and `go test -race ./internal/engine ./test/engine -count=1`. Synchronize with channels and bounded test contexts, not sleeps or the six-second production deadline.

## Task 4 — Apply guards at both real-document entrypoints

**Files:** both `cmd/*/main.go` files and new `cmd/goosie/main_test.go`.

- [ ] Add a real-loader regression using an `httptest.Server` counter: `loadURLCtx` with width zero must reject before any request and return a nil layer. Extend this to NaN/Inf scale and rounding-to-zero viewport cases. Use small safe inputs; these tests must not allocate a giant surface before failing.
- [ ] Add direct `build`/CLI validation cases so non-CLI callers cannot bypass viewport checks. Validate the original `float64` DPR before `devSize` conversion in CLI/build paths; also guard `loadURLCtx`'s direct boundary. Keep browser invalid-usage exit 2 and runtime failure exit 1; headless retains its current nonzero failure convention.
- [ ] In `loadURLCtx`, replace unchecked painting and area-proportional capacity with this sequence; propagate errors with context and return a nil layer:

```go
list, err := sess.PaintChecked(scale)
if err != nil {
    return nil, paint.SceneSpec{}, frame.Color(0), fmt.Errorf("goosie: paint document: %w", err)
}
dl := list.Build(1)
extent := dl.Extent()
budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
if err != nil {
    return nil, paint.SceneSpec{}, frame.Color(0), fmt.Errorf("goosie: size document cache: %w", err)
}
pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
layer := frame.NewLayer(1, extent, budgetBytes, pool)
layer.SetContent(dl)
```

  Keep layer identity appropriate for the existing one-layer frame plan; do not derive it from cache size. Recheck caller cancellation before returning the completed layer. Do not change async tab ownership in this task.

- [ ] Apply the same checked paint/cache sequence to `cmd/goosie-headless`, preserving its current origin-anchored layer bounds. Validate viewport before reading/parsing input or allocating fonts/surfaces.
- [ ] Replace headless `os.ReadFile` with a bounded file read: open, defer close, read `engine.MaxDocumentBytes+1` through a limited reader, and reject the extra byte before parsing. Remove the preliminary `dom.Parse`, its now-unused import, and `extractStyleSheets`; pass `nil` author CSS to `engine.NewSession`, which already collects embedded styles in document order. Do not add image/network fetching to this executable.
- [ ] Add direct-loader cases: a painted extent just above the tile limit returns no layer; a long valid document uses a capped cache; a refused image leaves ordinary page content available; embedded stylesheet ordering is applied once in the headless path.
- [ ] Run `go test ./cmd/goosie ./test/engine ./internal/image -count=1` and `go build ./cmd/goosie ./cmd/goosie-headless`. Inspect that every real-document `Paint` call in these two binaries now uses `PaintChecked`; synthetic trusted scene paths are unchanged.

## Task 5 — Verify executable contracts and valid rendering

**File:** new `test/entrypoints/document_limits_test.go`.

- [ ] Build both binaries into `t.TempDir`, following the existing temporary-build pattern in `test/gate/m1_gate_test.go:225-237`. Execute with `exec.CommandContext` and per-case deadlines. Use deterministic temporary HTML/local servers and separate output paths, never live sites or repository fixture edits.
- [ ] Test both binaries with NaN/Inf DPR, round-to-zero viewport, and 16,385×1 device dimensions. Assert nonzero exit, a validation error, and no newly produced PNG. The dimension-overflow case is deliberately narrow so the unfixed code cannot exhaust memory.
- [ ] Use a painted box sized 65,537×65,536 CSS pixels at DPR 1 with a small viewport to exercise document-tile rejection. Do not use millions of tiles or force tile-cache preallocation in a failing regression. Test exactly-at-limit geometry through allocation-free engine tests rather than rendering the whole surface.
- [ ] Test headless HTML of exactly the byte limit and one byte beyond using comment-only payloads (`"<!--"`, padding, `"-->"`) so the text/layout limits do not obscure the file-byte boundary; assert oversized input is rejected before document processing. Use smaller DOM/CSS limit cases separately to verify those existing checks remain intact.
- [ ] Run a valid 64×64 color-block document through both executables and assert PNG size and selected pixel values. This tests supported visual output, not just a successful exit or two presentations. If the existing headless two-present readiness behavior makes this fail, retain the failure and require a narrowly scoped readiness follow-up; do not weaken pixel assertions or silently broaden this plan.
- [ ] Exercise oversized raster metadata through the browser's screenshot path, where the image fetcher is actually wired. Keep rejection-before-decode proof in the spy tests; the CLI test should use a cheap malformed/oversized header that cannot generate a huge pixel buffer. Assert ordinary page content still renders.
- [ ] Run the focused entrypoint suite, then the full regression commands below. Record outcomes and any known baseline failures without changing unrelated code.

```bash
go test ./test/entrypoints -count=1 -v
go test -count=1 ./...
go test -race -count=1 ./...
go build ./...
go vet -all ./...
CGO_ENABLED=0 go build ./...
```

Expected after this scoped change: new allocation/entrypoint tests and existing full tests/build pass; no new race reports. The pre-existing vet and Darwin no-cgo failures remain separately tracked unless explicitly assigned their own fix. Do not report an all-green release baseline while they remain.

- [ ] Perform a supported static-document render comparison using current reference artifacts and unchanged tolerances. Validate every expected output exists, sizes match, and rendering commands succeeded; the current `testdata/parity.py` exit status alone is not a gate because it skips missing pairs and returns zero with failing scores (`:30-52,99-109`). A fresh live-site sweep is not required for this focused boundary change.
- [ ] Review `git diff --check` and the scoped diff. No dependencies, architecture rules, CI permissions, fixtures/references, or unrelated UI features may be changed. Ask for review; commit only if explicitly requested.

## Completion criteria

- [ ] Both document entrypoints validate viewport before conversion/allocation and use checked painting plus bounded cache sizing.
- [ ] Signed/negative extent accounting cannot undercount the admitted tile span.
- [ ] Headless input reaches the bounded engine parser without an unbounded preliminary read/parse.
- [ ] Every raster decoder entrypoint rejects over-limit metadata before full decode.
- [ ] Per-document reservations include in-flight and retained results, roll back failures, and deduplicate shared URLs.
- [ ] Actual application paths reject invalid input, render valid small content, and retain the expected existing test results.
- [ ] Evidence distinguishes admission limits from total-memory, CPU-time, native safety and public-release guarantees.

## Deferred, with evidence

- **Independent tabs and state ownership:** `cmd/goosie/main.go:199-201,315-525`; `cmd/goosie/toolbar_window.go:34-69`. Next application-lifecycle deliverable, not part of this allocation patch.
- **Chrome/content separation, resize and headless readiness:** `cmd/goosie/toolbar_window.go:38-43,108-110`; `cmd/goosie-headless/main.go:161-174`. Do not claim complete UI correctness from valid-content smoke tests.
- **Aggregate CSS/font fetch limits and CPU cancellation:** `internal/engine/imports.go:18-63`; `internal/engine/engine.go:187-215,333-367`. The image attempt/pixel caps do not solve all resource exhaustion.
- **Native pointer/stride contracts and process isolation:** `internal/platform/darwin/window.go:230-255`; `internal/platform/darwin/shim_darwin.m:721-727`. Still blocks an untrusted-web production claim.
- **Origin/mixed-content/private-network policy:** `internal/net/http.go:273-281`; `internal/engine/limits.go:571-584`.
- **Dynamic APIs, interaction and persistence:** `cmd/goosie/main.go:573-591`; `internal/net/http.go:270-288`; `internal/tabs/tab.go:9-18`.
- **Vet, no-cgo, release smoke and compatibility-gate defects:** `test/style/gradient_test.go:113`; `internal/platform/platform_darwin.go:21`; `.github/workflows/release.yml:49-56`; `testdata/parity.py:30-52,99-109`.

No calendar commitment or production release follows from completing this plan. The next gate is a separately reviewed tab/document lifecycle plan, while native/resource-policy/security blockers remain tracked in the roadmap.

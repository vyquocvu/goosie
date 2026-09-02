# Stream A: First-Paint Render Pipeline Optimization

## Status
**DRAFT** — awaiting user review

## Goal
Reduce first-paint latency (HTML parse → CSS extract → render tree → style cascade → layout → display list → raster) through architecture-first rework of the style and layout pipelines, followed by evidence-driven micro-tuning. Zero pixel or behavior change.

## Success Criteria
- Measurable improvement in `perf-review` stage timings on the offline fixture set vs. committed baseline.
- Measurable improvement in `bench.sh` suite benchmarks (benchstat, p < 0.05 on affected benchmarks).
- Zero failures in `go test ./...` (including layoutgolden).
- Pixel-hash manifest unchanged (SHA-256 of headless PNG output for fixture set).
- E2E `CompareGoosieVsBrowser` passes on representative fixtures per AGENTS.md requirements.

## Scope
**In scope:**
- `internal/renderer/style.go` — style cascade, `StyleManager`, `ApplyStyles`, `applyMatchingRules`
- `internal/renderer/style_resolver.go` — `StyleResolver`
- `internal/renderer/layout.go` — layout engine, `ComputeLayout`
- `internal/renderer/node.go` — `RenderNode`, `Style` struct
- `internal/renderer/display_list.go` — `DisplayListBuilder`, `convertPaintCommands`
- `internal/renderer/headless.go` — `RenderHTMLToImage` entry point
- `internal/css/match_cache.go` — `MatchCache`, `ElementKey`, `StylePool`
- `internal/css/computed.go` — `ComputedStyle`, `Fingerprint`
- `internal/css/selector.go` — selector matching
- `cmd/perf-review/` — baseline measurement extension
- `scripts/bench.sh` — benchmark runner

**Out of scope (later streams):**
- JS runtime (Stream B)
- Network subsystem (Stream C)
- Scroll/interaction smoothness (Stream D)
- GUI layer (Stream E)

## Approach
Architecture-first rework, gated by measurement. Each numbered change is a separate, independently verifiable commit.

## Phases

### Phase 0 — Measurement Foundation (hard prerequisite)

#### 0.1 First-paint benchmark harness
Extend `cmd/perf-review` with a fixed offline fixture set:
- Representative subset of `testdata/` pages (typography, layout, flexbox, grid, forms, tables, CSS advanced, edge cases).
- 2–3 synthetic large-page fixtures (generated HTML with many nodes/rules).
- Stage timings: parse → CSS extract → render tree → style → layout → display list → raster.
- Output: JSON with per-stage durations, total first-paint time, allocs/op.

#### 0.2 Pixel-reference harness
Script or test that:
- Renders the fixture set headlessly via `RenderHTMLToImage`.
- Computes SHA-256 of each output PNG.
- Compares against a committed manifest (`docs/perf/pixel-manifest.json`).
- Fails if any hash differs.
- Provides an `UPDATE_MANIFEST=true` flag for intentional baseline changes.

#### 0.3 Baseline capture
- Run `bench.sh suite` → commit results as `docs/perf/baseline-bench.txt`.
- Run `perf-review` on fixture set → commit results as `docs/perf/baseline-perf-review.json`.
- Capture CPU/mem pprof of `RenderHTMLToImage` on 2–3 representative fixtures → commit as `docs/perf/baseline-*.prof`.

### Phase 1 — Style Pipeline Rework (architectural core)

#### 1.1 Enumerated property atoms
**Problem:** `renderer.Style` has ~20 string-valued fields (`Display`, `Position`, `Float`, `TextAlign`, ...) compared via string equality in layout hot paths.
**Solution:** Replace hot string fields with interned atom/enum types (reuse existing CSS atom interning from `internal/css/properties.go`). Conversions happen at parse boundaries; layout code compares uint8/uint16 values.
**Risk:** Low — mechanical refactor, behavior-preserving if atom values are 1:1 with string values.
**Verification:** Unit tests for atom conversion; layoutgolden; pixel-hash manifest.

#### 1.2 Computed-style deduplication & shared inheritance
**Problem:** `ComputedStyle` is a per-node pointer; inheritance copies 14 fields per node; `CustomProperties` map is deep-copied.
**Solution:**
- Extend `Fingerprint`/`StylePool` pattern (from `css.ComputedStyle`) to `renderer.Style`.
- Structurally share identical computed styles across nodes (pointer equality).
- Inheritance: pointer-share an immutable inherited block; child nodes receive the same pointer unless overridden (copy-on-write).
- `CustomProperties`: share parent map pointer; clone only on `--x: ...` override.
**Risk:** Medium — requires proving that shared styles are truly immutable during the render pass.
**Verification:** Unit tests for fingerprint equality; layoutgolden; pixel-hash; benchstat on style benchmarks.

#### 1.3 Bucketed selector matching
**Problem:** `applyMatchingRules` is O(nodes × rules); every node walks both default + author stylesheets.
**Solution:** Partition prepared rules by right-most compound selector (tag/id/class buckets). Each node tests only candidate rules from the relevant bucket. Integrate with existing `MatchCache` invalidation contract.
**Risk:** Medium — must preserve specificity ordering and source order within buckets.
**Verification:** Unit tests for rule ordering; layoutgolden; pixel-hash; benchstat on style benchmarks.

#### 1.4 Inline-style parse cache
**Problem:** `style=` attribute is re-parsed per node per render.
**Solution:** Memoize inline style parsing keyed by the attribute string (bounded LRU, same pattern as `MatchCache`).
**Risk:** Low — pure caching, invalidation on stylesheet change.
**Verification:** Unit tests for cache hit/miss; layoutgolden; pixel-hash.

### Phase 2 — Layout & Display-List Tuning (evidence-driven)

Fix only what Phase 0 profiles implicate. Likely candidates:
- **Intrinsic size memoization:** Key by (node structure hash, available width) — extension of existing `intrinsic_size_cache.go`.
- **Text-run glyph allocation:** Pool `frame.Glyph` slices in `buildTextRun` (headless path).
- **`convertPaintCommands` allocations:** Reduce intermediate allocations in the paint command conversion loop.

Each as a separate, small benchmark-gated change. No speculative rework.

### Phase 3 — Verification Protocol (applies to every change)

For each numbered change in Phases 1–2:
1. **benchstat comparison:** Old vs new, `-count=10`, for affected benchmarks + `perf-review` stage timings.
2. **Full test suite:** `go test ./...` (including layoutgolden) — zero failures.
3. **Pixel manifest:** Hashes unchanged.
4. **E2E (if rendering output paths touched):** `CompareGoosieVsBrowser` on a representative fixture per AGENTS.md.
5. **Commit:** Each change is a separate commit with benchstat delta in commit message.

**Abort criterion:** If Phase 0 profiles show style is *not* among the top hotspots, re-rank Phase 1 items by actual evidence rather than executing in listed order.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Cascade changes cause subtle behavior changes | Per-node golden style snapshot tests (assert on computed style dump of a fixture, in the style of layoutgolden) |
| String→enum migration affects many call sites | Done property-by-property behind tests; mechanical refactor |
| Shared styles mutated unexpectedly | Prove immutability during render pass; add defensive checks in debug builds |
| Bucketed matching breaks specificity ordering | Unit tests for rule ordering within buckets; layoutgolden |

## Deliverables
1. `docs/perf/baseline-*.txt|json|prof` — committed baseline measurement files.
2. `docs/perf/pixel-manifest.json` — committed pixel hash manifest.
3. Extended `cmd/perf-review` with offline fixture set and stage timings.
4. Pixel-reference harness (script or test).
5. Phase 1 changes (1.1–1.4) as separate commits.
6. Phase 2 changes (evidence-driven) as separate commits.
7. Final benchstat comparison showing improvement vs. baseline.

## Timeline
- Phase 0: 1–2 days (baseline capture, harness implementation).
- Phase 1: 3–5 days (architectural rework, one change at a time).
- Phase 2: 2–3 days (evidence-driven tuning).
- Total: 6–10 days for Stream A.

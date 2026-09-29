# Goosie QA Plan

> **Owner:** QA Lead  
> **Status:** Living document — update as subsystems mature  
> **Last updated:** 2026-04-12

---

## Table of Contents

1. [Test Layer Definitions](#1-test-layer-definitions)
2. [Feature Coverage Matrix](#2-feature-coverage-matrix)
3. [Quality Gates](#3-quality-gates)
4. [WPT Integration Strategy](#4-wpt-integration-strategy)
5. [CI/CD Integration](#5-cicd-integration)
6. [Test Infrastructure Needs](#6-test-infrastructure-needs)

---

## 1. Test Layer Definitions

The testing pyramid is organized into five layers, from fast unit tests at the base to slow spec-compliance tests at the apex. Each layer has a distinct purpose, execution environment, and failure semantics.

```
                    ┌─────────────┐
                    │  Layer 4    │  Fuzz testing
                    │  (Apex)     │  DOM parser, CSS parser, style cascade
                   ┌┴─────────────┴┐
                   │   Layer 3     │  WPT subset (spec conformance)
                  ┌┴───────────────┴┐
                  │    Layer 2      │  Visual parity (56 fixtures vs Chromium)
                 ┌┴─────────────────┴┐
                 │     Layer 1       │  Integration tests (cross-package)
                ┌┴───────────────────┴┐
                │      Layer 0        │  Unit tests (in-package)
                └─────────────────────┘
```

### Layer 0 — Unit Tests

**Purpose:** Verify individual functions and methods in isolation. Fast feedback, zero I/O, no external dependencies.

**Scope:** Every internal package has in-package `_test.go` files. These test pure logic: CSS tokenization, DOM tree operations, layout calculations, tile math, session serialization, etc.

**Location:** `internal/*/` — 132 test files across 23 subsystems, ~771 test functions total.

**Execution:**
- Every PR via `go test -count=1 ./...`
- Race detector via `go test -race ./...`
- Sub-second per package; full suite under 30 seconds

**Key files:**
- `internal/css/` — tokenizer, parser, selector matching
- `internal/dom/` — node tree, element operations, bounded parsing
- `internal/layout/` — box model, flex, grid, table layout
- `internal/frame/` — tile grid, damage tracking, scene graph
- `internal/raster/` — CPU tile rasterizer, worker pool
- `internal/surface/` — compositing, damage-blit
- `internal/paint/` — display list, render commands
- `internal/style/` — cascade resolution, viewport styles
- `internal/engine/` — resource limits, document orchestration
- `internal/session/` — persistence, tab state
- `internal/bookmarks/`, `internal/history/`, `internal/download/`, `internal/tabs/`

**Pass criteria:** 100% of unit tests must pass. Any failure blocks PR merge.

---

### Layer 1 — Integration Tests

**Purpose:** Verify cross-package interactions. Test that subsystems compose correctly end-to-end within the Go process.

**Scope:** Tests that wire multiple packages together: HTML in -> styled layout out, scene construction through the full frame path, session round-trips, engine lifecycle with real DOM + CSS + layout.

**Location:** `test/` directory packages that import multiple `internal/` packages.

**Key files:**
- `test/frame/` — full frame path integration (scene -> layer -> raster -> compose)
- `test/engine/` — engine lifecycle with real documents
- `test/layout/` — layout pipeline with real CSS + DOM
- `test/paint/` — paint commands through raster pipeline
- `test/surface/` — surface composition with real frames
- `test/raster/` — rasterizer with real tile workloads
- `test/platform/` — headless platform integration
- `test/entrypoints/` — binary entrypoint smoke tests
- `test/net/` — network layer integration
- `test/js/` — JS runtime integration
- `test/domtest/` — DOM conformance tests

**Execution:**
- Every PR via `go test -count=1 ./...` (same command as unit tests; Go resolves both)
- Race detector in separate CI job

**Pass criteria:** 100% must pass. Any failure blocks PR merge.

---

### Layer 2 — Visual Parity Tests

**Purpose:** Verify that goosie renders HTML fixtures pixel-identically to Chromium. This is the primary correctness signal for the rendering pipeline.

**Scope:** 56 HTML fixtures across 17 categories, rendered by both goosie (headless) and Chromium (Playwright), then compared pixel-by-pixel.

**Location:**
- Fixtures: `testdata/render/` (56 HTML files in 17 subdirectories)
- Comparison: `testdata/parity.py`
- Chromium renderer: `testdata/render-compare.js` (Playwright)
- Live URL parity: `testdata/parity_urls.py` (20 URLs in `urls.txt`)

**Fixture categories:**

| Category | Count | What it tests |
|----------|-------|---------------|
| basic | 2 | Simple HTML structure |
| box-model | 4 | Margins, padding, width/height |
| borders | 3 | Border styles, widths, colors |
| colors | 3 | Background colors, color values |
| complex | 4 | Multi-feature compositions |
| flex | 6 | Flexbox layout |
| forms | 5 | Form controls rendering |
| grid | 2 | CSS Grid layout |
| lists | 2 | Ordered/unordered lists |
| overflow | 2 | Overflow clipping/scrolling |
| positioning | 3 | absolute/relative/fixed positioning |
| selectors | 3 | CSS selector matching |
| semantic | 3 | Semantic HTML elements |
| tables | 2 | Table layout |
| typography | 9 | Fonts, text, line-height |
| unicode | 2 | Unicode text rendering |

**Comparison algorithm:**
- Per-pixel RGB channel difference threshold: 30 (out of 255)
- Pass threshold: 90.0% of pixels must be within tolerance
- Rendered at 800x600, DPR 1.0

**Execution:**
- **Current status:** NOT in CI. Run manually via `testdata/parity.py`
- **Target:** Every PR (see Section 5)

**Pass criteria:** >= 90% pass rate across all 56 fixtures. Any regression in a previously-passing fixture is a P0 blocker.

---

### Layer 3 — WPT Subset

**Purpose:** Measure spec compliance against the canonical Web Platform Tests suite. Provides an industry-standard conformance signal.

**Scope:** Curated subset of WPT reftests in directories where goosie has partial or full support. Tests are rendered by goosie headless and compared against reference renders.

**Location:**
- Harness: `test/wpt/` (6 files: `wpt.go`, `wpt_runner.go`, `wpt_server.go`, `wpt_checkout.go`, `wpt_report.go`, `wpt_test.go`)
- Curation: `test/wpt/curate.go` — maps goosie capabilities to WPT directories
- Pinned commit: tracked in `DefaultWPTCommit` in `test/wpt/wpt.go`

**Currently curated directories (based on goosie feature support):**

| Feature | Supported | WPT Directories |
|---------|-----------|-----------------|
| HTML basic | Yes | `html/semantics/`, `html/dom/` |
| CSS basic | Yes | `css/css-box-model/`, `css/css-color/`, `css/css-text/` |
| CSS Flexbox | Yes | `css/css-flexbox/` |
| CSS Grid | No | `css/css-grid/` (excluded) |
| Canvas | No | `html/canvas/` (excluded) |
| SVG | No | `svg/` (excluded) |
| JS DOM | No | `dom/` (excluded) |
| CSS Gradients | No | (excluded) |
| CSS Pseudo-elements | No | (excluded) |

**Test harness design:**
1. Checkout pinned WPT commit via GitHub tarball API (`wpt_checkout.go`)
2. Serve test files via local HTTP server (`wpt_server.go`)
3. Render each test with goosie headless screenshot
4. Render reference page (or use relation="match"/"mismatch" metadata)
5. Compare images using same algorithm as parity.py (threshold=30)
6. Generate JSON + HTML report (`wpt_report.go`)

**Configuration:**
- Viewport: 800x600
- DPR: 1.0 (configurable)
- Timeout: 30s per test
- Pass threshold: 95.0% (configurable in `test/wpt/wpt.go`)

**Execution:**
- **Current status:** Harness exists, tests not running in CI
- **Target:** Nightly (see Section 5)

**Pass criteria:** >= 95% pass rate on curated subset. Per-directory breakdown reported in dashboard.

---

### Layer 4 — Fuzz Testing

**Purpose:** Find parser crashes, infinite loops, and resource exhaustion by feeding mutated inputs to the core parsing subsystems.

**Scope:** Three fuzz targets using Go's native `testing.F` framework.

**Targets:**

| Target | File | What it fuzzes |
|--------|------|----------------|
| `FuzzParseDocument` | `test/dom/fuzz_test.go` | `dom.ParseBounded` — HTML parser with tight (Nodes:24, Depth:8) and loose (Nodes:4096, Depth:64) limits. Categorizes results into 4 buckets: boundChecked, refusalExplained, refusalDropped, refusedBoth |
| `FuzzParseStylesheet` | `test/css/fuzz_test.go` | `css.ParseForViewport` — CSS parser idempotency across intervening parses |
| `FuzzResolveCascade` | `test/style/fuzz_test.go` | `style.ResolveViewport` — style cascade idempotency, catches rule index stamp counter leaks |

**Additional coverage:**
- `TestParsePathCoverage` in `test/dom/fuzz_test.go` — audits parser with 4000 synthetic corpus inputs

**Execution:**
- Nightly CI: 120 seconds per target (`-fuzztime=120s`)
- PR CI: not run (too slow for PR feedback); corpus-only seed tests run via `go test`

**Pass criteria:** Zero crashes, zero hangs, zero resource limit violations. Any corpus regression is P0.

---

## 2. Feature Coverage Matrix

For each browser subsystem, this matrix lists what is tested today, what is missing, and the priority of filling the gap.

### Rendering Pipeline

| Subsystem | Tested Today | Gaps | Priority |
|-----------|-------------|------|----------|
| **HTML Parser** (`internal/dom/`) | `test/dom/fuzz_test.go` (fuzz + corpus), unit tests in `internal/dom/` | No WPT html5lib tree-construction tests; no malformed-HTML recovery suite | P0 |
| **CSS Parser** (`internal/css/`) | `test/css/fuzz_test.go` (fuzz), unit tests in `internal/css/` | No WPT css-syntax tests; no invalid-CSS recovery suite | P0 |
| **Style Cascade** (`internal/style/`) | `test/style/fuzz_test.go` (fuzz), unit tests in `internal/style/` | No specificity edge-case suite; no @media query tests | P1 |
| **Layout** (`internal/layout/`) | Unit tests in `internal/layout/`, parity fixtures (box-model, flex, grid) | No WPT layout tests; no flexbox alignment edge cases; no grid auto-placement tests | P0 |
| **Paint** (`internal/paint/`) | `test/paint/` integration tests | No display list snapshot tests; no z-ordering edge cases | P1 |
| **Frame/Scene** (`internal/frame/`) | `test/frame/` integration, `test/gate/` performance gates | No scene graph diff tests; no damage tracking correctness suite | P1 |
| **Raster** (`internal/raster/`) | `test/raster/` integration, gate benchmarks | No tile content verification suite; no worker pool stress test | P1 |
| **Surface** (`internal/surface/`) | `test/surface/` integration | No compositor layer ordering tests | P2 |
| **Image** (`internal/image/`) | Unit tests in `internal/image/` | No animated image tests; no ICC profile handling tests | P2 |

### Browser Features

| Subsystem | Tested Today | Gaps | Priority |
|-----------|-------------|------|----------|
| **Engine** (`internal/engine/`) | `test/engine/` lifecycle, resource limits in `internal/engine/limits.go` | No concurrent document loading stress test; no limit exhaustion recovery tests | P0 |
| **Session** (`internal/session/`) | Unit tests in `internal/session/`, `test/session/` | No concurrent tab session isolation test; no session corruption recovery | P1 |
| **Tabs** (`internal/tabs/`) | Unit tests in `internal/tabs/`, `test/tabs/` | No tab crash recovery test; no large tab count stress test | P1 |
| **Bookmarks** (`internal/bookmarks/`) | Unit tests in `internal/bookmarks/`, `test/bookmarks/` | No import/export round-trip tests; no large bookmark tree perf test | P2 |
| **History** (`internal/history/`) | Unit tests in `internal/history/`, `test/history/` | No history migration tests; no concurrent write safety test | P2 |
| **Download** (`internal/download/`) | Unit tests in `internal/download/`, `test/download/` | No partial download resume test; no download cancellation cleanup test | P2 |
| **Network** (`internal/net/`) | `test/net/` integration | No redirect chain tests; no certificate pinning tests | P1 |
| **JS** (`internal/js/`) | `test/js/` integration | No WPT DOM binding tests; no JS memory leak detection | P1 |

### Infrastructure & Quality

| Subsystem | Tested Today | Gaps | Priority |
|-----------|-------------|------|----------|
| **Architecture** (`internal/archtest/`) | Import boundary tests, cgo enforcement, gofmt checks | No dependency cycle detection beyond imports; no API surface stability checks | P1 |
| **Gate/Performance** (`test/gate/`) | M1 zero-alloc warm scroll, M1 cold scroll burst, M3 scroll gate, footprint gate | No startup time gate; no memory regression gate in CI; no jank detection | P0 |
| **Platform** (`internal/platform/`) | `test/platform/` headless integration | No Darwin window lifecycle tests in CI; no Windows headless tests | P1 |
| **Toolbar** (`internal/toolbar/`) | Unit tests in `internal/toolbar/`, `test/toolbar/` | No toolbar interaction integration tests | P2 |
| **Accessibility** (`internal/ax/`) | Unit tests in `internal/ax/` | No accessibility tree correctness tests; no screen reader output tests | P2 |

### Resource Limits (defined in `internal/engine/limits.go`)

| Limit | Value | Tested | Gap |
|-------|-------|--------|-----|
| MaxDocumentBytes | 8 MB | Fuzz with tight limits | No explicit limit enforcement test at boundary |
| MaxDocumentNodes | 50,000 | Fuzz with tight limits | No test verifying exact cutoff behavior |
| MaxDocumentDepth | 128 | Fuzz with tight limits | No deeply-nested document stress test |
| MaxAttributes | 64 | Unit tests | No attribute overflow integration test |
| MaxCSSBytes | 4 MB | Fuzz | No large-stylesheet parsing perf test |
| MaxCSSRules | 32,768 | Unit tests | No rule count boundary test |
| MaxLayoutObjects | 131,072 | Gate tests (indirect) | No layout object exhaustion test |
| MaxDPR | 8 | Config tests | No extreme DPR rendering test |
| MaxDocumentTiles | 65,536 | Gate tests | No tile exhaustion recovery test |
| MaxTileCacheBytes | 64 MB | Footprint gate | No cache eviction under pressure test |

---

## 3. Quality Gates

Quality gates define the pass/fail criteria for merging code, running nightly builds, and cutting releases. Gates use **counters** (not durations) for CI determinism, except for macOS nightly where timing is reliable.

### 3.1 PR Merge Gate

These must all pass before a PR can merge. Enforced by `.github/workflows/ci.yml`.

| Gate | Metric | Threshold | Current Status |
|------|--------|-----------|----------------|
| Build | Compiles on all PR targets | 0 errors | Passing |
| Unit + Integration tests | `go test -count=1 ./...` | 100% pass | 2 failures (gofmt in `test/wpt/curate.go`) |
| Race detector | `go test -race ./...` | 0 races | Passing |
| Architecture | `go test ./internal/archtest/` | Import boundaries, cgo, gofmt, no mutable globals | Failing (gofmt) |
| Visual parity | 56 fixtures vs Chromium | >= 90% pass rate | Not in CI |

**PR gate enforcement:**
- All jobs must be green
- No flaky test tolerance — any failure blocks merge
- Architecture tests are a hard gate (import violations = design regression)

### 3.2 Nightly Gate

Run daily at 06:00 UTC on macOS (M1). Enforced by `.github/workflows/nightly-bench.yml`.

| Gate | Metric | Threshold | Script/Location |
|------|--------|-----------|-----------------|
| Warm scroll | Zero allocations per frame | 0 allocs/op | `test/gate/m1_gate_test.go::TestGate_WarmScrollZeroAllocsZeroAllocs` |
| Cold scroll burst | Zero allocations on UI path | 0 allocs/op | `test/gate/m1_gate_test.go::TestGate_ColdScrollBurstNoRedundantJobs` |
| Mean frame time | Warm scroll mean | <= 16ms | `scripts/v2-gate-check.sh -mean 16` |
| P99 frame time | Warm scroll p99 | <= 33ms | `scripts/v2-gate-check.sh -p99 33` |
| Warm scroll latency | Total warm scroll time | <= 5ms | `scripts/v2-gate-check.sh -warm-ms 5` |
| Cold scroll latency | Total cold scroll time | <= 8ms | `scripts/v2-gate-check.sh -cold-ms 8` |
| Startup time | Binary to first frame | <= 1000ms | `scripts/v2-gate-check.sh -startup-ms 1000` |
| Tile redundancy | Cold scroll jobs == stale tiles | Exact match | `TestGate_ColdScrollBurstNoRedundantJobs` |
| GPU linkage | No Metal/OpenGL/Vulkan symbols | 0 symbols | `TestGate_NoGPULinkAGE` |
| Fuzz (DOM) | 120s fuzz run | 0 crashes | `FuzzParseDocument` |
| Fuzz (CSS) | 120s fuzz run | 0 crashes | `FuzzParseStylesheet` |
| Fuzz (Cascade) | 120s fuzz run | 0 crashes | `FuzzResolveCascade` |
| Memory footprint | Retained memory vs configured caches | Within slack bounds | `test/gate/footprint_gate_test.go` (slack: 12MB retained, 4MB growth) |
| Scroll efficiency | 40,960px document, 600 frames | < 5% tile miss rate | `test/gate/scroll_gate_test.go` |

**Nightly gate enforcement:**
- `scripts/v2-gate-check.sh` exits 0 = pass, 1 = budget missed, 2 = bad invocation
- Benchmark JSON artifacts are archived for trend analysis
- Fuzz failures are P0 — investigate within 24 hours

### 3.3 Release Gate

Run on every tagged release (`v*`). Enforced by `.github/workflows/release.yml`.

| Gate | Metric | Threshold |
|------|--------|-----------|
| Build matrix | 5 platforms (darwin arm64/amd64, linux amd64/arm64, windows amd64) | All build successfully |
| Tests | Full test suite on release host | 100% pass |
| Smoke test | Binary runs headless, frame path reached, all frames presented | `scripts/release-smoke.sh` passes |
| No tile failures | Zero tile rasterization failures | 0 failures |
| No worker panics | Zero worker goroutine panics | 0 panics |

**Release gate enforcement:**
- `scripts/release-smoke.sh` validates: binary starts, frame path is reached, all frames are presented, no tile failures, no worker panics
- Release is blocked until all matrix jobs pass
- Binaries are built but not published until gate passes

### 3.4 Coverage Requirements

| Layer | Coverage Target | Measurement | Enforcement |
|-------|----------------|-------------|-------------|
| Layer 0 (Unit) | Track trend (no regression) | `go test -coverprofile` | Nightly report |
| Layer 1 (Integration) | All cross-package paths exercised | Manual audit quarterly | QA review |
| Layer 2 (Parity) | >= 90% fixture pass rate | `testdata/parity.py` | PR gate (target) |
| Layer 3 (WPT) | >= 95% on curated subset | `test/wpt/` harness | Nightly report |
| Layer 4 (Fuzz) | 120s/target/day, 0 crashes | Nightly CI | Nightly gate |

---

## 4. WPT Integration Strategy

### 4.1 Phase 1: Import Curated Subset (Current)

Start with WPT directories that map to goosie's existing capabilities. The curation logic in `test/wpt/curate.go` already defines this mapping.

**Priority directories for initial import:**

| Priority | WPT Directory | Rationale |
|----------|--------------|-----------|
| P0 | `css/css-box-model/` | Core rendering correctness |
| P0 | `css/css-color/` | Color parsing and application |
| P0 | `html/semantics/` (basic) | HTML parsing correctness |
| P1 | `css/css-flexbox/` | Flexbox is supported; validate against spec |
| P1 | `css/css-text/` | Text layout correctness |
| P2 | `css/css-position/` | Positioning is partially supported |
| P2 | `css/selectors/` | Selector matching correctness |

**Excluded directories (no goosie support yet):**
- `css/css-grid/` — Grid not implemented
- `html/canvas/` — Canvas not implemented
- `svg/` — SVG not implemented
- `dom/` — JS DOM bindings not implemented
- `css/css-images/` (gradients) — Not implemented
- `css/css-pseudo/` — Pseudo-elements not implemented

### 4.2 Curation Approach

The curation system in `test/wpt/curate.go` uses a feature-support model:

```
GoosieSupport() -> map[Feature]bool
CuratedDirs()   -> map[Feature][]string   // maps supported features to WPT dirs
ShouldSkip()    -> classifies tests to skip
```

**Curation rules:**
1. Only import directories for features where `GoosieSupport()` returns `true`
2. Skip tests that require JS execution (`ShouldSkip` returns true for JS-dependent tests)
3. Skip tests with external dependencies (network, fonts not available locally)
4. Skip tests known to be flaky in headless rendering contexts
5. Maintain an exclude list per directory for known-failing tests (with issue tracker references)

**Updating curation:**
- When a new feature lands, add it to `GoosieSupport()` and `CuratedDirs()`
- When a test starts passing, remove it from the exclude list
- When a test starts failing, add it to the exclude list with a tracking issue

### 4.3 Test Harness Design

The harness in `test/wpt/` is already functional. Key components:

```
test/wpt/
  wpt.go          — Types, Config, defaults (viewport 800x600, DPR 1.0, timeout 30s)
  wpt_checkout.go — Downloads pinned WPT commit via GitHub tarball API
  wpt_server.go   — HTTP server for test files with path traversal prevention
  wpt_runner.go   — Test discovery, rendering, image comparison (threshold=30)
  wpt_report.go   — JSON + HTML dashboard with per-directory breakdown
  wpt_test.go     — Entry points: TestWPTSuite, TestWPTCurator, config tests
  curate.go       — Feature-support mapping, skip classification
```

**Harness improvements needed:**
1. **Subtest reporting** — Report each WPT test as a named Go subtest for CI visibility
2. **Baseline tracking** — Store per-test pass/fail baselines; flag regressions
3. **Incremental updates** — Support updating WPT commit without full re-download
4. **Timeout handling** — Hard kill on per-test timeout (currently 30s, may need tuning)
5. **Font fallback** — Handle missing system fonts gracefully in comparison

### 4.4 Pass Rate Targets

| Milestone | Target | Timeline |
|-----------|--------|----------|
| Initial import | Establish baseline (measure current pass rate) | Immediate |
| Month 1 | >= 80% on curated subset | Fix low-hanging rendering bugs |
| Month 3 | >= 90% on curated subset | Address flexbox, positioning gaps |
| Month 6 | >= 95% on curated subset | Near-spec compliance on supported features |
| Ongoing | >= 95% with expanding directory list | Add directories as features land |

### 4.5 WPT Commit Pinning

The WPT commit is pinned in `DefaultWPTCommit` (`test/wpt/wpt.go`). Update strategy:
- Update quarterly or when a new feature requires tests from a newer commit
- Always verify pass rate does not regress after commit update
- Archive the previous commit's results for comparison

---

## 5. CI/CD Integration

### 5.1 Current CI Topology

| Workflow | Trigger | File | Jobs |
|----------|---------|------|------|
| CI | PR + push to main | `.github/workflows/ci.yml` | build, test, race, arch |
| Build | PR + push (duplicate) | `.github/workflows/build.yml` | build, test, race |
| Nightly | Daily 06:00 UTC | `.github/workflows/nightly-bench.yml` | test, fuzz, bench, gate |
| Release | Tag `v*` | `.github/workflows/release.yml` | 5-platform matrix + smoke |
| Security | Weekly Monday | `.github/workflows/security.yml` | go vet -all, race |

### 5.2 Test Allocation by Trigger

#### Every PR (fast feedback, < 5 min target)

| Test Suite | Layer | Current | Action Needed |
|-----------|-------|---------|---------------|
| `go test -count=1 ./...` | 0+1 | Yes | Keep |
| `go test -race ./...` | 0+1 | Yes | Keep |
| `go test ./internal/archtest/` | Infra | Yes | Keep |
| Visual parity (56 fixtures) | 2 | **No** | **Add** — requires headless goosie + Chromium in CI |
| Fuzz corpus (seed only) | 4 | Partial (runs with `go test`) | Keep — catches corpus regressions |

#### Nightly (thorough validation, < 30 min target)

| Test Suite | Layer | Current | Action Needed |
|-----------|-------|---------|---------------|
| Full test suite | 0+1 | Yes | Keep |
| Fuzz targets (120s each) | 4 | Yes (3 targets) | Keep; consider adding more targets |
| Benchmarks (warm/cold scroll) | Gate | Yes (M1 macOS) | Keep |
| Gate checks (v2-gate-check.sh) | Gate | Yes | Keep |
| WPT curated subset | 3 | **No** | **Add** — run WPT harness on curated dirs |
| Visual parity (56 fixtures) | 2 | **No** | **Add** as fallback if not in PR CI |
| Visual parity (20 live URLs) | 2 | **No** | **Add** — catch real-world regressions |
| Memory footprint gate | Gate | Yes | Keep |
| Scroll efficiency gate | Gate | Yes | Keep |

#### Release (confidence check, < 15 min target)

| Test Suite | Layer | Current | Action Needed |
|-----------|-------|---------|---------------|
| Full test suite (5 platforms) | 0+1 | Yes | Keep |
| Release smoke test | Integration | Yes | Keep |
| Visual parity (56 fixtures) | 2 | **No** | **Add** — validate rendering on release binaries |

### 5.3 Failure Policies

| Failure Type | PR | Nightly | Release |
|-------------|-----|---------|---------|
| Unit/integration test failure | **Block merge** | Alert within 1 hour | **Block release** |
| Race detector failure | **Block merge** | Alert within 1 hour | **Block release** |
| Architecture violation | **Block merge** | Alert within 1 hour | **Block release** |
| Visual parity regression | Alert (once in CI) | Alert within 4 hours | **Block release** |
| WPT pass rate drop > 2% | N/A (nightly only) | Alert within 4 hours | N/A |
| Fuzz crash | N/A (nightly only) | **P0 — investigate within 24h** | N/A |
| Gate budget miss (timing) | N/A (nightly only) | Alert within 4 hours | **Block release** |
| Gate budget miss (allocs) | N/A (nightly only) | **P0 — investigate within 24h** | **Block release** |
| Smoke test failure | N/A | N/A | **Block release** |
| Flaky test | Fix within 1 week | Quarantine after 3 flakes in 7 days | N/A |

### 5.4 Flaky Test Policy

1. **Detect:** A test that passes and fails on identical inputs across runs
2. **Quarantine:** After 3 failures in 7 nightly runs, add to quarantine list
3. **Fix:** Quarantined tests must be fixed or deleted within 2 weeks
4. **Restore:** Once fixed, run for 7 consecutive nightly passes before un-quarantining
5. **Known flaky:** `TestResponseTextSniffsCharsetFromMeta` — currently under observation

---

## 6. Test Infrastructure Needs

### 6.1 Tools to Build

| Tool | Purpose | Effort | Priority |
|------|---------|--------|----------|
| **Parity CI runner** | Run `testdata/parity.py` in CI with headless goosie + Chromium | 2-3 days | P0 |
| **WPT subtest reporter** | Report each WPT test as a named Go subtest for CI dashboards | 1-2 days | P1 |
| **WPT baseline tracker** | Store per-test pass/fail baselines, flag regressions | 2-3 days | P1 |
| **Flaky test detector** | Analyze nightly test history, auto-quarantine after N flakes | 2-3 days | P1 |
| **Coverage trend reporter** | Track coverage per package over time, flag regressions | 1-2 days | P2 |
| **Limit boundary tests** | Explicit tests for each `internal/engine/limits.go` constant at boundary | 2-3 days | P0 |
| **Rendering regression suite** | Categorized HTML fixtures targeting specific rendering bugs | Ongoing | P1 |
| **Concurrent document stress test** | Load multiple documents simultaneously, verify no races or limit leaks | 2-3 days | P1 |

### 6.2 Existing Infrastructure to Reuse

| Infrastructure | Location | What to Reuse |
|---------------|----------|---------------|
| **Gate test harness** | `test/gate/gate_test.go` | Full frame path wiring, headless window, worker pool, composer. Already provides `assertZeroAllocs`, `assertBurstAllocatesNothing`. |
| **M1 gate tests** | `test/gate/m1_gate_test.go` | Zero-alloc assertions, GPU linkage check, architecture boundary check |
| **Gate check script** | `scripts/v2-gate-check.sh` | JSON artifact parsing, budget assertion (mean, p99, startup, tiles). Exit codes: 0=pass, 1=miss, 2=bad args |
| **Release smoke script** | `scripts/release-smoke.sh` | Headless binary validation, frame path check, tile/worker verification |
| **Parity comparison** | `testdata/parity.py` | Pixel comparison algorithm (threshold=30, pass at 90%), band analysis with `--inspect` |
| **Live URL parity** | `testdata/parity_urls.py` | Real-world URL rendering comparison, `--snapshot` mode for deterministic baselines |
| **WPT harness** | `test/wpt/` (6 files) | Full WPT test lifecycle: checkout, serve, render, compare, report |
| **WPT curation** | `test/wpt/curate.go` | Feature-support model, directory mapping, skip classification |
| **Fuzz corpus** | `test/dom/`, `test/css/`, `test/style/` | Seed corpus for fuzz targets, 4000-input synthetic corpus in DOM parser |
| **Architecture tests** | `internal/archtest/` | Import boundary enforcement, cgo restriction, gofmt, mutable global detection |
| **CI workflows** | `.github/workflows/` | 5 workflow files covering PR, nightly, release, security |
| **Makefile targets** | `Makefile` | `build`, `test`, `test-race`, `bench`, `gate` |
| **Render fixtures** | `testdata/render/` | 56 HTML fixtures across 17 categories |

### 6.3 Effort Estimates

| Work Item | Effort | Dependencies |
|-----------|--------|--------------|
| Fix gofmt failure in `test/wpt/curate.go` | 10 min | None — unblocks 2 failing tests |
| Add parity tests to PR CI | 2-3 days | Chromium/Playwright in CI image |
| Add parity tests to nightly CI | 1 day | Parity CI runner |
| Add WPT suite to nightly CI | 1-2 days | WPT harness (exists) |
| Build limit boundary test suite | 2-3 days | None |
| Build concurrent document stress test | 2-3 days | None |
| Build WPT baseline tracker | 2-3 days | WPT harness |
| Build flaky test detector | 2-3 days | Nightly CI history storage |
| Build coverage trend reporter | 1-2 days | CI artifact storage |
| Add rendering regression fixtures | Ongoing | Parity CI runner |
| **Total initial investment** | **~15-20 days** | |

### 6.4 Immediate Action Items

1. **Fix gofmt failure** — Run `gofmt -w test/wpt/curate.go` to unblock `TestRepoIsGofmtFormatted` and `TestGate_ArchtestBoundaries`
2. **Investigate flaky test** — `TestResponseTextSniffsCharsetFromMeta` — add to quarantine watch list
3. **Add parity to nightly CI** — Lowest effort, highest value: add `testdata/parity.py` invocation to `nightly-bench.yml`
4. **Add WPT to nightly CI** — Harness exists; wire `TestWPTSuite` into nightly workflow
5. **Build limit boundary tests** — Explicit tests for each constant in `internal/engine/limits.go`

---

## Appendix: Test Inventory Summary

| Metric | Count |
|--------|-------|
| Go source files | ~101 |
| Go test files | ~161 |
| Test packages | 31 |
| Test functions | ~771 |
| Fuzz targets | 3 |
| Benchmarks | 5 |
| Render fixtures | 56 |
| Live parity URLs | 20 |
| CI workflows | 5 |
| Internal packages | 23 |
| Subsystems tested | 23 |

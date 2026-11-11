# SP-2 — V0 measurability: pinned oracles, committed goldens, honest gates

- Date: 2026-11-12
- Status: approved (user delegated autonomous execution without approval gates)
- Source of truth for V0: `docs/engine-compatibility-review.md` §6 V0 (lines 393-403)
- Predecessor: SP-1 (commits `d601ec1`, `0f99f2e`)

## Goal

Make every verification claim in this repo falsifiable: the oracle a test
compares against is pinned and asserted, at least one pixel baseline per
layout family is committed, no gate can report success while skipping its
own work, and the docs agree on which document is authoritative.

## Current state (audited 2026-11-12, HEAD 0f99f2e)

Already done — do NOT redo:

- `golang.org/x/sys@v0.48.0` pinned in `test/gatecheck/testdata/dependency-licenses.txt:23`,
  three-way agreement with `go.mod` and `go list -m`; per-target oracle in
  `licenses_test.go`.
- `TestManyQuerySelectorAll` race-scoped timeout (build-tag split 15s/45s,
  `test/engine/qsa_timeout_{race,default}_test.go`), no skip.
- Go 1.26.10 (`go.mod:3`); every CI job reads `go-version-file: go.mod`.
- macOS ARM64 CI job exists (`.github/workflows/ci.yml:49-84`), but runs only
  4 packages.
- testharness.js blanket skip is gone from code (`test/wpt/curate.go`,
  `test/wpt/testharness.go`); upstream commit pinned in
  `testdata/wpt-config.json:2`.
- Parity report records the Chromium build when `--render-chromium`
  (`testdata/parity.py:157,888-892`).

Open defects this spec closes:

1. **`make wpt` silently skips the suite.** `scripts/run-wpt.sh` never exports
   `WPT_RUN_SUITE=1`, so `TestWPTSuite` skips and the script exits 0 with
   "WPT run completed successfully." Verified locally 2026-11-12.
2. **Split-brain oracle, recorded but never asserted.** CI render parity pins
   `playwright==1.49.1` inline (Chromium 131.0.6778.33); Node scripts use
   `^1.63.0` (Chromium 153.0.8010.12). `pillow`/`numpy` unpinned. No code
   asserts the running oracle matches the pin.
3. **Zero committed PNG goldens** (`git ls-files '*.png'` → 0). No golden
   regression gate exists in `go test ./...`.
4. **`v2-gate-check.sh` wired nowhere** (review A36). AGENTS.md policy: timing
   budgets gate on the macOS nightly only — so the fix is a nightly workflow,
   not the per-PR CI.
5. **Race job can serve a cached green** (`ci.yml:47` lacks `-count=1`).
6. **Corpus floor un-ratcheted**: `MIN_SCORED_FIXTURES = 93` vs 105 actual
   fixtures (`testdata/parity.py:75`).
7. **Docs authority contradiction**: `docs/README.md:16,23` makes `docs/spec/`
   mandatory and demotes `roadmap-v2.md` to history; `AGENTS.md:19` says
   `roadmap-v2.md` is the current release-gate authority. README table also
   omits `SPRINT_PLAN_AND_WBS.md`. `engine-compatibility-review.md:344,354`
   are factually stale.

## Design decisions

### D1 — Pin and assert both oracles; keep the split deliberate

Unifying to one Chromium would force re-baselining 105 parity fixtures plus
4 known-gap bands, or re-recording every interaction reference. Each corpus
is already self-consistent with its own oracle. SP-2 makes both oracles
exact, asserted, and documented; unification is deferred to a later SP with
its own re-baseline budget.

- New `testdata/oracle-pins.json` — single source of truth:
  render oracle (python playwright 1.49.1 / chromium 131.0.6778.33) and
  interaction oracle (node playwright 1.63.0 / chromium 153.0.8010.12).
- New `testdata/requirements.txt`: `playwright==1.49.1`, `pillow==11.3.0`,
  `numpy==1.26.4` — the exact local versions, all installable on Python 3.9
  (local) and 3.12 (CI). CI installs `-r testdata/requirements.txt`.
- `parity.py` asserts the installed playwright version equals the render pin
  whenever `--render-chromium` is passed; mismatch exits 2 before rendering.
- `package.json`: `^1.63.0` → `1.63.0` (lockfile already exact).
- New `scripts/oracle.js`: `assertOracle(kind)` reads `oracle-pins.json`,
  compares `require('playwright-core/package.json').version`, throws on
  mismatch. Called by `scripts/interaction-parity-reference.js` and every
  `scripts/verify-phase*.js` (one require line each).
- New gatecheck test `TestOraclePinsAreConsistent`: requirements.txt,
  package.json, package-lock.json, and `interaction-parity/reference-meta.json`
  must all agree with `oracle-pins.json`. Fail-closed, no skip.

### D2 — Committed goosie goldens with embedded fonts

`GOOSIE_SYSTEM_FONTS=0` (`internal/raster/fonts.go:698`) makes the font set a
function of the binary alone — the documented escape hatch for byte-comparable
renders. Goldens are goosie's own output (regression ratchet), not Chromium
references, so they need no browser at test time and run in `go test ./...`
on every platform.

- New `testdata/render-goldens/` — one PNG per layout family, seven fixtures:
  block `basic/01-headings-paragraphs.html`, inline `basic/02-inline-elements.html`,
  float `floats/01-clear-directions.html`, flex `flex/01-flex-row.html`,
  grid `grid/01-basic-grid.html`, table `tables/01-basic-table.html`,
  positioning `positioning/02-absolute.html`. Selection recorded in
  `testdata/render-goldens/manifest.json` (fixture path, family, sha256).
- New `test/goldens/golden_test.go`:
  - builds `./cmd/goosie-headless` once (pattern: `test/entrypoints/limits_test.go:21`),
  - renders each manifest fixture with `GOOSIE_SYSTEM_FONTS=0` at 800x600 dpr 1,
  - decodes both PNGs and compares dimensions and every RGBA pixel; any
    difference fails with the first differing coordinate and a count,
  - writes the actual render to a temp dir on failure for inspection.
- Recording: `go test ./test/goldens/ -run TestRecordGoldens` (guarded by
  `GOOSIE_RECORD_GOLDENS=1`, otherwise skips) re-renders and rewrites the
  PNGs + manifest hashes. Deliberate engine changes re-record in the same
  commit, the same way the known-gaps ratchet works.
- One-pixel acceptance: `TestGoldenComparatorDetectsOnePixel` perturbs one
  pixel of a decoded golden in memory and asserts the comparator reports a
  difference. No fixture or golden file is modified.

### D3 — `run-wpt.sh` fails closed

Export `WPT_RUN_SUITE=1`; after the run, require `results.json` to exist and
`total > 0`, otherwise exit non-zero with "suite did not run". A skipped
suite can never again print "completed successfully".

### D4 — CI hardening + nightly workflow

- `ci.yml` race job: add `-count=1`.
- `ci.yml` darwin job: add `./test/engine/ ./test/gatecheck/` to the test
  list; install `jq` (brew) so the gate-check contract tests run instead of
  skipping. python3/numpy/PIL-dependent gatecheck tests may still skip on
  macOS; that is recorded, not hidden.
- New `.github/workflows/nightly.yml` (cron daily):
  - `macos-gate` on `macos-latest`: build, `make gate`,
    `scripts/v2-gate-check.sh /tmp/gate.json` with default budgets
    (mean 16ms, p99 33ms). Timing gates live here, per AGENTS.md policy.
  - `wpt-conformance` on `ubuntu-latest`: cache `/tmp/goosie-wpt` keyed by the
    pinned commit, build the binary, `WPT_RUN_SUITE=1 go test -timeout 40m
    -run TestWPTSuite ./test/wpt/`, upload `results.json` + `report.html`.
    Gated by the existing `min_pass_rate` ratchet (85.0 in
    `testdata/wpt-config.json`); the local baseline run decides whether the
    floor holds or must be re-measured (never lowered to green a red run —
    if the baseline is below 85, the floor stays and the job is expected red
    until engine work lifts it; recorded in the completion summary).

### D5 — Corpus floor ratchet

`MIN_SCORED_FIXTURES` 93 → 105 with a dated comment line, same style as the
existing ratchet log. `TestParityFixtureFloorMatchesCorpus` already polices
downward movement; raising the floor locks the 12-fixture gain.

### D6 — Docs authority

`AGENTS.md` is the user's instruction file and is not changed.
`docs/README.md` is aligned to it: `roadmap-v2.md` is the execution authority
(current status + ordered release gates); `docs/spec/` is the requirements
and architecture reference (SRS, vision, blueprint, WBS) that informs work
but does not override the roadmap's recorded state. The mandatory-language
line 16 is rewritten; `SPRINT_PLAN_AND_WBS.md` added to the table.
`engine-compatibility-review.md:344,354` corrected to the audited reality,
and the V0 checklist annotated with done/open status and evidence.

## Acceptance criteria

1. `bash scripts/run-wpt.sh` with no WPT_DIR runs the real suite; a forced
   skip path exits non-zero (contract tested).
2. At least one upstream testharness.js test passes through goja in the
   baseline run (evidence: results.json).
3. `parity.py --render-chromium` under a wrong playwright version exits 2
   before rendering (selftest); CI installs from requirements.txt.
4. `go test ./test/goldens/` passes on this machine with ≥7 committed PNGs;
   the one-pixel selftest proves the comparator catches a single pixel.
5. `go test ./...`, `go vet -all ./...`, race pass on affected packages,
   archtest, and `make parity` all green locally.
6. Docs no longer contradict AGENTS.md; stale review lines corrected.

## Limits (recorded, not hidden)

- No push is authorized: remote CI matrix green and the nightly jobs cannot
  be observed from this session. Local equivalents are the evidence; the
  workflows are reviewed by `actionlint` if available, else by inspection.
- The macOS gate budgets cannot be exercised natively here (no display);
  `v2-gate-check.sh`'s contract is already covered by `test/gatecheck`.
- Oracle unification (one Chromium for both corpora) is explicitly deferred.

## Commit shape

One commit per task (T1..T7), conventional prefixes, no squashing required
by this spec. Nothing pushed.
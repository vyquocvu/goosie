# TASK-060 — a cleared float does not clear (GOOSIE-FLOATCLEAR-001)

**Priority:** P1. **Method:** UNIT (float placement) + LIVE (csszengarden.com, zh.wikipedia.org, vnexpress.net).
**Owner:** integrator. **Defect:** `GOOSIE-FLOATCLEAR-001` (P1, found while attributing sweep #14; closed by `656fcc2`).
**Base:** combined tree `test/all-fixes` at `7ce3c11` (TASK-058 + TASK-059).
**Language:** English — every number here is quotable verbatim into the defect registry.

## 1. Reproduction

Found by pixel-diffing sweep #14 (`1b3dff93`, the combined TASK-058+TASK-059 build) against its
predecessor on `www.csszengarden.com`. The wrap fix (TASK-059) moved `.preamble` from X=845 — where it
was painted mostly outside the 1280-wide document — to X=0, but the page got *worse*: ink went from
738,632 to 1,000,392 and `pct_exact` from 68.49 to 58.74. A crop of the screenshot showed why: the
preamble was now painted on top of `.explanation`, two articles interleaved.

Offline layout dumps of the live document (`/tmp/zen-inline.html`, saved with its inline stylesheet),
1280x1024:

| box | pre-wrap (`7f71648c`) | wrap only (`1b3dff93`) | Chromium 1.63 |
|-----|----------------------|------------------------|----------------|
| `.summary` | (0,310) 653x121 | (0,310) 653x121 | (0,310) 845x188 |
| `.preamble` | (845,310) 653x292 | (0,499) 653x292 | (0,498) 845x325 |
| `.explanation` | (0,310) 653x344 | (0,310) 653x344 | (0,823) 845x425 |

Chromium stacks the three `float:left;clear:left;width:66%` blocks below each other; the engine ignored
`clear` on floats and started `.explanation` at its container's top. The live page's `.supporting` is
`display:inline` with block children, so even the engine's existing clearance support had no state to
work with there.

Minimal fixtures (`/tmp/clear-probe`, Chromium 1.63 at 800x600):

| fixture | engine before | Chromium |
|---------|---------------|----------|
| float 100x100, then float `clear:left` 100x100 | second at (100,0) | (0,100) |
| the same two inside `#wrap1`, a third `clear:left` inside sibling `#wrap2` | third at (0,0) | (0,200) |
| float, then `clear:left` float inside `span[display:inline]` holding a block child | (0,0) | (0,100) |
| float, `clear:left` float, then an uncleared float | (100,0) | (100,100) |
| five floats: `f`, `f c`, `f c` in a wrapper, `f c` in a second wrapper, `f` | (0,0)(0,100)(0,200)(0,300)(100,0) | (0,0)(0,100)(0,200)(0,300)(100,300) |

## 2. Cause

Three separate gaps in `internal/layout/block.go`:

1. The float arm of the block loop never called `clearanceFloor`; only in-flow block children did.
2. `floatBottomL/R` were local to one `blockInto` call. A plain block child re-started from zero, so
   `clear` inside it could not see floats held by an earlier sibling's subtree, although both boxes
   share one formatting context. The merge-up existed; there was no matching seed-down.
3. No rule 4 of CSS 2.1 §9.5.1: a later float could rise above an earlier float's margin-box top after
   `clear` had pushed that earlier float down.

## 3. Fix (`656fcc2`)

- Float placement applies `clearanceFloor(k.Style, floatBottomL, floatBottomR)` before the fit test and
  resets the cleared side's lead, so the new float starts at the container edge.
- `Object` gains `floatSeedL/R` and `floatTop`/`floatSeedTop`/`floatSeedHas`. Before `blockInto`, a
  plain block child (or an inline box holding block content, which layout blockifies) receives the
  parent's float bottoms and top in its own content coordinates; `blockInto` consumes the seed once.
  Floats inside that child merge back up, as before.
- A placed float records `floatTop`, and later floats are floored by it. The top seed is mapped the same
  way, so a sibling subtree's cleared float also stops a later float from jumping to the top.

## 4. Verification measured on this host

| check | result |
|-------|--------|
| `go test ./test/layout -run 'TestClearedFloat\|TestFloatAfter' -v` | 4/4 PASS |
| mutation M1: `if ...; false && f > floatY` on the float floor | rc 1: `cleared float Y = 0, want 100`, `cleared float X = 100, want 0`; test 2 and test 4 also red |
| mutation M2: `k.floatSeedL/R = 0` | rc 1: test 2 `Y = 100, want 200`, test 4 `Y = 0, want 100` |
| mutation M3: drop `\|\| blockifiedInline` | rc 1: test 4 `Y = 0, want 100` |
| mutation M4: disable `floatTop` floor and seed | rc 1: `float after a cleared float Y = 0, want 100` |
| restore after each mutation | `sha256sum -c` OK, tree byte-identical, tests green |
| `go vet -all ./...` | rc 0 |
| `go test -count=1 ./...` | rc 0, 43 packages ok |
| five-float fixture dump vs Chromium | all five boxes match: (0,0), (0,100), (0,200), (0,300), (100,300) |
| csszengarden.com, final binary, 1280x1024 dpr 1 | `UNFLAGGED`, pct **85.95**, ink **749,081** vs Chromium 755,020, coverage **0.992**, noise floor 0.0 — against wrap-only 58.74 / ink 1,000,392 / coverage 1.325 and pre-wrap 68.49 |
| `.preamble` / `.explanation` after fix | (0,499) / (0,823); Chromium (0,498) / (0,823) |
| zh.wikipedia.org/wiki/中国 (sweep corpus) | pct **65.82** (wrap-only 64.48, own pct noise 0.49), ink 266,070 vs Chromium 252,605, coverage **0.974** (wrap-only 1.13) |
| vnexpress.net | ink 267,822, pct 74.89, coverage 0.874 — unchanged from wrap-only 267,822 in the same-host run; no regression |
| vneconomy.vn | pct 69.8 vs wrap-only 70.46, ink 311,766 vs 281,371; this page's own double render in the wrap-only run reported Δcoverage **0.421** / Δpct **4.68**, so the delta is inside its noise. The changed rectangle is an image placeholder that paints in one run and not the other, not a layout shift |
| noaa.gov | **not verifiable in this session**: all three binaries (pre-wrap, wrap-only, final) returned `LAYOUT_EMPTY`, rc 0, doc_height 0 at the same minutes, with `AwsWafIntegration is not defined` in one run — the origin served a WAF challenge. The earlier same-day measurement was 762,080 ink on both branches |
| `make parity` on `test/all-fixes` at `656fcc2` (build + `testdata/parity.py --render --render-chromium`) | **93/93 PASS**, 0 failing, 0 errors, average **96.23%**, known gaps **5 fixtures / 10 bands** unchanged (9 shifted, 1 truly blank) — identical to TASK-059 §4 |

## 5. Registry

- `BR-259 | P1 | INT, PARITY | PASS` — `clear` on a float drops it below earlier floats in the same
  formatting context, across sibling subtrees and blockified inline boxes. Three oracles.
- `BR-260 | P1 | INT | PASS` — a float after a cleared float may not rise above its margin-box top.
  One oracle; Y only, see limits.
- Registered in `docs/qa-test-catalog.md`, `test/qc/traceability.golden.json`, and `// qc:BR-259` /
  `// qc:BR-260` markers; case count 257 → 259, p0/p1 tagged 132 → 134.

## 6. Limits disclosed, not hidden

- The width a float reserves on its line (`floatLead`/`floatTrail`) is still a scalar per side and is
  not merged across sibling subtrees. In the cross-subtree fixture the later float gets the correct
  Y=100 but starts at X=0 where Chromium puts it at X=100; BR-260's oracle asserts Y only and says so.
  The width model also does not track per-float vertical extents, so a float that ended above the
  current line still reserves its width until a wrap resets the side.
- Seeds are handed to children in the generic block path. Flex, grid and table containers lay their
  children out by their own algorithms and do not receive or merge this state.
- Only the margin-box top is carried; a mixed-extent left/right line (one side deeper than the other)
  is approximated by the per-side bottom scalars.
- The live csszengarden reference is a moving page (it serves theme names and a footer year via PHP);
  the comparison above used one saved copy for geometry and live runs for the score, both at the same
  minute. The page reported noise floor 0.0 across its two renders.

## 7. Sweep 15: 100 live pages on the final branch build

Run from a clean clone of `test/all-fixes` at `5be91b3`, binary SHA-256 `410274bd…`,
`vcs.modified=false`, 1280x1024 dpr 1. Report `docs/qa-live-parity/2026-10-26T0851Z-5be91b3c`:
100 pages, 94 two-sided (floor 75 met), no `GOOSIE_TIMEOUT` and no `BLANK_OUTPUT`. Sweep 14
(`1b3dff93`) is the same tree before TASK-060; each row below was compared against the page's own
rerender delta (global noise pct 16.49, coverage 0.389).

| page | sweep 14 -> 15 | attribution |
|------|----------------|-------------|
| csszengarden.com | pct 58.74 -> **85.95**, ink 1,000,392 -> 749,081, coverage 1.325 -> 0.992 | TASK-060 fix; matches §4 exactly |
| rfc-editor.org/rfc/rfc9110.html | ink **99,217 -> 82,260** (-16,957), pct 89.08 -> 89.85, blank bands 1 -> 0 | TASK-060 fix: the old build overlaid the RFC header metadata on itself. Reproduced A/B/A/B today: `1b3dff93` gives 99,217 ink in both runs, `5be91b3` 82,260 in both; the new build stacks RFC number, date and authors as a browser does |
| zh.wikipedia.org/wiki/中国 | ink 307,406 -> 266,070, coverage 1.121 -> 0.972, pct 64.33 -> **65.79** | TASK-060 fix; matches §4 (65.82 / 0.974) |
| vnexpress.net | ink 264,173 -> 267,822, pct 50.76 -> 67.10 | engine ink stable; Chromium moved 618,429 -> 363,680, so the pct change is reference drift (TASK-059; still `MISSING_REGIONS`) |
| zingnews.vn | `MISSING_REGIONS` -> `LOW_COVERAGE`, pct 52.93 -> 37.65 | engine ink unchanged (337,440 -> 337,526); Chromium painted 430,556 -> 646,013 and one more reference band, so coverage fell 0.622 -> 0.428 on the reference side |
| noaa.gov | `LOW_COVERAGE` -> `UNFLAGGED`, ink 189,444 -> 762,080 | the WAF challenge that blocked both binaries during §4 did not fire; 762,080 is the earlier same-day value on both branches, Chromium's ink identical (890,589) |
| vi.wikipedia / de.wikipedia | label flips with pct and coverage unchanged (66.72 -> 66.42 / 1.911 -> 1.871; 74.92 -> 74.96 / 0.719 -> 0.715) | band placement, no ink movement |
| en.wikipedia Cuneiform / Goat | Goosie ink identical or -1.6%; Chromium moved -29%..+34% | reference-side live content |
| target.com | `UNFLAGGED` -> `GOOSIE_CRASH`, rc 1 in 0.24 s, diag "downloaded to .../download" | the origin answered that one request as a download; both binaries render it identically today (rc 0, PNG 758,405 B, two runs each) |
| bestbuy.com | `FETCH_FAILED` in both runs | network |

No row shows engine-side loss of content or a new failure attributable to TASK-059/060; the two label
flips with an unchanged engine ink count are band/reference effects. The report is committed on
`test/all-fixes`.

## 8. Still owed

- Land on `main` together with TASK-058/059 and the other agent's WIP; the three closed entries
  (`GOOSIE-IMGLOAD-001`, `GOOSIE-FLOATWRAP-001`, `GOOSIE-FLOATCLEAR-001`) are already in
  `docs/qa-open-defects.json` on this branch, and the 100-site sweep above is from this branch — after
  the merge the sweep must be repeated from a stamped clean clone of the merged tree.
- `make parity` on the merged tree. It was run on `test/all-fixes` at `656fcc2` (93/93, see §4); the
  merge brings in unrelated layout changes (`internal/layout/inline.go` and neighbours), so it has to be
  re-run from the merged tree before landing.

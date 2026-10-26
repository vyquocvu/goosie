# TASK-059 — a float that does not fit walks off the document (GOOSIE-FLOATWRAP-001)

**Priority:** P1. **Method:** UNIT (float placement) + LIVE (vnexpress.net, the BLANK_OUTPUT row of sweeps #12 and #13).
**Owner:** integrator. **Defect:** `GOOSIE-FLOATWRAP-001` (P1, open in `docs/qa-open-defects.json`).
**Base:** `main` at `174a14a`; branch `fix/float-wrap`, fix commit `9c1849c`.
**Language:** English — every number here is quotable verbatim into the defect registry.

## 1. Reproduction

sweep #12 (`640ba5aa`) and sweep #13 (`7f71648c`) both score `https://vnexpress.net/` as `BLANK_OUTPUT`:
`ink_goosie 0` against `ink_chromium` 277,588 / 272,133. The page is not failing to load — rc 0 in 1.23 s,
one unrelated script error — it is painted entirely off the document.

`vnexpress.net` builds its body from consecutive `width:100%` floated sections (the site CSS has
`.section{width:100%;float:left}`). Pre-fix float placement appended every float to the cursor without
testing the remaining line space, so the first sat at X=0, the second at X=1280, the third at X=2560 and
so on: a layout dump of the pre-fix tree (`/tmp/screen-probe/vne-layout.txt`, taken from the integration
tree before this fix; the dump binary also carried an unrelated `screen` shim) shows
`header.section X=1280`, `section.wrap-main-nav X=2560`, `section.section X=3840`, `X=5120`, … . Only the
first section (a 1280x270 ad band) intersects the viewport; the body text is at X>=1280.

Minimal offline repro added as `TestFullWidthFloatsStackInsteadOfWalkingOffTheEdge`: a 400 px container
with `float:left;width:100%;height:50px`, `float:left;width:100%;height:70px` and
`float:right;width:100%;height:30px`. Pre-fix the second float is at X=400 and the right float at X=0,Y=0.

## 2. Cause

`internal/layout/block.go`, the float arm of `blockIntoInner`: `floatLead`/`floatTrail` grew by each
float's margin box with no fit test, and `k.Y` was the container's current cursor `y` for every float.
CSS 2.1 §9.5.2: if a float does not fit beside the floats already on the line, it is shifted downward
until it fits. The engine had no such shift.

## 3. Fix

Before placing, compute `floatY = y`; if `floatLead+outer+floatTrail > contentW`, move `floatY` to
`floatBottom` (the deepest float already placed, both sides) and reset `floatLead`/`floatTrail` to 0.
Place at `floatY` and record that float's bottom from `floatY`. Exactly filling the line is still
allowed (`>` not `>=`). No other layout path changes.

## 4. Verification measured on this host

| check | result |
|-------|--------|
| unit oracle, fixed | (0,0), (0,50), (0,120) |
| mutation: `if false && floatLead+outer+floatTrail > contentW` | FAIL: `second float at (400,0), want (0,50)` and `right float at (0,0), want (0,120)`; restore byte-identical, PASS |
| `go test ./test/layout ./internal/layout -count=1` | rc 0 |
| `go test -count=1 ./...` on the combined branch (`fix/float-wrap` + the Reflow fix) | rc 0, 43 ok |
| `go vet -all ./...` | rc 0 |
| `make parity` | 93/93 PASS, 0 failing, average 96.23%, known gaps 5 fixtures / 10 bands unchanged |
| vnexpress A/B, stamped clean builds, 1280x1024 dpr 1, `-screenshot` | `main 59be346`: rc 0, PNG 6,120 B, **345,600** non-white pixels of 1,310,720 (exactly the 1280x270 ad band). `fix/float-wrap d71e004`: rc 0, PNG 342,793 B, **621,938** non-white pixels |
| vnexpress.net in sweep #14 (stamped `1b3dff93`, 100 sites) | `MISSING_REGIONS`, ink 264,173 against Chromium 618,679 (coverage 0.386, pct_exact 50.42); sweep #13 at `7f71648c` was `BLANK_OUTPUT` with ink 0 |
| controlled A/B/A on this host, same minutes | `7f71648c` ink **0**, `1b3dff93` ink **268,005**, `7f71648c` ink **0** again; the Chromium reference itself moved 358,996 -> 618,679 -> 274,875, so the claim rests on ink_goosie leaving 0, not on a pct |

The 345,600 figure is the same number every run of the pre-fix build produced; the only painted region
is the first, non-floated ad band, which is what makes the blankness a layout displacement rather than a
decode or fetch failure.

## 5. Registry proposal

- `BR-257 | P1 | INT | PASS` — a float wider than the remaining line starts a new line below the floats
  already placed; full-width siblings stack instead of marching one viewport right per float.
  Oracle `TestFullWidthFloatsStackInsteadOfWalkingOffTheEdge`; mutation fails both the second and third
  float assertions.
- Defect `GOOSIE-FLOATWRAP-001`, owner this doc, closed by the fix commit once merged.
- Registered as BR-257 in `docs/qa-test-catalog.md`, `test/qc/traceability.golden.json`, and the
  `// qc:BR-257` marker on the oracle (commit `669c94c` on the combined tree; case count 257,
  p0/p1 tagged 132).

## 6. Still owed

- Landing on `main` (blocked while another agent holds `internal/layout/block.go` uncommitted); registry
  updated only after the merge.
- Sweep #14 finished from the stamped combined build `1b3dff9` in a clean clone: vnexpress.net is
  `MISSING_REGIONS` (ink 264,173 against 618,679, coverage 0.386, pct 50.42), and the controlled A/B/A
  above reproduces ink 0 -> 268,005 -> 0. The page is no longer blank but still not the reference. The
  report is committed on `main` as `docs/qa-live-parity/2026-10-26T0755Z-1b3dff93`.

## 7. Follow-up: the wrap exposed a missing `clear` (TASK-060)

Attributing sweep #14 pixel by pixel found one genuinely code-caused regression: `csszengarden.com`
went from ink 738,632 / pct 68.49 (pre-fix) to ink 1,000,392 / pct 58.74 with this wrap fix. The cause
was not the wrap. Once this fix moved `.preamble` back into the document at X=0, it overlapped
`.explanation`, because the engine never honoured `clear` on a float. Chromium stacks the three
`float:left;clear:left` articles at (0,310), (0,498) and (0,823); the engine had `.preamble` at (0,499)
and `.explanation` at (0,310).

TASK-060 (`656fcc2`) adds clearance to the float path, seeds the shared formatting-context state into
child subtrees (including blockified inline boxes), and applies §9.5.1 rule 4 to float tops. On the
final combined build the same page scores ink **749,081** / pct **85.95** / coverage **0.992** (noise
floor 0.0), with `.preamble` at (0,499) and `.explanation` at (0,823) — Chromium (0,498)/(0,823). The
wrap fix is not over-eager: it was correct, and it uncovered GOOSIE-FLOATCLEAR-001.

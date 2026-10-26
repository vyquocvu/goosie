# TASK-053 — generated boxes duplicate `box-shadow` (and the other copied paint layers) (GOOSIE-SHADOWRUN-001)

**Priority:** P2 (raise it only with a live-site measurement, not with a nicer argument). **Method:** INT (display-list oracles in `test/paint/`) + PARITY (one new scored fixture in `testdata/render/css-features/`).
**Owner:** agent tasked with TASK-053. **Defect:** `GOOSIE-SHADOWRUN-001`, which now carries two multipliers and names both.
**Base:** `main` at the commit that registers TASK-050 (check `git log -1`; do not branch off an older merge).
**Language:** English.

## 0. Rules

- **Do not touch** `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`,
  `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`,
  `testdata/parity.py`. Propose BR ids and test names in §8; add a fixture if you want one and report the
  corpus numbers instead of moving the floor.
- No `git push`, no merge, no PR. Measure everything you write, and correct this doc where your data disagrees.
- Do not edit `internal/style/style.go` below line ~4000 — TASK-050 is running against
  `extractBackgroundURL` (`:4655`) in the same file right now. Your changes belong near `AnonymousBlockStyle`
  (`:593`) and in `internal/paint/`.

## 1. Where this came from

TASK-049 collapsed the anonymous block's **background** layers (`style.AnonymousBlockStyle: s = *parent`
now zeroes all four), registered as BR-200/201/202 with the display-list and pixel oracles. Its §6.3 recorded
that the same assignment still **copies** `BoxShadow`, `TextShadow`, `BorderRadius`, `Mask*` and
`BorderImage`, and that `paintBoxShadow` (`internal/paint/builder_effects.go:14`, called from
`builder_box.go:162`) is not gated by box anonymity at all — so a block that generates anonymous boxes emits a
shadow per band.

Measured by the integrator on 2026-10-26 (`/tmp/t49-shadow/`, one translucent shadow layer, block split by
inline text): Goosie paints **21,363** shadow-band px where Chromium paints **1,001**, and **15,289** vs
**813** on the second band; interior non-white pixels 7,343 → 14,531, `(211,52,52)` 613 → 3,446, and a point
Chromium leaves white at `(180,90)` comes back `(211,52,52)`. That is the same doubling class as
`GOOSIE-INLINEBG-001` and `GOOSIE-ANONBG-001`, one property further along the copied struct.

## 2. First job: measure Chromium's rule per property, do not guess it

For each property below, build a document where a block with that property generates at least one anonymous
box (raw text beside a block child is the reliable generator) and record what Chromium actually paints:

| property | question to answer with pixels, not prose |
|----------|-------------------------------------------|
| `box-shadow` | one shadow for the element, or one per generated box? (predict: one — prove it) |
| `text-shadow` | anonymous boxes carry inline text: does the shadow apply to that text? (predict: yes — so zeroing it would be a bug, unlike the background case) |
| `border-radius` | inert without a background or border on the anonymous box, or does it clip something? |
| `mask-image` / `mask-*` | does a mask on the generating element apply once, or per box? |
| `border-image` | anonymous boxes have no border: does the slice reach them at all? |

Write the table into §9 with the commands and counts. **The fix must mirror the measured rule property by
property**; a blanket "zero everything the parent had" is how TASK-049 nearly shipped a `text-shadow`
regression, and it is a different defect from the one it fixes.

## 3. The fix, and the decision you have to justify

Two places can gate this; pick one per property and say why:

1. **Style layer** (what TASK-049 did for backgrounds): zero the property in `AnonymousBlockStyle`
   (`internal/style/style.go:593`), mirroring `AnonymousTableStyle:630`. Cheap, local, provable at the
   cascade, and it keeps `paint` anonymous-agnostic. Correct only for properties an anonymous box should
   never paint.
2. **Paint layer**: gate `paintBoxShadow` (and friends) on the box being anonymous. This needs layout to
   expose anonymity, which it currently does **not**: `flagAnonymous` is unexported and anonymous boxes get
   no `BoxKey`. If you take this route you must add an accessor in `internal/layout` and keep the arena's
   invariants; say what you exposed and why paint cannot avoid needing it.

Do not fix `paintBoxShadow`'s unrelated ungatedness (inset layers, multiple layers, opacity interaction) — if
you find a second shadow bug while measuring, register it in §9 as a proposal instead.

## 4. Oracles

- **Display list:** per declaring element, exactly N shadow commands where N is the number of layers it
  declares, with anonymous boxes contributing 0 — the shape of
  `test/paint/anonymous_block_background_test.go` (BR-201) is the model, including the two-nested-elements
  case and the shorthand case.
- **Pixels:** rasterize a fixed tile through `raster.RasterizeTile` on a real `Session.PaintChecked` display
  list and sweep the whole block rather than cherry-picking points, the way BR-202 does; derive thresholds
  from the Chromium measurement of the same document, and write the pre-fix number next to the threshold so
  nobody can pass by accident.
- **`text-shadow` in the other direction:** if §2 shows the anonymous box's text *should* carry the shadow,
  that needs an oracle too, so a future blanket-zero "cleanup" cannot silently reintroduce this defect's
  opposite.

## 5. Mutations

Report failures killed and the restore rc. At minimum: revert the fix per property (each must die); make the
gate apply to `text-shadow` as well if §2 says it must not (the text-shadow oracle must die); move the gate
into `paint` without exposing anonymity correctly (a control must fail loudly, not silently).

## 6. Out of scope

Background layers (closed by TASK-049), `url()` parsing (TASK-050), anything in `internal/js` (TASK-047/051/052
own that package and are queued), and the known gaps in `testdata/render-known-gaps.json` — a new fixture must
not move a gap band to make itself pass.

## 7. Acceptance

1. `go test -count=1 ./...`, `go test ./internal/archtest/`, `go vet -all ./...`, `gofmt -l` clean; timings and exit codes.
2. §2's table measured, and the shipped behaviour equal to it per property.
3. The §1 numbers (21,363 / 15,289) re-measured on your host before the fix and after it, with the after-side
   compared against Chromium's 1,001 / 813 — a reduction that stops short of the reference is a partial fix
   and must be reported as one.
4. Corpus parity re-run (`89/89`, average `96.17%`, known gaps `5 fixtures / 10 bands` as of the integrator's
   last pass): no fixture lost, no band moved, and the new fixture's own score reported.
5. §8 proposes BR ids continuing from the catalog total you read (report it; TASK-050 may already have taken
   some — take the next free ones and let the integrator renumber).
6. §9 states what you did not measure.

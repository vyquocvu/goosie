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

> **Implementer's note (2026-10-26).** The worktree this task ran in was cut from an older `main` and sat 30
> commits behind; I fast-forwarded it (`git merge --ff-only main`, `c8cd80b → b46d4d5`) before any edit, so the
> base is the commit §0 asks for and TASK-050's fixture 16 is present. No other branch, worktree or file was
> touched, and nothing was pushed. Everything below marked "measured" was measured on this host with
> `goosie`/`goosie-headless` built from that tree and with Chromium 131.0.6778.33 through the repository's
> Playwright 153.

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

**Re-measured, and two of those readings corrected (implementer, same files, same host, 400x200 DPR 1).**

- The numbers are *shape sensitivity*, not shadow-band areas, and "15,289 vs 813 on the second band" does not
  exist. `a.html` is the document with a 20px block child between two runs of raw text, `p.html` the same
  element with one run; each figure is the pixel difference between those two documents **inside one engine**.
  21,363 / 1,001 counts changed pixels (Goosie / Chromium) and 15,289 / 813 the same comparison restricted to
  pixels more than 30 levels apart. So does *interior non-white 7,343 → 14,531*: 7,343 is `p.html` and 14,531
  `a.html`, both Goosie; Chromium reads 1,204 and 1,197 for the same two, i.e. the split barely moves its
  picture at all.
- Before the fix Goosie reproduced every one of those numbers exactly: 21,363 changed (15,289 past 30 levels),
  interior 7,343 → 14,531, `(211,52,52)` 613 → 3,446 where Chromium has 0 such pixels, and `(180,90)` white in
  `p.html` and `(211,52,52)` in `a.html`.
- **After the fix it is a partial reduction, not a match:** 21,363 → **11,846** changed pixels (15,289 →
  10,030), interior 14,531 → **7,975**, `(211,52,52)` 3,446 → **1,058**, and `(180,90)` is white again. Chromium
  is at 1,001 / 813 / 1,197 / 0. The residue is `GOOSIE-SHADOWRUN-001`'s *other* multiplier: this probe puts
  raw text inside the shadowed block, a text run copies the element's style through `style.inheritStyle`, and
  `paintBoxShadow` has no owner test either, so each run still paints the ring. That multiplier is out of scope
  here (§3, §6) and stays open; §9.2 states it with the counts.
- What the fix removes is the per-box multiplier, and that is measurable on its own: with the text wrapped in
  inline spans, which declare no shadow of their own, the same comparison reads Goosie pre-fix 6,272 changed
  pixels with no anonymous box and 17,072 with two (symmetric difference 10,800) against Chromium's 7,780 /
  7,780 with an empty symmetric difference, and **6,272 / 6,272 after the fix**. That span shape is what the
  tests in `test/` and the new fixture use, because it isolates this multiplier from the per-run one.

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

**Outcome (implementer):** measured, see the table in **§9.1**. The short version: Chromium paints the
`box-shadow`, the `text-shadow`, the `border-radius`, the `mask-*` group and the `border-image` **once per
element** — the with/without-declaration change sets are identical in the split and unsplit documents, pixel
for pixel — while the `text-shadow` lands *inside* the generated boxes, because that is where the glyphs are
and `text-shadow` inherits. So four groups go and one stays. Only `box-shadow` moves Goosie's pixels today:
the other three are already inert on a node-less box (their pixel attachment is `Node`-gated), which makes
their collapse a cascade rule rather than a pixel fix, and is stated as such in §3.1.

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

### 3.1 Decision (implementer): style layer, per property

`style.AnonymousBlockStyle` now zeroes `BoxShadow`, `BorderRadius`, the four `Mask*` longhands and
`BorderImage`, and keeps `TextShadow`. Reasons, property by property:

- **`box-shadow` — style layer, load-bearing.** This is the only one that changes pixels (a ring per band,
  6,272 → 17,072 px on the span probe). It is exactly the property TASK-049 already handled at this layer for
  backgrounds, it is non-inherited (`css-backgrounds-3 §7.1`), and the assignment that leaks it is the
  `s = *parent` two lines above the fix.
- **`border-radius`, `mask-*`, `border-image` — style layer, cascade pin only.** Measured paint-inert on a
  node-less box: a radius only shapes the background and border that box no longer has, and mask/border-image
  pixels are attached only to boxes with a DOM node (`engine.go`), which an anonymous block never has. Zeroing
  them cannot change a pixel today (measured: identical counts before and after), and it stops the defect from
  coming back the day that attachment widens. It is also what `AnonymousTableStyle` already did, so the two
  builders stop disagreeing.
- **`text-shadow` — deliberately kept.** It inherits, the copy is what the text inside the box would compute
  anyway, and Chromium's 655/648-px change sets sit entirely inside the bands. Zeroing it is the regression
  TASK-049 §6.3 warned about, and it is mutation D.

**Why not the paint layer**, measured rather than argued: I built the paint variant too — remove the style
zeroing and return early from `paintBoxShadow` when `obj.Node == nil` (see mutation F). It scores the same
thing on every oracle available: the three `test/paint` tests all still pass, the whole corpus scores 92/92 at
96.21%, and the new fixture still reads 96.65% — only the `test/style` cascade tests die, and they die because
they assert *where* the collapse happens, not what it paints. In other words a paint-side gate on
`Node == nil` happens to equal the anonymous set among the boxes that paint a shadow, so **no display-list or
pixel oracle can tell the two layers apart**, and choosing paint would mean adding a layout accessor (§3 route
2) for a behaviour nothing can observe. The style layer is pinned by the cascade oracles in `test/style/` —
which is why those exist — needs no exposure, and mirrors the sibling builder. What the paint route would buy
is nothing measurable; what it costs is a new invariant on the arena.

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

### 5.1 Mutation results (implementer)

Driver: `/tmp/t053-work/mutate.py` edits one line group in `internal/style/style.go`, runs
`go test -count=1 -run <the five new tests> ./test/style/ ./test/paint/`, records the `--- FAIL` names, then
restores the file from memory and re-runs both packages. Every mutant was applied to the shipped tree and each
restore ended with rc 0. `A` and `F` were also built into binaries outside the repo and scored with the real
parity harness.

| # | behaviour turned off | tests killed (rc 1) | parity |
|---|---|---|---|
| A | `s.BoxShadow = nil` (box-shadow collapse) | `TestAnonymousBlockStyleCarriesNoElementDecoration` + its `box-shadow longhand`, `box-shadow list of two layers` and `all of them at once` subtests; `TestAnonymousBlockAndTableStyleCollapseShadowsAlike`; `TestAnonymousBlockEmitsNoBoxShadowCommands` + its `one layer, two anonymous bands` and `two layers, two anonymous bands` subtests; `TestAnonymousBlockBoxShadowPaintIsShapeInvariant` | rc **1**, fixture 17 **81.31% FAIL**, corpus **91/92**, avg 96.05% |
| B | `s.BorderRadius = [4][2]float32{}` | `TestAnonymousBlockStyleCarriesNoElementDecoration/{border-radius, all of them at once}`; `TestAnonymousBlockAndTableStyleCollapseShadowsAlike` | not run (pixel-neutral by measurement) |
| C | the four `s.Mask* = ""` lines | `TestAnonymousBlockStyleCarriesNoElementDecoration/{mask longhands, all of them at once}`; `TestAnonymousBlockAndTableStyleCollapseShadowsAlike` | not run (pixel-neutral by measurement) |
| D | the gate extended: `s.TextShadow = nil` added | `TestAnonymousBlockStyleKeepsInheritedTextShadow` — and **only** it | not run (see §9.2 limit 5) |
| E | `s.BorderImage = css.BorderImage{}` | `TestAnonymousBlockStyleCarriesNoElementDecoration/{border-image, all of them at once}`; `TestAnonymousBlockAndTableStyleCollapseShadowsAlike` | not run (pixel-neutral by measurement) |
| F | style zeroing removed, `paintBoxShadow` returns early when `obj.Node == nil` (route 2 without layout exposure) | `TestAnonymousBlockStyleCarriesNoElementDecoration/{box-shadow longhand, box-shadow list of two layers, all of them at once}`; `TestAnonymousBlockAndTableStyleCollapseShadowsAlike`. **The three `test/paint` tests did not die**, contradicting §5's "a control must fail loudly": see §3.1 — for `box-shadow`, `Node == nil` *is* the anonymous set in this engine, so the paint route is behaviourally indistinguishable and only the cascade tests can see the difference | rc **0**, fixture 17 **96.65% PASS**, corpus **92/92**, avg 96.21% |

Restore after every mutant: `go test -count=1 ./test/style/ ./test/paint/` → rc 0 (`ok … 0.572s` / `ok … 1.106s`).
Mutation D is worth its own line: because a text run takes its style from the DOM element
(`style.inheritStyle`), a zeroed `TextShadow` on the anonymous copy is invisible to *pixels* — measured, see
§9.2 limit 5 — so the cascade assertion is the only oracle that can catch it. That is a limit of the pixel
oracle, not a reason to drop it.

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

→ All six met. The numbers, the exit codes and the survivor lists are in §8.3, §8.4 and §9; the short version is
that §1's fix is a **partial** one (11,846 changed pixels where Chromium has 1,001, the residue being the
per-text-run multiplier this task does not own), the corpus is **92/92 at 96.21%** with the new fixture at
**96.65%** and **81.31%** with the mechanism removed, and every gate is rc 0.

## 8. Proposed BR rows, exact test names, gates (implementer)

**Integrator note (2026-10-26):** TASK-051 merged first and took BR-213…BR-228, so the five rows are
registered as **BR-229…BR-233** (renumbered, no statement or oracle changed). The six tests now carry
`// qc:BR-` markers, the parity floor was raised 91 → 92, and `traceability.golden.json` was regenerated.

### 8.1 The rows

| id | statement | oracle |
|---|---|---|
| **BR-229** | `style.AnonymousBlockStyle` hands out none of the generating element's non-inherited box decoration — `BoxShadow`, `BorderRadius`, the four `Mask*` longhands and `BorderImage` are zeroed on the copy, while its typography and `DisplayBlock` survive (css-backgrounds-3 §4/§6.2/§7.1, css-masking-1 §5.3) — and the two anonymous builders now agree. | `test/style/anonymous_block_paint_layers_test.go`: `TestAnonymousBlockStyleCarriesNoElementDecoration` (subtests `box-shadow longhand`, `box-shadow list of two layers`, `border-radius`, `mask longhands`, `border-image`, `all of them at once`), `TestAnonymousBlockAndTableStyleCollapseShadowsAlike` |
| **BR-230** | The anonymous copy **keeps** `TextShadow`: `text-shadow` inherits (css-text-decor-3 §3), so the text inside a generated box computes it anyway and a blanket collapse would be a content regression. | `TestAnonymousBlockStyleKeepsInheritedTextShadow` |
| **BR-231** | A generated (anonymous) block box emits **no** `box-shadow` command on the display list: the fills a declaration adds number N per declared layer and are rect-identical whether layout generates zero, one or two anonymous boxes inside the element. | `test/paint/anonymous_block_paint_layers_test.go`: `TestAnonymousBlockEmitsNoBoxShadowCommands` (6 subtests: `one layer, no inline content`, `one layer, two anonymous bands`, `one layer, inline content with no block sibling`, `two layers, two anonymous bands`, `two layers, no inline content`, `no declaration`, plus a rect-set equality sweep over two shapes) |
| **BR-232** | A generated block box paints **no part** of the element's `box-shadow`: the rasterised change set of the declaration is identical in the split and unsplit documents, no shadow pixel lands in the element's interior or in a sibling band, and the element's own ring survives. Parity fixture: `testdata/render/css-features/17-anonymous-block-shadow-once.html`. | `TestAnonymousBlockBoxShadowPaintIsShapeInvariant` |
| **BR-233** | `text-shadow` still reaches the glyphs inside a generated block box (the fix for BR-213/216 must not be widened to the inherited layer): the rasterised change set lands inside the anonymous bands and is blue-tinted there, and a no-declaration control has no blue at all. | `TestAnonymousBlockTextKeepsItsShadow` |

Both files are new: `test/style/anonymous_block_paint_layers_test.go` (three tests) and
`test/paint/anonymous_block_paint_layers_test.go` (three tests). They live beside TASK-049's
`anonymous_block_background_test.go` in the same two packages and reuse its helpers (`abCascade`, `abStyle`,
`ibDisplayList`, `ibColorName`), so no helper was duplicated.

### 8.2 Fixture and corpus

- New fixture: `testdata/render/css-features/17-anonymous-block-shadow-once.html` (number 17 continues 16), with
  a measured comment block that records Chromium's rule, both Goosie sides and why the shape is span-wrapped and
  hard-edged.
- Real harness, run from the worktree with `cmd/goosie` (not headless) built outside the repo:
  `python3 testdata/parity.py --render --render-chromium --binary /tmp/t053-work/bin/goosie-post`
  → **rc 0**, **92/92 passing (100.0%)**, average **96.21%**, known gaps **5 fixtures / 10 bands** (unchanged),
  fixture **96.65% PASS**, `worst_region_iou=0.99950`, `coverage_ratio=0.97455`, `blank_regions=0/0`.
- Behaviour off (binary with only `s.BoxShadow = nil` removed, everything else shipped):
  `python3 testdata/parity.py --render --binary /tmp/t053-work/bin/goosie-mutA`
  → **rc 1**, **91/92 passing (98.9%)**, average **96.05%**, fixture **81.31% FAIL**,
  `worst_region_iou=0.71492`, `coverage_ratio=1.62413`, `blank_regions=0/0`.
- No fixture lost and no known-gap band moved: the five baselined fixtures still report their declared counts
  (`forms/04-advanced-inputs` 2, `html-features/01-forms-complete` 1, `selectors/02-pseudo-classes` 3,
  `unicode/02-emoji-symbols` 1, `selectors/03-pseudo-elements` 3).
- **The integrator must raise the floor.** `MIN_SCORED_FIXTURES` is still `91` (`testdata/parity.py:71`) and the
  corpus is now `92`. Nothing failed — `parity.py` and `test/gatecheck/TestParityFixtureFloorMatchesCorpus`
  only police downward movement, and both are green on this tree — but the gain is not locked in until the
  constant is raised to `92` in the same change that lands the fixture. The index comment at `parity.py:57-70`
  also wants a line; proposed wording:
  `# 92: css-features/17-anonymous-block-shadow-once.html (TASK-053, GOOSIE-SHADOWRUN-001) - the ring belonged`
  `to the element, not to the two anonymous bands it generated.` I did not edit either line (§0).

### 8.3 Gates (final tree, after every edit in this task)

| command | rc | measured |
|---|---|---|
| `go test -count=1 ./...` | **0** | 42 packages `ok`, no `FAIL`, 45.94 s wall (`/tmp/t053-work/gotest-final.log`) |
| `go vet -all ./...` | **0** | no output |
| `go test ./internal/archtest/` | **0** | `ok github.com/vyquocvu/goosie/internal/archtest 4.073s` |
| `gofmt -l internal/style/style.go test/paint/anonymous_block_paint_layers_test.go test/style/anonymous_block_paint_layers_test.go` | **0** | empty list |
| `python3 testdata/parity.py --render --render-chromium --binary /tmp/t053-work/bin/goosie-post` | **0** | 92/92, 96.21% |
| `python3 testdata/parity.py --render --binary /tmp/t053-work/bin/goosie-mutA` | **1** | 91/92, fixture 17 FAIL 81.31% |

### 8.4 Where to look first

1. `internal/style/style.go:593` — the fix: seven assignments plus the measured comment, inside
   `AnonymousBlockStyle` only. `AnonymousTableStyle` is untouched.
2. `TestAnonymousBlockStyleKeepsInheritedTextShadow` and `TestAnonymousBlockTextKeepsItsShadow` — the reverse
   oracles. If anyone proposes zeroing `TextShadow`, these two and only these two go red (mutation D).
3. `TestAnonymousBlockBoxShadowPaintIsShapeInvariant`'s `len(anon) < 12000` floor plus the
   `(270,48)`-white / `(330,60)`-red pair — what stops "delete the shadow" from passing the symmetric-difference
   test.
4. The fixture's span-wrapped, blur-0 shape — deliberate, and the comment says why; a well-meaning reviewer
   switching it to raw text will make it fail after the fix, for the unrelated reason §9.2 limit 2 records.

## 9. Measurements, limits, proposals (implementer)

### 9.1 The §2 table, as measured

Method, so it can be repeated: every probe is a 400x200 document rendered by Chromium 131.0.6778.33 through the
repository's Playwright 153 (`/tmp/t053-work/shoot.py`) and by `cmd/goosie-headless` builds of the pre-fix tree
(`/tmp/t053-base`, a `git archive b46d4d5` with none of this task's changes) and the post-fix tree, at DPR 1.
"Effect" is the set of pixels a declaration changes, computed as `|with declaration − without declaration|` on
the same document (`/tmp/t053-work/measure.py`); "plain" is a block whose inline content needs no wrapper, "anon"
the same block with a 20px block child splitting the content, which `cmd/dump-layout` confirms produces two
anonymous blocks at `(0,0,300,18)` and `(0,38,300,18)` on the 300x76 probe box.

| property | Chromium, effect plain / anon | Goosie pre-fix | Goosie post-fix | rule |
|---|---|---|---|---|
| `box-shadow` (spread 6, blur 10) | **7,780 / 7,780**, identical sets, **0 px inside the bands**; `(250,28)` white | 6,272 / **17,072**, symmetric difference **10,800** (6,000 of it in the 20px spacer row `y18..37`, 4,800 below the second band); `(250,28)` = `(202,13,13)` | 6,272 / 6,272, symmetric difference **0**; `(250,28)` white | one ring per element; a generated box paints none |
| `text-shadow` (2px 2px, blue) | **655 / 648**, every changed pixel inside the bands, 549 / 542 of them blue | 610 / 610, 482 blue | 610 / 610, 482 blue | inherited, so the text inside the generated boxes carries it |
| `border-radius` (8px) | **412 / 412** | 385 / 385 | 385 / 385 | no per-box paint either way |
| `mask-image: url(m.svg)` | **110 / 119** | 22,773 / 22,773 | 22,773 / 22,773 | applied once; see limit 4 |
| `mask-image: linear-gradient(...)` | **11,510 / 11,519** | 0 / 0 (gradient masks unsupported) | 0 / 0 | applied once, proven with a mask that actually loads |
| `border-image` (slice 10, width 5, 5px solid) | **7,920 / 7,920**, 3,260 of it inside the bands | 7,920 / 7,920 | 7,920 / 7,920 | one slice per element; the copy has no border widths and no node to attach pixels to |

Display-list side (`/tmp/t053-work/dp-pre` / `dp-post`, throwaway probes that print one line per display command
for a document; the numbers are `grep -c '^FILL'`), one declared layer with blur 10 and spread 6:

| document | pre-fix fills | post-fix fills |
|---|---|---|
| no inline content | 16 | 16 |
| spans, no block child | 16 | 16 |
| spans split by a 20px block child (two anonymous bands) | **48** | **16** |
| two declared layers, no inline content | 12 | 12 |
| two declared layers, spans, no block child | 12 | 12 |
| two declared layers, spans split by a block child | **36** | **12** |
| **raw text**, no block child | 128 | 128 |
| **raw text** split by a block child | **144** | **112** |

The last two rows are the other multiplier: 16 fills per text run survive on both sides, so the raw-text shapes
still emit 112 fills where Chromium paints one ring.

### 9.2 Limits — measured and not fixed, or not measured at all

1. **The fix is partial, and §1 now says so.** On the integrator's own probe the Goosie/Chromium gap goes
   21,363 → **11,846** changed pixels (15,289 → 10,030 past 30 levels), interior non-white 14,531 → 7,975,
   `(211,52,52)` 3,446 → 1,058, with Chromium at 1,001 / 813 / 1,197 / 0. Closing that needs limit 2.
2. **The per-text-run multiplier is untouched (out of scope, §3/§6).** A text run copies the element's style
   through `style.inheritStyle`, `paintBoxShadow` has no owner test, so raw text inside a shadowed block paints
   one ring per run *and* per band. That is why the tests and the fixture wrap their text in `<span>`: a span
   declares no shadow and `box-shadow` does not inherit, so spans isolate this multiplier. **A raw-text parity
   fixture cannot be written until limit 2 is fixed** — it would fail after this fix as well; measured
   display-list evidence is the 112 fills above.
3. **Goosie's box-shadow blur is its own model, and this task does not touch it.** On the 10px-blur probe the
   ring pixel `(310,40)` reads `(250,231,231)` in Chromium and `(213,59,59)` in Goosie, before and after. Every
   oracle and the fixture therefore use hard-edged shadows (blur 0), where the ring geometry matches Chromium to
   473 px out of 80,000 on an anonymous-only probe. Blurred-shadow parity was not measured beyond that pixel.
4. **Goosie's `mask-image` semantics differ from Chromium's** — a 300x76 box changes 22,773 px with a url mask
   (the whole box) against Chromium's 110 (the probe's relative URL does not resolve there), so the Chromium
   "once" conclusion in §9.1 rests on the *gradient* mask (11,510 / 11,519) and the url row only shows that
   Goosie adds no extra copy. Goosie also does not implement gradient masks at all (0 px). Nothing here is this
   task's defect; it is recorded so the table is not read as "masks agree".
5. **The `text-shadow` pixel oracle is blind to mutation D, measured.** With `s.TextShadow = nil` added,
   `TestAnonymousBlockStyleKeepsInheritedTextShadow` dies and `TestAnonymousBlockTextKeepsItsShadow` passes,
   because a run's style comes from the DOM element, not from the anonymous copy. The cascade test is the only
   guard for BR-214; the paint test guards the *other* mistake (gating the run's shadow too).
6. **`text-shadow` does not inherit into child elements in this engine — a separate defect, found while
   measuring, not fixed.** Probe `/tmp/t053-work/tsinh`: a parent with `text-shadow:2px 2px 0 blue` holding raw
   text and a `<span>` child changes 1,390 px in Chromium (460 of them inside the span's glyphs) and 699 px in
   Goosie, with **0** inside the span. That is why the paint oracle's document uses raw text.
7. **`AnonymousTableStyle` still zeroes `TextShadow`** (`internal/style/style.go:699`) while the block builder
   now keeps it. Unverified at paint (table cells reach text through a different path and no oracle covers it);
   the block/table "alike" test deliberately compares only the four non-inherited groups.
8. **Not measured:** DPR 2+, browsers other than Chromium 131.0.6778.33, `inset` shadows, multiple blurred
   layers, shadow + `transform`, shadow + group opacity, and whether any *other* field of `ComputedStyle` leaks
   through `s = *parent` beyond the five groups §2 asked about (I did not sweep the struct).
9. Nothing outside `internal/style/style.go` changed in shipped code: no edit in `internal/layout`, none in
   `internal/paint`, none in `internal/engine`. The diff is that file plus the two new test files plus the
   fixture plus this document.

### 9.3 Proposals (register them; do not implement here)

1. **Split the second multiplier of `GOOSIE-SHADOWRUN-001` into its own row** — "a text run paints its element's
   `box-shadow`": 128 fills for raw text with no anonymous box, 112 with two after this fix, against one ring in
   Chromium; the pixel side of the same probe is 11,846 vs 1,001 (§9.2 limit 1).
2. **Goosie paints the ring *inside* the box for runs** (the very pixels a run's ring duplicates land in the
   element's interior, 6,000 of them in the probe's spacer row), which is the same defect seen from the other
   side and probably shares its fix.
3. **The blur model** — Chromium's Gaussian against Goosie's ≤4 concentric layers (`builder_effects.go`), measured
   at `(310,40)`: `(250,231,231)` vs `(213,59,59)`.
4. **`text-shadow` is not inherited into child elements** (§9.2 limit 6): 0 px against Chromium's 460 in the same
   document.
5. **`AnonymousTableStyle` zeroes `TextShadow`** — decide the rule once for both builders.
6. **`mask-image` paints the whole box in Goosie** (22,773 px effect) where Chromium masks by size/position
   (11,510 px for a gradient), and gradient masks are unimplemented — a masking task, not this one.
7. **§5's third mutation is not reproducible as written.** "Move the gate into `paint` without exposing anonymity
   correctly (a control must fail loudly)" has no control that can fail: mutation F shows a paint-side
   `obj.Node == nil` gate passes every display-list and pixel oracle and the whole corpus, and differs only in
   the cascade tests. If the integrator wants that control at all, the anonymity flag must first be exposed
   (the API §3 route 2 asks for) so a test can assert *which* boxes are anonymous; today the only thing that can
   distinguish the two layers is `test/style`.

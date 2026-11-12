# SP-7: Inline Border Fragmentation — Design

**Status:** approved (standing autonomous delegation; no approval gates). Commits on `main`, **no push**. Chat in Vietnamese; this doc in English per repo convention.

**Sub-project:** SP-7 is the first evidence-driven engine feature after SP-6 (multi-window) shipped. It is one bounded vertical slice — inline-element border fragmentation — chosen from the QC defect ledger and re-verified against HEAD. L3/L4 incremental invalidation stays deferred (SP-7+); group-opacity compositing is a separate later sub-project.

## 1. Terrain and evidence (re-verified against code, not the ledger)

The QC ledger `docs/qa-open-defects.json` is **partly stale**. Before scoping, every candidate was checked against the current tree:

- **`GOOSIE-PSEUDO-001` ("::first-line is not supported at all") is STALE — already implemented.** `internal/style/firstline.go` (`filterFirstLineProperties`), `internal/style/pseudo.go:37,42-48,98,103-105` (admission in both resolve copies), `internal/layout/arena.go:293-295,480-487` (`FirstLineStyles` map + extraction), `internal/layout/inline.go:466-473,1516-1585` (`applyFirstLineRuns` overrides font/color/background/text-decoration/text-transform/letter-spacing/word-spacing/line-height on `lines[0]` and remeasures). `engine/pseudo_style_provider_test.go` and `js/inner_text.go:265-275` consume it. **Excluded from SP-7.** (Ledger re-measure/closure is separate docs hygiene, not this feature.)
- **L3/L4 (incremental style/layout invalidation) stays deferred.** SP-5 terrain (`docs/superpowers/specs/2026-11-12-sp5-flow-root-and-paint-identity-design.md:29-49`, roadmap `:197,:212`): hard behavioral assertions (`test/engine/refresh_test.go:217-222,240-245` demand `LayoutBuilds+1` and `Arena != origArena`), no per-rule `Deps` (`internal/style/matcher.go:37-51`), every reflow a full `layout.Build` (`internal/engine/engine.go:1885-1895`), `Refresh` partial branch a full-pipeline stub (`internal/engine/refresh.go:65-88`), `Plan.Subtree` recorded but unused, no cross-reflow `BoxKey→BoxSlot` persistence. "No foundation yet to measure the incremental benefit." Multi-sprint architecture — not a single-plan feature.
- **`GOOSIE-COMPOSITE-001` (group opacity double-darkening) is CONFIRMED open** (`internal/raster/tile.go:74,76,78,210-213,226,243,284` fold `applyOpacity(c.Color, c.Opacity)` per command; no offscreen group buffer exists). Measured oracle: container `opacity:0.5` + two `rgba(0,0,255,0.5)` 60×60 boxes overlapping 30px → overlap Chromium `(158,158,255)` vs Goosie `(143,143,255)`. Closing needs an offscreen per-group composite at the tile raster — a hot-path + memory-budget architecture change. **Deferred to its own sub-project** (higher risk than SP-7's slice).
- **`GOOSIE-INLINEBORDER-001` (inline borders never painted) is CONFIRMED open and is SP-7.** `internal/paint/builder_box.go:203` runs `paintBorders`/`paintOutline` only when `computed.Display != style.DisplayInline`, so an inline element's own border is dropped rather than fragmented per line. Measured oracle (integrator, 2026-10-26, `div{width:300;height:60;margin:10;background:rgba(200,0,0,.5)}` + `span{border:3px solid rgba(0,0,200,.5)}` around "two"): `cmd/dump-paint` emits **0** BORDER commands for the inline; Chromium holds **198 px of `(113,63,163)`** (the span border composited once over the block tint `(227,127,127)`), Goosie holds none. Blast radius: any inline `border` (chips, rules, `<a>`/`<abbr>`/`<code>` styling) is silently lost. **No corpus fixture puts a border on an inline box**, so the parity ratchet (`testdata/render-known-gaps.json`, exact-equality at `testdata/parity.py:459-470`) is untouched by the fix; SP-7 adds a new scored fixture.

**Precedent.** `GOOSIE-INLINEBG-001` (closed, TASK-048) already fragments inline **backgrounds** per line: layout splices a box per word carrying the generating element's computed style (`style.inheritStyle: cs := *parent`), and `internal/paint/builder_box.go:65-75` (`fragmentOwnsBackground`) + `:187-201` let the text fragments own the per-line background while the inline element's own box paints none. Borders follow the same fragmentation shape, with one difference: a background fills every word fragment, but a border draws **top/bottom on every line fragment and left/right only at the true start/end of the inline run** (css-text-decor L3 §4.6 analogue). That start/end distinction is the new work.

**Layout substrate already present.** `internal/layout/inline.go:264-352`: `Inline` runs `inlineInto` (positions word/replaced fragments per line) then `inlineSelfRects` (post-order, sets each `display:inline` element box to the **union** of its descendants' fragment border rects via `inlineFragmentBounds`). The union rect is correct for a single-line inline but wrong for a multi-line one (it fills the inter-line gaps). `lineBox`/`textRun` (`inline.go:792-869`) carry per-run geometry and the line index is known in the positioning loop (`inline.go:491-574`).

## 2. Goal

An inline element with a `border` paints that border fragmented across the lines it spans — top/bottom strips on each line fragment, a left strip only at the run's true start and a right strip only at its true end — matching Chromium, with a scored fixture and a positive per-fragment oracle. Single-line inlines (the common chip/rule/code case) paint one border box identical to today's block border. No change to block borders, outlines, backgrounds, or the parity corpus.

## 3. Architecture

Two layers, following the inline-background precedent:

### 3.1 Layout — per-line fragment rects on the inline box

- Add a field to `layout.Object` (`internal/layout/arena.go`): `InlineFragments []frame.RectF` — the ordered (top line → bottom line) per-line **border rects** of a `display:inline` element, in float32 **CSS px** (layout's coordinate space; `frame.Rect` is int32 device px), populated only when the element has a nonzero border. Paint converts each fragment with `.ToDevice(b.scale)`, mirroring `paintContextFor` (`builder_box.go:164`). `frame.RectF` is the dependency leaf; a slice of values (no `*layout.Object`) keeps `ArenaPointerFields` archtest happy.
- Populate it in two steps that reuse the existing post-order union walk, so nesting falls out naturally:
  1. **Capture (positioning loop, `inline.go:491-574`):** the line index `li` is known here, unlike a post-hoc Y-cluster (fragile for mixed-font/mixed-vertical-align lines). For each positioned non-atomic word/replaced fragment, record its border rect under its **immediate generating inline element** (the text node's DOM parent, resolved by node exactly as `fragmentOwnsBackground` does), keyed by line, unioning X within the line. Atomic inline-block/replaced children are skipped (they own their own box, as `inlineFragmentBounds` skips blocks).
  2. **Union up the tree (`inlineSelfRects`, post-order):** each `display:inline` element's `InlineFragments` is the per-line union of its own captured word-fragment rects and its child inline elements' `InlineFragments`, ordered top line → bottom line. This is `inlineFragmentBounds` done per line instead of as one union, so a nested inline ancestor fragments around its descendants without a separate parent-chain walk.
- `inlineSelfRects` keeps setting the single union rect (hit-testing/hover/geometry depend on it); `InlineFragments` is additive metadata for paint only. Each fragment rect is the per-line **content union expanded outward** by the box's border+padding: top/bottom on every fragment, the inline-start edge on the first and the inline-end edge on the last, mapped to physical edges by `direction` (LTR start=left/end=right, RTL start=right/end=left). A single-line inline yields a one-element list expanded on all four edges (the union `BorderRect()` itself stays the bare content union, so the fragment is larger by the border thickness). Capture is gated on a nonzero border width, so the common borderless inline allocates nothing and the corpus stays byte-identical.

### 3.2 Paint — emit one border per fragment

- In `paintBoxDecoration` (`internal/paint/builder_box.go:185-210`), replace the blanket `Display != DisplayInline` border guard with: block/other boxes paint borders as today; a `display:inline` box with `len(InlineFragments) > 0` paints **one border per fragment rect**, in order.
- Per fragment, reuse `paintBorders` (`internal/paint/builder_borders.go:13-62`) but constrain which edges draw: every fragment draws top+bottom; `InlineFragments[0]` additionally draws left; `InlineFragments[len-1]` additionally draws right. Implement via a small edge-mask parameter or a fragment-aware variant so the existing `BorderSpec`/`toDevice` logic is reused, not duplicated. A single-fragment inline draws all four edges (identical to a block border).
- Outline stays gated as today (out of scope; `GOOSIE-INLINEBORDER-001` is about borders). Backgrounds/shadows keep their existing `paintsInlineFragments` path unchanged.
- Border-image on an inline: **out of scope (deferred).** Inline fragments use the normal solid-border path; `paintBorderImage` is not invoked for inline fragments. The plan records this as a known limitation (an inline with `border-image` paints its normal border, not the image) rather than silently diverging. The primary oracle uses solid square borders.

## 4. Data flow

`style` (border widths/colors already on the inline element's computed style, copied to word fragments) → `layout.Inline` positions fragments per line and records `InlineFragments` (ordered per-line border rects) on the inline element box → `paint.paintBoxDecoration` reads `InlineFragments` and emits one `CmdBorder` per fragment with the start/end edge mask → `raster` draws each `CmdBorder` unchanged (`tile.go:210-213` already applies per-command opacity to border colors). No new display-command kind; no raster change.

## 5. Error handling and edge cases

- **No border / zero width:** `InlineFragments` stays nil; paint emits nothing (today's behavior). `paintBorders` already no-ops when all widths are 0.
- **Single-line inline:** one fragment, all four edges — must equal the block-border rendering pixel-for-pixel (primary oracle).
- **Multi-line inline:** top/bottom on each line, left only on the first line's fragment, right only on the last line's fragment; no border in the inter-line gaps (the union-rect bug this replaces).
- **Empty inline (no fragments):** `inlineFragmentBounds` returns `ok=false`; `InlineFragments` nil; nothing painted.
- **Nested inline borders:** each border-bearing ancestor fragments independently; plan adds a fixture.
- **Mixed fonts / vertical-align on a line:** capture in the positioning loop (line index known) avoids Y-cluster fragility; fragment rect is the line's union of the element's word border rects.
- **Atomic children (inline-block/img):** skipped, own their own box (matches `inlineFragmentBounds`).
- **border-image / border-radius on inline:** both deferred for inline fragments (§3.2, §8); the primary oracle uses solid square borders. Recorded as known limitations, not silent divergence.
- **RTL/bidi:** fragments are ordered visual top→bottom; the first fragment draws the **inline-start** edge and the last draws the **inline-end** edge, mapped to left/right by the element's `direction` (LTR: start=left, end=right; RTL: start=right, end=left). The primary oracle is LTR; RTL inline-border pixel sign-off is deferred (§8), but the edge-mask mapping is direction-aware so the behavior is defined, not accidental.

## 6. Testing

- **Positive per-fragment oracle ([R-7]):** a `test/layout` (or `test/paint`) test asserting `InlineFragments` length and rects for a single-line and a multi-line inline, AND a display-list test counting `CmdBorder` commands per fragment with the correct edge mask (left only on first, right only on last). Every oracle has a positive expectation — not just "a rebuild happened" (the `GOOSIE-PSEUDO-002` failure mode).
- **Pixel oracle:** extend the integrator's measured document into a scored fixture `testdata/render/.../inline-border*.html` + Chromium reference; assert the span border region holds `(113,63,163)` (single composite over the block tint), not the double-darkened or absent color. Register the fixture in the parity corpus; it must PASS at the ≥90% band threshold and not change any existing fixture's band count.
- **Mutation evidence:** (M1) revert the paint guard to suppress inline borders → oracle red (0 BORDER commands). (M2) paint a full four-edge border on **every** word fragment (no start/end mask) → multi-line oracle red (left/right strips around each word). (M3) drop the per-line capture (union rect only) → multi-line oracle red (border fills inter-line gaps). Each mutant reverted by re-edit, suite green after.
- **No-regression matrix:** `go build -o /dev/null ./...`, `go vet -all ./...`, `go test ./internal/archtest/` (new `[]frame.RectF` field, no `*layout.Object`, no mutable Go global), `go test ./...`, `go test -race` (engine/layout/paint/raster subset), gate suite, `make parity` (existing 105/105 unchanged at 96.39% gaps 4/6; new fixture passes). Native untouched.

## 7. Acceptance

SP-7 ships when: an inline element's border paints fragmented per line (top/bottom each line, left/right only at the run's true start/end), verified by a positive per-fragment display-list oracle + a scored Chromium pixel fixture; single-line inline borders equal block-border rendering; the three mutants are red then restored; archtest/matrix/race/gate green; the existing parity corpus is byte-identical (105/105, 96.39%, gaps 4/6) with the new fixture passing. `GOOSIE-INLINEBORDER-001` closed in the ledger with anchors + mutation record. Roadmap records SP-7's slice and the next evidence-driven candidate. Commits on `main`, **no push**.

## 8. Deferred (YAGNI, with evidence)

- **Group-opacity offscreen compositing (`GOOSIE-COMPOSITE-001`)**: its own sub-project — raster group-buffer architecture, hot-path + memory budget, measured overlap oracle ready.
- **L3/L4 incremental invalidation**: still SP-7+ per SP-5 terrain; needs a measurement foundation first.
- **Inline outline fragmentation, inline border-image/border-radius fragmentation, RTL inline-border visual sign-off**: edge cases beyond the solid-square-border primary oracle; each its own increment when a journey exists.
- **Color emoji / CJK tofu (`GOOSIE-COVERAGE-001`)**: font-library limitation (sbix/COLR unsupported by `golang.org/x/image/font/opentype`; Apple Color Emoji exceeds `MaxFallbackFontBytes`), not a bounded engine slice.
- **QC ledger re-measure/closure of stale `GOOSIE-PSEUDO-001`**: docs hygiene, separate from this feature; recorded here so future planning does not re-implement `::first-line`.
- **Native visual sign-off**: no display; headless pixel read-back + Chromium parity remain the visual evidence, consistent with all prior work.

## 9. Execution record (SP-7, 2026-11-12)

Shipped on `main`, no push. The field is `[]frame.RectF` (float32 CSS px); paint
converts each fragment with `.ToDevice(b.scale)`, mirroring `paintContextFor`.

Commits: `1a0b47ef` (layout capture), `c9bb914e` (paint fragments), `588ec17d`
(fixture + floor 105→106), and the docs commit closing GOOSIE-INLINEBORDER-001.

Oracles green: `test/layout/inline_border_fragment_test.go` (fragment count,
line order, single-line edge expansion) and `test/paint/inline_border_fragment_test.go`
(one `CmdBorder` per fragment, start/end edge mask, single-line four edges,
borderless none). Mutants M1 (suppress guard), M2 (four edges per fragment),
M3 (union-rect-only) each killed a named oracle and were reverted by re-edit;
the working tree is byte-clean of mutants.

Matrix: build / vet / full `go test ./...` / race (layout+paint) / archtest all
green. Parity **106/106**, average **96.41%**, known gaps **4 fixtures / 6 bands**;
the new `borders/04-inline-fragment.html` scores **98.86% PASS** with no blank
band, and a per-fixture diff against the pre-SP-7 baseline confirms all **105
prior fixtures byte-identical** (zero score/status drift).

Deviation from the plan: inline borders/padding are resolved at **`Block` entry**
(`resolveInlineEdges`), not in `Inline`, because `inlineInto` first runs inside
the `Block` pass (`flagInlineLaidOut` blocks a re-run in `Inline`); the scratch
map is reset there too, and `Inline` re-resolves edges for percentage padding
without resetting the capture. Pure inline children never receive
`resolveBoxSizes`, so their arena `Border*`/`Padding*` were zero until this
explicit resolution. The nil-`Style` arena root is recursed through (pruned only
on `DisplayNone`). `BorderRect()` stays the bare content union, which is why
resolving inline edges leaves the corpus byte-identical. Deferred per §8.
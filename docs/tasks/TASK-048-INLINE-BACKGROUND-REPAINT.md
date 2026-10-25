# TASK-048 — text-run boxes repaint the block's background (GOOSIE-INLINEBG-001)

**Priority:** P2. **Method:** INT (display-list + rasterised-pixel oracles in `test/paint/`) + PARITY (new scored fixture in `testdata/render/css-features/`).
**Owner:** agent tasked with TASK-048 (branch `worktree-task-048-inline-background-repaint`). **Defect:** `GOOSIE-INLINEBG-001`.
**Base:** `main` at `fe4928c`.
**Language:** English, because every number in §1–§7 has to be quotable verbatim into the defect registry.

## 0. Scope and rules honoured in this branch

- `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `test/qc/catalog.go`,
  `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json` were **not** touched. BR ids I propose and the
  exact test names carrying them are in §8; tagging `// qc:BR-xxx` and `-update` is the integrator's move. Adding a
  marker for an id that is not in the catalog yet would have red `go test ./test/qc/` ("unknown QC ID"), so none was added.
- `testdata/parity.py` was touched at the corpus floor and nowhere else: `MIN_SCORED_FIXTURES` 87 → 88 with the
  one-line provenance entry the file already keeps for every floor bump, in the same commit as the fixture (the diff is
  4 added lines and 1 removed). No scoring rule, threshold, fixture walk or reference path in the harness was changed.
- No `git push`, no merge, no PR.
- Every claim below carries the command, the real number, and the exit code measured on this host (arm64, go1.26.10,
  Chromium 131.0.6778.33 via the repo's own Playwright install).

## 1. Reproduction, measured by me before touching anything

### 1.1 Display list, `cmd/dump-paint`

Probe `/tmp/t048-opaque.html`:

```html
<!doctype html>
<html><head><style>
  body { margin:0 }
  div { width:300px; height:60px; background:rgb(200,0,0); }
</style></head>
<body><div>one two three four</div></body></html>
```

`go build -o /tmp/goosie-dump-paint-prefix ./cmd/dump-paint` on a tree with `internal/paint/builder_box.go` reverted
to `fe4928c`, then `/tmp/goosie-dump-paint-prefix /tmp/t048-opaque.html` (exit 0):

```
EXTENT Rect(0,-2 300x62)
FILL rect=(0,0,300,60) color=3355443455
FILL rect=(0,0,24,18) color=3355443455
TEXT x=0 y=14 size=16 color=255 text="o"
FILL rect=(23,0,28,18) color=3355443455
TEXT x=23 y=14 size=16 color=255 text=" "
FILL rect=(27,0,52,18) color=3355443455
TEXT x=27 y=14 size=16 color=255 text="t"
FILL rect=(51,0,56,18) color=3355443455
TEXT x=51 y=14 size=16 color=255 text=" "
FILL rect=(55,0,88,18) color=3355443455
TEXT x=55 y=14 size=16 color=255 text="t"
FILL rect=(87,0,92,18) color=3355443455
TEXT x=87 y=14 size=16 color=255 text=" "
FILL rect=(91,0,118,18) color=3355443455
TEXT x=91 y=14 size=16 color=255 text="f"
```

One `FILL` for the block, then **one more per text run**. `3355443455 = 0xC80000FF` is `rgb(200,0,0)` premultiplied
with alpha 255, so the element's own background is what each run paints.

### 1.2 Pixels, and where the entry's numbers need correcting

**The entry's rect list is short by one.** `docs/qa-open-defects.json` quotes
`(0,0,24,18) (23,0,28,18) (27,0,52,18) (51,0,56,18) (55,0,88,18) (87,0,92,18)` — six runs. The real emission is
**seven**: the entry omits `(91,0,118,18)`, the box for the fourth word "four". That matters because the count is the
oracle: a test that pins "six extra fills disappear" would still pass if a mechanism fixed six of seven. Say it loudly:
**seven fragment fills, not six.**

Everything else in the entry reproduces exactly:

- probe `/tmp/t048-translucent.html` = the same block with `background:rgba(200,0,0,0.5)`;
- inside a word box Goosie renders **(213,63,63)**, the block elsewhere **(227,127,127)**;
- Chromium renders **(227,127,127) uniformly**.

Measured at DPR 1 over the whole 300x60 block (18,000 CSS pixels) — Chromium screenshot via Playwright, Goosie via its
own headless `-screenshot` path, counts by exact RGB (`/tmp/tcount.py`, exit 0):

| render | twice-composited (213,63,63) | once-composited (227,127,127) | other (glyph ink + AA) |
| --- | --- | --- | --- |
| Chromium 131.0.6778.33 | **0** | 17,300 | 700 |
| Goosie at `fe4928c` | **1,395** | 15,876 | 729 |
| Goosie after this fix | **0** | 17,372 | 628 |

`(213,63,63)` is exactly `rgba(200,0,0,.5)` composited over `(227,127,127)`: 0.5·200+0.5·227 = 213, 0.5·0+0.5·127 = 63.
The entry's three sampled points are the same points I measured; the 1,395-pixel figure shows the defect is not a
sampling artefact.

## 2. Cause, from reading the two stages that matter

- Layout splices a box per word for every text node, and each of those boxes carries a **verbatim copy** of the
  generating element's computed style: `internal/style/style.go:1613-1625` (`inheritStyle`) does `cs := *parent`,
  `BackgroundColor` included, and `buildSubtreeWithHost` (`internal/layout/arena.go:454-470`) hands out an arena slot
  for text nodes as well as elements.
- The paint stage then treated every box as a background owner. `internal/paint/builder_box.go`
  (`paintBoxDecoration`) gates the whole decoration on `paintsInlineFragments := computed.Display != style.DisplayInline
  || (obj.Node != nil && obj.Node.Text())`. A text run is `Display == Inline` with a text node, so the second clause
  pulled it into `paintBoxBackground` — and it filled with the copied background.
- The style stage had already learned this lesson in one place only: `style.AnonymousBlockStyle`
  (`internal/style/style.go:593-611`) zeroes `BackgroundColor` with the comment "painting it twice would double the
  alpha of a translucent box". Anonymous blocks were covered; **text runs were not**.
- The legitimate half, which must not be collapsed: an **inline** element's own box paints no background at all (the
  `Display != Inline` test skips it) precisely so its per-line fragments carry it. That is CSS-correct fragmentation,
  and `test/engine/inline_background_test.go` already guards it.

So the correct stage is the display list: the decision "does this box emit a background" belongs to the stage that
decides what exists, not to the rasteriser (which faithfully paints whatever it is handed) and not to the cascade
(whose copy is load-bearing for fragmentation, `background-position`, `background-size` and inheritance; zeroing it
would silently delete inline backgrounds instead of deleting duplicate emissions).

## 3. The fix

`internal/paint/builder_box.go`, one predicate plus one gate. The predicate lives on the `Builder` because resolving
the generating element needs the arena:

```go
func (b *Builder) fragmentOwnsBackground(obj *layout.Object) bool {
	if obj.Node == nil || !obj.Node.Text() || obj.Node.Parent == nil {
		return true
	}
	generator, ok := b.arena.ForNode(obj.Node.Parent.ID)
	if !ok {
		return true
	}
	st := b.arena.Get(generator).Style
	return st == nil || st.Display == style.DisplayInline
}
```

and in `paintBoxDecoration` the background call becomes conditional while the shadow call deliberately stays where it
was (§6 explains why):

```go
	if len(computed.BoxShadow) > 0 {
		b.paintBoxShadow(obj, paint.rect, paint.radius, paint.opacity)
	}
	if b.fragmentOwnsBackground(obj) {
		b.paintBoxBackground(obj, paint)
	}
```

Why this shape:

- **Look the generator up by node, not by walking arena parents.** Layout reparents runs into the anonymous blocks and
  line boxes it splices, so `obj.Parent` is usually anonymous; `arena.ForNode(text node's DOM parent)` lands on the
  element whose style was copied.
- **Generated content is the exception, exactly as `boxDeclaresOpacity` treats it.** A `::before`/`::after`/
  `::first-letter` box is built from a synthetic text node with no parent element (`buildPseudoElement`,
  `internal/layout/arena.go:604`), so it returns early with `true` and keeps its own background.
- **Failures fall back to `true`** (unknown generator, absent style): an unidentified box keeps painting, so the worst
  case is the status quo, never a silently vanished background.
- It covers all four background layers at once, because all four are emitted by `paintBoxBackground`: colour, linear
  gradient, radial gradient, `background-image`.
- **TASK-044's semantics are untouched.** `paintBox(id, clip, ordinal, inheritedOpacity)` keeps its signature, and
  `boxDeclaresOpacity`, the `effective <= 0` drop, `visibilityHidden`, `applyBoxEffects`, `overflowClip` and
  `paintChildren` are byte-identical. The acceptance re-run is in §7 (`test/paint/opacity_paint_test.go`,
  `visibility_paint_test.go`, `invisible_box_test.go`).

Verified emission after the fix — the same dump as §1.1, block fill only:

```
EXTENT Rect(0,-2 300x62)
FILL rect=(0,0,300,60) color=3355443455
TEXT x=0 y=14 size=16 color=255 text="o"
TEXT x=23 y=14 size=16 color=255 text=" "
TEXT x=27 y=14 size=16 color=255 text="t"
TEXT x=51 y=14 size=16 color=255 text=" "
TEXT x=55 y=14 size=16 color=255 text="t"
TEXT x=87 y=14 size=16 color=255 text=" "
TEXT x=91 y=14 size=16 color=255 text="f"
```

One `FILL`, seven `TEXT`, no other command. The legitimate fragmentation is untouched: the inline control of §4.2
still emits one fill per fragment across both lines — `(0,0,22,18) (21,0,26,18) (25,0,50,18) (49,0,54,18)
(53,0,75,18) (74,0,79,18) (78,0,103,18)` on line one and `(0,18,22,36) (21,18,26,36) (25,18,42,36)` on line two — and
the pre-existing `test/engine/inline_background_test.go` oracle
(`TestInlineElementBackgroundPaintsPerFragment`) still passes.

## 4. Oracles

`test/paint/inline_background_repaint_test.go` (new, 341 lines, `package paint_test`, so it can only use exported API:
`engine.NewSession`, `Session.PaintChecked`, `paint.List.Build`, `raster.RasterizeTile`).
Both helpers go through the untrusted-input contract — `engine.NewSession(...)` → `Session.PaintChecked(1)` →
`dl.Build(1).All()` — so nothing is asserted about a stage the display list cannot see.

1. `TestBlockBackgroundIsEmittedOnceNotPerTextRun` — 10 subtests, each pinning the **exact** fill rects and the
   document's total solid-fill count, so a mechanism that fixes one shape by breaking another cannot pass:
   `block with a declared height` (1 fill `(0,0,300,60)`), `block with an auto height`, `raw text beside a block
   sibling` (two declarations, 2 fills), `inline fragment inside a block` (block `(0,0,300,18)` + span `(27,0,52,18)`),
   `block that wraps onto two lines` (one fill `(0,0,120,36)`, not one per line-box or per word), `table cell`
   `(2,2,89,22)`, `list item` `(0,0,800,18)`, `inline-block` `(0,0,300,18)`, `paragraph` `(0,0,800,18)`, and
   `gradient background` asserted as a **kind histogram** (`gradient=1, text=7, fill=0`) because the duplicated layer
   arrives as `CmdGradient`, not `CmdFill`.
2. `TestInlineFragmentsStillCarryTheirOwnBackground` — the four controls that must stay: a wrapping span (10 fragments
   over two lines), a span inside a block, a `::before` (3 fills), `::first-letter` (1 fill).
3. `TestTranslucentBlockBackgroundCompositesOnce` — the pixel oracle at the rasteriser seam: rasterise the 300x60
   translucent block holding `one two three four` into a `frame.Bitmap` over white and require (227,127,127) at five
   points including all four word boxes, then sweep all 18,000 pixels: `nDoubled == 0` and `nOnce >= 17000`
   (Chromium's own count is 17,300, §1.2).

The two tests pair up on purpose rather than doubling every shape: 1 lists the boxes whose **own** element is
non-inline, 2 lists the owners whose background legitimately lives in the fragments (an inline `span`, and generated
content). §5 then shows one half dies when the other is broken — mutation A leaves 2 untouched, mutation B kills it.

## 5. Mutations — every case dies when the mechanism is removed or inverted

Each mutation was applied to `internal/paint/builder_box.go` only, on top of the final state (fix in place, oracle file
in place), reverted from a saved copy immediately after its run, and the restored tree re-ran green (exit **0**). Tests
run with `go test -count=1 -run "TestBlockBackground|TestInlineFragments|TestTranslucentBlock" ./test/paint/`; exit codes
are read straight from `$?` (this shell has no `$PIPESTATUS`). Line numbers are the committed file's.

### 5.1 Mutation A — mechanism removed (predicate returns `true` always)

```go
	st := b.arena.Get(generator).Style
	_ = st
	return true // MUTATION A: mechanism removed
```

Exit code **1**: 12 `--- FAIL` headers (all 10 block subtests, their parent, and the pixel test) carrying 26 failing
assertion lines. Quoted from the log: the block test's first subtest and its last two in full, the eight between them
replaced by one `...` line, and the pixel test complete.

```
--- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun (0.07s)
    --- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun/block_with_a_declared_height (0.01s)
        inline_background_repaint_test.go:175: block background rgb(10,120,20) emitted 8 fill(s) [(0,0,300,60) (0,0,24,18) (23,0,28,18) (27,0,52,18) (51,0,56,18) (55,0,88,18) (87,0,92,18) (91,0,118,18)], want 1 [(0,0,300,60)]: a text run re-emitted the element's background
        inline_background_repaint_test.go:190: document emitted 8 solid fills in total, want 1 (one per declared background)
    ...
    --- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun/paragraph (0.01s)
        inline_background_repaint_test.go:175: block background rgb(10,120,20) emitted 8 fill(s) [(0,0,800,18) (0,0,24,18) (23,0,28,18) (27,0,52,18) (51,0,56,18) (55,0,88,18) (87,0,92,18) (91,0,118,18)], want 1 [(0,0,800,18)]: a text run re-emitted the element's background
        inline_background_repaint_test.go:190: document emitted 8 solid fills in total, want 1 (one per declared background)
    --- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun/gradient_background (0.01s)
        inline_background_repaint_test.go:194: command kind gradient = 8, want 1 (full histogram map[text:7 gradient:8])
--- FAIL: TestTranslucentBlockBackgroundCompositesOnce (0.01s)
    inline_background_repaint_test.go:307: pixel (5,17) inside the first word box = (213,63,63), want (227,127,127): the element's own 0.5 alpha reached that point more than once
    inline_background_repaint_test.go:307: pixel (30,17) inside the second word box = (213,63,63), want (227,127,127): the element's own 0.5 alpha reached that point more than once
    inline_background_repaint_test.go:307: pixel (60,17) inside the third word box = (213,63,63), want (227,127,127): the element's own 0.5 alpha reached that point more than once
    inline_background_repaint_test.go:307: pixel (95,17) inside the fourth word box = (213,63,63), want (227,127,127): the element's own 0.5 alpha reached that point more than once
    inline_background_repaint_test.go:328: 1395 of 18000 block pixels carry the twice-composited background (213,63,63); Chromium has none
    inline_background_repaint_test.go:332: only 15876 of 18000 block pixels carry the once-composited background (227,127,127), want >=17000 (Chromium measures 17300)
FAIL	github.com/vyquocvu/goosie/test/paint	0.773s
```

Every one of the 10 cases dies; fills observed vs pinned, read from that log:

| subtest | fills emitted | fills wanted |
| --- | --- | --- |
| `block with a declared height` | 8 | 1 |
| `block with an auto height` | 8 | 1 |
| `raw text beside a block sibling` | 7 + 4 (11 total) | 1 + 1 (2 total) |
| `inline fragment inside a block` | 7 + span 1 (8 total) | 1 + 1 (2 total) |
| `block that wraps onto two lines` | 12 | 1 |
| `table cell` | 6 | 1 |
| `list item` | 6 | 1 |
| `inline-block` | 6 | 1 |
| `paragraph` | 8 | 1 |
| `gradient background` | `CmdGradient` 8 | 1 (with `CmdText` 7, `CmdFill` 0) |

The pixel test fails with the same **1,395** number §1.2 measured through the headless renderer — the display-list
oracle and the rasterised oracle report the same defect in different units.
`TestInlineFragmentsStillCarryTheirOwnBackground` still **passes** under A, as it must: A is the defect itself, and the
controls exist to catch the opposite mistake (5.2).

Also under A, TASK-044's oracles stay green (`go test -count=1 -run "TestGroupOpacity|TestVisibility|TestInvisible|Opacity|Visibility" ./test/paint/`,
exit **0**) — i.e. mutation A cannot be mistaken for a TASK-044 regression, and conversely my fix does not lean on them.

### 5.2 Mutation B — over-collapse (predicate returns `false` always)

```go
	st := b.arena.Get(generator).Style
	_ = st
	return false // MUTATION B: over-collapse, no fragment ever owns a background
```

Exit code **1**; the other nine block cases now pass (the collapse hides the defect), and the *owner*
cases die — 5 `--- FAIL` headers, 4 assertion lines:

```
--- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun (0.06s)
    --- FAIL: TestBlockBackgroundIsEmittedOnceNotPerTextRun/inline_fragment_inside_a_block (0.01s)
        inline_background_repaint_test.go:175: span background rgb(200,0,200) emitted 0 fill(s) [], want 1 [(27,0,52,18)]: a text run re-emitted the element's background
        inline_background_repaint_test.go:190: document emitted 1 solid fills in total, want 2 (one per declared background)
--- FAIL: TestInlineFragmentsStillCarryTheirOwnBackground (0.02s)
    --- FAIL: TestInlineFragmentsStillCarryTheirOwnBackground/span_wrapping_onto_two_lines (0.01s)
        inline_background_repaint_test.go:252: inline fragments painted 0 fill(s) [], want 10 [(0,0,22,18) (21,0,26,18) (25,0,50,18) (49,0,54,18) (53,0,75,18) (74,0,79,18) (78,0,103,18) (0,18,22,36) (21,18,26,36) (25,18,42,36)]
    --- FAIL: TestInlineFragmentsStillCarryTheirOwnBackground/span_inside_a_block (0.01s)
        inline_background_repaint_test.go:252: inline fragments painted 0 fill(s) [], want 1 [(27,0,52,18)]
```

and the pre-existing engine oracle dies with them, run over `./test/paint/ ./test/engine/ ./test/qc/` (exit **1**):

```
--- FAIL: TestInlineElementBackgroundPaintsPerFragment (0.01s)
    inline_background_test.go:43: no background painted for the inline span; text fragments lost their fill
FAIL	github.com/vyquocvu/goosie/test/engine	5.593s
ok  	github.com/vyquocvu/goosie/test/qc	0.725s
```

Note what B does **not** touch: the `::before` and `::first-letter` controls still pass, because generated content
returns early at `obj.Node.Parent == nil` and never reaches the mutated line, and TASK-044's oracles stay green (exit
**0**). That is the mutation evidence for why the early return is part of the mechanism rather than an optimisation.

### 5.3 Mutation C — predicate inverted (`st.Display != style.DisplayInline`)

```go
	return st == nil || st.Display != style.DisplayInline // MUTATION C: predicate inverted
```

Exit code **1**, and this time all three tests fail — the block cases come back (runs of a block owner paint again)
*and* the inline controls die (fragments of an inline owner stop painting): 15 `--- FAIL` headers and 29 assertion
lines, i.e. every one of the 10 block subtests, both inline span controls, and the pixel test
(`(213,63,63)` at all four word boxes, `1395 of 18000`). The `::before` and `::first-letter` controls are the only
cases that survive C, again because they never reach the comparison. Wider run
`go test -count=1 ./test/paint/ ./test/engine/` exit **1** with `TestInlineElementBackgroundPaintsPerFragment` failing
again (`inline_background_test.go:43`). So the sign of the comparison is load-bearing in both directions; a reader who
flips it gets a red suite, not a green one.

Under C, TASK-044's oracles still pass (exit **0**, the 10 top-level tests of `opacity_paint_test.go` (5),
`visibility_paint_test.go` (4) and `invisible_box_test.go` (1)).

## 6. Does the same double emission hit **border** and **box-shadow**? (reported, deliberately not fixed)

Checked without changing anything, on the same dump path, post-fix.

### 6.1 Border: not duplicated — but inline borders are never painted at all

`/tmp/t048-border.html` (`div{width:300px;height:60px;border:3px solid rgba(0,0,200,0.5)}` holding
`one two three four`) emits exactly **one** decoration command:

```
EXTENT Rect(0,0 306x66)
BORDER rect=(0,0,306,66)
TEXT x=3 y=17 ...   (7 TEXT commands, no other BORDER)
```

Text runs are `display:inline`, and `paintBorders` sits behind `computed.Display != style.DisplayInline`, so a run
cannot re-emit the element's border. **No border duplication exists to register.**

What *does* exist is the mirror image of the same gate: an inline element's own border is dropped instead of
fragmented. `/tmp/t048-inline-border.html` — `div{width:300px;height:60px;background:rgba(200,0,0,0.5)}` with
`span{border:3px solid rgba(0,0,200,0.5)}` around "two" — dumps **zero** `BORDER` commands post-fix, while Chromium
lays the span's border box at `(67,37) 30x24` and paints it. Pixels (DPR 1, `/tmp/deco_px.py`):

| point | Chromium | Goosie (fixed) |
| --- | --- | --- |
| span border left strip, over the tint (68,50) | (113,63,163) | (68,38,38) — no border |
| span border bottom strip, over the tint (75,59) | (113,63,163) | (227,127,127) — plain tint |
| span border top strip, over white (75,38) | (127,127,227) | (255,255,255) — no border |
| block tint away from the span (200,80) | (227,127,127) | (227,127,127) |

Suggested registration (integrator's call, I did not edit the registry): a **missing-paint** defect, "an inline
element's border is never emitted; CSS fragmentation wants one border box per line fragment", P2, distinct from
GOOSIE-INLINEBG-001 because the mechanism to fix it is the opposite (emit per fragment, not suppress).

### 6.2 Box-shadow: duplicated exactly like the background, and left that way on purpose

`/tmp/t048-shadow2.html` (`div{width:300px;height:60px;background:rgba(200,0,0,0.5);
box-shadow:2px 2px 4px 1px rgba(0,0,200,0.5)}`, text `one two three four`) post-fix dumps **66 `FILL` commands**:
1 page background, 1 element background, and **64 shadow strips** = 8 for the element's own box + 8 per text run × 7
runs. Counting command: `grep -c "color=25728" /tmp/dump-shadow2.txt` → `64`; `25728 = 0x00006480` is
`rgba(0,0,200,0.5)` premultiplied (B = 200·0.5 = 100 = 0x64, A = 128 = 0x80), so every one of those 64 fills is the
same declared shadow. Chromium paints one shadow for the box.

Because the shadow strips of a run land *inside* the parent block (a run's own rect is inside the block's padding
area, so its offset blur bands overlap the block's tint), the duplication is visible as colour, not just as work:

| point | Chromium | Goosie fixed | Goosie pre-fix |
| --- | --- | --- | --- |
| inside block, under word 1 (50,61) | (227,127,127) | **(85,47,172)** | (85,47,172) |
| inside block, under word 3 (100,61) | (227,127,127) | **(31,17,189)** | (31,17,189) |
| inside block, past the last word (200,61) | (227,127,127) | (227,127,127) | (227,127,127) |
| outside the block, real drop shadow (344,104) | (247,247,253) | (95,95,220) | (95,95,220) |

Fixed and pre-fix columns are identical, which is the measurement that shows my change deliberately did **not** touch
the shadow path: `paintBoxShadow` remains gated only on `paintsInlineFragments`. The third row confirms the
contamination is per word box, i.e. the same shape as the background defect; the "past the last word" row is clean in
both.

Suggested registration: **"a `box-shadow` declaration is emitted once per text-run box as well as once per element, so
translucent shadows stack inside the generating box"**, P2, with the (50,61) = (85,47,172) vs (227,127,127) delta as
its evidence line and this task's §5.1 test file as the template for its oracle. It is the same style-copy cause, but
I did not fold it into this commit because the task said to report it, and a shadow oracle needs its own pinned
command counts.

The last row is a *different* mismatch and is flagged only so it is not mistaken for duplication: Goosie's shadow
strength/blur at a point where Chromium has already faded out reads (95,95,220) against (247,247,253). That is a
shadow-intensity question, unchanged by this task, and belongs to whoever owns `paintBoxShadow`'s blur model.

## 7. Side effects found, gates, and acceptance

### 7.1 `display:contents` loses its per-word tint (a fix, not a regression)

`/tmp/t048-contents.html` (`div{display:contents;background:rgba(200,0,0,0.5)}`, text `one two`): pre-fix dumped 3
fragment fills and rendered (227,127,127) at (30,8) — between the words; post-fix dumps **0** fills and renders
white there, which is what Chromium does (a `display:contents` element generates no box, so it paints no background).
The element's background had only ever been reaching the page through the duplicated run boxes.

### 7.2 New scored fixture

`testdata/render/css-features/14-inline-background-repaint.html` — a translucent block background behind text that
fills two full-width lines, an inline highlight inside a second such block, a translucent gradient behind text, and a
narrow tinted block whose *text* fragments across four lines while its tint stays one composite (Chromium ink rows at
5-26, 35-52, 65-86, 95-116 of its 120px content box). Chromium-measured reference values are quoted in the fixture's own
comment block: tint (227,127,127) uniformly, Goosie pre-fix (213,63,63) at (159,148) inside a word box, span-over-tint
(123,153,110) in Chromium vs (123,152,109) in Goosie, gradient band (152,190,220) → (201,173,219).

`MIN_SCORED_FIXTURES` 87 → 88 in the same commit as the fixture, as required.

Scores (`make parity` = `python3 testdata/parity.py --render --render-chromium --binary ./bin/goosie`):

| run | result |
| --- | --- |
| baseline at `fe4928c`, no fix, 87 fixtures | 87/87, average match 96.14%, exit 0 |
| with fix + fixture, floor 88 | **88/88, average 96.17%**, exit **0** |
| fixture 14 scored with the fix | **94.19% PASS** (`worst_region_iou=0.9986`, `coverage_ratio=0.9943`, `blank_regions=0/0`) |
| fixture 14 scored with the fix reverted (`/tmp/score14.py`, same Chromium reference render) | **78.18% FAIL** (pass line 90) |

The last row is the fixture's own mutation evidence: with the mechanism removed the fixture loses **16.01** points,
i.e. **104,727** mismatched pixels against **27,869** with the fix (of 480,000), concentrated in the tinted text bands.
Both columns render the identical file at the harness's own settings (800x600, DPR 1); the pre-fix column comes from
`/tmp/goosie-prefix`, a build of this tree with `internal/paint/builder_box.go` reverted to `fe4928c`, re-checked before
use by confirming it still puts (213,63,63) on 1,395 pixels of the §1.2 probe. The reference side is the same Chromium
render the harness scores, so both columns are judged against one identical image.

### 7.3 Per-fixture movement over 0.2 points (before `fe4928c` vs after, all 87 shared fixtures)

Three fixtures moved over the reporting line, **all upward**, no regressions:

| fixture | before | after | movement | why |
| --- | --- | --- | --- | --- |
| `overflow/01-overflow.png` | 90.40% | 92.60% | **+2.20** | its box uses `background-color: rgba(231,76,60,0.7)` behind text — exactly this defect, now composited once |
| `colors/01-text-background.png` | 93.64% | 95.22% | **+1.58** | `.color-rgba { background-color: rgba(0,255,255,0.5) }` behind text |
| `colors/03-opacity.png` | 99.29% | 99.71% | **+0.42** | `opacity:.8/.6/.4/.2` red boxes containing text: the runs re-filled the copied background *inside* a faded group, double-darkening the word boxes |

13 of the 87 shared fixtures changed score at all; the other 74 are identical to the baseline. Ten of the thirteen are
sub-threshold, and they are **not** noise: two independent post-fix runs (the one above, and the earlier
`/tmp/parity_after.log` at 05:52) agree exactly on all 87 shared fixtures — the only fixture that differs between those
two runs is #14, whose own text I edited in between — so every delta here is caused by the change and is
deterministic. Measured per
fixture by `/tmp/t048-movers.py`, which renders each fixture with `/tmp/goosie-prefix` and `./bin/goosie` at 800x600
DPR 1 and scores both against the harness's own Chromium reference with `parity.py`'s rule:

| fixture | before → after | pixels the fix changes | match gained | match lost |
| --- | --- | --- | --- | --- |
| `edge-cases/02-deep-nesting.png` | 98.69% → 98.86% (+0.17) | 8,376 | 886 | 71 |
| `layout-features/04-positioning.png` | 97.76% → 97.86% (+0.10) | 1,408 | 478 | 1 |
| `html-features/04-media-elements.png` | 98.91% → 98.95% (+0.04) | 4,169 | 321 | 131 |
| `css-features/03-gradients.png` | 97.66% → 97.68% (+0.02) | 7,074 | 625 | 532 |
| `overflow/02-text-overflow.png` | 95.50% → 95.52% (+0.02) | 10,773 | 66 | 4 |
| `typography/09-white-space.png` | 91.65% → 91.67% (+0.02) | 4,282 | 116 | 35 |
| `box-model/02-padding.png` | 98.47% → 98.48% (+0.01) | 1 | 1 | 0 |
| `typography/06-line-height.png` | 92.03% → 92.02% (−0.01) | 182 | 8 | 48 |
| `positioning/03-fixed.png` | 90.64% → 90.63% (−0.01) | 128 | 0 | 56 |
| `css-features/07-shadows.png` | 93.36% → 93.26% (−0.10) | 972 | 26 | 531 |

Two mechanisms account for the movers that lose pixels, and I found the second one only by measuring it. The two
`typography/` fixtures use only fully opaque `background-color: #ecf0f1`, so I expected zero change — and measured
nonzero. The reason is a second consequence of the same emission I removed: run rects **overlap** (`(0,0,24,18)` then
`(23,0,28,18)`), and each run emits `FILL` before its own `TEXT`, so a later run's background fill painted *over* the
previous run's already-drawn glyphs. Of the 182 changed pixels in 06-line-height, 64 sit where Chromium has ink and
Goosie pre-fix had `#ecf0f1` — e.g. at (122,123) Chromium (205,208,209), pre-fix (236,240,241), post-fix (32,33,33):
the duplicated fill was erasing glyph ink. Removing it restores ink, which is right in itself, and the residual loss
comes from ink Goosie draws where Chromium leaves background (its own text metrics, not this change): 48 pixels flip
from "erased, therefore matching a background pixel" to "ink, therefore not matching". **The shape my fix is wrong for
is this: it exposes existing glyph-placement drift on opaque-background text instead of leaving it hidden under
redundant fills.**

The one fixture that loses more than it gains is `css-features/07-shadows` (−0.10), and I checked its display list in
both builds before explaining it, because the mechanism is not the one I first assumed. At (92,250) Chromium has
(255,255,255), the pre-fix build has (255,255,255) and the post-fix build has **(135,135,255)**. That point is inside
the fixture's `.box-shadow-spread` box (`background: white`, `box-shadow: 3px 3px 5px 8px rgba(100,100,255,.4)`), which
holds text; the dumps show per-run shadow strips `rect=(81,246,97,264) (81,246,94,264) (85,246,101,264)
(85,246,98,264) color=673736294` — `0x28286666`, i.e. `rgba(100,100,255,.4)` premultiplied — in **both** builds, and one
extra command only in the pre-fix build: `FILL rect=(84,246,134,264) color=4294967295`, a text run's copy of the box's
opaque white. Emitted after those strips, that duplicated white fill hid the shadow leakage; with the fill correctly
suppressed (the box itself already painted that white) the §6.2 shadow duplication is visible. Whole-fixture command
counts: 432 `FILL` at `fe4928c` against 401 post-fix, with `TEXT` unchanged at 107. On the gaining side of the same
fixture, at (99,134) Chromium has (66,66,66), pre-fix (238,238,238), post-fix (60,60,60) — the ink-erasing mechanism
above. So this fixture's −0.10 is evidence for registering the shadow duplication, not evidence that the background gate
is wrong: 531 pixels lost against 26 gained, and the one I traced end to end is a shadow band.

Net across the corpus: +81 pixels in 09-white-space, −40 in 06-line-height, the three alpha fixtures sum to **+4.20**
points, and all 13 changed fixtures sum to **+4.46** points over the 87 shared fixtures.

Known gaps did not move: still **5 fixtures / 10 bands**, drift-adjusted 9 shifted / 1 truly blank, in both runs — I
did not touch `testdata/render-known-gaps.json`.

### 7.4 Gates on this host, after the final tree state

```
gofmt -l .                                                                            # exit 0, no paths listed
gofmt -l internal/paint/builder_box.go test/paint/inline_background_repaint_test.go    # exit 0, no paths listed
go vet -all ./...                                                                     # exit 0, no output
go test -count=1 ./...                                                                # exit 0, 42 packages ok
go test -race -count=1 ./...                                                          # exit 0, 42 packages ok, no DATA RACE
go test -count=1 ./internal/archtest/                                                 # exit 0
make gate                                                                             # exit 0
make parity                                                                           # exit 0   (88/88, average match 96.17%)
```

`make gate` verbatim tail: `counters rasterized=2976 reused=53457 failed=0 refused=0 deferred=0 panics=0
writes=359136000 bytes=780140544 budget=785383424 evictions=0 miss_pct=5.27 presents=600 idle=0 dropped_vsyncs=0
plans_published=1 plans_superseded=0`, `env goos=darwin goarch=arm64 go=go1.26.10 cpu=10 workers=4`,
`run backend=darwin requested="auto" interactive=true frames=600 stamped=600 elapsed_s=5.01 fps=119.8`,
`timings mean_ms=1.502 p50_ms=1.577 p99_ms=2.117 max_ms=9.748`.

Key packages, verbatim from the non-race run: `ok test/paint 2.404s`, `ok test/engine 8.523s`, `ok test/qc 2.725s`,
`ok test/gatecheck 32.368s`, `ok test/wpt 19.336s`; under `-race` the same five are
`2.619s / 25.605s / 3.754s / 31.213s / 36.940s`.
TASK-044's acceptance re-run on the final tree: `go test -count=1 -v -run "TestOpacityZeroDropsBox|TestOpacityFractionReachesCommand|TestOpacityAboveOnePaintsOpaque|TestGroupOpacity|TestGroupOpacityCountedOncePerElement|TestVisibilityHidden|TestDisplayNoneStillCollapses|TestBoxThatCSSPaintsNothingEmitsNoCommands" ./test/paint/`
exit **0**, 21 PASS lines over the 10 top-level tests of the three files (5 in `opacity_paint_test.go`, 4 in
`visibility_paint_test.go` incl. `TestDisplayNoneStillCollapses`, 1 in `invisible_box_test.go`):
`TestOpacityZeroDropsBox`, `TestOpacityFractionReachesCommand`, `TestOpacityAboveOnePaintsOpaque`,
`TestGroupOpacityAppliesToSubtree`, `TestGroupOpacityCountedOncePerElement`, `TestVisibilityHiddenDropsBoxPaint`,
`TestVisibilityHiddenKeepsLayout`, `TestVisibilityHiddenSubtreeInheritsHidden`, `TestDisplayNoneStillCollapses`,
`TestBoxThatCSSPaintsNothingEmitsNoCommands`. `go test -count=1 ./test/paint/` as a whole is exit **0** too.

## 8. §proposals — BR ids for the integrator to tag

Highest existing id in `docs/qa-test-catalog.md` **and** `test/qc/traceability.golden.json` is **BR-197** (verified by
`grep -oE 'BR-[0-9]+' … | sort -n | tail`). I propose **BR-198** and **BR-199**, and I have deliberately put **no**
`// qc:` marker in any test file, because an unknown id makes `go test ./test/qc/` fail with "unknown QC ID". Tagging
and `-update` belongs to the integrator; the mapping is exact — every test name below is the whole of what that id
would cover:

- **BR-198** — a background belongs to the box that owns it: a block's background is emitted exactly once per
  line-fragment set, while inline fragments keep their own (and generated content keeps its own).
  - `test/paint/inline_background_repaint_test.go`: `TestBlockBackgroundIsEmittedOnceNotPerTextRun` (10 subtests),
    `TestInlineFragmentsStillCarryTheirOwnBackground` (4 subtests).
- **BR-199** — a translucent block background composites once over the whole box, including wherever a word sits
  (rasterised-pixel oracle at the `raster.RasterizeTile` seam, Chromium's numbers as the expectation).
  - `test/paint/inline_background_repaint_test.go`: `TestTranslucentBlockBackgroundCompositesOnce`.
- Fixture PARITY for both: `testdata/render/css-features/14-inline-background-repaint.html`
  (`MIN_SCORED_FIXTURES` 87 → 88 in the same commit as the fixture; 94.19% with the fix, 78.18% without — §7.2).
- Not BRs, registrations the integrator may want from §6: (i) inline borders never painted (missing paint, opposite
  mechanism), (ii) `box-shadow` emitted once per text-run box as well as per element (still open, deliberately not
  fixed here), and (iii) shadow blur/strength at (344,104) reading (95,95,220) where Chromium has (247,247,253).

## 9. Unresolved / limits, stated plainly

1. **Box-shadow duplication is still open** (§6.2). The gate that fixes the background was not extended to shadows in
   this commit; the numbers needed to register it are here.
2. **Inline borders are still never painted** (§6.1) — untouched, reported only.
3. **The predicate reads the generating element's `display`, not its layout box type.** A box whose generator is
   `display:inline` but which has been given a block-ifying computed style elsewhere would be judged by the copy of the
   style the run carries. Shapes measured here behave correctly: table cell, list item, inline-block and paragraph in
   §4's table, pseudo elements in §4's controls, `display:contents` in §7.1, and a block-ified flex item probed at
   `/tmp/t048-flex.html` (`<div style="display:flex"><i>`; `i` is inline in the UA sheet, and the single post-fix fill —
   against **8** at `fe4928c` — is the evidence that its computed display is not inline once flex block-ifies it). An
   exotic generator whose arena entry is missing falls back to `true`, i.e.
   status quo, which is the safe direction but is *not* a proof.
4. **`background-image` and radial-gradient layers are inside the same gate but this task proves neither.** The colour
   and linear-gradient paths have pinned oracles (§4); a hand probe of a real image layer
   (`/tmp/t048-realimg.html`, `background-image:url(/t048-img.png)` with a decoded PNG beside it) emitted **no** image
   command in *either* build — command kinds were `EXTENT 1, FILL 8, TEXT 7` pre-fix and `EXTENT 1, FILL 1, TEXT 7`
   post-fix — so the image layer never reached the display list at all and this task says nothing about whether it
   would duplicate. That is its own investigation, not a covered case.
5. **No live-site measurement** was taken for this defect; the corpus movement in §7.3 is the only fleet-wide evidence,
   and the next live sweep decides whether real pages improve by more than the three fixtures suggest.
6. **`display:contents` tint disappearing** (§7.1) matches Chromium but is a visible behaviour change for anyone who
   relied on the old emission; it belongs in the release note, not in a silent commit message.

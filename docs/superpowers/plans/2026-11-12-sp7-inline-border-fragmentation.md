# SP-7 Inline Border Fragmentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Paint a `display:inline` element's border as one box per line fragment (top/bottom on every fragment, inline-start edge only on the first, inline-end edge only on the last), closing GOOSIE-INLINEBORDER-001.

**Architecture:** Layout captures per-line content unions under each generating inline element during the inline positioning loop (keyed by line index, resolved by DOM parent exactly like paint's `fragmentOwnsBackground`), then folds them post-order in `inlineSelfRects` into a new `Object.InlineFragments []frame.RectF` (CSS px), expanded outward by the box's border/padding. Paint replaces the blanket non-inline border guard with one `CmdBorder` per fragment via an edge-masked `paintBorderFragment`. No raster change, no new command kind, native untouched.

**Tech Stack:** Go, `internal/layout` (arena), `internal/paint` (display list), `internal/frame` (geometry), `testdata/parity.py` (Chromium-scored corpus), headless TDD.

**Spec:** `docs/superpowers/specs/2026-11-12-sp7-inline-border-fragmentation-design.md` (committed `6efa3858`).

**Standing delegation:** no approval gates, commits on `main`, **NO push**. Chat prose Vietnamese; this doc English.

---

## Conventions (apply to every task)

- `gofmt -w <changed files>` BEFORE every commit.
- NEVER pipe test output through `tail`/`head` (masks exit codes). Redirect to a `/tmp` log and `echo $?`:
  `go test ./test/layout/ > /tmp/sp7-t1.log 2>&1; echo "exit=$?"` then read the log with the Read tool.
- Build with `go build -o /dev/null ./...` (NEVER bare `go build`, which overwrites `./goosie`).
- Revert mutations by re-editing the source back (NEVER `git checkout`).
- Stage files by name (never `git add -A`).
- archtest invariants: no `*layout.Object` fields added to `internal/engine`, no mutable package-level Go globals (the new map is an `Arena` instance field, not a global), cgo confined to `internal/platform/darwin` (untouched here).
- Coordinate spaces: `layout.Object.X/Y/W/H` and `frame.RectF` are float32 **CSS px**; `frame.Rect` is int32 **device px**. `InlineFragments` is `[]frame.RectF`; paint converts each with `.ToDevice(b.scale)`, mirroring `paintContextFor` (`builder_box.go:164`).

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `internal/layout/arena.go` | box + arena data | add `Object.InlineFragments []frame.RectF`; add `Arena.inlineFragLines` scratch map |
| `internal/layout/inline.go` | inline pass | reset map in `Inline`; resolve generator + record per-line content union in positioning loop; `finalizeInlineFragments` post-order in `inlineSelfRects`; `unionRectF` helper; `sort` import |
| `internal/paint/builder_borders.go` | border commands | add `paintBorderFragment` (edge-masked) + `paintInlineBorderFragments` (per-fragment loop) |
| `internal/paint/builder_box.go` | box decoration | replace non-inline border guard with inline-fragment branch |
| `test/layout/inline_border_fragment_test.go` | layout oracle | fragment count / order / borderless cases |
| `test/paint/inline_border_fragment_test.go` | display-list oracle | one `CmdBorder` per fragment, start/end edge mask |
| `testdata/render/borders/04-inline-fragment.html` | scored pixel oracle | wrapping bordered span over a translucent block |
| `testdata/parity.py` | corpus floor | `MIN_SCORED_FIXTURES` 105 → 106 + ratchet comment |
| `docs/qa-open-defects.json` | defect ledger | close GOOSIE-INLINEBORDER-001 |
| `docs/roadmap-v2.md` | release gates | SP-7 record |
| spec + this plan | execution records | append measured results |

---

## Task 1: Layout — capture per-line inline fragments

**Files:**
- Modify: `internal/layout/arena.go` (Object field after `BidiLevels` :68; Arena field after `FirstLineStyles` :295)
- Modify: `internal/layout/inline.go` (`sort` import :3-13; `Inline` :264-271; positioning loop :573-575; `inlineSelfRects` :286-289; new helpers)
- Test: `test/layout/inline_border_fragment_test.go` (create)

- [ ] **Step 1: Write the failing layout oracle**

Create `test/layout/inline_border_fragment_test.go`:

```go
package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

// GOOSIE-INLINEBORDER-001 layout oracle: a display:inline element that wraps
// across lines records one border-box fragment per line, in line order; a
// single-line element records exactly one; a borderless element records none.
func TestInlineFragmentsCapture(t *testing.T) {
	// Single line: one fragment.
	arena := session(t, `<div style="width:400px"><span style="border:3px solid blue">word</span></div>`, 400)
	layout.Inline(arena, layout.ObjectID(1))
	span := findByTag(arena, "span")
	if span == nil {
		t.Fatal("no span box in arena")
	}
	if len(span.InlineFragments) != 1 {
		t.Fatalf("single-line span: got %d fragments, want 1", len(span.InlineFragments))
	}

	// Wrapping: one fragment per line, strictly increasing Y0 (line order).
	arena2 := session(t, `<div style="width:40px"><span style="border:3px solid blue">aa bb cc dd ee ff</span></div>`, 40)
	layout.Inline(arena2, layout.ObjectID(1))
	span2 := findByTag(arena2, "span")
	if span2 == nil {
		t.Fatal("no wrapping span box in arena")
	}
	if len(span2.InlineFragments) < 2 {
		t.Fatalf("wrapping span: got %d fragments, want >= 2", len(span2.InlineFragments))
	}
	for i := 1; i < len(span2.InlineFragments); i++ {
		if span2.InlineFragments[i].Y0 <= span2.InlineFragments[i-1].Y0 {
			t.Fatalf("fragments not in line order at %d: %v then %v",
				i, span2.InlineFragments[i-1], span2.InlineFragments[i])
		}
	}

	// Borderless inline: no fragments captured (corpus stays byte-identical).
	arena3 := session(t, `<div style="width:40px"><span>aa bb cc dd</span></div>`, 40)
	layout.Inline(arena3, layout.ObjectID(1))
	span3 := findByTag(arena3, "span")
	if span3 == nil {
		t.Fatal("no borderless span box in arena")
	}
	if len(span3.InlineFragments) != 0 {
		t.Fatalf("borderless span: got %d fragments, want 0", len(span3.InlineFragments))
	}
}

// A single-line span's one fragment is both first and last, so it is the
// content union expanded by the border on all four edges. inlineSelfRects sets
// the span's BorderRect() to exactly that content union, so the fragment must
// equal BorderRect() grown by the 3px border on every side. Deterministic: no
// dependence on how many words land on each line.
func TestInlineFragmentsEdgeExpansion(t *testing.T) {
	arena := session(t, `<div style="width:400px"><span style="border:3px solid blue">word</span></div>`, 400)
	layout.Inline(arena, layout.ObjectID(1))
	span := findByTag(arena, "span")
	if span == nil || len(span.InlineFragments) != 1 {
		t.Fatalf("need a single-line span with one fragment, got %v", span)
	}
	f := span.InlineFragments[0]
	x0, y0, x1, y1 := span.BorderRect()
	if !almostEqual(f.X0, x0-3) || !almostEqual(f.Y0, y0-3) || !almostEqual(f.X1, x1+3) || !almostEqual(f.Y1, y1+3) {
		t.Fatalf("fragment %v should be the content union (%v,%v,%v,%v) expanded by 3px on every edge",
			f, x0, y0, x1, y1)
	}
}
```

- [ ] **Step 2: Run the oracle to verify it fails**

Run: `go test ./test/layout/ -run TestInlineFragments > /tmp/sp7-t1-red.log 2>&1; echo "exit=$?"`
Expected: `exit=1`, compile error `span.InlineFragments undefined (type *layout.Object has no field or method InlineFragments)`.

- [ ] **Step 3: Add the `InlineFragments` field to `Object`**

In `internal/layout/arena.go`, after the `BidiLevels []byte` field (:68) and before the `GridCols` comment (:70), insert:

```go
	// InlineFragments holds the per-line border-box rects (CSS px) of a
	// display:inline element that wraps across lines, in line order. Paint draws
	// one border per fragment with the inline-start edge only on the first and
	// the inline-end edge only on the last (css-text-decor-3 §4.6). Empty for
	// non-inline boxes and for inline boxes with no border.
	InlineFragments []frame.RectF
```

- [ ] **Step 4: Add the scratch map to `Arena`**

In `internal/layout/arena.go`, after the `FirstLineStyles map[dom.NodeID]*style.ComputedStyle` field (:295), before the closing `}` of the struct (:296), insert:

```go

	// inlineFragLines accumulates per-line fragment rects during the inline pass,
	// keyed by generating inline element then line index. Scratch space: Inline
	// resets it, the positioning loop unions word-fragment content rects into it,
	// and inlineSelfRects folds it into each box's InlineFragments (storing the
	// expanded border boxes back so a parent inline can union them).
	inlineFragLines map[ObjectID]map[int]frame.RectF
```

- [ ] **Step 5: Add the `sort` import to `inline.go`**

In `internal/layout/inline.go`, change the import block (:3-13) to add `"sort"` after `"math"`:

```go
import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)
```

- [ ] **Step 6: Reset the map at `Inline` entry**

In `internal/layout/inline.go`, replace the `Inline` function body (:264-271):

```go
func Inline(a *Arena, root ObjectID) {
	a.inlineFragLines = make(map[ObjectID]map[int]frame.RectF)
	inlineInto(a, root)
	// Give display:inline element boxes a box of their own. The line pass above
	// positions only the word/replaced fragments under an inline element, never
	// the element itself, so an inline <a> would otherwise stay at the zero rect
	// and be unhittable, invisible to hover, and reported with no geometry.
	inlineSelfRects(a, root)
}
```

- [ ] **Step 7: Capture per-line content unions in the positioning loop**

In `internal/layout/inline.go`, in the positioning loop, replace the three lines (:573-575):

```go
			runObj.W = wordW
			runObj.H = run.contentH
			x += wordW
```

with:

```go
			runObj.W = wordW
			runObj.H = run.contentH
			// Record this word's content rect under its generating inline
			// element, keyed by line, so a wrapping span fragments its border.
			// Only border-bearing generators are tracked; borderless inline
			// content (the whole existing corpus) captures nothing.
			if gen, ok := inlineFragmentGenerator(a, runObj); ok {
				g := a.Get(gen)
				if g.BorderTop > 0 || g.BorderRight > 0 || g.BorderBottom > 0 || g.BorderLeft > 0 {
					fx0, fy0, fx1, fy1 := runObj.BorderRect()
					a.recordInlineFrag(gen, li, frame.RectF4(fx0, fy0, fx1, fy1))
				}
			}
			x += wordW
```

- [ ] **Step 8: Add the generator resolver, recorder, and union helper**

In `internal/layout/inline.go`, immediately after the `inlineFragmentBounds` function (ends :352), insert:

```go

// inlineFragmentGenerator resolves the display:inline element that owns a word
// fragment, by DOM parent rather than arena parent: layout reparents runs into
// anonymous boxes, so the box a run hangs off is frequently anonymous while the
// element that owns the copied style is the text node's DOM parent. Mirrors
// paint's fragmentOwnsBackground. Returns false when the generator is not an
// inline element (e.g. text sitting directly in a block).
func inlineFragmentGenerator(a *Arena, frag *Object) (ObjectID, bool) {
	if frag.Node == nil || !frag.Node.Text() || frag.Node.Parent == nil {
		return 0, false
	}
	gen, ok := a.ForNode(frag.Node.Parent.ID)
	if !ok {
		return 0, false
	}
	g := a.Get(gen)
	if g.Style == nil || g.Style.Display != style.DisplayInline {
		return 0, false
	}
	return gen, true
}

// recordInlineFrag unions a word fragment's content rect into the generating
// element's per-line accumulation.
func (a *Arena) recordInlineFrag(gen ObjectID, line int, r frame.RectF) {
	if a.inlineFragLines == nil {
		a.inlineFragLines = make(map[ObjectID]map[int]frame.RectF)
	}
	byLine := a.inlineFragLines[gen]
	if byLine == nil {
		byLine = make(map[int]frame.RectF)
		a.inlineFragLines[gen] = byLine
	}
	if cur, ok := byLine[line]; ok {
		byLine[line] = unionRectF(cur, r)
	} else {
		byLine[line] = r
	}
}

// unionRectF returns the smallest rect covering both inputs (frame.RectF has no
// Union method; the int32 frame.Rect one does not apply to CSS-px floats).
func unionRectF(a, b frame.RectF) frame.RectF {
	return frame.RectF4(minF(a.X0, b.X0), minF(a.Y0, b.Y0), maxF(a.X1, b.X1), maxF(a.Y1, b.Y1))
}

// finalizeInlineFragments folds the per-line accumulation into obj.InlineFragments.
// Post-order: each inline child's fragments are already expanded border boxes, so
// unioning them per line gives this box's content area including nested inline
// borders. Each line's union is then expanded outward by this box's border and
// padding: top/bottom on every fragment, the inline-start edge on the first and
// the inline-end edge on the last, mapped to physical edges by direction.
func (a *Arena) finalizeInlineFragments(id ObjectID) {
	// Nothing captured anywhere (the whole borderless-inline corpus): return
	// before allocating, so layout alloc counters stay identical for the gates.
	if len(a.inlineFragLines) == 0 {
		return
	}
	obj := a.Get(id)
	merged := make(map[int]frame.RectF)
	for line, r := range a.inlineFragLines[id] {
		merged[line] = r
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display != style.DisplayInline {
			continue
		}
		for line, r := range a.inlineFragLines[kid] {
			if cur, ok := merged[line]; ok {
				merged[line] = unionRectF(cur, r)
			} else {
				merged[line] = r
			}
		}
	}
	if len(merged) == 0 {
		return
	}
	lines := make([]int, 0, len(merged))
	for line := range merged {
		lines = append(lines, line)
	}
	sort.Ints(lines)

	rtl := obj.Style.Direction == "rtl"
	frags := make([]frame.RectF, 0, len(lines))
	expanded := make(map[int]frame.RectF, len(lines))
	for i, line := range lines {
		r := merged[line]
		first := i == 0
		last := i == len(lines)-1
		x0, y0, x1, y1 := r.X0, r.Y0, r.X1, r.Y1
		y0 -= obj.PaddingTop + obj.BorderTop
		y1 += obj.PaddingBottom + obj.BorderBottom
		expandLeft := (first && !rtl) || (last && rtl)
		expandRight := (last && !rtl) || (first && rtl)
		if expandLeft {
			x0 -= obj.PaddingLeft + obj.BorderLeft
		}
		if expandRight {
			x1 += obj.PaddingRight + obj.BorderRight
		}
		f := frame.RectF4(x0, y0, x1, y1)
		frags = append(frags, f)
		expanded[line] = f
	}
	obj.InlineFragments = frags
	a.inlineFragLines[id] = expanded
}
```

- [ ] **Step 9: Call the finalizer post-order in `inlineSelfRects`**

In `internal/layout/inline.go`, in `inlineSelfRects`, replace the block (:286-289):

```go
	if obj.Style.Display != style.DisplayInline {
		return
	}
	x0, y0, x1, y1, ok := inlineFragmentBounds(a, id)
```

with:

```go
	if obj.Style.Display != style.DisplayInline {
		return
	}
	// Fold per-line border fragments before sizing the union box: children are
	// already finalized (post-order), so a nested inline's borders union in.
	a.finalizeInlineFragments(id)
	x0, y0, x1, y1, ok := inlineFragmentBounds(a, id)
```

- [ ] **Step 10: Run the oracle to verify it passes**

Run: `go test ./test/layout/ -run TestInlineFragments > /tmp/sp7-t1-green.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, both `TestInlineFragmentsCapture` and `TestInlineFragmentsEdgeExpansion` PASS.

- [ ] **Step 11: Verify the whole layout package and build**

Run: `go test ./test/layout/ ./internal/layout/ > /tmp/sp7-t1-pkg.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.
Run: `go build -o /dev/null ./... > /tmp/sp7-t1-build.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 12: gofmt and commit**

```bash
gofmt -w internal/layout/arena.go internal/layout/inline.go test/layout/inline_border_fragment_test.go
git add internal/layout/arena.go internal/layout/inline.go test/layout/inline_border_fragment_test.go
git commit -m "$(cat <<'EOF'
feat(layout): record per-line inline border fragments

A display:inline element that wraps now records one border-box rect per
line in Object.InlineFragments, captured under the generating element by
DOM parent and folded post-order so nested inline borders union in.
Borderless inline content captures nothing, keeping the corpus unchanged.
EOF
)"
```

---

## Task 2: Paint — one border per fragment with an edge mask

**Files:**
- Modify: `internal/paint/builder_borders.go` (add two methods after `paintBorders` :62)
- Modify: `internal/paint/builder_box.go` (`paintBoxDecoration` guard :203-206)
- Test: `test/paint/inline_border_fragment_test.go` (create, reuses `ibDisplayList`)

- [ ] **Step 1: Write the failing display-list oracle**

Create `test/paint/inline_border_fragment_test.go`:

```go
package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/paint"
)

// ibBorders returns every CmdBorder in list order.
func ibBorders(cmds []paint.DisplayCmd) []paint.DisplayCmd {
	var out []paint.DisplayCmd
	for _, c := range cmds {
		if c.Kind == paint.CmdBorder {
			out = append(out, c)
		}
	}
	return out
}

// GOOSIE-INLINEBORDER-001 display-list oracle: a wrapping bordered span emits
// one CmdBorder per line fragment. The first fragment draws top+bottom+left
// (no right); the last draws top+bottom+right (no left). A single-line span
// draws all four edges on its one fragment.
func TestInlineBorderFragments(t *testing.T) {
	// Narrow viewport forces the span onto multiple lines.
	css := `div{width:80px;background:rgba(200,0,0,.5)}span{border:3px solid rgba(0,0,200,.5)}`
	body := `<div><span>aa bb cc dd ee ff</span></div>`
	cmds := ibDisplayList(t, css, body, 100)
	borders := ibBorders(cmds)
	// The block itself has no border, so every CmdBorder is a span fragment.
	if len(borders) < 2 {
		t.Fatalf("wrapping span: got %d border commands, want >= 2 (one per line)", len(borders))
	}
	first := borders[0]
	last := borders[len(borders)-1]
	if first.Border.Left.Width == 0 {
		t.Errorf("first fragment must draw the inline-start (left) edge")
	}
	if first.Border.Right.Width != 0 {
		t.Errorf("first fragment must NOT draw the inline-end (right) edge, got width %d", first.Border.Right.Width)
	}
	if last.Border.Right.Width == 0 {
		t.Errorf("last fragment must draw the inline-end (right) edge")
	}
	if last.Border.Left.Width != 0 {
		t.Errorf("last fragment must NOT draw the inline-start (left) edge, got width %d", last.Border.Left.Width)
	}
	for i, b := range borders {
		if b.Border.Top.Width == 0 || b.Border.Bottom.Width == 0 {
			t.Errorf("fragment %d must draw top and bottom edges", i)
		}
	}
}

// A single-line bordered span draws all four edges on exactly one fragment.
func TestInlineBorderSingleLine(t *testing.T) {
	css := `div{width:400px}span{border:3px solid rgba(0,0,200,.5)}`
	body := `<div><span>word</span></div>`
	cmds := ibDisplayList(t, css, body, 400)
	borders := ibBorders(cmds)
	if len(borders) != 1 {
		t.Fatalf("single-line span: got %d border commands, want 1", len(borders))
	}
	b := borders[0]
	if b.Border.Top.Width == 0 || b.Border.Bottom.Width == 0 || b.Border.Left.Width == 0 || b.Border.Right.Width == 0 {
		t.Errorf("single-line span must draw all four edges, got %+v", b.Border)
	}
}

// A borderless inline span emits no border command (corpus stays byte-identical).
func TestInlineBorderNone(t *testing.T) {
	css := `div{width:80px}span{color:blue}`
	body := `<div><span>aa bb cc dd</span></div>`
	cmds := ibDisplayList(t, css, body, 100)
	if got := len(ibBorders(cmds)); got != 0 {
		t.Fatalf("borderless span: got %d border commands, want 0", got)
	}
}
```

- [ ] **Step 2: Run the oracle to verify it fails**

Run: `go test ./test/paint/ -run TestInlineBorder > /tmp/sp7-t2-red.log 2>&1; echo "exit=$?"`
Expected: `exit=1`, `TestInlineBorderFragments` fails with `got 0 border commands, want >= 2` (the inline guard currently drops all inline borders).

- [ ] **Step 3: Add the edge-masked fragment border methods**

In `internal/paint/builder_borders.go`, immediately after `paintBorders` (ends :62), insert:

```go

// paintInlineBorderFragments draws one border per line fragment of a wrapping
// display:inline element. The fragment rects are CSS px on the box; each is
// scaled to device space here. border-image and border-radius on inline
// fragments are deferred (normal solid path, zero radius).
func (b *Builder) paintInlineBorderFragments(obj *layout.Object, opacity float32) {
	s := obj.Style
	if s == nil {
		return
	}
	rtl := s.Direction == "rtl"
	n := len(obj.InlineFragments)
	for i, frag := range obj.InlineFragments {
		rect := frag.ToDevice(b.scale)
		b.paintBorderFragment(obj, rect, opacity, i == 0, i == n-1, rtl)
	}
}

// paintBorderFragment draws the border edges one inline fragment owns: top and
// bottom on every fragment, the physical left edge on the inline-start of the
// run and the physical right edge on the inline-end, mapped by direction
// (LTR start=left/end=right, RTL start=right/end=left). Widths and colours come
// off the box exactly as paintBorders does; a nonzero width paints >= 1 device px.
func (b *Builder) paintBorderFragment(obj *layout.Object, rect frame.Rect, opacity float32, first, last, rtl bool) {
	s := obj.Style
	toDevice := func(cssPx float32) int32 {
		if cssPx <= 0 {
			return 0
		}
		if w := int32(cssPx * b.scale); w > 0 {
			return w
		}
		return 1
	}
	drawLeft := (first && !rtl) || (last && rtl)
	drawRight := (last && !rtl) || (first && rtl)
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = SideSpec{Width: toDevice(obj.BorderTop), Color: convertColor(s.BorderTopColor)}
	}
	if obj.BorderBottom > 0 {
		border.Bottom = SideSpec{Width: toDevice(obj.BorderBottom), Color: convertColor(s.BorderBottomColor)}
	}
	if drawLeft && obj.BorderLeft > 0 {
		border.Left = SideSpec{Width: toDevice(obj.BorderLeft), Color: convertColor(s.BorderLeftColor)}
	}
	if drawRight && obj.BorderRight > 0 {
		border.Right = SideSpec{Width: toDevice(obj.BorderRight), Color: convertColor(s.BorderRightColor)}
	}
	if border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0 {
		b.list.Append(DisplayCmd{
			Kind:    CmdBorder,
			Rect:    rect,
			Border:  border,
			Opacity: opacity,
		})
	}
}
```

- [ ] **Step 4: Replace the non-inline border guard**

In `internal/paint/builder_box.go`, in `paintBoxDecoration`, replace (:203-206):

```go
	if computed.Display != style.DisplayInline && computed.Display != style.DisplayNone {
		b.paintBorders(obj, paint.rect, paint.radius, paint.opacity)
		b.paintOutline(computed, paint.rect, paint.opacity)
	}
```

with:

```go
	switch {
	case computed.Display == style.DisplayInline:
		// An inline element fragments its border per line; the union box would
		// draw a single rectangle across the line gaps. Outline on inline
		// fragments stays deferred (GOOSIE-INLINEBORDER-001 §8).
		if len(obj.InlineFragments) > 0 {
			b.paintInlineBorderFragments(obj, paint.opacity)
		}
	case computed.Display != style.DisplayNone:
		b.paintBorders(obj, paint.rect, paint.radius, paint.opacity)
		b.paintOutline(computed, paint.rect, paint.opacity)
	}
```

- [ ] **Step 5: Run the oracle to verify it passes**

Run: `go test ./test/paint/ -run TestInlineBorder > /tmp/sp7-t2-green.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, all three tests PASS.

- [ ] **Step 6: Verify the whole paint package and build**

Run: `go test ./test/paint/ ./internal/paint/ > /tmp/sp7-t2-pkg.log 2>&1; echo "exit=$?"`
Expected: `exit=0` (the existing inline-background and zero-border oracles must stay green).
Run: `go build -o /dev/null ./... > /tmp/sp7-t2-build.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 7: gofmt and commit**

```bash
gofmt -w internal/paint/builder_borders.go internal/paint/builder_box.go test/paint/inline_border_fragment_test.go
git add internal/paint/builder_borders.go internal/paint/builder_box.go test/paint/inline_border_fragment_test.go
git commit -m "$(cat <<'EOF'
feat(paint): fragment inline-element borders per line

Replace the blanket non-inline border guard with one CmdBorder per line
fragment: top/bottom on every fragment, the inline-start edge only on the
first and the inline-end edge only on the last, direction-aware. Closes the
paint half of GOOSIE-INLINEBORDER-001; non-inline borders are unchanged.
EOF
)"
```

---

## Task 3: Scored Chromium parity fixture

**Files:**
- Create: `testdata/render/borders/04-inline-fragment.html`
- Modify: `testdata/parity.py` (`MIN_SCORED_FIXTURES` :77 + ratchet comment :75-76)

- [ ] **Step 1: Create the fixture**

Create `testdata/render/borders/04-inline-fragment.html`. The span wraps over a translucent block so the border blends to the ledger's measured `(113,63,163)` = `rgba(0,0,200,.5)` over the block tint `(227,127,127)`:

```html
<!DOCTYPE html>
<html>
<head>
    <title>Borders - Inline fragment</title>
    <style>
        html, body { margin: 0; font: 16px serif; }
        .tint {
            width: 160px;
            margin: 20px;
            background: rgba(200, 0, 0, .5);
        }
        .tint span {
            border: 3px solid rgba(0, 0, 200, .5);
        }
    </style>
</head>
<body>
    <div class="tint"><span>alpha bravo charlie delta echo foxtrot golf hotel</span></div>
</body>
</html>
```

- [ ] **Step 2: Raise the corpus floor in the same change**

In `testdata/parity.py`, after the `# 105: SP-2 ...` comment lines (:75-76) and before `MIN_SCORED_FIXTURES = 105` (:77), add a ratchet comment and change the value to 106:

```python
# 106: borders/04-inline-fragment.html (SP-7, GOOSIE-INLINEBORDER-001) - a
# bordered inline span wraps over a translucent block; its border fragments
# per line instead of vanishing behind the non-inline paint guard.
MIN_SCORED_FIXTURES = 106
```

- [ ] **Step 3: Verify the floor test passes**

Run: `go test ./test/gatecheck/ -run TestParityFixtureFloorMatchesCorpus > /tmp/sp7-t3-floor.log 2>&1; echo "exit=$?"`
Expected: `exit=0` (106 fixtures >= floor 106).

- [ ] **Step 4: Build the render binary and run the scored corpus**

Run: `go build -o ./bin/goosie ./cmd/goosie > /tmp/sp7-t3-bin.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.
Run: `make parity > /tmp/sp7-t3-parity.log 2>&1; echo "exit=$?"`
Expected: `exit=0`. Read `/tmp/sp7-t3-parity.log`: **106/106 passing**, the new `borders/04-inline-fragment.html` scores >= 90% (PASS), known gaps stay **4 fixtures / 6 bands** (the new fixture registers no gap), and the prior 105 fixtures keep their scores.

If the new fixture scores below PASS_AT or registers a blank band, STOP and investigate the fragment geometry (Task 1/2) before proceeding — do NOT add a `render-known-gaps.json` entry to paper over it.

- [ ] **Step 5: Commit**

```bash
git add testdata/render/borders/04-inline-fragment.html testdata/parity.py
git commit -m "$(cat <<'EOF'
test(parity): score the inline border fragment fixture

borders/04-inline-fragment.html wraps a bordered span over a translucent
block; MIN_SCORED_FIXTURES 105 -> 106 in the same change. The border blends
to the ledger's measured (113,63,163) once fragmented per line.
EOF
)"
```

---

## Task 4: Mutation proof (three mutants, red → restore)

Each mutant is applied by Edit, proven to kill a named oracle, then reverted by
re-editing the source back (NEVER `git checkout`). No commit carries a mutant.

- [ ] **Step 1: M1 — suppress the inline guard (restore the old drop-all behaviour)**

In `internal/paint/builder_box.go`, temporarily replace the `case computed.Display == style.DisplayInline:` branch body so inline draws nothing:

```go
	case computed.Display == style.DisplayInline:
		// MUTANT M1: inline borders dropped (pre-fix behaviour).
```

Run: `go test ./test/paint/ -run TestInlineBorderFragments > /tmp/sp7-m1.log 2>&1; echo "exit=$?"`
Expected: `exit=1` (`got 0 border commands, want >= 2`).
Restore the branch body to the committed `if len(obj.InlineFragments) > 0 { b.paintInlineBorderFragments(obj, paint.opacity) }` form by re-editing.
Run: `go test ./test/paint/ -run TestInlineBorder > /tmp/sp7-m1-restore.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 2: M2 — four edges on every fragment (ignore the start/end mask)**

In `internal/paint/builder_borders.go`, temporarily change `paintBorderFragment`'s edge predicates to always true:

```go
	drawLeft := true  // MUTANT M2: every fragment draws both inline edges.
	drawRight := true // MUTANT M2
```

Run: `go test ./test/paint/ -run TestInlineBorderFragments > /tmp/sp7-m2.log 2>&1; echo "exit=$?"`
Expected: `exit=1` (`first fragment must NOT draw the inline-end (right) edge`).
Restore both predicates to `(first && !rtl) || (last && rtl)` and `(last && !rtl) || (first && rtl)` by re-editing.
Run: `go test ./test/paint/ -run TestInlineBorder > /tmp/sp7-m2-restore.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 3: M3 — union rect only (one border on the union box, not per fragment)**

In `internal/paint/builder_box.go`, temporarily replace the inline branch body to paint a single union-box border:

```go
	case computed.Display == style.DisplayInline:
		// MUTANT M3: one border on the union box instead of per fragment.
		b.paintBorders(obj, paint.rect, paint.radius, paint.opacity)
```

Run: `go test ./test/paint/ -run TestInlineBorderFragments > /tmp/sp7-m3.log 2>&1; echo "exit=$?"`
Expected: `exit=1` (`got 1 border commands, want >= 2`).
Restore the branch body to the committed per-fragment form by re-editing.
Run: `go test ./test/paint/ -run TestInlineBorder > /tmp/sp7-m3-restore.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 4: Confirm the working tree is byte-clean of mutants**

Run: `git diff --stat > /tmp/sp7-m4-clean.log 2>&1; echo "exit=$?"`
Expected: no unstaged changes to `internal/paint/builder_box.go` or `internal/paint/builder_borders.go` (the mutants were reverted by re-edit; the committed Task 2 code is intact). Read the log to confirm.

---

## Task 5: Full no-regression matrix

- [ ] **Step 1: Build everything**

Run: `go build -o /dev/null ./... > /tmp/sp7-matrix-build.log 2>&1; echo "exit=$?"`
Expected: `exit=0`.

- [ ] **Step 2: Vet**

Run: `go vet -all ./... > /tmp/sp7-matrix-vet.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, no diagnostics.

- [ ] **Step 3: Full test suite**

Run: `go test ./... > /tmp/sp7-matrix-test.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, every package `ok` (no failures). Read the log to confirm.

- [ ] **Step 4: Race detector on the touched packages**

Run: `go test -race ./internal/layout/ ./internal/paint/ ./test/layout/ ./test/paint/ > /tmp/sp7-matrix-race.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, no race reports.

- [ ] **Step 5: Architecture rules**

Run: `go test ./internal/archtest/ > /tmp/sp7-matrix-arch.log 2>&1; echo "exit=$?"`
Expected: `exit=0` (imports, cgo confinement, gofmt, no mutable globals — the new map is an Arena field, not a global).

- [ ] **Step 6: Scored corpus unchanged + new fixture**

Run: `make parity > /tmp/sp7-matrix-parity.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, **106/106**, prior 105 fixtures byte-identical scores, average ~96.4%, known gaps **4 fixtures / 6 bands**. Read the log to confirm the new fixture passes and no existing score moved.

---

## Task 6: Documentation — close the defect and record SP-7

**Files:**
- Modify: `docs/qa-open-defects.json` (GOOSIE-INLINEBORDER-001 entry :398-406)
- Modify: `docs/roadmap-v2.md` (SP-7 record near the SP-6 bullet :210-212)
- Modify: `docs/superpowers/specs/2026-11-12-sp7-inline-border-fragmentation-design.md` (append execution record; refine §3.1 field type)
- Modify: `docs/superpowers/plans/2026-11-12-sp7-inline-border-fragmentation.md` (append execution record)

- [ ] **Step 1: Close GOOSIE-INLINEBORDER-001 in the ledger**

In `docs/qa-open-defects.json`, in the GOOSIE-INLINEBORDER-001 object (:398-406), change `"status": "open"` to `"status": "closed"`, change `"owner": "needs a task"` to `"owner": "docs/superpowers/plans/2026-11-12-sp7-inline-border-fragmentation.md"`, append a closure sentence to the `evidence` string, and set `"closed_by"` to the Task 2 paint commit hash (fill the real short hash after Task 2). The closure sentence to append (before the closing quote of `evidence`):

```
 CLOSURE (SP-7, 2026-11-12): layout records per-line border-box fragments on Object.InlineFragments (internal/layout/inline.go finalizeInlineFragments, captured under the generating inline element by DOM parent in the positioning loop); paint emits one CmdBorder per fragment with top/bottom on every fragment and the inline-start/inline-end edges only on the first/last (internal/paint/builder_borders.go paintBorderFragment, builder_box.go paintBoxDecoration). Oracles: test/layout/inline_border_fragment_test.go (fragment count/order/edge expansion) and test/paint/inline_border_fragment_test.go (one border per fragment, start/end mask). Three mutants killed: M1 suppress guard, M2 four-edge-per-fragment, M3 union-rect-only. Fixture borders/04-inline-fragment.html scores PASS, MIN_SCORED_FIXTURES 105 -> 106, corpus gaps unchanged 4/6. Deferred (§8): inline outline, border-image and border-radius on fragments, RTL pixel sign-off.
```

Set `"closed_by": "<task2-short-hash> (paint) + <task1-short-hash> (layout) + <task3-short-hash> (fixture), SP-7"`.

- [ ] **Step 2: Validate the ledger JSON**

Run: `python3 -c "import json;json.load(open('docs/qa-open-defects.json'));print('ok')" > /tmp/sp7-t6-json.log 2>&1; echo "exit=$?"`
Expected: `exit=0`, prints `ok`.

- [ ] **Step 3: Record SP-7 in the roadmap**

In `docs/roadmap-v2.md`, after the SP-6 multi-window bullet (:210) and its "Tiếp: SP-7" line, add an SP-7 record bullet (Vietnamese, matching the surrounding style):

```markdown
- Inline border fragmentation — đã có (SP-7, GOOSIE-INLINEBORDER-001 đóng): border của phần tử `display:inline` được vẽ theo từng mảnh dòng (top/bottom mọi mảnh, cạnh đầu chỉ ở mảnh đầu, cạnh cuối chỉ ở mảnh cuối), thay vì bị bỏ rơi sau guard non-inline. Layout ghi `Object.InlineFragments []frame.RectF`; paint phát một `CmdBorder` mỗi mảnh. Fixture `borders/04-inline-fragment.html` tính điểm, sàn 105→106, gaps giữ 4/6. Hoãn: outline/border-image/border-radius trên mảnh inline, sign-off pixel RTL.
```

Update the "Tiếp:" line to point at the next evidence-driven candidate (group opacity / L3-L4 invalidation) rather than SP-7.

- [ ] **Step 4: Refine the spec field type and append its execution record**

In `docs/superpowers/specs/2026-11-12-sp7-inline-border-fragmentation-design.md`, change §3.1's `InlineFragments []frame.Rect` to `InlineFragments []frame.RectF` (layout is float32 CSS px; paint converts with `.ToDevice(b.scale)`). Append:

```markdown
## 9. Execution record (SP-7, 2026-11-12)

Shipped on `main`, no push. Field refined to `[]frame.RectF` (CSS px) during
planning; paint converts each fragment with `.ToDevice(b.scale)`, mirroring
`paintContextFor`. Commits: <task1> (layout capture), <task2> (paint
fragments), <task3> (fixture + floor 105→106), <task6> (docs). Oracles green:
test/layout/inline_border_fragment_test.go, test/paint/inline_border_fragment_test.go.
Mutants M1/M2/M3 each killed a named oracle and were reverted by re-edit.
Matrix: build/vet/test/race/archtest green; parity 106/106, prior 105
byte-identical, gaps 4/6. Deferred per §8.
```

- [ ] **Step 5: Append this plan's execution record**

At the end of `docs/superpowers/plans/2026-11-12-sp7-inline-border-fragmentation.md`, append:

```markdown
---

## Execution record (SP-7, 2026-11-12)

| Task | Commit | Result |
|------|--------|--------|
| 1 layout capture | <hash> | InlineFragments []frame.RectF; layout oracle green |
| 2 paint fragments | <hash> | edge-masked CmdBorder per fragment; display-list oracle green |
| 3 fixture + floor | <hash> | borders/04-inline-fragment.html PASS; 105→106 |
| 4 mutants | (no commit) | M1/M2/M3 red → restored by re-edit |
| 5 matrix | (no commit) | build/vet/test/race/archtest/parity green |
| 6 docs | <hash> | ledger closed, roadmap + spec/plan records |

Deviations: field type `[]frame.Rect` → `[]frame.RectF` (CSS px) applied in
Task 1 and reconciled in spec §3.1. No other deviations. NO push.
```

- [ ] **Step 6: gofmt, validate, and commit**

```bash
gofmt -w internal/layout/arena.go internal/layout/inline.go internal/paint/builder_borders.go internal/paint/builder_box.go
git add docs/qa-open-defects.json docs/roadmap-v2.md docs/superpowers/specs/2026-11-12-sp7-inline-border-fragmentation-design.md docs/superpowers/plans/2026-11-12-sp7-inline-border-fragmentation.md
git commit -m "$(cat <<'EOF'
docs(qa): close GOOSIE-INLINEBORDER-001 and record SP-7

Inline border fragmentation shipped: ledger closed with anchors and the
three mutants, roadmap SP-7 bullet, spec field type refined to RectF and
execution records appended to spec and plan.
EOF
)"
```

---

## Self-Review

**1. Spec coverage:**
- §3.1 layout field + two-step capture → Task 1 (field Steps 3-4, capture Step 7, post-order union Steps 8-9). ✓
- §3.2 paint guard replacement + edge mask + reuse BorderSpec/toDevice + outline gated + border-image deferred → Task 2 (Steps 3-4; `paintBorderFragment` reuses `BorderSpec`/`SideSpec`/`convertColor` and the same `toDevice` rule; no `paintBorderImage` call; outline only in the non-inline branch). ✓
- §4 no raster change / no new cmd kind → only `CmdBorder` reused; no raster files touched. ✓
- §5 edge cases: no border (Task 1 Step 1 borderless case + Task 2 `TestInlineBorderNone`), single-line all four edges (Task 2 `TestInlineBorderSingleLine`), multi-line mask (Task 2 `TestInlineBorderFragments`), nested (post-order union, Task 1 Step 8), atomic children skipped (merge loop checks `Display == DisplayInline`, inline-block is `DisplayInlineBlock`), direction-aware mapping (Task 1 Step 8 `expandLeft/Right`, Task 2 Step 3 `drawLeft/Right`), border-image/radius/RTL sign-off deferred (records). ✓
- §6 testing: layout oracle + display-list oracle + scored fixture + 3 mutants + matrix → Tasks 1-5. ✓
- §7 acceptance: ledger closed, corpus byte-identical + new fixture → Task 3 Step 4, Task 5 Step 6, Task 6 Step 1. ✓
- §8 deferred items recorded → Task 6. ✓

**2. Placeholder scan:** the only intentional fill-ins are commit short hashes (`<task1-short-hash>` etc.) in Task 6, which cannot be known until the commits exist; every code step has complete literal code, every run step has an exact command and expected output. No "TBD"/"handle edge cases"/"similar to Task N". ✓

**3. Type consistency:** `InlineFragments []frame.RectF` used identically in arena.go (Task 1 Step 3), the layout test (Task 1 Step 1), `finalizeInlineFragments` (Step 8), and paint `paintInlineBorderFragments` (Task 2 Step 3, `.ToDevice(b.scale)` → `frame.Rect`). `recordInlineFrag(gen ObjectID, line int, r frame.RectF)`, `inlineFragmentGenerator(a *Arena, frag *Object) (ObjectID, bool)`, `unionRectF(a, b frame.RectF) frame.RectF`, `finalizeInlineFragments(id ObjectID)`, `paintBorderFragment(obj *layout.Object, rect frame.Rect, opacity float32, first, last, rtl bool)`, `paintInlineBorderFragments(obj *layout.Object, opacity float32)` — signatures match across all call sites. `minF`/`maxF` exist (block.go:3046/3053). `sort` added to inline.go imports. ✓

---

## Execution record (SP-7, 2026-11-12)

| Task | Commit | Result |
|------|--------|--------|
| 1 layout capture | `1a0b47ef` | `InlineFragments []frame.RectF`; layout oracle green |
| 2 paint fragments | `c9bb914e` | edge-masked `CmdBorder` per fragment; display-list oracle green |
| 3 fixture + floor | `588ec17d` | `borders/04-inline-fragment.html` 98.86% PASS; 105→106; prior 105 byte-identical |
| 4 mutants | (no commit) | M1/M2/M3 each red → restored by re-edit; tree byte-clean |
| 5 matrix | (no commit) | build/vet/test/race/archtest green; parity 106/106, 96.41%, gaps 4/6 |
| 6 docs | (this commit) | ledger closed, roadmap + spec/plan records |

Deviations from the plan as written:

1. **Field type** `[]frame.Rect` → `[]frame.RectF` (float32 CSS px), applied in
   Task 1 and reconciled in spec §3.1 during planning. Paint converts with
   `.ToDevice(b.scale)`.
2. **Border resolution + map reset moved to `Block` entry.** Task 1 Step 6 reset
   the scratch map in `Inline`; execution discovered `inlineInto` first runs
   inside the `Block` pass (`block.go` call sites) and `flagInlineLaidOut` blocks
   a re-run in `Inline`, so capture happens during `Block`. Added
   `resolveInlineEdges(a, root, viewportW)` + `a.inlineFragLines = make(...)` at
   `Block` entry; `Inline` re-resolves edges (for percentage padding against the
   now-known block width) **without** resetting the map. Pure inline children
   never receive `resolveBoxSizes`, so their arena `Border*`/`Padding*` were zero
   until this explicit resolution.
3. **nil-`Style` root recursion.** `resolveInlineEdges` prunes only on
   `DisplayNone` and guards the resolution block with `obj.Style != nil`, always
   recursing — the arena root (ID 1, the document) has nil `Style` and an early
   `Style == nil` return would have stopped the walk before any inline element.
4. **`frame` import added to `block.go`** for the `map[ObjectID]map[int]frame.RectF`
   reset at `Block` entry.

`BorderRect()` stays the bare content union regardless of border/padding values,
which is why resolving inline edges leaves the corpus byte-identical (verified by
a per-fixture score diff against the pre-SP-7 baseline: zero drift on 105). NO push.
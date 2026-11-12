# SP-8 Group Opacity Offscreen Compositing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Composite a faded CSS `opacity` group once offscreen (render group → fade once → blend down) instead of multiplying its opacity per command at tile-raster time, so two translucent layers that overlap inside one `opacity<1` group darken correctly instead of twice. Closes `GOOSIE-COMPOSITE-001`.

**Architecture:** Paint marks each *clean* faded group — an opacity-declaring box with no positioned `z-index>0` descendant — by recording the contiguous sub-range of commands its subtree emitted. The flat display list is sorted by `Z` once per content version, so a group is recovered after sorting through a stable append-sequence id each command carries. `LayerDL` exposes `Groups()`/`GroupAt(i)`. The tile rasterizer renders a group's range into a tile-sized scratch bitmap (drawing each member command exactly as today, with the group's own opacity withheld from the per-command field), fades the buffer once, and composites it onto the tile; nested clean groups recurse. Non-clean faded boxes and all non-opacity content keep the existing per-command path unchanged.

**Tech Stack:** Go; `internal/paint` (`cmd.go`, `list.go`, `builder_box.go`); `internal/raster` (`tile.go`, `pool.go`, `scheduler.go`); `internal/frame` (`Bitmap`, `BitmapPool`, `BlendOver`/`BlitOver`); headless TDD + `testdata/parity.py`.

**Spec:** `docs/superpowers/specs/2026-11-12-sp8-group-opacity-compositing-design.md`.

**Execution mode:** standing autonomous delegation. Self-approve every gate, commit directly on `main`, **NO push**. Chat in Vietnamese; this document and code comments in English. `gofmt -w` before every commit. **Never** pipe test output through `tail` — redirect to a `/tmp` log and `echo $?`, then read the log with the Read tool. Build check is `go build -o /dev/null ./...`. Revert mutants by re-editing (never `git checkout`).

### Two design refinements over the spec (both keep the spec's observable API identical)

1. **Sort-survival uses an append-sequence id, not a `GroupID` stamp.** The spec recorded `[Start,End)` as absolute indices during the walk and claimed they stay valid after `List.SortStable`. Absolute indices *shift* when a higher-`Z` sibling is moved ahead of a group, even though a clean group's own commands stay contiguous. So each command carries a stable `Seq int32` (its append ordinal), a recorded group stores the ordinal span `[first,last)`, and `Build` reconstructs each group's sorted `[Start,End)` from the inverse permutation. Contiguity is guaranteed by the clean-group rule (§Task 2), so `End = Start + (last-first)`. This changes no exported behavior.
2. **A nested group that would share its head command with an enclosing recorded group is not recorded** (its own fade threads per-command instead). This only occurs for a decoration-less faded container whose first painted command is itself a faded subgroup; the outer group is kept and the inner collapses into it. The result never loses a fade — it only occasionally fails to dedupe a second-order overlap — so it is always safe and never worse than today.

---

## File Structure

- `internal/paint/cmd.go` — add `DisplayCmd.Seq int32`.
- `internal/paint/list.go` — add `Group` type, `pendingGroup`, `List.pending`, `List.AddGroup`, `Seq` assignment in `Append`, `Reset` clears `pending`, group reconstruction in `Build`, `LayerDL.groups`/`firstGroupAt` + `Groups()`/`GroupAt(i)`, `sameCommand` ignores `Seq`.
- `internal/paint/builder_box.go` — clean-group recording: `isCleanFadedGroup` DFS, opacity threading at a group boundary, `AddGroup` call, `groupHeadStack` guard.
- `internal/raster/tile.go` — extract `drawCommand`; add `rasterizeTile` (scratch param), `compositeGroup`, `fadePixels`, `acquireScratch`; `RasterizeTile` delegates with a nil pool.
- `internal/raster/pool.go` — `Job` gains `Scratch *frame.BitmapPool`; `DefaultRaster` passes `j.Scratch`.
- `internal/raster/scheduler.go` — `Scheduler.groupScratch *frame.BitmapPool`; create in `NewScheduler`; set `Scratch` on the submitted `Job`.
- `internal/paint/list.go` `ContentDamage` — group-set diff (§Task 5).
- `test/paint/list_group_test.go` — NEW: reconstruction + sort robustness.
- `test/paint/opacity_paint_test.go` — updated to the group model.
- `test/raster/group_opacity_test.go` — NEW: the `(158,158,255)` / `(191,191,255)` pixel oracle.
- `test/paint/group_damage_test.go` — NEW: opacity-only change repaints.
- `testdata/render/css-features/18-group-opacity-overlap.html` — NEW scored fixture.
- `testdata/parity.py` — `MIN_SCORED_FIXTURES` 106 → 107 (same commit as the fixture).
- `docs/qa-open-defects.json`, `docs/roadmap-v2.md`, spec/plan records — §Task 9.

---

## Task 1: `DisplayCmd.Seq` + the group data model in `paint`

**Files:**
- Modify: `internal/paint/cmd.go:180-181` (after the `Z` field)
- Modify: `internal/paint/list.go` (`List` struct, `Append`, `Reset`, `Build`, `LayerDL`, `sameCommand`)
- Test: `test/paint/list_group_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `test/paint/list_group_test.go`:

```go
package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func lgFill(x0, y0, x1, y1 int32, z uint32) paint.DisplayCmd {
	return paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(x0, y0, x1, y1), Color: frame.RGB(0, 0, 255), Z: z}
}

// A clean group recorded over the last two commands survives a z-sort that moves
// a higher-Z sibling ahead of it: the group's Start/End are recomputed from the
// sorted list, so they describe where the members actually landed.
func TestGroupRangeSurvivesSort(t *testing.T) {
	l := paint.NewList(4)
	// Append a lifted sibling FIRST so the sort reorders the slab, then the two
	// group members, then register the group over the trailing two commands.
	l.Append(lgFill(0, 0, 10, 10, 5))
	l.Append(lgFill(10, 0, 30, 10, 0))
	l.Append(lgFill(12, 0, 28, 10, 0))
	l.AddGroup(0.5, 2)
	dl := l.Build(1).Publish()

	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1", len(gs))
	}
	g := gs[0]
	if g.Start != 0 || g.End != 2 {
		t.Errorf("group range = [%d,%d), want [0,2): the two Z=0 members sort to the front", g.Start, g.End)
	}
	if g.OwnOpacity != 0.5 {
		t.Errorf("OwnOpacity = %v, want 0.5", g.OwnOpacity)
	}
	if _, ok := dl.GroupAt(g.Start); !ok {
		t.Errorf("GroupAt(%d) = false, want the group starting there", g.Start)
	}
	if _, ok := dl.GroupAt(g.End); ok {
		t.Errorf("GroupAt(%d) = true, want false just past the group", g.End)
	}
}

// A group is only what AddGroup declared: an opacity-less list has none.
func TestNoGroupsDeclaredNoneRecorded(t *testing.T) {
	l := paint.NewList(2)
	l.Append(lgFill(0, 0, 10, 10, 0))
	l.Append(lgFill(10, 0, 20, 10, 0))
	dl := l.Build(1).Publish()
	if len(dl.Groups()) != 0 {
		t.Fatalf("Groups() = %d, want 0", len(dl.Groups()))
	}
	if _, ok := dl.GroupAt(0); ok {
		t.Errorf("GroupAt(0) = true on a group-less list")
	}
}
```

- [ ] **Step 2: Run it, confirm it fails to compile**

```bash
go test ./test/paint/ -run 'TestGroupRangeSurvivesSort|TestNoGroupsDeclaredNoneRecorded' >/tmp/sp8-t1.log 2>&1; echo "exit=$?"
```
Expected: FAIL — `AddGroup`, `Groups`, `GroupAt` undefined (compile error).

- [ ] **Step 3: Add `Seq` to `DisplayCmd`**

In `internal/paint/cmd.go`, immediately after the `Z uint32` field (around line 180), add:

```go

	// Seq is the command's append ordinal in the list that produced it. It is set
	// by List.Append and travels with the command through List.SortStable, so a
	// faded group recorded by ordinal range can be re-derived from the *sorted*
	// slab (see List.Build). Excluded from sameCommand: it is positional identity,
	// not painted content.
	Seq int32
```

- [ ] **Step 4: Add the group model to `list.go`**

In `internal/paint/list.go`, replace the `List` struct (lines 20-23) with:

```go
type List struct {
	cmds   []DisplayCmd
	bounds frame.Rect
	// pending holds the groups registered during the walk, each as an append-
	// ordinal span. Build re-derives them as sorted index spans and clears this.
	pending []pendingGroup
}

// Group is one clean faded opacity group: the contiguous half-open command range
// [Start,End) in paint order that an opacity-declaring box emitted, to be faded
// once by OwnOpacity and composited onto the destination by the rasterizer.
// Bounds is the device-pixel union of the range's command bounds (a tile fast
// reject and a damage hint). Parent is the enclosing group's index in Groups(),
// or -1 for a top-level group. Only clean groups are recorded (§Task 2).
type Group struct {
	OwnOpacity float32
	Bounds     frame.Rect
	Start      int
	End        int
	Parent     int
}

// pendingGroup is a group as the walk leaves it: its own opacity over the append
// ordinals [first,last), before the list has been sorted.
type pendingGroup struct {
	own        float32
	first, last int32
}
```

Replace `Append` (lines 51-60) with:

```go
func (l *List) Append(c DisplayCmd) {
	c.Seq = int32(len(l.cmds))
	l.cmds = append(l.cmds, c)
	if b, ok := c.Bounds(); ok {
		if len(l.cmds) == 1 || l.bounds.Empty() {
			l.bounds = b
		} else {
			l.bounds = l.bounds.Union(b)
		}
	}
}

// AddGroup records a clean faded group covering the most recent count commands,
// faded once by own opacity at composite time. The builder calls it right after
// painting a clean opacity box, whose whole subtree is exactly those commands.
// A count that does not resolve to a positive opacity is a no-op: an opaque or
// fully transparent box is not a group (the latter is dropped before it paints).
func (l *List) AddGroup(own float32, count int) {
	if count <= 0 || own <= 0 || own >= 1 {
		return
	}
	last := int32(len(l.cmds))
	l.pending = append(l.pending, pendingGroup{own: own, first: last - int32(count), last: last})
}
```

Replace `Reset` (lines 39-47) — add clearing `pending` after the existing loop:

```go
func (l *List) Reset() {
	// Zero before reuse so a stale Glyphs slice or image reference cannot be
	// retained by the backing array and keep decoded pixels alive.
	for i := range l.cmds {
		l.cmds[i] = DisplayCmd{}
	}
	l.cmds = l.cmds[:0]
	l.bounds = frame.Rect{}
	l.pending = l.pending[:0]
}
```

Replace `Build` (lines 100-117) with:

```go
func (l *List) Build(version uint64) *LayerDL {
	dl := &LayerDL{cmds: l.cmds, version: version, bounds: l.bounds}
	// Freeze each command's bounds once: Intersecting runs per tile per frame,
	// and a text command's bounds walk its glyphs, so recomputing them there
	// costs O(tiles x commands x glyphs) per frame. The sidecar pays the same
	// walk exactly once per content version; values are identical to Bounds().
	var cb []cmdBounds
	if n := len(l.cmds); n > 0 {
		cb = make([]cmdBounds, n)
		for i := range l.cmds {
			r, ok := l.cmds[i].Bounds()
			cb[i] = cmdBounds{r: r, ok: ok}
		}
		dl.cb = cb
	}
	// Recover groups from the sorted slab. Build is always called after
	// SortStable (engine/paint.go), and a clean group's members share one Z, so
	// they stayed a contiguous run; Start is where the head landed, End Start+len.
	if len(l.pending) > 0 {
		dl.groups, dl.firstGroupAt = reconstructGroups(l.cmds, cb, l.pending)
	}
	l.cmds = nil
	l.bounds = frame.Rect{}
	l.pending = l.pending[:0]
	return dl
}
```

Add the reconstruction helper after `Build`:

```go
// reconstructGroups maps every recorded ordinal span onto the sorted command
// list. posOf[Seq] gives the sorted position of the command appended as ordinal
// Seq; a clean group's span is contiguous there, so End = Start + (last-first).
// Groups are ordered by Start then by End descending, so a parent is emitted
// before each group it encloses; Parent and firstGroupAt fall out of a single
// left-to-right pass with a stack of still-open ranges.
func reconstructGroups(cmds []DisplayCmd, cb []cmdBounds, pending []pendingGroup) ([]Group, []int32) {
	firstGroupAt := make([]int32, len(cmds))
	for i := range firstGroupAt {
		firstGroupAt[i] = -1
	}
	if len(pending) == 0 {
		return nil, firstGroupAt
	}
	posOf := make([]int32, len(cmds))
	for i := range cmds {
		posOf[cmds[i].Seq] = int32(i)
	}
	mapped := make([]Group, 0, len(pending))
	for _, p := range pending {
		start := int(posOf[p.first])
		end := start + int(p.last-p.first)
		var b frame.Rect
		for i := start; i < end && i < len(cb); i++ {
			if cb[i].ok {
				if b.Empty() {
					b = cb[i].r
				} else {
					b = b.Union(cb[i].r)
				}
			}
		}
		mapped = append(mapped, Group{OwnOpacity: p.own, Bounds: b, Start: start, End: end, Parent: -1})
	}
	slices.SortStableFunc(mapped, func(a, b int) int {
		switch {
		case mapped[a].Start < mapped[b].Start:
			return -1
		case mapped[a].Start > mapped[b].Start:
			return 1
		case mapped[a].End > mapped[b].End:
			return -1
		case mapped[a].End < mapped[b].End:
			return 1
		}
		return 0
	})
	groups := make([]Group, 0, len(mapped))
	var stack []int
	for idx := range mapped {
		g := mapped[idx]
		for len(stack) > 0 && groups[stack[len(stack)-1]].End <= g.Start {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			g.Parent = stack[len(stack)-1]
		}
		groups = append(groups, g)
		firstGroupAt[g.Start] = int32(len(groups) - 1)
		stack = append(stack, len(groups)-1)
	}
	return groups, firstGroupAt
}
```

- [ ] **Step 5: Add the `LayerDL` fields and accessors**

In `internal/paint/list.go`, extend the `LayerDL` struct (lines 124-133) — add two fields after `cb`:

```go
	// groups holds the clean faded groups reconstructed by Build, in paint order.
	// A list with no groups leaves it nil.
	groups []Group
	// firstGroupAt[i] is the index into groups of the group starting at command
	// i, or -1. It lets the rasterizer find a group's start in O(1) while walking
	// the list once, without a scan per command.
	firstGroupAt []int32
```

Add accessors after the existing `At`/`All` methods (around line 176):

```go
// Groups returns the clean faded groups in paint order. Nested groups appear
// after their parent and inside its [Start,End). Nil when the list has none.
func (d *LayerDL) Groups() []Group { return d.groups }

// GroupAt returns the group whose range starts at command i. The rasterizer
// jumps the whole range when a group starts at the walk index, so nested groups
// are discovered by containment rather than a separate traversal.
func (d *LayerDL) GroupAt(i int) (Group, bool) {
	if i < 0 || i >= len(d.firstGroupAt) {
		return Group{}, false
	}
	gi := d.firstGroupAt[i]
	if gi < 0 {
		return Group{}, false
	}
	return d.groups[gi], true
}
```

- [ ] **Step 6: Leave `sameCommand` content-only (do not compare `Seq`)**

Confirm `sameCommand` (lines 224-234) does not reference `Seq` — it already does not; `Seq` is positional and must not dirty damage. No edit. (Task 5 will add the *group* diff separately.)

- [ ] **Step 7: gofmt, build, run the test**

```bash
gofmt -w internal/paint/cmd.go internal/paint/list.go
go build -o /dev/null ./...
go test ./test/paint/ -run 'TestGroupRangeSurvivesSort|TestNoGroupsDeclaredNoneRecorded' >/tmp/sp8-t1b.log 2>&1; echo "exit=$?"
```
Expected: build exit 0, test exit 0 (PASS).

- [ ] **Step 8: Run the full paint + raster unit suites for regressions**

```bash
go test ./test/paint/ ./internal/raster/ >/tmp/sp8-t1c.log 2>&1; echo "exit=$?"
```
Expected: PASS (raster unaffected: nothing reads `Groups()` yet; `Intersecting`/`CachedIntersects` unchanged).

- [ ] **Step 9: Commit**

```bash
git add internal/paint/cmd.go internal/paint/list.go test/paint/list_group_test.go
git commit -m "feat(paint): carry faded groups through the sorted display list"
```

---

## Task 2: Clean-group recording in the builder

**Files:**
- Modify: `internal/paint/builder_box.go` (`paintBox` lines ~80-134; add `isCleanFadedGroup`, `groupHeadStack`)
- Test: `test/paint/opacity_paint_test.go` (rewrite per-command assertions to the group model; add fallback test)
- Test: `test/paint/group_paint_test.go` (create — clean/nested/fallback oracles)

- [ ] **Step 1: Write the failing paint oracle**

Create `test/paint/group_paint_test.go`. It reuses the frozen-list path `visDisplayList` already uses (`engine.NewSession` → `sess.PaintChecked(1.0)` → `dl.Build(1)`), but publishes to a `*LayerDL` so `Groups()`/`GroupAt` are visible. The list `PaintChecked` returns is already `SortStable`'d (engine/paint.go:105), so `Build` reconstructs groups in sorted order.

```go
package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// gpLayer builds a frozen display list (so groups are visible) from html,
// mirroring visDisplayList but returning the *LayerDL rather than list.All().
func gpLayer(t *testing.T, html string) *paint.LayerDL {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	list, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	return list.Build(1).Publish()
}

// A clean faded box emits exactly one group whose own opacity is the declared
// fraction, and its member commands carry NO opacity of their own: the group
// fade is withheld from the per-command field and applied once at composite.
func TestCleanFadedBoxRecordsOneGroup(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.g{opacity:0.5;width:100px;height:50px;background:rgb(200,0,0)}
	</style></head><body><div class="g"></div></body></html>`
	dl := gpLayer(t, html)
	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (one clean faded declaration)", len(gs))
	}
	if gs[0].OwnOpacity != 0.5 {
		t.Errorf("group OwnOpacity = %v, want 0.5", gs[0].OwnOpacity)
	}
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Opacity != 0 {
			t.Errorf("member fill carried per-command opacity %v, want 0 (withheld; the group fades it once)", c.Opacity)
		}
	}
}

// A faded box with a positioned z-index>0 descendant is NOT clean: recording it
// would break the contiguous-same-Z invariant the group model relies on, so it
// falls back to today's per-command path (its opacity reaches DisplayCmd.Opacity).
func TestFadedBoxWithZDescendantIsNotGrouped(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.g{opacity:0.5;width:120px;height:80px;background:rgb(200,0,0)}
	.k{position:relative;z-index:3;width:40px;height:20px;background:rgb(0,0,200)}
	</style></head><body><div class="g"><div class="k"></div></div></body></html>`
	dl := gpLayer(t, html)
	if len(dl.Groups()) != 0 {
		t.Fatalf("Groups() = %d, want 0 (a z-lifted descendant makes the box non-clean)", len(dl.Groups()))
	}
	saw := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Opacity == 0.5 {
			saw = true
		}
	}
	if !saw {
		t.Errorf("fallback did not thread opacity per-command: no fill carried 0.5")
	}
}

// Two translucent children overlapping inside one clean container: the container
// is a single group spanning both, and neither child carries its own fade.
func TestTwoChildrenOneContainerGroup(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
	body{margin:0}
	.c{opacity:0.5;width:120px;height:60px}
	.a{position:absolute;left:0;top:0;width:60px;height:60px;background:rgb(0,0,255)}
	.b{position:absolute;left:30px;top:0;width:60px;height:60px;background:rgb(0,0,255)}
	</style></head><body><div class="c"><div class="a"></div><div class="b"></div></div></body></html>`
	dl := gpLayer(t, html)
	gs := dl.Groups()
	if len(gs) != 1 {
		t.Fatalf("Groups() = %d, want 1 (the container only; the children have no opacity of their own)", len(gs))
	}
}
```

- [ ] **Step 2: Run it, confirm the group tests fail**

```bash
go test ./test/paint/ -run 'TestCleanFadedBoxRecordsOneGroup|TestFadedBoxWithZDescendantIsNotGrouped|TestTwoChildrenOneContainerGroup' >/tmp/sp8-t2.log 2>&1; echo "exit=$?"
```
Expected: FAIL — today every faded box threads opacity per command and records zero groups, so `Groups()` is empty and member fills carry `Opacity==0.5`.

- [ ] **Step 3: Add the group-head guard field to the builder**

In `internal/paint/builder.go`, extend the `Builder` struct (lines 13-32) with a field:

```go
	// groupHeadStack holds the commandStart ordinal of each currently-open clean
	// faded group, outermost first. A nested group that would share its head
	// command with an enclosing recorded group is not recorded separately (see
	// paintBox); keeping the outer fade is always safe and never double-counts.
	groupHeadStack []int32
```

- [ ] **Step 4: Add the clean-subtree DFS**

In `internal/paint/builder_box.go`, add after `zIndexOf` (lines 140-145):

```go
// isCleanFadedGroup reports whether obj's subtree contains no descendant with a
// lifting z-index. It is the precondition for recording a faded group: a clean
// group's every command carries the box's single Z, so List.SortStable keeps the
// whole range contiguous and the rasterizer can render [Start,End) as one buffer.
// The walk is bounded by the same arena depth the paint walk already allows, so
// it adds no unbounded recursion. A false read is always safe — the box simply
// falls back to the per-command path.
func (b *Builder) isCleanFadedGroup(obj *layout.Object) bool {
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		k := b.arena.Get(kid)
		if _, ok := zIndexOf(k.Style); ok {
			return false
		}
		if !b.isCleanFadedGroup(k) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Thread opacity and record at a group boundary**

In `internal/paint/builder_box.go`, replace the opacity block (lines 90-102) with:

```go
	// Group opacity: an element's own opacity multiplies the accumulated opacity
	// of every ancestor. When this box is a *clean* faded group we withhold its
	// own fraction from the threaded value and record it instead, so the group is
	// faded once offscreen at composite time rather than per command (which would
	// double-darken overlapping children). A non-clean faded box keeps threading
	// own × inherited into every command exactly as before.
	effective := inheritedOpacity
	groupOpacity := float32(0)
	if boxDeclaresOpacity(obj) {
		o := obj.Style.Opacity
		if o < 0 {
			o = 0
		} else if o > 1 {
			o = 1
		}
		if o > 0 && o < 1 && b.isCleanFadedGroup(obj) {
			groupOpacity = o
		} else {
			effective *= o
		}
	}
	if effective <= 0 {
		return
	}
```

Replace the paint block (lines 110-134) with:

```go
	childClip := inheritedClip
	commandStart := b.list.Len()
	// A nested group that would open on the same command as an enclosing
	// recorded group is not recorded: its fade threads per-command inside the
	// outer buffer instead. Safe (never loses a fade), and the only case it
	// touches is a decoration-less faded container whose first command is an
	// already-grouped faded child.
	recordThis := groupOpacity > 0 && !(len(b.groupHeadStack) > 0 && b.groupHeadStack[len(b.groupHeadStack)-1] == int32(commandStart))
	if obj.Style != nil {
		box := b.paintContextFor(obj, effective)
		start := b.list.Len()
		if !box.hidden && !visibilityHidden {
			b.paintBoxDecoration(obj, box, ordinal)
			b.paintBoxContents(obj, box, inheritedClip)
		}
		b.applyBoxEffects(obj, box, start)
		if inheritedClip != nil {
			clipCommands(b.list, start, *inheritedClip)
		}
		childClip = b.overflowClip(obj, childClip)
	}
	if recordThis {
		b.groupHeadStack = append(b.groupHeadStack, int32(commandStart))
	}
	b.paintChildren(obj, childClip, effective)
	if recordThis {
		b.groupHeadStack = b.groupHeadStack[:len(b.groupHeadStack)-1]
		b.list.AddGroup(groupOpacity, b.list.Len()-commandStart)
	}
	if z, ok := zIndexOf(obj.Style); ok {
		commands := b.list.All()
		for i := commandStart; i < len(commands); i++ {
			if commands[i].Z == 0 {
				commands[i].Z = z
			}
		}
	}
```

Note: the original block ran `b.paintChildren` inside the `if obj.Style != nil` area at line 125; it is now hoisted so the head-stack push/pop wraps the children call for every box (boxes with nil Style cannot be groups, so `recordThis` is false for them and the push/pop is skipped). Keep `paintChildren` at the same place relative to the decoration block.

- [ ] **Step 6: gofmt + build + group tests**

```bash
gofmt -w internal/paint/builder.go internal/paint/builder_box.go test/paint/group_paint_test.go
go build -o /dev/null ./...
go test ./test/paint/ -run 'TestCleanFadedBoxRecordsOneGroup|TestFadedBoxWithZDescendantIsNotGrouped|TestTwoChildrenOneContainerGroup' >/tmp/sp8-t2b.log 2>&1; echo "exit=$?"
```
Expected: PASS.

- [ ] **Step 7: Update `opacity_paint_test.go` to the group model**

Read `test/paint/opacity_paint_test.go` in full, then apply these edits (keep `TestOpacityZeroDropsBox` and `TestOpacityAboveOnePaintsOpaque` **unchanged** — `opacity:0` still paints nothing, `opacity:5` clamps to 1 and is not a group so its command still carries 1):

- `TestOpacityFractionReachesCommand` (lines 55-72): a clean faded `div` now records a group and the fill carries `Opacity==0` (residual). Rewrite to build a `*LayerDL` and assert `len(dl.Groups())==1` with `OwnOpacity==0.5` for each shape, instead of reading `cmd.Opacity`.
- `TestGroupOpacityAppliesToSubtree` (lines 98-118): rewrite so the parent's `.5` is asserted as `Groups()[0].OwnOpacity==0.5` spanning both the parent and child fills, and both member `DisplayCmd.Opacity==0`.
- `TestGroupOpacityCountedOncePerElement` (lines 120-224): the counting guarantee moves from "each command carries 0.5" to "exactly one group with OwnOpacity==want and no command carries a stray fade." Replace the per-command `want` check with: assert `len(dl.Groups())==1`, `Groups()[0].OwnOpacity == s.want`, and every `CmdFill`/`CmdText` has `Opacity==0`. Keep all shape bodies and Chromium measurement comments; update the doc comment to reference the group model and `GOOSIE-OPACITY-002` (count once per element) plus `GOOSIE-COMPOSITE-001` (fade once per group). `gpLayer` (from `group_paint_test.go`, same `paint_test` package) is the helper to build the frozen list.

Concretely, replace the tail of `TestGroupOpacityCountedOncePerElement`'s loop body (lines 209-222) with:

```go
		t.Run(s.name, func(t *testing.T) {
			html := `<!DOCTYPE html><html><head><style>html,body{margin:0}body{font:16px serif;color:#000}` +
				s.css + `</style></head><body>` + s.body + `</body></html>`
			dl := gpLayer(t, html)
			gs := dl.Groups()
			if len(gs) != 1 {
				t.Fatalf("shape painted %d groups, want exactly 1 (the declaration is counted once, whatever boxes layout inserts)", len(gs))
			}
			if !opClose(gs[0].OwnOpacity, s.want) {
				t.Errorf("group OwnOpacity = %v, want %v (Chromium %s)", gs[0].OwnOpacity, s.want, s.chromium)
			}
			for i, c := range dl.All() {
				if (c.Kind == paint.CmdText || c.Kind == paint.CmdFill) && c.Opacity != 0 {
					t.Errorf("command %d carried per-command opacity %v, want 0 (the fade lives on the group and is applied once)", i, c.Opacity)
				}
			}
		})
```

If the `tableAnon` shape (`<td style="opacity:0.5">raw cell text<div>block</div></td>`) has a non-clean subtree or emits more than one group, keep it as a fallback shape: change its `want` assertion by allowing either one group at 0.5 **or** (non-clean) every command at 0.5 with zero groups. Add a small branch and a comment noting the table wrapper case may fall back. Re-run and confirm each shape.

- [ ] **Step 8: Run the whole opacity + paint suites**

```bash
gofmt -w test/paint/opacity_paint_test.go
go test ./test/paint/ >/tmp/sp8-t2c.log 2>&1; echo "exit=$?"
```
Expected: PASS (all opacity shapes under the group model, fallback branch green).

- [ ] **Step 9: Commit**

```bash
git add internal/paint/builder.go internal/paint/builder_box.go test/paint/group_paint_test.go test/paint/opacity_paint_test.go
git commit -m "feat(paint): record clean faded opacity groups in the display walk"
```

---

## Task 3: Extract `drawCommand` in the rasterizer (pure refactor)

**Files:**
- Modify: `internal/raster/tile.go:57-93` (the loop body and switch)
- Test: existing `test/raster/tile_test.go`, `test/raster/mask_test.go` (regression gate; no new behavior)

- [ ] **Step 1: Write the refactor as a behavior-preserving extraction**

In `internal/raster/tile.go`, move the per-command dispatch (currently the `switch c.Kind { ... }` block and its preceding `cmdClip` computation, lines 61-92) into a new unexported function, and call it from the loop:

```go
// drawCommand paints one command into dst at the tile-local offset (dx,dy),
// clipped to tileClip (dst's own bounds) intersected with the command's own
// Clip. It is the single place a command's Kind becomes pixels, so an ordinary
// tile draw and a group scratch draw are guaranteed identical.
func drawCommand(dst *frame.Bitmap, dl *paint.LayerDL, i int, dx, dy int32, tileClip frame.Rect, f *Fonts, g *GlyphAtlas) error {
	c := dl.At(i)
	cmdClip := tileClip
	if !c.Clip.Empty() {
		cmdClip = tileClip.Intersection(c.Clip.Translate(dx, dy))
		if cmdClip.Empty() {
			return nil
		}
	}
	switch c.Kind {
	case paint.CmdFill:
		if c.Mask != nil {
			drawMaskedFill(dst, c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), c.Mask, cmdClip)
		} else if c.Radius.Empty() {
			dst.FillRect(c.Rect.Translate(dx, dy), applyOpacity(c.Color, c.Opacity), nil)
		} else {
			dst.FillRounded(c.Rect.Translate(dx, dy), c.Radius, applyOpacity(c.Color, c.Opacity), nil)
		}
	case paint.CmdBorder:
		drawBorder(dst, &c, dx, dy, cmdClip)
	case paint.CmdText:
		return drawText(dst, &c, dx, dy, cmdClip, f, g)
	case paint.CmdImage:
		drawImage(dst, &c, dx, dy, cmdClip)
	case paint.CmdGradient:
		dst.FillLinearGradient(c.Rect.Translate(dx, dy), c.Radius, c.Gradient, nil)
	case paint.CmdRadialGradient:
		dst.FillRadialGradient(c.Rect.Translate(dx, dy), c.Radius, c.RadialGradient, nil)
	}
	return nil
}
```

Then the `RasterizeTile` loop (lines 57-93) becomes:

```go
	for i := 0; i < dl.Len(); i++ {
		if !dl.CachedIntersects(i, b) {
			continue
		}
		if err := drawCommand(out, dl, i, dx, dy, clip, f, g); err != nil {
			return err
		}
	}
```

`drawBorder`, `drawText`, `drawImage`, `drawMaskedFill`, `applyOpacity`, `opacityCoverage` are unchanged; note `drawBorder`/`drawText`/`drawImage` already take the destination as their first arg, so no signature change. `c` in those helpers was `&c` — now it is `&cmdcopy` inside `drawCommand`; keep passing a pointer to the local value.

- [ ] **Step 2: gofmt + build + raster suite**

```bash
gofmt -w internal/raster/tile.go
go build -o /dev/null ./...
go test ./test/raster/ ./internal/raster/ >/tmp/sp8-t3.log 2>&1; echo "exit=$?"
```
Expected: PASS — no group logic yet, `GroupAt` returns false for every hand-built list, so output is byte-identical to before the extraction.

- [ ] **Step 3: Commit**

```bash
git add internal/raster/tile.go
git commit -m "refactor(raster): extract drawCommand so tiles and groups share one path"
```

---

## Task 4: Offscreen group composite + pooled scratch

**Files:**
- Modify: `internal/raster/tile.go` (`RasterizeTile` → `rasterizeTile`, `compositeGroup`, `fadePixels`, `acquireScratch`)
- Modify: `internal/raster/pool.go` (`Job.Scratch`, `DefaultRaster`)
- Modify: `internal/raster/scheduler.go` (`groupScratch` field, `NewScheduler`, Job at `:508`)
- Test: `test/raster/group_opacity_test.go` (create — the pixel oracle)

- [ ] **Step 1: Write the failing pixel oracle**

Create `test/raster/group_opacity_test.go`:

```go
package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

// GOOSIE-COMPOSITE-001 measured oracle (Chromium, 300x120, DPR 1, white):
// a container opacity:0.5 holding two rgba(0,0,255,0.5) 60x60 boxes overlapping
// 30px. Non-overlap pixel = (191,191,255); overlap pixel = (158,158,255). Before
// offscreen compositing Goosie produced (143,143,255) in the overlap (each box
// pre-faded to 0.25, then 0.25-over-0.25 = 0.4375).
func TestGroupOpacityOverlapComposite(t *testing.T) {
	blue := frame.RGBA(0, 0, 255, 128) // rgba(0,0,255,0.5) premultiplied
	l := paint.NewList(3)
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: blue})
	l.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: blue})
	l.AddGroup(0.5, 2)
	dl := l.Build(1).Publish()

	out := frame.NewBitmap(120, 80)
	if err := raster.RasterizeTile(dl, frame.Rect4(0, 0, 120, 80), out, nil, nil, frame.RGB(255, 255, 255)); err != nil {
		t.Fatalf("RasterizeTile: %v", err)
	}
	if got := out.At(15, 40); got != frame.RGB(191, 191, 255) {
		t.Errorf("non-overlap pixel = %v, want (191,191,255)", got)
	}
	if got := out.At(50, 40); got != frame.RGB(158, 158, 255) {
		t.Errorf("overlap pixel = %v, want (158,158,255) (the double-darkening defect)", got)
	}
}
```

- [ ] **Step 2: Run it, confirm it fails**

```bash
go test ./test/raster/ -run TestGroupOpacityOverlapComposite >/tmp/sp8-t4.log 2>&1; echo "exit=$?"
```
Expected: FAIL — the overlap reads `(143,143,255)` today (per-command fade), because `RasterizeTile` ignores groups.

- [ ] **Step 3: Add the scratch helpers and group walk to `tile.go`**

In `internal/raster/tile.go`, replace the exported `RasterizeTile` (lines 35-95) with a thin delegator plus the real work, and add the helpers:

```go
// RasterizeTile paints the tile covering bounds into out. It is the public
// entry point every tool and test calls, and it holds no group scratch pool, so
// faded groups fall back to a one-off on-demand buffer — a slower path that
// exists so a one-off caller (PDF export, a unit test) can rasterize without
// building a pool. The frame path passes a pooled scratch through DefaultRaster.
func RasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts, g *GlyphAtlas, bg frame.Color) error {
	return rasterizeTile(dl, bounds, out, f, g, bg, nil)
}

func rasterizeTile(dl *paint.LayerDL, bounds frame.Rect, out *frame.Bitmap, f *Fonts, g *GlyphAtlas, bg frame.Color, scratch *frame.BitmapPool) error {
	if out == nil {
		return ErrNilDestination
	}
	if out.Empty() {
		return fmt.Errorf("raster: tile buffer has no pixels for %v", bounds)
	}
	if bg.Opaque() {
		out.FillRect(out.Bounds(), bg, nil)
	} else {
		out.Reset()
	}
	if dl == nil || dl.Len() == 0 {
		return nil
	}
	b := bounds.Canon()
	dx, dy := -b.X0, -b.Y0
	clip := out.Bounds()
	for i := 0; i < dl.Len(); i++ {
		if grp, ok := dl.GroupAt(i); ok {
			if err := compositeGroup(dl, grp, b, out, dx, dy, clip, f, g, scratch); err != nil {
				return err
			}
			i = grp.End - 1 // the loop's i++ lands on the first command past the group
			continue
		}
		if !dl.CachedIntersects(i, b) {
			continue
		}
		if err := drawCommand(out, dl, i, dx, dy, clip, f, g); err != nil {
			return err
		}
	}
	return nil
}

// compositeGroup renders grp's [Start,End) into a scratch buffer the size of dst,
// fading overlapping members against each other (not against the tile), applies
// grp's own opacity once across the buffer, then blends it onto dst. Nested
// groups recurse with their own scratch. A tile-sized scratch means a group that
// straddles tiles is faded per-tile, which is per-pixel and so still exact.
func compositeGroup(dl *paint.LayerDL, grp paint.Group, tile frame.Rect, dst *frame.Bitmap, dx, dy int32, clip frame.Rect, f *Fonts, g *GlyphAtlas, pool *frame.BitmapPool) error {
	if !grp.Bounds.Empty() && !grp.Bounds.Intersects(tile) {
		return nil // whole group misses this tile
	}
	sc, pooled := acquireScratch(dst, pool)
	if pooled {
		defer pool.Release(sc)
	}
	sc.Reset()
	scClip := sc.Bounds()
	for i := grp.Start; i < grp.End; i++ {
		if child, ok := dl.GroupAt(i); ok {
			if err := compositeGroup(dl, child, tile, sc, dx, dy, scClip, f, g, pool); err != nil {
				return err
			}
			i = child.End - 1
			continue
		}
		if !dl.CachedIntersects(i, tile) {
			continue
		}
		if err := drawCommand(sc, dl, i, dx, dy, scClip, f, g); err != nil {
			return err
		}
	}
	fadePixels(sc, grp.OwnOpacity)
	dst.BlitOver(sc, frame.Point{X: 0, Y: 0}, sc.Bounds(), clip, nil)
	return nil
}

// acquireScratch returns a buffer the same size as dst. With a matching pool it
// is pooled (the steady-state frame path must not allocate); without one it is a
// one-off NewBitmap for a tool or test rasterizing a single tile.
func acquireScratch(dst *frame.Bitmap, pool *frame.BitmapPool) (*frame.Bitmap, bool) {
	if pool != nil && pool.Size() == dst.Size() {
		return pool.Acquire(), true
	}
	return frame.NewBitmap(dst.W, dst.H), false
}

// fadePixels multiplies every pixel of b by o (premultiplied scale, the same
// operation applyOpacity performs per command). o in (0,1) is the only value a
// recorded group carries.
func fadePixels(b *frame.Bitmap, o float32) {
	if o >= 1 {
		return
	}
	if o <= 0 {
		b.Reset()
		return
	}
	for y := 0; y < b.H; y++ {
		base := y * b.Stride
		for x := 0; x < b.W; x++ {
			off := base + x*4
			c := frame.Color(uint32(b.RGBA[off])<<24 | uint32(b.RGBA[off+1])<<16 | uint32(b.RGBA[off+2])<<8 | uint32(b.RGBA[off+3]))
			c = applyOpacity(c, o)
			b.RGBA[off] = byte(c >> 24)
			b.RGBA[off+1] = byte(c >> 16)
			b.RGBA[off+2] = byte(c >> 8)
			b.RGBA[off+3] = byte(c)
		}
	}
}
```

`drawCommand` (Task 3) is now called with `(dst, dl, i, ...)` — confirm the extraction kept that exact signature. Remove the now-duplicated inline `switch` from the old loop body (already replaced).

- [ ] **Step 4: Thread the scratch pool through `Job` and `DefaultRaster`**

In `internal/raster/pool.go`, extend the `Job` struct (lines 38-45) with:

```go
	// Scratch is a tile-sized buffer pool the rasterizer borrows to composite a
	// faded group offscreen, acquired/released per group so nothing is cached in a
	// package global. nil means "allocate a one-off" (tools, tests that build a Job
	// by hand); the frame path always supplies one sized to the tile.
	Scratch *frame.BitmapPool
```

Change `DefaultRaster` (lines 74-78) to pass it:

```go
func DefaultRaster(f *Fonts, g *GlyphAtlas) RasterFunc {
	return func(j Job) error {
		return rasterizeTile(j.DL, j.Bounds, j.Out, f, g, j.Background, j.Scratch)
	}
}
```

`DefaultRaster`'s exported signature is unchanged, so no call site edits are needed outside `scheduler.go`.

- [ ] **Step 5: Own the scratch pool in the scheduler and set it on the Job**

In `internal/raster/scheduler.go`, add a field to the `Scheduler` struct near the other buffers (after line 79):

```go
	// groupScratch is the pool every raster job borrows one tile-sized buffer from
	// to composite a faded group. It is created once in NewScheduler, so a warm
	// scroll reuses buffers and the frame path allocates nothing (invariant 6).
	// Named apart from the `scratch int64` alloc counter it sits next to.
	groupScratch *frame.BitmapPool
```

In `NewScheduler` (lines 169-193), before `s.reserveScratch(false)`, size the pool to the tile:

```go
	ts := frame.TileSize
	if l != nil && l.Grid != nil {
		if n := l.Grid.TileSize(); n > 0 {
			ts = n
		}
	}
	s.groupScratch = frame.NewBitmapPool(frame.Size{W: ts, H: ts}, 32)
```

At the Job construction (line 508) add `Scratch: s.groupScratch`:

```go
		if err := s.pool.Submit(Job{Layer: s.layer, Coord: c, DL: dl, Bounds: c.Rect(ts), Out: out, Background: s.cur.Background, Scratch: s.groupScratch}); err != nil {
```

(If `ts` is not in scope at that line, read the tile size into a local there the same way it is obtained for `c.Rect(ts)` already used in the surrounding loop.)

- [ ] **Step 6: gofmt + build + pixel oracle + raster suite**

```bash
gofmt -w internal/raster/tile.go internal/raster/pool.go internal/raster/scheduler.go test/raster/group_opacity_test.go
go build -o /dev/null ./...
go test ./test/raster/ ./internal/raster/ -run 'Group|Tile|Mask|Pool|Scheduler' >/tmp/sp8-t4b.log 2>&1; echo "exit=$?"
```
Expected: PASS — overlap `(158,158,255)`, non-overlap `(191,191,255)`; every pre-existing tile/pool/scheduler test still green (no groups in hand-built jobs → `acquireScratch` one-off path only fires for group tiles; scheduler warm-scroll alloc count unchanged because `groupScratch` is created in setup, not the frame loop).

- [ ] **Step 7: Race check on raster + the scroll gate**

```bash
go test -race ./test/raster/ ./internal/raster/ >/tmp/sp8-t4c.log 2>&1; echo "exit=$?"
go test ./test/gate -run 'TestScrollGate|TestScratch' >/tmp/sp8-t4d.log 2>&1; echo "exit=$?"
```
Expected: PASS (scratch is per-buffer from a mutex-guarded pool; `ScratchAllocs()` still 0).

- [ ] **Step 8: Commit**

```bash
git add internal/raster/tile.go internal/raster/pool.go internal/raster/scheduler.go test/raster/group_opacity_test.go
git commit -m "feat(raster): composite faded opacity groups offscreen with a pooled tile scratch"
```

---

## Task 5: Damage correctness for group opacity changes

**Files:**
- Modify: `internal/paint/list.go` (`ContentDamage`, add `groupRangesDirty`/`addGroupBounds` helpers)
- Test: `test/paint/group_damage_test.go` (create)

- [ ] **Step 1: Write the failing damage test**

Create `test/paint/group_damage_test.go`:

```go
package paint_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// Two identical command lists whose only difference is a group's OWN opacity must
// still report damage over the group's range, because OwnOpacity lives outside
// the command fields and sameCommand never sees it.
func TestContentDamageGroupOpacityOnlyChange(t *testing.T) {
	before := paint.NewList(2)
	before.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: frame.RGB(0, 0, 255)})
	before.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: frame.RGB(0, 0, 255)})
	before.AddGroup(0.5, 2)

	after := paint.NewList(2)
	after.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(10, 10, 70, 70), Color: frame.RGB(0, 0, 255)})
	after.Append(paint.DisplayCmd{Kind: paint.CmdFill, Rect: frame.Rect4(40, 10, 100, 70), Color: frame.RGB(0, 0, 255)})
	after.AddGroup(0.25, 2)

	bo, ao := before.Build(1), after.Build(1)
	got := paint.ContentDamage(bo, ao)
	want := frame.Rect4(10, 10, 100, 70)
	if got != want {
		t.Fatalf("damage = %v, want the group's command-range union %v (opacity-only change)", got, want)
	}
}
```

- [ ] **Step 2: Run it, confirm it fails**

```bash
go test ./test/paint/ -run TestContentDamageGroupOpacityOnlyChange >/tmp/sp8-t5.log 2>&1; echo "exit=$?"
```
Expected: FAIL — commands are identical, so `ContentDamage` returns an empty rect today.

- [ ] **Step 3: Diff the group slices in `ContentDamage`**

In `internal/paint/list.go`, in `ContentDamage` (lines 179-222), after the command-diff path has computed `dirty` for the same-length and general cases, add a group comparison before returning. Refactor the two `return dirty` sites to `goto done` / a labeled tail, or wrap the existing body so it computes `dirty` then, before returning, runs the block below. The added logic:

```go
	// A group's OwnOpacity is stored outside the command fields, so two lists with
	// identical commands can still differ in how a region is faded. Compare the
	// reconstructed groups: any difference in count, own opacity, or range dirties
	// the union of the changed groups' command bounds in both lists. A membership
	// change is already caught by the command diff (the members move), but a pure
	// opacity change would otherwise read as undamaged and leave stale pixels.
	dirty = dirty.Union(groupDamage(old, next))
done:
	return dirty
```

Add the helper after `ContentDamage`:

```go
// groupDamage unions the command-range bounds of every group whose description
// changed between two frozen lists, so an opacity-only mutation repaints. When
// the group counts differ conservatively paint every group's range in both.
func groupDamage(a, b *LayerDL) frame.Rect {
	var dirty frame.Rect
	add := func(d *LayerDL, g Group) {
		for i := g.Start; i < g.End; i++ {
			if i >= 0 && i < len(d.cb) && d.cb[i].ok {
				dirty = dirty.Union(d.cb[i].r)
			}
		}
	}
	if len(a.groups) != len(b.groups) {
		for _, g := range a.groups {
			add(a, g)
		}
		for _, g := range b.groups {
			add(b, g)
		}
		return dirty
	}
	for k := range a.groups {
		if a.groups[k] != b.groups[k] {
			add(a, a.groups[k])
			add(b, b.groups[k])
		}
	}
	return dirty
}
```

`Group` is all comparable scalar/value fields (`float32`, `frame.Rect`, `int`), so `a.groups[k] != b.groups[k]` compiles. Confirm `frame.Rect` is comparable (it is — `Rect` is a value struct used with `==` throughout `sameCommand`).

- [ ] **Step 4: gofmt + build + damage tests**

```bash
gofmt -w internal/paint/list.go test/paint/group_damage_test.go
go build -o /dev/null ./...
go test ./test/paint/ -run 'Group|ContentDamage' >/tmp/sp8-t5b.log 2>&1; echo "exit=$?"
```
Expected: PASS — new group test plus all pre-existing `TestContentDamage*` cases (they have no groups → `groupDamage` returns empty → unchanged).

- [ ] **Step 5: Full paint + raster + engine suite (damage feeds scroll)**

```bash
go test ./test/paint/ ./test/raster/ ./test/gate/ >/tmp/sp8-t5c.log 2>&1; echo "exit=$?"
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/paint/list.go test/paint/group_damage_test.go
git commit -m "fix(paint): repaint the region a group-opacity-only change fades differently"
```

---

## Task 6: Scored parity fixture

**Files:**
- Create: `testdata/render/css-features/18-group-opacity-overlap.html`
- Modify: `testdata/parity.py` (`MIN_SCORED_FIXTURES` 106 → 107, same commit)

- [ ] **Step 1: Write the fixture**

Create `testdata/render/css-features/18-group-opacity-overlap.html`:

```html
<!DOCTYPE html>
<html>
<head>
    <title>Overlapping translucent layers in one faded group (GOOSIE-COMPOSITE-001)</title>
    <style>
        /* A container at opacity:0.5 holding two rgba(0,0,255,0.5) boxes that
           overlap 30px, on white.  Measured against Chromium (viewport 320x200,
           DPR 1): a single box reads (191,191,255) - 0.5 color x 0.5 group = 0.25
           over white.  The overlap must read (158,158,255): blue composited twice
           at 0.5 = 0.75, then the group faded once at 0.5 = 0.375.  The per-command
           defect produced (143,143,255) because each box was pre-faded to 0.25 and
           the two fades compounded.  The container carries no background of its own,
           so the ONLY disagreement the harness can see is the overlap - exactly the
           pixels offscreen compositing changes. */
        html, body { margin: 0; padding: 0; }
        body { background: #ffffff; font-family: sans-serif; color: #222; }
        h1 { font-size: 16px; margin: 8px 12px 2px 12px; }
        p  { font-size: 12px; line-height: 16px; margin: 2px 12px; }
        .scene { position: relative; width: 260px; height: 130px; margin: 12px; }
        .fade  { opacity: 0.5; position: absolute; inset: 0; }
        .box   { position: absolute; width: 90px; height: 90px; background: rgba(0,0,255,0.5); }
        .a { left: 10px; top: 10px; }
        .b { left: 40px; top: 10px; } /* overlaps .a by 60px */
        .row { margin: 10px 12px; }
        .solo { opacity: 0.5; width: 90px; height: 24px; background: rgba(0,0,255,0.5); }
    </style>
</head>
<body>
    <h1>two fades inside one group composite before the group fade</h1>
    <p>The overlap must be the brighter (158,158,255), never the twice-darkened (143,143,255).</p>

    <div class="scene">
        <div class="fade">
            <div class="box a"></div>
            <div class="box b"></div>
        </div>
    </div>

    <div class="row"><div class="solo"></div></div>
    <p>A lone faded box (191,191,255) sits below so the corpus also proves the
       non-overlapping case is unchanged by moving opacity onto a group.</p>
    <p>filler line so the document has ink the harness compares on both engines.</p>
</body>
</html>
```

- [ ] **Step 2: Raise the floor in the same commit**

In `testdata/parity.py`, change `MIN_SCORED_FIXTURES` from `106` to `107` and add the standard one-line comment naming the fixture and defect:

```python
MIN_SCORED_FIXTURES = 107  # 107: css-features/18-group-opacity-overlap.html (SP-8, GOOSIE-COMPOSITE-001 offscreen group fade)
```

- [ ] **Step 3: Confirm the corpus is discoverable + run parity for the new fixture**

```bash
python3 testdata/parity.py >/tmp/sp8-t6.log 2>&1; echo "exit=$?"
```
Expected: exit 0; the run scores ≥107 fixtures, the new `18-group-opacity-overlap` PASSes, and the prior 106 fixtures stay PASS with no band/status/gap change (known gaps stay 4 fixtures / 6 bands). Read `/tmp/sp8-t6.log` with the Read tool. If Chromium is unavailable in this environment, record that and rely on the Task 4 pixel oracle as the deterministic gate (note it in the commit body).

- [ ] **Step 4: Commit**

```bash
git add testdata/render/css-features/18-group-opacity-overlap.html testdata/parity.py
git commit -m "test(render): score the group-opacity overlap fixture against Chromium"
```

---

## Task 7: Mutants — prove each new gate is live

**Files:** (temporary edits, each reverted by re-edit; the tree must end byte-clean)

Run each mutant, confirm the named oracle goes RED, then revert the exact edit. Do not commit any mutant.

- [ ] **Step 1: Disable group composite**

Temporarily make `compositeGroup` in `internal/raster/tile.go` fall through to the old per-command path: change its guard so it draws members straight onto `dst` and never fades/blits (e.g. early `return nil` after the loop that draws into `dst` instead of `sc`, skipping `fadePixels`/`BlitOver`).

```bash
go test ./test/raster/ -run TestGroupOpacityOverlapComposite >/tmp/sp8-m1.log 2>&1; echo "exit=$?"
```
Expected: FAIL — overlap reads `(143,143,255)` again. Revert the edit exactly; re-run the raster suite to confirm PASS.

- [ ] **Step 2: Fade twice (group + per-command)**

Temporarily, in `builder_box.go` Step 5's block, record the group AND still multiply into `effective` (set `effective *= o` inside the clean-group branch, before the record). The overlap now fades per-command *and* as a group.

```bash
go test ./test/raster/ -run TestGroupOpacityOverlapComposite >/tmp/sp8-m2.log 2>&1; echo "exit=$?"
```
Expected: FAIL — overlap reads a third, darker value (both fades applied). Revert; confirm PASS.

- [ ] **Step 3: Skip the clean-group check (accept a non-clean box)**

Temporarily, in `builder_box.go`, replace `b.isCleanFadedGroup(obj)` with `true`. The z-lifted-descendant box becomes a recorded group whose members are NOT contiguous after `SortStable`, so its recovered `[Start,End)` spans foreign commands.

```bash
go test ./test/paint/ -run 'TestFadedBoxWithZDescendantIsNotGrouped' >/tmp/sp8-m3.log 2>&1; echo "exit=$?"
```
Expected: FAIL — a group is now recorded where the test demands none. Revert; confirm PASS.

- [ ] **Step 4: Prove the tree is byte-clean**

```bash
git status --porcelain >/tmp/sp8-m4.log 2>&1; echo "exit=$?"
```
Expected: no modified files (only the intended new/committed files appear as clean). Read the log; if any tracked file is modified, re-revert.

---

## Task 8: Full verification matrix

- [ ] **Step 1: Build + vet**

```bash
go build -o /dev/null ./... >/tmp/sp8-v1.log 2>&1; echo "build=$?"
go vet -all ./... >/tmp/sp8-v2.log 2>&1; echo "vet=$?"
```
Expected: build=0, vet=0.

- [ ] **Step 2: Whole test suite + race**

```bash
go test ./... >/tmp/sp8-v3.log 2>&1; echo "test=$?"
go test -race ./test/raster/ ./internal/raster/ ./test/paint/ ./internal/paint/ >/tmp/sp8-v4.log 2>&1; echo "race=$?"
```
Expected: test=0, race=0. Read the logs; if a direct-`RasterizeTile` pixel test regressed because a fixture page now composites a group, verify the new value is the correct single-fade and update that assertion (do not weaken it).

- [ ] **Step 3: archtest (imports, cgo, globals, gofmt)**

```bash
go test ./internal/archtest/ >/tmp/sp8-v5.log 2>&1; echo "archtest=$?"
```
Expected: PASS — the scratch pool is a `Scheduler` field (no package global), `Group` lives in `paint` (already imports `frame`), `raster→paint` allowed, no cgo.

- [ ] **Step 4: Gate suite**

```bash
go test ./test/gate -run TestGate -v >/tmp/sp8-v6.log 2>&1; echo "gate=$?"
```
Expected: PASS — `ScratchAllocs()` stays 0 (pool created in `NewScheduler`), warm-scroll tile reuse unchanged.

- [ ] **Step 5: Parity corpus**

```bash
make parity >/tmp/sp8-v7.log 2>&1; echo "parity=$?"
```
Expected: PASS — 107 scored fixtures, prior 106 unchanged within rounding, gaps stay 4/6.

- [ ] **Step 6: Fix anything red, re-run, then proceed to docs.**

---

## Task 9: Documentation close-out

**Files:**
- Modify: `docs/qa-open-defects.json` (close `GOOSIE-COMPOSITE-001`)
- Modify: `docs/roadmap-v2.md` (SP-8 bullet + forward pointer)
- Modify: `docs/superpowers/specs/2026-11-12-sp8-group-opacity-compositing-design.md` (§10 execution record)
- Modify: this plan file (`Execution Record` at the end)

- [ ] **Step 1: Close the defect**

In `docs/qa-open-defects.json`, find `GOOSIE-COMPOSITE-001` (around lines 344-352). Set `status` to `"closed"`, `owner` to the plan path `docs/superpowers/plans/2026-11-12-sp8-group-opacity-compositing.md`, append to `evidence` a CLOSURE sentence with the two oracle pixels `(158,158,255)` / `(191,191,255)`, the fixture `css-features/18-group-opacity-overlap.html`, the clean-group scoping, and the `closed_by` commit hashes from this run. Leave the scoping note that non-clean faded boxes (a `z-index>0` descendant) remain on the per-command path (tracked as a known limitation, not reopened).

- [ ] **Step 2: Update the roadmap**

In `docs/roadmap-v2.md`, add an SP-8 bullet near the SP-7 bullet (~line 210) recording group-opacity offscreen compositing shipped and `GOOSIE-COMPOSITE-001` closed, and update the forward pointer to the next evidence-driven candidate (L3/L4 incremental style+layout invalidation, still gated on "needs a measurement foundation first," or the next parity band by evidence).

- [ ] **Step 3: Append the spec execution record (§10)**

Add a `## 10. Execution record` section to the spec noting: the two refinements (Seq-based sort survival; the same-head nested-group skip), the mutants killed, the parity result, and any deviation from §7 observed while running it.

- [ ] **Step 4: Append this plan's Execution Record**

Add an `## Execution Record` to this file listing the commit hashes, the parity number, deviations, and the final matrix results.

- [ ] **Step 5: Commit**

```bash
gofmt -w docs/qa-open-defects.json 2>/dev/null
git add docs/qa-open-defects.json docs/roadmap-v2.md docs/superpowers/specs/2026-11-12-sp8-group-opacity-compositing-design.md docs/superpowers/plans/2026-11-12-sp8-group-opacity-compositing.md
git commit -m "docs(qa): close GOOSIE-COMPOSITE-001 with the offscreen group-fade oracles"
```

---

## Plan self-review

**Spec coverage:** §3.1 group representation → Task 1 (Group/List/Build/GroupAt). §3.2 clean rule → Task 2 (`isCleanFadedGroup`). §3.3 opacity threading → Task 2 (Step 5 block). §3.4 raster composite + drawCommand → Tasks 3 & 4. §4 damage → Task 5. §5 memory/invariants (pooled scratch, no globals, ≤ one tile) → Task 4 Steps 4-7 + Task 8 Step 3. §7 tests 1-6 → Tasks 4,2,6,5,7,8. §9 acceptance → Task 6 (fixture/floor), Task 8 (matrix), Task 9 (ledger).

**Placeholder scan:** Every code step shows full code. The two intentional "read the exact surrounding identifier" notes (`visSessionPaint`/`visNewSession` in Task 2 Step 1, `ts` scope in Task 4 Step 5) point at a known existing call to reuse verbatim — they are disambiguation, not unfinished logic.

**Type consistency:** `Group{OwnOpacity float32, Bounds frame.Rect, Start,End,Parent int}`, `List.AddGroup(own float32, count int)`, `DisplayCmd.Seq int32`, `LayerDL.GroupAt(i int) (Group,bool)` / `Groups() []Group`, `rasterizeTile(..., scratch *frame.BitmapPool)`, `compositeGroup(dl, grp paint.Group, tile frame.Rect, dst *frame.Bitmap, dx,dy int32, clip frame.Rect, f,g,pool)`, `Job.Scratch *frame.BitmapPool`, `Scheduler.groupScratch` — names/arity used identically across Tasks 1→9. `frame.TileSize`/`Grid.TileSize()` both int32 for `frame.Size{W,H int32}`.

---

## Execution Record

Completed 2026-11-12 on `main`. Commits, in the order they landed (tasks ran 3 → 4 → 2
to keep every commit green, then 1, 5, 6):

- e472d1a2 — Task 1: `feat(paint)` carry faded groups through the sorted display list
  (`Group`, `firstGroupAt`/`Seq`, `GroupAt`/`Groups`).
- 658cdcdd — Task 3: `refactor(raster)` extract `drawCommand` so tiles and groups share
  one path.
- a9f9bdc5 — Task 4: `feat(raster)` composite faded opacity groups offscreen with a
  pooled tile scratch (`compositeGroup`, `fadePixels`, `acquireScratch`, `Job.Scratch`,
  `Scheduler.groupScratch`); pixel oracle `TestGroupOpacityOverlapComposite`.
- 96b736d3 — Task 2: `feat(paint)` record clean faded boxes as composable opacity groups
  (`isCleanFadedGroup`, `groupHeadStack`, the same-head fold-back); reworked
  `opacity_paint_test.go` and added `group_paint_test.go`.
- af39907f — Task 5: `fix(paint)` repaint the region a group-opacity-only change fades
  differently (`groupDamage` in `ContentDamage`); `group_damage_test.go`.
- edae05a3 — Task 6: `test(render)` score `css-features/18-group-opacity-overlap.html`,
  `MIN_SCORED_FIXTURES` 106 → 107.

**Parity (Task 6/8).** 107/107 PASS (100.0%), average 96.43%, the new fixture 98.37%
(`worst_region_iou 1.0`, no blank bands), prior 106 unchanged, known gaps still 4
fixtures / 6 bands.

**Deviations from the plan.**
1. Execution order flipped to 3 → 4 → 2: the raster consumer is backward-compatible with
   a builder that emits no groups, whereas withholding the fade at the builder first
   would break raster output until a consumer existed.
2. Task 7 Step 2 named `TestGroupOpacityOverlapComposite` for the fade-twice mutant, but
   that test builds its display list by hand and never runs the builder, so a
   `builder_box.go` mutation cannot turn it red. The live gate is the builder-driven
   `TestGroupOpacityCountedOncePerElement` (red on all 7 shapes under M2); the mutant was
   verified there instead.
3. Two invariants discovered while running the raster composite: `GroupAt(grp.Start)`
   returns the group itself (child probe guarded to `i > grp.Start`), and the overlap
   oracle needs a per-channel tolerance of 2 for integer rounding. Both are recorded in
   spec §10.

**Verification matrix (Task 8).** `go build -o /dev/null ./...`, `go vet -all ./...`,
`go test ./...`, `go test -race` on {test,internal}/{raster,paint},
`go test ./internal/archtest/`, `go test ./test/gate -run TestGate` (ScratchAllocs 0) and
`make parity` — all exit 0. Tree byte-clean after the mutants were reverted.

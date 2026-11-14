# SP-8 Group Opacity Offscreen Compositing — Design

**Goal:** Composite a faded `opacity` group once offscreen instead of multiplying
its opacity per command at tile-raster time, so two translucent layers that
overlap inside one `opacity<1` group darken correctly instead of twice. Closes
`GOOSIE-COMPOSITE-001`.

**Architecture:** Paint records each *clean* faded group — an opacity-declaring
box with no positioned `z-index>0` descendant — as a contiguous command
sub-range `[Start,End)` plus its own opacity, frozen alongside the flat display
list. The tile rasterizer renders a group's range into a tile-sized scratch
bitmap (drawing each member command exactly as it does today, with the group's
own opacity withheld from the per-command field), applies the group opacity once
to the buffer, and composites the buffer onto the tile; nested clean groups
recurse. Non-clean groups and all non-opacity content keep the existing
per-command path unchanged.

**Tech Stack:** Go; `internal/paint` (display list + builder walk);
`internal/raster` (`tile.go` command loop + `applyOpacity`); `internal/frame`
(`Bitmap`, `BitmapPool`, `BlendOver`/`BlitOver`); headless TDD + `testdata/parity.py`.

**Spec status:** design approved under the standing autonomous delegation;
executed without further sign-off. Commits on `main`, NO push.

---

## 1. Problem and evidence

`GOOSIE-COMPOSITE-001` (P2, open): group opacity is folded into every command's
premultiplied color at `internal/raster/tile.go` `applyOpacity` (`tile.go:115-119`,
called per fill/border/text/image). Because each command blends its own faded
color straight onto the tile, two translucent paintings that overlap inside one
faded group darken twice.

Measured oracle (integrator, 2026-10-26, `/tmp/t46-check/group.html`, 300×120,
DPR 1): a container with `opacity:0.5` holding two `rgba(0,0,255,0.5)` 60×60
boxes overlapping 30 px, on white.

- Single box (non-overlapping): Goosie `(191,191,255)` == Chromium `(191,191,255)`.
  `0.5` (color alpha) × `0.5` (group) = `0.25` effective over white → `255·0.75 = 191`.
- Overlap: Chromium `(158,158,255)` (blue composited twice at 0.5 → 0.75, then
  the group faded once at 0.5 → 0.375 → `255·0.625 ≈ 159`); Goosie `(143,143,255)`
  (each box pre-faded to 0.25, then 0.25 over 0.25 → 0.4375 → `255·0.5625 ≈ 143`).

Root: the engine never holds a group buffer — `DisplayCmd.Opacity` carries the
ancestor×own product and every command applies it alone. TASK-044 fixed *which*
commands exist and *what alpha* each carries (GOOSIE-OPACITY-002 counting); it did
not change how they are combined. This slice is the combining.

## 2. Scope (bounded slice)

In scope:

- A faded-group representation that survives the frozen flat list and z-sorting.
- Offscreen composite at the tile raster: render group → fade once → blend down.
- Nested faded groups (range containment) composed recursively.
- A `z`-scoping rule that keeps every recorded group contiguous (see §3.2).
- Damage correctness when a group's own opacity changes.
- A pixel-level unit oracle + a scored Chromium parity fixture.

Out of scope / deferred (recorded in §8): `isolation`, `mix-blend-mode`, CSS
filter/backdrop-filter compositing, `will-change`/real layer promotion, the full
stacking-context tree, faded groups that contain a positioned `z-index>0`
descendant, and DPR≠1 visual sign-off.

## 3. Design

### 3.1 Group representation (paint)

A new value type in `internal/paint`:

```
// Group is one clean faded opacity group: the contiguous sub-range of commands
// [Start,End) in paint order that an opacity-declaring box emitted, faded once
// by OwnOpacity and composited onto the destination. Nested groups are
// sub-ranges inside their parent's range (Parent indexes the group slice, -1 for
// a top-level group). Only clean groups are recorded.
type Group struct {
    OwnOpacity float32     // the box's own opacity, strictly in (0,1); applied once
    Bounds     frame.Rect // device-px union of the range's command bounds; a tile-reject hint only
    Start, End int         // [Start, End) index range into the command slice
    Parent     int         // enclosing group's index in the group slice, or -1
}
```

`List` gains a `groups []Group` built during the walk; `Build` copies it into
`LayerDL` with a `firstGroupAt []int32` sidecar mapping command index → the index
of the group that starts there, or -1, so the rasterizer finds group starts in
O(1) without a scan. `LayerDL` exposes `GroupAt(i int) (*Group, bool)` (the group
starting at command i) and `Groups() []Group`. Commands and groups share one
index space; the command slice itself is never reordered within a group (see
§3.2), so `[Start,End)` stays valid after `SortStable`.

### 3.2 Why groups stay contiguous — the clean-group rule

`List.SortStable` (`list.go:52-57`) sorts the flat slice by `Z`, stable, and
`LayerDL.Intersecting` (`list.go:174-190`) relies on the list being contiguous in
paint order (a span, not a filtered copy — no per-tile allocation). A naive
opacity group could break if a descendant with `z-index>0` is stamped a `Z` that
lifts it past the group's tail.

Rule: **a box becomes a recorded group only if it declares `opacity∈(0,1)` AND its
subtree has no descendant with a lifting `z`** — i.e. no box below it for which
`zIndexOf` (`builder_box.go:72-77`) returns true. Call this *clean*. Cleanliness
is monotone up the tree: if any descendant escapes, every ancestor's potential
group is also non-clean. So a clean group's whole range carries a single `Z`, and
a stable sort keeps equal-`Z` elements in their original relative order — nothing
was interleaved between the group's commands at append time, so they remain
adjacent after sorting. Non-clean boxes take the existing per-command path (their
own opacity is folded into the threaded `effective`, exactly as today).

The builder determines cleanliness with a bounded DFS over the layout subtree at
group open, checking only `zIndexOf(kid.Style)`; it is bounded by the same
admission limits that already bound the paint walk (no new unbounded recursion).
A conservative read (any escaping descendant ⇒ not clean) is always safe: the
worst case is that a group falls back to today's approximate behavior, never that
a recorded group is non-contiguous.

### 3.3 Opacity threading at a group boundary

The defect and the fallback must never *lose* a fade. At each box the builder
already computes `effective = inheritedOpacity × own` (`builder_box.go:44-47`).

- **Clean group:** do **not** multiply this box's own `o` into the threaded value.
  Emit the box's own decoration/contents and its children with `effective` passed
  down **unchanged** (so a residual inline fade from a non-clean ancestor above
  still reaches every command as it does today), and record `Group{OwnOpacity: o,
  Start: commandStart, End: afterChildren}`. The commands inside therefore carry
  only the non-group fades in `DisplayCmd.Opacity`; the group's `o` is applied
  once at composite.
- **Non-clean opacity box:** unchanged — multiply `o` into `effective`, thread it
  down, emit no group.

Correctness of the two cases:

- Non-overlapping single child: fade-after-composite ≡ fade-per-command. Buffer
  pixel = color composited at its own alpha×(residual inline), then faded by `o`
  once = exactly today's per-command `applyOpacity(color, o×…)`. Identical → **the
  corpus does not move** where there is no intra-group overlap.
- Overlapping children: both drawn to the buffer at residual inline alpha,
  combined correctly (0.75 blue for the two 0.5 boxes), then faded by `o` once →
  **0.375, matching Chromium** — the fix.

### 3.4 Raster composite (raster)

Refactor the per-command `switch c.Kind` body of `RasterizeTile`
(`tile.go:42-55`) into a helper `drawCommand(dst *frame.Bitmap, dl, i, dx, dy,
clip, f, g)` that draws one command into an arbitrary destination bitmap, honoring
`c.Opacity`, `c.Clip`, and `CachedIntersects` exactly as now. `RasterizeTile`
then walks the frozen list; when the loop index `i` equals a group's `Start`
(`LayerDL.GroupAt(i)`), it:

1. Acquires a scratch `*frame.Bitmap` from a tile-sized `frame.BitmapPool` (the
   same fixed size the tile buffers use; contents cleared to transparent).
2. Runs `i` over `[Start,End)`, dispatching nested groups recursively (a second
   scratch) and drawing ordinary members into the scratch via `drawCommand`, each
   clipped to the scratch/tile as today.
3. Applies `applyOpacity(px, group.OwnOpacity)` across the scratch, then
   `out.BlitOver(scratch, …)` onto the tile.
4. Releases the scratch.

The tile is fixed-size (`frame.TileSize`), so a group larger than the tile, or a
group straddling tiles, needs no clamp beyond the tile itself: each tile renders
the members intersecting it into its own tile-sized scratch. Fading is per-pixel,
so an overlap region spanning a tile boundary is still correct — both boxes cover
the pixel in both tiles' scratches. `Group.Bounds` is only a fast reject for tiles
that miss the group entirely. The per-command drawing code is otherwise unchanged,
so image/text/gradient/border/mask paths inherit group compositing for free.

## 4. Damage and equality

A group's `OwnOpacity` lives outside the command fields, so `sameCommand`
(`list.go:151-154`) and therefore `ContentDamage` (`list.go:111-149`) would treat
a frame that only changed a group's opacity as undamaged. `ContentDamage` must
also compare the group slices between the two `LayerDL`s: if the group set for the
dirty command range differs (count, `OwnOpacity`, or range), widen the damage to
the union of the changed groups' command ranges (falling back to the full extent
when in doubt). A unit test drives an opacity-only change and asserts the overlap
region is repainted.

## 5. Memory, limits, and invariants

- **Scratch buffers** are tile-sized and come from a `frame.BitmapPool` of the
  existing `TileSize` dimension — no new bitmap size, no second pool. They must be
  acquired/released, not cached in a package global (`internal/archtest`
  mutable-globals rule): ownership rides on the rasterizer job/scheduler the way
  tile buffers already do (`raster/scheduler.go` `reserveScratch`). Nesting depth
  bounds the simultaneous acquisitions; depth is naturally small and each buffer
  is one tile.
- **No new display-command kind** and no change to the frozen `[]DisplayCmd`
  element type. Only a parallel `[]Group` + `firstGroupAt` sidecar are added, both
  in `internal/paint`, which already imports `frame`.
- **Import/arch rules:** `Group` in `paint` is visible to `raster`
  (`raster → paint` is allowed, `archtest/rules.go:24`); nothing in `frame` gains a
  dependency. Pure Go (no cgo). `internal/engine` gains no `*layout.Object` field.
- **Admission limits:** the scratch is ≤ one tile, already within the tile cache
  budget (`limits.go` `MaxTileCacheBytes`, `MaxDocumentTiles`); no unbounded
  allocation is introduced in the warm-scroll path.

## 6. Correctness argument (why non-overlap is byte-identical)

For a set of commands that do not share any pixel, `BlitOver` onto a cleared
scratch then fading the scratch by `o` then blending to the tile equals blending
each pre-faded command directly to the tile, because each output pixel is written
by at most one command and `applyOpacity` (uniform premultiplied scale) commutes
with the single `BlendOver` onto that pixel. The only pixels whose result changes
are those covered by ≥2 faded commands within one clean group — precisely the
defect. Hence the existing 105-fixture corpus, which has no intra-group
translucent overlap (fixture `13-anonymous-group-opacity` deliberately omits a
container background to isolate the counting case), is expected to stay within
rounding noise; any fixture that does move is one the fix should improve.

## 7. Testing

1. **Raster unit oracle** (`internal/raster` or `test/raster`): hand-build a tiny
   `paint.Layer` — container group `opacity 0.5` over two overlapping
   `rgba(0,0,255,0.5)` rects on white — `RasterizeTile`, read back pixels: the
   non-overlap pixel is `(191,191,255)` and the overlap pixel is `(158,158,255)`.
   This is the deterministic, display-free gate on the defect itself.
2. **Paint oracle update** (`test/paint/opacity_paint_test.go`): keep the counting
   guarantees (GOOSIE-OPACITY-002) by asserting the new group records — a clean
   opacity box emits exactly one `Group{OwnOpacity}` whose range spans its
   subtree, own opacity withheld from member `DisplayCmd.Opacity`, nested groups
   contained — and that a box with a `z-index>0` descendant emits no group (falls
   back). Update the per-command-opacity assertions that assumed the old product.
3. **Scored parity fixture** `testdata/render/css-features/18-group-opacity-overlap.html`:
   the ledger document (faded container, two overlapping translucent boxes) scored
   against Chromium; the overlap region matches after the fix. Register the fixture
   and raise `MIN_SCORED_FIXTURES` by one in the same commit.
4. **Damage test:** opacity-only mutation repaints the overlap region (§4).
5. **Mutants** (each red, then reverted by re-edit): disable group composite;
   fade twice (group + per-command); skip the clean-group z check (accept a
   non-contiguous group → assert contiguity/invariant catches it).
6. **Full matrix:** build / vet / `go test ./...` / `-race` (raster+paint) /
   archtest / `make parity`.

## 8. Deferred (YAGNI, with evidence)

- `isolation`, `mix-blend-mode`, and blend modes beyond `src-over`.
- CSS `filter` / `backdrop-filter` compositing (the `DisplayCmd.Filter` string is
  carried today but not consumed at raster; a group buffer is the natural future
  host but is out of scope here).
- `will-change`/transform-driven real layer promotion and the multi-layer
  composition path (`frame.Layer`/scheduler already plumbed, unused).
- Faded groups that contain a positioned `z-index>0` descendant (non-clean): left
  on the current approximate path; a stacking-context-atomic sort is a separate
  change.
- DPR≠1 and RTL visual sign-off (no display; headless pixel read-back + Chromium
  parity remain the evidence).

## 9. Acceptance criteria

- `GOOSIE-COMPOSITE-001` closed in `docs/qa-open-defects.json` with the two
  oracle pixels, the fixture, the mutants, and the clean-group scoping noted.
- Raster unit oracle asserts `(158,158,255)` in the overlap and `(191,191,255)`
  outside it.
- Parity: 106→107 scored fixtures, all PASS; **no fixture regresses below PASS,
  no blank-band/status change, known gaps stay 4 fixtures / 6 bands**; the prior
  fixtures unchanged within rounding.
- Full `go test ./...`, `-race` on raster+paint, `go vet -all`, and
  `go test ./internal/archtest/` all green. Native (darwin) untouched; no cgo.
- Warm-scroll allocation invariant preserved: group scratches are pooled tile-sized
  buffers, no package globals, no per-frame non-pooled pixel allocation.

## 10. Execution record

Shipped 2026-11-12 across commits e472d1a2, 658cdcdd, a9f9bdc5, 96b736d3, af39907f,
edae05a3 (fixture + floor). The design held; running it surfaced four things worth
recording.

1. **Sort survival via `Seq`.** §3.2 requires a group's members stay contiguous through
   `List.SortStable`. They do only if every member carries the group's single Z, which
   the clean-group rule (`isCleanFadedGroup`: no descendant with a lifting `z-index`)
   guarantees. The `[Start,End)` ordinal is captured *before* the sort, so the ranges
   are recovered after `SortStable` from a per-command `Seq int32` rather than trusted
   from build order.

2. **Same-head nested group is skipped, and its withheld fade folds back.** When an
   enclosing faded box and a nested faded child share the same command head, only the
   enclosing group is recorded; the child is not recorded, so its own fraction (already
   withheld from `effective`) would vanish. `builder_box.go` restores it —
   `if groupOpacity > 0 && !recordThis { effective *= groupOpacity; groupOpacity = 0 }`
   — so the child paints per-command inside the enclosing buffer. Pinned by
   `TestNestedFadedChildSharingHeadFadesOnce`.

3. **`GroupAt(grp.Start)` returns the group itself.** `compositeGroup` must probe for a
   nested child only at `i > grp.Start`, or it recurses on itself and overflows the
   stack. The builder never records a group that shares its enclosing group's head, so a
   child always starts strictly after the head.

4. **Raster oracle tolerance.** Integer premultiplication rounds one unit away from
   Chromium on a multiply-composited pixel, so the overlap oracle asserts
   `(158,158,255)` within a per-channel tolerance of 2 (`pixClose`) — tight enough that
   the pre-fix `(143,143,255)` and the no-fade `(63,63,255)` are both rejected. Same
   reason `color_idle_scroll` carries `tol:1` for composited groups.

**Mutants (each red, then reverted by re-edit).** M1 disable the offscreen composite
(members straight to `dst`, no fade/blit) → `TestGroupOpacityOverlapComposite` red.
M2 fade twice (record the group *and* multiply into `effective`) → caught by the
builder-driven `TestGroupOpacityCountedOncePerElement`, red on all seven shapes — **not**
by the hand-built raster test, which constructs its list directly and never runs the
builder; the plan's Step 7 mis-named that oracle and this record corrects it.
M3 force `isCleanFadedGroup` to `true` → `TestFadedBoxWithZDescendantIsNotGrouped` red
(a group recorded where none belongs).

**Parity.** `css-features/18-group-opacity-overlap.html` scored 98.37% PASS
(`worst_region_iou 1.0`, no blank bands); corpus 107/107 (100.0%), average 96.43%, known
gaps unchanged at 4 fixtures / 6 bands. The prior 106 fixtures stayed PASS.

**Matrix.** `go build -o /dev/null ./...`, `go vet -all`, `go test ./...`,
`go test -race` on {test,internal}/{raster,paint}, `go test ./internal/archtest/`,
`go test ./test/gate -run TestGate` (ScratchAllocs 0), and `make parity` all green.
Native (darwin) untouched; no new cgo; no package-level globals added.

**Execution-order deviation.** Tasks ran 3 → 4 → 2 rather than 2 → 3 → 4 so each commit
stayed green: the raster consumer (Task 4) is backward-compatible with a builder that
emits no groups yet, whereas making the builder withhold the group fade (Task 2) first
would have broken the raster output until a consumer existed.

## 11. Follow-up slice: the offscreen pass must cost the group, not the tile

Shipped 2026-11-12. §3.3 faded and blended the **whole scratch buffer** per group, so
`css-features/13-anonymous-group-opacity.html`'s nine faded boxes each paid a 65,536-pixel
fade plus a 65,536-pixel blend — the group's cost was the tile's cost. The parity fixture
scored 92.68% because it is correct, not because it is cheap.

**Measured first.** `test/raster/group_bench_test.go` drives the exported frame-path entry
(`raster.DefaultRaster` on one `Job` with a pooled `Scratch`), so no worker or channel is in
the number. At `-benchtime 5000x` on an Apple M1 Pro, before this slice: a whole-tile fill
list costs **26.6 µs**, and adding one 40×40 faded box on top costs **326.0 µs** — ~90% of
the frame's group work was fading and blending pixels that were transparent when it started.
A list of 25 faded boxes in one tile cost **7.37 ms**, which alone is more than a paced
frame budget.

**Change.** `compositeGroup` accumulates the boxes its members actually wrote (`wrote`)
while drawing them, then runs `fadeRect` and `BlitOver` over exactly that region. The
accumulator is threaded through `drawCommand` and `drawText` as an `ink *frame.Rect`; the
flat tile path still passes `nil`, so an ungrouped command pays one nil check and nothing
else. `fadePixels`' per-channel float multiply became a 256-entry `fadeScale` table, which
is bit-identical because `applyOpacity` truncates with the same `uint32(float32(v) * o)`.

**Why the region is accumulated and not read off `Group.Bounds`.** A text command's recorded
bounds end at the baseline (`paint.TextRun.Bounds` sets `Y1 = glyph Y`), so descender ink
lives outside the bounds a group's rectangle would supply. `drawText` therefore contributes
the glyph's real mask box. The mutant that takes the shortcut is not hypothetical: it is the
design §3.3 originally implied, and `TestGroupOpacityFadedTextKeepsDescenderInk` fails on it
with `extent 29x35` against `29x49` and a descender pixel left at 255 instead of 127.

**What stays whole-tile, on purpose.** `sc.Reset()` still clears the buffer: it comes back
from the pool dirty at an unknown place, and a memset is ~17 ns against 288 µs for the fade it replaced, so refining it would trade ~20 ns/op for a per-buffer dirty-rect mechanism
through `frame.Bitmap` (which the composer would then have to keep honest). Deferred with
evidence rather than assumed; the same reasoning is why the 32-buffer scheduler-wide idle
pool stays, since the benchmarks report `0 allocs/op` and the gate keeps `ScratchAllocs()
== 0` as the hard floor.

**Correctness evidence.** Pixel output is unchanged, proved rather than asserted: the same
corpus rendered by HEAD (`0dca8516`) and by this tree is **108/108 PNGs byte-identical**, and
the score is identical per fixture — 107/107 PASS, average 96.43%, known gaps 4 fixtures /
6 bands, zero fixtures whose `match_pct` moved. `BlitOver` maps `srcRect`'s top-left to
`dstOrigin`, so the region blit has to name the region's own corner as the origin: passing
`(0,0)` shifted every faded group by its offset and dropped
`css-features/13-anonymous-group-opacity.html` to 78.33%. The corpus caught that in one run;
the unit tests did not, because a single-tile overlap fixture is symmetric about the mistake.

| Case (one 256×256 tile) | Before | After | Factor |
| --- | --- | --- | --- |
| One faded 40×40 box | 326,003 ns/op | 45,293 ns/op | 7.2× |
| 25 faded 40×40 boxes | 7,367,511 ns/op | 208,564 ns/op | 35.3× |
| One faded box covering the tile (the floor) | 836,500 ns/op | 695,802 ns/op | 1.20× |
| No group (control) | 26,629 ns/op | 27,988 ns/op | noise |

All four report `0 B/op  0 allocs/op`. Catalog row: BR-266.

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
premultiplied color at `internal/raster/tile.go` `applyOpacity` (`tile.go:180-189`,
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

`List.SortStable` (`list.go:84-94`) sorts the flat slice by `Z`, stable, and
`LayerDL.Intersecting` (`list.go:265-292`) relies on the list being contiguous in
paint order (a span, not a filtered copy — no per-tile allocation). A naive
opacity group could break if a descendant with `z-index>0` is stamped a `Z` that
lifts it past the group's tail.

Rule: **a box becomes a recorded group only if it declares `opacity∈(0,1)` AND its
subtree has no descendant with a lifting `z`** — i.e. no box below it for which
`zIndexOf` (`builder_box.go:140-145`) returns true. Call this *clean*. Cleanliness
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
already computes `effective = inheritedOpacity × own` (`builder_box.go:90-99`).

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
(`tile.go:71-92`) into a helper `drawCommand(dst *frame.Bitmap, dl, i, dx, dy,
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
(`list.go:224-234`) and therefore `ContentDamage` (`list.go:179-222`) would treat
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
  (`raster → paint` is allowed, `archtest/rules.go:44`); nothing in `frame` gains a
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
</content>

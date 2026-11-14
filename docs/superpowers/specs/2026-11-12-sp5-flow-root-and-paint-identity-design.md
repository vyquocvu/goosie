# SP-5 — flow-root landing (L2-second) + paint-identity completion (L5)

**Date:** 2026-11-12
**Status:** approved (autonomous delegation, approval gates waived)
**Builds on:** SP-4 (L1 BoxKey identity + L2-first pass registry, both zero-diff).

## 1. Why this scope

The SP-4 L2-second landing was deferred with a measured anchor
(`testdata/fc-containment/`). SP-5 exploration found the precise upstream root
cause: **`display:flow-root` is not parsed at all.** `parseDisplay`
(`internal/style/style.go:3718`) has no `flow-root` case and falls through to
`DisplayInline` (`:3753`). Every `flow-root` box therefore becomes inline, which
drops its block margin (the −10px offset) and mis-places its sole inline-block
child (the (0,0) landing). Both measured symptoms are downstream of this one gap.

This is a narrow, evidence-driven fix — not the large "authoritative FC roots"
containment refactor the SP-4 plan drafted. The containment machinery already
exists and already works for `overflow:hidden`: `startsNewFC`
(`internal/layout/block.go:2212`) returns true for non-visible overflow
(`:2218`), and the block pass grows a box to its float bottom when
`startsNewFC` is true (`:438`). Teaching `flow-root` to establish a formatting
context reuses that exact path.

**Zero-diff safety:** no fixture in `testdata/render/` (105) or
`testdata/layout-parity/` (8) uses `flow-root`. Adding the keyword changes
behavior only for pages that declare it, so the scored corpus stays byte-identical.

### Deferred to SP-6+ (with evidence)

L3 (dependency-tier style invalidation) and L4 (dirty-root reflow) are **not** in
SP-5. The terrain map shows they are multi-sprint architecture that conflicts
with current hard behavioral assertions and missing foundations:

- The anonymous/pseudo restyle veto is a **hard test assertion**
  (`test/engine/refresh_test.go:215,238`: a pseudo/width change must rebuild the
  arena, `LayoutBuilds+1`, `Arena != origArena`). Retiring it (review §4.3)
  breaks these tests as written and needs a deliberate re-baseline.
- There is **no per-rule `Deps`** field (`internal/style/matcher.go:22-30`); the
  rule index buckets for matching only. L3 builds this from scratch.
- Every reflow is a full `layout.Build` + three passes
  (`internal/engine/engine.go:1374`); `Refresh`'s "partial" branch is a
  full-pipeline stub (`internal/engine/refresh.go:57-76`); `Plan.Subtree` is
  recorded but never consumed (`internal/engine/invalidation.go`). L4 needs the
  L1 cross-reflow `BoxKey→BoxSlot` persistence that does not exist yet
  (`Reflow` resets selection/find, `engine.go:1473-1476`).

These are independent subsystems (brainstorming scope-check): each gets its own
spec → plan → cycle in SP-6+. SP-5 ships the two bounded, measurable pieces below.

## 2. Component A — `flow-root` establishes a formatting context

**Approach (narrow):** parse `flow-root` to `DisplayBlock` plus a new
`FlowRoot bool` on `ComputedStyle`, and make `startsNewFC` honor the flag. This
reuses the existing block-flow path (margins, block sizing) and the existing
float-containment path (`startsNewFC` → grow to float bottom), instead of adding
a new `Display` enum value that would touch every display switch.

**Changes:**
- `internal/style/style.go`: add `FlowRoot bool` to `ComputedStyle` (near
  `Display`, `:213`). Add a helper `applyDisplay(cs *ComputedStyle, value string)`
  that sets `cs.FlowRoot = (value == "flow-root")` and
  `cs.Display = parseDisplay(value)`, overriding to `DisplayBlock` when
  `FlowRoot`. Route both call sites (`:1762`, `:2238`) through it.
- `internal/layout/block.go`: in `startsNewFC` (`:2212`), return true when
  `s.FlowRoot` (a flow-root block establishes a BFC: contains floats, stops
  margin collapse). Place the check after the block-level switch so a flow-root
  block is recognized.
- `internal/engine/style_diff.go`: add `FlowRoot` to `layoutStyleKey`
  (`:13-91`, `:98`) so a flow-root toggle is treated as a layout-affecting
  change (it changes containment), not a color-only reuse.

**Behavior contract (from the Chromium anchor):** a `flow-root` block is
block-level (honors width/margin), contains its floats (height grows to the float
bottom), and its margins do not collapse with its children's. A sole inline-block
child sits at the group's content origin in normal flow.

## 3. Component B — paint identity completion (L5)

The one genuinely **long-lived** arena pointer is `findMark.obj`
(`internal/engine/engine.go:2001`), held in `Session.findMarks` between `Find`
and `Paint` and only cleared by `Reflow`. It is dereferenced for **only** its
`.Key` at paint time (`internal/engine/paint.go:65`); the highlight rect is
already stored by value in `findMark.rect` (a `Match`). Migrate it to `BoxKey`:

- `findMark` carries `key layout.BoxKey` instead of `obj *layout.Object`.
- `Find` (`engine.go:2056`) stores `words[first].Key`.
- The paint producer (`internal/engine/paint.go:65`) uses `m.key` directly. No
  `ForKey` re-resolution is needed: the rect is a stored value, and find marks
  are cleared on every `Reflow` (`engine.go:1475`), so a mark never outlives its
  generation. This removes the held pointer entirely.

The transient per-paint overlays (`focusState.object`, `ListBox.Obj`,
`ValidationBubble.Obj`, `InspectHighlight.Obj`) are re-fetched from DOM-node
anchors every `Paint` via `objectFor` and are valid only during the walk — they
are **not** the cross-reflow fragility L1/L5 target, so they stay. This distinction
is documented.

**Archtest widening:** extend the arena-pointer guard to flag long-lived
`*layout.Object` **struct fields** in `internal/engine` (not just map keys), so a
future regression that re-introduces a held arena pointer is caught. The rule
must exempt the transient paint-overlay structs in `internal/paint` (walk-local)
and walk-local parameters.

## 4. Data flow

`flow-root` declaration → `applyDisplay` sets `Display=block, FlowRoot=true` →
cascade computes style → `layoutStyleKey` includes `FlowRoot` → block pass:
`startsNewFC` true → box contains floats and stops margin collapse → arena
geometry matches Chromium. Find: `Find` stores `BoxKey` → `Paint` re-resolves via
`ForKey` → display command names its box without a held arena pointer.

## 5. Error handling / edge cases

- `flow-root` with `float` or `position:absolute`: those already establish a BFC
  via the display/position rules; `FlowRoot` is additive and must not double-count.
- `FlowRoot` on a non-block used display (e.g. a flex item that declares
  `display:flow-root`): `applyDisplay` forces `DisplayBlock`, matching CSS
  (flow-root is block-level). Verify against Chromium if a fixture exercises it.
- Find marks never outlive their generation: `Reflow` clears `findMarks`
  (`engine.go:1475`), so the stored `key`+`rect` values are always current; no
  stale-key resolution path is needed.

## 6. Testing

- **Chromium oracle:** `scripts/verify-fc-containment-geometry.js` must print
  `FC CONTAINMENT ORACLE OK` (already does; it is the reference).
- **Promote the fixture:** move `testdata/fc-containment/nested-float-containment.*`
  into `testdata/layout-parity/float-containment-001.*`, regenerate the reference
  with `testdata/gen_layout_refs.py`, and let `TestLayoutParityWithChromium`
  gate it (1px tolerance). This is the acceptance test for Component A.
- **Unit:** a `flow-root` parse test (`FlowRoot` set, `Display==block`); a
  `startsNewFC` table case for flow-root; a `layoutStyleKey` case proving a
  flow-root toggle is layout-affecting.
- **L5:** a find-across-reflow test asserting find marks survive by key (or are
  correctly cleared); the widened archtest rule with a dirty fixture (held
  `*layout.Object` field flagged) and a clean fixture (transient overlay exempt).
- **Zero-diff gate:** `make parity` 105/105 ≥96.39%, gaps 4/6 unchanged; full
  suite, race subset, vet, archtest, gate, qc all green; gate counters unchanged.
- **Mutation evidence:** drop the `FlowRoot` branch in `startsNewFC` → the
  promoted layout-parity fixture goes red; restore → green. Re-introduce a held
  `*layout.Object` field → widened archtest red; restore → green.

## 7. Acceptance

SP-5 ships when: the promoted `float-containment-001` layout-parity fixture is
green against Chromium; `findMark` holds no arena pointer and the widened
archtest guards it; the full matrix + zero-diff parity are green; roadmap and
review record L2 complete and L5 complete, with L3/L4 deferred to SP-6+ with the
evidence above. Commits on `main`, no push.
# SP-4 — L1 BoxKey identity + L2 formatting-context registry — Design

> Program position: fourth sub-project of the agreed chain SP-1 → SP-2 (V0) → SP-3 (P2 journeys) → **SP-4 (L1+L2)** → SP-5 (L3+L4+L5) → SP-6 (P3) → SP-7 (evidence-driven engine features). Source of truth: `docs/engine-compatibility-review.md` §4.1–4.2, §6 (L1/L2). This spec corrects that review's stale 2026-10-24 snapshot against the code at HEAD `3ee61413`.

**Goal:** Give every layout box a stable, unique `BoxKey` and make paint name boxes by key instead of arena pointer (L1); then make the display-dispatch chain an explicit, instrumented formatting-context pass registry (L2) — both landings behavior-preserving with zero pixel diff.

**Architecture:** Refactor on the existing seams. `BoxKey` already exists (`internal/layout/boxkey.go`) and is assigned to root/element/pseudo boxes, but nothing reads it and generated boxes are unkeyed. L1 completes the identity: a `byKey` index, keys for word/anonymous boxes, and migration of the two production `map[*layout.Object]` (paint selection + find) to `BoxKey`. L2 wraps the existing free-function layout passes in a registry that dispatches in exactly today's order, adds per-FC counters, then (second landing) makes FC roots authoritative for float/margin containment.

**Tech Stack:** Go 1.26; `internal/layout` (arena, block, inline, anonymous, table, grid, position), `internal/paint` (builder, selection, find), `internal/engine` (selection producers, Reflow, WorkCounts); Playwright/Chromium anchors; the 105-fixture parity corpus + committed embedded-font goldens.

---

## 1. Ground truth (corrected against HEAD `3ee61413`)

The review's snapshot is stale. Verified current state:

**Already done (do not re-plan):**
- `BoxKey uint64` = (node32 | kind16 | ordinal16), `BoxKind` enum (15 kinds, `BoxKindUnknown=0` … `BoxKindPseudoFirstLetter=14`), `NewBoxKey`, `NodeID()/Kind()/Ordinal()`, `boxKindForStyle` — `internal/layout/boxkey.go:5-74`.
- `Object.Key BoxKey` field — `arena.go:20`. Assigned in `Build` for root (`arena.go:236`), elements with `n.ID != 0` (`arena.go:300`), `::before/::after` (`arena.go:457`), `::first-letter` (`arena.go:478`). Tests: `test/layout/boxkey_test.go`.
- Float-containment seed/merge is **committed**, not dirty — `block.go:404-437`; `floatBottomL/R`, `floatSeedL/R`, `floatTop`, `floatSeedTop/Has` fields at `arena.go:57-62`.
- Pseudo-background maps are keyed by `*style.ComputedStyle`, **not** `*layout.Object` — `engine.go:58-59`. Style pointers survive reflow, so these are already arena-pointer-free. **No migration needed.**

**Missing (the actual L1 work):**
- No `Arena.byKey` index. The only node index is `byNode map[dom.NodeID]ObjectID` (`arena.go:124`).
- No production code reads `Object.Key`. Every `NewBoxKey` call passes ordinal `0`.
- Word/text-run boxes (`inline.go:597-604`), anonymous blocks (`anonymous.go:104-105`), and anonymous table rows/cells (`table.go:265,287` via `spliceAnonymousBlock`) are **unkeyed** (`Key` zero).
- Two production `map[*layout.Object]`: `Builder.selection map[*layout.Object]SelSpan` (`paint/builder.go:13`) and `Builder.find map[*layout.Object][]FindMark` (`paint/builder.go:14`). Both rebuilt every `Paint` from the live arena; never outlive one generation. `SelSpan.Obj *layout.Object` (`paint/selection.go:9-13`), `FindMark.Obj *layout.Object` (`paint/find.go:8-12`).
- The false pointer-stability comment is on `Alloc` at `arena.go:143-145` (claims the pointer is stable across `Alloc`; `append` doubling invalidates it).

**L2 ground truth:**
- Dispatch is a 5-branch `if/else-if` inside `blockIntoInner` at `block.go:180-191`: `ContentVisibility=="hidden"` (short-circuit, sets W/H) → table (`layoutTable`) → grid (`layoutGrid`) → flex (`layoutFlexColumn` if column/column-reverse else `layoutFlexRow`) → block-flow `else` (the inline child loop `block.go:192-441`). Inline and positioned are **not** display branches here.
- All passes are **free functions** over `*Arena`: container passes share `func(a *Arena, id ObjectID, containingW float32) float32` returning **content height**; `blockInto*` return **outer height**; `layoutTable` also writes `obj.W`/`obj.H` itself (`table.go:219,228`). No builder/context struct exists.
- Engine pass order `Build → Block → Inline → applyImplicitPopoverAnchors → Positioning`, duplicated in `Reflow` (`engine.go:1361-1371`), `refresh.go:47-52/80-85/121-126`, `cssom_view.go:184-192`.
- `startsNewFC(a, obj)` at `block.go:2215-2234` encodes **margin-collapse-through**, not full FC establishment. 6 callers: `block.go:405,425,441,2243,2251`, `inline.go:72`.
- `WorkCounts` (`engine.go:169-172`) = StyleResolves, StyleNodeVisits, LayoutBuilds, DisplayListBuilds. **No per-FC/per-pass counter exists.** Tile/alloc counters live in `raster`/`frame`/`surface`, not engine.
- Stale package doc `arena.go:1-1` claims a "block, inline, float, flex, grid, table" order that does not match the code.

---

## 2. L1 — complete BoxKey identity

### 2.1 The `byKey` index

`Arena` gains `byKey map[BoxKey]ObjectID`. A single helper `func (a *Arena) setKey(id ObjectID, k BoxKey)` sets `a.Objects[id].Key = k` and `a.byKey[k] = id`. Every key-assignment site calls it: `Build`/`buildSubtreeWithHost`/`buildPseudoElement`/`buildFirstLetterPseudoElement` (replacing the direct `obj.Key = …` writes) and the new generated-box sites (§2.2). `byKey` is allocated in `NewArena`. A lookup `func (a *Arena) ForKey(k BoxKey) (ObjectID, bool)` mirrors `ForNode`.

The index is **generation-scoped** (one arena per reflow, unchanged). It is the seam L4 will use for copy-forward; L1 does not change the rebuild strategy.

### 2.2 Keys for generated boxes

Generated boxes get a key whose **ordinal is a per-arena monotonic counter per kind**, guaranteeing within-generation uniqueness regardless of allocation order or how many times a node is processed:

- `Arena` gains `wordSeq uint16` and `anonSeq uint16` (zero in `NewArena`).
- **Word/text-run** (`inline.go:597`, after `a.Alloc()`): `a.setKey(wordID, NewBoxKey(uint32(k.Node.ID), BoxKindWord, a.wordSeq)); a.wordSeq++`. `k.Node.ID` is the originating text node (the per-word `nodeCopy` preserves `ID`). Add `BoxKindWord BoxKind = 15` to the enum.
- **Anonymous block / table row / cell** (`anonymous.go:104`, inside `spliceAnonymousBlock`): `a.setKey(id, NewBoxKey(a.anonNodeID(parent), BoxKindAnonymous, a.anonSeq)); a.anonSeq++`. `anonNodeID(parent)` walks `Parent` links to the nearest box with `Node != nil && Node.ID != 0` and returns its node ID (0 if none). The global `anonSeq` ordinal — not the node ID — carries uniqueness, so nested anonymous boxes under different parents never collide.

**Why global monotonic ordinals:** the review's natural word key `(textNodeID, runeStart, runeEnd)` does not fit the 16-bit ordinal field, and per-node ordinals can collide if a node is collected twice. A global per-kind counter makes the bijection trivially true within a generation, which is all the paint maps (rebuilt per `Paint`) require. Cross-generation word/anonymous identity is best-effort (ordinal shifts with content) — the review acknowledges this; true copy-forward stability is L4.

**Overflow:** `uint16` ordinals cap at 65535 generated boxes per kind per generation. The fixture corpus and admission limits (`MaxDocumentNodes`) stay far below this. If a counter would overflow, `setKey` leaves the box unkeyed (`Key` zero) rather than wrap and collide; the bijection test asserts no overflow on the corpus. Documented limitation, not silently wrapped.

### 2.3 Paint names boxes by key

Migrate the two production pointer maps to `BoxKey`:
- `SelSpan.Obj *layout.Object` → `SelSpan.Key BoxKey` (`paint/selection.go`). `Builder.selection map[BoxKey]SelSpan` (`paint/builder.go:13`); `SetSelection` keys by `sp.Key`; the consumer `paint/builder_text.go:39` looks up `b.selection[obj.Key]`.
- `FindMark.Obj *layout.Object` → `FindMark.Key BoxKey` (`paint/find.go`). `Builder.find map[BoxKey][]FindMark`; `SetFind` keys by `m.Key`; consumer `paint/builder_text.go:36` looks up `b.find[obj.Key]`.
- Producers in `engine/selection.go` (`selectionSpans` ~564, `imeMarkedSpans` ~590) and the find-mark producer set `Key` from the live word box's `Key` instead of its pointer. Word boxes are keyed during layout (§2.2), before paint, so the keys exist.

After this, **zero** production `map[*layout.Object]` remains. Transient per-paint pointer holders (`Builder.focus.object`, `InspectHighlight.Obj`, `ValidationBubble.Obj`, `ListBox.Obj`, `engine.findMark.obj`) are re-resolved from the live arena each paint and never cross an allocating call — they stay as-is (the documented re-fetch pattern). `objectFor` (`focus.go:2722`) is a per-paint linear re-resolution, not a held pointer — unchanged.

### 2.4 Pointer-lifetime rule

- Delete the false comment at `arena.go:143-145`. Replace with the rule: **a `*Object` is valid only until the next call that can allocate; hold `ObjectID`, re-fetch at use.** The existing re-fetch sites (`block.go:144,279,418,442,475,551,806`, `inline.go:408,614,642`, `table.go:269`) become the documented pattern.
- Correct the stale package doc `arena.go:1-1` to the real order (Block dispatches flex/grid/table per box; engine runs Block → Inline → Positioning).
- New archtest rule: no production package declares a map keyed by `*layout.Object` (grep-based, like the existing cgo/globals rules). This is the enforceable proxy for "paint does not hold arena pointers."

### 2.5 L1 acceptance

- **Bijection test** (`test/layout/boxkey_bijection_test.go`): after `Build`+`Block`+`Inline`+`Positioning` on the largest text-heavy fixture, a table fixture, a flex fixture, and a pseudo-element fixture, assert: every box with `Node != nil` or `flagAnonymous` or a word box has a non-zero `Key`; `len(byKey)` equals the count of keyed boxes; `ForKey(obj.Key) == obj`'s ObjectID for every keyed box; no two boxes share a key. Include a fixture that exercises anonymous table rows/cells and nested anonymous blocks.
- **No held pointer:** archtest passes (zero `map[*layout.Object]`); the three+ re-fetch sites reviewed.
- **Zero diff:** `make parity` 105/105 at ≥96.39% with the same 4-fixture/6-band known gaps; committed goldens byte-identical; `go test ./...` green; gate counters unchanged.

---

## 3. L2 — formatting contexts as layout roots

### 3.1 First landing: behavior-preserving pass registry

Replace the `else if table / else if grid / else if flex` chain (`block.go:185-190`) with a registry lookup, leaving the `ContentVisibility=="hidden"` short-circuit first and the block-flow `else` loop unchanged:

```go
type FCKind int
const (
    FCBlockFlow FCKind = iota
    FCTable
    FCGrid
    FCFlexRow
    FCFlexColumn
)

type layoutPassFunc func(a *Arena, id ObjectID, containingW float32) float32

// fcRegistry maps a used display value to its pass. Flex dispatches on
// direction inside its wrapper, exactly as the else-if did.
var fcRegistry = map[style.Display]layoutPassFunc{
    style.DisplayTable:       layoutTable,
    style.DisplayInlineTable: layoutTable,
    style.DisplayGrid:        layoutGrid,
    style.DisplayInlineGrid:  layoutGrid,
    style.DisplayFlex:        layoutFlex,
    style.DisplayInlineFlex:  layoutFlex,
}

func layoutFlex(a *Arena, id ObjectID, containingW float32) float32 {
    obj := a.Get(id)
    if obj.Style.FlexDirection == style.FlexColumn || obj.Style.FlexDirection == style.FlexColumnReverse {
        return layoutFlexColumn(a, id, containingW)
    }
    return layoutFlexRow(a, id, containingW)
}
```

Dispatch becomes:
```go
if obj.Style != nil && obj.Style.ContentVisibility == "hidden" {
    // unchanged W/H short-circuit
} else if pass, ok := fcRegistry[obj.Style.Display]; ok {
    y = pass(a, id, containingW)
} else {
    // unchanged block-flow child loop (block.go:192-441)
}
```

Table/grid/flex are mutually exclusive display values, so map lookup reproduces the else-if precedence exactly. The same functions run with the same arguments and the same return conventions (content height; `layoutTable` still writes W/H). The block-flow `else` is **not** extracted into a function in this landing — it stays inline to avoid touching its shared local state. `nil`-style guard preserved (the registry branch is only reached when `obj.Style != nil`, matching today's `obj.Style != nil &&` conditions; a `nil` style falls to block-flow exactly as today).

**Per-FC counters:** `Arena` gains `PassCounts [5]uint64` indexed by `FCKind`, incremented once per dispatch (block-flow incremented in the `else`). A `func (a *Arena) FCCount(k FCKind) uint64` accessor. The engine surfaces these through a new `WorkCounts`-adjacent snapshot or a `Session.LayoutPassCounts()` accessor (layout cannot import engine; counters live on the arena, engine reads them after reflow). Tests assert which pass ran for a flex/grid/table/block fixture.

### 3.2 Second landing: authoritative FC roots

Make FC boundaries authoritative for float/margin containment instead of the local seed/merge patch:
- Widen `startsNewFC` (`block.go:2215`) from margin-collapse-through to the CSS FC-establishment rule set: the box's own `float`, `position:absolute/fixed`, `display:inline-block`, `display:table-cell`, flex/grid **item**-ness (parent display), `overflow` not visible, `contain:paint/layout`, `flow-root`. Keep the margin-collapse callers (`collapsesThroughTop/Bottom`) correct by separating the two predicates if widening changes their meaning — `startsNewFC` keeps its margin semantics for those callers, and a new `establishesFC` drives containment.
- The block pass descends into FC roots and merges their float extents through the registry rather than the ad-hoc `floatSeed*`/`floatBottom*` hand-down/merge-up. Remove the local merge patch only once the new path reproduces it.

**This landing changes behavior** and is gated on a new Chromium-verified fixture, not zero-diff. It is the riskiest part of SP-4; if the registry landing is clean but this one cannot match Chromium within budget, it is recorded as deferred to SP-5 with evidence (the review §7 permits reordering), and SP-4 ships L1 + L2-first-landing.

### 3.3 L2 acceptance

- **Registry landing:** zero fixture diff (parity 105/105 ≥96.39%, goldens byte-identical); per-FC counters asserted by a test that lays out a flex, a grid, a table, and a block fixture and checks the corresponding `PassCounts` entry incremented and the others did not.
- **FC-root landing:** a deliberately nested float/margin fixture (`testdata/render/edge-cases/08-nested-float-containment.html` or next free slot) matches Chromium via a new Playwright anchor `scripts/verify-fc-containment-geometry.js`; the existing float/clearance fixtures stay green; the local merge patch is removed only after the anchor passes.

---

## 4. Verification architecture

Every behavior-preserving landing (L1, L2-first) is gated on **all** of:
- `go build ./...`, `go vet -all ./...`, gofmt clean.
- `go test ./... -count=1` green; `go test -race ./internal/layout/ ./internal/paint/ ./internal/engine/ ./cmd/goosie/ ./test/engine/ ./test/gatecheck/ ./test/goldens/ ./internal/archtest/ -count=1` green.
- `go test ./internal/archtest/` green (including the new no-pointer-map rule).
- `go test ./test/gate -run TestGate` green; gate counters (StyleNodeVisits, LayoutBuilds, DisplayListBuilds, tiles) unchanged.
- `make parity` 105/105 at ≥96.39%, known gaps 4 fixtures/6 bands unchanged.
- Committed goldens (`testdata/render-goldens/`) byte-identical (`go test ./test/goldens/`).
- `go test ./test/qc -count=1` green (regenerate golden only if catalog rows change).

The L2-second landing adds the Chromium anchor as its gate and does **not** require zero diff on the new fixture (it is new), but must not regress the existing corpus.

TDD with red→green mutations: each new test is shown red on the pre-change tree (or with a deliberate mutation) then green. Mutations are reverted with `git checkout` only after pending work is committed.

---

## 5. Out of scope (deferred, with owner)

- **L3 dependency-tier invalidation, L4 dirty-root reflow + copy-forward, L5 admission/teardown** → SP-5. L1's `byKey` index and generation-scoped arena are the seam L4 reuses; L1 does not implement copy-forward or change the rebuild strategy.
- **Session cross-reflow `map[BoxKey]BoxSlot` retention** → L4 (no production consumer in L1; selection ends on reflow per `engine.go:1465`, paint maps are rebuilt per paint). Adding an unused map now would violate YAGNI.
- **Display-list IR / diffing by key** → later optimization (review §4.4).
- **Text shaping, bidi, multi-line flex, table conformance** → SP-7 / deferred register.
- **Pseudo-background map migration** → not needed (already keyed by stable `*style.ComputedStyle`).
- **`objectFor` linear scan optimization** → not an identity concern; unchanged.

---

## 6. Risks and falsifiers

| Risk | Falsifier |
| --- | --- |
| Word/anonymous key collisions break selection/find | Bijection test on text-heavy + table + pseudo fixtures; any duplicate key fails. Global monotonic ordinals make collisions impossible below overflow. |
| Paint migration changes pixels | Zero-diff parity + goldens after L1. Any nonzero diff falsifies behavior-preservation; revert. |
| Registry changes dispatch order/return conventions | Zero-diff parity + goldens after L2-first; per-FC counter test asserts the right pass ran. |
| Widening `startsNewFC` breaks margin collapse | Separate `establishesFC` from the margin predicate; existing float/clearance fixtures + gate stay green; new Chromium anchor for containment. |
| Ordinal overflow (>65535 generated boxes/kind) | Bijection test asserts no overflow on corpus; `setKey` leaves overflow boxes unkeyed rather than wrap. Documented limitation. |
| L2-second cannot match Chromium within budget | Defer to SP-5 with evidence; ship L1 + L2-first (review §7 permits reordering). |

---

## 7. Self-review

- **Spec coverage:** L1 (review §4.1, §6 L1) → §2; L2 (§4.2, §6 L2) → §3; paint identity (§4.5) → §2.3; acceptance evidence → §2.5, §3.3, §4. Deferred L3/L4/L5 (§4.3–4.6) → §5.
- **Placeholder scan:** no TBD/TODO; the L2-second deferral is an explicit, evidence-gated decision, not a placeholder.
- **Type consistency:** `BoxKey`/`BoxKind`/`NewBoxKey`/`setKey`/`ForKey`/`byKey`/`wordSeq`/`anonSeq`/`BoxKindWord` (§2) used consistently; `FCKind`/`layoutPassFunc`/`fcRegistry`/`layoutFlex`/`PassCounts`/`FCCount` (§3) consistent; `SelSpan.Key`/`FindMark.Key` replace `.Obj` everywhere named.
- **Grounding:** every file:line verified against HEAD `3ee61413` by code exploration; stale review claims corrected in §1.
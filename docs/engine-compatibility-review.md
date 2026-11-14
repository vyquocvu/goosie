# Goosie — Engine Compatibility Review and Target Design

- **Status:** Review and design proposal, for decision. No engine code is changed by this document.
- **Date:** 2026-10-24.
- **Snapshot:** branch `main`, HEAD `63d226bd6fa76a90bbb6efb10d319f665aa2b1d2`, working tree dirty (8 modified files, 6 untracked test files). `go build ./...` and `go vet ./...` pass; `go test ./test/layout ./test/paint ./test/style` and `go test ./internal/archtest` pass. CI on this commit is red (run `40069270435`; Section 5.0).
- **Lens:** engine compatibility (HTML/CSS/DOM/JS). The application layer is covered only where it affects compatibility evidence.
- **Audience:** maintainers deciding the next architectural investment.

Every `file:line` citation refers to the working tree at the snapshot above. Where a citation points at uncommitted work, it is marked **(dirty)**. Commands behind counts are listed in Appendix B.

---

## 0. How to read this document

Section prose uses four labels. Appendix A uses a compact ledger code: **V** verified, **C** claimed but not reproduced, **D** documented but contradicted by code, **K** known-broken or placeholder.

| Label | Meaning |
| --- | --- |
| **Implemented** | A code path exists. Conformance is not implied. |
| **Verified** | Reproduced by a command or direct read on 2026-10-24; the evidence is named. |
| **Claimed, unproven** | Asserted in docs or prior reports but not reproducible from checked-in artifacts in this review. |
| **Known-broken** | Reproduced divergence, or an explicit placeholder read in the code. First-party Go carries no explanatory comments (32,893 comment-only lines and 683 trailing comments removed 2026-11-14), so a comment is no longer a citable kind of evidence. |

Rules:

- No number appears without a denominator, a date, and the command that produced it.
- No claim is promoted to "verified" because the roadmap says so. Docs are evidence of intent, not of behavior.
- "Not run" is stated explicitly. This review did not run the full suite, the WPT suite, live URL parity, or native macOS verification.

---

## 1. Scope, method, and product contract

### 1.1 The declared product

`docs/roadmap-v2.md` defines the product as a daily-use macOS browser built on the Go engine, with a headless path sharing the same engine. It explicitly does not promise full-web compatibility, and it defers password autofill and cloud sync (`docs/roadmap-v2.md:101`). `docs/engine-compat-roadmap.md` adds the compatibility method: publish a supported feature/site set, a test set, reference results, remaining bugs, and security limits rather than a single "Chrome-compatible" label (`docs/engine-compat-roadmap.md:9-11`).

That contract is sound. The review asks one question against it:

> What must change so that compatibility work is **measurable** and **safe to iterate on**?

### 1.2 Method

Three audits were run in parallel (engine core, application layer, verification harness), followed by direct verification of every load-bearing claim in this document. The verification pass re-read the cited code rather than trusting the audits. Nothing in this document depends on a prior report that was not re-checked.

### 1.3 What this review did not do

- It did not run the full suite, `go test -race ./...`, the full WPT suite, `make parity`, or native macOS verification.
- It did not run CI locally; it inspected the latest `main` run (`gh run view 40069270435`, Appendix B, commands B22-B23).
- It did not modify the engine or the app.
- It did not measure timing or memory.
- It did not validate the uncommitted parity work as a coherent change; that work is treated as working-tree state, not as landed behavior.

Consequently, any compatibility number in this document comes from checked-in code or docs and is labeled accordingly.

---

## 2. Current architecture as built

### 2.1 The pipeline that actually runs

The documented pipeline is Parse → Style → Layout → Paint, with layout described as block, inline, float, flex, grid, table, then positioning (`PROJECT.md:31-33`). The package comment this row also cited at `internal/layout/arena.go:1` is gone with the rest of them; `PROJECT.md` is now the only place that enumeration lives.

The code runs a narrower sequence. `Session.Reflow` (`internal/engine/engine.go:874`) performs:

1. `layout.Build` (`internal/engine/engine.go:889`)
2. `layout.Block` (`internal/engine/engine.go:892`)
3. `layout.Inline` (`internal/engine/engine.go:893`)
4. `layout.Positioning` (`internal/engine/engine.go:894`)

Flex, grid, and table are not separate passes. They are selected inside the block pass by a `display` `else-if` chain (`internal/layout/block.go:175-183`). Float layout is likewise handled inside block and inline layout, not as an ordered pass. The six-pass description is therefore **claimed, not implemented**.

This matters beyond documentation accuracy. A display-property chain makes each formatting context a branch in a 2,953-line function (Appendix B, command B21), so a conformance fix for one context shares control flow with every other context, and no context can be laid out or invalidated independently.

`startsNewFC` (`internal/layout/block.go:2022`) partially encodes formatting-context boundaries, but it answers a narrower question — whether a descendant margin can collapse through a box — not "does this box establish an independent formatting context". The float containment added in the uncommitted work (`internal/layout/block.go:379-388`, dirty) is another local patch over the same missing abstraction.

### 2.2 State ownership and duplication

`Session` owns the document, computed styles, resolver, arena, image/font tables, JS runtime, media, focus, selection, and invalidation state (`internal/engine/engine.go:27-113`). The major owners are:

| Owner | State | Evidence |
| --- | --- | --- |
| `engine.Session` | DOM, styles, pseudo styles, resolver, arena, images, fonts, JS, media, focus | `internal/engine/engine.go:27-113` |
| `style.Resolver` | Compiled rule index plus a full tree/attribute snapshot for interactive fast paths | `internal/style/style.go:1154-1178`, `internal/style/incremental.go:159-208` |
| `layout.Arena` | Box objects, parent/child links, per-node index | `internal/layout/arena.go:116-123` |
| `paint.Builder` | Display list plus a selection map keyed by `*layout.Object` | `internal/paint/builder.go:7-12` |
| `raster.Scheduler` | Viewport, scroll, tile plans and scratch | `internal/raster/scheduler.go:14` |

Some state is duplicated by design: computed styles live both in `Session.Styles` and in each `Object.Style`; decoded images live in session maps and are copied onto arena objects during each `Reflow` (`internal/engine/engine.go:906-911`). That duplication is tolerable while the arena is rebuilt every frame, but it becomes a correctness surface as soon as arena identity must survive a reflow.

The uncommitted work adds two more pointer-keyed maps for pseudo-element backgrounds, `map[*style.ComputedStyle]image.Image` (`internal/engine/engine.go:48-50`, dirty). This is the same identity pattern the target design removes: a pointer into computed styles is used as a long-lived key, and the key changes whenever the resolver reallocates styles.

### 2.3 What is genuinely incremental

The engine is not uniformly full-rebuild. Four fast paths exist:

1. **Scroll frames.** A scroll-only plan performs no style or layout work (`internal/engine/refresh.go:19-21`), and the scheduler shifts tiles. The gate suite asserts zero style/layout/alloc work on warm scroll (`test/gate/m1_gate_test.go:30-44`, `test/gate/scroll_gate_test.go:69-80`).
2. **Tile reuse.** Tiles are reused across frames by the scheduler and only dirty tiles are rasterized (`cmd/goosie/presentation.go:113-133`; gate counters assert tiles rasterized vs reused).
3. **Interactive state restyle.** `:hover`, `:focus`, `:focus-visible`, and `:active` can restyle a subtree when `interactiveSelectorSupport` certifies the stylesheet (`internal/style/style.go:1175-1176`, `internal/style/incremental.go:53-156`).
4. **Geometry-neutral style reuse.** When computed style changes do not affect layout geometry and the arena has no anonymous boxes, only style pointers are refreshed (`internal/layout/arena.go:139-141`).

Each has a significant qualifier:

- The interactive path is still **O(N) per interaction**: `ResolveInteractiveStates` walks the entire tree and all attributes to compare snapshots before it can decide that only one subtree changed (`internal/style/incremental.go:68-68`, `internal/style/incremental.go:170-208`). The "fast path" avoids restyling, not scanning.
- The style-only reuse path is vetoed by any anonymous box, including `::before`/`::after` text boxes and generated table parts (`internal/layout/arena.go:141-141`, `internal/layout/arena.go:149-157`). Most real pages contain at least one.
- `Session.Refresh` and `Session.RefreshNode` — the methods whose names promise subtree work — are full rebuilds: the `plan.Subtree` branch re-parses every author sheet, re-resolves the whole document and rebuilds the arena (`internal/engine/refresh.go:57-76`), line for line the same work as the `FullDoc` branch (`internal/engine/refresh.go:36-53`), and `RefreshNode` does it again without even a plan to narrow it (`internal/engine/refresh.go:81-110`). They have **no production callers** (Appendix B, command B6).

Everything else is total. A DOM mutation from JS enters `PumpJS` (`internal/engine/engine.go:1574`), which calls `RestyleAndReflow` (`internal/engine/engine.go:1607`, `internal/engine/engine.go:1614`); that path re-resolves the whole document and rebuilds the arena and layout. The display list is rebuilt per repaint (`cmd/goosie/presentation.go:94-101`); `paint.List.Reset` exists but has no caller (`internal/paint/list.go:22`).

### 2.4 Concurrency and lifetime

The engine uses bounded pools: fonts 8 (`internal/engine/engine.go:316-316`), images 12 (`internal/engine/engine.go:658-659`), scripts 8 (`internal/engine/engine.go:1446-1447`), and JS fetch 6 (`internal/js/fetch.go:15-18`). JS is per document (`internal/engine/engine.go:82`).

Three lifetime gaps are visible in code:

- `Session.Close` clears interaction state, stops media, and closes the JS runtime, but does not wait for deferred font/image/script loads or JS network callbacks (`internal/engine/engine.go:1511-1526`). `deferredActive` only gates `PumpJS` reflows (`internal/engine/engine.go:109-113`, `internal/engine/engine.go:1596`).
- The JS `HTTPFetcher` interface carries no context (`internal/js/fetch.go:29-29`). The browser adapter has one (`cmd/goosie/page_load.go:37-60`), but `Runtime.Close` only flips a flag (`internal/js/js.go:2045-2049`), so in-flight fetch/XHR/worker work is not joined by the runtime.
- `FetchTimeout` is declared (`internal/js/fetch.go:20-20`) and asserted by a test (`test/js/fetch_test.go:220-221`) but never used to bound a request (Appendix B, command B7).
- `raster.Fonts.Register` appends every registered font to a process-lived slice with no cap or release (`internal/raster/fonts.go:179-187`). A long-lived browser process accumulates custom fonts across documents.

None of these is a proven crash. They are lifetime ambiguities that become failure modes under tab churn, rapid navigation, or hostile pages.

### 2.5 Admission: strong at the parser, porous at the JS boundary

The document and image caps are explicit and well placed: `MaxDocumentBytes` 16 MiB, node/depth/attribute caps in `internal/engine/limits.go:23-46`, and encoded-size/dimension/pixel caps in `internal/image/decode.go:16-22`. `PaintChecked` validates scale, arena, and extent before painting (`internal/engine/paint.go:13`) and is the untrusted-input contract.

Two paths depart from that model:

- **JS DOM growth.** `appendChild` calls `dom.Node.AppendChild` directly and notifies the mutation sink (`internal/js/js.go:1458-1470`). There is no node, depth, or attribute check at the mutation. `innerHTML` and `insertAdjacentHTML` apply per-call limits of 1,024 nodes, depth 32, 256 attributes (`internal/js/js.go:431`, `internal/js/js.go:1319`), which are repeatable and therefore not a document-level cap. The next `validateArena` notices after allocation (`internal/engine/limits.go:382-442`).
- **JS networking.** The shared subresource byte budget (`sub *subBudget`, `internal/engine/engine.go:58-60`) is charged by CSS imports, fonts, images, and media. JS fetch/XHR does not reference it (Appendix B, command B8). Its own caps are `MaxConcurrentFetches = 6` and `MaxFetchBodyBytes = 1 MiB` (`internal/js/fetch.go:17-20`) against `MaxResponseBytes = 16 MiB` for documents (`internal/net/http.go:28`). Withdrawn: this row claimed "the comment in `internal/js/fetch.go:18-20` still says the response cap is 8 MiB" — measured 2026-11-14, no 8 MiB fetch cap exists anywhere in the tree, and the note it cited read `MaxResponseBytes = 16 MiB` at HEAD, agreeing with `net`. The claim was false before the comments were removed and is now unciteable.

Streaming navigation and downloads intentionally bypass the document cap (`internal/net/http.go:517-521`, `cmd/goosie/document_loader.go:132-147`). That is a defensible product decision, but it must be stated as one in the security model rather than left implicit.

### 2.6 Application-layer snapshot

The application is further along than the roadmap implies, but the gaps are sharp:

| Area | State | Evidence |
| --- | --- | --- |
| Address bar | URL/host/search heuristic exists; a bare single word becomes `https://word` rather than a search | `cmd/goosie/page_load.go:91-136`, especially `:133-136` |
| Omnibox suggestions | Search/bookmark/history/internal pages; history scan is duplicated; no incognito filter; not cleared on tab switch | `cmd/goosie/main.go:374-478`, `internal/toolbar/toolbar.go:284-295` |
| Tab labels | Tabs render the document URL, not the document title | `cmd/goosie/navigation.go:235-240`, `internal/tabs/draw.go:108` |
| Session restore | Saves URL/title/history, no scroll position; restored background tabs are not loaded | `internal/session/session.go:15-18`, `cmd/goosie/interaction.go:701-704` |
| Private mode | Tab-level incognito; profile bookmarks/history still feed suggestions | `cmd/goosie/config.go:114`, `cmd/goosie/main.go:374-478` |
| Find | Counts and scrolls to matches; no match highlight is painted | `cmd/goosie/interaction.go:632-633`, `internal/engine/engine.go:1235` |
| Zoom | Applied as a paint scale (`dpr * zoom`), not as a layout viewport change | `cmd/goosie/presentation.go:92-94` |
| Permissions | JavaScript, images, cookies only | `internal/perms/perms.go:15-19` |

These are user-visible, but they are downstream of the compatibility problem. They are deferred in Section 8 rather than mixed into this design.

---

## 3. Compatibility verdict

### 3.1 What is sound and must be preserved

| Decision | Why it holds |
| --- | --- |
| Parse → Style → Layout → Paint stage boundaries | Each stage has a clear input and output; the seams are the right places to make work local. |
| Flat `ObjectID uint32` arena, not a pointer-linked box tree | Index-based storage is the right substrate for stable identity, generation-scoped slots, and future reuse. |
| Per-node immutable-by-replacement computed styles | Replacing a style pointer is cheap and makes invalidation explicit. |
| `PaintChecked` as the untrusted-input contract | It gives binaries one validated entry point; keep it. |
| Import rules and no-mutable-globals enforced by `archtest` | The layering constraints are real and mechanically checked (`internal/archtest/rules.go:20-52`). |
| Explicit per-document and per-image caps | The caps are concrete and mostly well placed; the fix is coverage, not replacement. |

### 3.2 Load-bearing liabilities

| # | Liability | Impact on compatibility | Evidence |
| --- | --- | --- | --- |
| L1 | No stable box identity across reflow | Incremental style/layout, scroll anchoring, animation continuity, and any per-box cache cannot be built; pointer-keyed maps are fragile. | `internal/layout/arena.go:127-129` (false stability claim), `internal/layout/block.go:401`, `internal/layout/block.go:417` (manual re-fetch), `internal/paint/builder.go:10` (`map[*layout.Object]SelSpan`) |
| L2 | Layout is a display-property `else-if` chain, not formatting contexts | A fix for flex shares control flow with grid, table, block, and float; no context can be measured, invalidated, or tested in isolation. | `internal/engine/engine.go:889-894`, `internal/layout/block.go:175-183` |
| L3 | No selector dependency data; mutation always full-resolves | Every DOM write pays a whole-document style pass and layout rebuild; the interactive path still scans every node. | `internal/engine/engine.go:1607`, `internal/engine/engine.go:1614+`, `internal/style/incremental.go:68-68`, `internal/style/incremental.go:170-208` |
| L4 | Paint is coupled to the layout arena; no committed visual goldens | A layout refactor cannot be validated against a frozen baseline; raster regressions are invisible to the checked-in suite. | `internal/paint/builder.go:3` (imports `layout`), `cmd/goosie/presentation.go:94-101`, zero PNGs under `testdata/` |
| L5 | Compatibility evidence is not independent or reproducible | Published pass numbers cannot be regenerated from checked-in artifacts; the JS/DOM conformance surface is skipped. | `test/wpt/wpt_runner.go:157-189`, `test/wpt/curate.go:325-328`, `testdata/parity.py:95-128`, zero committed render references |
| L6 | Admission is fragmented | A hostile or buggy script can grow the DOM past document caps and consume network budget outside the shared accounting. | `internal/js/js.go:1458-1470`, `internal/js/js.go:431`, `internal/js/js.go:1319`, `internal/engine/engine.go:58-60` |

### 3.3 The diagnosis

Two facts compound:

1. **Every compatibility fix is currently unverifiable.** The corpus is 80 small local fixtures scored against a live Chromium whose build is not recorded, at a 30/channel tolerance; the WPT pilot is 8 locally authored pairs; every assertion test (`testharness.js`) is skipped. On the latest main CI run, the same parity job averages 93.54% with 14 fixtures below the 90% gate, while the docs report 95.99% on macOS (Section 5.0). A change can improve the rendered result and still be reported as a regression, or regress an unmeasured surface and be reported as a pass.
2. **Every layout change is total.** The arena is rebuilt, the display list is rebuilt, and any mutation re-resolves the whole document. A conformance fix therefore has a large blast radius and no cheap way to localize the damage.

The engine can add features faster than it can prove them. That is the compatibility bottleneck — not any single missing CSS property.

---

## 4. Target architecture

The target keeps the existing stage boundaries and refactors on their seams. It introduces **one architectural spine**:

> **Give boxes stable identity, and make formatting contexts explicit layout roots.**

The two halves are coupled: a formatting context can only be laid out or invalidated independently if its boxes can be named across generations.

### 4.1 `BoxKey` → `BoxSlot` identity

**Definition.** A `BoxKey` is stable for the lifetime of a document. A `BoxSlot` (`ObjectID`) is valid only within one layout generation.

| Box kind | Key |
| --- | --- |
| DOM element box | `NodeID` |
| Inline fragment of a split element | `(NodeID, fragmentIndex)` |
| `::before` / `::after` box | `(NodeID, before/after)` |
| List marker | `(NodeID, marker)` |
| Anonymous block wrapping inline content | `(parentKey, role, ordinal)` |
| Generated table row/cell | `(parentKey, role, ordinal)` |
| Word/text run | `(textNodeID, runeStart, runeEnd)` |

**Rules.**

- `Arena` gains `byKey map[BoxKey]ObjectID`, populated by `Build`.
- `Session` retains `map[BoxKey]BoxSlot` across reflows so identity survives a rebuild.
- `Alloc`/`Get` stop promising pointer stability. The rule is: **a `*Object` is valid only until the next call that can allocate; hold `ObjectID`, re-fetch at use.** Nothing in the source states it any more — the note on `Alloc` went with the 2026-11-14 comment removal — so this document and `archtest.ArenaPointerMaps` are the rule's only homes, and the manual re-fetch sites (`internal/layout/block.go:401`, `internal/layout/block.go:417`, `internal/layout/block.go:504`) are the pattern rather than a workaround.
- Pointer-keyed maps are replaced by `BoxKey`: paint selection (`internal/paint/builder.go:10`), and the uncommitted pseudo-background maps (`internal/engine/engine.go:48-50`, dirty).
- For the first landing, keep one arena per generation and **copy forward** unchanged boxes by key. Copying is cheap; the expensive operation is layout, not allocation. A persistent arena is a later optimization, only if counters justify it.

**Acceptance evidence.**

- Key↔box bijection asserted on the largest text-heavy fixture and on a table/flex/pseudo fixture.
- No `*layout.Object` held across an allocating call: grep/archtest rule plus review of the three current re-fetch sites.
- The 80 render fixtures produce a zero pixel diff before and after the keying change.

**Status (2026-11-12, SP-4) — L1 landed.** The `BoxKey` type and its accessors
already existed before this sprint; SP-4 added the missing identity machinery.
`Arena` now carries `byKey map[BoxKey]ObjectID` plus monotonic `wordSeq`/`anonSeq`
ordinals, and every key site (root, element, pseudo, first-letter, word,
anonymous block/row/cell) routes through `a.setKey` (`internal/layout/arena.go`,
`inline.go`, `anonymous.go`). Paint selection and find marks are keyed by
`BoxKey`, not `*layout.Object` (`paint.SelSpan.Key`, `paint.FindMark.Key`). The
false pointer-stability comment on `Alloc` is corrected — and the note itself is gone since 2026-11-14, so the rule survives only here and in the gate — and the re-fetch rule is
enforced by `archtest.ArenaPointerMaps` (`internal/archtest/rules.go`), which
forbids any `map[*layout.Object]…` in `internal/` and `cmd/`. Bijection oracle:
`test/layout/boxkey_bijection_test.go`. Zero-diff verified on the **105**-fixture
corpus (the corpus grew past the 80 cited above): 105/105, average 96.39%,
known gaps 4 fixtures / 6 bands unchanged. Correction to §2.2/§4.1: the
pseudo-background maps are keyed by `*style.ComputedStyle`
(`internal/engine/engine.go:58-59`), **not** `*layout.Object` — a separate
identity surface that the arena-pointer archtest rule does not cover and that
SP-4 did not migrate; it remains an open liability. The cross-reflow
`Session map[BoxKey]BoxSlot` persistence and copy-forward are still target-only
(SP-6+, deferred behind L3/L4); L1 ships within-generation identity.

### 4.2 Formatting contexts as layout roots

**Definition.** A formatting context (FC) is a layout root with its own measure/arrange contract. The existing algorithms are wrapped, not rewritten:

```
type FCKind int // block-flow, inline-flow, flex-row, flex-column, grid, table, list, positioned

type LayoutPass interface {
    Measure(containing BlockSize) Size
    Arrange(origin Point) Size
    ContentSize() Size
    FloatExtents() (left, right float32)
}
```

A pass registry maps the used `display` value to a `LayoutPass` implementation. The first landing dispatches to exactly the same functions in exactly the same order as the current `else-if` chain (`internal/layout/block.go:175-183`). This is a behavior-preserving refactor, and its acceptance test is a zero diff on the fixture corpus and the WPT reftest subset.

The second landing makes FC boundaries authoritative: the block pass descends into FC roots and merges their float extents, instead of patching containment locally. `startsNewFC` (`internal/layout/block.go:2212`) is the seed for FC-root detection, but its predicate must be widened to the CSS rule set (floats, absolutely positioned boxes, inline-blocks, flex/grid items, table cells, overflow not visible, `flow-root`).

**Acceptance evidence.**

- Registry landing: 80/80 fixture diff is zero; the same style/layout/work counters are recorded per FC.
- FC-root landing: float and margin containment tests pass without the local `floatBottomL/R` merge; a deliberately nested float/margin fixture matches Chromium.

**Status (2026-11-12, SP-4) — L2 first landing landed; second landing deferred.**
The display `else-if` chain is now a pass registry: `fcRegistry` + `lookupPass`
(`internal/layout/fc.go`) dispatch the used display value to the same pass
functions, with the block-flow body kept byte-for-byte. The dispatch site is
`internal/layout/block.go:180-191` (the `ContentVisibility` branch, then the
`lookupPass` branch at 261, then the block-flow `else`), not the stale
`248-256` cited above. Each pass increments `Arena.PassCounts [5]uint64` indexed
by `FCKind` (`FCBlockFlow`, `FCTable`, `FCGrid`, `FCFlexRow`, `FCFlexColumn`),
surfaced via `Arena.FCCount` and `Session.LayoutPassCounts` (read under `opMu`).
The registry is populated in `init` to avoid a static initialization cycle (every
pass calls back into `lookupPass`). Oracles: `test/layout/fc_registry_test.go`,
`test/engine/fc_counts_test.go`. Zero-diff verified: 105/105, 96.39%, gate
counters unchanged. Correction: `startsNewFC` is at `internal/layout/block.go:2212`
(not `2659`); it still answers only the margin-collapse question. The second
landing (authoritative FC roots) is **deferred to SP-5** with measured evidence:
the `scripts/verify-fc-containment-geometry.js` anchor + `testdata/fc-containment/`
fixture show Goosie already matches every containment **height**; the residual
divergence is **positioning** (an inline-block sole-child-of-flow-root lands at
the origin; a flow-root group margin is dropped), which is a different root cause
than the containment refactor drafted here and needs its own re-scoped plan.

**Status (2026-11-12, SP-5) — L2 second landing landed via `flow-root`.** The
positioning divergence above traced to a single root cause: `parseDisplay`
(`internal/style/style.go`) had no `flow-root` case, so it fell through to
`DisplayInline` and every `.grp{display:flow-root}` wrapper became inline,
breaking block flow and margin handling. SP-5 parses `flow-root` to `DisplayBlock`
plus a `FlowRoot bool` flag (`applyDisplay`), and `startsNewFC`
(`internal/layout/block.go:2212`) honors the flag. Correction to the SP-4 note:
`startsNewFC` already drives **both** float containment (`block.go:438`,
`if floatBottom > y && startsNewFC(...)`) and the margin-collapse boundary — not
only margin collapse — so honoring `FlowRoot` there lands containment without the
full authoritative-FC-root refactor drafted above (which remains target-only).
`layoutStyleKey` (`internal/engine/style_diff.go`) carries `FlowRoot` so a toggle
is layout-affecting. The anchor fixture is promoted to
`testdata/layout-parity/float-containment-001.html` + `.reference.json` (9 boxes,
gated by `TestLayoutParityWithChromium` at 1px); Goosie now matches Chromium on
every box, height **and** position. The non-gated `testdata/fc-containment/`
copies are removed and `scripts/verify-fc-containment-geometry.js` points at the
promoted fixture. Oracles: `test/style/flow_root_parse_test.go` (parse +
non-inheritance), `test/layout/fc_flow_root_test.go`,
`internal/engine/style_diff_flow_root_test.go`. Zero-diff preserved: 105/105,
96.39%, gaps 4/6; layout-parity now 9 fixtures green.

### 4.3 Dependency-tier invalidation

The resolver's rule index already buckets selectors by key. Extend each compiled rule with `Deps`:

- key classes, IDs, tag names, and attribute names;
- state pseudo-classes (`:hover`, `:focus`, `:focus-visible`, `:active`, form states);
- combinator direction (ancestor, descendant, sibling, `:has`);
- structural and functional pseudos that cannot be localized.

On a DOM or state change, classify the work into three tiers:

| Tier | Scope | Examples |
| --- | --- | --- |
| A: subtree | Changed node and descendants | `:hover` on a node matched by a compound selector; class changes whose rules only target the subtree |
| B: FC root | Restyle and re-layout one formatting context | Sibling combinators inside one FC; `:has` whose anchor is inside the FC |
| C: full document | Everything else | Cross-FC combinators, structural changes, viewport/font/stylesheet changes, unknown rules |

Every fallback records an enumerated reason (`fallback_cross_fc`, `fallback_structural`, `fallback_unknown_rule`, `fallback_viewport`). Tests assert **which tier ran**, not just that the output matched. A randomized differential test compares incremental resolution against a full resolve after every mutation; one mismatch fails the tier.

Anonymous and pseudo boxes are regenerated per dirty FC root from the persisted `BoxKey` parent. This retires the anonymous-box veto in `RefreshComputedStyles` (`internal/layout/arena.go:141-141`, `internal/layout/arena.go:149-157`).

### 4.4 Dirty-root reflow

With identity and FC roots in place, a mutation becomes:

1. Apply the DOM change; classify invalidation (Section 4.3).
2. Restyle the affected nodes; mark the nearest enclosing FC roots dirty.
3. If a dirty root's containing block or intrinsic size can change, propagate to its parent FC root; if the propagation crosses roots, degrade to Tier C.
4. Copy forward unaffected boxes by `BoxKey`; measure/arrange only dirty roots.
5. Rebuild the display list. List diffing by `BoxKey` is a later optimization, not part of this landing.

**Acceptance evidence.**

- A work counter proportional to dirty roots (not document size) on a fixture with many siblings.
- Pixel equality between incremental reflow and a forced full reflow on the entire fixture corpus and the interaction fixtures.
- The existing `WorkCounts` counters (`StyleNodeVisits`, `LayoutBuilds`, `Allocs`, tiles rasterized vs reused) are the measurement currency.

### 4.5 Paint identity

Paint currently imports `layout` directly (`internal/paint/builder.go:3`) and keys selection by `*layout.Object` (`internal/paint/builder.go:10`). Replace those keys with `BoxKey`. A fully independent display-list IR is not required for this design; the goal is that a display command can name its box without holding a pointer into the arena.

**Status (2026-11-12, SP-5) — paint identity complete.** The display-list side
landed in SP-4 (`paint.SelSpan`/`paint.FindMark` carry `Key layout.BoxKey`). SP-5
removed the last long-lived held arena pointer on the engine side: `findMark`
(`internal/engine/engine.go`) now stores `key layout.BoxKey` instead of
`obj *layout.Object`, the `Find` producer records `words[first].Key`, and
`internal/engine/paint.go` emits `m.key`. The transient overlays (focus, select
listbox, validation bubble, inspect highlight) are **not** held pointers: each is
re-fetched per paint via `Session.objectFor` from a DOM-node anchor
(`internal/engine/paint.go`), so they are walk-local and exempt by design. The
archtest guard is widened from maps to fields: `ArenaPointerMaps` (SP-4) forbids
`map[*layout.Object]…` across `internal/`+`cmd/`, and `ArenaPointerFields` (SP-5,
`internal/archtest/rules.go`) forbids `*layout.Object` struct fields in
`internal/engine`; func-typed fields and parameters are not flagged. Oracles:
`internal/archtest/pointer_field_test.go`; behavior guarded by the existing
`TestFindHighlightsPainted` (`test/engine/find_test.go`). Mutation (re-add the
`obj` field) turns the archtest red. Zero-diff preserved: 105/105, 96.39%.

### 4.6 Admission and teardown

- **One fallible insertion path.** Add a document-scoped mutation budget (nodes, depth, attributes, attribute bytes) and make `AppendChild`/`InsertBefore` return an error at the single choke point (`internal/dom/node.go:116`, `:153`). The parser keeps `ParseBounded` and an explicit budget bypass because it validates during parse. JS maps the error to a `DOMException`. `validateArena` remains the backstop, not the first line of defense.
- **One network budget.** Charge JS fetch/XHR against the same `subBudget` as CSS/font/image/media (`internal/engine/engine.go:58-60`); its 1 MiB request-body and 16 MiB response caps (`internal/js/fetch.go:17-20`, `internal/net/http.go:28`) already agree with the document path, so the work is the budget, not the numbers.
- **One teardown contract.** `Session.Close` cancels deferred work and waits for in-flight loads before releasing; the JS runtime exposes a context or a wait primitive; the font registry is bounded or generation-scoped.

**Acceptance evidence.**

- A script-generated node flood is rejected with a DOM exception and the document remains within `MaxDocumentNodes`/`MaxDocumentDepth`.
- A fetch-heavy page is accounted against the shared subresource budget.
- `go test -race` closes a session with in-flight loads without a leak or a late callback into a dead document.

### 4.7 What this design deliberately does not change

- Layout algorithms (flex remains single-line, grid remains its subset, table remains heuristic).
- Text measurement and line breaking: there is no shaping or bidi stage; `Metrics` exposes per-glyph advances (`internal/layout/arena.go:96-112`).
- One compositing layer per document (`cmd/goosie/main.go:300`).
- Chrome rendering, overlays, and the application event model.

These are real compatibility gaps, but they are feature work. They are in the Deferred register, not in the spine.

---

## 5. Verification architecture

The current harness mixes three different kinds of evidence under one "parity" label. They must be separated because they answer different questions and fail for different reasons.

### 5.0 Current state of the loop (2026-10-24)

The compatibility loop is not green on the snapshot commit. The latest `main` CI run for `63d226b` (`gh run view 40069270435`, 2026-10-24T06:12Z) reports:

| Job | Result | Evidence |
| --- | --- | --- |
| WPT pilot reftests | pass (36s) | run `40069270435` |
| Build, vet, unit tests | fail | `TestLinkedDependenciesPinned`: `golang.org/x/sys@v0.48.0` is not in `test/gatecheck/testdata/dependency-licenses.txt` (`test/gatecheck/licenses_test.go:47`) |
| Race detector | fail | `TestManyQuerySelectorAll` timed out under race (`js: script execution interrupted`, `test/engine/adversarial_regression_test.go:70`), plus the same license pin |
| Dependency vulnerability scan | fail | one Go standard-library vulnerability in `crypto/x509`, fixed in Go 1.26.10; the runner used 1.26.9. Trace enters at `internal/net/http.go:791` |
| Render parity | fail | average 93.54%, 14 fixtures below 90%; worst `colors/01-text-background` at 80.33% |

The parity result is the important one for this review: the corpus that `docs/qa-test-plan.md:98` reports at 95.99% on macOS averages 93.54% on Ubuntu CI, with 14 fixtures below the 90% gate. That gap does not show that the engine changed; it shows that the oracle and platform are not controlled. A number without a runner, browser build, and font set is not a compatibility measurement.

The split also shows the signal is fragmented: the cheapest test is green, the unit/race gates are red for an unrecorded dependency license and a race-timeout, the audit is red for a standard-library patch level, and the parity job is red for reasons its report does not separate from engine behavior.

### 5.1 Three evidence classes

| Class | Question | Oracle | Failure meaning |
| --- | --- | --- | --- |
| **Conformance** | Does the engine implement the platform correctly? | Upstream WPT (testharness + reftests) at a pinned commit | A spec behavior is wrong or unsupported. |
| **Regression** | Did we break what already worked? | Committed goosie goldens + deterministic unit/gate tests | A known-good output changed. |
| **Parity** | Does it look/behave like the reference browser? | Pinned Chromium/Playwright render | A visual or interaction difference from the reference. |

The current reports blur these. In particular:

- The "WPT pilot" is 8 locally authored test/reference pairs under `testdata/wpt-pilot` (16 HTML files; Appendix B, command B9). It is a useful smoke test, not upstream WPT conformance evidence.
- **Reftests are same-browser by design.** A WPT reftest renders the test page and its reference page with the browser under test; that is the methodology, not a flaw. The runner follows it (`test/wpt/wpt_runner.go:157-189`). The flaw is the evidence claim: the pilot is local and tiny, and the docs present it as conformance. The runner already has a Chromium reference-directory option (`test/wpt/wpt.go:92-94`); reports must state which mode produced them.
- The blanket `testharness.js` skip is gone: `test/wpt/curate.go` curates ~25 `Type: TestHarness` directories and `test/wpt/testharness.go` executes them in Goosie's real document (corrected 2026-11-12, SP-2 audit). What remained open was running the upstream subset in CI — the nightly `wpt-conformance` job now does this against the pinned commit.
- On a dimension mismatch, the WPT comparator scores only the overlapping region (`test/wpt/wpt_runner.go:273-273`). A page can be the wrong size and still pass. That must become a failure, or a separately reported error.
- Render parity renders Chromium live through Playwright (`testdata/parity.py`) at a 30/channel tolerance and 90% pixel pass. Since SP-2 (2026-11-12) the report records the Chromium build, CI installs exact pins from `testdata/requirements.txt`, and both oracles are asserted at test start against `testdata/oracle-pins.json` (render: playwright 1.49.1 / Chromium 131; interaction: playwright 1.63.0 / Chromium 153, matching `testdata/interaction-parity/reference-meta.json`). The two-oracle split is deliberate and documented; unifying it needs its own re-baseline of the parity corpus or the interaction references.

### 5.2 Target: conformance

- Run **upstream WPT** at the pinned commit (`testdata/wpt-config.json:2`) with a published directory allowlist and a fixed unsupported taxonomy. Keep the current commit pinning method; make the allowlist and counts part of the report.
- **Unskip testharness.js** by binding it to the goja DOM. Report pass/fail/unsupported/error/timeout with a reason from a closed taxonomy. Do not mark a failing test "unsupported" to improve the rate (`docs/engine-compat-roadmap.md:52` already states this rule).
- Keep the local pilot as a smoke test, labeled as such.
- Fail or separately report dimension mismatches; do not score the overlap as a pass.
- `TestWPTSuite` is opt-in (`WPT_RUN_SUITE=1`, `test/wpt/wpt_test.go:41-44`); `go test ./...` never starts it, and per-PR CI runs only the 8-pair local pilot (corrected 2026-11-12, SP-2 audit). The nightly `wpt-conformance` job owns the conformance run with an explicit checkout cache keyed by the pinned commit.

### 5.3 Target: regression goldens

- Commit goosie-rendered PNGs for the fixture corpus and key journey pages, with SHA-256, engine commit, platform, DPR, and font-set metadata.
- Regenerate only through an explicit, reviewable command (for example `make goldens`), never implicitly in CI.
- Since SP-2 (2026-11-12) seven goosie-rendered PNG goldens are committed (`testdata/render-goldens/`, one per layout family) with SHA-256 manifest entries, recorded with `GOOSIE_SYSTEM_FONTS=0` so the pixels are a function of the binary alone; `test/goldens/` compares pixel-exact and re-records only via `GOOSIE_RECORD_GOLDENS=1`. The corpus-wide Chromium-referenced goldens remain open.

### 5.4 Target: parity

- Record the Playwright version **and** the Chromium build in a lockfile; assert it at test start; write it into every report. A parity number without an oracle version is not evidence.
- Keep the 30/channel, 90% threshold as a **regression fence**, not as a conformance claim. Publish per-fixture scores and denominators.
- Commit the Chromium references or generate them from the pinned build. Do not compare a locally generated reference against a CI-generated one.
- Interaction parity already has the right shape: checked-in event logs plus browser metadata (`testdata/interaction-parity/reference-events.json`, `testdata/interaction-parity/reference-meta.json`). Extend the fixture count and keep the metadata current.

### 5.5 CI and timing

- Add a **macOS ARM64** job. `internal/platform/darwin` is cgo-only and its tests carry `darwin && cgo` tags; today they run only on release tags (`.github/workflows/release.yml:20-24`). PRs can break the window, dialog, clipboard, and media shims without any signal.
- Keep Ubuntu for unit, race, archtest, and WPT-subset jobs.
- **Timing policy:** measure and report; do not assert absolute milliseconds. If a stable runner class exists, use a generous regression fence against a recorded baseline on that runner, and encode "no baseline" as no gate. Wire `scripts/v2-gate-check.sh` to the macOS job; today it is referenced only by tests and docs (Appendix B, command B11). This matches the existing rule that gate CI asserts counters, not durations (`test/gate/m1_gate_test.go:3-3`).

### 5.6 Reporting contract

Every published compatibility number must include:

- the denominator and the skip count;
- the oracle (WPT commit, or Playwright/Chromium build);
- the engine commit and date;
- the evidence class (conformance, regression, or parity);
- the unsupported taxonomy and the reason counts.

Live-URL sweeps (`testdata/urls.txt`) remain exploratory. They are useful for finding failures, not for release evidence.

---

## 6. Sequencing

The sequence is dependency-ordered capability work, not a schedule. Each milestone has an acceptance test that can fail.

### V0 — Make compatibility falsifiable (precondition)

- Restore the existing gates to green: pin `golang.org/x/sys@v0.48.0` in `test/gatecheck/testdata/dependency-licenses.txt`, make `TestManyQuerySelectorAll` robust under `-race` (or scope its timeout to the race build), and move to Go 1.26.10+ for the `crypto/x509` advisory.
- Unskip testharness.js and report a first upstream WPT subset with the fixed taxonomy.
- Add the pinned-oracle lockfile and assert it at test start.
- Commit the first goldens (at least one per layout family: block, inline, float, flex, grid, table, positioning).
- Add the macOS ARM64 CI job.

**Acceptance:** the full CI matrix is green on the snapshot commit; at least one upstream testharness test passes through goja; a parity report records its Chromium build; a golden diff is produced by a deliberate one-pixel change; the macOS job is green.

V0 is a gate, not the spine. Without it, every later claim is unverifiable.

**V0 status (2026-11-12, SP-2):** license pin, QSA race timeout, Go 1.26.10,
macOS ARM64 job, testharness unskip (in code), and Chromium-build recording
were already done before SP-2. SP-2 adds: pinned+asserted oracles
(`testdata/oracle-pins.json`), seven committed embedded-font goldens
(`testdata/render-goldens/`), fail-closed `run-wpt.sh`, the nightly macOS
pacing gate and WPT conformance job, the 105-fixture floor ratchet, and the
docs authority alignment. Oracle unification (one Chromium for both corpora)
is deferred with its own re-baseline budget.

### L1 — `BoxKey` identity

- Add keys, the slot map, the pointer-lifetime rule, and `BoxKey`-keyed paint selection.
- Keep one arena per generation with copy-forward.

**Acceptance:** bijection test; no held `*Object` across allocation; zero fixture diff.

**L1 status (2026-11-12, SP-4):** landed within-generation identity — `byKey`
index, monotonic word/anonymous ordinals, all key sites via `setKey`,
`BoxKey`-keyed paint selection/find, the corrected `Alloc` lifetime rule, and the
`archtest.ArenaPointerMaps` guard. Bijection oracle
`test/layout/boxkey_bijection_test.go`; zero diff on 105/105 fixtures (96.39%).
Cross-reflow slot persistence and copy-forward remain target-only (SP-5+). See
§4.1 for the pseudo-background-map correction (keyed by `*style.ComputedStyle`,
not migrated here).

**L1 status (2026-11-12, SP-5):** paint identity completed — the engine's last
long-lived held arena pointer (`findMark.obj`) is migrated to `key layout.BoxKey`,
and `archtest.ArenaPointerFields` now forbids `*layout.Object` struct fields in
`internal/engine` (maps were already forbidden by `ArenaPointerMaps`). Transient
paint overlays stay walk-local (re-fetched via `objectFor`). See §4.5.
Cross-reflow slot persistence and copy-forward are now **deferred to SP-6+**: the
terrain (hard anon/pseudo-veto test assertions, no per-rule `Deps`, full-rebuild
`Reflow`, stub partial `Refresh`, unused `Plan.Subtree`) means L3/L4 must land
before cross-reflow reuse can be measured. Zero diff preserved: 105/105, 96.39%.

### L2 — Formatting contexts as layout roots

- Add the pass registry and wrap the existing algorithms.
- First landing is behavior-preserving; second landing makes FC roots authoritative for float/margin containment.

**Acceptance:** zero fixture diff at the registry landing; per-FC counters; nested float/margin fixture matches Chromium.

**L2 status (2026-11-12, SP-4):** first landing landed — `fcRegistry`/`lookupPass`
dispatch (`internal/layout/fc.go`), per-FC `PassCounts` surfaced via
`Session.LayoutPassCounts`, zero diff on 105/105 (96.39%), gate counters
unchanged. Second landing **deferred to SP-5** with evidence: the
`testdata/fc-containment/` anchor shows containment heights already match
Chromium and the residual gap is positioning (inline-block-in-flow-root placement
and a dropped flow-root group margin), a different root cause needing a re-scoped
plan. See §4.2.

**L2 status (2026-11-12, SP-5):** second landing landed via `flow-root` — the
positioning gap traced to `parseDisplay` lacking a `flow-root` case (fell through
to inline). `flow-root` now parses to `DisplayBlock` + a `FlowRoot` flag,
`startsNewFC` (`internal/layout/block.go:2212`) honors it for both float
containment and margin collapse, and `layoutStyleKey` carries the flag. The
anchor fixture is promoted to `testdata/layout-parity/float-containment-001.*`
(gated, 9 boxes match Chromium at 1px); the non-gated copies are removed. Zero
diff preserved: 105/105, 96.39%, gaps 4/6; layout-parity now 9 fixtures. The full
authoritative-FC-root refactor (descend-and-merge float extents) remains
target-only. See §4.2.

### L3 — Dependency-tier invalidation

- Add `Deps` per rule and the three tiers with enumerated fallback counters.
- Randomized differential test: incremental vs full resolution.

**Acceptance:** a class/attribute mutation restyles only its tier; the counter names the tier; the differential test is clean over a fixed mutation corpus.

### L4 — Dirty-root reflow

- Reuse unaffected boxes by key; measure/arrange only dirty FC roots.
- Display-list diffing remains deferred.

**Acceptance:** work proportional to dirty roots; incremental output equals full reflow on the fixture and interaction corpora.

### L5 — Admission and teardown

- Fallible insertion path with a document mutation budget.
- JS fetch charged to the shared budget.
- `Close` cancels and joins deferred work; font registry bounded.

**Acceptance:** node flood rejected; fetch accounted; race close with in-flight loads.

**Dependency graph:** V0 → L1 → L2 → L3 → L4. L5 depends only on V0. L3 and L5 can proceed in parallel with L2 if ownership of `style` and `dom/js` is separate.

### The headline recommendation

> **Make compatibility falsifiable (V0), then make it local (L1–L2).** Stable box identity and explicit formatting contexts are the single architectural investment that lets every later conformance fix be measured, isolated, and kept.

---

## 7. Risks and falsification experiments

| Risk | Why it could be wrong | Cheapest falsifier |
| --- | --- | --- |
| FC wrapping changes pixels | Float and margin behavior currently leaks through implicit boundaries (`internal/layout/arena.go:51-53`, `internal/layout/block.go:379-388` dirty). Making boundaries explicit may surface latent bugs. | Land the registry without changing behavior; run the 80 fixtures. Any nonzero diff falsifies "behavior-preserving". |
| `BoxKey` collisions or mis-keying | Anonymous and word boxes have no natural key; ordinals can shift when content changes. | Assert key↔box bijection on the largest text-heavy fixture, a table fixture, and a pseudo-element fixture. |
| Tiered invalidation is unsafe | Combinators and `:has` create cross-subtree dependencies that are easy to under-approximate. | Randomized DOM-mutation differential (incremental vs full resolve) over a fixed 100-mutation corpus. One mismatch fails the tier. |
| testharness integration yields little | The goja DOM surface may be too thin for most tests, producing mostly missing-API failures. | Run 100 unskipped testharness tests. If most fail for missing APIs, keep testharness as a reporting surface and re-scope V0 to goldens plus upstream reftests. |
| Pinned Chromium is the wrong oracle | Chromium is an implementation, not the spec; goldens become browser snapshots. | Render the golden corpus under two Chromium majors. A high diff rate means goldens must be labeled as browser snapshots, and WPT remains the conformance oracle. |
| The architecture is not the real bottleneck | If the goal is short-term visible parity, text shaping and multi-line flex may yield more user-visible improvement per unit of work. | Compare one sprint of feature work (for example flex wrapping) against one sprint of L1–L2 on the same fixtures. If features move parity more without increasing regression risk, reorder. |

The last risk is real and should be decided by the maintainer, not hidden. This document recommends the architecture because the evidence shows the *measurement* gap is the binding constraint for a sustainable compatibility program. If the goal is a demo rather than a program, the ordering changes.

---

## 8. Deferred register

Each item is deliberately excluded from this design. "Evidence" names the code that shows the gap.

| Deferred capability | Why deferred | Evidence |
| --- | --- | --- |
| Text shaping and bidi | A layout-quality feature, not an identity/invalidation concern; needs its own design and reference corpus. | Per-glyph `Metrics` (`internal/layout/arena.go:96-112`); no shaping/bidi stage in the pipeline. |
| Table conformance | Heuristic sizing; needs a fixture corpus and likely a separate algorithm. | `internal/layout/table.go:467-470` (column distribution). |
| Sticky/fixed scroll tracking | Requires a scroll-aware positioning context; currently parent/ICB-relative. | `internal/layout/position.go:15` (no scroll argument), `:51`, `:91`. |
| Multi-layer compositing | One layer per document today. | `cmd/goosie/main.go:300`. |
| Canvas stubs and custom-element gaps | JS/API surface, separate from the layout spine. | `internal/js/canvas.go:1110-1121`, `internal/js/custom_elements.go:129-150`. |
| HSTS test coverage | Security verification gap; no test references exist. | `internal/net/http.go:463`, `:490`; Appendix B, command B12. |
| Chrome UX: titles/favicons, restore, private window, omnibox search, find highlight, zoom reflow, menus | User-visible, but downstream of engine compatibility and expensive without a chrome widget layer. | Section 2.6. |
| Uncommitted SVG/flex/select/float parity work | Working-tree state at snapshot; not reviewed as a coherent change. | `git status` (8 modified, 6 untracked). |
| Edits to `PROJECT.md`, `docs/roadmap-v2.md`, `docs/engine-compat-roadmap.md`, `docs/qa-test-plan.md` | This document recommends reconciliation (Section 9); it does not perform the edits. | Section 9. |

**Correction (2026-11-14) — the multi-line flex row above is withdrawn.** It claimed the
implementation was "explicitly single-line" and pointed at a `layoutFlexRow` comment.
Read on the current tree: `layoutFlexRow` (`internal/layout/block.go:492`) breaks items
into lines (`internal/layout/block.go:631-650`), places each line's cross axis through
`distributeFlex(obj.Style.AlignContent, …)` (`internal/layout/block.go:791`), and
`layoutFlexColumn` (`internal/layout/block.go:1022`) does the column case; 13 tests in
`test/layout/flex_multiline_test.go` (wrap row, two-per-line, wrap-reverse, seven
`align-content` cases, wrap with gap, column, no-wrap) pass on this host
(`go test ./test/layout/ -run 'FlexWrap|AlignContent'`, ok 0.698s). The citation was also
mis-pointed at HEAD: `internal/layout/block.go:571-577` was the float-clearance seed note,
not a flex one. So the row described neither the code nor its own evidence, and the review
had already labelled this class **D** — documented but contradicted by code.

---

## 9. Reconciliation appendix

This document is additive. It does not silently rewrite the existing docs; each claim below needs a targeted correction.

### `PROJECT.md`

- **Pass order.** `PROJECT.md:31-33` says layout runs block, inline, float, flex, grid, table, positioning. The code runs `Block`, `Inline`, `Positioning`, with flex/grid/table selected inside `Block` (`internal/engine/engine.go:889-894`, `internal/layout/block.go:175-183`). Correct the sentence or mark it as intended architecture.
- **Incrementality.** `PROJECT.md:46-50` describes `Session.Refresh`/`RefreshNode` as local plans. They are full rebuilds with no production callers (`internal/engine/refresh.go:16-89`). Label them as scaffolding.

### `docs/roadmap-v2.md`

- **P0 address bar** (`:33`) is partial: a bare word becomes `https://word` (`cmd/goosie/page_load.go:133-136`).
- **P0 tab labels** (`:34`) is partial: tabs show URLs, not titles (`cmd/goosie/navigation.go:235-240`).
- **P1 session restore** (`:41`) does not save scroll position (`internal/session/session.go:15-18`) and does not load restored background tabs (`cmd/goosie/interaction.go:701-704`).
- **P0 suggestions** (`:32`) does not filter profile sources for incognito tabs and can remain visible across tab switches (`cmd/goosie/main.go:374-478`, `internal/toolbar/toolbar.go:284-295`).

### `docs/engine-compat-roadmap.md`

- **E1** (`:45-55`) should carry the caveat that the pilot is 8 local pairs and that testharness tests are skipped (`test/wpt/curate.go:325-328`); the upstream full-suite run is separate.
- **E0–E8** should be relabeled against V0/L1–L5 so there is one sequencing vocabulary. E0/E1 map to V0; E2 to L3/L5; E3 to L3/L4; E4 to L2/L4; E6/E7 to L5 and the deferred chrome work; E8 to the counters used as acceptance evidence.
- **The 2026-10-24 color/selector baseline** (`:14-17`) is a real measured result and should stay; it is a subset, not a suite claim, which the document already says.

### `docs/qa-test-plan.md`

- **QA-015 and the summary** (`:92-98`) report 80/80 fixtures at 95.99% average. Those numbers are **claimed, unproven from checked-in artifacts**: the references are generated live by Chromium and no PNGs are committed. The latest CI parity run on the same commit averages 93.54% with 14 fixtures below 90% (Section 5.0), so the 95.99% figure is also platform-specific. Keep the numbers only if the run date, Chromium build, font set, and command are recorded; otherwise mark them "reported, not reproduced".
- The gate/timing language (`:30`, `:245`) should point at the new macOS job and the regression-fence policy.

---

## 10. Decision log and what would change the verdict

### Decision requested

Approve V0 plus L1–L2 as the next compatibility investment, with L3–L5 sequenced behind them and the Deferred register left untouched. V0 begins by making the existing CI matrix green; a measurement program cannot be built on a red gate.

### What would change the verdict

- A production incremental path exists outside the audited files and already gives subtree style/layout with stable identity.
- testharness.js cannot be bound to the goja DOM without a DOM rewrite, making V0's conformance half impractical.
- The registry landing cannot be made behavior-preserving on the fixture corpus, meaning the FC abstraction leaks more than the audit suggests.
- Pinned Chromium proves unstable across runners, making goldens browser snapshots rather than regression baselines (the design already labels them as such; the verdict on parity would weaken).
- Feature work (text shaping, flex wrapping) improves parity materially without increasing regression risk, which would justify reordering.

---

## Appendix A — Claims ledger

Statuses: **V** verified by direct read/command on 2026-10-24; **C** claimed in docs, not reproduced; **D** documented but contradicted by code; **K** known-broken/placeholder.

| # | Claim | Status | Evidence |
| --- | --- | --- | --- |
| A1 | Reflow runs Build, Block, Inline, Positioning only | V | `internal/engine/engine.go:889-894` |
| A2 | Flex/grid/table dispatch inside the block pass | V | `internal/layout/block.go:175-183` |
| A3 | Docs claim six ordered passes | D | `PROJECT.md:31-33`, `internal/layout/arena.go:1-1` |
| A4 | `Alloc` claims pointer stability across growth | D | `internal/layout/arena.go:127-129` |
| A5 | Callers manually re-fetch after growth | V | `internal/layout/block.go:401`, `internal/layout/block.go:417`, `internal/layout/block.go:504` |
| A6 | Anonymous boxes veto style-only reuse | V | `internal/layout/arena.go:141-141`, `internal/layout/arena.go:149-157` |
| A7 | Pseudo boxes have no DOM key | V | `internal/layout/arena.go:333-345` |
| A8 | `Refresh`/`RefreshNode` are full rebuilds | V | `internal/engine/refresh.go:25-28`, `:38-40`, `:78-84` |
| A9 | `Refresh`/`RefreshNode` have no production callers | V | Appendix B, command B6 |
| A10 | DOM mutation calls full `RestyleAndReflow` | V | `internal/engine/engine.go:1574-1607`, `:2186+` |
| A11 | Interactive state path scans the whole tree | V | `internal/style/incremental.go:68-68`, `:170-208` |
| A12 | Interactive fast path requires certified selectors | V | `internal/style/style.go:1175-1176` |
| A13 | Paint imports `layout` directly | V | `internal/paint/builder.go:3` |
| A14 | Paint selection is keyed by `*layout.Object` | V | `internal/paint/builder.go:10` |
| A15 | Display list is rebuilt per repaint | V | `cmd/goosie/presentation.go:94-101` |
| A16 | `List.Reset` has no caller | V | `internal/paint/list.go:22`; Appendix B, command B13 |
| A17 | `Close` does not join deferred loads | V | `internal/engine/engine.go:1511-1526` |
| A18 | JS `appendChild` has no document cap | V | `internal/js/js.go:1458-1470` |
| A19 | `innerHTML` caps are per call | V | `internal/js/js.go:431`, `:1319` |
| A20 | JS fetch is outside the subresource budget | V | `internal/engine/engine.go:58-60`; Appendix B, command B8 |
| A21 | `FetchTimeout` is unused | V | `internal/js/fetch.go:20-20`; Appendix B, command B7 |
| A22 | Font registry appends without a cap | V | `internal/raster/fonts.go:179-187` |
| A23 | Fixed/sticky positioning has no scroll input | V | `internal/layout/position.go:15`, `:51`, `:91` |
| A24 | Flex row is single-line | V | `internal/layout/block.go:417-417` |
| A25 | Text measurement is per-glyph advance | V | `internal/layout/arena.go:96-112` |
| A26 | One layer per document | V | `cmd/goosie/main.go:300` |
| A27 | WPT pilot is 8 local pairs | V | 16 HTML files under `testdata/wpt-pilot` (B9) |
| A28 | WPT pilot renders test and ref with goosie | V | `test/wpt/wpt_runner.go:157-189` |
| A29 | All testharness tests are skipped | V | `test/wpt/curate.go:325-328` |
| A30 | WPT comparator scores only the overlap on size mismatch | V | `test/wpt/wpt_runner.go:273-273` |
| A31 | Render parity uses live Chromium, 30/channel, 90% | V | `testdata/parity.py:36-37`, `:95-128` |
| A32 | CI pins Playwright 1.49 but records no browser build | V | `.github/workflows/ci.yml:94-97`, `testdata/parity.py:95-128` |
| A33 | Interaction refs were captured with Playwright 1.63 / Chrome 153 | V | `testdata/interaction-parity/reference-meta.json` |
| A34 | Zero committed PNG goldens | V | Appendix B, command B10 |
| A35 | CI is Ubuntu-only for PR checks | V | `.github/workflows/ci.yml:19-21`, `:35`, `:47`, `:76`, `:80` |
| A36 | `v2-gate-check.sh` is not wired to CI | V | Appendix B, command B11 |
| A37 | HSTS has no test coverage | V | Appendix B, command B12 |
| A38 | Full WPT suite downloads/reuses a pinned checkout | V | `test/wpt/wpt_checkout.go:15-57`, `testdata/wpt-config.json:2` |
| A39 | 80/80 at 95.99% is reported but not reproducible from checked-in artifacts | C | `docs/qa-test-plan.md:92-98`, `docs/engine-compat-roadmap.md:29` |
| A40 | Tabs show URLs, not titles | V | `cmd/goosie/navigation.go:235-240`, `internal/tabs/draw.go:108` |
| A41 | Session state has no scroll | V | `internal/session/session.go:15-18` |
| A42 | Restored background tabs are not loaded | V | `cmd/goosie/interaction.go:701-704` |
| A43 | Omnibox has no incognito filter and is not cleared on tab switch | V | `cmd/goosie/main.go:374-478`, `internal/toolbar/toolbar.go:284-295` |
| A44 | Bare word becomes an https host | V | `cmd/goosie/page_load.go:133-136` |
| A45 | Zoom is a paint scale, not a layout viewport change | V | `cmd/goosie/presentation.go:92-94` |
| A46 | Find has no painted match highlight | V | `cmd/goosie/interaction.go:632-633`; no paint consumer of `Find` matches |
| A47 | Permissions are limited to JS/images/cookies | V | `internal/perms/perms.go:15-19` |
| A48 | CI on the snapshot commit is red; parity averages 93.54% with 14 fixtures below 90% | V | run `40069270435`; Appendix B, commands B22-B23 |
| A49 | Canvas image and pixel APIs are stubs | K | `internal/js/canvas.go:1110-1121` |
| A50 | Custom-element query APIs return `undefined` for unsupported paths | K | `internal/js/custom_elements.go:129-150` |
## Appendix B — Commands and counts

Run from the repository root on 2026-10-24. Commands are zsh-safe (no reliance on `PIPESTATUS`).

| ID | Command | Result |
| --- | --- | --- |
| B1 | `git rev-parse HEAD` | `63d226bd6fa76a90bbb6efb10d319f665aa2b1d2` |
| B2 | `git status --short` | 8 modified, 6 untracked |
| B3 | `go build ./... ; echo $?` | `0` |
| B4 | `go vet ./... ; echo $?` | `0` |
| B5 | `go test ./test/layout ./test/paint ./test/style`; `go test ./internal/archtest` | pass; archtest 6.854s |
| B6 | `grep -rn '\.Refresh(\|RefreshNode(' --include='*.go' . \| grep -v '_test.go'` | only the `RefreshNode` definition in `internal/engine/refresh.go`; no production callers |
| B7 | `grep -rn 'FetchTimeout' --include='*.go' .` | definition plus one test assertion; no request use |
| B8 | `grep -n 'subBudget' internal/engine/*.go` | engine only; no reference in `internal/js` |
| B9 | `find testdata/wpt-pilot -name '*.html' \| wc -l` | 16 (8 pairs) |
| B10 | `find testdata -name '*.png' \| wc -l` | 0 |
| B11 | `grep -rn 'v2-gate-check' . --include='*' \| grep -v '^./.git'` | tests and docs only; no workflow reference |
| B12 | `grep -rn 'HSTS\|hsts' --include='*_test.go' . \| wc -l` | 0 |
| B13 | `grep -rn 'func (l \*List) Reset\|\.Reset()' internal/paint internal/engine cmd` | definition only; the hit in `internal/engine/engine.go:1587` is a different type |
| B14 | `grep -rh '^func Test' --include='*_test.go' . \| wc -l` | 2,044 test functions in 411 files |
| B15 | `grep -rn 't\.Skip' --include='*_test.go' . \| wc -l` | 35 skips in 18 files |
| B16 | `find testdata/render -name '*.html' -o -name '*.htm' \| wc -l` | 80 fixtures in 20 categories |
| B17 | `find testdata/interaction-parity -name '*.html' \| wc -l` | 7 |
| B18 | `ls testdata/layout-parity/*.html \| wc -l` | 6 |
| B19 | `go version`; `uname -sm` | go1.26.9; Darwin arm64 |
| B20 | `.github/workflows/ci.yml` job scan | 5 jobs: checks, race, audit, wpt-pilot, parity; all `ubuntu-latest` |
| B21 | `wc -l internal/layout/block.go` | 2,953 lines |
| B22 | `gh run view 40069270435` | 5 jobs; only WPT pilot passed; checks and race failed on `TestLinkedDependenciesPinned`; race also timed out in `TestManyQuerySelectorAll`; audit found one stdlib vulnerability; parity failed |
| B23 | `gh run view --job 120798981594 --log \| grep -E 'FAIL  .*\\.png\|average match'` | average 93.54%; 14 failing fixtures; worst 80.33% |

## Appendix C — Glossary

- **BoxKey** — document-lifetime stable name for a box.
- **BoxSlot / ObjectID** — generation-scoped index into the arena.
- **Formatting context (FC)** — a layout root whose internal layout does not depend on the outside except through its containing block; block flow, inline flow, flex, grid, table, and positioned roots are kinds.
- **Tier A/B/C** — invalidation scopes: subtree, one FC root, full document.
- **Conformance / regression / parity** — the three evidence classes in Section 5.1.
- **V0 / L1–L5** — the capability milestones in Section 6.

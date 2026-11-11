# SP-3 — P2 journeys: main menu, responsive toolbar, inspector node highlight

- Date: 2026-11-12
- Status: approved (user delegated autonomous execution without approval gates)
- Source of truth for P2: `docs/roadmap-v2.md` §P2 (lines 116-179) and the P2 gate (line 179)
- Predecessor: SP-2 (V0 measurability); SP-1 (debug removal, hover hardening)
- Closes the three remaining P2 NOT RUN rows: **BR-052**, **BR-142**, **BR-143**
  (`docs/qa-test-catalog.md:74,311,312`).

## Goal

Make the three published P2 journeys run repeatably in the automated suite:
reach the bookmark/history managers from a real main menu (BR-052), keep every
toolbar/tab/address control reachable and non-overlapping when the window is
resized small or large (BR-143), and select a DOM node in the Web Inspector so
it is highlighted in the inspected page without mutating the DOM (BR-142).

The P2 gate (`roadmap-v2.md:179`) is explicit: published flows must run
repeatably in the automated test suite **and** through the native app, and a
general WPT pass rate is not a substitute for user-journey results. This spec
delivers the automated (headless `platform/headless`) half of each journey and
records the native CUA half as display-blocked, exactly as the already-signed
rows BR-050/BR-051/BR-058/BR-140/BR-141 do.

## Current state (audited 2026-11-12)

Already done — do NOT redo:

- Internal managers exist and are reachable: `goosie://bookmarks`,
  `goosie://history`, `goosie://downloads`, `goosie://settings` via the
  in-document nav header (`internalNavHeader`, `cmd/goosie/internal_pages.go:30`)
  and omnibox suggestions (`querySuggestions`, `cmd/goosie/main.go:582`).
  Their INT/SEC mechanics are signed PASS (BR-048/049/050/051/058).
- A reusable dropdown exists: `ContextMenu` (`cmd/goosie/context_menu.go`),
  with `Show/Hide/Rect/UpdateHover/HitItem/Draw`, viewport clamping, and action
  routing through `handleContextMenuAction` (`cmd/goosie/interaction.go:833`).
  It is already drawn in `chromeWindow.Present` (`toolbar_window.go:468`) and
  hit-tested first in `chromeWindow.intercept` (`toolbar_window.go:620`).
- The tab bar is already responsive: `internal/tabs/layout.go`
  (`MinTabWidth=80`, `TabWidthFor`, `ChevronsShown`, `EnsureTabVisible`).
- Unclipped post-walk paint overlays exist as precedent: validation bubble
  (`internal/paint/validation.go`, `Builder.paintValidationBubble`) and select
  listbox (`internal/paint/listbox.go`, `Builder.paintSelectListBox`), both
  emitted after `paintBox` in `Builder.Build` (`internal/paint/builder.go:90`)
  so no ancestor overflow clips them.
- Node→box mapping exists: `Session.objectFor(n)` (`internal/engine/focus.go:2965`,
  caller holds `opMu`) → `*layout.Object` → `BorderRect()`. `dom.Node.Walk`
  (`internal/dom/node.go:1844`) and `Node.ID` (`NodeID uint32`, `node.go:26,54`)
  let the inspector resolve a node by numeric id.
- A headless journey harness exists: `newBrowserE2E` (`cmd/goosie/e2e_test.go:39`),
  `inject`, `waitFor`, `clickToolbarButton`, `navigateViaAddressBar`,
  `window.AccessibilityTree()` + `findAXNode`, `window.Stats().Presents`.
- The deadlock discipline from SP-2/T8b is in force: mutate/read session state
  under `opMu`, capture, unlock, then dispatch DOM events; toolbar host
  callbacks fire after `State.mu` unlock via the `pending []func()` queue.

Open gaps this spec closes:

1. **BR-052 — no main menu.** `internal/toolbar` has no menu/hamburger/overflow
   concept (grep: zero matches). Bookmark/history are reachable only via
   in-document links and omnibox suggestions, not a dedicated chrome menu.
2. **BR-143 — toolbar geometry breaks at small widths.** `internal/toolbar/layout.go`
   is pure arithmetic with no clamping: `AddressBarRect` inverts (`X1 < X0`)
   below width 152; the right-anchored `ButtonDownloads` overlaps Reload/Forward/Back
   below width 144. There is no `MinToolbarWidth`, no overflow affordance.
3. **BR-142 — inspector DOM tree is static.** `renderDOMTreeHTML`
   (`cmd/goosie/internal_pages.go:1812`) emits syntax-highlighted text; nodes are
   not selectable and there is no highlight-in-page mechanism. Selecting a node
   and highlighting it without DOM mutation does not exist.

## Design decisions

### D1 — Main menu reuses ContextMenu; one new right-anchored button (BR-052)

Add a single `ButtonMenu` to the toolbar, right-anchored at the far right
(`x = width - ButtonSize - ButtonGap`). Pressing it opens a `ContextMenu`
anchored to the button's bottom-left, clamped to the viewport, populated with
navigation items:

- **Bookmarks** → `goosie://bookmarks`
- **History** → `goosie://history`
- **Downloads** → `goosie://downloads`
- **Settings** → `goosie://settings`

plus the overflow traversal actions **Back**, **Forward**, **Reload** (needed by
D2 when those buttons collapse). Items route through the existing
`handleContextMenuAction` → `navigateTab` / `traverseTab` / `reloadTab` path —
no new navigation plumbing. The menu is drawn in `Present` and hit-tested first
in `intercept`, alongside the existing right-click context menu (the two share
the `ContextMenu` type but are distinct instances so a right-click menu and the
main menu never fight over one `Visible` flag).

The standalone `ButtonDownloads` is **kept** at normal widths (backward
compatible with `toolbar_test.go:424`, `downloads_test.go:180`,
`accessibilityChrome`); it sits immediately left of `ButtonMenu`. At small
widths it collapses into the menu per D2. `OnDownloads` wiring is unchanged.

Rationale: reuse over reinvention (the repo's "one owner" rule). A general
action list is a `ContextMenu`, not the URL-suggestion machinery.

### D2 — Responsive toolbar layout with a single geometry owner (BR-143)

Introduce one pure function that owns all toolbar geometry:

```go
// internal/toolbar/layout.go
type Layout struct {
    Buttons     []Button     // visible left-cluster + downloads, in paint order
    Rects       map[Button]frame.Rect
    Address     frame.Rect
    Menu        frame.Rect   // always present
}
func ComputeLayout(width int32) Layout
```

Contract (asserted by a geometry-invariant test across widths
`{120, 144, 152, 200, 320, 640, 1280}`):

- `Menu` is always right-anchored, always visible, always within `[0, width]`.
- `Address.X1 > Address.X0` always; `Address` width ≥ `MinAddressWidth` (64px).
- No two visible rects overlap (pairwise intersection empty).
- Every visible rect lies within `[0, width]`.
- Left buttons collapse in reverse priority **Reload → Forward → Back** as
  width shrinks; `ButtonDownloads` collapses before the left cluster. A collapsed
  button is absent from `Buttons`/`Rects` (not painted, not hit-tested, not in
  the AX tree) and its action is available in the main menu (D1).
- At normal widths (≥ 640) the layout matches today's: Back/Forward/Reload at
  x=4/40/76, Downloads then Menu on the right, address bar between.

`ButtonRect(btn, width)` and `AddressBarRect(width)` become thin wrappers over
`ComputeLayout` so existing callers keep compiling; `Draw`, `handlePointerLocked`,
and `accessibilityChrome` iterate `Layout.Buttons` instead of a fixed enum list
so collapsed buttons disappear from every surface at once (one owner). The tab
bar already shrinks to `MinTabWidth` and shows chevrons, so it stays reachable;
the resize journey asserts this rather than changing it.

Rationale: the geometry bug is real and measured (inversion < 152, overlap
< 144). A clamp-only fix would still overlap; collapsing into the menu is the
standard browser overflow pattern and reuses D1's menu as the affordance, so
"still accessible" holds even when a button is not painted.

### D3 — Inspector highlight is a post-walk paint overlay, never a DOM write (BR-142)

Engine surface (mirrors `SetValidationBubble`/`findMarks`):

```go
// internal/engine
func (s *Session) SetInspectHighlight(n *dom.Node)  // resolve objectFor under opMu, store
func (s *Session) ClearInspectHighlight()
func (s *Session) InspectHighlight() *dom.Node      // current selection, nil if none
```

`Session.Paint` wires `builder.SetInspectHighlight(obj)` before `Build`;
`Builder.Build` calls a new `paintInspectHighlight()` **after** `paintBox`
(unclipped, like the validation bubble). The overlay is one semi-transparent
`CmdFill` (blue, `Opacity` ≈ 0.25) plus a `CmdBorder` around `obj.BorderRect()`,
with `Clip` left zero so ancestor overflow never clips it. It stores only an
`*layout.Object` on the Builder/Session — **no attribute, style, or tree
mutation**. If the node is detached or has no box, the overlay is skipped.

Inspector UI (`cmd/goosie/internal_pages.go`):

- `renderDOMTreeHTML` wraps each element row in a link
  `goosie://inspect?tab=<id>&select=<nodeID>` and marks the selected row
  (background highlight) when `nodeID == selected`.
- `buildInspectPageHTML` handles `select`: resolve the node in
  `targetTab.Session().Doc` via `Walk` matching `Node.ID`, call
  `targetTab.Session().SetInspectHighlight(node)`, then arrange a repaint of the
  background target tab so its layer picks up the overlay. The literal token
  `select=clear` (and a "Clear highlight" link) calls `ClearInspectHighlight` +
  repaint. Numeric ids are never used as a clear sentinel.
- The selection is paint-only on the target session; the inspector tab itself
  just re-renders its tree with the selected row marked.

Implementation constraint (cross-tab repaint safety): `buildInspectPageHTML`
runs inside `renderInternalPage` on the inspector tab's load goroutine, before
that tab's session is committed. Applying `SetInspectHighlight` to the *target*
session is safe (it locks only the target's `opMu`), but the target repaint must
NOT call `repaintTab` synchronously here: `repaintTab` ends in
`publishAccessibility`, which reads the *active* tab — the half-built inspector
session. The plan must schedule the target repaint after the inspector
navigation commits (e.g. a deferred background-tab repaint on the chrome pump),
and a headless test must assert no race/panic on the selection path.

Acceptance (headless): selecting a node (a) adds the overlay to the target
session's display list, (b) leaves the DOM byte-identical — node count, max
depth, and the selected subtree's `outerHTML` unchanged before/after — and
(c) marks the tree row. The native visual highlight is CUA/display-blocked and
recorded as such.

Rationale: the validation-bubble precedent already proves an unclipped,
DOM-free overlay; reusing it keeps the highlight out of the cascade and makes
"selection does not mutate the DOM" true by construction.

### D4 — CUA halves are display-blocked, recorded not faked

This session has no display. Each row's automated INT/headless mechanics are
implemented and verified; the native CUA half is recorded as
"native CUA chưa chạy do phiên không có display", matching the signed precedent
(BR-050/051/058/140/141). BR-142 carries a CUA-only tag today; this spec adds
INT coverage for its mechanics and notes CUA pending. No `min_pass_rate` or gate
threshold is lowered to green a red run (SP-2 rule).

## File structure

| File | Responsibility | Change |
|---|---|---|
| `internal/toolbar/layout.go` | single geometry owner | add `ButtonMenu`, `MinAddressWidth`, `ComputeLayout`, `Layout`; wrap `ButtonRect`/`AddressBarRect` |
| `internal/toolbar/toolbar.go` | draw + hit-test | iterate `Layout.Buttons`; add `ButtonMenu` press → `OnMenu` callback; menu icon in `drawButton` |
| `internal/toolbar/state` (in toolbar.go) | callbacks | add `OnMenu func()` fired via `pending` after unlock |
| `cmd/goosie/main.go` | wiring | wire `OnMenu` → open main `ContextMenu`; add menu actions to `handleContextMenuAction` |
| `cmd/goosie/toolbar_window.go` | chrome present/intercept/AX | draw + hit-test main menu; `accessibilityChrome` iterates `Layout.Buttons` |
| `cmd/goosie/interaction.go` | action routing | `handleContextMenuAction` cases: bookmarks/history/downloads/settings/back/forward/reload |
| `internal/paint/inspect.go` (new) | overlay | `InspectHighlight{Obj}`, `SetInspectHighlight`, `paintInspectHighlight` |
| `internal/paint/builder.go` | emission | field + setter + post-walk call |
| `internal/engine/inspect.go` (new) | session state | `SetInspectHighlight`/`ClearInspectHighlight`/`InspectHighlight` |
| `internal/engine/paint.go` | wire overlay | `SetInspectHighlight` before `Build` |
| `cmd/goosie/internal_pages.go` | inspector UI | selectable tree rows + `select` action + cross-tab repaint |

Tests (new): `internal/toolbar/layout_test.go` (geometry invariant),
`cmd/goosie/journey_main_menu_test.go` (BR-052), `cmd/goosie/journey_resize_test.go`
(BR-143), `test/engine/inspect_highlight_test.go` + `test/paint/inspect_highlight_test.go`
(BR-142 overlay + DOM immutability), `cmd/goosie/journey_inspector_test.go`
(BR-142 end-to-end). Existing toolbar/downloads/AX tests updated only where a
hardcoded rect assertion changes under the new layout (documented per case).

## Testing strategy

TDD red→green per task, following the repo's mutation discipline (each oracle
gets at least one mutant that must go red, then restored). Layers:

1. **Geometry unit** (`internal/toolbar`): `ComputeLayout` invariant across the
   width sweep; fails on HEAD below 152px (documents the bug).
2. **Engine/paint unit** (`test/engine`, `test/paint`): overlay present in the
   display list after `SetInspectHighlight`; DOM byte-identical; detached node
   skipped; `ClearInspectHighlight` removes it.
3. **Headless journeys** (`cmd/goosie`, `platform/headless`): main menu opens
   bookmark/history; resize small→large keeps controls reachable + non-overlapping
   and the address bar still navigates; inspector node selection highlights the
   target tab without DOM mutation.
4. **Regression**: `go test ./...`, `go test -race` (affected packages),
   `go vet -all ./...`, `go test ./internal/archtest/`, gate suite, and
   `make parity` (must stay 105/105 @ ~96.39% — chrome UI changes do not touch
   the `testdata/render` content corpus; the overlay is opt-in and unused by any
   fixture).

## Out of scope

- `GOOSIE-JOURNEYFLAKE-001` remainder (two scroll journeys +
  `TestLateTileNeverBlocksPresent` under `-race` load): the closing question is a
  per-suite load-contract design decision, explicitly deferred from the
  bounded-slice queue (`roadmap-v2.md`). Not a P2 NOT RUN row.
- `GOOSIE-LISTBOX-002`, `GOOSIE-OPTIONAX-005`: need a display host.
- Native macOS CUA verification of any row (no display this session).
- Autofill/password sync, WebGL/WebRTC (chain-wide exclusions).
- New internal pages or manager features beyond what BR-052/142/143 require.

## Verification commands

```bash
go build ./...
gofmt -l internal/toolbar internal/paint internal/engine cmd/goosie
go vet -all ./...
go test ./internal/toolbar/ ./internal/paint/ ./internal/engine/ ./test/engine/ ./test/paint/ ./cmd/goosie/ -count=1
go test -race ./internal/toolbar/ ./internal/engine/ ./cmd/goosie/ -count=1
go test ./internal/archtest/
go test ./test/gate -run TestGate -v
go test ./... -count=1
make parity   # expect 105/105, average ~96.39%, known gaps unchanged
```

## Acceptance / sign-off

- BR-052: main menu opens `goosie://bookmarks` and `goosie://history` in the
  active tab via a headless journey; catalog row → PASS (INT), CUA pending.
- BR-143: geometry invariant green across the width sweep; headless resize
  journey keeps controls reachable + non-overlapping; catalog row → PASS (INT),
  CUA pending.
- BR-142: inspector node selection highlights the target tab, DOM byte-identical;
  catalog row → PASS (INT mechanics added), CUA pending.
- `roadmap-v2.md` P2 status updated: no remaining P2 NOT RUN rows; the P2 gate's
  automated half satisfied, native half recorded display-blocked.
- All verification commands green; parity unchanged; commits on `main`, no push.
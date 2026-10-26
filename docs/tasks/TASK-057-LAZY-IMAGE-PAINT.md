# TASK-057 — a script-assigned image on a connected `<img>` must reach paint (GOOSIE-LAZYIMG-001)

## 0. Rules

- One worktree, on this repo, from `main`. Do not push. Do not touch `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`, `testdata/parity.py`, `testdata/live-parity.py`, `docs/qa-live-parity/*`. The integrator owns those.
- `go vet -all ./...`, `go test ./internal/archtest/`, gofmt clean. No new dependencies. No package-level mutable globals in `internal/`. `internal/js` may not import `internal/image` or `internal/engine`; that is why the pixel value crosses the boundary opaquely (see §3.1).
- Report every command's rc and the numbers it printed. A claim without a measurement is not a result.

## 1. What the integrator measured on merged `main` (`6668ad8` + records `80a2f61`, `1effdce`, `0598377`), 2026-10-26

All probes were throwaway and deleted; the temporary `DBG` instrumentation in `internal/js/element_interfaces.go` and `internal/engine/script_image.go` was restored byte-identical.

1. During-parse assignment works: `<img id=i>` plus a synchronous `i.src = <data: 7x9>` in the page script paints 6,300 blue pixels (70x90 CSS box). Scripts run before `loadImages`, so the parse-time collector fetches/decodes the current attribute and `applyImages` fills `Session.images` (`internal/engine/engine.go:1196`).
2. After-load assignment does not: `setTimeout(function(){ i.src = <same data:>; }, 0)` followed by `PumpJS` leaves `onload`/`decode()` reporting `LOADED 7x9` (so the js element state is correct) while the laid-out object's `Image` is **nil** and the box stays empty. `decodeScriptImage` decodes and reports, but deliberately keeps no pixels (`internal/engine/script_image.go:83-86`: "The decoded pixels are not retained").
3. The attachment point is `Reflow` (`internal/engine/engine.go:1352-1357`): `Object.Image` is assigned from `Session.images` every time the arena is rebuilt. Nothing fills that map after load, so no reflow can ever paint the image.

## 2. Cause, one sentence with its proof

`LoadScriptImage` reports width/height to `internal/js` and throws the decoded image away, and no pump turn after the load reflows, so a script-assigned source on an in-document `<img>` can never become an `Object.Image`.

## 3. The fix

### 3.1 Carry the pixels across the js boundary, opt-in and opaque

`internal/js` decides whether a result is current (`st.gen`, `internal/js/element_interfaces.go:1560-1568`) and is the only place that knows which node asked. Extend that seam rather than guessing nodes by URL in the engine:

- `LoadedImage` gains an opaque `Pixels any` field, set by the engine loader on success.
- `js.Options` gains an image-pixel sink, installed through a setter like `SetImageLoader` (`internal/js/js.go:88-160`, `element_interfaces.go:44-50`). Signature: `func(node *dom.Node, rawSrc string, pixels any)`, called from `applyImageResult`'s success branch, after the state update and **before** the `load` event is dispatched. Never called on the error branch. `internal/js` must not interpret `pixels`; `any` keeps it free of `internal/image`/`internal/engine`.
- The engine installs the sink next to `SetImageLoader` (`internal/engine/engine.go:494`), before any script runs.

`rawSrc` is `st.src` (the attribute value this result corresponds to), not a re-resolved URL: reconciliation compares attribute text, and re-resolving in the engine would use `s.imgBase` while js resolved against `js.Options.URL` — two bases that can differ.

### 3.2 Drain on the pump's own goroutine, and force the layout the sink cannot

The sink runs inside `PumpJS` → `jsRT.Tick` → microtask drain, i.e. on the JS goroutine. It must not reflow there. It records `{nodeID, rawSrc, image, w, h}` in an `imgMu`-protected pending table and sets a **layout-pending** bit. `PumpJS` then, after `Tick` and `TickRAF` (so a later RAF `src` write wins):

1. Drains pending entries: attach only if `node.IsConnected()` (`internal/dom/node.go:368`) and the node is an `<img>` and `node.GetAttribute("src") == rawSrc`. Attach = `s.images[id] = img` and `s.natural[id] = {w,h}` under the locks. Anything else is dropped (a detached `new Image()`, a stale result whose src changed again).
2. Reconciles already-retained entries: drop `s.images[id]` / `s.natural[id]` when the node is disconnected or its `src` attribute no longer equals the retained `rawSrc`. This is what makes a later failing or empty assignment stop painting the old pixels; the js element state cannot tell the engine about those (empty/unparseable sources never reach the loader).
3. If anything attached or dropped, reflow. **`mutated = true` alone is not enough**: `RestyleAndReflow` may early-return after `Arena.RefreshComputedStyles` when layout styles are unchanged (`internal/engine/engine.go:2335`), which neither rebuilds the arena nor re-runs layout with the new `s.natural`. The image change must reach a real `Reflow` exactly once in the turn — either call `Reflow` when the restyle path did not, or give the restyle path a force-layout signal. Do not rebuild layout twice in one turn; `WorkCounts().LayoutBuilds` is the probe.
4. Keep the layout-pending bit set across turns when the existing `deferredActive > 0` skip postpones the reflow (`internal/engine/engine.go:2290-2292`), so the next tick still forces it.

A turn with a pending attach returns `true` (repaint). Keep the `!mutated && !imagesChanged` early return for quiescent ticks.

### 3.3 Lifetime and release rules

- Retained script pixels live for as long as the entry passes §3.2 reconciliation; a replacement assignment overwrites them, a changed/failed/empty `src` or a disconnect drops them, and session close releases the session. Reinserting a node that was dropped-and-reconnected while still holding a valid src does **not** re-attach old pixels; the next `src` assignment loads again. State this in a comment.
- Do **not** refund the decoded-pixel reservation on a drop. The existing rule is deliberate (`script_image.go:84-86`): the charge bounds cumulative decode work per document, and a refund would let a `src`-cycling page decode unbounded bytes. Pixels that are dropped become garbage; their charge stays.
- A detached `new Image()` still decodes, still reports `naturalWidth/Height`, and is still charged, but is never retained for paint.

### 3.4 Out of scope

- Parse-time `<img>` pixels replaced by a later script `src` change are not dropped by this task's reconciliation, which only owns script-retained entries. If you measure that stale-pixel case, record it as evidence for a separate defect; do not widen this change.
- `srcset` / `<picture>` assigned by script after load, CSS background images assigned by script, `<input type=image>`, and canvas `drawImage`.
- Synchronous layout inside an `onload` handler (a browser reflows on demand there; this engine reflows at the next pump).
- TASK-056's paced-run pump. If it has not landed, an end-to-end `-screenshot` check cannot see the sink run; the engine-level oracles below do not depend on it.

## 4. Oracles (BR-250..BR-252 are reserved for this task)

Fixtures use `engine.WithJS(rt)`, `engine.WithImages("https://example.test/page", fetch)` with a blocking test fetcher, and a 7x9 PNG as in `test/engine/script_image_test.go`. Synchronise on channels and on script-set flags (`window.done = ...`), not sleeps. Wait for the flag with a bounded pump loop and a test deadline.

1. **BR-250 — an after-load assignment paints.** Document: `<img id="lazy">` (no src) plus `setTimeout(function(){ var i=document.getElementById('lazy'); i.onload=function(){window.done='yes'}; i.src='https://example.test/lazy.png'; }, 0)`. The fetcher must signal that it was entered and then block until the test releases the bytes, proving the decode completes after `NewSession` returned. In the pump turn that runs the load microtask: `window.done === 'yes'`, the arena object for the img node has a non-nil `Image` whose pixel is the fixture colour, and an empty intrinsic box became 7x9. Control: the same assignment written synchronously during parse (the §1.1 case) still paints; do not regress it.
2. **BR-251 — the intrinsic size reaches layout in the same turn, once.** After BR-250 attaches, a second timer assigns a 10x4 image. The first pump turn that reports its `onload` must show a 10x4 box, and `WorkCounts().LayoutBuilds` must increase by exactly one for that turn (no double layout, no second tick needed). Assert the delta, not an absolute count.
3. **BR-252 — stale pixels stop painting.** After a successful attach, a third timer assigns a URL the fetcher rejects. The first pump turn that reports `onerror` must show `Object.Image == nil` and the intrinsic box cleared, with no image command for that node. Sub-case: `src = ''` (never reaches the loader) must clear the same way on the pump after the assignment. Detached retention is memory-only and has no pixel oracle: cover it with an internal `internal/engine` test (package `engine`) asserting the retained/pending tables stay empty for a `new Image()` that never enters the document, and say so in the test comment.

## 5. Mutations

- M01: make the sink a no-op (today's behaviour) → BR-250 and BR-251 red, restore green.
- M02: retain and attach but do not mark/force the layout (drop step 3 of §3.2) → BR-250 red on the turn its `onload` fires, restore green.
- M03: skip the connected/src-identity checks when draining → BR-252 red (stale pixels still painted) and the detached internal test red, restore green.
- M04 (probe, not a required red): refund the reservation on a drop → report what the budget test does; expected to be rejected. Report the outcome either way.

## 6. Acceptance

- BR-250..252 pass, each with a message naming what was measured; the internal detached-retention test passes.
- `go test ./...` rc 0; `go test -race ./internal/engine ./internal/js ./test/engine` rc 0; `go vet -all ./...` rc 0; `go test ./internal/archtest/` rc 0; gofmt clean.
- Re-measure the two §1 probes on the branch: during-parse control still paints 6,300 blue pixels; after-load assignment now paints the same. If TASK-056 has landed by then, repeat through the real `-screenshot` path and report the command; otherwise state that it was not measurable and why.
- Worktree branch, commit(s) naming the defect, and a results section appended to this file: exact commands, rc, counts, mutation outcomes, and anything not measured. Do not register catalog rows; the integrator does that after re-measuring on `main`.

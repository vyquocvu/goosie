# TASK-056 — paced runs must run the page's event loop, and script-image fetches must outlive the load call (GOOSIE-PUMP-001)

## 0. Rules

- One worktree, on this repo, from `main`. Do not push. Do not touch `docs/qa-test-catalog.md`, `docs/qa-open-defects.json`, `docs/qa-ratchet.json`, `test/qc/catalog.go`, `test/qc/traceability.golden.json`, `testdata/render-known-gaps.json`, `testdata/parity.py`, `testdata/live-parity.py`, `docs/qa-live-parity/*`. The integrator owns those.
- `go vet -all ./...`, `go test ./internal/archtest/`, gofmt clean. No new dependencies. No package-level mutable globals in `internal/`.
- Report every command's rc and the numbers it printed. A claim without a measurement is not a result.

## 1. What the integrator measured on merged `main` (`6668ad8` + records `80a2f61`, `1effdce`, `0598377`), 2026-10-26

All probes were throwaway and deleted; the temporary `DBG` instrumentation in `internal/js/element_interfaces.go` and `internal/engine/script_image.go` was restored byte-identical.

1. A page whose script does `setTimeout(function(){document.body.style.background='#...'},0)` keeps its pre-timer colour after `goosie -backend headless -gate -frames 200` (3.49 s, 57 fps). The JS event loop is never pumped: `cmd/goosie/main.go:665-666` installs the tick only when `f.window` is a `*chromeWindow`, and a paced run builds no toolbar/tab manager, so nothing calls `Session.PumpJS` (`cmd/goosie/presentation.go:39-56`, reached only from `pumpChrome`, `cmd/goosie/media.go:52-55`).
2. A page doing `var i=new Image(); i.onload=...; i.src='http://127.0.0.1:PORT/x.png'` calls the loader (`DBG` line), enters `fetchImageBytes`, and the client returns `context canceled`; the local server never logs `GET /x.png` in `-screenshot` or in the 200-frame gate run. Cause: `cmd/goosie/document_loader.go:303-309` builds `imgCtx` with `context.WithTimeout` and `defer imgCancel()`, which fires as soon as `loadURLCtx` returns. Parsed `<img>` fetches complete inside that call (a control page with `<img src=/x.png width=70 height=90>` fetched and painted 6,300 blue pixels), so the cancel only kills work that outlives the call.
3. Control, same page, loop pumped: an engine-session probe logged `LOADED 7x9`, and a `loadURLCtx` probe with a `PumpJS` loop fetched `/x.png` and reported `LOADED 7x9`. The machinery is correct; the paced run's wiring is not.

## 2. Cause, one sentence with its proof

Paced headless runs never pump the page's event loop and their image fetch context is canceled when the load function returns, so no timer, RAF callback or asynchronous image completion that a page schedules during load can run or even reach the network before the screenshot is written.

## 3. The fix

### 3.1 Pump one event-loop turn per vsync, before the frame can settle

The screenshot driver is the right seam, not a free-running ticker: `steadyDriver.pump` (`cmd/goosie/drive.go`) already consumes one `surface.EvVsync` at a time and decides `ready` per vsync. Add a hook (e.g. `watchVsync(func())`, stored under the same mutex discipline as `watchIdle`) and call it for every vsync **before** `ready` is consulted, so a change the pump causes is visible to the same frame or at least resets the idle/settle decision that would otherwise end the run.

- Wire the hook to `f.pumpJS()` (not `pumpChrome`; media is out of scope and `pumpMedia` has its own repaint path) for screenshot mode.
- Do the same for the gate driver (`newDriver`, same file) if it shares the shape; a gate run that measures frames of a page whose timers never fire is measuring a page no browser shows.
- Do not wire it on the interactive darwin path: `chromeWindow.setTick` already pumps there. Guard on the same `*chromeWindow` fact the current code uses, or pass the hook explicitly at the paced construction sites.
- The hook runs on the driver's pump goroutine. That is the same single-thread pattern the interactive tick uses (the JS runtime's contract is "only the Run/Tick goroutine drains"), so it must be one goroutine, never several. `repaintTab` is already called from that kind of goroutine by the interactive pump.

### 3.2 Keep the script-image fetch context alive

In `loadURLCtx`, the image contexts must survive the call for the case that needs them after it. Simplest shape that keeps the existing bound: stop deferring `imgCancel` when the session can still create script images (i.e. in every path where JS ran), and let the existing `engine.ImageFetchBudget` (6 s) deadline and the parent navigation context be the lifetime. State the choice in a comment at the declaration. Do not widen the budget.

Check the other two `defer ...Cancel()` families for the same shape: CSS and scripts are requested during the call only; fonts have a deferred path of their own. Only images are shown to need the change; do not change the rest.

### 3.3 Out of scope

- Retaining script-loaded pixels for paint (that is TASK-057, a separate defect GOOSIE-LAZYIMG-001). Do not touch `Session.images`.
- Making the paced run wait for arbitrary timers or network beyond the existing budgets; only the per-vsync pump and the context lifetime.
- The live sweep harness itself.

## 4. Oracles (BR-253..BR-255 are reserved for this task)

1. **BR-253 — a zero-delay timer runs before a paced screenshot settles.** End-to-end through the real screenshot path (process or in-process equivalent that uses `main`'s wiring): a page sets its background from `setTimeout(...,0)`; the written PNG must have the post-timer colour, and a control page that never sets one keeps the initial colour. If you cannot drive the real PNG path from a test, assert on the composed layer/plan contents at the frame the driver settles, and say exactly what you asserted.
2. **BR-254 — a script-assigned image source reaches the network in a paced run.** Local server, page `var i=new Image(); i.onload=function(){document.body.style.background='#00ff00'}; i.src='/x.png'`. The server must record `GET /x.png` **after** the load function returned (not inside it), and the pump must dispatch `onload` within the paced run. A version that passes only because the fetch finished inside `loadURLCtx` is the bug, not the fix — make the server hold the response briefly (e.g. 50 ms) so the old code fails.
3. **BR-255 — a timer that mutates the DOM in a paced run is repainted.** `setTimeout` inserts a coloured block or changes a colour; the settled frame must show it (assert pixels or the plan's commands, whichever is honest for the harness level).

Keep the timing out of assertions: synchronise with channels/conditions, not sleeps.

## 5. Mutations

- M01: remove the pump hook call (restore today's behaviour) → BR-253 and BR-255 red, restore green.
- M02: restore `defer imgCancel()` → BR-254 red, restore green.
- M03: call `pumpJS` from a second goroutine in addition to the hook (double pump) → must trip the race detector if it does make the runtime races; report what happened either way. This is a probe of the safety claim, not a required red test.

## 6. Acceptance

- BR-253..255 pass, each with a message that names what was measured.
- `go test ./...` rc 0; `go test -race ./cmd/goosie ./internal/engine ./internal/js` rc 0; `go vet -all ./...` rc 0; `go test ./internal/archtest/` rc 0; gofmt clean.
- A re-measurement of the two probe pages from §1 on the built binary: the timer colour appears, and the local server logs `GET /x.png` in `-screenshot` and in `-gate -frames 200`.
- Worktree branch, commit(s) with messages naming the defect, and a results section appended to this file: exact commands, rc, counts, mutation outcomes, and anything not measured. Do not register catalog rows; the integrator does that after re-measuring on `main`.

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

## 7. Results

Worktree: `/Users/vyquocvu/Develop/Browser/goosie/.qoder/worktrees/agent-general-purpose-91ba62a7`, branch `worktree-agent-general-purpose-91ba62a7`, based on `main` at `ac7539a`. Go 1.26.10 darwin/arm64. Nothing pushed. Mutations M01-M03 were applied one at a time and restored with `git checkout -- <file>`; the tree committed below is the restored one.

### 7.1 Commits

| commit | what |
| --- | --- |
| `fa198e4` | `fix(paced): pump the page's event loop once per vsync (GOOSIE-PUMP-001)` - the whole fix, the driver unit test, and the oracles |
| `1a3c806` | `style(paced): gofmt drive_test.go (GOOSIE-PUMP-001)` - trailing blank line |

### 7.2 What changed

- `cmd/goosie/drive.go`: `driver.watchVsync(func() bool)` / `turn()` and `steadyDriver.watchVsync` / `turn()`. Both pumps call the hook for **every** `surface.EvVsync`, before the gate's sent counter and before `steadyDriver.ready`. The hook runs on the pump goroutine (the loop is built from the driver, so it is attached after construction under the same mutex discipline as `watchIdle`). The gate driver skips the hook once `done` is closed, because shutdown may already be closing the session while the headless window still delivers ticks; `finish` is only ever called on the pump goroutine, so that check orders every pump before the close. `steadyDriver.pump` resets `settled` when the hook reports it published work: the idle count is read from the loop's last frame, so without the reset the run could settle on the vsync whose pump had just changed the page.
- `cmd/goosie/presentation.go`: `pumpJS` now reports whether it published a repaint and, when `tabMgr == nil` (a paced run), dispatches to `pumpPaced` / `repaintPaced`. `repaintPaced` mirrors `repaintTab`: reuse the layer through `Scheduler.UpdateLayer` (empty damage derives `paint.ContentDamage`), new layer + plan on geometry change. It additionally republishes the plan when `Session.BackgroundColor()` changed, because the canvas colour lives only in `FramePlan.Background` (the display list has no canvas command); the scheduler treats that as a full redraw.
- `cmd/goosie/main.go`: the paced branch keeps the session (`pacedSession`) and the colour it published (`pacedBG`) instead of discarding them; both paced construction sites wire `watchVsync(f.pumpJS)`. The interactive path is untouched: `chromeWindow.setTick -> pumpChrome` is still the only pump there.
- `cmd/goosie/document_loader.go`: the image family no longer defers `imgCancel`; the comment at the declaration states the lifetime (engine `ImageFetchBudget` 6 s deadline + parent navigation context). `_ = imgCancel` keeps `go vet`'s lostcancel quiet. CSS, script and font families unchanged.
- `cmd/goosie/media.go`: `closeAllSessions` closes `pacedSession` too (not nil-ed, so a lingering pump read is safe; `Session.Close` is idempotent).
- `cmd/goosie/drive_test.go`: `TestSteadyVsyncHookRunsBeforeReadyAndRestartsSettling` - hook called once per vsync, and work it publishes restarts the settle count.
- `test/entrypoints/paced_event_loop_test.go`: BR-253, BR-254, BR-255 through the real `-screenshot` binary, plus `TestGatePacedRunPumpsThePageEventLoop` (a 30 ms timer whose `fetch('/ping')` can only run once the pace loop is up). The BR-254 server holds `/x.png` for 50 ms, records the request, and records `r.Context().Err()` after the hold; bookkeeping is channels/`sync.Once`, no polling sleeps in the tests.

### 7.3 Acceptance commands

| command | rc | numbers |
| --- | --- | --- |
| `go test ./...` | 0 | 43 packages `ok`, 0 `FAIL`, 27 `no test files`; `test/entrypoints` 63.6 s, `test/gate` 35.8 s, `test/gatecheck` 28.5 s |
| `go test -race ./cmd/goosie ./internal/engine ./internal/js` | 0 | `cmd/goosie` 23.192 s, `internal/engine` 2.827 s, `internal/js` 2.329 s |
| `go vet -all ./...` | 0 | 0 lines of output |
| `go test ./internal/archtest/ -count=1` | 0 | 2.912 s |
| `gofmt -l .` | 0 | 0 files listed |
| `go test ./test/entrypoints -run 'TestPaced\|TestGatePacedRunPumpsThePageEventLoop' -count=1` | 0 | 4 tests, 16.0 s |

### 7.4 Oracles

All four pass on the committed tree. Each failure message names the pixel and both colours, or the request the server did/did not see.

- BR-253 `TestPacedScreenshotRunsAZeroDelayTimer`: `setTimeout(...,0)` sets body background `#ff0000`; pixel (100,75) of the written PNG is `#ff0000`. Control page (same markup, no script): `#0000ff`. The sample point is viewport centre, which no box covers, so it also measures the plan-background republish.
- BR-254 `TestPacedScreenshotScriptImageLoadOutlivesTheLoadCall`: server answered `/x.png` once after a 50 ms hold, request context after the hold `nil`; pixel (100,75) is `#00ff00` (onload). The page paints `#ff0000` from `onerror`, and the test names that colour if the callback was the error path instead.
- BR-255 `TestPacedScreenshotRepaintsATimerMutation`: timer appends a 120x80 `#00c000` block at (0,0); pixel (60,40) is `#00c000`; control is `#ffffff`.
- `TestGatePacedRunPumpsThePageEventLoop` (supporting, no BR number): 20-frame gate run; the 30 ms timer's `fetch('/ping')` must reach the server, which it does.

### 7.5 Mutations

Commands are the oracle command above; each mutation was restored before the next.

- **M01 - remove the hook call from both drivers.** BR-253 red `pixel (100,75) = #0000ff, want #ff0000`; BR-254 red `pixel (100,75) = #0000ff, want #00ff00` (note: with the imgCtx fix still in place the server *did* see `GET /x.png` and the context survived the hold, so this red is purely the missing onload dispatch); BR-255 red `pixel (60,40) = #ffffff, want #00c000`; gate probe red (`no GET /ping`). 4 tests failed, 16.2 s. Restore: package `ok` 16.1 s.
- **M02 - restore `defer imgCancel()`.** BR-253 pass, BR-254 red `the image request was canceled while the server held it for 50ms (context canceled): the script-image fetch did not outlive the load call`, BR-255 pass, gate probe pass. Restore: BR-254 `ok` 5.9 s.
- **M03 - double pump (`go f.pumpJS()` beside the hook in both drivers).** Built `go build -race -o /tmp/task056/goosie-race ./cmd/goosie` and ran `-screenshot` of the timer page: rc **66**, **4** `WARNING: DATA RACE` reports, first one reading `Invalidation.Empty` from `Session.PumpJS` on the second goroutine's `pumpPaced` (`presentation.go:75`). The claim that the hook must be one goroutine holds. Restored; the fixed tree rebuilt with `-race` ran the timer and image pages rc 0 with **0** race reports.

### 7.6 Re-measurement of the §1 probes (built binary, not the tests)

`go build -o /tmp/task056/goosie ./cmd/goosie`; a local server logged every request and held `/x.png` 50 ms.

| run | rc | observation |
| --- | --- | --- |
| `goosie -backend headless -width 200 -height 150 -dpr 1 -screenshot -out acc-timer.png -url .../timer` | 0 | pixel (100,75) `#ff0000` (the timer colour appears) |
| `goosie ... -screenshot -out acc-img.png -url .../img` | 0 | server logged `GET /x.png`, `request-context-after-hold=<nil>`, pixel (100,75) `#00ff00` |
| `goosie ... -gate -frames 200 -out acc-img-gate.json -url .../img` | 0 | report `frames: 200`; server logged `GET /img` then `GET /x.png`, `request-context-after-hold=<nil>` |
| `goosie ... -gate -frames 200 -out acc-tick-gate.json -url .../tick` | 0 | report `frames: 200`; server logged `GET /tick` then `GET /ping` (the 30 ms timer fired under the gate pump) |

The baseline is §1's measurement on pre-fix `main`, not re-run here: no `GET /x.png` in either mode and the pre-timer colour. Note that under M01 (hook removed, context fix retained) the server *did* see `GET /x.png`, so the request line alone is not the discriminator; the onload pixel and the gate's `/ping` are.

### 7.7 Not measured / notes

- The native interactive path (`chromeWindow.setTick`) was not re-measured; it is deliberately not wired to the new hook and its code was not changed.
- `-bench` shares the gate driver and hook; no separate rate measurement was taken.
- BR-254's "after the load function returned" is measured indirectly by the server: the response is held 50 ms, which outlives `loadURLCtx` on this machine, and the handler's `r.Context().Err()` after the hold is `nil` only if the fetch context survived the call. `loadURLCtx` itself is not instrumented. On a machine where the load call took longer than the hold, M02 would be indistinguishable at the server; the pixel assertion would still be red under M02 only if the cancel beat the completion.
- The BR-253/BR-255 control runs are part of the same tests; no live-site sweep, parity harness or `-print-pdf` path was exercised here, and none of the integrator-owned files listed in §0 were touched.
- One addition beyond the literal §3.1 text: `pumpJS` reports whether it published work and the screenshot driver restarts its settle count on a repaint, which is §3.1's "at least resets the idle/settle decision". `repaintPaced` also republishes the plan on a canvas-colour change; without that the body-background oracles cannot see the change at the viewport centre, because the canvas colour is not in the display list.
- No measurement contradicted the spec; no threshold or budget was adjusted.


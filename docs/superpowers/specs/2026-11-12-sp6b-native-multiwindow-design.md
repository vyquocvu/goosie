# SP-6b — native darwin multi-window shim + Cmd+N

**Date:** 2026-11-12
**Status:** approved (autonomous delegation, approval gates waived)
**Builds on:** SP-6a (Go-side owned `windowManager`, shared profile, N loops, quit-on-last, headless+race verified; commits `e44fd150`…`c117f852`). Second and final sub-project of SP-6 (P3 multi-window).

## 1. Why this scope

SP-6a delivered the Go foundation: `cmd/goosie/window_manager.go` owns N `framePath`s over one shared profile, runs N loops, and quits when the last window closes — proven on headless N=1/N=2 under race. But SP-6a deliberately left the **native macOS path single-window** and added **no user-facing way to open window #2**. Two gaps remain, and they are one vertical slice:

1. **The AppKit half assumes one window.** Every real single-window assumption is in `internal/platform/darwin/shim_darwin.m` and its Go bridge `window.go`. A second native window cannot be created today: `darwin.Open` refuses off-main, the app delegate is replaced per create, `GoosieAppRun` exits on one window's quit, and `GoosieClose` never orders the window out.
2. **No Cmd+N / New Window.** `handleTabShortcut` claims Cmd+Shift+N (incognito) but plain Cmd+N falls through unhandled; the painted main menu has no New Window item; `framePath` has no link to the manager that could spawn a window.

SP-6b closes both: adapt the native shim so N windows function (compile-verified — no display here to create an `NSWindow`), and wire Cmd+N + File→New Window through the SP-6a manager (headless TDD). After SP-6b, multi-window is a complete, user-reachable feature on both backends.

### 1.1 Terrain (re-derived and persisted 2026-11-12)

The SP-6a spec cited terrain items S1–S15 from an in-session Explore map that was **never written to disk**. This section re-derives the load-bearing findings from the current source so they are durable. Line numbers are verified against HEAD `c117f852`.

| # | Finding | Exact location | Consequence for N>1 |
|---|---------|----------------|---------------------|
| S1/S3 | App delegate is **per-window**, replaced on every create, bound to one `gw` | `shim_darwin.m:1073-1074` (`gw->appDel = [[GoosieAppDelegate alloc] initWithWindow:gw]; [app setDelegate:gw->appDel]`), `:918-948` | Last-created window's delegate wins; `applicationWillTerminate` pushQuits only that one `gw`. Need **one shared delegate + a window registry**. |
| S2 | `applicationShouldTerminateAfterLastWindowClosed:YES` | `shim_darwin.m:927` | With the hand-turned loop (not `[NSApp run]`), last-window-close must be handled by the registry, not AppKit's terminate machinery. |
| S4 | `GoosieAppRun(gw)` exits on **one** `gw->quit` | `shim_darwin.m:1143-1171` (loop checks `gw->quit` at `:1155-1158`) | Closing window #1 ends the app loop while window #2 lives → window #2 stops drawing. Must turn until **app-wide** quit. |
| S12 | `GoosieClose(gw)` never orders the window out | `shim_darwin.m:1470-1474` (`stopDisplayLink` + `pushQuit` only) | A closed window's `NSWindow` stays on screen (`setReleasedWhenClosed:NO`). Must `[gw->window close]` on the main queue. |
| S13 | `gw->window` is **never assigned** in create | `shim_darwin.m:1082-1108` builds `win` but only stores `gw->view`; `gw->window` stays NULL | **Latent bug today:** `GoosieChooseSavePath:1541` and `GoosieChooseFiles:1579` bail with `-1` on `!gw->window`, so native file/save panels never open. Also blocks S12 (needs the handle). Fix: retain `gw->window = win`. |
| S15 | `darwin.Open` is main-thread-only, **no trampoline** | `window.go:80-83` (`if C.GoosieIsMainThread()==0 { return nil, ErrNotMainThread }`), C gate `shim_darwin.m:1046` | Cmd+N runs on the **chrome pump goroutine** (off-main); `m.spawn → openWindow → darwin.Open` returns `ErrNotMainThread`. Need a **main-queue create trampoline** while `Run` turns. |
| S11 | Per-window pool/composer/3-slot ring scale linearly | `shim_darwin.m:55` (`kGoosieSurfaceSlots=3`), per-`gw` surf ring | Memory grows with window count; bounded only by user action. Shared pool / cap deferred (§9). |

**Supporting Go-side facts (verified):**

- The main thread is pinned once in `cmd/goosie/main.go:828` (`runtime.LockOSThread()` in `main()`), never released. `run() → m.run() → r.Run()` all execute on that goroutine; every other goroutine (chrome pump, loop, teardown, navigation) is off-main.
- `darwin.Window.Run()` (`window.go:361-366`) is `C.GoosieAppRun(w.gw)`; `chromeWindow.Run()` (`toolbar_window.go:601-605`) forwards to it. The manager handoff is `window_manager.go:206-208` (`if r, ok := primary.window.(platform.Runner); ok { r.Run() }`), **primary-only**.
- `darwin.Window.Close()` (`window.go:368-382`) is `closing.Do{ close(detach); C.GoosieClose(gw) }` then `<-pumpDone`; documented thread-safe, called off-main from the teardown goroutine (`window_manager.go:120`).
- The shim's hand-turned loop **drains the main dispatch queue**: `commitSurface` (`shim_darwin.m:990`) reaches the layer via `dispatch_async(dispatch_get_main_queue(), …)` in production, and `GoosieClipboardRead`/`GoosieChooseSavePath`/`GoosieChooseFiles` already use `dispatch_sync(dispatch_get_main_queue(), block)` from off-main (`:1488`, `:1566`, `:1679`). This is the **precedent that makes the S15 trampoline safe**.
- `platform.Select` (`platform.go:182-197`) with `Backend==""` **silently falls back to headless** on a native open failure (`:189-194`); only `Backend=="native"` surfaces `ErrNotMainThread` as a hard error. A mixed-backend app is the trap to avoid.
- `handleTabShortcut` (`toolbar_window.go:1203`) runs on the **chrome pump goroutine** (`intercept` at `:923-925`, before the loop sees the event). Cmd+Shift+N is `:1228-1232`; plain Cmd+N matches no case and returns false at `:1357`.
- The painted main menu is `openMainMenu` (`toolbar_window.go:194-212`); actions dispatch in `handleContextMenuAction` (`interaction.go:836`, main-menu cases `:897-916`). **No NSMenu / File menu exists** — the only native menu is the shim's app menu + Quit (`shim_darwin.m:929-942`).
- `framePath` (`main.go:121-227`) has **no manager pointer** and no `onNewWindow` hook; `newTab`/`newIncognitoTab` are tab-level, never touch the platform. `m.spawn` (`window_manager.go:59`) builds a whole new `framePath`.
- archtest `MutableGlobals` walks **`.go` files only** (`internal/archtest/rules.go:467`); C statics in `.m` are not scanned. cgo must stay confined to `internal/platform/darwin` (`rules.go:335-359`). The design below introduces **no mutable Go global and no C static** (the registry lives in the shared ObjC delegate's heap ivars).

## 2. Architecture

Two layers, one vertical slice. **N=1 native behavior is preserved exactly** — the shared delegate with one registered window quits when that window closes, which is today's behavior.

### 2.1 Layer A — native C shim (compile-verified)

All changes in `internal/platform/darwin/shim_darwin.m` (+ `shim.h` for the one new C symbol) and the Go bridge `window.go`. The registry and app-quit flag are **main-thread-confined** ObjC ivars on the one shared delegate — no atomics, no mutex, no C static, because every mutation (create, windowWillClose, terminate) and every read (GoosieAppRun) happens on the main thread.

- **One shared `GoosieAppDelegate`.** Created on the first `GoosieWindowCreate` (when `[NSApp delegate]` is nil), reused thereafter; every `gw->appDel` points to it. It owns:
  - a registry of live `GoosieWindow *` (a small main-thread-only C array ivar, grown on register),
  - `int _appQuit` (main-thread-only).
  - `registerWindow:` appends; `unregisterWindow:` removes by pointer (idempotent) and sets `_appQuit=1` when the registry empties; `quitAll` pushQuits every registered window and sets `_appQuit=1`; `appQuit` reads the flag.
- **`GoosieWindowCreate`** retains `gw->window = win` (S13), reuses/creates the shared delegate, and `registerWindow:gw`. If the shared delegate already reports `appQuit`, it returns a new error code `6` (app quitting) rather than building a window into a dying app.
- **`GoosieWindowDelegate.windowWillClose:`** → `pushQuit(gw)` + `[appDel unregisterWindow:gw]`. `windowShouldClose:` unchanged (pushQuit, return YES). Unregister is idempotent so the close button and `GoosieClose`'s dispatched `[win close]` cannot double-decrement.
- **`GoosieAppDelegate`**: `applicationShouldTerminateAfterLastWindowClosed:` stays YES; `applicationWillTerminate:` → `quitAll` (pushQuit every window, not one). The menu gains nothing native (New Window is the painted Go menu, §2.2) — the app menu keeps Quit.
- **`GoosieAppRun(gw)`** turns until `[gw->appDel appQuit]` instead of `gw->quit`. On exit it still `pushQuit(gw)` (wakes the primary pump). Because every window shares `appDel`, the primary's handle reaches the app-wide flag. N=1: the single window's close empties the registry → appQuit → loop ends, exactly as today.
- **`GoosieClose(gw)`** keeps `stopDisplayLink` + `pushQuit`, and adds `dispatch_async(dispatch_get_main_queue(), ^{ if (gw->window) [gw->window close]; })` (S12). Marshaled because Close is called off-main; `[win close]` fires `windowWillClose:` → unregister → (last window) appQuit. `setReleasedWhenClosed:NO` means close orders out without freeing, honoring the shim's no-`GoosieWindowFree` contract (`shim.h:188-198`).
- **`GoosieWindowCreateOnMain(title,w,h,&err)`** (new C symbol, S15): if `pthread_main_np()` → call `GoosieWindowCreate` directly; else → `dispatch_sync(dispatch_get_main_queue(), ^{ result = GoosieWindowCreate(…); })` and return it. The create block re-checks `appQuit` (via the shared delegate) so a Cmd+N racing app-quit returns code 6 instead of hanging. This is the clipboard/panel `dispatch_sync` precedent applied to create.

### 2.2 Layer B — Go wiring (headless TDD)

- **`darwin.Open`** (`window.go:77-104`) calls `C.GoosieWindowCreateOnMain` instead of `C.GoosieWindowCreate`, and the hard off-main gate (`:80-83`) is removed — the trampoline marshals off-main creates. `createErr` (`window.go:109-125`) gains a mapping for the new code `6` → an "app quitting" error (e.g. `ErrAppQuitting`). `Run()`/`Close()` signatures unchanged; their doc comments updated for app-wide semantics ("Run returns when the **app** quits"; "Close ends **this window**; the app loop ends when the last window closes"). Any existing test that pins the old off-main `ErrNotMainThread` refusal must be updated: off-main `Open` now marshals and succeeds (when a display exists) rather than refusing.
- **`framePath` manager link.** Add `newWindow func()` to `framePath` (a closure, not a back-pointer, to keep `framePath` decoupled from `windowManager`). `m.spawn` installs `f.newWindow = func() { if _, err := m.spawn(""); err != nil { fmt.Fprintf(os.Stderr, "goosie: new window: %v\n", err) } }` after the window is built. A nil `newWindow` (legacy paced/screenshot/private path, which never sets it) means "no manager" and the shortcut/menu no-op.
- **Manager registry-wide error/report.** `run()` currently snapshots `errs` at `:200` (right after the primary spawn), so a later window's loop error is never drained. Move the `errs` snapshot to **after `<-m.done`** (under `m.mu`), so every spawned window's error is collected. `primary.report(nil)` stays the final report (primary owns the session/journal); the change is error collection only.
- **Cmd+N.** Add `onNewWindow func()` to `chromeWindow` (near `onNewIncognitoTab`, `toolbar_window.go:33`). Split the `:1228` case: `(key=='n'||key=='N') && shift` → `onNewIncognitoTab` (unchanged); `(key=='n'||key=='N') && !shift` → `onNewWindow`. Wire `cw.onNewWindow = f.newWindow` in `openWindow` (beside `onNewIncognitoTab` at `main.go:761`).
- **File→New Window.** Add `{Label: "New Window", Action: "new_window"}` to `openMainMenu` (`toolbar_window.go:198-207`, top of the list) and `case "new_window": if f.newWindow != nil { f.newWindow() }` to `handleContextMenuAction` (`interaction.go:897-916`). Painted menu, no NSMenu — fully headless-testable.

## 3. Data flow

**Cmd+N on native (the load-bearing path):**
```
chrome pump goroutine: intercept → handleTabShortcut('n', Cmd) → cw.onNewWindow()
  → f.newWindow() → m.spawn("")
    → buildWindow(c, m.profile)            [pure Go, off-main OK]
    → f.openWindow() → platform.Select → darwin.Open
      → C.GoosieWindowCreateOnMain          [off-main]
        → dispatch_sync(main_queue, create) [main thread is in GoosieAppRun, drains queue]
          → GoosieWindowCreate on main: gw->window=win, shared delegate, registerWindow
        ← returns gw
    → startWindow(f)                        [loop + teardown goroutines]
    → registry append (m.mu), f.newWindow installed
    → navigateTab("")                       [no-op for empty URL]
```
The main thread keeps turning the app-wide `GoosieAppRun` throughout; the chrome pump goroutine blocks only for the duration of the create (one window), then resumes. No deadlock: the main thread is never waiting on the chrome pump goroutine.

**Close one window (others live):** last tab → `onCloseLast → f.window.Close()` (unchanged) → `darwin.Window.Close` → `GoosieClose(gw)` → pushQuit(gw) (pump exits) + dispatch_async `[gw->window close]` → `windowWillClose:` → unregister → registry non-empty → **app loop keeps turning**. The Go teardown goroutine then joins the loop, closes sessions/pool, and `removeWindow(f)`. Other windows keep drawing.

**Quit on last window:** the last `windowWillClose:` empties the registry → `_appQuit=1` → `GoosieAppRun` returns → `r.Run()` returns → manager `<-m.done` (already closed by the last `removeWindow`) → `save()` → drain **all** errs → `primary.report(nil)`.

**Cmd+Q / SIGTERM:** `terminate:`/`applicationWillTerminate:` → `quitAll` pushQuits every window → every pump's `GoosieNextEvent` returns 0 → every events channel closes → every `loop.Run` returns → every teardown runs → `removeWindow` empties the registry → done. (The manager's Option-B signal goroutine from SP-6a still cancels `m.ctx` on SIGTERM, which stops the loops directly; both paths converge on done.)

## 4. Error handling / edge cases

- **Spawn failure off-main:** the trampoline returns a create error (no display=4, no display link=5, app quitting=6, OOM=3). `m.spawn` closes the new pool and returns the error; `f.newWindow` logs it to stderr; existing windows are unaffected. No partial window is appended (SP-6a spawn already guarantees this).
- **Cmd+N racing app-quit:** `GoosieWindowCreateOnMain` checks `appQuit` before dispatching and the create block re-checks it, returning code 6 rather than `dispatch_sync`-ing into a loop that has stopped (which would hang). The chrome pump is also being detached during teardown, so late keys are unlikely; the check is the belt-and-braces.
- **Silent headless fallback:** with `Backend==""`, a failed native open falls back to headless (`platform.go:189-194`). For the interactive manager this means a machine with no display runs N headless windows (correct, SP-6a behavior). Native multi-window requires a display; the trampoline does not change the fallback, it only makes off-main creates succeed when a display exists. Documented, not "fixed" — the fallback is intentional for no-display machines.
- **Double close / idempotency:** `unregisterWindow:` is idempotent (remove-by-pointer; a window already gone is a no-op), so the close button's `windowWillClose:` and `GoosieClose`'s dispatched `[win close]` cannot double-decrement. `darwin.Window.Close`'s `closing.Do` still guards the Go side.
- **Registry threading invariant:** register (create, on main via trampoline), unregister (`windowWillClose:`, main), `quitAll` (terminate, main), and `appQuit` reads (`GoosieAppRun`, main) are **all main-thread**. No lock is needed; the invariant is documented in the shim and is the reason the design avoids C statics and atomics.
- **No native regression (N=1):** one shared delegate with one registered window; its close empties the registry → appQuit → `GoosieAppRun` returns. `GoosieClose` orders out the one window. `gw->window` is now retained, which **fixes** the file panels (a behavior change, but a bug fix: they were dead before). The legacy paced/screenshot/private path (`main.go:902+`) never sets `f.newWindow`, never spawns a second window, and is byte-for-byte unchanged.
- **Memory (S11):** N windows hold N pools/composers/3-slot rings until process exit (the shim's no-free contract). Bounded by user action; shared pool / window cap deferred (§9).

## 5. Testing

**Headless TDD (the acceptance core, red-green + mutation):**
- `TestManagerCmdNSpawnsWindow` — a Cmd+N key delivered to window 1's chrome (`handleTabShortcut('n', ModCommand)`) spawns window 2; registry length 2; window 1's tabs untouched. Mutation: make `onNewWindow` a no-op → red.
- `TestManagerNewWindowMenuAction` — `handleContextMenuAction("new_window")` spawns window 2; the painted main menu lists "New Window". Mutation: drop the menu item / case → red.
- `TestManagerCmdShiftNStillIncognito` — Cmd+Shift+N still opens an incognito **tab** in the same window (no new window), proving the case split did not regress incognito.
- `TestManagerCollectsLaterWindowError` — a window spawned after the primary whose loop returns an error has that error surfaced by `run()` (registry-wide drain). Mutation: revert to the primary-only `errs` snapshot → red.
- `TestManagerNewWindowNilHookNoop` — a `framePath` with `newWindow==nil` (legacy path) no-ops on Cmd+N / "new_window" without panicking.
- SP-6a's six lifecycle tests stay green (spawn, close-one-keep-others, quit-on-last, last-tab-close, tab isolation, profile sharing).

**Compile-verified (native C — no display to create an NSWindow):**
- `go build -o /dev/null ./cmd/goosie/` (cgo, darwin/arm64) and `go vet ./internal/platform/darwin/` clean.
- Existing native tests stay green: `go test ./internal/platform/darwin/` (media/accept/AX/translate — they link the shim, so a compile/type error breaks them) and `go test ./test/engine/ -run Media` (native media TestMain).
- N=1 behavioral equivalence argued in §4; the changed C functions (`GoosieWindowCreate`, `GoosieAppRun`, `GoosieClose`, delegates) are **not** exercised by any automated test here because window creation needs `GoosieHasDisplay()`. This is the honest ceiling, consistent with SP-6a and all prior native CUA work.

**No-regression matrix:** `go test ./...`, `go test -race ./...`, `go vet -all ./...`, `go test ./internal/archtest/` (cgo confined, no new Go global), gate suite, qc — all green. `make parity` unchanged (105/105, ≥96.39%, gaps 4/6): SP-6b touches no engine/layout/paint code.

**Visual:** native multi-window visual sign-off is **deferred** (no display). Headless pixel read-back (SP-6a) remains the visual evidence for the Go lifecycle; the native shim is compile-verified.

## 6. Acceptance

SP-6b ships when: Cmd+N and File→New Window spawn a second window through the manager on headless (TDD green + mutation evidence); Cmd+Shift+N still opens an incognito tab; the manager collects every window's loop error; the native shim compiles + vets clean with one shared delegate, app-wide quit, retained `gw->window`, order-out close, and a main-queue create trampoline; existing native tests and the full matrix + race + archtest + gate + qc are green; parity is byte-identical; N=1 native behavior is preserved (file panels now work). Commits on `main`, **no push**. Roadmap records SP-6 complete (multi-window shipped) and SP-7 as next.

## 7. Deferred to SP-7 / later (YAGNI, with evidence)

- **Per-window titles** (`openWindow` hardcodes `Title:"Goosie"`, `main.go:717`), **cascade/offset placement (S8)**, **per-display display-link (S7)**: polish, each its own increment when a journey exists.
- **Window-aware session grouping** (aggregate all windows' tabs into one session file with grouping): SP-6a/6b keep the primary-only session save; grouping is a data-model change deferred.
- **Shared raster pool / window cap (S11)**, **drag-tab-between-windows**, **per-window sheet-modal dialogs**: independent enhancements.
- **Native multi-window runtime/visual sign-off**: needs a display + CUA harness; deferred consistent with all prior native work.
- **L3/L4 (incremental style/layout invalidation)**: still deferred to SP-7+ per the SP-5 terrain evidence; unchanged by SP-6.
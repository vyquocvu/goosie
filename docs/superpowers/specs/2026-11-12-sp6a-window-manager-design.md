# SP-6a — window manager + multi-window lifecycle (Go foundation)

**Date:** 2026-11-12
**Status:** approved (autonomous delegation, approval gates waived)
**Builds on:** SP-1…SP-5 (cleanup, measurability, P2 journeys, L1/L2 architecture, flow-root + paint identity). First sub-project of SP-6 (P3 product features).

## 1. Why this scope

The SP-6 P3 survey (`docs/roadmap-v2.md:204`, corrected this sprint) found **five of six** P3 features already shipped with tests — context menu, bookmark bar, Reader Mode, PDF export, view-source/inspect. **Multi-window is the only absent P3 item.** AGENTS.md forbids re-planning implemented features, so SP-6 targets multi-window alone.

The terrain map (Explore, 2026-11-12) splits the work cleanly by layer:

- **Go / surface / headless are already N-window clean.** `surface.Loop` holds exactly one `Window` (`internal/surface/loop.go:88-112`); N windows ⇒ N independent loops and event channels, no window-id field needed (`surface.Event` has none, `window.go:130-145`). `platform.Select` is stateless and re-entrant (`platform.go:182`). Headless has zero shared state and **two windows already coexist** in one test (`cmd/goosie/render_work_test.go:699,1124`); a shared `ManualClock` ticks them in lockstep.
- **Every real single-window assumption is native** (`internal/platform/darwin/shim_darwin.m`): the hand-rolled `GoosieAppRun` exits on one window's `quit` (`:1143-1171`), the app delegate is replaced on each create and bound to one `gw` (`:1071-1074`, `:918-948`), `applicationShouldTerminateAfterLastWindowClosed=YES` (`:927`), `Open` is main-thread-only with **no main-queue create trampoline** (`window.go:80-83`, `:1046`), and `GoosieClose` never orders the window out (`:1470-1474`).
- **The app layer is one-`framePath`-shaped:** `run` (`cmd/goosie/main.go:902-1126`) orchestrates a single window's loop goroutine, shutdown goroutine, and the main-thread `r.Run()` hand-off.

**Decomposition (brainstorming scope-check).** Multi-window is one feature with two dependent layers. They are split into two sub-projects to isolate the risky, compile-only-verifiable native surgery behind a fully headless-verifiable Go foundation:

- **SP-6a (this spec):** the Go-side `windowManager`, profile/window state sharing, and multi-window lifecycle — proven on headless with the race detector. The native single-window path is preserved unchanged (no regression, no user-facing shortcut yet).
- **SP-6b (deferred, §9):** the darwin shim adaptation (app-wide quit, shared delegate, main-queue create trampoline, close-orders-out) **plus** the user-facing Cmd+N / File→New Window wiring. The user feature lands complete in SP-6b; SP-6a is its necessary, tested foundation.

This ordering means the lifecycle logic (spawn, close-one-keep-others, quit-on-last) is proven deterministically on headless **before** the native run loop is touched.

## 2. Architecture

### 2.1 Owned manager, not a global

`internal/` forbids mutable package-level vars (`archtest.MutableGlobals`, `rules.go:492`; gate `m1_gate_test.go:317`). The manager is therefore an **owned value** created in `run` and threaded through, never a package-global registry. It lives in `cmd/goosie` (a `FreeDirs` exempt package, `rules.go:76`), so it may import everything.

```
windowManager (owned by run)
  ├─ profile: shared stores, created ONCE, same pointers in every window
  │    client, privateClient, fonts, bookmarks, history, downloads,
  │    storage, incognitoStorage, broadcastHub, incognitoBroadcastHub,
  │    perms, preferences, flags, userscripts, theme
  └─ windows: []*framePath  (each owns its window-level machinery)
       window, loop, sched, pool, composer, layer, rec, toolbar, tabMgr,
       zoom, find state, navResults/imgResults, runCtx/runCancel/navWg, …
```

### 2.2 Profile-shared vs per-window (no field-access refactor)

`framePath` (`main.go:122-228`) keeps its existing fields. The manager does **not** rename `f.bookmarks` → `f.profile.bookmarks` (that would touch every access site). Instead it **creates the profile-level stores once and assigns the same pointers** into each `framePath`; per-window machinery is built fresh per window. This is the minimal-invasion seam.

Sharing is **semantically correct**, not merely convenient: same profile ⇒ one cookie jar / HTTP pool (`client`, `privateClient`), one `localStorage` per origin (`storage`, `incognitoStorage` — web-platform semantics span windows), one `BroadcastHub`, one permissions store (`perms`, per-origin), one history, one bookmark store, one download manager, one glyph atlas (`fonts`), one preferences/flags/theme set.

Per-window (built fresh): `window`, `loop`, `sched`, `pool`, `composer`, `layer`, `rec`, `toolbar`, `tabMgr`, `zoom`, find state, nav channels/`WaitGroup`s, `runCtx`/`runCancel`, `emulationPreset`, `mediaBackend`, `document`/`docHeight`. A per-window `pool` matches today's `build` (one pool per framePath); sharing a pool is a deferred optimization (§9).

`config` is a **value copy per window**: each `framePath` keeps its own copy so the existing `sizeMu`-guarded `config.width/height` remain per-window (each window resizes independently). Run-level fields (`backend`, `dpr`, `vsyncPeriod`, profile/session file paths) are identical across copies, set once by the manager from the parsed run config.

### 2.3 N loops, one app loop on native

Each window runs its own `loop.Run(ctx)` goroutine and its own shutdown goroutine (the existing per-window teardown order: cancel → join loop → `runCancel` → `navWg.Wait` → `closeAllSessions` → `window.Close`). The manager joins all window loops. On **native**, the main thread hands off to the platform `Runner` once (today `GoosieAppRun`); in SP-6a native runs exactly one window, so this hand-off is unchanged. On **headless** there is no `Runner` (`platform.go:66-68`), so the manager simply waits for its N loop goroutines.

## 3. Component A — `windowManager` + lifecycle

**New file `cmd/goosie/window_manager.go`** (owned value):

- `type windowManager struct` holding the shared profile stores, `windows []*framePath`, a `sync.Mutex` guarding the slice, the run-level `config`, and a `quitOnce`/done signal.
- `func (m *windowManager) spawn(initialURL string) (*framePath, error)`: build a fresh per-window `framePath` (reusing `buildWindow`/`openWindow` machinery), wire the shared profile pointers, keep its `tabMgr`'s `onCloseLast → f.window.Close()` unchanged, append under lock, and start its loop + teardown goroutines (`startWindow`). The teardown observes the loop stop and calls `removeWindow`. Returns the window. The first window is the primary (keeps the crash journal); later windows get `sessionActiveFile=""`.
- `func (m *windowManager) closeWindow(f *framePath)`: the explicit close entry (tests, and the native close path in SP-6b). It only calls `f.window.Close()`; the teardown goroutine started by `startWindow` does the join and removal.
- `func (m *windowManager) removeWindow(f *framePath)`: drop `f` from the slice under lock; when the slice becomes empty, signal the manager done (`doneOnce`) so `run` proceeds to final save + exit. Idempotent (a window is removed once).
- `func (m *windowManager) run(ctx) error`: spawn the first window, run N loops, on native hand the main thread to the `Runner`, wait for all loops / manager-done, then perform the existing final save (session/history/flags). Quit-on-last-window is the manager-done signal.

**Refactor `run` (`main.go:902-1126`)** to construct a `windowManager` and delegate the **interactive browser path** to it. The existing single-window orchestration becomes the manager's N=1 case. The paced/screenshot/private driver paths (`c.paced()`, `c.screenshot`, `c.private`) **bypass the manager** and keep their existing single-window orchestration and frame-count finish semantics unchanged — they are diagnostic drivers, not the interactive browser, and SP-6a does not multi-window them.

**Last-tab-close → close-window (indirect).** Today `tabs.NewManager(onCloseLast)` wires `onCloseLast → f.window.Close()` (`main.go:378-382`), and `TabManager.CloseTab` fires it in a goroutine when the last tab closes (`internal/tabs/manager.go:154-161`). SP-6a **keeps `onCloseLast` unchanged**: closing the last tab still calls `f.window.Close()`. The manager's per-window teardown goroutine (`startWindow`) waits on the loop's `stopped` signal — which fires when `window.Close()` ends the loop — then runs the teardown order and calls `m.removeWindow(f)`. So closing a window's last tab closes **that window** and removes it from the registry; the app quits only when `removeWindow` empties the slice (the **last window**). `showActiveTabAfterBatch` already no-ops at `Count()==0` (`interaction.go:1391`), so no empty-window repaint is introduced.

## 4. Component B — headless multi-window verification harness

No production change; a test harness in `cmd/goosie` (package `goosie`, internal, to reach `windowManager`/`framePath`) built on `headless.New` + a shared `ManualClock`:

- Spawn N windows against one clock; `Advance` ticks all in lockstep (the pattern already used by `render_work_test.go`).
- Inject events per window via `headless.Window.Inject` (`headless/window.go:222`) and assert per-window routing (an event in window 1 never touches window 2's tabs).
- Read back pixels per window (`Presented`/`WritePNG`) to prove independent rendering.

## 5. Data flow

**Spawn:** `run` → `m.spawn(url)` → build per-window machinery + share profile pointers → `tabMgr.NewTab` → `openWindow` (headless: `platform.Select` → `headless.New`) → `surface.NewLoop` → start loop + shutdown goroutines → append to `m.windows`.

**Close one (others live):** user closes window's last tab (or window close button) → `onCloseLast`/close → `f.window.Close()` → the loop stops → f's teardown goroutine runs the order (wcancel/runCancel/navWg.Wait/closeAllSessions/window.Close/pool.Close) → `m.removeWindow(f)` drops it from the slice → other windows' loops keep running.

**Quit on last:** `m.removeWindow` empties the slice → manager-done signal (`doneOnce`) → `m.run` stops waiting → final save (session/history/flags) → return. On native (N=1 in SP-6a) this is identical to today: the single window's close ends `GoosieAppRun` and `run` saves + exits.

## 6. Error handling / edge cases

- **Spawn failure:** `m.spawn` returns the error; `run` fails fast if the **first** window cannot open (today's behavior). A failed **later** spawn (headless test only in SP-6a) leaves existing windows running and reports the error; no partial window is appended.
- **Teardown ordering:** each window keeps the existing order (`closeAllSessions` before `window.Close`, so video/audio stops while the main-thread loop still services it — `main.go:1078-1085`). The manager joins every window's `navWg` and loop before the final save.
- **Concurrent close:** `closeWindow` is idempotent and mutex-guarded; `onCloseLast` fires in a goroutine (`manager.go:159`), so two rapid closes cannot double-remove or race the slice.
- **Shared-store lifetime:** profile stores are created once by the manager and outlive every window; the final `history.Save`/`flags.Save` runs once after all windows close (not per window), avoiding N concurrent writes to one file.
- **Memory:** per-window `pool`/`composer`/3-slot surface ring scale linearly with window count (terrain S11). SP-6a bounds this only by the headless tests' window count; a shared pool / window cap is deferred (§9).
- **No native regression:** native still runs N=1 through the unchanged `GoosieAppRun`; SP-6a adds no native code and no user-facing shortcut, so the shipping macOS path is byte-for-byte the current single-window behavior.

## 7. Testing

- **Headless lifecycle TDD (the acceptance core):**
  - `TestManagerSpawnTwoWindows` — N=2 windows coexist, each renders independently (pixel read-back differs).
  - `TestManagerCloseOneKeepsOthers` — close window 1; window 2's loop keeps presenting; manager not done.
  - `TestManagerQuitOnLastWindow` — close the last window; manager-done fires; `run` returns.
  - `TestManagerLastTabClosesWindow` — closing a window's last tab triggers `closeWindow` for that window only.
  - `TestManagerPerWindowTabIsolation` — an event/tab in window 1 never mutates window 2's `tabMgr`.
  - `TestManagerSharesProfileStores` — a bookmark added in window 1 is visible in window 2 (same `*bookmarks.Store`); same for history.
- **Race:** `go test -race ./cmd/goosie/` with the multi-window tests (N loops + shared stores + concurrent close).
- **No-regression:** the existing single-window tests (`main_test.go`, `tab_lifecycle_test.go`, `render_work_test.go`, session restore) stay green; native single-window path unchanged.
- **Full matrix:** `go test ./...`, `go test -race ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, gate suite, qc — all green. `archtest` confirms the manager is an owned value (no new mutable global) and cgo stays confined to darwin (SP-6a adds none).
- **Mutation evidence:** break quit-on-last (manager-done never fires) → `TestManagerQuitOnLastWindow` red; restore → green. Break profile sharing (per-window stores) → `TestManagerSharesProfileStores` red; restore → green.
- **Visual:** SP-6a is backend logic with no new user-facing native UI; headless pixel read-back is the visual evidence. Native multi-window visual sign-off belongs to SP-6b (and is deferred there too — no display, consistent with all prior native CUA work).

## 8. Acceptance

SP-6a ships when: the `windowManager` owns N `framePath`s with correctly shared profile stores; the headless lifecycle tests (spawn, close-one-keep-others, quit-on-last, last-tab-closes-window, tab isolation, profile sharing) are green under `-race`; the native single-window path and all existing tests are unchanged/green; the full matrix + archtest + gate + qc are green; mutation evidence recorded. Commits on `main`, **no push**. Roadmap records SP-6a complete and SP-6b (native shim + Cmd+N) as the next sub-project.

## 9. Deferred to SP-6b / later (YAGNI, with evidence)

- **SP-6b — native shim adaptation + user wiring:** app-wide quit in `GoosieAppRun` (terrain S4), one shared app delegate with a window registry (S1/S3), `applicationShouldTerminateAfterLastWindowClosed` semantics (S2), a main-queue create trampoline so a non-main goroutine can spawn window #2 while `Run` turns (S15), `GoosieClose` ordering the window out (S12), and retaining the per-window `NSWindow` handle (S13, also a latent file-panel bug). **Plus** Cmd+N (`handleTabShortcut` currently handles only Cmd+Shift+N→incognito, `toolbar_window.go:1228`) and a File→New Window menu item. Compile-verified; native visual sign-off deferred (no display).
- **Window-aware session persistence grouping:** aggregating all windows' tabs into one session file with window grouping. SP-6a preserves today's single-window session save (native N=1); grouping lands with SP-6b's user-facing multi-window.
- **Drag-tab-between-windows, shared raster pool, window cap, per-window sheet-modal dialogs, cascade/offset placement (S8), per-display display-link (S7):** independent enhancements, each its own future increment when a journey + acceptance criteria exist.
- **L3/L4 (incremental style/layout invalidation):** still deferred to SP-7+ per the SP-5 terrain evidence (hard restyle-veto assertions, no per-rule `Deps`, full-pipeline reflow). Unchanged by SP-6.
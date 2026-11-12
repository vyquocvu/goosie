# SP-6a window manager + multi-window lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Approval gates are waived by standing delegation; commits on `main` are authorized, **no push**.

**Goal:** Introduce an owned `windowManager` that runs N `framePath` windows with shared profile stores and quit-on-last-window-close, verified on headless under the race detector, while the native single-window path and all diagnostic drivers stay unchanged.

**Architecture:** Split `build(c)` into `buildProfile(c)` (shared stores, created once) + `buildWindow(c, p)` (per-window machinery), keeping `build(c)` as a wrapper so all 7 existing callers are untouched. The manager owns one `*profile` and N `*framePath`; each window runs its own loop + teardown goroutine under a per-window context; closing a window removes it, and the last removal signals app-done. `run()` delegates the interactive non-private path to the manager; paced/screenshot/private keep the existing orchestration verbatim.

**Tech Stack:** Go, `cmd/goosie` (package `main`, a `FreeDirs` archtest-exempt package), `internal/platform/headless`, `internal/surface`, `internal/tabs`.

**Spec:** `docs/superpowers/specs/2026-11-12-sp6a-window-manager-design.md`.

**Standing rules:** `gofmt -w` before every commit (`TestRepoIsGofmtFormatted` enforces). NEVER pipe test output through `tail` (masks exit codes) — redirect to `/tmp` logs, `echo $?`, then read. Revert mutations with `git checkout` ONLY after the surrounding work is committed. Read `docs/roadmap-v2.md` with sed/Grep (Read fails on its size). No new mutable package-level vars under `internal/` (the manager lives in `cmd/goosie` and is an owned value). cgo stays confined to `internal/platform/darwin` — SP-6a adds none.

**Key terrain facts (verified 2026-11-12):**
- `build(c config) (*framePath, error)` — `cmd/goosie/main.go:232-474`. Callers: `main.go:936` (production), `main_test.go:782,798,808`, `e2e_test.go:53,507,563`, `journey_history_private_test.go:35`. Keep `build(c)` working identically.
- Profile stores created inline in `build`: `main.go:235-277` (preferences/downloadDir/fonts/client/bookmarks/history) and the `framePath` literal `main.go:332-349` (client, privateClient, fonts, bookmarks, history, downloads, storage, incognitoStorage, broadcastHub, incognitoBroadcastHub, theme, userscripts, flags, perms, preferences). Load calls `main.go:354-362`; theme default `:351-353`.
- Per-window machinery in `build`: sched/pool/composer/layer/rec/spec (`:311-330`), nav channels/zoom (`:338-340`), drain goroutines (`:363-373`), toolbar/tabMgr wiring (`:374-467`), initial plan (`:468-472`).
- `run(args)` — `main.go:902-1126`. Early returns: clearData (`:907`), printPDF (`:915`). Interactive orchestration: `:936-1126`. Paced/screenshot driver setup: `:990-1030`. Loop goroutine `:1051`, shutdown goroutine `:1061-1089`, Runner handoff `:1091-1095`, final save `:1097-1118`, report `:1125`.
- `onCloseLast` wired at `main.go:378-382` → `f.window.Close()`; fired by `tabs.TabManager.CloseTab` when the last tab closes (`internal/tabs/manager.go:154-161`, in a goroutine). **Keep this wiring** — the manager observes the resulting window close.
- `runSessionSnapshots`/`saveActiveSession` (`navigation.go:901,925`) write `f.config.sessionActiveFile` (one profile-level file) and no-op when it is `""`. Secondary windows get `sessionActiveFile=""` so they do not clobber the primary's journal (window-aware grouping is deferred to SP-6b).
- `report(d *driver)` (`report.go:21`) prints a summary to stderr and returns nil when `c.out==""`; `report(nil)` is the interactive exit path.
- Headless: `platform.Select` → `headless.New` (SystemClock when no Clock plumbed); `headless.Window.Inject` (`headless/window.go:222`), `Close` idempotent. Two headless windows already coexist (`render_work_test.go:699,1124`). E2E harness `newBrowserE2E` (`e2e_test.go:40-78`) uses `build`+`openWindow`+manual loop, NOT `run()`.

---

## Task 1: Split `build` into profile + window (pure refactor)

**Files:**
- Create: `cmd/goosie/profile.go`
- Modify: `cmd/goosie/main.go` (`build` `:232-474`)

This task changes NO behavior. `build(c)` must produce an identical `framePath`. The existing suite (E2E, main_test, journey_history_private, and every `build` caller) is the regression guard.

- [ ] **Step 1: Create the profile struct + buildProfile**

Create `cmd/goosie/profile.go`. Move the profile-level store creation out of `build` into `buildProfile`. Read `main.go:235-277` and `:332-362` first and move exactly those blocks.

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/flags"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/userscript"
)

// profile holds the stores every window of one invocation shares: one cookie
// jar, one history, one bookmark store, one localStorage per origin, one
// permissions store, one glyph atlas. The windowManager creates it once and
// hands the same pointers to each framePath, so a bookmark added in one window
// is visible in every other. It is an owned value, never a package global.
type profile struct {
	client                net.HTTP
	privateClient         net.HTTP
	fonts                 *raster.Fonts
	bookmarks             *bookmarks.Store
	history               *history.Store
	downloads             *download.Manager
	storage               *js.OriginStorageRegistry
	incognitoStorage      *js.OriginStorageRegistry
	broadcastHub          *js.BroadcastHub
	incognitoBroadcastHub *js.BroadcastHub
	perms                 *perms.Manager
	preferences           preferences.Settings
	flags                 *flags.Manager
	userscripts           *userscript.Manager
	theme                 string
}

// buildProfile creates the shared stores once. It is the profile-level half of
// the old build: preferences/download-dir resolution, fonts, HTTP clients,
// bookmark/history load, and the registry/manager constructors, plus the
// userscripts/flags/perms Load calls that used to run per build.
func buildProfile(c config) (*profile, error) {
	profilePreferences, prefErr := preferences.Load(c.preferencesFile)
	if prefErr != nil {
		fmt.Fprintf(os.Stderr, "goosie: preferences: %v\n", prefErr)
	}
	effectiveDir, err := effectiveDownloadDir(c.downloadDir, c.downloadDirExplicit, profilePreferences.DownloadDir)
	if err != nil {
		return nil, fmt.Errorf("goosie: download directory: %w", err)
	}
	profilePreferences.DownloadDir = effectiveDir

	fonts, err := raster.NewFonts()
	if err != nil {
		return nil, fmt.Errorf("goosie: %w", err)
	}

	var client net.HTTP
	if c.private {
		client = net.DefaultPrivateClient()
	} else {
		persistent, cerr := net.DefaultClientWithCookies(c.cookieFile)
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "goosie: cookies: %v\n", cerr)
			client = net.DefaultClient()
		} else {
			client = persistent
		}
	}

	var bookmarkStore *bookmarks.Store
	var historyStore *history.Store
	if !c.private {
		bookmarkStore, err = bookmarks.Load(c.bookmarkFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: bookmarks: %v\n", err)
			bookmarkStore = nil
		}
		historyStore, err = history.Load(c.historyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "goosie: history: %v\n", err)
			historyStore = nil
		}
	}

	p := &profile{
		client:                client,
		privateClient:         net.DefaultPrivateClient(),
		fonts:                 fonts,
		bookmarks:             bookmarkStore,
		history:               historyStore,
		downloads:             download.NewManager(effectiveDir),
		storage:               js.NewOriginStorageRegistry(filepath.Join(c.stateDir(), "storage"), false),
		incognitoStorage:      js.NewOriginStorageRegistry("", true),
		broadcastHub:          js.NewBroadcastHub(),
		incognitoBroadcastHub: js.NewBroadcastHub(),
		theme:                 c.theme,
		userscripts:           userscript.NewManager(c.userscriptDir),
		flags:                 flags.NewManager(c.flagsFile),
		perms:                 perms.NewManager(c.permsFile),
		preferences:           profilePreferences,
	}
	if p.theme == "" {
		p.theme = "system"
	}
	if !c.paced() && p.userscripts != nil {
		_ = p.userscripts.Load()
	}
	if !c.paced() && p.flags != nil {
		_ = p.flags.Load()
	}
	if !c.paced() && p.perms != nil {
		_ = p.perms.Load()
	}
	return p, nil
}

// close releases the shared HTTP clients once, after every window is gone.
func (p *profile) close() {
	if p.client != nil {
		if err := p.client.Close(); err != nil {
			fmt.Fprintln(os.Stderr, programErrorLine(err))
		}
	}
	if p.privateClient != nil {
		_ = p.privateClient.Close()
	}
}
```

NOTE: confirm the import paths and constructor signatures against the existing `build` body (`main.go:235-362`) — they are copied verbatim from there. `programErrorLine` is defined at `main.go:858`. The `effectiveDir` is used for the downloads manager exactly as `build` used `c.downloadDir` after resolution (`:243,337`).

- [ ] **Step 2: Rewrite build as buildWindow + wrapper**

In `cmd/goosie/main.go`, replace the body of `build` (`:232-474`) so that:
1. `build(c config) (*framePath, error)` becomes a wrapper:

```go
func build(c config) (*framePath, error) {
	p, err := buildProfile(c)
	if err != nil {
		return nil, err
	}
	return buildWindow(c, p)
}
```

2. Add `buildWindow(c config, p *profile) (*framePath, error)` containing the **per-window** remainder of the old build: the scene/layer selection (`:279-309`), the worker pool + recorder (`:311-322`), the `framePath` literal (`:323-350`) with profile fields assigned from `p`, the drain goroutines (`:363-373`), the toolbar/tabMgr wiring (`:374-467`), and the initial plan (`:468-472`). The literal's profile fields become:

```go
	f := &framePath{
		sched:                 raster.NewScheduler(layer, wp, frame.Viewport{Size: dev}, scale, raster.Pref{}),
		pool:                  wp,
		composer:              surface.NewComposer(dev, frame.NewBitmapPool(dev, 2)),
		layer:                 layer,
		docHeight:             spec.DocHeight,
		rec:                   rec,
		spec:                  spec,
		config:                c,
		client:                p.client,
		privateClient:         p.privateClient,
		fonts:                 p.fonts,
		bookmarks:             p.bookmarks,
		history:               p.history,
		downloads:             p.downloads,
		navResults:            make(chan navResult, 16),
		imgResults:            make(chan imgResult, 16),
		zoom:                  1.0,
		storage:               p.storage,
		incognitoStorage:      p.incognitoStorage,
		broadcastHub:          p.broadcastHub,
		incognitoBroadcastHub: p.incognitoBroadcastHub,
		theme:                 p.theme,
		userscripts:           p.userscripts,
		flags:                 p.flags,
		perms:                 p.perms,
		preferences:           p.preferences,
	}
```

Delete the now-duplicated profile creation (`:235-277`), the theme default (`:351-353`), and the Load calls (`:354-362`) from `buildWindow` — they live in `buildProfile` now. Keep `dev := c.devSize()` and `scale := float32(c.dpr)` at the top of `buildWindow`. Keep the `onCloseLast → f.window.Close()` wiring (`:378-382`) UNCHANGED. The scene/layer block references `client.Close()` on the sceneSpec error path (`:290`); change that to `p.client.Close()`.

NOTE: read the whole old `build` before editing. Every line that is not profile creation moves to `buildWindow`, but the profile locals (`fonts`, `client`, `profilePreferences`, `bookmarkStore`, `historyStore`) now live in `buildProfile`, so the per-window remainder must substitute them. The verified substitutions (grep `\bfonts\b|\bclient\b|\bprofilePreferences\b` over `main.go:279-472`) are EXACTLY:
- `:290` `_ = client.Close()` → `_ = p.client.Close()` (sceneSpec error path)
- `:311` `raster.DefaultRaster(fonts, raster.NewGlyphAtlas(glyphAtlasBudget, fonts))` → use `p.fonts` for both `fonts`
- `:375` `toolbar.NewState(dev.W, fonts)` → `toolbar.NewState(dev.W, p.fonts)`
- `:398` `normalizeAddress(c.url, profilePreferences.SearchURLTemplate)` → `p.preferences.SearchURLTemplate`
- `:404` `shouldRestoreSession(c.private, profilePreferences.StartupMode)` → `p.preferences.StartupMode`

The `framePath` literal's profile fields are already shown above with `p.xxx`. The toolbar wiring (`:374-467`) otherwise references `f.downloads`, `f.toolbar`, `f.tabMgr` — unchanged, since `f.downloads` now points at the shared `p.downloads`. After editing, `go build ./...` must pass; any remaining bare `fonts`/`client`/`profilePreferences` in `buildWindow` is a missed substitution.

- [ ] **Step 3: Verify the refactor is behavior-preserving**

Run: `gofmt -w cmd/goosie/profile.go cmd/goosie/main.go && go build ./... > /tmp/sp6a-t1.log 2>&1; echo $?; cat /tmp/sp6a-t1.log`
Expected: `0`, no output.

Run: `go test ./cmd/goosie/ -run 'TestE2E|TestBuild|TestJourneyHistoryPrivate' -count=1 > /tmp/sp6a-t1.log 2>&1; echo $?; cat /tmp/sp6a-t1.log`
Expected: `0`, PASS. (These exercise `build(c)` end-to-end on headless.)

Run: `go test ./cmd/goosie/ -count=1 > /tmp/sp6a-t1-all.log 2>&1; echo $?; tail -40 /tmp/sp6a-t1-all.log`
Expected: `0`, ok. (Full package green proves the wrapper is identical for all callers.)

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/goosie/profile.go cmd/goosie/main.go
git add cmd/goosie/profile.go cmd/goosie/main.go
git commit -m "refactor(goosie): split build into shared profile + per-window machinery (SP-6a)"
```

---

## Task 2: windowManager skeleton + spawn (two headless windows)

**Files:**
- Create: `cmd/goosie/window_manager.go`
- Test: `cmd/goosie/window_manager_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/goosie/window_manager_test.go` (package `main`, internal, to reach `windowManager`/`framePath`). The harness builds a headless config and a manager, then spawns windows.

```go
package main

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/platform"
)

func testManagerConfig(t *testing.T) config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	c, err := parse([]string{
		"-backend", platform.Headless,
		"-width", "320", "-height", "240", "-dpr", "1",
		"-download-dir", home,
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return c
}

// waitWindows polls the manager's window count until it equals want or the
// deadline passes. Teardown runs on a goroutine, so the count settles async.
func waitWindows(t *testing.T, m *windowManager, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		n := len(m.windows)
		m.mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	t.Fatalf("window count = %d, want %d", n, want)
}

func TestManagerSpawnTwoWindows(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	if f1 == f2 {
		t.Fatal("spawn returned the same framePath twice")
	}
	if f1.window == f2.window {
		t.Fatal("two windows share one platform window")
	}
	waitWindows(t, m, 2)
	// Both loops are live: each window presents independently.
	if f1.loop == nil || f2.loop == nil {
		t.Fatal("a spawned window has no loop")
	}
}
```

- [ ] **Step 2: Run it red**

Run: `go test ./cmd/goosie/ -run TestManagerSpawnTwoWindows -count=1 > /tmp/sp6a-t2.log 2>&1; echo $?; cat /tmp/sp6a-t2.log`
Expected: FAIL — `undefined: newWindowManager` / `undefined: windowManager`.

- [ ] **Step 3: Write the manager skeleton + spawn**

Create `cmd/goosie/window_manager.go`:

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/session"
)

// windowManager owns one profile and N windows. It is an owned value created by
// run, never a package global (internal/ forbids mutable globals; cmd/goosie is
// archtest-exempt). Each window runs its own loop and teardown goroutine under a
// per-window context; closing a window removes it, and removing the last one
// signals done so run proceeds to the final save and exit.
type windowManager struct {
	config  config
	profile *profile

	mu      sync.Mutex
	windows []*framePath
	primary *framePath
	errs    []<-chan error

	ctx    context.Context
	cancel context.CancelFunc
	sig    chan os.Signal

	done     chan struct{}
	doneOnce sync.Once
}

func newWindowManager(c config) (*windowManager, error) {
	p, err := buildProfile(c)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &windowManager{
		config:  c,
		profile: p,
		ctx:     ctx,
		cancel:  cancel,
		sig:     make(chan os.Signal, 1),
		done:    make(chan struct{}),
	}, nil
}

// spawn builds and opens a new window sharing the manager's profile, starts its
// loop and teardown, and loads initialURL when non-empty. The first window is
// the primary and keeps the crash journal; later windows get sessionActiveFile=""
// so they do not clobber the primary's journal (window-aware session grouping is
// deferred to SP-6b).
func (m *windowManager) spawn(initialURL string) (*framePath, error) {
	c := m.config
	m.mu.Lock()
	isFirst := len(m.windows) == 0
	if !isFirst {
		c.sessionActiveFile = ""
	}
	m.mu.Unlock()

	f, err := buildWindow(c, m.profile)
	if err != nil {
		return nil, err
	}
	if err := f.openWindow(); err != nil {
		_ = f.pool.Close()
		return nil, err
	}
	loopErr := m.startWindow(f)

	m.mu.Lock()
	m.windows = append(m.windows, f)
	m.errs = append(m.errs, loopErr)
	if isFirst {
		m.primary = f
	}
	m.mu.Unlock()

	if initialURL != "" {
		f.navigateTab(initialURL)
	}
	return f, nil
}

// startWindow runs f's loop and teardown under a per-window context. It returns
// a channel carrying the loop's exit error. The teardown mirrors the proven
// single-window order: cancel the loop, join it, stop navigation, close every
// document, then close the window. Closing the window is what onCloseLast
// (last tab) and the native close button already do, so the manager simply
// observes the resulting stop and removes the window.
func (m *windowManager) startWindow(f *framePath) <-chan error {
	wctx, wcancel := context.WithCancel(m.ctx)
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	if f.config.sessionActiveFile != "" {
		go f.runSessionSnapshots(f.runCtx)
	}
	f.started = time.Now()

	loopErr := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		loopErr <- f.loop.Run(wctx)
		close(stopped)
	}()
	go func() {
		select {
		case <-stopped:
		case <-m.sig:
		}
		wcancel()
		<-stopped
		f.runCancel()
		f.navWg.Wait()
		f.closeAllSessions()
		_ = f.window.Close()
		if f.pool != nil {
			_ = f.pool.Close()
		}
		m.removeWindow(f)
	}()
	return loopErr
}

// removeWindow drops f from the registry and signals done when the last window
// is gone. Idempotent: a window is removed once.
func (m *windowManager) removeWindow(f *framePath) {
	m.mu.Lock()
	for i, w := range m.windows {
		if w == f {
			m.windows = append(m.windows[:i], m.windows[i+1:]...)
			break
		}
	}
	empty := len(m.windows) == 0
	m.mu.Unlock()
	if empty {
		m.doneOnce.Do(func() { close(m.done) })
	}
}

// closeWindow is the explicit close entry (used by tests and, in SP-6b, by the
// native close path). It closes the platform window; the teardown goroutine
// started by startWindow does the join and removal.
func (m *windowManager) closeWindow(f *framePath) {
	if f != nil && f.window != nil {
		_ = f.window.Close()
	}
}

// cleanup cancels the app context and waits for every window's teardown to
// finish, then releases the shared clients once. Pools are closed per-window in
// startWindow's teardown (Pool.Stats is safe after Close, so report still reads
// final counts). Waiting for done before profile.close avoids closing the shared
// HTTP client while a document is still tearing down.
func (m *windowManager) cleanup() {
	m.cancel()
	m.mu.Lock()
	spawned := len(m.errs) > 0
	m.mu.Unlock()
	if spawned {
		select {
		case <-m.done:
		case <-time.After(5 * time.Second):
		}
	}
	m.profile.close()
}

var _ = frame.Rect{}
var _ = session.Save
var _ = signal.Notify
var _ = syscall.SIGTERM
var _ = platform.Headless
```

NOTE: the trailing `var _ = ...` lines are temporary placeholders that keep the `frame`, `session`, `signal`, `syscall`, and `platform` imports compiling until Task 4 adds `run`/`save`, which use them. **Task 4 removes all five placeholders.** After Task 4, `frame` is no longer referenced (`run`/`save` use `platform`, `session`, `signal`, `syscall`, `fmt`, `os`, `context`, `sync`, `time`), so drop the `frame` import then. Confirm `f.loop`, `f.pool`, `f.runCtx`, `f.runCancel`, `f.navWg`, `f.started` field names against `main.go:122-228`.

- [ ] **Step 4: Run it green**

Run: `gofmt -w cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go && go test ./cmd/goosie/ -run TestManagerSpawnTwoWindows -count=1 > /tmp/sp6a-t2.log 2>&1; echo $?; cat /tmp/sp6a-t2.log`
Expected: `0`, PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
git add cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
git commit -m "feat(goosie): windowManager spawns N headless windows over a shared profile (SP-6a)"
```

---

## Task 3: close-one-keeps-others + quit-on-last-window

**Files:**
- Modify: `cmd/goosie/window_manager_test.go`

The lifecycle logic already exists in `removeWindow`/`startWindow` (Task 2). This task pins it with tests.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/goosie/window_manager_test.go`:

```go
func TestManagerCloseOneKeepsOthers(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	m.closeWindow(f1)
	waitWindows(t, m, 1)

	select {
	case <-m.done:
		t.Fatal("manager signalled done while a window is still open")
	default:
	}
	// f2 is still registered and its loop has not been torn down.
	m.mu.Lock()
	stillThere := len(m.windows) == 1 && m.windows[0] == f2
	m.mu.Unlock()
	if !stillThere {
		t.Fatal("closing window 1 removed window 2")
	}
}

func TestManagerQuitOnLastWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	m.closeWindow(f1)
	waitWindows(t, m, 1)
	m.closeWindow(f2)

	select {
	case <-m.done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager never signalled done after the last window closed")
	}
	waitWindows(t, m, 0)
}
```

- [ ] **Step 2: Run them green (logic exists from Task 2)**

Run: `go test ./cmd/goosie/ -run 'TestManagerCloseOneKeepsOthers|TestManagerQuitOnLastWindow' -count=1 > /tmp/sp6a-t3.log 2>&1; echo $?; cat /tmp/sp6a-t3.log`
Expected: `0`, PASS. If either fails, the bug is in `removeWindow`/`startWindow` teardown — fix there, not in the test.

- [ ] **Step 3: Mutation check — break quit-on-last**

In `removeWindow`, temporarily comment out the `m.doneOnce.Do(func() { close(m.done) })` block.
Run: `go test ./cmd/goosie/ -run TestManagerQuitOnLastWindow -count=1 > /tmp/sp6a-t3m.log 2>&1; echo $?; cat /tmp/sp6a-t3m.log`
Expected: FAIL — "manager never signalled done". Restore the block (it is uncommitted; re-edit it back). Re-run green.

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/goosie/window_manager_test.go
git add cmd/goosie/window_manager_test.go
git commit -m "test(goosie): pin close-one-keeps-others and quit-on-last-window (SP-6a)"
```

---

## Task 4: manager.run orchestration + final save (headless N=1)

**Files:**
- Modify: `cmd/goosie/window_manager.go` (add `run`, `save`; remove Task 2 placeholders)
- Modify: `cmd/goosie/window_manager_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/goosie/window_manager_test.go`. This runs the full interactive orchestration on headless: spawn primary, run, close the window from another goroutine, assert clean exit + session/history save.

```go
func TestManagerRunSavesAndTearsDown(t *testing.T) {
	c := testManagerConfig(t)
	m, err := newWindowManager(c)
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	// Seed a bookmark so the shared history/bookmark save path has state.
	if m.profile.bookmarks != nil {
		m.profile.bookmarks.Toggle("https://example.test/", "Example")
	}

	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()

	// Wait for the primary window to exist, then close it to end the run.
	var primary *framePath
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		primary = m.primary
		m.mu.Unlock()
		if primary != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if primary == nil {
		t.Fatal("run never spawned a primary window")
	}
	m.closeWindow(primary)

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the window closed")
	}

	if _, err := os.Stat(c.sessionFile); err != nil {
		t.Fatalf("session file not saved on clean exit: %v", err)
	}
	if _, err := os.Stat(c.sessionActiveFile); !os.IsNotExist(err) {
		t.Fatalf("crash journal not removed on clean exit: %v", err)
	}
}
```

Add `"os"` to the test imports. NOTE: `bookmarks.Store` exposes `Toggle(url, title) bool` (adds when absent, removes when present) and `Contains(url) bool` — verified at `internal/bookmarks/bookmarks.go:79,139`. There is no `Add` method; `Toggle` returns bool, no error.

- [ ] **Step 2: Run it red**

Run: `go test ./cmd/goosie/ -run TestManagerRunSavesAndTearsDown -count=1 > /tmp/sp6a-t4.log 2>&1; echo $?; cat /tmp/sp6a-t4.log`
Expected: FAIL — `m.run undefined`.

- [ ] **Step 3: Add run + save; remove placeholders**

In `cmd/goosie/window_manager.go`, remove the Task 2 `var _ = ...` placeholder lines and add:

```go
// run drives the interactive browser: spawn the primary window, load its URL,
// hand the main thread to a native run loop if the backend owns one, wait for
// every window to close, then save once and report. Paced/screenshot/private
// runs do not come here — run() keeps their existing single-window path.
func (m *windowManager) run() error {
	signal.Notify(m.sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(m.sig)
	defer m.cancel()

	if _, err := m.spawn(m.config.url); err != nil {
		return err
	}
	primary := m.primary
	if m.config.url == "" && primary.restoredActiveURL != "" {
		primary.navigateTabNoHistory(primary.restoredActiveURL)
	}

	if r, ok := primary.window.(platform.Runner); ok {
		r.Run()
	}
	<-m.done

	m.save()

	var firstErr error
	for _, ec := range m.errs {
		if err := <-ec; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("goosie: %w", firstErr)
	}
	return primary.report(nil)
}

// save persists the primary window's session plus the shared history/flags once,
// after every window is gone, and removes the crash journal. This is the
// non-private interactive save (run() routes private runs elsewhere). Window-aware
// session grouping is deferred to SP-6b.
func (m *windowManager) save() {
	primary := m.primary
	if primary == nil {
		return
	}
	primary.sessionMu.Lock()
	if !primary.sessionDisabled.Load() {
		if err := session.Save(m.config.sessionFile, sessionState(primary)); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: save session: %v\n", err)
		}
	}
	primary.sessionMu.Unlock()
	if m.profile.history != nil {
		if err := m.profile.history.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: save history: %v\n", err)
		}
	}
	if m.profile.flags != nil {
		if err := m.profile.flags.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: save flags: %v\n", err)
		}
	}
	if m.config.sessionActiveFile != "" {
		_ = os.Remove(m.config.sessionActiveFile)
	}
}
```

Add `"fmt"` to the imports. Ensure `platform`, `session`, `signal`, `syscall`, `frame` imports are all used now (`frame` may still be unused — if so, drop it; `run`/`save` use `platform`, `session`, `signal`, `syscall`, `fmt`, `os`, `context`, `sync`, `time`). Remove any import that is no longer referenced.

NOTE: `m.errs` channels are buffered (size 1) and each loop goroutine sends exactly once before closing `stopped`, so receiving after `<-m.done` never blocks. `primary.report(nil)` matches the interactive exit at `main.go:1125` (`d` is nil for interactive). Confirm `sessionState`, `session.Save`, `primary.restoredActiveURL`, `navigateTabNoHistory` against `navigation.go:851,634` and `main.go:1100`.

- [ ] **Step 4: Run it green**

Run: `gofmt -w cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go && go test ./cmd/goosie/ -run 'TestManager' -count=1 > /tmp/sp6a-t4.log 2>&1; echo $?; cat /tmp/sp6a-t4.log`
Expected: `0`, all TestManager* PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
git add cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
git commit -m "feat(goosie): windowManager.run drives N loops with one final save (SP-6a)"
```

---

## Task 5: route the interactive run() through the manager

**Files:**
- Modify: `cmd/goosie/main.go` (`run` `:902-1126`)

The paced/screenshot/private paths keep the existing orchestration **verbatim**. Only the interactive non-private path delegates to the manager.

- [ ] **Step 1: Add the manager branch to run()**

In `cmd/goosie/main.go`, immediately after the printPDF early return (after `:935`, before `f, err := build(c)` at `:936`), insert:

```go
	if !c.paced() && !c.private {
		m, err := newWindowManager(c)
		if err != nil {
			return err
		}
		defer m.cleanup()
		return m.run()
	}
```

Leave the rest of `run` (`:936-1126`) unchanged. It now serves only paced/screenshot/private runs. `c.paced()` is `gate||bench||screenshot` (`config.go:104`), so `-gate`, `-bench`, `-screenshot`, `-print-pdf` (early return), and `-private` all bypass the manager exactly as before.

NOTE: `m.cleanup()` closes the shared clients and any live pools; `m.run()` already waits for all windows to tear down before returning, so cleanup runs after teardown. The old path's `defer f.pool.Close()` / `defer f.client.Close()` (`:940-945`) remain for the bypass paths. Do not delete them.

- [ ] **Step 2: Verify no regression to the bypass paths + build**

Run: `gofmt -w cmd/goosie/main.go && go build ./... > /tmp/sp6a-t5.log 2>&1; echo $?; cat /tmp/sp6a-t5.log`
Expected: `0`.

Run: `go test ./cmd/goosie/ -run 'TestClearData|TestPDFExport|TestE2E|TestGate|TestBench|TestScreenshot' -count=1 > /tmp/sp6a-t5.log 2>&1; echo $?; cat /tmp/sp6a-t5.log`
Expected: `0`, PASS (these exercise the unchanged bypass paths and the build wrapper). Confirm the real test names with `go test ./cmd/goosie/ -list '.*' | grep -iE 'clear|pdf|gate|bench|screenshot|e2e'` and adjust the `-run` pattern to match.

Run: `go test ./cmd/goosie/ -count=1 > /tmp/sp6a-t5-all.log 2>&1; echo $?; tail -40 /tmp/sp6a-t5-all.log`
Expected: `0`, ok.

- [ ] **Step 3: Commit**

```bash
gofmt -w cmd/goosie/main.go
git add cmd/goosie/main.go
git commit -m "feat(goosie): route interactive runs through the windowManager (SP-6a)"
```

---

## Task 6: last-tab-close closes that window

**Files:**
- Modify: `cmd/goosie/window_manager_test.go`

The mechanism already exists: `onCloseLast → f.window.Close()` (`main.go:378`) → loop stops → `startWindow` teardown → `removeWindow`. This test pins it.

- [ ] **Step 1: Write the test**

Append to `cmd/goosie/window_manager_test.go`:

```go
func TestManagerLastTabClosesWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	waitWindows(t, m, 2)

	// f1 has exactly one tab (the startup tab). Closing it fires onCloseLast,
	// which closes f1's window; the manager must remove f1 and keep f2.
	tabs1 := f1.tabMgr.Tabs()
	if len(tabs1) != 1 {
		t.Fatalf("f1 tab count = %d, want 1", len(tabs1))
	}
	f1.closeTab(tabs1[0].ID())

	waitWindows(t, m, 1)
	m.mu.Lock()
	kept := len(m.windows) == 1 && m.windows[0] == f2
	m.mu.Unlock()
	if !kept {
		t.Fatal("closing window 1's last tab did not leave window 2 registered")
	}
	select {
	case <-m.done:
		t.Fatal("manager quit while window 2 was still open")
	default:
	}
}
```

NOTE: confirm `f.tabMgr.Tabs()` returns `[]*tabs.Tab` and `Tab.ID()` exists (`internal/tabs/manager.go`). `closeTab` is `interaction.go:1319`. The startup tab is created in `buildWindow` (`main.go:384`). If a spawned window starts with zero tabs because no URL was given, confirm against `buildWindow` — the interactive path always creates `firstTab` (`:384`), so count is 1.

- [ ] **Step 2: Run it green**

Run: `go test ./cmd/goosie/ -run TestManagerLastTabClosesWindow -count=1 > /tmp/sp6a-t6.log 2>&1; echo $?; cat /tmp/sp6a-t6.log`
Expected: `0`, PASS. If it fails because `onCloseLast` fires asynchronously (`manager.go:159` uses `go m.onCloseLast()`), `waitWindows` already polls up to 3s — that covers the goroutine. If the window does not close, verify `buildWindow` kept the `onCloseLast → f.window.Close()` wiring.

- [ ] **Step 3: Commit**

```bash
gofmt -w cmd/goosie/window_manager_test.go
git add cmd/goosie/window_manager_test.go
git commit -m "test(goosie): last-tab-close closes its window, keeps the others (SP-6a)"
```

---

## Task 7: shared profile stores + per-window tab isolation

**Files:**
- Modify: `cmd/goosie/window_manager_test.go`

- [ ] **Step 1: Write the tests**

Append to `cmd/goosie/window_manager_test.go`:

```go
func TestManagerSharesProfileStores(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}

	if f1.bookmarks != f2.bookmarks {
		t.Error("windows do not share one bookmark store")
	}
	if f1.history != f2.history {
		t.Error("windows do not share one history store")
	}
	if f1.storage != f2.storage {
		t.Error("windows do not share one localStorage registry")
	}
	if f1.client != f2.client {
		t.Error("windows do not share one HTTP client")
	}
	if f1.bookmarks == nil {
		t.Skip("no bookmark store in this config")
	}
	f1.bookmarks.Toggle("https://shared.test/", "Shared")
	if !f2.bookmarks.Contains("https://shared.test/") {
		t.Error("a bookmark added in window 1 is not visible in window 2")
	}
}

func TestManagerPerWindowTabIsolation(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	f2, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 2: %v", err)
	}

	if f1.tabMgr == f2.tabMgr {
		t.Fatal("windows share one tab manager")
	}
	before := f2.tabMgr.Count()
	f1.newTab()
	f1.newTab()
	if f1.tabMgr.Count() != before+2 {
		t.Fatalf("f1 tab count = %d, want %d", f1.tabMgr.Count(), before+2)
	}
	if f2.tabMgr.Count() != before {
		t.Fatalf("f2 tab count changed to %d; windows are not isolated", f2.tabMgr.Count())
	}
}
```

NOTE: verified APIs — `bookmarks.Store.Toggle(url, title) bool` and `Contains(url) bool` (`internal/bookmarks/bookmarks.go:79,139`); `tabs.TabManager.Count() int` (`manager.go:457`), `Tabs() []*Tab` (`manager.go:463`); `f.newTab()` (`interaction.go:1258`). The bookmark store is non-nil for a non-private config with a writable HOME (the harness sets `t.TempDir()`), so the `Skip` guard is belt-and-suspenders.

- [ ] **Step 2: Run them green**

Run: `go test ./cmd/goosie/ -run 'TestManagerSharesProfileStores|TestManagerPerWindowTabIsolation' -count=1 > /tmp/sp6a-t7.log 2>&1; echo $?; cat /tmp/sp6a-t7.log`
Expected: `0`, PASS.

- [ ] **Step 3: Mutation check — break profile sharing**

In `buildWindow` (`main.go`), temporarily change `bookmarks: p.bookmarks` to `bookmarks: nil` (or a fresh `bookmarks.Load`). 
Run: `go test ./cmd/goosie/ -run TestManagerSharesProfileStores -count=1 > /tmp/sp6a-t7m.log 2>&1; echo $?; cat /tmp/sp6a-t7m.log`
Expected: FAIL — "windows do not share one bookmark store". Restore `bookmarks: p.bookmarks`. Re-run green.

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/goosie/window_manager_test.go
git add cmd/goosie/window_manager_test.go
git commit -m "test(goosie): profile stores shared, tabs isolated per window (SP-6a)"
```

---

## Task 8: race + full matrix + verification

**Files:** none (verification only)

- [ ] **Step 1: Race the manager tests**

Run: `go test -race ./cmd/goosie/ -run 'TestManager' -count=1 > /tmp/sp6a-t8-race.log 2>&1; echo $?; cat /tmp/sp6a-t8-race.log`
Expected: `0`, PASS, no DATA RACE. If a race appears, it is in the manager's shared state — guard `m.windows`/`m.errs`/`m.primary` under `m.mu` (reads included). The profile stores have their own internal locking.

- [ ] **Step 2: Full suite + race + vet + archtest + gate + qc**

Run each, redirecting to `/tmp` and checking `$?`:
```bash
go test ./... > /tmp/sp6a-t8-all.log 2>&1; echo $?
go test -race ./... > /tmp/sp6a-t8-race-all.log 2>&1; echo $?
go vet -all ./... > /tmp/sp6a-t8-vet.log 2>&1; echo $?
go test ./internal/archtest/ > /tmp/sp6a-t8-arch.log 2>&1; echo $?
go test ./test/gate -run TestGate -v > /tmp/sp6a-t8-gate.log 2>&1; echo $?
go test ./test/qc/ > /tmp/sp6a-t8-qc.log 2>&1; echo $?
```
Expected: all `0`. Read any non-zero log fully. The raster worker-pool timing test (`test/raster/TestUpdateLayerCoalescesDamage`) can flake under full-suite load; if it fails, re-run that package in isolation (`go test ./test/raster/ -count=1`) to confirm it is load-induced, not a regression (SP-6a touches no raster code).

- [ ] **Step 3: Zero-diff parity (no engine change, must be unchanged)**

Run: `make parity > /tmp/sp6a-t8-parity.log 2>&1; echo $?; tail -25 /tmp/sp6a-t8-parity.log`
Expected: `0`, 105/105 scored, ≥96.39%, gaps 4 fixtures / 6 bands (the SP-3/SP-4/SP-5 baseline). SP-6a changes no engine/layout/paint code, so parity must be byte-identical. If it moved, a refactor leaked into the engine — investigate.

- [ ] **Step 4: Commit (if any fix was needed)**

If Step 1–3 required a fix, `gofmt -w` the changed files and commit:
```bash
git add -A
git commit -m "fix(goosie): address race/matrix findings in the window manager (SP-6a)"
```
If everything was green with no changes, skip this commit.

---

## Task 9: docs + roadmap + execution record

**Files:**
- Modify: `docs/roadmap-v2.md` (P3 section `:204`, multi-window line)
- Modify: `docs/superpowers/specs/2026-11-12-sp6a-window-manager-design.md` (§3 mechanism note)
- Modify: `docs/superpowers/plans/2026-11-12-sp6a-window-manager.md` (execution record)

- [ ] **Step 1: Record SP-6a in the roadmap**

Read the P3 section with `sed -n '200,215p' docs/roadmap-v2.md`. Update the multi-window line to record SP-6a complete (Go foundation: windowManager, shared profile, N loops, quit-on-last, headless-verified) and SP-6b pending (native shim + Cmd+N). Use the Edit tool on the exact line. Keep the Vietnamese style of the surrounding text. Example replacement for the multi-window bullet:

```markdown
- **Multi-window — đang làm:** SP-6a đã có nền tảng Go (`cmd/goosie/window_manager.go`: `windowManager` sở hữu N `framePath`, profile dùng chung, N loop, quit-khi-cửa-sổ-cuối-đóng; `build` tách `buildProfile`/`buildWindow`), kiểm chứng headless + race. SP-6b còn lại: thích ứng shim darwin (app-wide quit, delegate chung, main-queue trampoline, close-orders-out) + Cmd+N/menu New Window. Chưa có grouped-session đa cửa sổ, kéo tab giữa cửa sổ.
```

- [ ] **Step 2: Align the spec §3 mechanism note**

In the spec §3, the "Last-tab-close → close-window" paragraph says SP-6a "rewires `onCloseLast → m.closeWindow(f)`". The implemented mechanism is indirect: `onCloseLast → f.window.Close()` (unchanged) → the manager's `startWindow` teardown observes the stop → `removeWindow`. Edit that paragraph to describe the actual mechanism (keep `onCloseLast` unchanged; the manager observes window closure). This keeps spec and code consistent.

- [ ] **Step 3: Append the execution record to this plan**

Add a section `## Execution record (2026-11-12, completed)` listing: the commit hashes (Tasks 1–8), any deviations (e.g. the `onCloseLast` indirect mechanism, the `bookmarks.Toggle`/`Contains` API used, placeholder-import removal), and the verification results (race green, full matrix green, parity 105/105 unchanged, mutation evidence). Note the verification limit: native interactive N=1 is compile-verified + structurally equivalent to the prior single-window orchestration; native multi-window visual sign-off belongs to SP-6b (no display).

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/goosie/*.go
git add docs/roadmap-v2.md docs/superpowers/specs/2026-11-12-sp6a-window-manager-design.md docs/superpowers/plans/2026-11-12-sp6a-window-manager.md
git commit -m "docs(sp6a): record the window-manager foundation complete, SP-6b next"
```

---

## Self-review notes

- **Spec coverage:** §2.1 owned manager → T2; §2.2 profile/per-window split + config copy → T1 (buildProfile/buildWindow) + T2 (spawn copies config, secondary `sessionActiveFile=""`); §2.3 N loops + native Runner handoff → T4 (`run`); §3 manager/spawn/closeWindow/quit-on-last → T2/T3/T4; §3 last-tab-close → T6; §4 headless harness → T2–T7 tests; §6 error/edge (spawn failure, teardown order, concurrent close, shared-store lifetime, no native regression) → T2 spawn error path, T4 teardown order, T3 idempotent removeWindow, T1/T5 bypass preserved; §7 testing (6 lifecycle tests + race + matrix + mutations) → T2–T8; §8 acceptance → T8/T9; §9 deferrals honored (no Cmd+N, no native shim, no session grouping).
- **Placeholder scan:** the Task 2 `var _ = ...` import placeholders are explicitly removed in Task 4 (called out in both tasks). No TBD/TODO. Every code step shows code. The `buildWindow` profile-local substitutions (`p.fonts`, `p.client`, `p.preferences`) are enumerated explicitly with line numbers; the bookmark API is the verified `Toggle`/`Contains` (no `Add`). Remaining NOTEs confirm only test-name patterns at run time.
- **Type consistency:** `windowManager` fields (`config`, `profile`, `mu`, `windows`, `primary`, `errs`, `ctx`, `cancel`, `sig`, `done`, `doneOnce`) used consistently across T2/T3/T4. `newWindowManager(c)`, `m.spawn(url) (*framePath, error)`, `m.startWindow(f) <-chan error`, `m.removeWindow(f)`, `m.closeWindow(f)`, `m.cleanup()`, `m.run() error`, `m.save()` — names match across tasks and tests. `buildProfile(c) (*profile, error)`, `buildWindow(c, p) (*framePath, error)`, `build(c)` wrapper, `profile.close()` — consistent T1→T2→T4.
- **Risk gates:** T1 is a pure refactor guarded by the full existing suite (7 build callers). T5 leaves paced/screenshot/private byte-for-byte unchanged. Native interactive N=1 is structurally equivalent to the prior orchestration (Runner handoff → wait → save → report) and compile-verified; the manager logic is proven on headless N=1 and N=2 under race.

---

## Execution record (2026-11-12, completed)

Executed inline (executing-plans), TDD red-green with mutation checks, all commits on `main`, **no push**.

### Commits (Tasks 1–8)

| Task | Commit | Subject |
|------|--------|---------|
| T1 | `e44fd150` | refactor(goosie): split build into shared profile + per-window machinery (SP-6a) |
| T2 | `fd3a4cb6` | feat(goosie): windowManager spawns N headless windows over a shared profile (SP-6a) |
| T3 | `46d5426a` | test(goosie): pin close-one-keeps-others and quit-on-last-window (SP-6a) |
| T4 | `7796a98c` | feat(goosie): windowManager.run drives N loops with one final save (SP-6a) |
| T5 | `fb1a2d5c` | feat(goosie): route interactive runs through the windowManager (SP-6a) |
| T6 | `0eac84b2` | test(goosie): last-tab-close closes its window, keeps the others (SP-6a) |
| — | `fa3bf2c2` | fix(deps): clear govulncheck — Go 1.26.11 + x/net v0.62.0 (mid-execution interruption) |
| T7 | `869c3bd7` | test(goosie): profile stores shared, tabs isolated per window (SP-6a) |
| T8 | (none) | race + full matrix + parity all green with no fix needed → no commit |
| T9 | (this) | docs(sp6a): record the window-manager foundation complete, SP-6b next |

### Deviations from the plan

- **T1 — `buildWindow` locals.** Added `var err error` (the old `err` came from the profile section that moved into `buildProfile`; `spec, err = sceneSpec(...)` must stay an assignment, not `:=`, to avoid shadowing `spec`). Added `c.downloadDir = p.preferences.DownloadDir` to preserve the original `build`'s mutation of `c.downloadDir` before it is stored on `f.config` (`p.preferences.DownloadDir == effectiveDir`). Removed the now-unused `path/filepath` import.
- **T2/T4 — Option B signal handling (fixes a plan bug).** The plan's per-teardown `select { <-stopped; <-m.sig }` distributes one shared signal channel across N teardowns, so a signal wakes only **one** window for N>1 and strands the rest, hanging `<-m.done`. Implemented instead: `run()` owns **one** signal goroutine `select { <-m.sig: m.cancel(); <-m.done: }`; `m.cancel()` cancels `m.ctx` → every per-window `wctx` child → every loop stops → every teardown runs → `done`. Each teardown waits **only** on `<-stopped`.
- **T4 — mutex snapshots.** `run()` snapshots `primary`/`errs` and `save()` snapshots `primary` under `m.mu` (reads included), so the registry is never read unguarded.
- **T7 — test strengthened after a surviving mutation.** The `bookmarks: nil` mutation initially **survived**: both `f1`/`f2` got nil → pointer equality held → the test's `t.Skip` guard fired and masked the regression. Changed `t.Skip("no bookmark store…")` → `t.Fatal("bookmark store is nil; a non-private config with a writable HOME must load one")`. Re-ran the mutation → **FAIL (caught)** → restored `bookmarks: p.bookmarks` → green. Bookmark API used is the verified `Toggle`/`Contains` (no `Add`).
- **Last-tab-close is indirect (spec §3 aligned in T9).** `onCloseLast → f.window.Close()` is unchanged; the `startWindow` teardown observes the loop stop → `removeWindow` → quit-on-last. The spec's §3/§5 and the roadmap were corrected to describe this.

### Mid-execution interruption — govulncheck (`fa3bf2c2`)

User request "fix govulncheck ./..." handled between T6 and T7:
- **Toolchain:** `go 1.26.11` (clears stdlib vulns; `GOTOOLCHAIN=auto`).
- **Deps:** `x/net v0.62.0`, `x/text v0.43.0` (direct), `x/sys v0.49.0` (indirect); `x/image v0.45.0` unchanged.
- **License gate 8:** re-audited the LICENSE text of each bumped module (all BSD-3-Clause) and amended `test/gatecheck/testdata/dependency-licenses.txt` **and** every per-target row in `licenses_test.go` (`x/net`/`x/text` on all 5 targets; `x/sys` on the 3 amd64 rows) in the same commit. `permittedLicenses` not relaxed.
- **Latent test race:** `test/js/fetch_cancel_test.go` `cancelTrackingFetcher.cancelled` `bool` → `atomic.Bool` (write on the fetch goroutine, read on the test goroutine, only a sleep between) — a test-harness race exposed by go1.26.11, not production code.

### Verification (T8)

- **Manager race:** `go test -race ./cmd/goosie/ -run TestManager -count=1` → `ok 2.009s`, no DATA RACE.
- **Full matrix (all exit 0):** `go test ./...` (71 pkgs ok), `go test -race ./...` (71 pkgs ok), `go vet -all ./...` (clean), `go test ./internal/archtest/` (ok), `go test ./test/gate -run TestGate` (ok), `go test ./test/qc/` (ok).
- **Parity:** `make parity` → **105/105 passing, 96.39% average, gaps 4 fixtures / 6 bands** — byte-identical to the SP-3/SP-4/SP-5 baseline (SP-6a touches no engine/layout/paint code).
- **Mutation evidence:** quit-on-last (T3), last-tab-close (T6), profile-sharing (T7, after strengthening) each caught red then restored green.

### Verification limit

Native interactive N=1 is **compile-verified** and structurally equivalent to the prior single-window orchestration (Runner handoff → wait → save → report); the manager logic is proven on headless N=1 and N=2 under race. Native multi-window **visual sign-off belongs to SP-6b** (no display), consistent with all prior native CUA work.
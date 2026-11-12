# SP-6b Native Multi-Window Shim + Cmd+N Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make native macOS multi-window function (one shared app delegate + main-thread-confined window registry + app-wide quit + retained window handle + order-out close + main-queue create trampoline) and wire Cmd+N / File→New Window through the SP-6a `windowManager`, so multi-window is a complete, user-reachable feature on both backends.

**Architecture:** Two layers, one vertical slice. **Layer B (Go wiring)** is headless-TDD: `framePath.newWindow` closure installed by `m.spawn`, registry-wide loop-error drain, `chromeWindow.onNewWindow` + Cmd+N split + painted New Window menu item + `new_window` context action, and `darwin.Open` calling the create trampoline. **Layer A (native C shim)** is compile-verified (no display here to create an `NSWindow`): a single shared `GoosieAppDelegate` owns a main-thread-only registry of `GoosieWindow *` and an `_appQuit` flag; `GoosieAppRun` turns until app-wide quit; `GoosieClose` orders the window out; `gw->window` is retained (fixes the latent dead file/save panels); `GoosieWindowCreateOnMain` marshals off-main creates onto the main queue. N=1 native behavior is preserved exactly.

**Tech Stack:** Go 1.x (cgo), Objective-C / AppKit / CoreVideo (`shim_darwin.m`), `internal/surface` loop, `internal/platform/{headless,darwin}`, `cmd/goosie` chrome. Tests: `go test` (headless), `go test -race`, `go vet -all`, `internal/archtest`, gate suite, `make parity`.

**Spec:** `docs/superpowers/specs/2026-11-12-sp6b-native-multiwindow-design.md` (approved, gates waived).

**Standing delegation:** autonomous execution, no approval gates; commits on `main`; **NO push**. Chat prose in Vietnamese; this plan and all code/docs in English.

---

## File Structure

| File | Responsibility | Change |
|------|----------------|--------|
| `cmd/goosie/main.go` | `framePath` struct + `openWindow` chrome wiring | Add `newWindow func()` field; wire `cw.onNewWindow = f.newWindow` |
| `cmd/goosie/window_manager.go` | Owns N windows over one profile | Install `f.newWindow` in `spawn`; move `errs` snapshot after `<-m.done`; add `drainLoopErrs` |
| `cmd/goosie/toolbar_window.go` | `chromeWindow` chrome + shortcuts + painted menu | Add `onNewWindow func()`; split Cmd+N / Cmd+Shift+N; add New Window menu item |
| `cmd/goosie/interaction.go` | Context/main-menu action dispatch | Add `case "new_window"` |
| `cmd/goosie/window_manager_test.go` | Headless manager lifecycle tests | Add 7 SP-6b tests + `errors`/`strings`/`surface` imports |
| `internal/platform/darwin/shim_darwin.m` | Native AppKit shim | Shared delegate + registry + app-wide quit + retain `gw->window` + order-out close + create trampoline + `g_appLoopRunning` |
| `internal/platform/darwin/shim.h` | C symbol declarations | Declare `GoosieWindowCreateOnMain`; update `GoosieAppRun`/`GoosieClose`/create docs |
| `internal/platform/darwin/window.go` | Go bridge | `Open` uses trampoline (drop off-main gate); `createErr` code 6; update `Run`/`Close` docs |
| `internal/platform/darwin/errors.go` | Named bridge errors | Add `ErrAppQuitting` |
| `docs/roadmap-v2.md`, spec, this plan | Records | SP-6 complete; execution records |

**Ordering rationale:** Go wiring first (Tasks 1–2) because it is fully headless-testable and gives red-green + mutation evidence. Native C next (Tasks 3–4), compile-verified. Then the full no-regression matrix (Task 5) and docs (Task 6). Each task compiles and commits on its own.

**Verification ceiling (honest):** No automated test here creates an `NSWindow` (needs `GoosieHasDisplay()`). The changed C functions are verified by `go build`/`go vet` (cgo darwin/arm64) + the existing native tests that link the shim + N=1 equivalence reasoning. The Go wiring gets full headless TDD. This matches SP-6a and all prior native work.

**One spec refinement (recorded in Task 6):** the spec said "no atomics, no C static" for the registry. The registry and `_appQuit` stay main-thread-confined ObjC ivars (no lock). The create trampoline additionally needs ONE off-main-readable signal so it never `dispatch_sync`s into a main queue that nothing drains once `GoosieAppRun` has returned (which would hang). That signal is a single file-scope `static _Atomic int g_appLoopRunning` in `shim_darwin.m`, set/cleared only on the main thread in `GoosieAppRun`. archtest scans `.go` files only (`internal/archtest/rules.go:467`), so a C atomic in the cgo-confined `.m` is allowed and introduces no mutable Go global.

---

## Task 1: Manager on-demand spawn link + registry-wide error drain

**Files:**
- Modify: `cmd/goosie/main.go` (framePath struct, ~line 130)
- Modify: `cmd/goosie/window_manager.go` (`spawn` ~59-90, `run` ~198-222, new helper)
- Test: `cmd/goosie/window_manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/goosie/window_manager_test.go`. First extend the import block (currently `os`, `testing`, `time`, `platform`) to add `errors`, `strings`, and `surface`:

```go
import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
)
```

Then append these three tests:

```go
func TestDrainLoopErrsCollectsAll(t *testing.T) {
	first := make(chan error, 1)
	first <- nil
	later := make(chan error, 1)
	later <- errors.New("window 2 loop failed")
	last := make(chan error, 1)
	last <- nil
	got := drainLoopErrs([]<-chan error{first, later, last})
	if got == nil || got.Error() != "window 2 loop failed" {
		t.Fatalf("drainLoopErrs = %v, want window 2 loop failed", got)
	}
}

func TestManagerSpawnInstallsNewWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	waitWindows(t, m, 1)
	if f1.newWindow == nil {
		t.Fatal("spawn did not install a newWindow hook")
	}
	f1.newWindow()
	waitWindows(t, m, 2)
}

func TestManagerCollectsLaterWindowError(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)

	runErr := make(chan error, 1)
	go func() { runErr <- m.run() }()

	// Wait for the primary window and its first presented frame. By then run()
	// has taken its errs snapshot (in the old, primary-only code) and is blocked
	// at <-m.done, so a channel appended now stands for a window spawned later.
	var primary *framePath
	deadline := time.Now().Add(3 * time.Second)
	presented := false
	for time.Now().Before(deadline) {
		m.mu.Lock()
		primary = m.primary
		m.mu.Unlock()
		if primary != nil && primary.loop != nil && primary.loop.Stats().Frames > 0 {
			presented = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !presented {
		t.Fatal("primary window never presented a frame")
	}

	laterErr := make(chan error, 1)
	laterErr <- errors.New("later window loop failed")
	m.mu.Lock()
	m.errs = append(m.errs, laterErr)
	m.mu.Unlock()

	m.closeWindow(primary)

	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "later window loop failed") {
			t.Fatalf("run() = %v, want the later window's loop error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after the primary closed")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/goosie/ -run 'TestDrainLoopErrsCollectsAll|TestManagerSpawnInstallsNewWindow|TestManagerCollectsLaterWindowError' -v > /tmp/sp6b-t1-red.log 2>&1; echo "exit=$?"`
Then read `/tmp/sp6b-t1-red.log`.
Expected: FAIL — `undefined: drainLoopErrs`, `f1.newWindow undefined (type *framePath has no field or method newWindow)`. (Compile failure is the red state for these new symbols.)

- [ ] **Step 3: Add the `newWindow` field to `framePath`**

In `cmd/goosie/main.go`, replace:

```go
	spec     paint.SceneSpec
	config   config
	// sizeMu guards config.width/height, the live content viewport. The chrome
```

with:

```go
	spec     paint.SceneSpec
	config   config
	// newWindow opens another window through the owning manager. spawn installs
	// it; the legacy paced/screenshot/private path leaves it nil, so Cmd+N and
	// the New Window menu item no-op there.
	newWindow func()
	// sizeMu guards config.width/height, the live content viewport. The chrome
```

- [ ] **Step 4: Install the hook in `spawn` and add the registry-wide drain**

In `cmd/goosie/window_manager.go`, replace:

```go
	f, err := buildWindow(c, m.profile)
	if err != nil {
		return nil, err
	}
	if err := f.openWindow(); err != nil {
		_ = f.pool.Close()
		return nil, err
	}
```

with:

```go
	f, err := buildWindow(c, m.profile)
	if err != nil {
		return nil, err
	}
	// Install before openWindow, which wires cw.onNewWindow = f.newWindow: the
	// closure captures only m, so a window opened later spawns through the same
	// manager. A nil hook (legacy path) means "no manager" and the shortcut no-ops.
	f.newWindow = func() {
		if _, err := m.spawn(""); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: new window: %v\n", err)
		}
	}
	if err := f.openWindow(); err != nil {
		_ = f.pool.Close()
		return nil, err
	}
```

Then replace the tail of `run` (the `errs` snapshot through the final return):

```go
	m.mu.Lock()
	primary := m.primary
	errs := m.errs
	m.mu.Unlock()
	if m.config.url == "" && primary.restoredActiveURL != "" {
		primary.navigateTabNoHistory(primary.restoredActiveURL)
	}

	if r, ok := primary.window.(platform.Runner); ok {
		r.Run()
	}
	<-m.done

	m.save()

	var firstErr error
	for _, ec := range errs {
		if err := <-ec; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("goosie: %w", firstErr)
	}
	return primary.report(nil)
}
```

with:

```go
	m.mu.Lock()
	primary := m.primary
	m.mu.Unlock()
	if m.config.url == "" && primary.restoredActiveURL != "" {
		primary.navigateTabNoHistory(primary.restoredActiveURL)
	}

	if r, ok := primary.window.(platform.Runner); ok {
		r.Run()
	}
	<-m.done

	m.save()

	// Snapshot errs only after every window is gone: a window spawned after the
	// primary (Cmd+N) appends its loop-error channel here, and draining a
	// primary-only snapshot would silently drop a later window's failure.
	m.mu.Lock()
	errs := m.errs
	m.mu.Unlock()
	if firstErr := drainLoopErrs(errs); firstErr != nil {
		return fmt.Errorf("goosie: %w", firstErr)
	}
	return primary.report(nil)
}

// drainLoopErrs receives every window's loop-exit error and returns the first
// non-nil one. Each channel is buffered and written exactly once by startWindow's
// loop goroutine before that window's teardown removes it, so once the manager is
// done every channel already holds its value and none blocks.
func drainLoopErrs(errs []<-chan error) error {
	var firstErr error
	for _, ec := range errs {
		if err := <-ec; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/goosie/ -run 'TestDrainLoopErrsCollectsAll|TestManagerSpawnInstallsNewWindow|TestManagerCollectsLaterWindowError' -v > /tmp/sp6b-t1-green.log 2>&1; echo "exit=$?"`
Then read `/tmp/sp6b-t1-green.log`.
Expected: `exit=0`, all three PASS.

- [ ] **Step 6: Mutation check (drain is registry-wide, not primary-only)**

Temporarily edit `drainLoopErrs` to read only the first channel:

```go
func drainLoopErrs(errs []<-chan error) error {
	if len(errs) == 0 {
		return nil
	}
	return <-errs[0]
}
```

Run: `go test ./cmd/goosie/ -run 'TestDrainLoopErrsCollectsAll|TestManagerCollectsLaterWindowError' -v > /tmp/sp6b-t1-mut.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t1-mut.log`. Expected: FAIL (both tests red — the later error is dropped).
Revert `drainLoopErrs` to the full-loop version from Step 4 (re-edit, do NOT `git checkout`).
Re-run the Step 5 command; expected `exit=0`.

- [ ] **Step 7: gofmt + commit**

```bash
gofmt -w cmd/goosie/main.go cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
go build -o /dev/null ./cmd/goosie/
git add cmd/goosie/main.go cmd/goosie/window_manager.go cmd/goosie/window_manager_test.go
git commit -m "$(cat <<'EOF'
feat(window): spawn windows on demand and drain every window's loop error

framePath gains a newWindow closure installed by windowManager.spawn before
openWindow wires it, and run() snapshots the loop-error channels only after the
last window is gone so a window opened after the primary cannot hide a failure.
EOF
)"
```

---

## Task 2: Cmd+N + File→New Window chrome wiring

**Files:**
- Modify: `cmd/goosie/toolbar_window.go` (struct ~33, `openMainMenu` ~198, `handleTabShortcut` ~1228)
- Modify: `cmd/goosie/interaction.go` (`handleContextMenuAction` ~915)
- Modify: `cmd/goosie/main.go` (`openWindow` ~761)
- Test: `cmd/goosie/window_manager_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/goosie/window_manager_test.go` (imports `surface` already added in Task 1):

```go
func TestManagerCmdNSpawnsWindow(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	tabsBefore := f1.tabMgr.Count()
	if !cw.handleTabShortcut('n', surface.ModCommand) {
		t.Fatal("Cmd+N was not handled")
	}
	waitWindows(t, m, 2)
	if f1.tabMgr.Count() != tabsBefore {
		t.Errorf("window 1 tab count = %d, want %d (Cmd+N must not touch window 1's tabs)",
			f1.tabMgr.Count(), tabsBefore)
	}
}

func TestManagerNewWindowMenuAction(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	cw.openMainMenu()
	snap := cw.mainMenu.Snapshot()
	found := false
	for _, it := range snap.Items {
		if it.Action == "new_window" {
			found = true
			if it.Label != "New Window" {
				t.Errorf("new_window item label = %q, want New Window", it.Label)
			}
		}
	}
	if !found {
		t.Fatal("main menu has no New Window item")
	}
	f1.handleContextMenuAction("new_window", "")
	waitWindows(t, m, 2)
}

func TestManagerCmdShiftNStillIncognito(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	before := f1.tabMgr.Count()
	if !cw.handleTabShortcut('n', surface.ModCommand|surface.ModShift) {
		t.Fatal("Cmd+Shift+N was not handled")
	}
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("window count = %d, want 1 (Cmd+Shift+N opens a tab, not a window)", n)
	}
	if f1.tabMgr.Count() != before+1 {
		t.Fatalf("tab count = %d, want %d", f1.tabMgr.Count(), before+1)
	}
	if active := f1.tabMgr.Active(); active == nil || !active.Incognito() {
		t.Error("Cmd+Shift+N did not open an incognito tab")
	}
}

func TestManagerNewWindowNilHookNoop(t *testing.T) {
	m, err := newWindowManager(testManagerConfig(t))
	if err != nil {
		t.Fatalf("newWindowManager: %v", err)
	}
	t.Cleanup(m.cleanup)
	f1, err := m.spawn("")
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	waitWindows(t, m, 1)
	cw, ok := f1.window.(*chromeWindow)
	if !ok {
		t.Fatal("primary window is not a chromeWindow")
	}
	// Simulate the legacy paced/screenshot/private path, which never installs a
	// manager link: both hooks nil, neither shortcut nor menu action spawns or
	// panics. No key events are injected, so the idle pump never reads onNewWindow
	// concurrently with these writes.
	f1.newWindow = nil
	cw.onNewWindow = nil
	if cw.handleTabShortcut('n', surface.ModCommand) {
		t.Error("Cmd+N was handled with a nil onNewWindow hook")
	}
	f1.handleContextMenuAction("new_window", "")
	m.mu.Lock()
	n := len(m.windows)
	m.mu.Unlock()
	if n != 1 {
		t.Fatalf("window count = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/goosie/ -run 'TestManagerCmdNSpawnsWindow|TestManagerNewWindowMenuAction|TestManagerCmdShiftNStillIncognito|TestManagerNewWindowNilHookNoop' -v > /tmp/sp6b-t2-red.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t2-red.log`.
Expected: FAIL — `cw.onNewWindow undefined (type *chromeWindow has no field or method onNewWindow)` (compile red).

- [ ] **Step 3: Add the `onNewWindow` field to `chromeWindow`**

In `cmd/goosie/toolbar_window.go`, replace:

```go
	onNewTab          func()
	onNewIncognitoTab func()
	onCloseTab        func(uint64)
```

with:

```go
	onNewTab          func()
	onNewIncognitoTab func()
	onNewWindow       func()
	onCloseTab        func(uint64)
```

- [ ] **Step 4: Split Cmd+N from Cmd+Shift+N in `handleTabShortcut`**

In `cmd/goosie/toolbar_window.go`, replace:

```go
	case (key == 'n' || key == 'N') && shift:
		if cw.onNewIncognitoTab != nil {
			cw.onNewIncognitoTab()
			return true
		}
	case key == 'p' || key == 'P':
```

with:

```go
	case (key == 'n' || key == 'N') && shift:
		if cw.onNewIncognitoTab != nil {
			cw.onNewIncognitoTab()
			return true
		}
	case (key == 'n' || key == 'N') && !shift:
		if cw.onNewWindow != nil {
			cw.onNewWindow()
			return true
		}
	case key == 'p' || key == 'P':
```

- [ ] **Step 5: Add the New Window item to the painted main menu**

In `cmd/goosie/toolbar_window.go`, replace:

```go
	items := []ContextMenuItem{
		{Label: "Bookmarks", Action: "bookmarks"},
```

with:

```go
	items := []ContextMenuItem{
		{Label: "New Window", Action: "new_window"},
		{Label: "Bookmarks", Action: "bookmarks"},
```

- [ ] **Step 6: Dispatch the `new_window` action**

In `cmd/goosie/interaction.go`, replace:

```go
	case "print_pdf":
		f.printActiveTabToPDF()
	}
}
```

with:

```go
	case "print_pdf":
		f.printActiveTabToPDF()
	case "new_window":
		if f.newWindow != nil {
			f.newWindow()
		}
	}
}
```

- [ ] **Step 7: Wire `cw.onNewWindow` in `openWindow`**

In `cmd/goosie/main.go`, replace:

```go
			cw.onNewIncognitoTab = func() { f.newIncognitoTab() }
			cw.onInspect = func() { f.navigateTab("goosie://inspect") }
```

with:

```go
			cw.onNewIncognitoTab = func() { f.newIncognitoTab() }
			cw.onNewWindow = f.newWindow
			cw.onInspect = func() { f.navigateTab("goosie://inspect") }
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./cmd/goosie/ -run 'TestManagerCmdNSpawnsWindow|TestManagerNewWindowMenuAction|TestManagerCmdShiftNStillIncognito|TestManagerNewWindowNilHookNoop' -v > /tmp/sp6b-t2-green.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t2-green.log`.
Expected: `exit=0`, all four PASS.

- [ ] **Step 9: Mutation check (Cmd+N really spawns; incognito split intact)**

Temporarily make the new Cmd+N case a no-op by replacing the Step 4 added case body:

```go
	case (key == 'n' || key == 'N') && !shift:
		if cw.onNewWindow != nil {
			return true
		}
```

Run: `go test ./cmd/goosie/ -run TestManagerCmdNSpawnsWindow -v > /tmp/sp6b-t2-mut.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t2-mut.log`. Expected: FAIL (`waitWindows` times out at 1 window).
Revert to the Step 4 version (re-edit). Re-run Step 8; expected `exit=0`.

- [ ] **Step 10: Run the whole manager suite + gofmt + commit**

Run: `go test ./cmd/goosie/ -run TestManager -v > /tmp/sp6b-t2-suite.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t2-suite.log`. Expected: `exit=0` — all SP-6a lifecycle tests plus the seven SP-6b tests PASS.

```bash
gofmt -w cmd/goosie/toolbar_window.go cmd/goosie/interaction.go cmd/goosie/main.go cmd/goosie/window_manager_test.go
go build -o /dev/null ./cmd/goosie/
git add cmd/goosie/toolbar_window.go cmd/goosie/interaction.go cmd/goosie/main.go cmd/goosie/window_manager_test.go
git commit -m "$(cat <<'EOF'
feat(chrome): open a new window with Cmd+N and File > New Window

Plain Cmd+N now spawns a window through the manager while Cmd+Shift+N still opens
an incognito tab; the painted main menu gains a New Window item routed through the
same hook, and a nil hook (legacy path) no-ops.
EOF
)"
```

---

## Task 3: Native shim — shared delegate, registry, app-wide quit, retained window, order-out close

**Files:**
- Modify: `internal/platform/darwin/shim_darwin.m` (`GoosieAppDelegate` impl ~918-948, `windowWillClose:` ~914, `GoosieWindowCreate` ~1044-1141, `GoosieAppRun` ~1143-1171, `GoosieClose` ~1470-1474)

**Note:** compile-verified only (no display). Each step keeps the shim compiling. `stdatomic.h` is already included (`:40`); `kGoosieIdlePoll` is `:68`.

- [ ] **Step 1: Replace the per-window app delegate with a shared registry delegate**

In `internal/platform/darwin/shim_darwin.m`, replace the entire `GoosieAppDelegate` implementation:

```objc
@implementation GoosieAppDelegate {
	GoosieWindow *_gw;
}

- (instancetype)initWithWindow:(GoosieWindow *)gw {
	if ((self = [super init])) _gw = gw;
	return self;
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return YES; }

- (void)applicationWillFinishLaunching:(NSNotification *)notification {
	// A menu bar with a working Quit item, because ⌘Q is how a person stops a GUI
	// program and an app with no menu has no key equivalent to answer it with.
	NSMenu *bar = [[NSMenu alloc] init];
	NSMenuItem *item = [bar addItemWithTitle:@"Goosie" action:NULL keyEquivalent:@""];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:@"Quit Goosie"
                       action:@selector(terminate:)
                keyEquivalent:@"q"];
	[item setSubmenu:appMenu];
	[NSApp setMainMenu:bar];
	[appMenu release];
	[bar release];
}

- (void)applicationWillTerminate:(NSNotification *)notification {
	if (_gw) pushQuit(_gw);
}

@end
```

with:

```objc
// One shared delegate owns every window. The registry and _appQuit are confined
// to the main thread: register (create, via the trampoline), unregister
// (windowWillClose:), quitAll (terminate:) and appQuit reads (GoosieAppRun) all
// happen there, so no lock, atomic, or C static is needed for them.
@implementation GoosieAppDelegate {
	GoosieWindow **_windows;
	int _count;
	int _cap;
	int _appQuit;
}

- (instancetype)init {
	if ((self = [super init])) {
		_windows = NULL;
		_count = 0;
		_cap = 0;
		_appQuit = 0;
	}
	return self;
}

- (void)registerWindow:(GoosieWindow *)gw {
	if (!gw) return;
	for (int i = 0; i < _count; i++) {
		if (_windows[i] == gw) return;  // idempotent
	}
	if (_count == _cap) {
		int newCap = _cap ? _cap * 2 : 4;
		GoosieWindow **grown = (GoosieWindow **)realloc(_windows, sizeof(GoosieWindow *) * newCap);
		if (!grown) return;  // OOM: window still draws; quitAll may miss it (rare, fatal-ish)
		_windows = grown;
		_cap = newCap;
	}
	_windows[_count++] = gw;
}

- (void)unregisterWindow:(GoosieWindow *)gw {
	if (!gw) return;
	int found = 0;
	for (int i = 0; i < _count; i++) {
		if (_windows[i] == gw) {
			_windows[i] = _windows[_count - 1];
			_count--;
			found = 1;
			break;
		}
	}
	// Only a real removal that empties the registry quits the app: a failed
	// create's windowWillClose: (gw never registered) must not quit a live app.
	if (found && _count == 0) _appQuit = 1;
}

- (void)quitAll {
	for (int i = 0; i < _count; i++) pushQuit(_windows[i]);
	_appQuit = 1;
}

- (int)appQuit { return _appQuit; }

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender { return YES; }

- (void)applicationWillFinishLaunching:(NSNotification *)notification {
	// A menu bar with a working Quit item, because ⌘Q is how a person stops a GUI
	// program and an app with no menu has no key equivalent to answer it with.
	NSMenu *bar = [[NSMenu alloc] init];
	NSMenuItem *item = [bar addItemWithTitle:@"Goosie" action:NULL keyEquivalent:@""];
	NSMenu *appMenu = [[NSMenu alloc] init];
	[appMenu addItemWithTitle:@"Quit Goosie"
                       action:@selector(terminate:)
                keyEquivalent:@"q"];
	[item setSubmenu:appMenu];
	[NSApp setMainMenu:bar];
	[appMenu release];
	[bar release];
}

- (void)applicationWillTerminate:(NSNotification *)notification {
	[self quitAll];
}

@end
```

- [ ] **Step 2: Unregister on window close**

In `internal/platform/darwin/shim_darwin.m`, replace:

```objc
- (void)windowWillClose:(NSNotification *)note { pushQuit(_gw); }
```

with:

```objc
- (void)windowWillClose:(NSNotification *)note {
	pushQuit(_gw);
	[_gw->appDel unregisterWindow:_gw];
}
```

(`windowShouldClose:` is unchanged: pushQuit, return YES.)

- [ ] **Step 3: Share the delegate, retain `gw->window`, register, and add the app-quitting code**

In `internal/platform/darwin/shim_darwin.m`, replace:

```objc
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		gw->appDel = [[GoosieAppDelegate alloc] initWithWindow:gw];
		[app setDelegate:gw->appDel];
```

with:

```objc
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		// One delegate for the whole app: created on the first window, reused by
		// every later one. NSApplication.delegate is assign, so the alloc's +1 is
		// the process-lifetime retain (the shim never frees gw or its delegate).
		GoosieAppDelegate *shared = (GoosieAppDelegate *)[app delegate];
		if (![shared isKindOfClass:[GoosieAppDelegate class]]) {
			shared = [[GoosieAppDelegate alloc] init];
			[app setDelegate:shared];
		}
		gw->appDel = shared;
		if ([shared appQuit]) {
			// The app is already quitting; do not build a window into a dying app.
			free(gw);
			if (err) *err = 6;
			return NULL;
		}
```

Then replace:

```objc
		[win setReleasedWhenClosed:NO];
		gw->winDel = [[GoosieWindowDelegate alloc] initWithWindow:gw];
```

with:

```objc
		[win setReleasedWhenClosed:NO];
		// Retain the handle for the process: GoosieClose orders it out and the
		// file/save panels attach to it. Without this it stayed NULL and both
		// panels bailed with -1 (a latent bug), and close had nothing to order out.
		gw->window = win;
		gw->winDel = [[GoosieWindowDelegate alloc] initWithWindow:gw];
```

Then replace the end of the create (the display-link failure block and return):

```objc
		if (!startDisplayLink(gw)) {
			// The window is on screen and its view holds a pointer to gw, so gw is left
			// allocated rather than freed here: freeing it would be a use-after-free the
			// next click could reach. A create that fails is rare, fatal to the run, and
			// costs one struct and the objects above, which is what GoosieClose's note about
			// teardown already accepts for a window that closes.
			[win close];
			if (err) *err = 5;  // a display, but no display link to pace it with
			return NULL;
		}
	}
	return gw;
}
```

with:

```objc
		if (!startDisplayLink(gw)) {
			// The window is on screen and its view holds a pointer to gw, so gw is left
			// allocated rather than freed here: freeing it would be a use-after-free the
			// next click could reach. A create that fails is rare, fatal to the run, and
			// costs one struct and the objects above, which is what GoosieClose's note about
			// teardown already accepts for a window that closes.
			[win close];
			if (err) *err = 5;  // a display, but no display link to pace it with
			return NULL;
		}
		// Register only a fully built window. The failed-create [win close] above
		// fires windowWillClose: -> unregisterWindow:, which is a no-op for a gw
		// that was never registered (and cannot quit a live app).
		[shared registerWindow:gw];
	}
	return gw;
}
```

- [ ] **Step 4: Turn the run loop until app-wide quit**

In `internal/platform/darwin/shim_darwin.m`, replace the whole `GoosieAppRun`:

```objc
void GoosieAppRun(GoosieWindow *gw) {
	if (!gw) return;
	@autoreleasepool { [NSApp finishLaunching]; }
	// The loop is turned by hand rather than by -[NSApplication run], for one reason:
	// -run notices -stop: only at the top of an iteration it decides to take, and an app
	// with no pending input sits in a kernel message wait indefinitely. A run that can
	// end when the user moves the mouse is not a run a scripted gate can finish, so the
	// quit flag is checked here on a bounded wait instead. The cost is one wake-up per
	// kGoosieIdlePoll while idle; frame pacing comes from the display link, not from
	// this loop, so nothing about a drawn frame changes.
	for (;;) {
		@autoreleasepool {
			pthread_mutex_lock(&gw->mu);
			int quit = gw->quit;
			pthread_mutex_unlock(&gw->mu);
			if (quit) break;
			NSEvent *e = [NSApp nextEventMatchingMask:NSEventMaskAny
									untilDate:[NSDate dateWithTimeIntervalSinceNow:kGoosieIdlePoll]
										inMode:NSDefaultRunLoopMode
									   dequeue:YES];
			if (e) [NSApp sendEvent:e];
		}
	}
	// Nothing is coming after this. The loop is how a queued commit reaches the layer
	// and how a resize is noticed, and it has stopped turning, so a pump still waiting
	// in GoosieNextEvent has to be woken rather than left in a condwait nobody will
	// signal.
	pushQuit(gw);
}
```

with:

```objc
void GoosieAppRun(GoosieWindow *gw) {
	if (!gw) return;
	@autoreleasepool { [NSApp finishLaunching]; }
	// The loop is turned by hand rather than by -[NSApplication run], for one reason:
	// -run notices -stop: only at the top of an iteration it decides to take, and an app
	// with no pending input sits in a kernel message wait indefinitely. A run that can
	// end when the user moves the mouse is not a run a scripted gate can finish, so the
	// quit flag is checked here on a bounded wait instead. The cost is one wake-up per
	// kGoosieIdlePoll while idle; frame pacing comes from the display link, not from
	// this loop, so nothing about a drawn frame changes.
	//
	// The flag is the shared delegate's app-wide appQuit, not this window's quit:
	// closing one window of several must leave the loop turning for the others. With
	// one window its close empties the registry -> appQuit, exactly as before.
	for (;;) {
		@autoreleasepool {
			if ([gw->appDel appQuit]) break;
			NSEvent *e = [NSApp nextEventMatchingMask:NSEventMaskAny
									untilDate:[NSDate dateWithTimeIntervalSinceNow:kGoosieIdlePoll]
										inMode:NSDefaultRunLoopMode
									   dequeue:YES];
			if (e) [NSApp sendEvent:e];
		}
	}
	// Nothing is coming after this. The loop is how a queued commit reaches the layer
	// and how a resize is noticed, and it has stopped turning, so a pump still waiting
	// in GoosieNextEvent has to be woken rather than left in a condwait nobody will
	// signal.
	pushQuit(gw);
}
```

- [ ] **Step 5: Order the window out on close**

In `internal/platform/darwin/shim_darwin.m`, replace:

```objc
void GoosieClose(GoosieWindow *gw) {
	if (!gw) return;
	stopDisplayLink(gw);
	pushQuit(gw);
}
```

with:

```objc
void GoosieClose(GoosieWindow *gw) {
	if (!gw) return;
	stopDisplayLink(gw);
	pushQuit(gw);
	// Order the window out on the main thread (Close is called off-main). [win close]
	// fires windowWillClose: -> unregisterWindow:, which empties the registry and sets
	// appQuit when this was the last window, ending GoosieAppRun. setReleasedWhenClosed:NO
	// means close orders out without freeing, honoring the no-GoosieWindowFree contract.
	// gw is never freed, so the captured pointer stays valid; dispatch_async never blocks
	// the caller. unregisterWindow: is idempotent, so the close button's windowWillClose:
	// and this dispatched close cannot double-decrement.
	dispatch_async(dispatch_get_main_queue(), ^{
		if (gw->window) [gw->window close];
	});
}
```

- [ ] **Step 6: Compile, vet, and run the existing native tests**

Run: `go build -o /dev/null ./cmd/goosie/ > /tmp/sp6b-t3-build.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t3-build.log`. Expected: `exit=0` (cgo darwin/arm64 compiles the shim).

Run: `go vet ./internal/platform/darwin/ > /tmp/sp6b-t3-vet.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t3-vet.log`. Expected: `exit=0`.

Run: `go test ./internal/platform/darwin/ > /tmp/sp6b-t3-native.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t3-native.log`. Expected: `exit=0` (media/accept/AX/translate link the shim; a compile/type error would break them).

- [ ] **Step 7: Commit**

```bash
git add internal/platform/darwin/shim_darwin.m
git commit -m "$(cat <<'EOF'
feat(darwin): one shared app delegate with a window registry and app-wide quit

The shim now shares a single delegate that tracks every live window on the main
thread, turns GoosieAppRun until the last window closes, retains gw->window (which
also revives the file/save panels), and orders the window out on GoosieClose. N=1
behavior is unchanged; a second native window no longer ends the app loop.
EOF
)"
```

---

## Task 4: Native create trampoline + Go bridge

**Files:**
- Modify: `internal/platform/darwin/shim.h` (declare `GoosieWindowCreateOnMain`; doc updates ~123-145, ~188-198)
- Modify: `internal/platform/darwin/shim_darwin.m` (`g_appLoopRunning` + stores in `GoosieAppRun`; `GoosieWindowCreateOnMain` after `GoosieWindowCreate`)
- Modify: `internal/platform/darwin/errors.go` (add `ErrAppQuitting`)
- Modify: `internal/platform/darwin/window.go` (`Open` ~77-104, `createErr` ~109-125, `Run`/`Close` docs ~355-382)

- [ ] **Step 1: Declare the trampoline and update the C docs**

In `internal/platform/darwin/shim.h`, replace:

```c
// GoosieWindowCreate builds the application object, the window, and its display
// link, and shows the window. It must be called on the process's main thread -
// AppKit requires it and says so by returning NULL with *err set rather than by
// letting the call abort the process. w and h are device pixels; the window's
// frame is derived from them under the scale the backing store reports.
GoosieWindow *GoosieWindowCreate(const char *title, int w, int h, int *err);
```

with:

```c
// GoosieWindowCreate builds the application object, the window, and its display
// link, and shows the window. It must be called on the process's main thread -
// AppKit requires it and says so by returning NULL with *err set rather than by
// letting the call abort the process. w and h are device pixels; the window's
// frame is derived from them under the scale the backing store reports.
// *err codes: 1 not main thread, 2 bad size, 3 out of memory, 4 no display,
// 5 no display link, 6 the app is quitting (no window can be opened).
GoosieWindow *GoosieWindowCreate(const char *title, int w, int h, int *err);

// GoosieWindowCreateOnMain is the any-thread entry point. On the main thread it
// calls GoosieWindowCreate directly; off the main thread it marshals the create
// onto the main queue with dispatch_sync, which requires GoosieAppRun to be
// turning (it drains the queue). If the app loop is not running it returns NULL
// with *err 6 rather than dispatching into a queue nothing drains (which would
// hang). This is how Cmd+N, handled on the chrome pump goroutine, opens window #2
// while the main thread is inside Run.
GoosieWindow *GoosieWindowCreateOnMain(const char *title, int w, int h, int *err);
```

Then replace the `GoosieAppRun` doc:

```c
// GoosieAppRun turns the main event loop until the window is closed, by GoosieClose or
// by the close button. Call it on the main thread, from the goroutine that created the
// window and stays pinned to that thread. It returns on its own once the quit flag is
// set; nothing here needs a pending event to notice one.
void GoosieAppRun(GoosieWindow *gw);
```

with:

```c
// GoosieAppRun turns the main event loop until the app quits - that is, until the
// last window closes (by GoosieClose or the close button) or terminate: fires.
// Call it on the main thread, from the goroutine that created the first window and
// stays pinned to that thread. It returns on its own once the shared delegate's
// app-wide quit flag is set; nothing here needs a pending event to notice one.
// While it turns it drains the main dispatch queue, which is what lets the create
// trampoline and GoosieClose's order-out reach AppKit from other threads.
void GoosieAppRun(GoosieWindow *gw);
```

Then replace the `GoosieClose` doc:

```c
// GoosieClose stops the display link, asks the run loop to end, and wakes a
// GoosieNextEvent that is waiting. It is safe from any thread and safe twice.
```

with:

```c
// GoosieClose stops the display link, wakes a GoosieNextEvent that is waiting, and
// orders this window out on the main queue. The app run loop ends when the LAST
// window closes, not this one. It is safe from any thread and safe twice.
```

- [ ] **Step 2: Add the loop-running signal and the trampoline**

In `internal/platform/darwin/shim_darwin.m`, add the file-scope signal immediately before `GoosieWindowCreate` (just after `int GoosieHasDisplay(void) { return CGMainDisplayID() != 0; }`):

```objc
int GoosieHasDisplay(void) { return CGMainDisplayID() != 0; }

// g_appLoopRunning is 1 while GoosieAppRun is turning the main loop and 0 otherwise.
// It is the one off-main-readable signal in the shim: GoosieWindowCreateOnMain reads
// it to avoid dispatch_sync-ing into a main queue nothing drains once the loop has
// returned (which would hang). Set and cleared only on the main thread. The window
// registry and appQuit stay main-thread-confined ObjC ivars; this is just the loop's
// liveness, which an off-main creator must be able to see.
static _Atomic int g_appLoopRunning = 0;

GoosieWindow *GoosieWindowCreate(const char *title, int w, int h, int *err) {
```

Then add the trampoline immediately after `GoosieWindowCreate`'s closing brace (before `void GoosieAppRun(GoosieWindow *gw) {`):

```objc
	return gw;
}

GoosieWindow *GoosieWindowCreateOnMain(const char *title, int w, int h, int *err) {
	if (err) *err = 0;
	if (pthread_main_np()) {
		return GoosieWindowCreate(title, w, h, err);
	}
	if (!atomic_load(&g_appLoopRunning)) {
		// The main loop is not turning: a dispatch_sync would never be drained.
		if (err) *err = 6;
		return NULL;
	}
	__block GoosieWindow *result = NULL;
	__block int code = 0;
	dispatch_sync(dispatch_get_main_queue(), ^{
		result = GoosieWindowCreate(title, w, h, &code);
	});
	if (err) *err = code;
	return result;
}

void GoosieAppRun(GoosieWindow *gw) {
```

Then set/clear the signal in `GoosieAppRun`. Replace:

```objc
void GoosieAppRun(GoosieWindow *gw) {
	if (!gw) return;
	@autoreleasepool { [NSApp finishLaunching]; }
```

with:

```objc
void GoosieAppRun(GoosieWindow *gw) {
	if (!gw) return;
	atomic_store(&g_appLoopRunning, 1);
	@autoreleasepool { [NSApp finishLaunching]; }
```

and replace the tail:

```objc
	// Nothing is coming after this. The loop is how a queued commit reaches the layer
	// and how a resize is noticed, and it has stopped turning, so a pump still waiting
	// in GoosieNextEvent has to be woken rather than left in a condwait nobody will
	// signal.
	pushQuit(gw);
}
```

with:

```objc
	// Nothing is coming after this. The loop is how a queued commit reaches the layer
	// and how a resize is noticed, and it has stopped turning, so a pump still waiting
	// in GoosieNextEvent has to be woken rather than left in a condwait nobody will
	// signal. Clear the liveness flag first so a late off-main create returns code 6
	// instead of dispatching into a queue this loop no longer drains.
	atomic_store(&g_appLoopRunning, 0);
	pushQuit(gw);
}
```

- [ ] **Step 3: Add `ErrAppQuitting`**

In `internal/platform/darwin/errors.go`, replace:

```go
	// ErrNoDisplayLink reports a display but no way to be paced by it, which is a
	// window that would never redraw.
	ErrNoDisplayLink = errors.New("platform/darwin: the display cannot be paced")
)
```

with:

```go
	// ErrNoDisplayLink reports a display but no way to be paced by it, which is a
	// window that would never redraw.
	ErrNoDisplayLink = errors.New("platform/darwin: the display cannot be paced")
	// ErrAppQuitting reports a window create attempted while the app is shutting
	// down: the shared delegate has already quit, or the main run loop is no longer
	// turning to marshal an off-main create. The fix is to not open windows during
	// teardown, not to change the arguments.
	ErrAppQuitting = errors.New("platform/darwin: the app is quitting; no new window can be opened")
)
```

- [ ] **Step 4: Route `Open` through the trampoline and decode code 6**

In `internal/platform/darwin/window.go`, replace the `Open` doc + gate + create call:

```go
// Open creates the window. It must run on the process's main thread, on a goroutine
// pinned there with runtime.LockOSThread; anything else is ErrNotMainThread rather
// than the abort AppKit would otherwise raise.
func Open(cfg Config) (*Window, error) {
	if C.GoosieIsMainThread() == 0 {
		return nil, ErrNotMainThread
	}
	if cfg.Size.W <= 0 || cfg.Size.H <= 0 {
		return nil, fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	}
	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))

	var cerr C.int
	gw := C.GoosieWindowCreate(title, C.int(cfg.Size.W), C.int(cfg.Size.H), &cerr)
	if gw == nil {
		return nil, createErr(int(cerr), cfg)
	}
```

with:

```go
// Open creates the window. It may be called from any thread: the first window is
// opened on the main thread (pinned with runtime.LockOSThread) before Run, and a
// later window is opened off-main while Run turns - the shim's create trampoline
// marshals that onto the main queue. An off-main create with no running app loop
// returns ErrAppQuitting rather than hanging or aborting.
func Open(cfg Config) (*Window, error) {
	if cfg.Size.W <= 0 || cfg.Size.H <= 0 {
		return nil, fmt.Errorf("platform/darwin: a window needs a positive size, got %dx%d",
			cfg.Size.W, cfg.Size.H)
	}
	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))

	var cerr C.int
	gw := C.GoosieWindowCreateOnMain(title, C.int(cfg.Size.W), C.int(cfg.Size.H), &cerr)
	if gw == nil {
		return nil, createErr(int(cerr), cfg)
	}
```

Then add the code-6 mapping in `createErr`. Replace:

```go
	case 5:
		return ErrNoDisplayLink
	}
	return fmt.Errorf("platform/darwin: window create failed with code %d", code)
```

with:

```go
	case 5:
		return ErrNoDisplayLink
	case 6:
		return ErrAppQuitting
	}
	return fmt.Errorf("platform/darwin: window create failed with code %d", code)
```

- [ ] **Step 5: Update the `Run`/`Close` doc comments for app-wide semantics**

In `internal/platform/darwin/window.go`, replace:

```go
// Run enters AppKit's run loop and does not return until the window is closed.
//
// It belongs on the main thread, on the goroutine that called Open: the alternative is
// a second thread owning AppKit while the first one sits idle, which is not a shape
// NSApplication supports. A backend with no run loop does not have this method, and
// cmd/goosie finds out by asking for platform.Runner.
func (w *Window) Run() {
```

with:

```go
// Run enters AppKit's run loop and does not return until the app quits - that is,
// until the last window closes. Closing this window alone leaves Run turning for
// the others.
//
// It belongs on the main thread, on the goroutine that called Open: the alternative is
// a second thread owning AppKit while the first one sits idle, which is not a shape
// NSApplication supports. A backend with no run loop does not have this method, and
// cmd/goosie finds out by asking for platform.Runner.
func (w *Window) Run() {
```

Then replace:

```go
// Close stops the display link, ends the run loop, and waits for the pump to leave.
// It is idempotent, and it is safe to call from a goroutine that is not the main
// thread - which is the only way to call it while Run is turning the loop.
```

with:

```go
// Close stops the display link, orders this window out, and waits for the pump to
// leave. The app run loop ends when the LAST window closes, not this one. It is
// idempotent, and it is safe to call from a goroutine that is not the main thread -
// which is the only way to call it while Run is turning the loop.
```

- [ ] **Step 6: Compile, vet, and run the existing native tests**

Run: `go build -o /dev/null ./cmd/goosie/ > /tmp/sp6b-t4-build.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t4-build.log`. Expected: `exit=0`.

Run: `go vet ./internal/platform/darwin/ > /tmp/sp6b-t4-vet.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t4-vet.log`. Expected: `exit=0`.

Run: `go test ./internal/platform/darwin/ > /tmp/sp6b-t4-native.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t4-native.log`. Expected: `exit=0`.

Run: `go test ./test/engine/ -run Media > /tmp/sp6b-t4-media.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t4-media.log`. Expected: `exit=0` (native media TestMain links the shim).

- [ ] **Step 7: Commit**

```bash
git add internal/platform/darwin/shim.h internal/platform/darwin/shim_darwin.m internal/platform/darwin/errors.go internal/platform/darwin/window.go
git commit -m "$(cat <<'EOF'
feat(darwin): marshal off-main window creates onto the main queue

GoosieWindowCreateOnMain lets the chrome pump goroutine open window #2 while Run
turns, guarded by a loop-liveness flag so a create racing app-quit returns
ErrAppQuitting instead of hanging. darwin.Open uses it and decodes the new code 6;
Run/Close docs now describe app-wide quit and per-window close.
EOF
)"
```

---

## Task 5: Full no-regression matrix

**Files:** none (verification only; fix-forward if anything is red).

- [ ] **Step 1: Build + vet everything**

Run: `go build -o /dev/null ./... > /tmp/sp6b-t5-build.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-build.log`. Expected: `exit=0`.
Run: `go vet -all ./... > /tmp/sp6b-t5-vet.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-vet.log`. Expected: `exit=0`.

- [ ] **Step 2: Architecture rules (cgo confined, no new Go global, gofmt)**

Run: `go test ./internal/archtest/ > /tmp/sp6b-t5-arch.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-arch.log`. Expected: `exit=0`. (The C atomic lives in the cgo-confined `.m`; archtest scans `.go` only, so no mutable-Go-global violation.)

- [ ] **Step 3: Full test suite**

Run: `go test ./... > /tmp/sp6b-t5-all.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-all.log`. Expected: `exit=0`, all packages ok.

- [ ] **Step 4: Race detector**

Run: `go test -race ./cmd/goosie/ ./internal/platform/... ./internal/surface/ > /tmp/sp6b-t5-race.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-race.log`. Expected: `exit=0`, no DATA RACE. (The manager tests exercise spawn/close/teardown concurrency; the new nil-hook test writes hooks only while the pump is idle.)

- [ ] **Step 5: Gate suite**

Run: `go test ./test/gate -run TestGate -v > /tmp/sp6b-t5-gate.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-gate.log`. Expected: `exit=0`. (No duration assertions added; SP-6b touches no engine/layout/paint code.)

- [ ] **Step 6: Render parity (must be byte-identical)**

Run: `make parity > /tmp/sp6b-t5-parity.log 2>&1; echo "exit=$?"`
Read `/tmp/sp6b-t5-parity.log`. Expected: `exit=0`, 105/105 fixtures, score ≥ 96.39%, gaps 4/6 — unchanged from SP-6a (SP-6b touches no engine/layout/paint code).

- [ ] **Step 7: Commit any fix-forward (only if a step above required a change)**

If every step was green on first run, there is nothing to commit — record that in Task 6's execution record. If a fix was needed:

```bash
gofmt -w <changed files>
git add <changed files>
git commit -m "fix(sp6b): <what the matrix caught and why>"
```

---

## Task 6: Docs — roadmap SP-6 complete + execution records

**Files:**
- Modify: `docs/roadmap-v2.md` (multi-window bullet, ~line 210)
- Modify: `docs/superpowers/specs/2026-11-12-sp6b-native-multiwindow-design.md` (append execution record + the one-atomic refinement)
- Modify: `docs/superpowers/plans/2026-11-12-sp6b-native-multiwindow.md` (append execution record)

- [ ] **Step 1: Record SP-6 complete in the roadmap**

In `docs/roadmap-v2.md`, find the multi-window bullet (the line SP-6a marked "đang làm", ~line 210). Read it first (`Read` with offset/limit — the full file exceeds the Read token cap). Replace its status with a completed record: SP-6 multi-window shipped — SP-6a Go `windowManager` (shared profile, N loops, quit-on-last) + SP-6b native shim (one shared delegate + main-thread registry + app-wide quit + retained `gw->window` reviving file/save panels + order-out close + main-queue create trampoline) + Cmd+N / File→New Window. Headless N=1/N=2 + race green; native compile-verified (no display); parity 105/105 unchanged. Next: SP-7 (evidence-driven engine features). Keep the surrounding Vietnamese prose style consistent with the file.

- [ ] **Step 2: Append the spec execution record + refinement**

Append to `docs/superpowers/specs/2026-11-12-sp6b-native-multiwindow-design.md`:

```markdown
## 8. Execution record (2026-11-12, completed)

Implemented Tasks 1–6 of `docs/superpowers/plans/2026-11-12-sp6b-native-multiwindow.md` on `main`, no push.

**Refinement to §2.1 (one atomic):** the registry and `_appQuit` remain main-thread-confined ObjC ivars (no lock). The create trampoline additionally reads one file-scope `static _Atomic int g_appLoopRunning` (set/cleared only on main in `GoosieAppRun`) so an off-main create never `dispatch_sync`s into a main queue that nothing drains once the loop has returned. archtest scans `.go` only, so this C atomic in the cgo-confined `.m` adds no mutable Go global. Residual narrow TOCTOU (loop exits between the flag read and the dispatch) is accepted: appQuit implies every window is closing and every pump detaching, so a late Cmd+N is already unlikely; the flag is the belt-and-braces.

**Commits:** <fill from git log: Task 1 manager link+drain, Task 2 chrome Cmd+N/menu, Task 3 shared delegate/registry/quit/retain/close, Task 4 trampoline/bridge, Task 5 fix-forward if any, Task 6 docs>.

**Verification:** headless TDD green + mutation evidence (drain primary-only mutant red; Cmd+N no-op mutant red); `go build ./...`, `go vet -all ./...`, `go test ./...`, `go test -race` (cmd/goosie + platform + surface), archtest, gate, `make parity` 105/105 ≥96.39% gaps 4/6 — all green. Native: `go build`/`go vet ./internal/platform/darwin/` + existing native tests (media/accept/AX/translate) green.

**Verification limit:** no automated test creates an NSWindow (needs GoosieHasDisplay()); the changed C functions are compile-verified + N=1 equivalence reasoned, not runtime-exercised. Native multi-window visual sign-off deferred (no display), consistent with SP-6a and all prior native work.
```

- [ ] **Step 3: Append the plan execution record**

Append to `docs/superpowers/plans/2026-11-12-sp6b-native-multiwindow.md` a `## Execution record (2026-11-12, completed)` section: the commit table (Task → hash → one-line result), any deviations from the plan (e.g. the one-atomic refinement, the `found`-guard in `unregisterWindow:` so a failed create cannot quit a live app), the verification summary, and the verification limit. Fill hashes from `git log --oneline`.

- [ ] **Step 4: Commit the docs**

```bash
git add docs/roadmap-v2.md docs/superpowers/specs/2026-11-12-sp6b-native-multiwindow-design.md docs/superpowers/plans/2026-11-12-sp6b-native-multiwindow.md
git commit -m "$(cat <<'EOF'
docs(sp6): record multi-window shipped (SP-6a Go manager + SP-6b native shim + Cmd+N)

Roadmap marks SP-6 complete and SP-7 next; spec and plan gain execution records
including the one-atomic trampoline refinement and the compile-verified ceiling.
EOF
)"
```

- [ ] **Step 5: Finish the development branch**

Use superpowers:finishing-a-development-branch. Normal repo, on `main`, gates waived, **no push** → resolve to "keep as-is" (verify tests green first; do not merge/push/discard). Report SP-6 complete and SP-7 as the next sub-project.

---

## Self-Review

**1. Spec coverage:**
- §2.1 shared delegate + registry + `_appQuit` → Task 3 Step 1. ✓
- §2.1 `GoosieWindowCreate` retains `gw->window`, shares delegate, registers, code 6 → Task 3 Step 3. ✓
- §2.1 `windowWillClose:` unregister (idempotent) → Task 3 Steps 1–2. ✓
- §2.1 `applicationWillTerminate:` → quitAll → Task 3 Step 1. ✓
- §2.1 `GoosieAppRun` app-wide quit → Task 3 Step 4. ✓
- §2.1 `GoosieClose` order-out → Task 3 Step 5. ✓
- §2.1 `GoosieWindowCreateOnMain` trampoline + appQuit re-check → Task 4 Steps 1–2 (re-check is inside `GoosieWindowCreate`, Task 3 Step 3). ✓
- §2.2 `darwin.Open` trampoline + drop gate + `createErr` code 6 + `ErrAppQuitting` + Run/Close docs → Task 4 Steps 3–5. ✓
- §2.2 `framePath.newWindow` + `m.spawn` install → Task 1 Steps 3–4. ✓
- §2.2 registry-wide errs (snapshot after done) → Task 1 Step 4. ✓
- §2.2 Cmd+N split + `onNewWindow` field + wire → Task 2 Steps 3,4,7. ✓
- §2.2 File→New Window menu item + `new_window` action → Task 2 Steps 5,6. ✓
- §5 five headless tests + SP-6a suite → Task 1 (3 tests) + Task 2 (4 tests) + Task 2 Step 10 (full suite). ✓ (Spec named five; plan adds `TestDrainLoopErrsCollectsAll` and `TestManagerSpawnInstallsNewWindow` for deterministic mutation coverage of the drain and the install — seven total.)
- §5 compile-verified native + matrix + parity → Tasks 3–5. ✓
- §6 acceptance / roadmap SP-6 complete → Task 6. ✓
- §7 deferrals: untouched (no per-window titles, no session grouping, no shared pool). ✓

**2. Placeholder scan:** No TBD/TODO. The only `<fill from git log>` markers are in Task 6's execution-record prose, which is written AFTER the commits exist (hashes cannot be known before committing) — these are intentional record-time fills, not implementation gaps. Every code step shows complete literal code.

**3. Type consistency:**
- `newWindow func()` (framePath) ↔ `f.newWindow` install ↔ `cw.onNewWindow = f.newWindow` ↔ `onNewWindow func()` (chromeWindow) ↔ `case "new_window": f.newWindow()`. Consistent. ✓
- `drainLoopErrs(errs []<-chan error) error` defined in Task 1 Step 4, used in `run` and tested in `TestDrainLoopErrsCollectsAll`. Signature matches. ✓
- `GoosieWindowCreateOnMain(const char *, int, int, int *)` declared (shim.h), defined (.m), called (window.go `C.GoosieWindowCreateOnMain(title, C.int, C.int, &cerr)`). Matches `GoosieWindowCreate`'s signature. ✓
- `ErrAppQuitting` defined (errors.go), returned by `createErr` case 6. ✓
- C code 6 ↔ `createErr` case 6 ↔ `ErrAppQuitting`; codes 1–5 unchanged. ✓
- `registerWindow:`/`unregisterWindow:`/`quitAll`/`appQuit` declared in the `@implementation` ivar block and called consistently (`[shared registerWindow:gw]`, `[_gw->appDel unregisterWindow:_gw]`, `[self quitAll]`, `[gw->appDel appQuit]`, `[shared appQuit]`). ✓
- `g_appLoopRunning` defined before `GoosieWindowCreate`, stored in `GoosieAppRun`, loaded in `GoosieWindowCreateOnMain`. ✓
- Test helpers reused: `testManagerConfig`, `waitWindows` (existing); `surface.ModCommand`/`ModShift`, `tabs.Tab.Incognito()`, `ContextMenu.Snapshot()`, `surface.Loop.Stats().Frames` all verified present. ✓

**Ordering note verified:** `m.spawn` installs `f.newWindow` BEFORE `f.openWindow()` (which reads it to wire `cw.onNewWindow`). Installing after would capture nil — the plan installs before. ✓

---

## Execution record (2026-11-12, completed)

All six tasks executed inline on `main`, no push. TDD red-green + mutation evidence for the Go wiring (Tasks 1–2); native C compile-verified (Tasks 3–4); full matrix green on first run (Task 5).

| Task | Commit | Result |
|------|--------|--------|
| Plan | `28de361f` | this plan written + self-reviewed |
| Spec | `a96c2989` | approved design |
| 1 — manager link + drain | `9e0480d5` | `f.newWindow` installed by `m.spawn`; `drainLoopErrs` snapshots errs after `<-m.done`; 3 headless tests green; drain primary-only mutant red |
| 2 — chrome Cmd+N / menu | `dae83698` | `handleTabShortcut` Cmd+N split, `onNewWindow` field + wire, File→New Window item + `new_window` action; 4 headless tests green; Cmd+N no-op mutant red |
| 3 — shared delegate / registry | `5ffcbfdd` | one `GoosieAppDelegate` + main-thread registry + app-wide quit + retained `gw->window` + order-out close; build/vet/native green |
| 4 — trampoline + Go bridge | `6047dd0d` | `GoosieWindowCreateOnMain` + `g_appLoopRunning`; `darwin.Open` drops gate, decodes code 6 → `ErrAppQuitting`; Run/Close docs; build/vet/native/media green |
| 5 — no-regression matrix | (none) | all green first run, nothing to fix-forward |
| 6 — docs | (this commit) | roadmap SP-6 complete + SP-7 next; spec §8 + this record |

**Deviations from the plan (all minor, all verified):**
1. **`surface` import timing:** the plan put `surface` in the Task 1 test import block, but Task 1's tests do not use it (unused-import compile error). Removed it in Task 1 and added it in Task 2 Step 1 where `surface.ModCommand`/`ModShift` are first used.
2. **`@interface` selector declarations:** the four delegate methods (`registerWindow:`/`unregisterWindow:`/`quitAll`/`appQuit`) were also declared in `@interface GoosieAppDelegate` (shim_darwin.m:325), not only in `@implementation`, because `windowWillClose:` (in `GoosieWindowDelegate`, defined before the delegate `@implementation`) calls `unregisterWindow:` and clang needs a visible selector. Build/vet confirmed clean.
3. **`found`-guard in `unregisterWindow:`:** `_appQuit` is set only when the pointer was actually found AND `_count == 0`, so a failed create or a double unregister cannot quit a live app.
4. **One-atomic refinement:** `g_appLoopRunning` (see spec §8) — the registry/`_appQuit` stay main-thread-confined ivars; the trampoline reads one C atomic for loop liveness. archtest scans `.go` only, so no mutable-Go-global violation.

**Verification:** `go build -o /dev/null ./...`, `go vet -all ./...`, `go test ./internal/archtest/`, `go test ./...`, `go test -race ./cmd/goosie/ ./internal/platform/... ./internal/surface/`, `go test ./test/gate -run TestGate -v`, `make parity` (105/105, average 96.39%, gaps 4 fixtures / 6 bands — unchanged from SP-6a) — all exit 0, no DATA RACE, no FAIL. Native: `go build`/`go vet ./internal/platform/darwin/` + `go test ./internal/platform/darwin/` + `go test ./test/engine/ -run Media` green.

**Verification limit:** no automated test creates an NSWindow (needs `GoosieHasDisplay()`); the changed C functions are compile-verified + N=1 equivalence reasoned, not runtime-exercised. Native multi-window visual sign-off deferred (no display), consistent with SP-6a and all prior native work.
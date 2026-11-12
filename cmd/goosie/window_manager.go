package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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
// single-window order: join the loop, stop navigation, close every document,
// then close the window and its pool. Closing the window is what onCloseLast
// (last tab) and the native close button already do, so the manager simply
// observes the resulting stop and removes the window. App-wide quit (signal or
// cleanup) cancels m.ctx, which cancels every per-window context, so each loop
// stops and each teardown runs.
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
		<-stopped
		wcancel()
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

// run drives the interactive browser: spawn the primary window, load its URL,
// hand the main thread to a native run loop if the backend owns one, wait for
// every window to close, then save once and report. Paced/screenshot/private
// runs do not come here — run() keeps their existing single-window path.
func (m *windowManager) run() error {
	signal.Notify(m.sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(m.sig)
	defer m.cancel()

	// One signal handler quits the whole app: cancelling m.ctx cancels every
	// per-window context, so each loop stops and each teardown runs. Distributing
	// the shared signal channel across per-window teardowns would wake only one
	// window and strand the rest.
	go func() {
		select {
		case <-m.sig:
			m.cancel()
		case <-m.done:
		}
	}()

	if _, err := m.spawn(m.config.url); err != nil {
		return err
	}
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

// save persists the primary window's session plus the shared history/flags once,
// after every window is gone, and removes the crash journal. This is the
// non-private interactive save (run() routes private runs elsewhere). Window-aware
// session grouping is deferred to SP-6b.
func (m *windowManager) save() {
	m.mu.Lock()
	primary := m.primary
	m.mu.Unlock()
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

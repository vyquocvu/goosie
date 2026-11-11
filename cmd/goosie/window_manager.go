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

var _ = frame.Rect{}
var _ = session.Save
var _ = signal.Notify
var _ = syscall.SIGTERM
var _ = platform.Headless

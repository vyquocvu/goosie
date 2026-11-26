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

type windowManager struct {
	config  config
	profile *profile

	mu      sync.Mutex
	windows []*framePath
	primary *framePath
	errs    []<-chan error

	stopping     bool
	finalWindows []*framePath

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

func (m *windowManager) spawn(initialURL string) (*framePath, error) {
	return m.spawnWindow(initialURL, nil)
}

func (m *windowManager) spawnWindow(initialURL string, restored *session.WindowState) (*framePath, error) {
	c := m.config
	m.mu.Lock()
	isFirst := len(m.windows) == 0
	if !isFirst {
		c.sessionActiveFile = ""
		c.sessionFile = ""
	}
	m.mu.Unlock()

	f, err := buildWindow(c, m.profile)
	if err != nil {
		return nil, err
	}
	f.newWindow = func() {
		if _, err := m.spawn(""); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: new window: %v\n", err)
		}
	}
	if restored != nil {
		f.applySessionState(session.State{Tabs: restored.Tabs, Active: restored.Active})
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

func (m *windowManager) closeWindow(f *framePath) {
	if f != nil && f.window != nil {
		_ = f.window.Close()
	}
}

func (m *windowManager) stop() {
	m.mu.Lock()
	if !m.stopping {
		m.stopping = true
		m.finalWindows = append([]*framePath(nil), m.windows...)
	}
	m.mu.Unlock()
	m.cancel()
}

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

func (m *windowManager) run() error {
	signal.Notify(m.sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(m.sig)
	defer m.stop()

	go func() {
		select {
		case <-m.sig:
			m.stop()
		case <-m.done:
		}
	}()

	if err := m.start(); err != nil {
		return err
	}
	m.mu.Lock()
	primary := m.primary
	m.mu.Unlock()

	if r, ok := primary.window.(platform.Runner); ok {
		r.Run()
	}
	<-m.done

	m.save()

	m.mu.Lock()
	errs := m.errs
	m.mu.Unlock()
	if firstErr := drainLoopErrs(errs); firstErr != nil {
		return fmt.Errorf("goosie: %w", firstErr)
	}
	return primary.report(nil)
}

func (m *windowManager) start() error {
	primary, err := m.spawn(m.config.url)
	if err != nil {
		return err
	}
	if m.config.url != "" {
		return nil
	}
	navigateRestoredWindow(primary)
	if m.config.private || primary.crashedSessionState() != nil || !shouldRestoreSession(m.config.private, m.profile.preferences.StartupMode) {
		return nil
	}
	st, err := session.Load(m.config.sessionFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goosie: session: %v\n", err)
		return nil
	}
	windows := sessionWindowStates(st)
	if len(windows) <= 1 {
		return nil
	}
	for i, ws := range windows[1:] {
		if len(ws.Tabs) == 0 {
			continue
		}
		windowState := ws
		f, err := m.spawnWindow("", &windowState)
		if err != nil {
			return fmt.Errorf("goosie: restore window %d: %w", i+2, err)
		}
		navigateRestoredWindow(f)
	}
	return nil
}

func drainLoopErrs(errs []<-chan error) error {
	var firstErr error
	for _, ec := range errs {
		if err := <-ec; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *windowManager) save() {
	m.mu.Lock()
	primary := m.primary
	windows := append([]*framePath(nil), m.windows...)
	if m.stopping {
		windows = append([]*framePath(nil), m.finalWindows...)
	}
	m.mu.Unlock()
	if primary == nil {
		return
	}
	if len(windows) == 0 {
		windows = []*framePath{primary}
	}

	primary.sessionMu.Lock()
	sessionSuppressed := primary.sessionDisabled.Load()
	primary.sessionMu.Unlock()
	var st session.State
	for _, f := range windows {
		if f == nil || f.config.private {
			continue
		}
		f.sessionMu.Lock()
		if f.sessionDisabled.Load() {
			sessionSuppressed = true
			f.sessionMu.Unlock()
			continue
		}
		ws := currentWindowSessionState(f)
		f.sessionMu.Unlock()
		if len(ws.Tabs) == 0 {
			continue
		}
		st.Windows = append(st.Windows, session.WindowState{Tabs: ws.Tabs, Active: ws.Active})
	}
	if !sessionSuppressed && len(st.Windows) > 0 {
		st.Tabs = st.Windows[0].Tabs
		st.Active = st.Windows[0].Active
		if err := session.Save(m.config.sessionFile, st); err != nil {
			fmt.Fprintf(os.Stderr, "goosie: save session: %v\n", err)
		}
	}
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

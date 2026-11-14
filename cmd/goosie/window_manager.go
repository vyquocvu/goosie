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
	defer m.cancel()

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

	m.mu.Lock()
	errs := m.errs
	m.mu.Unlock()
	if firstErr := drainLoopErrs(errs); firstErr != nil {
		return fmt.Errorf("goosie: %w", firstErr)
	}
	return primary.report(nil)
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

package mcpserver

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownSignal is sent when the server receives an external termination request.
type ShutdownSignal int

const (
	// ShutdownNone means no shutdown requested.
	ShutdownNone ShutdownSignal = iota
	// ShutdownGraceful means a graceful shutdown was requested (SIGTERM).
	ShutdownGraceful
	// ShutdownStdin means stdin was closed and we should exit.
	ShutdownStdin
)

// ShutdownHandler manages server shutdown with timeout.
type ShutdownHandler struct {
	mu        sync.Mutex
	signal    ShutdownSignal
	triggered chan struct{}
	timeout   time.Duration
	onShutdown func(ctx context.Context) error
}

// NewShutdownHandler creates a shutdown handler with the given timeout.
// onShutdown is called during graceful shutdown to clean up resources.
func NewShutdownHandler(timeout time.Duration, onShutdown func(ctx context.Context) error) *ShutdownHandler {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ShutdownHandler{
		triggered: make(chan struct{}),
		timeout:   timeout,
		onShutdown: onShutdown,
	}
}

// Trigger marks the shutdown as initiated with the given signal type.
func (s *ShutdownHandler) Trigger(sig ShutdownSignal) {
	s.mu.Lock()
	if s.signal == ShutdownNone {
		s.signal = sig
		close(s.triggered)
	}
	s.mu.Unlock()
}

// Signal returns the received shutdown signal.
func (s *ShutdownHandler) Signal() ShutdownSignal {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signal
}

// Triggered returns a channel that is closed when shutdown begins.
func (s *ShutdownHandler) Triggered() <-chan struct{} {
	return s.triggered
}

// Wait blocks until shutdown is triggered or context is cancelled.
func (s *ShutdownHandler) Wait(ctx context.Context) ShutdownSignal {
	select {
	case <-s.triggered:
		s.mu.Lock()
		sig := s.signal
		s.mu.Unlock()
		return sig
	case <-ctx.Done():
		s.Trigger(ShutdownGraceful)
		return ShutdownGraceful
	}
}

// WaitWithTimeout blocks until shutdown is triggered or timeout.
// Returns the signal type and whether shutdown was graceful.
func (s *ShutdownHandler) WaitWithTimeout(ctx context.Context) (ShutdownSignal, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.Wait(ctx), ctx.Err()
}

// Execute runs the shutdown handler.
func (s *ShutdownHandler) Execute() error {
	if s.onShutdown == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	return s.onShutdown(ctx)
}

// InstallSignalHandlers installs OS signal handlers for SIGTERM and SIGINT.
// On Windows, only SIGINT is supported. The handler triggers graceful shutdown.
func InstallSignalHandlers(handler *ShutdownHandler) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		sig := <-sigCh
		// Convert signal to ShutdownSignal
		handler.Trigger(ShutdownGraceful)
		_ = sig // for future logging
	}()
}

// WaitForStdinClose blocks until stdin is closed.
// Returns when EOF is received on stdin.
func WaitForStdinClose(done <-chan struct{}) {
	// Use a small buffer to detect stdin closure
	buf := make([]byte, 1)
	for {
		select {
		case <-done:
			return
		default:
		}
		_, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
	}
}

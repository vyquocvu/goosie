package ui

import (
	"sync"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

// TestIsMainGoroutine_BeforeCaptureReturnsFalse verifies that without an
// explicit capture IsMainGoroutine lazily captures the current goroutine
// and returns true — this matters for tests that drive newBrowserInternal
// before the Fyne event loop has been started.
func TestIsMainGoroutine_BeforeCaptureReturnsTrue(t *testing.T) {
	// Reset the package-level capture so the test exercises the
	// lazy-capture path. The atomic is the only mutable global state
	// involved.
	prev := mainGoroutineID.Load()
	mainGoroutineID.Store(0)
	t.Cleanup(func() { mainGoroutineID.Store(prev) })

	assert.True(t, IsMainGoroutine(), "lazy capture should consider the caller as main")
}

// TestIsMainGoroutine_OnDifferentGoroutineReturnsFalse verifies that a
// goroutine distinct from the captured main goroutine reports false.
func TestIsMainGoroutine_OnDifferentGoroutineReturnsFalse(t *testing.T) {
	prev := mainGoroutineID.Load()
	captureMainGoroutineID()
	t.Cleanup(func() { mainGoroutineID.Store(prev) })

	var wg sync.WaitGroup
	var onOther bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		onOther = IsMainGoroutine()
	}()
	wg.Wait()

	assert.False(t, onOther, "goroutine other than the captured one should not report as main")
}

// TestRunOnMainThread_DirectWhenOnMain covers the synchronous fast path.
// When the caller is already on the main goroutine, RunOnMainThread must
// invoke fn synchronously (no scheduling, no waiting) so the test sees
// its side-effect before RunOnMainThread returns.
func TestRunOnMainThread_DirectWhenOnMain(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	_ = newBrowserInternal(app, w)

	var ran bool
	RunOnMainThread(func() {
		ran = true
	})
	assert.True(t, ran, "RunOnMainThread on main should run synchronously")
}

// TestRunOnMainThread_NilNoop asserts the nil-fn guard: passing nil must
// not panic and is treated as a no-op.
func TestRunOnMainThread_NilNoop(t *testing.T) {
	assert.NotPanics(t, func() { RunOnMainThread(nil) })
}

// TestRunOnMainThread_FromGoroutineMarshals verifies that calling
// RunOnMainThread from a non-main goroutine still executes fn, even
// though the production driver's DoFromGoroutine is a no-op chain in
// test mode (the test driver just runs fn inline via async.EnsureNotMain).
func TestRunOnMainThread_FromGoroutineMarshals(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	_ = newBrowserInternal(app, w)

	var (
		mu   sync.Mutex
		done bool
	)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunOnMainThread(func() {
			mu.Lock()
			done = true
			mu.Unlock()
		})
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, done, "RunOnMainThread from a goroutine should still execute fn")
}
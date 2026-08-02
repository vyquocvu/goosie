package ui

import (
	"runtime"
	"sync/atomic"

	"fyne.io/fyne/v2"
)

// Package-level cache of the Fyne main goroutine ID. Captured lazily on
// the first Browser construction that runs on the Fyne main goroutine;
// subsequent Browser constructions reuse the cached value if their
// goroutine matches.
//
// Why we need this: fyne.io/fyne/v2 does not export IsMainGoroutine (it
// lives in internal/async). Without it, calling fyne.DoAndWait from the
// main goroutine logs an error and queues the work to a different
// goroutine, leaving the caller blocked forever — a deadlock that
// manifests as "not responding" when the browser triggers a render from
// inside the Fyne event loop.
//
// The cache is intentionally package-level (rather than per-Browser)
// because in production there is exactly one Fyne main goroutine for
// the lifetime of the app; storing it once avoids the
// race-condition-where-the-wrong-goroutine-captures-first concern that
// would arise if we captured per-Browser at construction time. Tests
// that run sequentially all capture the same goroutine (the test
// goroutine); the package-level atomic is the simplest correct shape.
var mainGoroutineID atomic.Uint64

// captureMainGoroutineID stores the current goroutine's ID as the Fyne
// main goroutine. Must be called from the Fyne main thread before any
// goroutine that needs to marshal work to it.
//
// "First caller wins": if a different goroutine has already captured
// the ID, we keep the original. This matches the production scenario
// where the Fyne main goroutine pins itself via runtime.LockOSThread
// inside the glfw driver's init and is therefore always the first to
// reach this function via NewBrowser / NewBrowserWithDependencies.
func captureMainGoroutineID() {
	id := currentGoroutineID()
	for {
		prev := mainGoroutineID.Load()
		if prev != 0 {
			return
		}
		if mainGoroutineID.CompareAndSwap(0, id) {
			return
		}
	}
}

// IsMainGoroutine reports whether the caller is running on the Fyne main
// goroutine that was captured at browser construction time. Returns
// false before capture has happened (e.g. unit tests that never spin up
// the event loop) — callers that need the answer early can call
// captureMainGoroutineID explicitly.
func IsMainGoroutine() bool {
	main := mainGoroutineID.Load()
	if main == 0 {
		// Capture lazily so direct callers (tests) get a sensible answer.
		// captureMainGoroutineID is safe to call from any goroutine — the
		// "first caller wins" rule matches NewBrowser which always runs on
		// the Fyne main goroutine.
		captureMainGoroutineID()
		main = mainGoroutineID.Load()
	}
	return currentGoroutineID() == main
}

// RunOnMainThread runs fn on the Fyne main goroutine, blocking until fn
// returns. When the caller is already on the main goroutine, fn runs
// synchronously without going through fyne.Do (which would deadlock by
// re-queuing onto the very thread that needs to execute it).
//
// In headless mode there is no event loop, so fn runs on the caller's
// goroutine directly — there is no main thread to marshal onto.
//
// This is the only correct way to bridge a non-Fyne goroutine (the JS
// runtime, the documentloader mutation-coalescer timer goroutine, the
// image loader, etc.) into the renderer's Fyne canvas mutations. Fyne
// canvas objects (canvas.Text, canvas.Rectangle, Container.Refresh,
// widget.NewButton, etc.) must be created and mutated on the main
// goroutine; doing so off-thread logs the "Error in Fyne call thread,
// this should have been called in fyne.Do[AndWait]" diagnostic and, in
// the mutation-coalescer's case, leaves the app stuck because the
// queued function never runs while the main goroutine is the one
// waiting for it.
func RunOnMainThread(fn func()) {
	if fn == nil {
		return
	}
	if IsMainGoroutine() {
		fn()
		return
	}
	// Production driver (glfw) honours the wait flag and parks the
	// calling goroutine on a done channel until the main thread drains
	// funcQueue. Test driver ignores the wait flag and runs fn inline,
	// which is what we want for tests that don't spin up the event loop.
	fyne.DoAndWait(fn)
}

// currentGoroutineID returns the runtime stack's goroutine ID. The
// implementation mirrors fyne.io/fyne/v2/internal/async.goroutineID:
// parse the first whitespace-delimited number after the literal
// "goroutine " in runtime.Stack's output. This is fine for diagnostic
// use; Go's runtime deliberately hides the value for a reason but
// exposing it for a UI-thread check is a long-standing pattern.
func currentGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Header is "goroutine <id> [other stuff]:"
	// The first ' ' is after "goroutine"; everything up to the next ' '
	// is the decimal ID.
	const prefix = "goroutine "
	if n < len(prefix) {
		return 0
	}
	// Locate the trailing space after the ID.
	id := uint64(0)
	for i := len(prefix); i < n && buf[i] != ' '; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
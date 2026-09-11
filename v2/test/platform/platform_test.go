package platform_test

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/platform"
	"github.com/vyquocvu/goosie/v2/internal/platform/headless"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// This package is the only place that decides which window a v2 binary gets, and the
// decision has two hard requirements: a build on a machine with no display server has
// to yield a working window, and a run that names a GUI backend on a machine that
// cannot open one has to say so rather than crash or quietly draw nothing.

var _ surface.Window = (*headless.Window)(nil)

func testOptions() platform.Options {
	return platform.Options{
		Size:        frame.Size{W: 2 * frame.TileSize, H: 2 * frame.TileSize},
		Scale:       2,
		VsyncPeriod: time.Hour,
		Title:       "platform_test",
	}
}

func TestAvailableNamesABackend(t *testing.T) {
	name, interactive := platform.Available()
	if name == "" {
		t.Fatal("Available() named no backend")
	}
	if interactive && runtime.GOOS != "darwin" {
		t.Fatalf("Available() reports an interactive backend %q on %s, where v2 has no window shim", name, runtime.GOOS)
	}
	if got := platform.Backends(); len(got) == 0 {
		t.Fatal("Backends() is empty; there is nothing to select from")
	} else {
		found := false
		for _, b := range got {
			if b == "headless" {
				found = true
			}
		}
		if !found {
			t.Fatalf("Backends() = %v, which does not include headless", got)
		}
	}
}

// TestSelectDefaultsToAUsableWindow is the CI guarantee: with no backend named, on
// any platform, a caller gets a window it can present through.
func TestSelectDefaultsToAUsableWindow(t *testing.T) {
	name, interactive := platform.Available()
	w, err := platform.Select(testOptions())
	if err != nil {
		t.Fatalf("Select with no backend named: %v", err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	if got := w.ScaleFactor(); got != 2 {
		t.Fatalf("ScaleFactor() = %v, want the requested 2", got)
	}
	if _, ok := w.(*headless.Window); !ok && !interactive {
		t.Fatalf("Available() says %q is not interactive, but Select returned %T", name, w)
	}
}

// TestSelectHeadlessIsAlwaysAvailable is what lets a macOS runner do a headless gate
// run: naming the backend has to be enough, without a display server or an env var.
func TestSelectHeadlessIsAlwaysAvailable(t *testing.T) {
	w, err := platform.Select(appendHeadless(testOptions()))
	if err != nil {
		t.Fatalf("Select(headless): %v", err)
	}
	defer func() { _ = w.Close() }()
	if _, ok := w.(*headless.Window); !ok {
		t.Fatalf("the headless backend produced a %T", w)
	}
}

func appendHeadless(o platform.Options) platform.Options {
	o.Backend = platform.Headless
	return o
}

func TestSelectReportsAnUnknownOrUnavailableBackend(t *testing.T) {
	if _, err := platform.Select(platform.Options{Backend: "wayland"}); !errors.Is(err, platform.ErrUnknownBackend) {
		t.Fatalf("Select(wayland) = %v, want ErrUnknownBackend", err)
	}
	// A backend that exists in the table but cannot open a window here is the case
	// the GUI binary reports through Available() rather than failing the run.
	w, err := platform.Select(platform.Options{Backend: platform.Native, Size: testOptions().Size})
	switch {
	case err == nil:
		_ = w.Close()
		if _, inter := platform.Available(); !inter {
			t.Fatal("Select(native) opened a window while Available() says nothing interactive is available")
		}
	case errors.Is(err, platform.ErrBackendUnavailable):
	default:
		t.Fatalf("Select(native) = %v, want a window or ErrBackendUnavailable", err)
	}
}

// TestWindowNameReportsTheWindowNotTheMachine guards the run line a pacing gate reads.
// Available() answers a question about the machine and is answered before anything is
// opened, so a run that names headless on a Mac with a display would report "darwin"
// while drawing no pixels anywhere - and a nightly step that greps that line to prove a
// frame path was paced by a real display would pass on the very fallback it exists to
// catch. The window has to be the one that answers.
func TestWindowNameReportsTheWindowNotTheMachine(t *testing.T) {
	w, err := platform.Select(appendHeadless(testOptions()))
	if err != nil {
		t.Fatalf("Select(headless): %v", err)
	}
	defer func() { _ = w.Close() }()

	if got := platform.WindowName(w); got != platform.Headless {
		t.Fatalf("WindowName(headless window) = %q, want %q", got, platform.Headless)
	}
	if machine, _ := platform.Available(); machine == platform.Headless {
		// On a machine with no native backend the two answers coincide, so the
		// distinction below is only provable where a shim exists.
		t.Log("no native backend here; the two questions cannot disagree on this machine")
	} else if platform.WindowName(w) == machine {
		t.Fatalf("a headless window reported %q, the machine's backend: the report is "+
			"answering what this Mac could open, not what this run drew", machine)
	}
}

// unnamedWindow is a window that does not name itself, which is what a test fake in
// another package looks like to WindowName.
type unnamedWindow struct{ events chan surface.Event }

func (w *unnamedWindow) Events() <-chan surface.Event              { return w.events }
func (w *unnamedWindow) Present(*frame.Bitmap, []frame.Rect) error { return nil }
func (w *unnamedWindow) SetCursor(surface.Cursor)                  {}
func (w *unnamedWindow) ScaleFactor() float32                      { return 1 }
func (w *unnamedWindow) Close() error                              { return nil }

func TestWindowNameDefaultsToHeadless(t *testing.T) {
	var w surface.Window = &unnamedWindow{events: make(chan surface.Event)}
	if got := platform.WindowName(w); got != platform.Headless {
		t.Fatalf("WindowName(unnamed window) = %q, want %q: a window that draws nothing "+
			"must not be reported as a display backend", got, platform.Headless)
	}
}

// TestSelectCarriesTheWindowConfigThrough checks that the options are honoured rather
// than merely accepted, since a window that ignores its size or period silently
// changes what every pacing measurement downstream means.
func TestSelectCarriesTheWindowConfigThrough(t *testing.T) {
	o := appendHeadless(testOptions())
	o.VsyncPeriod = 8 * time.Millisecond
	w, err := platform.Select(o)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	defer func() { _ = w.Close() }()
	hw, ok := w.(*headless.Window)
	if !ok {
		t.Fatalf("want *headless.Window, got %T", w)
	}
	if got := hw.Size(); got != o.Size {
		t.Fatalf("Size() = %v, want the configured %v", got, o.Size)
	}
	if got := hw.VsyncPeriod(); got != o.VsyncPeriod {
		t.Fatalf("VsyncPeriod() = %v, want the configured %v", got, o.VsyncPeriod)
	}
}

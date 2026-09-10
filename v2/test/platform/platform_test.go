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

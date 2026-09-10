package platform

import (
	"errors"
	"fmt"
	"time"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/platform/headless"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// The backend names a caller may put in Options.Backend. Headless is always available;
// Native means "this machine's own window", which is only one on a platform v2 has a
// shim for.
const (
	Headless = "headless"
	Native   = "native"
)

var (
	// ErrUnknownBackend reports a backend name this build does not know at all. It is
	// worth distinguishing from unavailable: a typo is a bug in the caller, while an
	// unavailable backend is a property of the machine and is what Available is for.
	ErrUnknownBackend = errors.New("platform: no such window backend")
	// ErrBackendUnavailable reports a backend that exists in this build but cannot open
	// a window here - a GUI binary on a machine with no window server, or a shim built
	// without cgo. A caller that asked for a window reports this with Available rather
	// than failing obscurely.
	ErrBackendUnavailable = errors.New("platform: the window backend cannot open a window here")
)

// Options is what a binary asks for. Size and scale are in the units the frame path
// uses: device pixels and the device pixel ratio.
type Options struct {
	// Backend names a backend, or is empty for "whatever this machine can do".
	Backend string
	Size    frame.Size
	Scale   float32
	// VsyncPeriod paces a headless window. A native backend is paced by the display and
	// ignores it.
	VsyncPeriod time.Duration
	// Title is the window title a native backend shows.
	Title string
	// Clock paces a headless window; nil means the real clock. A gate run injects a manual
	// one so that frame intervals mean something in a report.
	Clock headless.Clock
}

// backend is one way of producing a window.
type backend interface {
	// name is what Available reports when this is the only option.
	name() string
	// interactive reports whether opening a window here would put pixels on a display.
	interactive() bool
	open(Options) (surface.Window, error)
}

// native is this platform's own window backend.
//
// It is a variable rather than a build-tagged branch in Select because the whole
// property CI depends on is that this package compiles and runs on a machine with no
// display server: the default is a backend that declines, and a platform with a shim
// installs the real one from an init in a file that only builds there.
var native backend = unavailable{}

type unavailable struct{}

func (unavailable) name() string                         { return Headless }
func (unavailable) interactive() bool                    { return false }
func (unavailable) open(Options) (surface.Window, error) { return nil, ErrBackendUnavailable }

// Available reports the name of the backend this build would use, and whether opening a
// window would actually put pixels on a display.
//
// The second answer is the one a GUI binary needs: refusing to open a window is a
// different event from opening an invisible one, and a run over ssh has to be told which
// it got.
func Available() (name string, interactive bool) {
	if native.interactive() {
		return native.name(), true
	}
	return Headless, false
}

// Backends lists the names Options.Backend accepts, for a usage message.
func Backends() []string {
	return []string{Headless, Native}
}

// Select opens a window.
//
// With no backend named it takes the platform's own window when one is available and
// headless otherwise, which is the rule that lets the same binary be a GUI app and a CI
// job. Naming headless always works, on every platform, with no display server, and that
// is what makes the headless gate runs identical in shape to the macOS one.
func Select(o Options) (surface.Window, error) {
	switch o.Backend {
	case Headless:
		return newHeadless(o), nil
	case Native:
		return native.open(o)
	case "":
		if w, err := native.open(o); err == nil {
			return w, nil
		}
		return newHeadless(o), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownBackend, o.Backend)
}

func newHeadless(o Options) *headless.Window {
	return headless.New(headless.Config{
		Size:        o.Size,
		Scale:       o.Scale,
		VsyncPeriod: o.VsyncPeriod,
		Clock:       o.Clock,
	})
}

package platform

import (
	"errors"
	"fmt"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/surface"
)

// The backend names a caller may put in Options.Backend. Headless is always available;
// Native means "this machine's own window", which is only one on a platform v2 has a
// shim for.
const (
	Headless = headless.Name
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

// Runner is a window whose platform owns the thread the program runs on. AppKit, X11,
// and Win32 all have a run loop that has to be turning for a window to receive or draw
// anything, and none of them let a library start it on a thread it did not choose.
//
// A binary that might get such a window asks for it once it has opened the window and
// started its frame loop elsewhere: Run blocks, so calling it is the caller's way of
// saying "this thread is yours now". A backend with no run loop - headless is the only
// one in v2 today - does not implement this, and the same binary then just waits on its
// frame loop. Close is what ends Run.
type Runner interface {
	Run()
}

// DroppedVsyncer is a window that can say how many pacing ticks it shed because nobody
// was reading. A frame path that draws slower than the display asks is not an error and
// is not a slow frame either - it is a skipped one - and a report that cannot tell the
// two apart cannot say what its mean was measured over. Headless windows count this
// directly and a native shim counts it in its own queue.
type DroppedVsyncer interface {
	DroppedVsyncs() int64
}

// Sizer is a window that has a size of its own, which is every window except one that
// takes whatever it is handed. A report asks because a run whose surface ended up a
// different shape from the one that was configured measured something else, and the
// difference is invisible in every other number in the report.
type Sizer interface {
	Size() frame.Size
}

// Namer is a window that can say which backend produced it. Every real window names
// itself; the interface exists so that a report can ask the window instead of asking the
// machine, which is a different question - see WindowName.
type Namer interface {
	Name() string
}

// WindowName reports the backend that opened w.
//
// Ask the window, not Available(): Available answers what this machine could put on a
// display, while a run's report has to say what it actually drew through. The two
// disagree the moment a caller names a backend - a headless gate run on a Mac with a
// display is a run with no window on it, and printing "darwin" beside its timings would
// describe a frame path this artifact did not measure. It also quietly passes any gate
// that checks the line for a native backend.
//
// A window that does not name itself is reported as headless, which is the honest
// default for the fakes that do that: they own their own event channel and no display.
func WindowName(w surface.Window) string {
	if n, ok := w.(Namer); ok {
		if name := n.Name(); name != "" {
			return name
		}
	}
	return Headless
}

// PresentCounter is a window that can say what the display did with the frames it was
// given: how many were queued to it, how many it refused, and how many actually
// reached the compositor. A loop can present a frame the platform never shows, and a
// baseline that reports only the loop's own count would call that a drawn frame.
type PresentCounter interface {
	Presents() (queued, dropped, committed int64)
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
//
// This is a question about the machine, not about a run: it is answered before anything
// is opened and does not change if the caller then names a different backend. A report
// describing frames that were drawn asks WindowName instead.
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

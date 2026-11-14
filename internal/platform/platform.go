package platform

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/platform/headless"
	"github.com/vyquocvu/goosie/internal/surface"
)

const (
	Headless = headless.Name
	Native   = "native"
)

var (
	ErrUnknownBackend     = errors.New("platform: no such window backend")
	ErrBackendUnavailable = errors.New("platform: the window backend cannot open a window here")
)

type Options struct {
	Backend     string
	Size        frame.Size
	Scale       float32
	VsyncPeriod time.Duration
	Title       string
	Clock       headless.Clock
}

type backend interface {
	name() string
	interactive() bool
	open(Options) (surface.Window, error)
}

type Runner interface {
	Run()
}

type SavePathPicker interface {
	ChooseSavePath(directory, filename string) (path string, chosen bool, err error)
}

type FilePicker interface {
	ChooseFiles(accept string, multiple bool) (paths []string, chosen bool, err error)
}

type DroppedVsyncer interface {
	DroppedVsyncs() int64
}

type Sizer interface {
	Size() frame.Size
}

type Namer interface {
	Name() string
}

func WindowName(w surface.Window) string {
	if n, ok := w.(Namer); ok {
		if name := n.Name(); name != "" {
			return name
		}
	}
	return Headless
}

type PresentCounter interface {
	Presents() (queued, dropped, committed int64)
}

var native backend = unavailable{}

type unavailable struct{}

func (unavailable) name() string                         { return Headless }
func (unavailable) interactive() bool                    { return false }
func (unavailable) open(Options) (surface.Window, error) { return nil, ErrBackendUnavailable }

func Available() (name string, interactive bool) {
	if native.interactive() {
		return native.name(), true
	}
	return Headless, false
}

func Backends() []string {
	return []string{Headless, Native}
}

func Select(o Options) (surface.Window, error) {
	switch o.Backend {
	case Headless:
		return newHeadless(o), nil
	case Native:
		return native.open(o)
	case "":
		if w, err := native.open(o); err == nil {
			return w, nil
		} else {
			fmt.Fprintf(os.Stderr, "platform: native window unavailable (%v); falling back to headless\n", err)
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

var clipboardFactory func() interface {
	Read() string
	Write(string)
}

func NewClipboard() interface {
	Read() string
	Write(string)
} {
	if clipboardFactory != nil {
		return clipboardFactory()
	}
	return nil
}

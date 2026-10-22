//go:build !darwin || !cgo

package darwin

import (
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
)

// This is the whole package on a platform v2 has no shim for, and on any platform
// built with CGO_ENABLED=0. It exists so that the import graph is the same everywhere:
// platform can name this package on Linux, the archtest can find cgo in exactly one
// directory, and the answer a caller gets for asking about a native window is an error
// rather than a build failure.
//
// The .m file is not excluded by a build tag - a tag cannot reach it, since it is not
// Go - but its _darwin suffix takes it out of the build on every other platform, which
// is the reason the file is not simply called shim.m.

// Config is the window a caller asks for. See window.go for the build that reads it.
type Config struct {
	Title string
	Size  frame.Size
	Scale float32
}

// Window is a window that cannot be opened.
type Window struct{}

// Available reports that this build has no way to put pixels on a display.
func Available() bool { return false }

// Open declines. The error is the point of the file: a caller on this platform has to
// be able to tell "no shim" apart from "shim refused", and cmd/goosie's report is what
// reads the difference.
func Open(Config) (*Window, error) { return nil, ErrPlatformUnavailable }

// NewMediaBackend reports that this build has no decoder. The nil is the answer
// rather than an omission, and it is nil rather than a backend whose Open fails:
// a caller that gets a backend has to decide whether a video is broken, while a
// caller that gets nothing knows there was never anything to break.
func NewMediaBackend() media.Backend { return nil }

// PumpMainRunLoopForTest has no run loop to service on a build with no shim.
func PumpMainRunLoopForTest(time.Duration) {}

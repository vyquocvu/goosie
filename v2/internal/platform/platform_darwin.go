package platform

import (
	"fmt"

	"github.com/vyquocvu/goosie/v2/internal/platform/darwin"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

// This file is filtered by its name rather than by a //go:build line, and that is a
// constraint rather than a style choice: the archtest's cgo rule flags any file that
// carries a cgo build tag outside the darwin package, so a tag here would fail the
// boundary test for naming the platform the boundary test is about.

func init() { native = darwinBackend{} }

// darwinBackend is macOS's window shim, seen through the narrow API the darwin package
// exports. It knows nothing about AppKit; the whole of that lives behind shim.h.
type darwinBackend struct{}

func (darwinBackend) name() string { return "darwin" }

// interactive asks the shim whether a window server is reachable, which is the
// question a GUI binary needs answered before it decides whether opening a window is
// worth trying. It deliberately does not open one.
func (darwinBackend) interactive() bool { return darwin.Available() }

func (darwinBackend) open(o Options) (surface.Window, error) {
	w, err := darwin.Open(darwin.Config{Title: o.Title, Size: o.Size, Scale: o.Scale})
	if err != nil {
		// Every failure here is a backend that exists and cannot be used from here -
		// the wrong thread, a session with no window server, a display with no pacing -
		// and Options.Backend already has one error for that. Wrapping rather than
		// mapping keeps the shim's own reason readable for a person running the binary,
		// while errors.Is still answers the caller's question.
		return nil, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	return w, nil
}

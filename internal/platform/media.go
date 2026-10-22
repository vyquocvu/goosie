package platform

import (
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/surface"
)

// MediaProvider is implemented by a window that can play media. It is an
// optional interface rather than a method on surface.Window for the same reason
// Runner is: a backend that cannot do something reports that it cannot, and the
// rest of the browser keeps working instead of failing to build.
type MediaProvider interface {
	MediaBackend() media.Backend
}

// MediaBackend returns the playback backend belonging to w, or nil when the
// window that was opened has no way to play media.
//
// The question is asked of the window rather than of the machine on purpose. A
// headless run on a Mac could ask the platform whether AVFoundation exists and
// be told yes, and a video that "plays" into nothing is worse than one that says
// it cannot: the element would sit there reporting a position that no frame ever
// followed.
func MediaBackend(w surface.Window) media.Backend {
	if p, ok := w.(MediaProvider); ok {
		return p.MediaBackend()
	}
	return nil
}

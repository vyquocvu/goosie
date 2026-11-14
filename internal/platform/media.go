package platform

import (
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/surface"
)

type MediaProvider interface {
	MediaBackend() media.Backend
}

func MediaBackend(w surface.Window) media.Backend {
	if p, ok := w.(MediaProvider); ok {
		return p.MediaBackend()
	}
	return nil
}

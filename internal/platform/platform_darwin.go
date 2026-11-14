package platform

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/platform/darwin"
	"github.com/vyquocvu/goosie/internal/surface"
)

func init() {
	native = darwinBackend{}
	clipboardFactory = func() interface {
		Read() string
		Write(string)
	} {
		return darwin.NewClipboard()
	}
}

type darwinBackend struct{}

func (darwinBackend) name() string { return darwin.Name }

func (darwinBackend) interactive() bool { return darwin.Available() }

func (darwinBackend) open(o Options) (surface.Window, error) {
	w, err := darwin.Open(darwin.Config{Title: o.Title, Size: o.Size, Scale: o.Scale})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBackendUnavailable, err)
	}
	return w, nil
}

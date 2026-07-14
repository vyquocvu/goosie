package raster

import "errors"

// ErrCGBackendNotSupported is returned on platforms where CoreGraphics is not available.
var ErrCGBackendNotSupported = errors.New("cg backend: not supported on this platform")

// NewCGBackend creates a new CoreGraphics-backed raster Backend.
// On macOS with CGo enabled this returns a real CoreGraphics implementation.
// On other platforms it returns ErrCGBackendNotSupported.
func NewCGBackend(width, height int) (Backend, error) {
	return newCGBackend(width, height)
}

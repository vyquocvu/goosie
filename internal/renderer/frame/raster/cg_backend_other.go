//go:build !darwin || !cgo

package raster

func newCGBackend(width, height int) (Backend, error) {
	return nil, ErrCGBackendNotSupported
}

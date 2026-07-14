//go:build !darwin || !cgo

package raster

// SelectBackend detects the best available backend for this platform.
// On platforms without CoreGraphics only the CPU backend is available.
func SelectBackend() BackendType {
	return BackendCPU
}

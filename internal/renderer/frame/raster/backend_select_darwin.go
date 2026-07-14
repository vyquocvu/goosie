//go:build darwin && cgo

package raster

// SelectBackend detects the best available backend for this platform.
// On macOS with CGo the CoreGraphics backend is always available.
func SelectBackend() BackendType {
	return BackendCoreGraphics
}

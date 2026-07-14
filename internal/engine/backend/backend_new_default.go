//go:build !darwin || !cgo

package backend

// New creates a platform-specific Backend or returns an error if WebView
// integration is not available on this platform.
//
// On platforms without WebView support (or when cgo is disabled), New
// returns the no-op DefaultBackend. Callers should check for ErrNotSupported
// and fall back to the pure-Go engine gracefully.
func New() Backend {
	return NewDefaultBackend()
}

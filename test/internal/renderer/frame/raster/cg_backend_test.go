package raster_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

func TestNewCGBackend(t *testing.T) {
	b, err := raster.NewCGBackend(100, 100)
	if err == raster.ErrCGBackendNotSupported {
		t.Skip("cg backend not supported on this platform")
	}
	if err != nil {
		t.Fatalf("raster.NewCGBackend: unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("raster.NewCGBackend returned nil backend with nil error")
	}
	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

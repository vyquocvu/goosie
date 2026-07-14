package raster

import (
	"testing"
)

func TestNewCGBackend(t *testing.T) {
	b, err := NewCGBackend(100, 100)
	if err == ErrCGBackendNotSupported {
		t.Skip("cg backend not supported on this platform")
	}
	if err != nil {
		t.Fatalf("NewCGBackend: unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("NewCGBackend returned nil backend with nil error")
	}
	if err := b.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

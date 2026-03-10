package image

import (
	"testing"
)

func TestLoadFromDataURI(t *testing.T) {
	// A simple 1x1 red pixel GIF base64 encoded
	// R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7 is a valid 1x1 GIF
	dataURI := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

	l := NewLoader(10)
	loader := l.(*loader)

	// Try to load the data URI
	img, err := loader.loadImage(dataURI)
	if err != nil {
		t.Fatalf("Failed to load data URI: %v", err)
	}

	if img == nil {
		t.Fatal("Loaded image is nil")
	}

	if img.Width != 1 {
		t.Errorf("Expected width 1, got %d", img.Width)
	}
	if img.Height != 1 {
		t.Errorf("Expected height 1, got %d", img.Height)
	}
	if img.Format != "gif" {
		t.Errorf("Expected format 'gif', got '%s'", img.Format)
	}
}

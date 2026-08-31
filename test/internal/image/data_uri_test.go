package image_test

import (
	"testing"

	img "github.com/vyquocvu/goosie/internal/image"
)

func TestLoadFromDataURI(t *testing.T) {
	// A simple 1x1 red pixel GIF base64 encoded
	// R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7 is a valid 1x1 GIF
	dataURI := "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)

	// Try to load the data URI
	id, err := loader.LoadImage(dataURI)
	if err != nil {
		t.Fatalf("Failed to load data URI: %v", err)
	}

	if id == nil {
		t.Fatal("Loaded image is nil")
	}

	if id.Width != 1 {
		t.Errorf("Expected width 1, got %d", id.Width)
	}
	if id.Height != 1 {
		t.Errorf("Expected height 1, got %d", id.Height)
	}
	if id.Format != "gif" {
		t.Errorf("Expected format 'gif', got '%s'", id.Format)
	}
}

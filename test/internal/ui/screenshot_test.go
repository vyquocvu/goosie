package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockWindow implements fyne.Window interface for testing
type mockWindow struct{}

func (m *mockWindow) Canvas() mockCanvas {
	return mockCanvas{}
}

type mockCanvas struct{}

func (m mockCanvas) Capture() image.Image {
	// Create a simple 10x10 test image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	return img
}

func TestTakeScreenshotToFile(t *testing.T) {
	// Create a temporary directory for test screenshots
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test_screenshot.png")

	// Note: This test requires a mock window since we can't create a real Fyne window in tests
	// For now, we test the saveImageAsPNG function directly

	// Create a test image
	testImg := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Test saving the image
	err := ui.SaveImageAsPNG(testImg, testFilePath)
	if err != nil {
		t.Fatalf("saveImageAsPNG failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
		t.Error("Screenshot file was not created")
	}

	// Verify file is a valid PNG (check magic bytes)
	file, err := os.Open(testFilePath)
	if err != nil {
		t.Fatalf("Failed to open screenshot file: %v", err)
	}
	defer file.Close()

	// PNG magic bytes: 137 80 78 71 13 10 26 10
	magic := make([]byte, 8)
	_, err = file.Read(magic)
	if err != nil {
		t.Fatalf("Failed to read file header: %v", err)
	}

	expectedMagic := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	for i, b := range expectedMagic {
		if magic[i] != b {
			t.Errorf("Invalid PNG magic byte at position %d: got %d, want %d", i, magic[i], b)
		}
	}
}

func TestDefaultScreenshotOptions(t *testing.T) {
	options := ui.DefaultScreenshotOptions()

	if options == nil {
		t.Fatal("DefaultScreenshotOptions() returned nil")
	}

	if options.Directory != "" {
		t.Errorf("Default directory should be empty, got: %s", options.Directory)
	}

	if options.Prefix != "goosie_screenshot" {
		t.Errorf("Default prefix should be 'goosie_screenshot', got: %s", options.Prefix)
	}
}

func TestSaveImageAsPNG_CreatesValidFile(t *testing.T) {
	tempDir := t.TempDir()
	testPath := filepath.Join(tempDir, "test_image.png")

	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))

	err := ui.SaveImageAsPNG(img, testPath)
	if err != nil {
		t.Fatalf("saveImageAsPNG failed: %v", err)
	}

	// Verify file exists and has content
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Size() == 0 {
		t.Error("PNG file is empty")
	}
}

func TestSaveImageAsPNG_InvalidPath(t *testing.T) {
	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	// Try to save to an invalid path
	invalidPath := "/nonexistent/directory/that/does/not/exist/screenshot.png"

	err := ui.SaveImageAsPNG(img, invalidPath)
	if err == nil {
		t.Error("Expected error when saving to invalid path, got nil")
	}

	// Verify error message contains expected information
	if !strings.Contains(err.Error(), "failed to create file") {
		t.Errorf("Expected error about file creation, got: %v", err)
	}
}

func TestTakeScreenshot_NilWindow(t *testing.T) {
	_, err := ui.TakeScreenshot(nil, nil)
	if err == nil {
		t.Error("Expected error when window is nil")
	}

	if !strings.Contains(err.Error(), "window is nil") {
		t.Errorf("Expected 'window is nil' error, got: %v", err)
	}
}

func TestTakeScreenshotToFile_NilWindow(t *testing.T) {
	err := ui.TakeScreenshotToFile(nil, "/tmp/test.png")
	if err == nil {
		t.Error("Expected error when window is nil")
	}

	if !strings.Contains(err.Error(), "window is nil") {
		t.Errorf("Expected 'window is nil' error, got: %v", err)
	}
}

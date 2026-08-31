package testutil_test

import (
	"github.com/vyquocvu/goosie/internal/testutil"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/canvas"
)

func TestRenderToImage(t *testing.T) {
	// Create a simple canvas object with a non-nil color
	rect := canvas.NewRectangle(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	rect.Resize(rect.MinSize())

	img, err := testutil.RenderToImage(rect, 100, 100)
	if err != nil {
		t.Fatalf("RenderToImage failed: %v", err)
	}

	if img == nil {
		t.Fatal("RenderToImage returned nil image")
	}

	bounds := img.Bounds()
	if bounds.Dx() < 1 || bounds.Dy() < 1 {
		t.Errorf("Image has invalid dimensions: %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestRenderToImage_NilObject(t *testing.T) {
	_, err := testutil.RenderToImage(nil, 100, 100)
	if err == nil {
		t.Error("Expected error for nil canvas object")
	}
}

func TestSaveImageAsPNG(t *testing.T) {
	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test.png")

	err := testutil.SaveImageAsPNG(img, filePath)
	if err != nil {
		t.Fatalf("SaveImageAsPNG failed: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	if info.Size() == 0 {
		t.Error("PNG file is empty")
	}
}

func TestSaveImageAsPNG_CreatesDirectory(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	tempDir := t.TempDir()
	nestedPath := filepath.Join(tempDir, "nested", "dir", "test.png")

	err := testutil.SaveImageAsPNG(img, nestedPath)
	if err != nil {
		t.Fatalf("SaveImageAsPNG failed to create nested directory: %v", err)
	}

	if _, err := os.Stat(nestedPath); err != nil {
		t.Errorf("File not created at nested path: %v", err)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"Test_Name", "Test_Name"},
		{"test/subtest", "test_subtest"},
		{"test with spaces", "test_with_spaces"},
		{"test@special#chars!", "testspecialchars"},
		{"Test-123.png", "Test-123.png"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := testutil.SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldSaveScreenshots(t *testing.T) {
	// Save original value
	original := os.Getenv(testutil.ScreenshotDir)
	defer os.Setenv(testutil.ScreenshotDir, original)

	// Test when not set
	os.Unsetenv(testutil.ScreenshotDir)
	if testutil.ShouldSaveScreenshots() {
		t.Error("ShouldSaveScreenshots should return false when env is not set")
	}

	// Test when set
	os.Setenv(testutil.ScreenshotDir, "/tmp/screenshots")
	if !testutil.ShouldSaveScreenshots() {
		t.Error("ShouldSaveScreenshots should return true when env is set")
	}
}

func TestSaveTestScreenshot_Disabled(t *testing.T) {
	// Save original value
	original := os.Getenv(testutil.ScreenshotDir)
	defer os.Setenv(testutil.ScreenshotDir, original)

	// Ensure screenshots are disabled
	os.Unsetenv(testutil.ScreenshotDir)

	rect := canvas.NewRectangle(color.RGBA{R: 0, G: 0, B: 255, A: 255})
	path, err := testutil.SaveTestScreenshot(rect, "test", 100, 100)

	if err != nil {
		t.Errorf("SaveTestScreenshot returned error when disabled: %v", err)
	}
	if path != "" {
		t.Errorf("SaveTestScreenshot returned path when disabled: %s", path)
	}
}

func TestSaveTestScreenshot_Enabled(t *testing.T) {
	tempDir := t.TempDir()

	// Save original value
	original := os.Getenv(testutil.ScreenshotDir)
	defer os.Setenv(testutil.ScreenshotDir, original)

	// Enable screenshots
	os.Setenv(testutil.ScreenshotDir, tempDir)

	rect := canvas.NewRectangle(color.RGBA{R: 0, G: 0, B: 255, A: 255})
	path, err := testutil.SaveTestScreenshot(rect, "TestExample", 100, 100)

	if err != nil {
		t.Fatalf("SaveTestScreenshot failed: %v", err)
	}

	if path == "" {
		t.Fatal("SaveTestScreenshot returned empty path")
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Screenshot file not created: %v", err)
	}
}

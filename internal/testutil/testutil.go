// Package testutil provides testing utilities for the Goosie browser.
package testutil

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// ScreenshotDir is the environment variable name for specifying screenshot output directory.
const ScreenshotDir = "GOOSIE_SCREENSHOT_DIR"

// RenderToImage renders a Fyne canvas object to an image.
// It creates a test canvas and captures it, cropping to the requested dimensions.
func RenderToImage(obj fyne.CanvasObject, width, height int) (image.Image, error) {
	if obj == nil {
		return nil, fmt.Errorf("canvas object is nil")
	}

	// Create a test app and window with light theme for white background
	a := test.NewApp()
	a.Settings().SetTheme(theme.LightTheme())
	defer a.Quit()

	w := a.NewWindow("Screenshot")
	w.Resize(fyne.NewSize(float32(width), float32(height)))
	bg := canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	bg.Resize(fyne.NewSize(float32(width), float32(height)))
	w.SetContent(container.NewMax(bg, obj))

	// Force layout calculation
	w.Content().Refresh()

	// Capture the canvas
	img := w.Canvas().Capture()
	if img == nil {
		return nil, fmt.Errorf("failed to capture canvas")
	}

	// Crop the captured image to the requested dimensions.
	// The Fyne test canvas may expand beyond the requested size due to widget
	// minimum size constraints, so we clip to the expected content area.
	bounds := img.Bounds()
	capW, capH := bounds.Max.X, bounds.Max.Y
	if capW > width || capH > height {
		cropW := capW
		if cropW > width {
			cropW = width
		}
		cropH := capH
		if cropH > height {
			cropH = height
		}
		type subImager interface {
			SubImage(r image.Rectangle) image.Image
		}
		if si, ok := img.(subImager); ok {
			img = si.SubImage(image.Rect(0, 0, cropW, cropH))
		}
	}

	return img, nil
}

// SaveRenderedScreenshot renders a canvas object and saves it as a PNG file.
func SaveRenderedScreenshot(obj fyne.CanvasObject, filepath string, width, height int) error {
	img, err := RenderToImage(obj, width, height)
	if err != nil {
		return fmt.Errorf("failed to render to image: %w", err)
	}

	return SaveImageAsPNG(img, filepath)
}

// SaveImageAsPNG saves an image to a file in PNG format.
func SaveImageAsPNG(img image.Image, filePath string) error {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// GetScreenshotDir returns the screenshot directory from environment variable.
// Returns empty string if not set.
func GetScreenshotDir() string {
	return os.Getenv(ScreenshotDir)
}

// ShouldSaveScreenshots returns true if screenshots should be saved.
func ShouldSaveScreenshots() bool {
	return GetScreenshotDir() != ""
}

// SaveTestScreenshot is a convenience function for saving screenshots in tests.
// It only saves if GOOSIE_SCREENSHOT_DIR is set.
// Returns the path where the screenshot was saved, or empty string if not saved.
func SaveTestScreenshot(obj fyne.CanvasObject, testName string, width, height int) (string, error) {
	dir := GetScreenshotDir()
	if dir == "" {
		return "", nil // Screenshots disabled
	}

	// Sanitize test name for filename
	filename := sanitizeFilename(testName) + ".png"
	filePath := filepath.Join(dir, filename)

	if err := SaveRenderedScreenshot(obj, filePath, width, height); err != nil {
		return "", err
	}

	return filePath, nil
}

// sanitizeFilename removes or replaces characters that are invalid in filenames.
func sanitizeFilename(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c)
		case c >= '0' && c <= '9':
			result = append(result, c)
		case c == '_' || c == '-' || c == '.':
			result = append(result, c)
		case c == ' ' || c == '/':
			result = append(result, '_')
		default:
			// Skip invalid characters
		}
	}
	return string(result)
}

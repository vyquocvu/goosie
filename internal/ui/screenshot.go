package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
)

// ScreenshotOptions configures screenshot capture behavior
type ScreenshotOptions struct {
	// Directory specifies where to save screenshots (defaults to current directory)
	Directory string
	// Prefix is prepended to the filename (defaults to "goosie_screenshot")
	Prefix string
}

// DefaultScreenshotOptions returns default options for taking screenshots
func DefaultScreenshotOptions() *ScreenshotOptions {
	return &ScreenshotOptions{
		Directory: "",
		Prefix:    "goosie_screenshot",
	}
}

// TakeScreenshot captures the window canvas and saves it as a PNG file
// Returns the path to the saved screenshot file
func TakeScreenshot(window fyne.Window, options *ScreenshotOptions) (string, error) {
	if window == nil {
		return "", fmt.Errorf("window is nil")
	}

	if options == nil {
		options = DefaultScreenshotOptions()
	}

	// Capture the canvas
	img := window.Canvas().Capture()
	if img == nil {
		return "", fmt.Errorf("failed to capture canvas")
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("%s_%s.png", options.Prefix, timestamp)

	// Build full path
	fullPath := filename
	if options.Directory != "" {
		fullPath = filepath.Join(options.Directory, filename)
	}

	// Save the image
	err := saveImageAsPNG(img, fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to save screenshot: %w", err)
	}

	return fullPath, nil
}

// TakeScreenshotToFile captures the window canvas and saves it to a specific file path
func TakeScreenshotToFile(window fyne.Window, filepath string) error {
	if window == nil {
		return fmt.Errorf("window is nil")
	}

	// Capture the canvas
	img := window.Canvas().Capture()
	if img == nil {
		return fmt.Errorf("failed to capture canvas")
	}

	return saveImageAsPNG(img, filepath)
}

// saveImageAsPNG saves an image to a file in PNG format
func saveImageAsPNG(img image.Image, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	err = png.Encode(file, img)
	if err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

package image_test

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	img "github.com/vyquocvu/goosie/internal/image"
	"github.com/stretchr/testify/require"
)

func requireLoopbackListener(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}
	require.NoError(t, ln.Close())
}

func TestNewLoader(t *testing.T) {
	l := img.NewLoader(10)
	if l == nil {
		t.Fatal("NewLoader returned nil")
	}
	loader := l.(*img.ImageLoader)
	if loader.GetCache() == nil {
		t.Error("Cache not initialized")
	}
	if loader.GetHTTPClient() == nil {
		t.Error("HTTP client not initialized")
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"https://example.com/image.png", true},
		{"/path/to/file.png", false},
		{"file.png", false},
		{"htt://invalid", false},
	}

	for _, tt := range tests {
		result := img.IsURL(tt.input)
		if result != tt.expected {
			t.Errorf("IsURL(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary test image
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a simple 10x10 red image
	testImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			testImg.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	// Save the image
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	if err := png.Encode(f, testImg); err != nil {
		f.Close()
		t.Fatalf("Failed to encode test image: %v", err)
	}
	f.Close()

	// Test loading
	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)
	data, err := loader.LoadSync(testImagePath)
	if err != nil {
		t.Fatalf("LoadSync failed: %v", err)
	}

	if data == nil {
		t.Fatal("LoadSync returned nil data")
	}
	if data.State != img.StateLoaded {
		t.Errorf("Expected state StateLoaded, got %v", data.State)
	}
	if data.Width != 10 {
		t.Errorf("Expected width 10, got %d", data.Width)
	}
	if data.Height != 10 {
		t.Errorf("Expected height 10, got %d", data.Height)
	}
	if data.Format != "png" {
		t.Errorf("Expected format 'png', got %s", data.Format)
	}
}

func TestLoadFromURL(t *testing.T) {
	requireLoopbackListener(t)
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a simple 10x10 blue image
		testImg := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				testImg.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
		png.Encode(w, testImg)
	}))
	defer server.Close()

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)
	data, err := loader.LoadSync(server.URL)
	if err != nil {
		t.Fatalf("LoadSync failed: %v", err)
	}

	if data == nil {
		t.Fatal("LoadSync returned nil data")
	}
	if data.State != img.StateLoaded {
		t.Errorf("Expected state StateLoaded, got %v", data.State)
	}
	if data.Width != 10 {
		t.Errorf("Expected width 10, got %d", data.Width)
	}
	if data.Height != 10 {
		t.Errorf("Expected height 10, got %d", data.Height)
	}
}

func TestLoadFromURLError(t *testing.T) {
	requireLoopbackListener(t)
	// Create a test HTTP server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)
	data, err := loader.LoadSync(server.URL)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
	if data == nil {
		t.Fatal("Expected data with error state")
	}
	if data.State != img.StateError {
		t.Errorf("Expected state StateError, got %v", data.State)
	}
}

func TestCaching(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	testImg := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, testImg)
	f.Close()

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)

	// First load
	data1, err := loader.LoadSync(testImagePath)
	if err != nil {
		t.Fatalf("First LoadSync failed: %v", err)
	}

	// Second load should come from cache
	data2, err := loader.LoadSync(testImagePath)
	if err != nil {
		t.Fatalf("Second LoadSync failed: %v", err)
	}

	// Both should return the same cached data
	if data1 != data2 {
		t.Error("Expected cached data to be reused")
	}

	// Verify cache contains the image
	if loader.GetCache().Get(testImagePath) == nil {
		t.Error("Expected image to be in cache")
	}
}

func TestLoadAsync(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	testImg := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, testImg)
	f.Close()

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)

	// Load async - should return loading state immediately
	data, err := loader.Load(testImagePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if data.State != img.StateLoading {
		t.Errorf("Expected state StateLoading, got %v", data.State)
	}

	// Wait a bit for async load to complete
	time.Sleep(100 * time.Millisecond)

	// Now it should be in cache
	cached := loader.GetCache().Get(testImagePath)
	if cached == nil {
		t.Error("Expected image to be cached after async load")
	}
	if cached.State != img.StateLoaded {
		t.Errorf("Expected cached state StateLoaded, got %v", cached.State)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)
	data, err := loader.LoadSync("/nonexistent/file.png")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
	if data == nil {
		t.Fatal("Expected data with error state")
	}
	if data.State != img.StateError {
		t.Errorf("Expected state StateError, got %v", data.State)
	}
}

func TestConcurrentLoads(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	testImg := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, testImg)
	f.Close()

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)

	// Start multiple concurrent loads of the same image
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			data, err := loader.Load(testImagePath)
			if err != nil {
				t.Errorf("Load %d failed: %v", id, err)
			}
			if data == nil {
				t.Errorf("Load %d returned nil data", id)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Wait for async load to complete
	time.Sleep(200 * time.Millisecond)

	// Should only have one cached entry
	if loader.GetCache().Len() != 1 {
		t.Errorf("Expected 1 cached entry, got %d", loader.GetCache().Len())
	}
}

// Helper function to create a test image file
func createTestImage(path string, width, height int) error {
	testImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			testImg.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, testImg); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	return nil
}

func TestDownscaleImage(t *testing.T) {
	// 1. Nil input returns nil
	if img.DownscaleImage(nil, 100, 100) != nil {
		t.Error("Expected nil for nil src image")
	}

	// 2. 4K image (3840 x 2160, 16:9)
	src4K := image.NewRGBA(image.Rect(0, 0, 3840, 2160))
	downscaled := img.DownscaleImage(src4K, 1920, 1080)
	if downscaled.Bounds().Dx() != 1920 || downscaled.Bounds().Dy() != 1080 {
		t.Errorf("Expected 1920x1080, got %dx%d", downscaled.Bounds().Dx(), downscaled.Bounds().Dy())
	}

	// 3. Aspect ratio preserving downscale to 300x200
	// 3840/2160 = 16/9. If maxW=300, maxH=200: ratio = min(300/3840, 200/2160) = 300/3840 = 0.078125
	// targetW = 300, targetH = round(2160 * 0.078125) = 169 <= 200.
	thumb := img.DownscaleImage(src4K, 300, 200)
	if thumb.Bounds().Dx() != 300 {
		t.Errorf("Expected width 300, got %d", thumb.Bounds().Dx())
	}
	if thumb.Bounds().Dy() != 169 {
		t.Errorf("Expected height 169, got %d", thumb.Bounds().Dy())
	}

	// 4. Smaller image returned unmodified
	small := image.NewRGBA(image.Rect(0, 0, 100, 100))
	same := img.DownscaleImage(small, 800, 600)
	if same != small {
		t.Error("Expected same image returned when already within bounds")
	}

	// 5. Zero/negative max dimensions returns original
	noCap := img.DownscaleImage(small, 0, 0)
	if noCap != small {
		t.Error("Expected same image returned when maxW/maxH <= 0")
	}

	// 6. ImageData.Downscale helper
	data := &img.ImageData{Image: src4K, Width: 3840, Height: 2160, Format: "png", State: img.StateLoaded}
	scaledData := data.Downscale(300, 200)
	if scaledData.Width != 300 || scaledData.Height != 169 {
		t.Errorf("Expected scaledData dimensions 300x169, got %dx%d", scaledData.Width, scaledData.Height)
	}
}

func TestLoaderDecodeClamping(t *testing.T) {
	tmpDir := t.TempDir()
	largeImgPath := filepath.Join(tmpDir, "large.png")

	// Create 2400 x 1200 image (exceeds default 1920x1080)
	err := createTestImage(largeImgPath, 2400, 1200)
	require.NoError(t, err)

	l := img.NewLoader(10)
	loader := l.(*img.ImageLoader)

	data, err := loader.LoadSync(largeImgPath)
	require.NoError(t, err)
	require.NotNil(t, data)
	require.NotNil(t, data.Image)

	// Max decode resolution must clamp to <= 1920x1080 while preserving 2:1 aspect ratio
	// 2400x1200 clamped to 1920x1080: ratio = 1920/2400 = 0.8 -> 1920 x 960
	if data.Width > img.DefaultMaxDecodeWidth || data.Height > img.DefaultMaxDecodeHeight {
		t.Errorf("Expected clamped dimensions <= %dx%d, got %dx%d",
			img.DefaultMaxDecodeWidth, img.DefaultMaxDecodeHeight, data.Width, data.Height)
	}
	if data.Width != 1920 || data.Height != 960 {
		t.Errorf("Expected 1920x960, got %dx%d", data.Width, data.Height)
	}
}


package image

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
	l := NewLoader(10)
	if l == nil {
		t.Fatal("NewLoader returned nil")
	}
	loader := l.(*loader)
	if loader.cache == nil {
		t.Error("Cache not initialized")
	}
	if loader.httpClient == nil {
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
		result := isURL(tt.input)
		if result != tt.expected {
			t.Errorf("isURL(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary test image
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a simple 10x10 red image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	// Save the image
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("Failed to encode test image: %v", err)
	}
	f.Close()

	// Test loading
	l := NewLoader(10)
	loader := l.(*loader)
	data, err := loader.LoadSync(testImagePath)
	if err != nil {
		t.Fatalf("LoadSync failed: %v", err)
	}

	if data == nil {
		t.Fatal("LoadSync returned nil data")
	}
	if data.State != StateLoaded {
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
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				img.Set(x, y, color.RGBA{B: 255, A: 255})
			}
		}
		png.Encode(w, img)
	}))
	defer server.Close()

	l := NewLoader(10)
	loader := l.(*loader)
	data, err := loader.LoadSync(server.URL)
	if err != nil {
		t.Fatalf("LoadSync failed: %v", err)
	}

	if data == nil {
		t.Fatal("LoadSync returned nil data")
	}
	if data.State != StateLoaded {
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

	l := NewLoader(10)
	loader := l.(*loader)
	data, err := loader.LoadSync(server.URL)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
	if data == nil {
		t.Fatal("Expected data with error state")
	}
	if data.State != StateError {
		t.Errorf("Expected state StateError, got %v", data.State)
	}
}

func TestCaching(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, img)
	f.Close()

	l := NewLoader(10)
	loader := l.(*loader)

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
	if loader.cache.Get(testImagePath) == nil {
		t.Error("Expected image to be in cache")
	}
}

func TestLoadAsync(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, img)
	f.Close()

	l := NewLoader(10)
	loader := l.(*loader)

	// Load async - should return loading state immediately
	data, err := loader.Load(testImagePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if data.State != StateLoading {
		t.Errorf("Expected state StateLoading, got %v", data.State)
	}

	// Wait a bit for async load to complete
	time.Sleep(100 * time.Millisecond)

	// Now it should be in cache
	cached := loader.cache.Get(testImagePath)
	if cached == nil {
		t.Error("Expected image to be cached after async load")
	}
	if cached.State != StateLoaded {
		t.Errorf("Expected cached state StateLoaded, got %v", cached.State)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	l := NewLoader(10)
	loader := l.(*loader)
	data, err := loader.LoadSync("/nonexistent/file.png")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
	if data == nil {
		t.Fatal("Expected data with error state")
	}
	if data.State != StateError {
		t.Errorf("Expected state StateError, got %v", data.State)
	}
}

func TestConcurrentLoads(t *testing.T) {
	tmpDir := t.TempDir()
	testImagePath := filepath.Join(tmpDir, "test.png")

	// Create a test image
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	f, err := os.Create(testImagePath)
	if err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	png.Encode(f, img)
	f.Close()

	l := NewLoader(10)
	loader := l.(*loader)

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
	if loader.cache.Len() != 1 {
		t.Errorf("Expected 1 cached entry, got %d", loader.cache.Len())
	}
}

// Helper function to create a test image file
func createTestImage(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
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

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	return nil
}

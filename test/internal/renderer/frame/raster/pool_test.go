package raster_test

import (
	"image"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// TestBufferPool_GetAndPut verifies basic buffer allocation and reuse.
func TestBufferPool_GetAndPut(t *testing.T) {
	pool := raster.NewBufferPool()

	// Get a buffer
	buf1 := pool.Get(100, 200)
	if buf1 == nil {
		t.Fatal("Get returned nil")
	}

	// Verify dimensions
	bounds := buf1.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 200 {
		t.Errorf("Expected 100x200, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Verify buffer is cleared (all zeros)
	for i, v := range buf1.Pix {
		if v != 0 {
			t.Errorf("Buffer not cleared at index %d: got %d, want 0", i, v)
			break
		}
	}

	// Put it back
	pool.Put(buf1)

	// Get another buffer of the same size - should be reused
	buf2 := pool.Get(100, 200)
	if buf2 == nil {
		t.Fatal("Second Get returned nil")
	}

	// Should be the same underlying array (reused)
	if &buf2.Pix[0] != &buf1.Pix[0] {
		t.Log("Warning: Buffer was not reused (may be GC'd)")
	}

	pool.Put(buf2)

	// Check stats
	stats := pool.Stats()
	if stats.Gets != 2 {
		t.Errorf("Expected 2 Gets, got %d", stats.Gets)
	}
	if stats.Puts != 2 {
		t.Errorf("Expected 2 Puts, got %d", stats.Puts)
	}
	if stats.Allocs != 1 {
		t.Errorf("Expected 1 Alloc, got %d", stats.Allocs)
	}
}

// TestBufferPool_DifferentSizes verifies that buffers of different sizes
// are pooled separately.
func TestBufferPool_DifferentSizes(t *testing.T) {
	pool := raster.NewBufferPool()

	// Get buffers of different sizes
	buf1 := pool.Get(100, 100)
	buf2 := pool.Get(200, 200)
	buf3 := pool.Get(100, 100)

	// Put them back
	pool.Put(buf1)
	pool.Put(buf2)
	pool.Put(buf3)

	// Get again - should reuse correctly
	buf4 := pool.Get(100, 100)
	buf5 := pool.Get(200, 200)

	if buf4.Bounds().Dx() != 100 {
		t.Errorf("Expected width 100, got %d", buf4.Bounds().Dx())
	}
	if buf5.Bounds().Dx() != 200 {
		t.Errorf("Expected width 200, got %d", buf5.Bounds().Dx())
	}

	pool.Put(buf4)
	pool.Put(buf5)

	stats := pool.Stats()
	// 3 gets initially, 2 more gets = 5 total
	if stats.Gets != 5 {
		t.Errorf("Expected 5 Gets, got %d", stats.Gets)
	}
	// 2 unique sizes = 2 allocations minimum
	if stats.Allocs < 2 {
		t.Errorf("Expected at least 2 Allocs, got %d", stats.Allocs)
	}
}

// TestBufferPool_ZeroDimensions verifies that zero/negative dimensions
// are normalized to 1x1.
func TestBufferPool_ZeroDimensions(t *testing.T) {
	pool := raster.NewBufferPool()

	tests := []struct {
		w, h     int
		wantW, wantH int
	}{
		{0, 0, 1, 1},
		{-10, 5, 1, 5},
		{10, -5, 10, 1},
		{-1, -1, 1, 1},
	}

	for _, tt := range tests {
		buf := pool.Get(tt.w, tt.h)
		bounds := buf.Bounds()
		if bounds.Dx() != tt.wantW || bounds.Dy() != tt.wantH {
			t.Errorf("Get(%d, %d) = %dx%d, want %dx%d",
				tt.w, tt.h, bounds.Dx(), bounds.Dy(), tt.wantW, tt.wantH)
		}
		pool.Put(buf)
	}
}

// TestBufferPool_NilPut verifies that Put handles nil gracefully.
func TestBufferPool_NilPut(t *testing.T) {
	pool := raster.NewBufferPool()

	// Should not panic
	pool.Put(nil)

	stats := pool.Stats()
	if stats.Puts != 0 {
		t.Errorf("Expected 0 Puts for nil, got %d", stats.Puts)
	}
}

// TestBufferPool_Concurrent verifies thread safety under concurrent access.
func TestBufferPool_Concurrent(t *testing.T) {
	pool := raster.NewBufferPool()
	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				width := 100 + (id * 10)
				height := 200
				buf := pool.Get(width, height)
				if buf == nil {
					t.Errorf("Goroutine %d: Get returned nil", id)
					return
				}
				// Simulate some work
				for j := range buf.Pix[:100] {
					buf.Pix[j] = byte(j)
				}
				pool.Put(buf)
			}
		}(g)
	}

	wg.Wait()

	stats := pool.Stats()
	expectedGets := goroutines * iterations
	if int(stats.Gets) != expectedGets {
		t.Errorf("Expected %d Gets, got %d", expectedGets, stats.Gets)
	}
	if int(stats.Puts) != expectedGets {
		t.Errorf("Expected %d Puts, got %d", expectedGets, stats.Puts)
	}
	if stats.ActiveNow != 0 {
		t.Errorf("Expected 0 ActiveNow, got %d", stats.ActiveNow)
	}
}

// TestBufferPool_Reset verifies that Reset clears all state.
func TestBufferPool_Reset(t *testing.T) {
	pool := raster.NewBufferPool()

	// Use the pool
	buf := pool.Get(100, 100)
	pool.Put(buf)

	// Reset
	pool.Reset()

	stats := pool.Stats()
	if stats.Gets != 0 || stats.Puts != 0 || stats.Allocs != 0 {
		t.Errorf("Reset didn't clear stats: %+v", stats)
	}
}

// TestGlobalBufferPool verifies the global pool convenience functions.
func TestGlobalBufferPool(t *testing.T) {
	// Reset global pool for clean test
	raster.GlobalBufferPool().Reset()

	buf := raster.GetBuffer(50, 50)
	if buf == nil {
		t.Fatal("GetBuffer returned nil")
	}

	raster.PutBuffer(buf)

	stats := raster.GlobalBufferPool().Stats()
	if stats.Gets != 1 {
		t.Errorf("Expected 1 Get, got %d", stats.Gets)
	}
}

// BenchmarkBufferPool_GetAndPut measures allocation performance.
// Goal: 0 B/op, 0 allocs/op after warmup (buffer reuse).
func BenchmarkBufferPool_GetAndPut(b *testing.B) {
	pool := raster.NewBufferPool()
	width, height := 1920, 1080

	// Warmup: allocate one buffer
	buf := pool.Get(width, height)
	pool.Put(buf)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf := pool.Get(width, height)
		// Simulate minimal work
		buf.Pix[0] = 0xFF
		pool.Put(buf)
	}

	b.StopTimer()
	stats := pool.Stats()
	b.Logf("Stats: Gets=%d, Puts=%d, Allocs=%d, Reuses=%d",
		stats.Gets, stats.Puts, stats.Allocs, stats.Reuses)
}

// BenchmarkBufferPool_Alloc measures pure allocation without reuse.
// This shows the cost of creating new buffers.
func BenchmarkBufferPool_Alloc(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	}
}

// BenchmarkBufferPool_Concurrent measures concurrent access performance.
func BenchmarkBufferPool_Concurrent(b *testing.B) {
	pool := raster.NewBufferPool()
	width, height := 800, 600

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get(width, height)
			buf.Pix[0] = 0xFF
			pool.Put(buf)
		}
	})
}

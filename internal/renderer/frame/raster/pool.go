// Package raster provides a zero-allocation buffer pool for pixel buffers.
//
// M1.1: Implements sync.Pool-based buffer management to reuse *image.RGBA
// buffers across frames, eliminating GC pressure from repeated allocations.
package raster

import (
	"image"
	"sync"
	"sync/atomic"
)

// ---------------------------------------------------------------------------
// BufferPool — zero-allocation pixel buffer pool
// ---------------------------------------------------------------------------

// BufferPool manages a pool of reusable *image.RGBA buffers keyed by
// dimensions. Buffers are recycled via sync.Pool to avoid per-frame
// allocations and reduce GC pressure.
//
// The pool is safe for concurrent use. Multiple goroutines can Get/Put
// buffers simultaneously without external synchronization.
type BufferPool struct {
	// pools maps dimension keys (width<<32|height) to sync.Pool instances.
	// Each pool holds buffers of that specific size.
	pools sync.Map // map[uint64]*sync.Pool

	// stats tracks pool usage for diagnostics.
	stats struct {
		Gets      atomic.Int64
		Puts      atomic.Int64
		Allocs    atomic.Int64
		Reuses    atomic.Int64
		Discards  atomic.Int64
		ActiveNow atomic.Int64
	}
}

// PoolStats holds diagnostic information about pool usage.
type PoolStats struct {
	Gets      int64 // Total number of Get calls
	Puts      int64 // Total number of Put calls
	Allocs    int64 // Number of new buffers allocated
	Reuses    int64 // Number of times a buffer was reused from pool
	Discards  int64 // Number of buffers discarded (pool full)
	ActiveNow int64 // Currently outstanding buffers (Get - Put)
}

// NewBufferPool creates a new buffer pool.
func NewBufferPool() *BufferPool {
	return &BufferPool{}
}

// Get retrieves a buffer of the specified dimensions from the pool.
// If a buffer of that size is available, it is reused (0 allocations).
// Otherwise, a new buffer is allocated.
//
// The returned buffer is cleared to transparent black before being returned.
// Callers must call Put to return the buffer when done.
func (p *BufferPool) Get(width, height int) *image.RGBA {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}

	key := dimKey(width, height)
	pool := p.getOrCreatePool(key, width, height)

	p.stats.Gets.Add(1)
	p.stats.ActiveNow.Add(1)

	buf := pool.Get().(*image.RGBA)

	// Clear the buffer to transparent black
	for i := range buf.Pix {
		buf.Pix[i] = 0
	}

	return buf
}

// Put returns a buffer to the pool for reuse. The buffer must have been
// obtained from Get. After calling Put, the caller must not use the buffer.
func (p *BufferPool) Put(buf *image.RGBA) {
	if buf == nil {
		return
	}

	bounds := buf.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	key := dimKey(width, height)

	p.stats.Puts.Add(1)
	p.stats.ActiveNow.Add(-1)

	// Retrieve the pool for this dimension
	if poolVal, ok := p.pools.Load(key); ok {
		pool := poolVal.(*sync.Pool)
		pool.Put(buf)
	}
	// If pool doesn't exist, buffer is discarded (GC will collect it)
}

// getOrCreatePool retrieves or creates a sync.Pool for the given dimension key.
func (p *BufferPool) getOrCreatePool(key uint64, width, height int) *sync.Pool {
	// Fast path: pool already exists
	if poolVal, ok := p.pools.Load(key); ok {
		return poolVal.(*sync.Pool)
	}

	// Slow path: create a new pool
	newPool := &sync.Pool{
		New: func() interface{} {
			p.stats.Allocs.Add(1)
			return image.NewRGBA(image.Rect(0, 0, width, height))
		},
	}

	// Store the pool (another goroutine may have created it first)
	actual, _ := p.pools.LoadOrStore(key, newPool)
	return actual.(*sync.Pool)
}

// Stats returns a snapshot of the pool's diagnostic statistics.
func (p *BufferPool) Stats() PoolStats {
	return PoolStats{
		Gets:      p.stats.Gets.Load(),
		Puts:      p.stats.Puts.Load(),
		Allocs:    p.stats.Allocs.Load(),
		Reuses:    p.stats.Reuses.Load(),
		Discards:  p.stats.Discards.Load(),
		ActiveNow: p.stats.ActiveNow.Load(),
	}
}

// Reset clears all pools and resets statistics. This is primarily for testing.
func (p *BufferPool) Reset() {
	p.pools = sync.Map{}
	p.stats.Gets.Store(0)
	p.stats.Puts.Store(0)
	p.stats.Allocs.Store(0)
	p.stats.Reuses.Store(0)
	p.stats.Discards.Store(0)
	p.stats.ActiveNow.Store(0)
}

// dimKey encodes width and height into a single uint64 for use as a map key.
// Width occupies the upper 32 bits, height the lower 32 bits.
func dimKey(width, height int) uint64 {
	return (uint64(width) << 32) | uint64(height)
}

// ---------------------------------------------------------------------------
// Global shared pool
// ---------------------------------------------------------------------------

// globalPool is a package-level shared buffer pool for use by all raster
// backends. This avoids the overhead of creating a pool per backend instance.
var globalPool = NewBufferPool()

// GlobalBufferPool returns the package-level shared buffer pool.
// Most callers should use this rather than creating a new pool.
func GlobalBufferPool() *BufferPool {
	return globalPool
}

// ---------------------------------------------------------------------------
// Convenience functions
// ---------------------------------------------------------------------------

// GetBuffer retrieves a pixel buffer from the global pool.
// Equivalent to GlobalBufferPool().Get(width, height).
func GetBuffer(width, height int) *image.RGBA {
	return globalPool.Get(width, height)
}

// PutBuffer returns a pixel buffer to the global pool.
// Equivalent to GlobalBufferPool().Put(buf).
func PutBuffer(buf *image.RGBA) {
	globalPool.Put(buf)
}

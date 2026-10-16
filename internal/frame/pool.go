package frame

import "sync"

// BitmapPool recycles fixed-dimension bitmaps so the steady-state frame path
// stops allocating. Invariant 6 (a warm scroll frame allocates nothing that is
// not pooled) is only achievable if tile buffers and the backing store come
// from here, which is why it lives in the leaf package rather than in surface:
// raster, surface, and the grid all hand buffers to each other.
//
// One pool serves one dimension. That is deliberately narrow: tiles are
// uniformly TileSize and the backing store is one viewport size, so a keyed map
// of free lists would be speculative generality. Callers needing two sizes hold
// two pools.
type BitmapPool struct {
	size     Size
	maxIdle  int
	mu       sync.Mutex
	idle     []*Bitmap
	created  int64
	acquired int64
	released int64
}

// NewBitmapPool returns a pool for buffers of the given size, retaining at most
// maxIdle of them. maxIdle <= 0 means retain without a cap, which is safe
// because the pool only ever holds buffers that were already allocated.
func NewBitmapPool(size Size, maxIdle int) *BitmapPool {
	pre := maxIdle
	if pre <= 0 || pre > 64 {
		pre = 64
	}
	return &BitmapPool{size: size, maxIdle: maxIdle, idle: make([]*Bitmap, 0, pre)}
}

// Size returns the dimension every buffer in the pool has.
func (p *BitmapPool) Size() Size { return p.size }

// Prealloc warms the pool so a benchmark's first frames are not measured
// through their own setup allocations.
func (p *BitmapPool) Prealloc(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < n && len(p.idle) < p.cap(); i++ {
		p.idle = append(p.idle, NewBitmap(int(p.size.W), int(p.size.H)))
		p.created++
	}
}

func (p *BitmapPool) cap() int {
	if p.maxIdle <= 0 {
		return 1 << 30
	}
	return p.maxIdle
}

// Acquire returns a buffer of the pool's size. Its contents are undefined: every
// consumer is responsible for clearing before writing, which is cheaper than
// zeroing here and then painting over the zeroing.
func (p *BitmapPool) Acquire() *Bitmap {
	p.mu.Lock()
	p.acquired++
	if n := len(p.idle); n > 0 {
		b := p.idle[n-1]
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		return b
	}
	p.created++
	size := p.size
	p.mu.Unlock()
	return NewBitmap(int(size.W), int(size.H))
}

// Release returns b to the pool. A buffer of another size is dropped rather than
// kept, because the next Acquire would hand out a wrongly-sized buffer and the
// bug would surface as a pixel-index panic far from its cause.
func (p *BitmapPool) Release(b *Bitmap) {
	if b == nil || b.Empty() {
		return
	}
	if b.W != int(p.size.W) || b.H != int(p.size.H) || b.Stride != b.W*4 {
		return
	}
	p.mu.Lock()
	p.released++
	full := len(p.idle) >= p.cap()
	if !full {
		p.idle = append(p.idle, b)
	}
	p.mu.Unlock()
}

// PoolStats is a snapshot of pool churn. Created is the number that ever
// allocated, so Created staying flat across frames is the direct evidence that
// steady state is allocation-free.
type PoolStats struct {
	Created, Acquired, Released, Idle int64
}

// Stats returns the churn counters.
func (p *BitmapPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStats{Created: p.created, Acquired: p.acquired, Released: p.released, Idle: int64(len(p.idle))}
}

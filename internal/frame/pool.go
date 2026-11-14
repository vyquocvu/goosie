package frame

import "sync"

type BitmapPool struct {
	size     Size
	maxIdle  int
	mu       sync.Mutex
	idle     []*Bitmap
	created  int64
	acquired int64
	released int64
}

func NewBitmapPool(size Size, maxIdle int) *BitmapPool {
	pre := maxIdle
	if pre <= 0 || pre > 64 {
		pre = 64
	}
	return &BitmapPool{size: size, maxIdle: maxIdle, idle: make([]*Bitmap, 0, pre)}
}

func (p *BitmapPool) Size() Size { return p.size }

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

type PoolStats struct {
	Created, Acquired, Released, Idle int64
}

func (p *BitmapPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStats{Created: p.created, Acquired: p.acquired, Released: p.released, Idle: int64(len(p.idle))}
}

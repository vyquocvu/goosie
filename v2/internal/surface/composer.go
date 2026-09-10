package surface

import "github.com/vyquocvu/goosie/v2/internal/frame"

// TileBlit is one finished tile and where it belongs on screen. Dst is in
// backing-store coordinates; the tile's own pixels are a square buffer whose
// local origin is Dst.
type TileBlit struct {
	Pixels *frame.Bitmap
	Dst    frame.Rect
}

// Composer owns the backing store the window is presented from and turns a set
// of tiles plus a damage list into pixels. It is the only code in v2 that writes
// the buffer handed to the platform, which is what makes the "damage blit"
// claim checkable: everything below it produces tiles, everything above it just
// presents.
//
// The backing store is allocated once per surface size and reused across every
// frame. Re-acquiring it per frame would be a viewport-sized allocation in the
// hot path, and the composer outliving a page navigation is the point.
type Composer struct {
	Backing *frame.Bitmap
	pool    *frame.BitmapPool

	// damage is scratch reused for the returned rect list, because the list's
	// lifetime is one frame and its caller is the next Present.
	damage []frame.Rect
}

// NewComposer returns a composer with a backing store sized to vp. The pool
// should be sized to vp too; when it is not, the buffer is allocated at vp rather
// than acquired, because presenting a buffer whose size is the pool's and not the
// viewport's is a whole-surface artefact rather than a startup convenience.
func NewComposer(vp frame.Size, pool *frame.BitmapPool) *Composer {
	var b *frame.Bitmap
	if pool.Size() == vp {
		b = pool.Acquire()
	} else {
		b = frame.NewBitmap(int(vp.W), int(vp.H))
	}
	return &Composer{Backing: b, pool: pool}
}

// NewComposerFromBacking returns a composer that writes into a buffer somebody
// else allocated. It exists for a platform that owns its front buffer and cannot
// hand a pooled one to the window server; the pool is still used on resize, so
// the composer's allocation story does not change with the backing store's.
func NewComposerFromBacking(b *frame.Bitmap, pool *frame.BitmapPool) *Composer {
	return &Composer{Backing: b, pool: pool}
}

// Resize swaps the backing store to a new size, returning the old buffer to the
// pool. It reports false when the size is unchanged, so a spurious resize event
// does not cost a full repaint.
//
// A real size change cannot be served from the pool, which holds buffers of one
// dimension only: the old buffer goes back (Release drops it, since it no longer
// matches the pool) and the new one is allocated at the requested size. That
// allocation is on a resize frame, which is not the steady state invariant 6 is
// about, and it is cheaper than keeping a free list per size a window has visited.
func (c *Composer) Resize(vp frame.Size) bool {
	if c.Backing != nil && c.Backing.Size() == vp {
		return false
	}
	if c.Backing != nil {
		c.pool.Release(c.Backing)
	}
	if c.pool.Size() == vp {
		c.Backing = c.pool.Acquire()
	} else {
		c.Backing = frame.NewBitmap(int(vp.W), int(vp.H))
	}
	return true
}

// Compose fills the layer background and then the tiles over the given damage,
// and returns the rects that actually changed and are worth presenting.
//
// Two properties are worth stating because the frame gate asserts both. First,
// empty damage writes nothing at all: a vsync with no change costs no pixel
// work, which is the difference between an idle CPU of zero and one core spinning
// on a full repaint. Second, tiles are clipped to each damaged rect rather than
// blitted whole, so a scroll that exposes two rows of pixels touches two rows and
// not the ninety-six visible tiles.
//
// The returned slice is owned by the composer and valid until the next Compose.
// writes, when non-nil, counts pixel writes: it is how CI proves the first
// property without timing anything.
func (c *Composer) Compose(bg frame.Color, tiles []TileBlit, damage []frame.Rect, writes *int64) []frame.Rect {
	out := c.damage[:0]
	vp := c.Backing.Bounds()
	for _, d := range damage {
		r := d.Intersection(vp)
		if r.Empty() {
			continue
		}
		out = append(out, r)
		c.Backing.FillRect(r, bg, writes)
		for i := range tiles {
			t := &tiles[i]
			if t.Pixels == nil || t.Pixels.Empty() {
				continue
			}
			// A tile's local pixel (lx, ly) belongs at Dst.Origin()+(lx, ly), so
			// going from a backing rect to source coordinates subtracts the
			// unclipped Dst origin: subtracting the clipped one would shift every
			// tile that hangs off the top or left edge of the surface.
			blit := t.Dst.Intersection(r)
			if blit.Empty() {
				continue
			}
			src := blit.Translate(-t.Dst.X0, -t.Dst.Y0).Intersection(t.Pixels.Bounds())
			if src.Empty() {
				continue
			}
			c.Backing.BlitOver(t.Pixels, blit.Origin(), src, r, writes)
		}
	}
	c.damage = out
	return out
}

// ShiftY scrolls the backing store vertically by dy device pixels and blits the
// rows already visible back at their new offset. Positive dy means content moves
// up, which is what scrolling down looks like.
//
// This is a memmove over rows, not a repaint: an exposed-height-only cost for a
// scroll. It is also deliberately optional. The composer's correctness never
// depends on ShiftY having run, so a caller can drop it on a platform where the
// window server does the equivalent, and the frame gate's zero-allocation claim
// holds either way. The rows it leaves behind are garbage; the caller must add
// them to the damage list before Compose.
func (c *Composer) ShiftY(dy int32, writes *int64) {
	b := c.Backing
	h := int32(b.H)
	if dy == 0 || dy >= h || -dy >= h {
		return
	}
	rowBytes := b.Stride
	// Bytes per row is a whole number of pixels because Stride is byte-aligned by
	// construction in NewBitmap and in the pool.
	if dy > 0 {
		// Rows [dy, h) move up to [0, h-dy): copy the destination tail from the
		// source tail, one row at a time, forward, since the destination starts
		// before the source.
		if writes != nil {
			*writes += int64(h-dy) * int64(b.W)
		}
		for y := 0; y < int(h-dy); y++ {
			copy(b.RGBA[y*rowBytes:(y+1)*rowBytes], b.RGBA[int(dy)*rowBytes+y*rowBytes:int(dy)*rowBytes+(y+1)*rowBytes])
		}
		return
	}
	if writes != nil {
		*writes += int64(h+dy) * int64(b.W)
	}
	// dy < 0: content moves down, so source rows [0, h+dy) land at [-dy, h).
	// Copy from the bottom up so a destination row is never written before the
	// source row that still needs it has been read.
	for s := int(h+dy) - 1; s >= 0; s-- {
		d := s - int(dy)
		copy(b.RGBA[d*rowBytes:(d+1)*rowBytes], b.RGBA[s*rowBytes:(s+1)*rowBytes])
	}
}

// Clear zeroes the backing store, used when a resize or a scale change makes
// every previously composed pixel wrong.
func (c *Composer) Clear() {
	if c.Backing != nil {
		c.Backing.Reset()
	}
}

// DamageScratch returns the composer's reused damage buffer, so a caller that
// accumulates rects between Compose calls does not allocate.
func (c *Composer) DamageScratch() []frame.Rect { return c.damage[:0] }

package surface

import "github.com/vyquocvu/goosie/internal/frame"

type TileBlit struct {
	Pixels *frame.Bitmap
	Dst    frame.Rect
}

type Composer struct {
	Backing *frame.Bitmap
	pool    *frame.BitmapPool

	damage []frame.Rect
}

func (c *Composer) ReserveDamage(n int) {
	if cap(c.damage) >= n {
		return
	}
	if c.damage == nil {
		c.damage = make([]frame.Rect, 0, n)
		return
	}
	grown := make([]frame.Rect, len(c.damage), n)
	copy(grown, c.damage)
	c.damage = grown
}

func NewComposer(vp frame.Size, pool *frame.BitmapPool) *Composer {
	var b *frame.Bitmap
	if pool.Size() == vp {
		b = pool.Acquire()
	} else {
		b = frame.NewBitmap(int(vp.W), int(vp.H))
	}
	return &Composer{Backing: b, pool: pool}
}

func NewComposerFromBacking(b *frame.Bitmap, pool *frame.BitmapPool) *Composer {
	return &Composer{Backing: b, pool: pool}
}

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

func (c *Composer) ShiftY(dy int32, writes *int64) {
	b := c.Backing
	h := int32(b.H)
	if dy == 0 || dy >= h || -dy >= h {
		return
	}
	rowBytes := b.Stride
	if dy > 0 {
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
	for s := int(h+dy) - 1; s >= 0; s-- {
		d := s - int(dy)
		copy(b.RGBA[d*rowBytes:(d+1)*rowBytes], b.RGBA[s*rowBytes:(s+1)*rowBytes])
	}
}

func (c *Composer) Clear() {
	if c.Backing != nil {
		c.Backing.Reset()
	}
}

func (c *Composer) DamageScratch() []frame.Rect { return c.damage[:0] }

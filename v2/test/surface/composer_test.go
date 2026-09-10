package surface_test

import (
	"testing"
	"unsafe"

	"github.com/vyquocvu/goosie/v2/internal/frame"
	"github.com/vyquocvu/goosie/v2/internal/surface"
)

func newComposer(w, h int32) *surface.Composer {
	return surface.NewComposer(frame.Size{W: w, H: h}, frame.NewBitmapPool(frame.Size{W: w, H: h}, 8))
}

// tile returns an opaque tile buffer of the given edge length filled with c, plus
// the rect it belongs at.
func tile(size int32, at frame.Point, c frame.Color) surface.TileBlit {
	b := frame.NewBitmap(int(size), int(size))
	b.FillRect(b.Bounds(), c, nil)
	return surface.TileBlit{Pixels: b, Dst: frame.RectAt(at, frame.Size{W: size, H: size})}
}

func TestComposeBlitsOverlappingTiles(t *testing.T) {
	c := newComposer(8, 4)
	white := frame.RGB(255, 255, 255)
	red := frame.RGB(255, 0, 0)
	blue := frame.RGB(0, 0, 255)

	tiles := []surface.TileBlit{
		tile(4, frame.Point{X: 0, Y: 0}, red),
		tile(4, frame.Point{X: 2, Y: 0}, blue),
	}
	got := c.Compose(white, tiles, []frame.Rect{frame.Rect4(0, 0, 8, 4)}, nil)
	if len(got) != 1 {
		t.Fatalf("damage list = %v, want the one clipped rect", got)
	}

	b := c.Backing
	// Hand-written expectation: red owns x<2, the second tile wins the overlap at
	// x>=2 because it is later in paint order, and nothing reaches past x<8.
	for x := 0; x < 8; x++ {
		var want frame.Color
		switch {
		case x < 2:
			want = red
		case x < 6:
			want = blue
		default:
			want = white
		}
		for y := 0; y < 4; y++ {
			if got := b.At(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestComposeClipsTileOutsideViewport(t *testing.T) {
	const guard = 32
	// A buffer with padding past its own end: an out-of-bounds write lands there
	// and is detected, rather than corrupting the heap for someone else to debug.
	b := &frame.Bitmap{RGBA: make([]byte, guard*4*4), W: 4, H: 4, Stride: guard * 4}
	for i := range b.RGBA {
		b.RGBA[i] = 0xAA
	}
	pool := frame.NewBitmapPool(b.Size(), 1)
	c := surface.NewComposerFromBacking(b, pool)

	tiles := []surface.TileBlit{tile(8, frame.Point{X: 2, Y: 2}, frame.RGB(1, 2, 3))}
	c.Compose(frame.RGB(9, 9, 9), tiles, []frame.Rect{frame.Rect4(-4, -4, 16, 16)}, nil)

	for i, v := range b.RGBA {
		if v == 0xAA {
			continue
		}
		// Only the 4x4 useful area may change; the guard padding must not.
		x := (i % (guard * 4)) / 4
		y := i / (guard * 4)
		if x >= 4 || y >= 4 {
			t.Fatalf("write escaped the useful area into padding at byte %d (pixel %d,%d)", i, x, y)
		}
	}
	if got := b.At(3, 3); got != frame.RGB(1, 2, 3) {
		t.Fatalf("clipped tile missed the corner it does cover: %v", got)
	}
}

func TestComposeEmptyDamageWritesNothing(t *testing.T) {
	c := newComposer(16, 16)
	var writes int64
	before := snapshot(c.Backing)
	got := c.Compose(frame.RGB(1, 1, 1), []surface.TileBlit{tile(8, frame.Point{}, frame.RGB(2, 2, 2))}, nil, &writes)
	if len(got) != 0 {
		t.Fatalf("empty damage produced %d rects", len(got))
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want 0: an idle vsync must cost no pixel work", writes)
	}
	if !same(before, c.Backing) {
		t.Fatal("Compose with no damage modified the backing store")
	}
}

func TestComposeScrollDamageTouchesOnlyExposedRows(t *testing.T) {
	const h = 100
	c := newComposer(200, h)
	tl := tile(200, frame.Point{}, frame.RGB(1, 2, 3))

	// A 10px scroll exposes the bottom 10 rows. Only those may be touched, and
	// each is written twice: once by the background fill and once by the tile.
	var writes int64
	c.Compose(frame.RGB(255, 255, 255), []surface.TileBlit{tl}, []frame.Rect{frame.Rect4(0, h-10, 200, h)}, &writes)
	if writes != 4000 {
		t.Fatalf("writes = %d, want 4000 (two passes over the exposed 200x10 band, not 2x20000 over the whole surface)", writes)
	}
	if c.Backing.At(0, 0) != frame.TransparentBlack {
		t.Fatal("damage-blit compose repainted a row outside the damage list")
	}
	if got := c.Backing.At(0, h-1); got != frame.RGB(1, 2, 3) {
		t.Fatalf("exposed row = %v, want the tile color", got)
	}
}

func TestComposerReusesBackingAcrossFrames(t *testing.T) {
	c := newComposer(64, 64)
	first := unsafe.Pointer(&c.Backing.RGBA[0])
	for i := 0; i < 20; i++ {
		c.Compose(frame.RGB(0, 0, 0), nil, []frame.Rect{c.Backing.Bounds()}, nil)
	}
	if unsafe.Pointer(&c.Backing.RGBA[0]) != first {
		t.Fatal("the backing store moved; a per-frame viewport allocation breaks invariant 6")
	}
	if n := testing.AllocsPerRun(100, func() {
		c.Compose(frame.RGB(0, 0, 0), nil, []frame.Rect{c.Backing.Bounds()}, nil)
	}); n != 0 {
		// The damage literal stays on the stack because Compose does not retain it,
		// so the composer's own overhead has to be zero for this to read 0.
		t.Fatalf("Compose allocated %v times per frame", n)
	}
}

func TestComposeWithScratchDamageIsAllocationFree(t *testing.T) {
	c := newComposer(64, 64)
	tiles := []surface.TileBlit{tile(64, frame.Point{}, frame.RGB(1, 1, 1))}
	damage := c.DamageScratch()
	damage = append(damage, c.Backing.Bounds())
	var writes int64
	if n := testing.AllocsPerRun(200, func() {
		c.Compose(frame.RGB(0, 0, 0), tiles, damage, &writes)
	}); n != 0 {
		t.Fatalf("Compose allocated %v times per frame with a scratch damage list", n)
	}
}

func TestShiftYMovesRowsAndCountsWrites(t *testing.T) {
	c := newComposer(4, 8)
	b := c.Backing
	for y := 0; y < 8; y++ {
		b.Set(0, y, frame.RGB(byte(y+1), 0, 0))
		b.Set(1, y, frame.RGB(byte(y+1), 0, 0))
		b.Set(2, y, frame.RGB(byte(y+1), 0, 0))
		b.Set(3, y, frame.RGB(byte(y+1), 0, 0))
	}
	var writes int64
	c.ShiftY(3, &writes)
	if writes != 20 {
		t.Fatalf("writes = %d, want 20 (5 moved rows x 4 px)", writes)
	}
	for y := 0; y < 5; y++ {
		if got := b.At(2, y); got != frame.RGB(byte(y+4), 0, 0) {
			t.Fatalf("row %d after shift = %v, want row %d's color", y, got, y+3)
		}
	}
	// Scrolling back up restores the original rows in the region that survives.
	c.ShiftY(-3, nil)
	for y := 3; y < 8; y++ {
		if got := b.At(1, y); got != frame.RGB(byte(y+1), 0, 0) {
			t.Fatalf("row %d after round trip = %v, want its original color", y, got)
		}
	}
	c.ShiftY(0, nil)
	c.ShiftY(1<<20, nil) // must be a no-op, not a panic
}

func TestComposerResizeKeepsBufferPooled(t *testing.T) {
	pool := frame.NewBitmapPool(frame.Size{W: 32, H: 32}, 4)
	c := surface.NewComposer(frame.Size{W: 32, H: 32}, pool)
	first := c.Backing
	if c.Resize(frame.Size{W: 32, H: 32}) {
		t.Fatal("Resize reported a change at the same size; a spurious resize would cost a full repaint")
	}
	created := pool.Stats().Created
	if got := pool.Stats(); got.Released != 0 {
		t.Fatalf("Released = %d after a no-op resize", got.Released)
	}
	c.Clear()
	if first == nil {
		t.Fatal("NewComposer returned no backing store")
	}
	if got := pool.Stats().Created; got != created {
		t.Fatalf("no-op resize allocated %d buffers", got-created)
	}
}

func snapshot(b *frame.Bitmap) []byte {
	out := make([]byte, len(b.RGBA))
	copy(out, b.RGBA)
	return out
}

func same(a []byte, b *frame.Bitmap) bool {
	if len(a) != len(b.RGBA) {
		return false
	}
	for i := range a {
		if a[i] != b.RGBA[i] {
			return false
		}
	}
	return true
}

package compositor

import (
	"image"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// Tile basics
// ---------------------------------------------------------------------------

func TestTileContains(t *testing.T) {
	tile := Tile{Bounds: frame.Rect{X: 100, Y: 200, W: 50, H: 50}}
	tests := []struct {
		name string
		pt   frame.Point
		want bool
	}{
		{"inside", frame.Point{X: 125, Y: 225}, true},
		{"top-left corner", frame.Point{X: 100, Y: 200}, true},
		{"outside right", frame.Point{X: 160, Y: 225}, false},
		{"outside below", frame.Point{X: 125, Y: 260}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tile.Contains(tt.pt); got != tt.want {
				t.Errorf("Contains(%v) = %v, want %v", tt.pt, got, tt.want)
			}
		})
	}
}

func TestTileIntersects(t *testing.T) {
	tile := Tile{Bounds: frame.Rect{X: 0, Y: 0, W: 100, H: 100}}
	tests := []struct {
		name string
		r    frame.Rect
		want bool
	}{
		{"full overlap", frame.Rect{X: 10, Y: 10, W: 80, H: 80}, true},
		{"partial overlap", frame.Rect{X: 50, Y: 50, W: 100, H: 100}, true},
		{"no overlap", frame.Rect{X: 200, Y: 200, W: 10, H: 10}, false},
		{"touching edge", frame.Rect{X: 100, Y: 0, W: 10, H: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tile.Intersects(tt.r); got != tt.want {
				t.Errorf("Intersects(%v) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TileCache — construction and defaults
// ---------------------------------------------------------------------------

func TestDefaultTileCacheConfig(t *testing.T) {
	cfg := DefaultTileCacheConfig()
	if cfg.TileWidth != 256 || cfg.TileHeight != 256 {
		t.Errorf("default tile size = %vx%v, want 256x256", cfg.TileWidth, cfg.TileHeight)
	}
	if cfg.MaxBytes != 32<<20 {
		t.Errorf("default MaxBytes = %d, want %d", cfg.MaxBytes, int64(32<<20))
	}
	if cfg.MaxTiles != 1024 {
		t.Errorf("default MaxTiles = %d, want 1024", cfg.MaxTiles)
	}
}

func TestNewTileCache_SanitizesConfig(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: -1, TileHeight: 0, MaxBytes: -5, MaxTiles: 0})
	cfg := c.Config()
	if cfg.TileWidth != 256 || cfg.TileHeight != 256 {
		t.Errorf("sanitized tile size = %vx%v, want 256x256", cfg.TileWidth, cfg.TileHeight)
	}
	if cfg.MaxBytes != 32<<20 {
		t.Errorf("sanitized MaxBytes = %d, want %d", cfg.MaxBytes, int64(32<<20))
	}
	if cfg.MaxTiles != 1024 {
		t.Errorf("sanitized MaxTiles = %d, want 1024", cfg.MaxTiles)
	}
}

// ---------------------------------------------------------------------------
// TileCache — Get / Put
// ---------------------------------------------------------------------------

func TestTileCache_PutAndGet(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	coord := TileCoord{Col: 1, Row: 2}
	tile := &Tile{
		Coord:    coord,
		Bounds:   c.BoundsForCoord(coord),
		Version:  1,
		Image:    image.NewRGBA(image.Rect(0, 0, 4, 4)),
		ByteSize: 64,
	}

	// Miss before put.
	if got := c.Get(coord); got != nil {
		t.Fatal("Get before Put should return nil")
	}

	c.Put(tile)

	// Hit after put.
	got := c.Get(coord)
	if got == nil {
		t.Fatal("Get after Put should return tile")
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
	if c.Bytes() != 64 {
		t.Errorf("Bytes = %d, want 64", c.Bytes())
	}
}

func TestTileCache_PutUpdatesExisting(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	coord := TileCoord{Col: 0, Row: 0}

	t1 := &Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), Version: 1, ByteSize: 100}
	c.Put(t1)

	t2 := &Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), Version: 2, ByteSize: 200}
	c.Put(t2)

	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1 (should replace, not duplicate)", c.Len())
	}
	if c.Bytes() != 200 {
		t.Errorf("Bytes = %d, want 200", c.Bytes())
	}
	got := c.Get(coord)
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2", got.Version)
	}
}

// ---------------------------------------------------------------------------
// TileCache — eviction by byte budget
// ---------------------------------------------------------------------------

func TestTileCache_EvictsByByteBudget(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 64, TileHeight: 64, MaxBytes: 200, MaxTiles: 100}
	c := NewTileCache(cfg)

	// Insert 3 tiles of 100 bytes each — only 2 fit in 200 byte budget.
	for i := int32(0); i < 3; i++ {
		coord := TileCoord{Col: i, Row: 0}
		c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 100})
	}

	if c.Len() > 2 {
		t.Errorf("Len = %d, want <= 2 (byte budget eviction)", c.Len())
	}
	_, _, evictions := c.Metrics()
	if evictions < 1 {
		t.Errorf("evictions = %d, want >= 1", evictions)
	}
}

// ---------------------------------------------------------------------------
// TileCache — eviction by tile count
// ---------------------------------------------------------------------------

func TestTileCache_EvictsByTileCount(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 64, TileHeight: 64, MaxBytes: 1 << 30, MaxTiles: 3}
	c := NewTileCache(cfg)

	for i := int32(0); i < 5; i++ {
		coord := TileCoord{Col: i, Row: 0}
		c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 1})
	}

	if c.Len() > 3 {
		t.Errorf("Len = %d, want <= 3 (tile count eviction)", c.Len())
	}
}

// ---------------------------------------------------------------------------
// TileCache — LRU eviction order
// ---------------------------------------------------------------------------

func TestTileCache_EvictsLRU(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 64, TileHeight: 64, MaxBytes: 1 << 30, MaxTiles: 2}
	c := NewTileCache(cfg)

	c0 := TileCoord{Col: 0, Row: 0}
	c1 := TileCoord{Col: 1, Row: 0}
	c2 := TileCoord{Col: 2, Row: 0}

	c.Put(&Tile{Coord: c0, Bounds: c.BoundsForCoord(c0), ByteSize: 1})
	c.Put(&Tile{Coord: c1, Bounds: c.BoundsForCoord(c1), ByteSize: 1})

	// Access c0 to make it more recent than c1.
	c.Get(c0)

	// Insert c2 — should evict c1 (least recently used).
	c.Put(&Tile{Coord: c2, Bounds: c.BoundsForCoord(c2), ByteSize: 1})

	if c.Get(c1) != nil {
		t.Error("c1 should have been evicted (LRU)")
	}
	if c.Get(c0) == nil {
		t.Error("c0 should still be present (recently accessed)")
	}
	if c.Get(c2) == nil {
		t.Error("c2 should still be present (just inserted)")
	}
}

// ---------------------------------------------------------------------------
// TileCache — Invalidate
// ---------------------------------------------------------------------------

func TestTileCache_Invalidate(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	coord := TileCoord{Col: 1, Row: 1}
	c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 10})

	if !c.Invalidate(coord) {
		t.Error("Invalidate should return true for existing tile")
	}
	got := c.Get(coord)
	if !got.Dirty {
		t.Error("tile should be dirty after Invalidate")
	}
}

func TestTileCache_InvalidateMissing(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	if c.Invalidate(TileCoord{Col: 99, Row: 99}) {
		t.Error("Invalidate should return false for missing tile")
	}
}

func TestTileCache_InvalidateRect(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 30, MaxTiles: 100}
	c := NewTileCache(cfg)

	// Insert a 3×3 grid of tiles.
	for col := int32(0); col < 3; col++ {
		for row := int32(0); row < 3; row++ {
			coord := TileCoord{Col: col, Row: row}
			c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 1})
		}
	}

	// Invalidate rect overlapping only the first column (x=0..100, y=0..300).
	count := c.InvalidateRect(frame.Rect{X: 0, Y: 0, W: 50, H: 250})
	if count != 3 {
		t.Errorf("InvalidateRect count = %d, want 3", count)
	}

	// Verify correct tiles are dirty.
	for row := int32(0); row < 3; row++ {
		coord := TileCoord{Col: 0, Row: row}
		tile := c.Get(coord)
		if tile == nil || !tile.Dirty {
			t.Errorf("tile (0,%d) should be dirty", row)
		}
	}
	// Tile at (1,0) should NOT be dirty.
	tile := c.Get(TileCoord{Col: 1, Row: 0})
	if tile.Dirty {
		t.Error("tile (1,0) should not be dirty")
	}
}

// ---------------------------------------------------------------------------
// TileCache — Clear and Metrics
// ---------------------------------------------------------------------------

func TestTileCache_Clear(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	c.Put(&Tile{Coord: TileCoord{Col: 0, Row: 0}, ByteSize: 10})
	c.Get(TileCoord{Col: 0, Row: 0}) // hit
	c.Get(TileCoord{Col: 9, Row: 9}) // miss

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes after Clear = %d, want 0", c.Bytes())
	}
	hits, misses, evictions := c.Metrics()
	if hits != 0 || misses != 0 || evictions != 0 {
		t.Errorf("Metrics after Clear = (%d,%d,%d), want (0,0,0)", hits, misses, evictions)
	}
}

func TestTileCache_MetricsTracking(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	coord := TileCoord{Col: 0, Row: 0}
	c.Put(&Tile{Coord: coord, ByteSize: 1})

	c.Get(coord)                     // hit
	c.Get(coord)                     // hit
	c.Get(TileCoord{Col: 5, Row: 5}) // miss

	hits, misses, _ := c.Metrics()
	if hits != 2 {
		t.Errorf("hits = %d, want 2", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

// ---------------------------------------------------------------------------
// CoordForPoint / BoundsForCoord / CoordsInRect
// ---------------------------------------------------------------------------

func TestCoordForPoint(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 50, MaxBytes: 1 << 20, MaxTiles: 100})
	tests := []struct {
		pt   frame.Point
		want TileCoord
	}{
		{frame.Point{X: 0, Y: 0}, TileCoord{0, 0}},
		{frame.Point{X: 99, Y: 49}, TileCoord{0, 0}},
		{frame.Point{X: 100, Y: 50}, TileCoord{1, 1}},
		{frame.Point{X: 250, Y: 125}, TileCoord{2, 2}},
	}
	for _, tt := range tests {
		got := c.CoordForPoint(tt.pt)
		if got != tt.want {
			t.Errorf("CoordForPoint(%v) = %v, want %v", tt.pt, got, tt.want)
		}
	}
}

func TestBoundsForCoord(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 50, MaxBytes: 1 << 20, MaxTiles: 100})
	b := c.BoundsForCoord(TileCoord{Col: 2, Row: 3})
	want := frame.Rect{X: 200, Y: 150, W: 100, H: 50}
	if b != want {
		t.Errorf("BoundsForCoord = %v, want %v", b, want)
	}
}

func TestCoordsInRect(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 20, MaxTiles: 100})

	// Rect spanning 2×2 tiles.
	coords := c.CoordsInRect(frame.Rect{X: 50, Y: 50, W: 150, H: 150})
	want := map[TileCoord]bool{
		{0, 0}: true, {1, 0}: true,
		{0, 1}: true, {1, 1}: true,
	}
	if len(coords) != len(want) {
		t.Fatalf("CoordsInRect len = %d, want %d", len(coords), len(want))
	}
	for _, c := range coords {
		if !want[c] {
			t.Errorf("unexpected coord %v", c)
		}
	}
}

func TestCoordsInRect_SingleTile(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 20, MaxTiles: 100})
	coords := c.CoordsInRect(frame.Rect{X: 10, Y: 10, W: 50, H: 50})
	if len(coords) != 1 {
		t.Errorf("len = %d, want 1", len(coords))
	}
	if coords[0] != (TileCoord{0, 0}) {
		t.Errorf("coord = %v, want (0,0)", coords[0])
	}
}

// ---------------------------------------------------------------------------
// PriorityForCoord
// ---------------------------------------------------------------------------

func TestPriorityForCoord(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 20, MaxTiles: 100})
	vp := frame.Viewport{Width: 200, Height: 200, ScrollX: 0, ScrollY: 0}

	// Tile (0,0) — fully visible.
	p := c.PriorityForCoord(TileCoord{0, 0}, vp, 50)
	if p != TilePriorityVisible {
		t.Errorf("(0,0) priority = %d, want Visible", p)
	}

	// Tile (2,2) — bounds [200,200,300,300], visible [0,0,200,200].
	// Touching edge — not intersecting visible rect interior.
	// With 50px margin, expanded visible = [-50,-50,300,300] — should be Near.
	p = c.PriorityForCoord(TileCoord{2, 2}, vp, 50)
	if p != TilePriorityNear {
		t.Errorf("(2,2) priority = %d, want Near", p)
	}

	// Tile (10,10) — far away, even with margin.
	p = c.PriorityForCoord(TileCoord{10, 10}, vp, 50)
	if p != TilePriorityHidden {
		t.Errorf("(10,10) priority = %d, want Hidden", p)
	}
}

func TestPriorityForCoord_WithScroll(t *testing.T) {
	c := NewTileCache(TileCacheConfig{TileWidth: 100, TileHeight: 100, MaxBytes: 1 << 20, MaxTiles: 100})
	vp := frame.Viewport{Width: 200, Height: 200, ScrollX: 500, ScrollY: 500}

	// Tile (5,5) — bounds [500,500,600,600] — visible.
	p := c.PriorityForCoord(TileCoord{5, 5}, vp, 0)
	if p != TilePriorityVisible {
		t.Errorf("(5,5) with scroll priority = %d, want Visible", p)
	}
}

// ---------------------------------------------------------------------------
// TileCache — Evict (memory.Evictor interface)
// ---------------------------------------------------------------------------

func TestTileCache_Evict_FreesTargetBytes(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 64, TileHeight: 64, MaxBytes: 1 << 30, MaxTiles: 1000}
	c := NewTileCache(cfg)

	// Insert 5 tiles of 100 bytes each = 500 bytes.
	for i := int32(0); i < 5; i++ {
		coord := TileCoord{Col: i, Row: 0}
		c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 100})
	}
	if c.Bytes() != 500 {
		t.Fatalf("Bytes before Evict = %d, want 500", c.Bytes())
	}

	// Evict 250 bytes — should remove at least 2 tiles (200 bytes) but
	// likely 3 tiles due to LRU loop (each 100 bytes).
	freed := c.Evict(250)
	if freed < 200 {
		t.Errorf("freed = %d, want >= 200", freed)
	}
	if c.Bytes() > 300 {
		t.Errorf("Bytes after Evict(250) = %d, want <= 300", c.Bytes())
	}
}

func TestTileCache_Evict_EmptyCache(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	freed := c.Evict(1000)
	if freed != 0 {
		t.Errorf("freed from empty cache = %d, want 0", freed)
	}
}

func TestTileCache_Evict_All(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	for i := int32(0); i < 3; i++ {
		coord := TileCoord{Col: i, Row: 0}
		c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 50})
	}

	freed := c.Evict(1 << 30) // Request more than total.
	if freed != 150 {
		t.Errorf("freed = %d, want 150 (all tiles)", freed)
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes = %d, want 0", c.Bytes())
	}
}

func TestTileCache_Evict_LRUOrder(t *testing.T) {
	cfg := TileCacheConfig{TileWidth: 64, TileHeight: 64, MaxBytes: 1 << 30, MaxTiles: 100}
	c := NewTileCache(cfg)

	c0 := TileCoord{Col: 0, Row: 0}
	c1 := TileCoord{Col: 1, Row: 0}
	c2 := TileCoord{Col: 2, Row: 0}

	c.Put(&Tile{Coord: c0, Bounds: c.BoundsForCoord(c0), ByteSize: 100})
	c.Put(&Tile{Coord: c1, Bounds: c.BoundsForCoord(c1), ByteSize: 100})
	c.Put(&Tile{Coord: c2, Bounds: c.BoundsForCoord(c2), ByteSize: 100})

	// Access c0 to make it most recent.
	c.Get(c0)

	// Evict 100 bytes — should remove c1 (oldest).
	freed := c.Evict(100)
	if freed != 100 {
		t.Errorf("freed = %d, want 100", freed)
	}
	if c.Get(c1) != nil {
		t.Error("c1 should have been evicted (LRU)")
	}
	if c.Get(c0) == nil {
		t.Error("c0 should still be present (recently accessed)")
	}
	if c.Get(c2) == nil {
		t.Error("c2 should still be present")
	}
}

func TestTileCache_Evict_Metrics(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	for i := int32(0); i < 4; i++ {
		coord := TileCoord{Col: i, Row: 0}
		c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 10})
	}

	c.Evict(25) // Should evict at least 3 tiles.
	_, _, evictions := c.Metrics()
	if evictions < 3 {
		t.Errorf("evictions = %d, want >= 3", evictions)
	}
}

// ---------------------------------------------------------------------------
// TileCache — Close
// ---------------------------------------------------------------------------

func TestTileCache_Close(t *testing.T) {
	c := NewTileCache(DefaultTileCacheConfig())
	c.Put(&Tile{Coord: TileCoord{Col: 0, Row: 0}, ByteSize: 10})
	c.Get(TileCoord{Col: 0, Row: 0}) // hit
	c.Get(TileCoord{Col: 9, Row: 9}) // miss

	c.Close()

	if c.Len() != 0 {
		t.Errorf("Len after Close = %d, want 0", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("Bytes after Close = %d, want 0", c.Bytes())
	}
	hits, misses, evictions := c.Metrics()
	if hits != 0 || misses != 0 || evictions != 0 {
		t.Errorf("Metrics after Close = (%d,%d,%d), want (0,0,0)", hits, misses, evictions)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkTileCache_Put(b *testing.B) {
	c := NewTileCache(TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		coord := TileCoord{Col: int32(i % 100), Row: int32(i / 100)}
		c.Put(&Tile{Coord: coord, ByteSize: 1})
	}
}

func BenchmarkTileCache_Get_Hit(b *testing.B) {
	c := NewTileCache(TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000})
	for i := int32(0); i < 100; i++ {
		c.Put(&Tile{Coord: TileCoord{Col: i, Row: 0}, ByteSize: 1})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Get(TileCoord{Col: int32(i % 100), Row: 0})
	}
}

func BenchmarkTileCache_Get_Miss(b *testing.B) {
	c := NewTileCache(DefaultTileCacheConfig())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Get(TileCoord{Col: int32(i), Row: int32(i)})
	}
}

func BenchmarkCoordForPoint(b *testing.B) {
	c := NewTileCache(DefaultTileCacheConfig())
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.CoordForPoint(frame.Point{X: float32(i % 1000), Y: float32(i / 1000)})
	}
}

func BenchmarkCoordsInRect(b *testing.B) {
	c := NewTileCache(DefaultTileCacheConfig())
	r := frame.Rect{X: 0, Y: 0, W: 1024, H: 768}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.CoordsInRect(r)
	}
}

func BenchmarkTileCache_Evict(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cfg := TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000}
		c := NewTileCache(cfg)
		for j := int32(0); j < 256; j++ {
			c.Put(&Tile{Coord: TileCoord{Col: j, Row: 0}, ByteSize: 4096})
		}
		b.StartTimer()
		c.Evict(1 << 20) // Evict 1 MB (256 tiles × 4096 bytes).
	}
}

func BenchmarkInvalidateRect(b *testing.B) {
	c := NewTileCache(TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000})
	for col := int32(0); col < 20; col++ {
		for row := int32(0); row < 20; row++ {
			coord := TileCoord{Col: col, Row: row}
			c.Put(&Tile{Coord: coord, Bounds: c.BoundsForCoord(coord), ByteSize: 1})
		}
	}
	r := frame.Rect{X: 100, Y: 100, W: 500, H: 500}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.InvalidateRect(r)
	}
}

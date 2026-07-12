package compositor

import (
	"image"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// SceneSnapshot — immutability and staleness
// ---------------------------------------------------------------------------

func TestSceneSnapshot_IsStale(t *testing.T) {
	snap := SceneSnapshot{Generation: 5}
	if snap.IsStale(5) {
		t.Error("generation 5 should not be stale at gen 5")
	}
	if snap.IsStale(3) {
		t.Error("generation 5 should not be stale at gen 3")
	}
	if !snap.IsStale(6) {
		t.Error("generation 5 should be stale at gen 6")
	}
}

func TestSceneSnapshot_FindTile(t *testing.T) {
	snap := SceneSnapshot{
		Tiles: []SnapshotTile{
			{Coord: TileCoord{Col: 0, Row: 0}, Version: 1},
			{Coord: TileCoord{Col: 1, Row: 0}, Version: 2},
			{Coord: TileCoord{Col: 0, Row: 1}, Version: 3},
		},
	}

	found := snap.FindTile(TileCoord{Col: 1, Row: 0})
	if found == nil {
		t.Fatal("FindTile should find (1,0)")
	}
	if found.Version != 2 {
		t.Errorf("Version = %d, want 2", found.Version)
	}

	missing := snap.FindTile(TileCoord{Col: 5, Row: 5})
	if missing != nil {
		t.Error("FindTile should return nil for missing coord")
	}
}

func TestSceneSnapshot_VisibleTiles(t *testing.T) {
	snap := SceneSnapshot{
		Viewport: frame.Viewport{Width: 100, Height: 100, ScrollX: 0, ScrollY: 0},
		Tiles: []SnapshotTile{
			{Coord: TileCoord{Col: 0, Row: 0}, Bounds: frame.Rect{X: 0, Y: 0, W: 100, H: 100}},
			{Coord: TileCoord{Col: 1, Row: 0}, Bounds: frame.Rect{X: 100, Y: 0, W: 100, H: 100}},
			{Coord: TileCoord{Col: 0, Row: 1}, Bounds: frame.Rect{X: 0, Y: 100, W: 100, H: 100}},
			{Coord: TileCoord{Col: 5, Row: 5}, Bounds: frame.Rect{X: 500, Y: 500, W: 100, H: 100}},
		},
	}

	visible := snap.VisibleTiles()
	// Only (0,0) intersects [0,0,100,100].
	if len(visible) != 1 {
		t.Fatalf("VisibleTiles len = %d, want 1", len(visible))
	}
	if visible[0].Coord != (TileCoord{Col: 0, Row: 0}) {
		t.Errorf("visible tile = %v, want (0,0)", visible[0].Coord)
	}
}

func TestSceneSnapshot_VisibleTilesWithScroll(t *testing.T) {
	snap := SceneSnapshot{
		Viewport: frame.Viewport{Width: 100, Height: 100, ScrollX: 100, ScrollY: 0},
		Tiles: []SnapshotTile{
			{Coord: TileCoord{Col: 0, Row: 0}, Bounds: frame.Rect{X: 0, Y: 0, W: 100, H: 100}},
			{Coord: TileCoord{Col: 1, Row: 0}, Bounds: frame.Rect{X: 100, Y: 0, W: 100, H: 100}},
		},
	}

	visible := snap.VisibleTiles()
	// Visible rect is [100,0,200,100] — only (1,0) intersects.
	if len(visible) != 1 {
		t.Fatalf("VisibleTiles len = %d, want 1", len(visible))
	}
	if visible[0].Coord != (TileCoord{Col: 1, Row: 0}) {
		t.Errorf("visible tile = %v, want (1,0)", visible[0].Coord)
	}
}

// ---------------------------------------------------------------------------
// SnapshotPublisher — publish and generation tracking
// ---------------------------------------------------------------------------

func TestSnapshotPublisher_PublishBumpsGeneration(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	pub := NewSnapshotPublisher(cache)

	if pub.CurrentGeneration() != 0 {
		t.Errorf("initial generation = %d, want 0", pub.CurrentGeneration())
	}

	vp := frame.Viewport{Width: 800, Height: 600}
	snap1 := pub.Publish(vp)
	if snap1.Generation != 1 {
		t.Errorf("snap1.Generation = %d, want 1", snap1.Generation)
	}

	snap2 := pub.Publish(vp)
	if snap2.Generation != 2 {
		t.Errorf("snap2.Generation = %d, want 2", snap2.Generation)
	}

	if pub.CurrentGeneration() != 2 {
		t.Errorf("CurrentGeneration = %d, want 2", pub.CurrentGeneration())
	}
}

func TestSnapshotPublisher_PublishCopiesTiles(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	coord := TileCoord{Col: 0, Row: 0}
	cache.Put(&Tile{
		Coord:    coord,
		Bounds:   cache.BoundsForCoord(coord),
		Version:  42,
		Image:    image.NewRGBA(image.Rect(0, 0, 4, 4)),
		ByteSize: 64,
	})

	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 800, Height: 600}
	snap := pub.Publish(vp)

	if len(snap.Tiles) != 1 {
		t.Fatalf("snap.Tiles len = %d, want 1", len(snap.Tiles))
	}
	if snap.Tiles[0].Version != 42 {
		t.Errorf("snap tile Version = %d, want 42", snap.Tiles[0].Version)
	}
	if snap.Tiles[0].ByteSize != 64 {
		t.Errorf("snap tile ByteSize = %d, want 64", snap.Tiles[0].ByteSize)
	}

	// Mutating cache after publish should NOT affect snapshot.
	cache.Invalidate(coord)
	snap2 := pub.Publish(vp)
	if snap2.Tiles[0].Dirty != true {
		t.Error("new snapshot should reflect dirty state")
	}
	if snap.Tiles[0].Dirty != false {
		t.Error("old snapshot should be immutable")
	}
}

func TestSnapshotPublisher_RejectStale(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 800, Height: 600}

	snap1 := pub.Publish(vp)
	if pub.RejectStale(&snap1) {
		t.Error("snap1 should not be stale yet")
	}

	pub.Publish(vp) // gen 2
	if !pub.RejectStale(&snap1) {
		t.Error("snap1 should be stale after gen 2")
	}
}

func TestSnapshotPublisher_ContentHashChanges(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 800, Height: 600}

	snap1 := pub.Publish(vp)

	// Different viewport → different hash.
	vp2 := frame.Viewport{Width: 800, Height: 600, ScrollY: 100}
	snap2 := pub.Publish(vp2)

	if snap1.ContentHash == snap2.ContentHash {
		t.Error("different viewports should produce different hashes")
	}
}

func TestSnapshotPublisher_EmptyCache(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 800, Height: 600}

	snap := pub.Publish(vp)
	if len(snap.Tiles) != 0 {
		t.Errorf("empty cache should produce empty snapshot, got %d tiles", len(snap.Tiles))
	}
	if snap.Generation != 1 {
		t.Errorf("generation = %d, want 1", snap.Generation)
	}
}

// ---------------------------------------------------------------------------
// Concurrency — concurrent publish and read
// ---------------------------------------------------------------------------

func TestSnapshotPublisher_ConcurrentPublish(t *testing.T) {
	cache := NewTileCache(DefaultTileCacheConfig())
	for i := int32(0); i < 10; i++ {
		coord := TileCoord{Col: i, Row: 0}
		cache.Put(&Tile{Coord: coord, Bounds: cache.BoundsForCoord(coord), ByteSize: 1})
	}

	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 800, Height: 600}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				snap := pub.Publish(vp)
				if snap.Generation == 0 {
					t.Errorf("generation should never be 0 after Publish")
				}
			}
		}()
	}
	wg.Wait()

	if pub.CurrentGeneration() != 800 {
		t.Errorf("generation = %d, want 800", pub.CurrentGeneration())
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkSnapshotPublisher_Publish(b *testing.B) {
	cache := NewTileCache(TileCacheConfig{TileWidth: 256, TileHeight: 256, MaxBytes: 1 << 30, MaxTiles: 10000})
	for col := int32(0); col < 16; col++ {
		for row := int32(0); row < 16; row++ {
			coord := TileCoord{Col: col, Row: row}
			cache.Put(&Tile{Coord: coord, Bounds: cache.BoundsForCoord(coord), ByteSize: 1, Version: 1})
		}
	}
	pub := NewSnapshotPublisher(cache)
	vp := frame.Viewport{Width: 1024, Height: 768}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pub.Publish(vp)
	}
}

func BenchmarkSceneSnapshot_VisibleTiles(b *testing.B) {
	tiles := make([]SnapshotTile, 256)
	for i := range tiles {
		tiles[i] = SnapshotTile{
			Coord:  TileCoord{Col: int32(i % 16), Row: int32(i / 16)},
			Bounds: frame.Rect{X: float32(i % 16 * 256), Y: float32(i / 16 * 256), W: 256, H: 256},
		}
	}
	snap := SceneSnapshot{
		Viewport: frame.Viewport{Width: 1024, Height: 768},
		Tiles:    tiles,
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		snap.VisibleTiles()
	}
}

func BenchmarkSceneSnapshot_FindTile(b *testing.B) {
	tiles := make([]SnapshotTile, 256)
	for i := range tiles {
		tiles[i] = SnapshotTile{Coord: TileCoord{Col: int32(i % 16), Row: int32(i / 16)}}
	}
	snap := SceneSnapshot{Tiles: tiles}
	target := TileCoord{Col: 15, Row: 15} // worst case
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		snap.FindTile(target)
	}
}

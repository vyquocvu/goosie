package compositor

import (
	"image"
	"sync"
	"sync/atomic"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// SceneSnapshot — immutable view of the compositor state at a point in time
// ---------------------------------------------------------------------------

// SceneSnapshot is an immutable snapshot of the compositor state at a
// specific generation. Presentation layers can read snapshots without
// locking mutable layout state. Stale snapshots are rejected via
// generation ID comparison.
type SceneSnapshot struct {
	// Generation is a monotonically increasing ID. Higher = newer.
	Generation uint64

	// Viewport is the viewport state at snapshot time.
	Viewport frame.Viewport

	// Tiles contains the tile entries valid at snapshot time.
	// Each entry is a copy of the tile metadata; the Image pointer
	// is shared (immutable after publication).
	Tiles []SnapshotTile

	// ContentHash is a fast equality check for frame content.
	ContentHash uint64
}

// SnapshotTile is an immutable view of a single tile within a snapshot.
type SnapshotTile struct {
	Coord    TileCoord
	Bounds   frame.Rect
	Version  uint64
	Dirty    bool
	Image    *image.RGBA
	ByteSize int64
}

// IsStale reports whether this snapshot is older than the given generation.
func (s *SceneSnapshot) IsStale(currentGeneration uint64) bool {
	return s.Generation < currentGeneration
}

// VisibleTiles returns tiles that overlap the snapshot's viewport.
func (s *SceneSnapshot) VisibleTiles() []SnapshotTile {
	vr := s.Viewport.VisibleRect()
	result := make([]SnapshotTile, 0, len(s.Tiles))
	for _, t := range s.Tiles {
		if t.Bounds.Intersects(vr) {
			result = append(result, t)
		}
	}
	return result
}

// FindTile returns the snapshot tile for the given coordinate, or nil.
func (s *SceneSnapshot) FindTile(coord TileCoord) *SnapshotTile {
	for i := range s.Tiles {
		if s.Tiles[i].Coord == coord {
			return &s.Tiles[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// SnapshotPublisher — creates snapshots from mutable TileCache state
// ---------------------------------------------------------------------------

// SnapshotPublisher creates immutable SceneSnapshots from the mutable
// TileCache. It maintains a generation counter and supports concurrent
// snapshot creation (readers) while the cache is being mutated (writer).
type SnapshotPublisher struct {
	mu         sync.Mutex
	generation atomic.Uint64
	cache      *TileCache
}

// NewSnapshotPublisher creates a publisher backed by the given tile cache.
func NewSnapshotPublisher(cache *TileCache) *SnapshotPublisher {
	return &SnapshotPublisher{
		cache: cache,
	}
}

// Publish creates an immutable SceneSnapshot from the current tile cache
// state. The generation counter is bumped atomically.
func (p *SnapshotPublisher) Publish(vp frame.Viewport) SceneSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	gen := p.generation.Add(1)

	// Copy tile metadata under cache lock.
	p.cache.mu.Lock()
	tiles := make([]SnapshotTile, 0, len(p.cache.tiles))
	for _, t := range p.cache.tiles {
		tiles = append(tiles, SnapshotTile{
			Coord:    t.Coord,
			Bounds:   t.Bounds,
			Version:  t.Version,
			Dirty:    t.Dirty,
			Image:    t.Image,
			ByteSize: t.ByteSize,
		})
	}
	p.cache.mu.Unlock()

	// Compute content hash from tile versions.
	var hash uint64
	for _, t := range tiles {
		hash = hash*31 + t.Version
	}
	hash = hash*31 + uint64(vp.ScrollX*1000) + uint64(vp.ScrollY*1000)

	return SceneSnapshot{
		Generation:  gen,
		Viewport:    vp,
		Tiles:       tiles,
		ContentHash: hash,
	}
}

// CurrentGeneration returns the latest generation number.
func (p *SnapshotPublisher) CurrentGeneration() uint64 {
	return p.generation.Load()
}

// RejectStale compares a snapshot's generation against the current one.
// Returns true if the snapshot is stale and should be discarded.
func (p *SnapshotPublisher) RejectStale(snap *SceneSnapshot) bool {
	return snap.IsStale(p.generation.Load())
}

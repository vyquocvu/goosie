// Package compositor provides tile-based retained rendering for smooth scrolling.
//
// M7.1: Divides content into configurable raster tiles, tracks tile content
// versions, reuses valid tiles across frames, prioritizes visible tiles,
// and evicts by byte budget and recency.
package compositor

import (
	"image"
	"sync"
	"sync/atomic"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
)

// ---------------------------------------------------------------------------
// Tile — a single retained raster tile
// ---------------------------------------------------------------------------

// TileCoord identifies a tile by its column and row in the tile grid.
type TileCoord struct {
	Col int32
	Row int32
}

// Tile is a single retained raster tile holding a portion of the rendered
// frame. Tiles are the unit of reuse: unchanged tiles are not re-rasterized.
type Tile struct {
	Coord    TileCoord
	Bounds   frame.Rect // Layout-space bounds
	Version  uint64     // Content version — bumped when tile is invalidated
	LastUsed uint64     // Frame number when last accessed (for LRU eviction)
	Image    *image.RGBA
	Dirty    bool
	ByteSize int64
}

// Contains reports whether the tile contains the given layout-space point.
func (t *Tile) Contains(p frame.Point) bool {
	return t.Bounds.Contains(p)
}

// Intersects reports whether the tile overlaps the given rect.
func (t *Tile) Intersects(r frame.Rect) bool {
	return t.Bounds.Intersects(r)
}

// ---------------------------------------------------------------------------
// TileCache — bounded cache of retained tiles
// ---------------------------------------------------------------------------

// TileCacheConfig configures the tile cache.
type TileCacheConfig struct {
	// TileWidth and TileHeight are the layout-space dimensions of each tile.
	TileWidth  float32
	TileHeight float32

	// MaxBytes is the maximum total byte budget for cached tiles.
	MaxBytes int64

	// MaxTiles is the maximum number of cached tiles.
	MaxTiles int
}

// DefaultTileCacheConfig returns sensible defaults: 256×256 tiles, 32 MB budget.
func DefaultTileCacheConfig() TileCacheConfig {
	return TileCacheConfig{
		TileWidth:  256,
		TileHeight: 256,
		MaxBytes:   32 << 20, // 32 MB
		MaxTiles:   1024,
	}
}

// TileCache manages a bounded set of retained tiles with LRU eviction.
type TileCache struct {
	mu     sync.Mutex
	config TileCacheConfig
	tiles  map[TileCoord]*Tile

	// currentBytes tracks total memory used by tile images.
	currentBytes int64

	// frameCounter is a monotonic counter for LRU tracking.
	frameCounter uint64

	// Metrics.
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// NewTileCache creates a tile cache with the given configuration.
func NewTileCache(cfg TileCacheConfig) *TileCache {
	if cfg.TileWidth <= 0 {
		cfg.TileWidth = 256
	}
	if cfg.TileHeight <= 0 {
		cfg.TileHeight = 256
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 32 << 20
	}
	if cfg.MaxTiles <= 0 {
		cfg.MaxTiles = 1024
	}
	return &TileCache{
		config: cfg,
		tiles:  make(map[TileCoord]*Tile, 64),
	}
}

// Get retrieves a tile by coordinate. Returns nil on miss.
// Updates the tile's LastUsed timestamp for LRU tracking.
func (c *TileCache) Get(coord TileCoord) *Tile {
	c.mu.Lock()
	defer c.mu.Unlock()

	t, ok := c.tiles[coord]
	if !ok {
		c.misses.Add(1)
		return nil
	}
	c.hits.Add(1)
	c.frameCounter++
	t.LastUsed = c.frameCounter
	return t
}

// Put inserts or updates a tile in the cache. Evicts LRU tiles if needed.
func (c *TileCache) Put(t *Tile) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.frameCounter++
	t.LastUsed = c.frameCounter

	// Remove old entry if updating.
	if old, ok := c.tiles[t.Coord]; ok {
		c.currentBytes -= old.ByteSize
	}

	// Evict until within budget.
	for c.currentBytes+t.ByteSize > c.config.MaxBytes && len(c.tiles) > 0 {
		c.evictLRU()
	}
	for len(c.tiles) >= c.config.MaxTiles {
		c.evictLRU()
	}

	c.tiles[t.Coord] = t
	c.currentBytes += t.ByteSize
}

// Invalidate marks a tile dirty by coordinate. Returns true if found.
func (c *TileCache) Invalidate(coord TileCoord) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	t, ok := c.tiles[coord]
	if !ok {
		return false
	}
	t.Dirty = true
	return true
}

// InvalidateRect marks all tiles overlapping the given rect as dirty.
func (c *TileCache) InvalidateRect(r frame.Rect) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for _, t := range c.tiles {
		if t.Bounds.Intersects(r) {
			t.Dirty = true
			count++
		}
	}
	return count
}

// Len returns the number of cached tiles.
func (c *TileCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.tiles)
}

// Bytes returns the current total byte usage.
func (c *TileCache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentBytes
}

// Clear removes all tiles and resets metrics.
func (c *TileCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tiles = make(map[TileCoord]*Tile, 64)
	c.currentBytes = 0
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
}

// Metrics returns cache hit/miss/eviction counts.
func (c *TileCache) Metrics() (hits, misses, evictions int64) {
	return c.hits.Load(), c.misses.Load(), c.evictions.Load()
}

// Config returns the cache configuration.
func (c *TileCache) Config() TileCacheConfig {
	return c.config
}

// evictLRU removes the least recently used tile. Must be called with mu held.
func (c *TileCache) evictLRU() {
	var oldest *Tile
	for _, t := range c.tiles {
		if oldest == nil || t.LastUsed < oldest.LastUsed {
			oldest = t
		}
	}
	if oldest == nil {
		return
	}
	c.currentBytes -= oldest.ByteSize
	delete(c.tiles, oldest.Coord)
	c.evictions.Add(1)
}

// ---------------------------------------------------------------------------
// TileCoordFromPoint — maps layout-space point to tile coordinate
// ---------------------------------------------------------------------------

// CoordForPoint returns the tile coordinate containing the given point.
func (c *TileCache) CoordForPoint(p frame.Point) TileCoord {
	return TileCoord{
		Col: int32(p.X / c.config.TileWidth),
		Row: int32(p.Y / c.config.TileHeight),
	}
}

// BoundsForCoord returns the layout-space bounds for a tile coordinate.
func (c *TileCache) BoundsForCoord(coord TileCoord) frame.Rect {
	return frame.Rect{
		X: float32(coord.Col) * c.config.TileWidth,
		Y: float32(coord.Row) * c.config.TileHeight,
		W: c.config.TileWidth,
		H: c.config.TileHeight,
	}
}

// CoordsInRect returns all tile coordinates that overlap the given rect.
// The rect is treated as half-open [X, X+W) × [Y, Y+H).
func (c *TileCache) CoordsInRect(r frame.Rect) []TileCoord {
	minCol := int32(r.X / c.config.TileWidth)
	minRow := int32(r.Y / c.config.TileHeight)

	right := r.X + r.W
	bottom := r.Y + r.H
	// Adjust for exact tile boundaries (half-open interval).
	maxCol := int32((right - 0.001) / c.config.TileWidth)
	maxRow := int32((bottom - 0.001) / c.config.TileHeight)
	if maxCol < minCol {
		maxCol = minCol
	}
	if maxRow < minRow {
		maxRow = minRow
	}

	coords := make([]TileCoord, 0, (maxCol-minCol+1)*(maxRow-minRow+1))
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			coords = append(coords, TileCoord{Col: col, Row: row})
		}
	}
	return coords
}

// ---------------------------------------------------------------------------
// TilePriority — visible, near-visible, hidden
// ---------------------------------------------------------------------------

// TilePriority indicates the rendering priority for a tile.
type TilePriority uint8

const (
	// TilePriorityVisible is for tiles currently in the viewport.
	TilePriorityVisible TilePriority = iota
	// TilePriorityNear is for tiles within the prefetch margin.
	TilePriorityNear
	// TilePriorityHidden is for tiles outside the viewport and margin.
	TilePriorityHidden
)

// PriorityForCoord returns the rendering priority for a tile coordinate
// given the current viewport and prefetch margin.
func (c *TileCache) PriorityForCoord(coord TileCoord, viewport frame.Viewport, prefetchMargin float32) TilePriority {
	bounds := c.BoundsForCoord(coord)
	visible := viewport.VisibleRect()
	expanded := visible.Expand(prefetchMargin)

	if bounds.Intersects(visible) {
		return TilePriorityVisible
	}
	if bounds.Intersects(expanded) {
		return TilePriorityNear
	}
	return TilePriorityHidden
}

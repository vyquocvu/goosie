package frame

import "fmt"

// TileSize is the edge length in device pixels of every tile in v2. It is a
// single constant rather than a per-layer parameter on purpose: one knob, and
// the budget arithmetic in the design document is stated in terms of it.
//
// 256 device px is 128 CSS px at DPR 2, which keeps a tile small enough that
// invalidating one costs little and large enough that per-tile overhead
// (map entry, LRU node, buffer header) stays negligible against its 256 KB of
// pixels. At a 3024x1900 viewport this yields 12x8 = 96 visible tiles.
const TileSize int32 = 256

// TileSizeBytes returns the buffer cost of one tile at this size.
func TileSizeBytes() int64 { return int64(TileSize) * int64(TileSize) * 4 }

// TileCoord names a tile in the infinite grid. Columns and rows are signed
// because content can live at negative content-space coordinates under
// transforms and negative margins.
type TileCoord struct{ Col, Row int32 }

// CoordFor maps a content-space point to its tile, using floor division so that
// negative coordinates land in the tile that actually covers them rather than
// collapsing toward the origin.
func CoordFor(p Point, tileSize int32) TileCoord {
	if tileSize <= 0 {
		return TileCoord{}
	}
	return TileCoord{Col: floorDiv(p.X, tileSize), Row: floorDiv(p.Y, tileSize)}
}

// Rect returns the tile's content-space bounds.
func (c TileCoord) Rect(tileSize int32) Rect {
	x0, y0 := c.Col*tileSize, c.Row*tileSize
	return Rect4(x0, y0, x0+tileSize, y0+tileSize)
}

func (c TileCoord) String() string { return fmt.Sprintf("T(%d,%d)", c.Col, c.Row) }

// TileState is a tile's lifecycle position.
type TileState uint8

const (
	// TileEmpty has no pixels: never rasterized, or evicted from the budget.
	TileEmpty TileState = iota
	// TileValid holds pixels current with the layer's content version.
	TileValid
	// TileStale is awaiting (or undergoing) re-rasterization. Stale tiles keep
	// their old pixels: that is what lets the UI thread present a slightly out
	// of date tile instead of blocking on the raster pool.
	TileStale
	// TileFailed exhausted its retries and renders blank with a counter.
	TileFailed
)

func (s TileState) String() string {
	switch s {
	case TileEmpty:
		return "empty"
	case TileValid:
		return "valid"
	case TileStale:
		return "stale"
	case TileFailed:
		return "failed"
	}
	return "unknown"
}

// MaxTileAttempts is how many times one tile may be rasterized before it is
// given up on, per the failure policy: retry once, then blank with a counter.
const MaxTileAttempts = 2

// Tile is one retained raster. Pixels is owned by the grid and borrowed from a
// BitmapPool; the grid is the only thing that releases it.
type Tile struct {
	Coord    TileCoord
	Bounds   Rect
	Version  uint64
	LastUsed uint64
	Pixels   *Bitmap
	Bytes    int64
	State    TileState
	Attempts int

	// prev/next thread the grid's LRU list through the tile itself. An intrusive
	// list is deliberate: container/list allocates an element per insertion, and
	// recency is touched every visible tile every frame, so a boxed list would
	// break invariant 6 during ordinary scrolling.
	prev, next *Tile
}

// ValidAt reports the tile's half of the spec's validity rule: current pixels
// for the given layer content version.
func (t *Tile) ValidAt(layerVersion uint64) bool {
	return t != nil && t.State == TileValid && t.Version >= layerVersion && t.Pixels != nil
}

// Blittable reports whether the tile has pixels worth presenting, valid or not.
// A stale tile is deliberately still shown while being re-rasterized: one late
// tile is a cosmetic artifact, blocking the UI thread is a missed vsync.
func (t *Tile) Blittable() bool { return t != nil && t.Pixels != nil && t.State != TileFailed }

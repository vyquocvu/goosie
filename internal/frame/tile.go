package frame

import "fmt"

const TileSize int32 = 256

func TileSizeBytes() int64 { return int64(TileSize) * int64(TileSize) * 4 }

type TileCoord struct{ Col, Row int32 }

func CoordFor(p Point, tileSize int32) TileCoord {
	if tileSize <= 0 {
		return TileCoord{}
	}
	return TileCoord{Col: floorDiv(p.X, tileSize), Row: floorDiv(p.Y, tileSize)}
}

func (c TileCoord) Rect(tileSize int32) Rect {
	x0, y0 := c.Col*tileSize, c.Row*tileSize
	return Rect4(x0, y0, x0+tileSize, y0+tileSize)
}

func (c TileCoord) String() string { return fmt.Sprintf("T(%d,%d)", c.Col, c.Row) }

type TileState uint8

const (
	TileEmpty TileState = iota
	TileValid
	TileStale
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

const MaxTileAttempts = 2

type Tile struct {
	Coord    TileCoord
	Bounds   Rect
	Version  uint64
	LastUsed uint64
	Pixels   *Bitmap
	Bytes    int64
	State    TileState
	Attempts int

	prev, next *Tile

	painting *Bitmap
}

func (t *Tile) ValidAt(layerVersion uint64) bool {
	return t != nil && t.State == TileValid && t.Version >= layerVersion && t.Pixels != nil
}

func (t *Tile) Blittable() bool { return t != nil && t.Pixels != nil && t.State != TileFailed }

package frame

import "sync"

type Content interface {
	Version() uint64
	Extent() Rect
}

type LayerID uint32

type Layer struct {
	mu             sync.RWMutex
	ID             LayerID
	ContentVersion uint64
	Bounds         Rect
	Grid           *Grid
	Content        Content
}

func NewLayer(id LayerID, bounds Rect, budget int64, pool *BitmapPool) *Layer {
	bounds = bounds.Canon()
	if bounds.X0 < 0 {
		bounds.X0 = 0
	}
	if bounds.Y0 < 0 {
		bounds.Y0 = 0
	}
	return &Layer{
		ID:     id,
		Bounds: bounds,
		Grid:   NewGrid(bounds, TileSize, budget, pool),
	}
}

func (l *Layer) SetContent(c Content) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Content = c
	if c != nil {
		l.ContentVersion = c.Version()
	}
}

func (l *Layer) ContentSnapshot() Content {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Content
}

func (l *Layer) VersionSnapshot() uint64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ContentVersion
}

func (l *Layer) Bump(v uint64, dirty Rect) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if v <= l.ContentVersion {
		return 0
	}
	l.ContentVersion = v
	return l.Grid.Advance(v, dirty.Canon())
}

func (l *Layer) InvalidateRect(rect Rect) int { return l.Grid.Invalidate(rect) }

func (l *Layer) InvalidateBackground() int { return l.Grid.InvalidateBackground() }

func (l *Layer) BackgroundVersion() uint64 { return l.Grid.BackgroundVersion() }

func (l *Layer) Needs(c TileCoord) bool { return l.Grid.Needs(c, l.VersionSnapshot()) }

func (l *Layer) VisibleCoords(vp Viewport, scratch []TileCoord) []TileCoord {
	return l.Grid.VisibleCoords(vp, scratch)
}

func (l *Layer) Stats() GridStats { return l.Grid.Stats() }

func (l *Layer) BytesInUse() int64 { return l.Grid.Stats().Bytes }

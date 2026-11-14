package frame

type Content interface {
	Version() uint64
	Extent() Rect
}

type LayerID uint32

type Layer struct {
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
	l.Content = c
	if c != nil {
		l.ContentVersion = c.Version()
	}
}

func (l *Layer) Bump(v uint64, dirty Rect) int {
	if v <= l.ContentVersion {
		return 0
	}
	l.ContentVersion = v
	return l.Grid.Advance(v, dirty.Canon())
}

func (l *Layer) InvalidateRect(rect Rect) int { return l.Grid.Invalidate(rect) }

func (l *Layer) Needs(c TileCoord) bool { return l.Grid.Needs(c, l.ContentVersion) }

func (l *Layer) VisibleCoords(vp Viewport, scratch []TileCoord) []TileCoord {
	return l.Grid.VisibleCoords(vp, scratch)
}

func (l *Layer) Stats() GridStats { return l.Grid.Stats() }

func (l *Layer) BytesInUse() int64 { return l.Grid.Stats().Bytes }

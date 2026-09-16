package frame

// Content is the frozen, paintable description a layer points at. It is an
// interface here rather than a concrete display list because frame is the
// dependency leaf and must not import paint: *paint.LayerDL implements these two
// methods, and the rest of the frame path never needs to know what a paint
// command looks like.
//
// Implementations are frozen once attached. That freeze is what lets raster
// workers read a display list with no lock and no copy, which in turn is what
// keeps the UI thread out of rasterization.
type Content interface {
	Version() uint64
	Extent() Rect
}

// LayerID names a compositing layer within a document.
type LayerID uint32

// Layer is one retained raster layer: a content extent, a frozen display list,
// and the tile grid caching its pixels. M1 through M3 have one scrolling layer
// per document plus layers only where stacking context and position: fixed or a
// transform demand one.
type Layer struct {
	ID             LayerID
	ContentVersion uint64
	Bounds         Rect
	Grid           *Grid
	Content        Content
}

// NewLayer returns a layer with its own tile grid over the given extent and byte
// budget. The pool supplies tile buffers and must be sized to the tile size.
func NewLayer(id LayerID, bounds Rect, budget int64, pool *BitmapPool) *Layer {
	return &Layer{
		ID:     id,
		Bounds: bounds.Canon(),
		Grid:   NewGrid(bounds, TileSize, budget, pool),
	}
}

// SetContent attaches a frozen display list without changing the content
// version. Use Bump for anything that altered pixels; this exists for the
// initial attach, where the version is already correct.
func (l *Layer) SetContent(c Content) {
	l.Content = c
	if c != nil {
		l.ContentVersion = c.Version()
	}
}

// Bump advances the content version to v and returns how many tiles went stale.
// Exactly the tiles intersecting dirty do; everything outside it still depicts
// the new version and keeps its pixels, which is why passing the real dirty rect
// rather than the whole document is what keeps an image load from costing a
// full re-raster. An empty dirty rect stales nothing.
func (l *Layer) Bump(v uint64, dirty Rect) int {
	if v <= l.ContentVersion {
		return 0
	}
	l.ContentVersion = v
	return l.Grid.Advance(v, dirty.Canon())
}

// InvalidateRect stales tiles intersecting rect without advancing the version,
// for the case where the content is already current and only cached pixels were
// dropped.
func (l *Layer) InvalidateRect(rect Rect) int { return l.Grid.Invalidate(rect) }

// Needs reports whether a tile must be rasterized for this layer's version.
func (l *Layer) Needs(c TileCoord) bool { return l.Grid.Needs(c, l.ContentVersion) }

// VisibleCoords appends this layer's on-screen coordinates to scratch.
func (l *Layer) VisibleCoords(vp Viewport, scratch []TileCoord) []TileCoord {
	return l.Grid.VisibleCoords(vp, scratch)
}

// Stats returns the grid counters, which the frame gate asserts on.
func (l *Layer) Stats() GridStats { return l.Grid.Stats() }

// BytesInUse returns the tile-buffer bytes currently held by this layer.
func (l *Layer) BytesInUse() int64 { return l.Grid.Stats().Bytes }

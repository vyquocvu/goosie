package frame

import "sync/atomic"

type Grid struct {
	tileSize int32
	extent   Rect

	tiles map[TileCoord]*Tile
	head  *Tile
	tail  *Tile

	free []*Tile

	used   int64
	budget atomic.Int64
	clock  uint64

	backgroundVersion uint64

	pool *BitmapPool

	evictions   int64
	rasterized  int64
	reused      int64
	droppedLate int64
	painting    int64
	counts      [4]int
}

type GridStats struct {
	Tiles         int
	Valid         int
	Stale         int
	Empty         int
	Failed        int
	Bytes         int64
	Budget        int64
	PaintingBytes int64
	Evictions     int64
	Rasterized    int64
	Reused        int64
	DroppedLate   int64
}

func NewGrid(extent Rect, tileSize int32, budget int64, pool *BitmapPool) *Grid {
	if tileSize <= 0 {
		tileSize = TileSize
	}
	one := int64(tileSize) * int64(tileSize) * 4
	if budget < one {
		budget = one
	}
	g := &Grid{
		tileSize: tileSize,
		extent:   extent.Canon(),
		pool:     pool,
	}
	g.budget.Store(budget)
	n := g.recordCap()
	g.tiles = make(map[TileCoord]*Tile, n)
	g.prebuild(n)
	return g
}

func (g *Grid) coordCount() int {
	if g.tileSize <= 0 || g.extent.Empty() {
		return 0
	}
	cols := int((g.extent.W() + g.tileSize - 1) / g.tileSize)
	rows := int((g.extent.H() + g.tileSize - 1) / g.tileSize)
	return cols * rows
}

func (g *Grid) recordCap() int {
	const budgetMultiple = 4
	n := g.coordCount()
	if limit := int(g.budget.Load()/(int64(g.tileSize)*int64(g.tileSize)*4)) * budgetMultiple; n > limit {
		n = limit
	}
	if n < 0 {
		return 0
	}
	return n
}

func (g *Grid) prebuild(n int) {
	if n <= 0 {
		return
	}
	if g.free == nil {
		g.free = make([]*Tile, 0, n)
	}
	for i := 0; i < n; i++ {
		g.free = append(g.free, &Tile{})
	}
}

func (g *Grid) TileSize() int32 { return g.tileSize }

func (g *Grid) Extent() Rect { return g.extent }

func (g *Grid) CoordFor(p Point) TileCoord { return CoordFor(p, g.tileSize) }

func (g *Grid) Inside(c TileCoord) bool { return c.Rect(g.tileSize).Intersects(g.extent) }

func (g *Grid) CoordRange(r Rect) (lo, hi TileCoord, ok bool) {
	if r.Empty() {
		return lo, hi, false
	}
	lo = CoordFor(Point{X: r.X0, Y: r.Y0}, g.tileSize)
	hi = CoordFor(Point{X: r.X1 - 1, Y: r.Y1 - 1}, g.tileSize)
	return lo, hi, true
}

func (g *Grid) Peek(c TileCoord) *Tile { return g.tiles[c] }

func (g *Grid) ensure(c TileCoord) *Tile {
	if t, ok := g.tiles[c]; ok {
		return t
	}
	if !g.Inside(c) {
		return nil
	}
	t := g.newRecord(c)
	g.tiles[c] = t
	g.counts[TileEmpty]++
	return t
}

func (g *Grid) newRecord(c TileCoord) *Tile {
	var t *Tile
	if n := len(g.free); n > 0 {
		t = g.free[n-1]
		g.free = g.free[:n-1]
	} else {
		t = new(Tile)
	}
	t.Coord = c
	t.Bounds = c.Rect(g.tileSize)
	t.State = TileEmpty
	t.BackgroundVersion = 0
	return t
}

func (g *Grid) setState(t *Tile, s TileState) {
	g.counts[t.State]--
	t.State = s
	g.counts[s]++
}

func (g *Grid) unlink(t *Tile) {
	if t.prev != nil {
		t.prev.next = t.next
	} else if g.head == t {
		g.head = t.next
	}
	if t.next != nil {
		t.next.prev = t.prev
	} else if g.tail == t {
		g.tail = t.prev
	}
	t.prev, t.next = nil, nil
}

func (g *Grid) linkFront(t *Tile) {
	if t.prev != nil || t.next != nil || g.head == t {
		g.unlink(t)
	}
	t.prev = nil
	t.next = g.head
	if g.head != nil {
		g.head.prev = t
	}
	g.head = t
	if g.tail == nil {
		g.tail = t
	}
}

func (g *Grid) Touch(c TileCoord) {
	t := g.tiles[c]
	if t == nil || t.Pixels == nil {
		return
	}
	t.LastUsed = g.clock
	g.reused++
	g.linkFront(t)
}

func (g *Grid) VisibleCoords(vp Viewport, scratch []TileCoord) []TileCoord {
	r := vp.Rect().Intersection(g.extent)
	lo, hi, ok := g.CoordRange(r)
	if !ok {
		return scratch
	}
	for row := lo.Row; row <= hi.Row; row++ {
		for col := lo.Col; col <= hi.Col; col++ {
			scratch = append(scratch, TileCoord{Col: col, Row: row})
		}
	}
	return scratch
}

func (g *Grid) PrefetchCoords(vp Viewport, dir Point, rows int, scratch []TileCoord) []TileCoord {
	if rows <= 0 || (dir.X == 0 && dir.Y == 0) {
		return scratch
	}
	r := vp.Rect()
	switch {
	case dir.Y > 0:
		r = Rect4(r.X0, r.Y1, r.X1, r.Y1+int32(rows)*g.tileSize)
	case dir.Y < 0:
		r = Rect4(r.X0, r.Y0-int32(rows)*g.tileSize, r.X1, r.Y0)
	case dir.X > 0:
		r = Rect4(r.X1, r.Y0, r.X1+int32(rows)*g.tileSize, r.Y1)
	default:
		r = Rect4(r.X0-int32(rows)*g.tileSize, r.Y0, r.X0, r.Y1)
	}
	r = r.Intersection(g.extent)
	lo, hi, ok := g.CoordRange(r)
	if !ok {
		return scratch
	}
	for row := lo.Row; row <= hi.Row; row++ {
		for col := lo.Col; col <= hi.Col; col++ {
			scratch = append(scratch, TileCoord{Col: col, Row: row})
		}
	}
	return scratch
}

func (g *Grid) Invalidate(rect Rect) int {
	r := rect.Intersection(g.extent)
	lo, hi, ok := g.CoordRange(r)
	if !ok {
		return 0
	}
	n := 0
	for row := lo.Row; row <= hi.Row; row++ {
		for col := lo.Col; col <= hi.Col; col++ {
			c := TileCoord{Col: col, Row: row}
			t := g.tiles[c]
			if t == nil {
				continue
			}
			if t.State == TileValid || t.State == TileFailed {
				g.setState(t, TileStale)
				t.Attempts = 0
				n++
			}
		}
	}
	return n
}

func (g *Grid) InvalidateBackground() int {
	g.backgroundVersion++
	n := 0
	for _, t := range g.tiles {
		switch {
		case t.State == TileValid || t.State == TileFailed:
			g.setState(t, TileStale)
			t.Attempts = 0
			n++
		case t.painting != nil:
			t.Attempts = 0
		}
	}
	return n
}

func (g *Grid) BackgroundVersion() uint64 { return g.backgroundVersion }

func (g *Grid) Advance(version uint64, dirty Rect) int {
	staled := 0
	for c, t := range g.tiles {
		if t.Version >= version {
			continue
		}
		if c.Rect(g.tileSize).Intersects(dirty) {
			switch {
			case t.Pixels != nil && t.State != TileStale:
				g.setState(t, TileStale)
				t.Attempts = 0
				staled++
			case t.State == TileFailed:
				g.setState(t, TileEmpty)
				t.Attempts = 0
			}
			continue
		}
		t.Version = version
		t.Attempts = 0
		if t.Pixels != nil {
			g.setState(t, TileValid)
		} else if t.State == TileFailed {
			g.setState(t, TileEmpty)
		}
	}
	return staled
}

func (g *Grid) Needs(c TileCoord, layerVersion uint64) bool {
	if !g.Inside(c) {
		return false
	}
	t := g.tiles[c]
	return t == nil || !t.ValidAt(layerVersion) || t.BackgroundVersion != g.backgroundVersion
}

func (g *Grid) Acquire(c TileCoord) (*Bitmap, bool) {
	t := g.ensure(c)
	if t == nil {
		return nil, false
	}
	if t.painting != nil {
		return t.painting, true
	}
	need := int64(g.tileSize) * int64(g.tileSize) * 4
	g.evictFor(need, c)
	t.painting = g.pool.Acquire()
	t.BackgroundVersion = g.backgroundVersion
	g.used += need
	g.painting += need
	if t.State != TileStale {
		g.setState(t, TileStale)
	}
	t.Attempts++
	return t.painting, true
}

func (g *Grid) evictFor(need int64, keep TileCoord) {
	budget := g.budget.Load()
	if g.used+need <= budget {
		return
	}
	for v := g.tail; v != nil; {
		prev := v.prev
		if v.Coord != keep && v.Pixels != nil {
			g.dropPixels(v)
			g.evictions++
		}
		v = prev
		if g.used+need <= budget {
			return
		}
	}
}

func (g *Grid) dropPixels(t *Tile) {
	if t.Pixels == nil {
		return
	}
	pixels := t.Pixels
	g.used -= t.Bytes
	t.Bytes = 0
	t.Pixels = nil
	t.Version = 0
	t.BackgroundVersion = 0
	g.unlink(t)
	g.setState(t, TileEmpty)
	g.pool.Release(pixels)
}

func (g *Grid) MarkValid(c TileCoord, version uint64, pixels *Bitmap) bool {
	t := g.tiles[c]
	if t == nil || pixels == nil || t.painting != pixels || t.BackgroundVersion != g.backgroundVersion {
		g.droppedLate++
		return false
	}
	t.painting = nil
	g.painting -= int64(len(pixels.RGBA))
	if old := t.Pixels; old != nil {
		g.used -= t.Bytes
		t.Pixels = nil
		t.Bytes = 0
		g.pool.Release(old)
	}
	t.Pixels = pixels
	t.Bytes = int64(len(pixels.RGBA))
	t.Version = version
	g.rasterized++
	g.setState(t, TileValid)
	t.Attempts = 0
	t.LastUsed = g.clock
	g.linkFront(t)
	return true
}

func (g *Grid) MarkFailed(c TileCoord) {
	t := g.tiles[c]
	if t == nil {
		return
	}
	if t.Pixels != nil {
		g.dropPixels(t)
	}
	g.setState(t, TileFailed)
}

func (g *Grid) Failed(c TileCoord) bool {
	t := g.tiles[c]
	return t != nil && t.State == TileFailed
}

func (g *Grid) Release(c TileCoord, pixels *Bitmap) bool {
	if pixels == nil {
		return false
	}
	t := g.tiles[c]
	if t != nil && t.painting == pixels {
		t.painting = nil
		need := int64(len(pixels.RGBA))
		g.painting -= need
		g.used -= need
		g.pool.Release(pixels)
		return true
	}
	if t != nil && t.Pixels == pixels {
		return false
	}
	g.pool.Release(pixels)
	return true
}

func (g *Grid) InFlight(c TileCoord) bool {
	t := g.tiles[c]
	return t != nil && t.painting != nil
}

func (g *Grid) InFlightIn(r Rect) int {
	rr := r.Intersection(g.extent)
	lo, hi, ok := g.CoordRange(rr)
	if !ok {
		return 0
	}
	n := 0
	for row := lo.Row; row <= hi.Row; row++ {
		for col := lo.Col; col <= hi.Col; col++ {
			if t := g.tiles[TileCoord{Col: col, Row: row}]; t != nil && t.painting != nil {
				n++
			}
		}
	}
	return n
}

func (g *Grid) Attempts(c TileCoord) int {
	t := g.tiles[c]
	if t == nil {
		return 0
	}
	return t.Attempts
}

func (g *Grid) Pixels(c TileCoord) *Bitmap {
	t := g.tiles[c]
	if t == nil {
		return nil
	}
	return t.Pixels
}

func (g *Grid) Blittable(c TileCoord) bool {
	return g.tiles[c].Blittable()
}

func (g *Grid) Tick() { g.clock++ }

func (g *Grid) Clock() uint64 { return g.clock }

func (g *Grid) SetBudget(budget int64) {
	one := int64(g.tileSize) * int64(g.tileSize) * 4
	if budget < one {
		budget = one
	}
	g.budget.Store(budget)
	for g.used > budget {
		v := g.tail
		if v == nil {
			break
		}
		g.dropPixels(v)
		g.evictions++
	}
}

func (g *Grid) Budget() int64 {
	if g == nil {
		return 0
	}
	return g.budget.Load()
}

func (g *Grid) Stats() GridStats {
	return GridStats{
		Tiles:         len(g.tiles),
		Valid:         g.counts[TileValid],
		Stale:         g.counts[TileStale],
		Empty:         g.counts[TileEmpty],
		Failed:        g.counts[TileFailed],
		Bytes:         g.used,
		Budget:        g.budget.Load(),
		PaintingBytes: g.painting,
		Evictions:     g.evictions,
		Rasterized:    g.rasterized,
		Reused:        g.reused,
		DroppedLate:   g.droppedLate,
	}
}

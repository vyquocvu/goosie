package frame

// Grid owns one layer's tiles, their pixel buffers, and the byte budget they fit
// within. v1 kept the LRU order in a separate TileCache from the tile map, which
// meant two structures had to agree about which tiles held memory. Here there is
// one owner, so the validity rule and the budget cannot drift apart.
//
// Tiles metadata lives in the map for every coordinate ever made visible; only
// pixel buffers are budgeted and evicted. That is a deliberate trade: a 64-byte
// map entry per coordinate is bounded by the document area divided by tile area,
// while the 256 KB buffers it points at are what actually need a ceiling.
type Grid struct {
	tileSize int32
	extent   Rect

	tiles map[TileCoord]*Tile
	head  *Tile // most recently used tile holding pixels
	tail  *Tile

	used   int64
	budget int64
	clock  uint64

	pool *BitmapPool

	evictions   int64
	rasterized  int64
	reused      int64
	droppedLate int64
	painting    int64
	counts      [4]int
}

// GridStats is a snapshot for reports and for the gate tests, which assert on
// counters rather than timings so CI stays deterministic.
type GridStats struct {
	Tiles  int
	Valid  int
	Stale  int
	Empty  int
	Failed int
	Bytes  int64
	Budget int64
	// PaintingBytes is the tile buffers currently on order at a worker. They count
	// toward Bytes and therefore against the budget, because the memory is really
	// held; a caller checking that tile memory plateaus has to expect a tile to
	// cost twice while it is being redrawn.
	PaintingBytes int64
	Evictions     int64
	Rasterized    int64
	Reused        int64
	DroppedLate   int64
}

// NewGrid returns a grid covering extent, budgeted at budget bytes and drawing
// buffers from pool. A budget below one tile is raised to one tile: a grid that
// cannot hold a single tile could never present anything, so failing early is
// more honest than evicting the tile currently being rasterized.
func NewGrid(extent Rect, tileSize int32, budget int64, pool *BitmapPool) *Grid {
	if tileSize <= 0 {
		tileSize = TileSize
	}
	one := int64(tileSize) * int64(tileSize) * 4
	if budget < one {
		budget = one
	}
	return &Grid{
		tileSize: tileSize,
		extent:   extent.Canon(),
		tiles:    make(map[TileCoord]*Tile),
		budget:   budget,
		pool:     pool,
	}
}

// TileSize returns the grid's tile edge length.
func (g *Grid) TileSize() int32 { return g.tileSize }

// Extent returns the layer bounds the grid was built for.
func (g *Grid) Extent() Rect { return g.extent }

// CoordFor maps a content-space point to its tile.
func (g *Grid) CoordFor(p Point) TileCoord { return CoordFor(p, g.tileSize) }

// Inside reports whether a coordinate can hold content at all.
func (g *Grid) Inside(c TileCoord) bool { return c.Rect(g.tileSize).Intersects(g.extent) }

// CoordRange returns the inclusive tile-coordinate span covering r.
func (g *Grid) CoordRange(r Rect) (lo, hi TileCoord, ok bool) {
	if r.Empty() {
		return lo, hi, false
	}
	lo = CoordFor(Point{X: r.X0, Y: r.Y0}, g.tileSize)
	hi = CoordFor(Point{X: r.X1 - 1, Y: r.Y1 - 1}, g.tileSize)
	return lo, hi, true
}

// Peek returns an existing tile, or nil.
func (g *Grid) Peek(c TileCoord) *Tile { return g.tiles[c] }

func (g *Grid) ensure(c TileCoord) *Tile {
	if t, ok := g.tiles[c]; ok {
		return t
	}
	if !g.Inside(c) {
		return nil
	}
	t := &Tile{Coord: c, Bounds: c.Rect(g.tileSize), State: TileEmpty}
	g.tiles[c] = t
	g.counts[TileEmpty]++
	return t
}

func (g *Grid) setState(t *Tile, s TileState) {
	g.counts[t.State]--
	t.State = s
	g.counts[s]++
}

// unlink removes a tile from the LRU order without dropping its pixels.
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

// Touch records that a tile contributed to the current frame, making it the
// least likely eviction victim. It is called for every visible tile every frame,
// so it is a pointer swap and nothing else.
func (g *Grid) Touch(c TileCoord) {
	t := g.tiles[c]
	if t == nil || t.Pixels == nil {
		return
	}
	t.LastUsed = g.clock
	g.reused++
	g.linkFront(t)
}

// VisibleCoords appends the coordinates covering the viewport to scratch,
// clamped to the layer extent, and returns the grown slice. Passing a retained
// scratch buffer is the caller's way of keeping frame bookkeeping allocation
// free, which is why this takes one instead of returning a fresh slice.
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

// PrefetchCoords appends the ring of coordinates ahead of a scroll direction,
// excluding anything already in the viewport. Rastering this band one screen
// early is what protects p99 rather than the mean: the tile the user is about to
// reveal is already warm.
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

// Invalidate marks every tile intersecting rect as needing re-rasterization and
// returns how many were marked. Pixels are kept: a stale tile still paints,
// which is the mechanism behind "present now, fix next vsync".
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
				// Never rasterized; it will be built current when first needed.
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

// Advance moves the whole grid to a new content version and returns how many
// tiles went stale. Only tiles intersecting dirty do: the content everywhere
// else is unchanged by definition, so those tiles are promoted in place and keep
// their pixels.
//
// This is what makes a dirty rect mean anything. Staling every tile on a version
// bump would be simpler and wrong: an image loading in the footer would cost a
// full-page re-raster, which is exactly the v1 behaviour this design removes. The
// scan is over the coordinates ever made visible, bounded by document area over
// tile area, and runs on content change rather than per frame.
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
				// The other branch of this loop promotes a failed tile out of Failed, and
				// the case that actually changed under this version deserves the same
				// treatment: a failed tile holds no pixels, so the retry-count cap is
				// guarding a raster of content that no longer exists. Leaving it Failed
				// would keep a blank rectangle on screen for the rest of the document's
				// life, which is a permanent artefact from one transient failure.
				g.setState(t, TileEmpty)
				t.Attempts = 0
			}
			continue
		}
		t.Version = version
		t.Attempts = 0 // a new version is a fair retry for a tile that failed
		if t.Pixels != nil {
			g.setState(t, TileValid)
		} else if t.State == TileFailed {
			g.setState(t, TileEmpty)
		}
	}
	return staled
}

// Needs reports whether a tile must be rasterized for the given layer version.
// This is the validity rule in the design document, as one expression.
func (g *Grid) Needs(c TileCoord, layerVersion uint64) bool {
	if !g.Inside(c) {
		return false
	}
	t := g.tiles[c]
	return t == nil || !t.ValidAt(layerVersion)
}

// Acquire reserves a buffer for a tile's next raster, evicting least-recently-used
// pixels as the budget requires, and returns it ready to be painted. The second
// result is false when the coordinate lies outside the layer.
//
// The buffer returned is never the tile's current pixels. That separation is the
// point: a stale tile keeps being composited while its replacement is drawn, and
// the UI thread and the worker never touch the same bytes. Calling Acquire twice
// for the same tile returns the same buffer, so a duplicate submission paints
// once rather than handing two workers one target.
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
	g.used += need
	g.painting += need
	if t.State != TileStale {
		g.setState(t, TileStale)
	}
	t.Attempts++
	return t.painting, true
}

// evictFor frees buffers until need more bytes fit, never evicting the tile
// being prepared. Because the budget is guaranteed to hold at least one tile and
// every other holder is a valid victim, the loop always terminates.
func (g *Grid) evictFor(need int64, keep TileCoord) {
	if g.used+need <= g.budget {
		return
	}
	for v := g.tail; v != nil; {
		prev := v.prev
		if v.Coord != keep && v.Pixels != nil {
			g.dropPixels(v)
			g.evictions++
		}
		v = prev
		if g.used+need <= g.budget {
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
	g.unlink(t)
	g.setState(t, TileEmpty)
	// A tile's own painting buffer is untouched here: it belongs to a worker until
	// that worker reports, and it is still counted against the budget.
	//
	// It is not cleared either. The pool documents an acquired buffer's contents as
	// undefined and every rasterizer clears before drawing, so zeroing on the way
	// out is a tile-sized memset on the thread that also has to present.
	g.pool.Release(pixels)
}

// MarkValid installs a finished raster and retires the pixels it replaced. It
// returns false when the result is stale news - the tile vanished, or the buffer
// was never this grid's - and the caller must then hand the buffer back.
//
// Only the buffer this grid handed out by Acquire is accepted, and that is what
// makes the swap safe: Pixels changes wholesale at one instant on one thread, so a
// compositor sees either the old rendering or the new one and never a mixture of
// the two.
func (g *Grid) MarkValid(c TileCoord, version uint64, pixels *Bitmap) bool {
	t := g.tiles[c]
	if t == nil || pixels == nil || t.painting != pixels {
		g.droppedLate++
		return false
	}
	t.painting = nil
	g.painting -= int64(len(pixels.RGBA))
	if old := t.Pixels; old != nil {
		// The superseded rendering goes straight back to the pool: no worker owns it,
		// and this is the UI thread, so nothing is reading it right now.
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

// MarkFailed records that a tile exhausted MaxTileAttempts; it keeps no pixels and
// renders as the layer background. A buffer still on order for the tile is left
// with its worker, which reports it back through Release.
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

// Failed reports whether a tile has been given up on.
func (g *Grid) Failed(c TileCoord) bool {
	t := g.tiles[c]
	return t != nil && t.State == TileFailed
}

// Release gives back an Acquired buffer that is not going to be reported through
// MarkValid, the usual case being a submission the queue refused. It is the only
// safe way for a raster caller to hand memory back, because the grid is the one
// that knows whether the buffer is a tile's painting target, a tile's presentable
// pixels, or something that came from elsewhere.
//
// It reports whether the buffer went back to the pool.
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
		// Still this tile's content, which the compositor is entitled to keep
		// blitting. The tile owns it and will release it when it is evicted.
		return false
	}
	g.pool.Release(pixels)
	return true
}

// InFlight reports whether a tile has a buffer on order at a worker. A caller that
// queued such a tile again would pay for a second raster of pixels it already has
// arriving, and under budget pressure the duplicate would evict the original.
func (g *Grid) InFlight(c TileCoord) bool {
	t := g.tiles[c]
	return t != nil && t.painting != nil
}

// Attempts returns how many rasterizations have been tried for a tile.
func (g *Grid) Attempts(c TileCoord) int {
	t := g.tiles[c]
	if t == nil {
		return 0
	}
	return t.Attempts
}

// Pixels returns a tile's buffer, valid or stale, or nil.
func (g *Grid) Pixels(c TileCoord) *Bitmap {
	t := g.tiles[c]
	if t == nil {
		return nil
	}
	return t.Pixels
}

// Tick advances the frame clock used for recency.
func (g *Grid) Tick() { g.clock++ }

// Clock returns the current frame clock.
func (g *Grid) Clock() uint64 { return g.clock }

// SetBudget changes the byte ceiling, evicting immediately if the new value is
// lower. The gate's memory plateau assertion depends on this being enforced
// eagerly rather than opportunistically.
func (g *Grid) SetBudget(budget int64) {
	one := int64(g.tileSize) * int64(g.tileSize) * 4
	if budget < one {
		budget = one
	}
	g.budget = budget
	for g.used > g.budget {
		v := g.tail
		if v == nil {
			break
		}
		g.dropPixels(v)
		g.evictions++
	}
}

// Stats returns a snapshot of grid state and counters.
func (g *Grid) Stats() GridStats {
	return GridStats{
		Tiles:         len(g.tiles),
		Valid:         g.counts[TileValid],
		Stale:         g.counts[TileStale],
		Empty:         g.counts[TileEmpty],
		Failed:        g.counts[TileFailed],
		Bytes:         g.used,
		Budget:        g.budget,
		PaintingBytes: g.painting,
		Evictions:     g.evictions,
		Rasterized:    g.rasterized,
		Reused:        g.reused,
		DroppedLate:   g.droppedLate,
	}
}

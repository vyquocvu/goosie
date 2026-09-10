package raster

import (
	"container/list"
	"sync"

	"github.com/vyquocvu/goosie/v2/internal/frame"
)

// glyphKey names one cached rendering.
//
// Size is a device-pixel size, so a glyph drawn for a 1x plan and the same glyph
// drawn for a 2x plan at the same device size are one entry, not two. That is
// the reuse the frame gate counts on: changing device ratio must re-render the
// layout, not the font.
type glyphKey struct {
	r    rune
	size int32
}

type glyphVal struct {
	k glyphKey
	g Glyph
}

// AtlasStats counts what the cache did, for the frame report and the tests.
type AtlasStats struct {
	Entries   int
	Bytes     int64
	Budget    int64
	Evictions int64
	Hits      int64
	Misses    int64
}

// GlyphAtlas is a byte-budgeted LRU of rendered glyphs shared by every worker.
//
// Two properties make it worth having separately from Fonts. It is the layer
// that can drop entries when a page uses forty sizes, because Fonts' per-face
// memo has no budget and would grow without bound; and it is safe for concurrent
// read on the hit path, which is the only path a warm scroll takes. A miss
// reaches into Fonts, which serializes face access, so a burst of first-pass
// rasters is bounded by one mutex rather than by a per-worker copy of a 900 KB
// font.
type GlyphAtlas struct {
	mu     sync.Mutex
	fonts  *Fonts
	budget int64
	used   int64

	order *list.List // front = most recently used
	index map[glyphKey]*list.Element
	stats AtlasStats
}

// NewGlyphAtlas returns an atlas holding at most budget bytes of glyph masks.
// A zero or negative budget disables caching rather than failing: every lookup
// goes to Fonts, which still memoizes per face, so correctness does not depend on
// the budget.
func NewGlyphAtlas(budget int64, f *Fonts) *GlyphAtlas {
	return &GlyphAtlas{
		fonts:  f,
		budget: budget,
		order:  list.New(),
		index:  map[glyphKey]*list.Element{},
		stats:  AtlasStats{Budget: budget},
	}
}

// Get returns the rendering of r at a device-pixel size, rasterizing it on a
// miss. The zero Glyph means the font has no such glyph.
func (a *GlyphAtlas) Get(r rune, size int32) Glyph {
	k := glyphKey{r: r, size: size}
	a.mu.Lock()
	defer a.mu.Unlock()
	if el, ok := a.index[k]; ok {
		a.order.MoveToFront(el)
		a.stats.Hits++
		return el.Value.(*glyphVal).g
	}
	a.stats.Misses++
	g := a.fonts.Glyph(size, r)
	if !g.Ok || g.Mask == nil {
		// Uncached: a miss with nothing to store. Repeated lookups for the same
		// absent glyph stay cheap because Fonts memoizes its own answer.
		return g
	}
	if a.budget <= 0 {
		return g
	}
	a.insert(k, g)
	return g
}

// GlyphBound returns the box r occupies relative to the pen, without exposing
// the mask. Tile selection and text bounds use it; a false result means the
// glyph is unknown or draws nothing.
func (a *GlyphAtlas) GlyphBound(r rune, size int32) (frame.Rect, bool) {
	g := a.Get(r, size)
	if !g.Ok || g.Mask == nil {
		return frame.Rect{}, false
	}
	return g.Bounds, true
}

func (a *GlyphAtlas) insert(k glyphKey, g Glyph) {
	val := &glyphVal{k: k, g: g}
	el := a.order.PushFront(val)
	a.index[k] = el
	a.used += g.Bytes()
	// Evicting the oldest entries can free less than one glyph if the new
	// insertion alone exceeds the budget; the loop still terminates because the
	// list is finite and the newest entry is never dropped here.
	for a.used > a.budget {
		back := a.order.Back()
		if back == nil || back == el {
			break
		}
		v := back.Value.(*glyphVal)
		a.used -= v.g.Bytes()
		delete(a.index, v.k)
		a.order.Remove(back)
		a.stats.Evictions++
	}
	a.refresh()
}

// Reset drops every cached glyph, used when a document is torn down and the
// masks would otherwise keep the atlas full for the next page.
func (a *GlyphAtlas) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.order.Init()
	a.index = map[glyphKey]*list.Element{}
	a.used = 0
	a.stats.Entries = 0
	a.stats.Bytes = 0
}

// Stats returns a snapshot of the cache counters.
func (a *GlyphAtlas) Stats() AtlasStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refresh()
	return a.stats
}

func (a *GlyphAtlas) refresh() {
	a.stats.Entries = len(a.index)
	a.stats.Bytes = a.used
}

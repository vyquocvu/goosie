package raster

import (
	"container/list"
	"sync"

	"github.com/vyquocvu/goosie/internal/frame"
)

type glyphKey struct {
	r    rune
	size int32
	slot frame.FontSlot
}

type glyphVal struct {
	k glyphKey
	g Glyph
}

type AtlasStats struct {
	Entries   int
	Bytes     int64
	Budget    int64
	Evictions int64
	Hits      int64
	Misses    int64
}

type GlyphAtlas struct {
	mu     sync.Mutex
	fonts  *Fonts
	budget int64
	used   int64

	order *list.List
	index map[glyphKey]*list.Element
	stats AtlasStats
}

func NewGlyphAtlas(budget int64, f *Fonts) *GlyphAtlas {
	return &GlyphAtlas{
		fonts:  f,
		budget: budget,
		order:  list.New(),
		index:  map[glyphKey]*list.Element{},
		stats:  AtlasStats{Budget: budget},
	}
}

func (a *GlyphAtlas) Get(r rune, size int32, slot frame.FontSlot) Glyph {
	k := glyphKey{r: r, size: size, slot: slot}
	a.mu.Lock()
	if el, ok := a.index[k]; ok {
		a.order.MoveToFront(el)
		a.stats.Hits++
		g := el.Value.(*glyphVal).g
		a.mu.Unlock()
		return g
	}
	a.mu.Unlock()
	g := a.fonts.Glyph(size, r, slot)
	a.mu.Lock()
	defer a.mu.Unlock()
	if el, ok := a.index[k]; ok {
		a.order.MoveToFront(el)
		a.stats.Hits++
		return el.Value.(*glyphVal).g
	}
	a.stats.Misses++
	if !g.Ok || g.Mask == nil {
		return g
	}
	if a.budget <= 0 {
		return g
	}
	a.insert(k, g)
	return g
}

func (a *GlyphAtlas) GlyphBound(r rune, size int32, slot frame.FontSlot) (frame.Rect, bool) {
	g := a.Get(r, size, slot)
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

func (a *GlyphAtlas) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.order.Init()
	a.index = map[glyphKey]*list.Element{}
	a.used = 0
	a.stats.Entries = 0
	a.stats.Bytes = 0
}

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

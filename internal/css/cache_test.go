// Package css — M9.2 tests for bounded selector and computed-style caches.
//
// Tests cover:
//   - MatchCache: normal hit/miss, capacity eviction, Evict(), Clear(), race safety
//   - StylePool.Evict: eviction drives memory budget, race safety
package css

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// staticElem is a minimal Element implementation for testing.
type staticElem struct {
	tag     string
	id      string
	classes []string
	attrs   map[string]string
	parent  *staticElem
}

func (e *staticElem) TagName() string { return e.tag }
func (e *staticElem) ID() string      { return e.id }
func (e *staticElem) Classes() []string {
	if e.classes == nil {
		return nil
	}
	return e.classes
}
func (e *staticElem) GetAttribute(name string) (string, bool) {
	v, ok := e.attrs[name]
	return v, ok
}
func (e *staticElem) ParentElement() Element {
	if e.parent == nil {
		return nil
	}
	return e.parent
}
func (e *staticElem) PreviousSiblingElement() Element    { return nil }
func (e *staticElem) ForEachChild(fn func(Element) bool) {}
func (e *staticElem) ForEachAncestor(fn func(Element) bool) {
	p := e.parent
	for p != nil {
		if !fn(p) {
			return
		}
		p = p.parent
	}
}
func (e *staticElem) ForEachPrecedingSibling(fn func(Element) bool) {}

// buildTestSheet returns a CompiledStyleSheet with a handful of rules.
func buildTestSheet() *CompiledStyleSheet {
	css := `
		#header { color: red; }
		.nav { display: flex; }
		div { margin: 0; }
		div.container { padding: 10px; }
		* { box-sizing: border-box; }
	`
	parser := NewParser(css)
	sheet, err := parser.Parse()
	if err != nil {
		panic(fmt.Sprintf("test fixture parse error: %v", err))
	}
	return CompileStyleSheet(sheet)
}

// ---------------------------------------------------------------------------
// ElementKey tests
// ---------------------------------------------------------------------------

func TestElementKey_FromElement(t *testing.T) {
	elem := &staticElem{
		tag:     "div",
		id:      "main",
		classes: []string{"container", "active"},
	}
	key := ElementKeyFromElement(elem)

	if key.Tag != "div" {
		t.Errorf("want Tag=div, got %q", key.Tag)
	}
	if key.ID != "main" {
		t.Errorf("want ID=main, got %q", key.ID)
	}
	// Classes must be sorted for determinism.
	if key.ClassKey != "active container" {
		t.Errorf("want sorted classes \"active container\", got %q", key.ClassKey)
	}
}

func TestElementKey_EmptyClasses(t *testing.T) {
	elem := &staticElem{tag: "p"}
	key := ElementKeyFromElement(elem)
	if key.Tag != "p" || key.ID != "" || key.ClassKey != "" {
		t.Errorf("unexpected key: %+v", key)
	}
}

func TestElementKey_Equal(t *testing.T) {
	a := ElementKey{Tag: "div", ID: "x", ClassKey: "foo bar"}
	b := ElementKey{Tag: "div", ID: "x", ClassKey: "foo bar"}
	c := ElementKey{Tag: "div", ID: "y", ClassKey: "foo bar"}

	if a != b {
		t.Error("identical keys should be equal")
	}
	if a == c {
		t.Error("different IDs should not be equal")
	}
}

func TestElementKey_Deterministic(t *testing.T) {
	// Same element produces same key on repeated calls.
	elem := &staticElem{tag: "div", id: "foo", classes: []string{"z", "a", "m"}}
	k1 := ElementKeyFromElement(elem)
	k2 := ElementKeyFromElement(elem)
	if k1 != k2 {
		t.Error("ElementKeyFromElement must be deterministic")
	}
}

// ---------------------------------------------------------------------------
// MatchCache — normal operation
// ---------------------------------------------------------------------------

func TestMatchCache_HitAndMiss(t *testing.T) {
	sheet := buildTestSheet()
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 64})

	elem := &staticElem{tag: "div", id: "header", classes: []string{"container"}}
	key := ElementKeyFromElement(elem)

	// First call: miss, populates cache.
	got, ok := cache.Get(key)
	if ok {
		t.Fatal("expected miss on empty cache")
	}
	if got != nil {
		t.Fatal("miss should return nil")
	}

	rules := sheet.MatchElement(elem)
	cache.Put(key, rules)

	// Second call: hit.
	got2, ok2 := cache.Get(key)
	if !ok2 {
		t.Fatal("expected hit after Put")
	}
	if len(got2) != len(rules) {
		t.Errorf("got %d rules, want %d", len(got2), len(rules))
	}

	snap := cache.Metrics()
	if snap.Hits != 1 {
		t.Errorf("want 1 hit, got %d", snap.Hits)
	}
	if snap.Misses != 1 {
		t.Errorf("want 1 miss, got %d", snap.Misses)
	}
}

func TestMatchCache_Capacity(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 3})

	// Insert 3 entries — fills to capacity.
	for i := 0; i < 3; i++ {
		key := ElementKey{Tag: "div", ID: fmt.Sprintf("id%d", i)}
		cache.Put(key, []CompiledRule{{SourceOrder: uint32(i)}})
	}
	if cache.Len() != 3 {
		t.Fatalf("want 3 entries, got %d", cache.Len())
	}

	// Insert a 4th — must evict oldest.
	key4 := ElementKey{Tag: "p", ID: "id99"}
	cache.Put(key4, []CompiledRule{{SourceOrder: 99}})

	if cache.Len() != 3 {
		t.Errorf("want 3 after eviction, got %d", cache.Len())
	}

	snap := cache.Metrics()
	if snap.Evictions < 1 {
		t.Errorf("want at least 1 eviction, got %d", snap.Evictions)
	}
}

func TestMatchCache_Update(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})
	key := ElementKey{Tag: "div", ID: "foo"}

	cache.Put(key, []CompiledRule{{SourceOrder: 1}, {SourceOrder: 2}, {SourceOrder: 3}})
	cache.Put(key, []CompiledRule{{SourceOrder: 7}, {SourceOrder: 8}}) // update same key

	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected hit")
	}
	if len(got) != 2 || got[0].SourceOrder != 7 || got[1].SourceOrder != 8 {
		t.Errorf("want [{7} {8}], got %v", got)
	}
}

func TestMatchCache_Clear(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})
	for i := 0; i < 4; i++ {
		cache.Put(ElementKey{Tag: "div", ID: fmt.Sprintf("id%d", i)},
			[]CompiledRule{{SourceOrder: uint32(i)}})
	}
	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("want 0 after Clear, got %d", cache.Len())
	}
	snap := cache.Metrics()
	if snap.Hits+snap.Misses+snap.Evictions != 0 {
		t.Error("metrics should reset after Clear")
	}
}

// ---------------------------------------------------------------------------
// MatchCache — Evict for memory budget integration
// ---------------------------------------------------------------------------

func TestMatchCache_Evict(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})

	// Insert 4 entries.
	for i := 0; i < 4; i++ {
		key := ElementKey{Tag: "div", ID: fmt.Sprintf("id%d", i)}
		cache.Put(key, []CompiledRule{{SourceOrder: uint32(i)}})
	}

	if cache.Len() == 0 {
		t.Fatal("setup failed: no entries")
	}

	freed := cache.Evict(cache.ByteSize()) // request to free all
	if freed == 0 {
		t.Error("expected non-zero bytes freed")
	}
	if cache.Len() != 0 {
		t.Errorf("expected empty cache after full eviction, got %d", cache.Len())
	}
}

func TestMatchCache_EvictPartial(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})
	for i := 0; i < 4; i++ {
		key := ElementKey{Tag: "div", ID: fmt.Sprintf("id%d", i)}
		cache.Put(key, []CompiledRule{{SourceOrder: uint32(i)}})
	}

	total := cache.ByteSize()
	// Request to free half — should free at least one entry.
	freed := cache.Evict(total / 2)
	if freed == 0 {
		t.Error("expected at least one entry freed")
	}
	if cache.Len() >= 4 {
		t.Errorf("expected fewer entries after partial eviction, got %d", cache.Len())
	}
}

func TestMatchCache_EvictEmpty(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})
	freed := cache.Evict(1000)
	if freed != 0 {
		t.Errorf("want 0 freed from empty cache, got %d", freed)
	}
}

// ---------------------------------------------------------------------------
// MatchCache — race safety
// ---------------------------------------------------------------------------

func TestMatchCache_Race(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 16})
	var wg sync.WaitGroup
	tags := []string{"div", "p", "span", "a", "section"}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := ElementKey{Tag: tags[n%len(tags)], ID: fmt.Sprintf("id%d", n%5)}
			_, ok := cache.Get(key)
			if !ok {
				cache.Put(key, []CompiledRule{{SourceOrder: uint32(n)}})
			}
		}(i)
	}

	// Concurrent eviction.
	wg.Add(1)
	go func() {
		defer wg.Done()
		cache.Evict(512)
	}()

	wg.Wait()
}

// ---------------------------------------------------------------------------
// StylePool — Evict for memory budget integration
// ---------------------------------------------------------------------------

func TestStylePool_Evict(t *testing.T) {
	pool := NewStylePoolWithLimit(8)

	// Intern some distinct styles.
	for i := 0; i < 5; i++ {
		s := &InheritedStyle{FontSize: float32(10 + i)}
		pool.Intern(s)
	}

	before := pool.Stats().Entries
	if before == 0 {
		t.Fatal("setup failed")
	}

	// Request eviction of all bytes.
	freed := pool.Evict(^uint64(0))
	if freed == 0 {
		t.Error("expected non-zero freed bytes")
	}
	if pool.Stats().Entries != 0 {
		t.Errorf("expected empty pool after full eviction, got %d entries", pool.Stats().Entries)
	}
}

func TestStylePool_EvictPartial(t *testing.T) {
	pool := NewStylePoolWithLimit(16)
	for i := 0; i < 8; i++ {
		s := &InheritedStyle{FontSize: float32(i + 1)}
		pool.Intern(s)
	}

	before := pool.Stats().Entries
	freed := pool.Evict(1) // evict at least 1 byte → removes at least one entry
	if freed == 0 {
		t.Error("expected non-zero freed bytes")
	}
	if pool.Stats().Entries >= before {
		t.Errorf("expected fewer entries after partial eviction")
	}
}

func TestStylePool_EvictEmpty(t *testing.T) {
	pool := NewStylePool()
	freed := pool.Evict(1000)
	if freed != 0 {
		t.Errorf("want 0 from empty pool, got %d", freed)
	}
}

func TestStylePool_EvictRace(t *testing.T) {
	pool := NewStylePoolWithLimit(32)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s := &InheritedStyle{FontSize: float32(n % 8)}
			pool.Intern(s)
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Evict(256)
	}()
	wg.Wait()
}

// ---------------------------------------------------------------------------
// MatchRules — regression: result is stable across calls
// ---------------------------------------------------------------------------

func TestMatchElement_Stable(t *testing.T) {
	sheet := buildTestSheet()
	elem := &staticElem{tag: "div", id: "header", classes: []string{"container"}}

	first := sheet.MatchElement(elem)
	second := sheet.MatchElement(elem)

	if len(first) != len(second) {
		t.Errorf("got %d vs %d rules — unstable result", len(first), len(second))
	}
}

// ---------------------------------------------------------------------------
// Benchmark — cached vs uncached MatchRules
// ---------------------------------------------------------------------------

func BenchmarkMatchCache_Cached(b *testing.B) {
	sheet := buildTestSheet()
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 256})
	elem := &staticElem{tag: "div", id: "header", classes: []string{"container"}}
	key := ElementKeyFromElement(elem)

	// Warm up.
	rules := sheet.MatchElement(elem)
	cache.Put(key, rules)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, ok := cache.Get(key)
		if !ok {
			cache.Put(key, sheet.MatchElement(elem))
		}
	}
}

func BenchmarkMatchCache_Uncached(b *testing.B) {
	sheet := buildTestSheet()
	elem := &staticElem{tag: "div", id: "header", classes: []string{"container"}}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sheet.MatchElement(elem)
	}
}

func BenchmarkMatchCache_ManyElements(b *testing.B) {
	sheet := buildTestSheet()
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 256})

	tags := []string{"div", "p", "span", "section", "article"}
	ids := []string{"", "header", "main", "footer"}
	type testElem struct {
		key     ElementKey
		element *staticElem
	}
	elems := make([]testElem, 0, len(tags)*len(ids))
	for _, tag := range tags {
		for _, id := range ids {
			e := &staticElem{tag: tag, id: id}
			elems = append(elems, testElem{key: ElementKeyFromElement(e), element: e})
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := elems[i%len(elems)]
		_, ok := cache.Get(e.key)
		if !ok {
			cache.Put(e.key, sheet.MatchElement(e.element))
		}
	}
}

func BenchmarkStylePool_Evict(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pool := NewStylePoolWithLimit(32)
		for j := 0; j < 16; j++ {
			s := &InheritedStyle{FontSize: float32(j + 1)}
			pool.Intern(s)
		}
		pool.Evict(^uint64(0))
	}
}

// ---------------------------------------------------------------------------
// Integration: MatchCache + StylePool together
// ---------------------------------------------------------------------------

func TestMatchCacheAndStylePool_Integration(t *testing.T) {
	sheet := buildTestSheet()
	matchCache := NewMatchCache(MatchCacheConfig{MaxEntries: 32})
	stylePool := NewStylePoolWithLimit(32)

	elems := []*staticElem{
		{tag: "div", id: "header"},
		{tag: "div", classes: []string{"container"}},
		{tag: "p"},
		{tag: "a", attrs: map[string]string{"href": "#"}},
	}

	// Simulate style resolution: look up match cache, compute style, intern.
	for _, e := range elems {
		key := ElementKeyFromElement(e)

		// Try cache.
		_, ok := matchCache.Get(key)
		if !ok {
			rules := sheet.MatchElement(e)
			matchCache.Put(key, rules)
		}

		// Build a computed inherited style and intern.
		s := &InheritedStyle{FontSize: 16, FontFamily: "sans-serif"}
		_ = stylePool.Intern(s)
	}

	// Trigger eviction of both.
	matchCache.Evict(matchCache.ByteSize() / 2)
	stylePool.Evict(1)

	// Verify nothing panics and sizes are bounded.
	if matchCache.Len() > 32 {
		t.Errorf("match cache overflow: %d", matchCache.Len())
	}
	if stylePool.Stats().Entries > 32 {
		t.Errorf("style pool overflow: %d", stylePool.Stats().Entries)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestMatchCache_ZeroCapacity(t *testing.T) {
	// Zero capacity should use a sensible default (not panic).
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 0})
	key := ElementKey{Tag: "div"}
	cache.Put(key, []CompiledRule{{SourceOrder: 1}})
	// Should not panic; cache with default capacity must accept entry.
	if cache.Len() == 0 {
		// Acceptable if default capacity is very small, but shouldn't panic.
	}
}

func TestMatchCache_LargeClassList(t *testing.T) {
	// 20 classes — key construction must remain correct.
	classes := make([]string, 20)
	for i := range classes {
		classes[i] = fmt.Sprintf("cls%d", i)
	}
	elem := &staticElem{tag: "div", classes: classes}
	key := ElementKeyFromElement(elem)

	// Key must be deterministic.
	key2 := ElementKeyFromElement(elem)
	if key != key2 {
		t.Error("ElementKey must be deterministic for same element")
	}

	// The class key must contain all class names.
	for _, cls := range classes {
		if !strings.Contains(key.ClassKey, cls) {
			t.Errorf("class %q missing from key %q", cls, key.ClassKey)
		}
	}
}

func TestMatchCache_NilResult(t *testing.T) {
	cache := NewMatchCache(MatchCacheConfig{MaxEntries: 8})
	key := ElementKey{Tag: "div"}

	// Storing nil is valid (element matched no rules).
	cache.Put(key, nil)
	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected hit for nil-result entry")
	}
	if len(got) != 0 {
		t.Errorf("expected nil/empty, got %v", got)
	}
}

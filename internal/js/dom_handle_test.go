package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// helper: create a store with a few nodes.
func newTestStore(t *testing.T) *dom.Store {
	t.Helper()
	s := dom.NewStore(64)
	_, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// helper: allocate a node and set its kind so IsValid returns true.
func allocNode(t testing.TB, s *dom.Store) dom.NodeID {
	t.Helper()
	id, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(id, dom.NodeKindElement); err != nil {
		t.Fatal(err)
	}
	return id
}

// ---------------------------------------------------------------------------
// NodeHandle — basic operations
// ---------------------------------------------------------------------------

func TestNodeHandle_Invalid(t *testing.T) {
	h := NodeHandle{}
	if h.IsValid() {
		t.Error("zero handle should not be valid")
	}
	_, err := h.Kind()
	if err != ErrInvalidHandle {
		t.Errorf("Kind() error = %v, want ErrInvalidHandle", err)
	}
}

func TestNodeHandle_ValidAfterAllocate(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)

	h := NewNodeHandle(id, s)
	if !h.IsValid() {
		t.Error("handle to allocated node should be valid")
	}
	if h.NodeID() != id {
		t.Errorf("NodeID = %d, want %d", h.NodeID(), id)
	}
}

func TestNodeHandle_NodeNone(t *testing.T) {
	s := dom.NewStore(64)
	h := NewNodeHandle(dom.NodeNone, s)
	if h.IsValid() {
		t.Error("NodeNone handle should not be valid")
	}
}

func TestNodeHandle_Kind(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	h := NewNodeHandle(id, s)

	kind, err := h.Kind()
	if err != nil {
		t.Fatal(err)
	}
	// Kind should be accessible (exact value depends on store init).
	_ = kind
}

func TestNodeHandle_Parent(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	h := NewNodeHandle(id, s)

	parent, err := h.Parent()
	if err != nil {
		t.Fatal(err)
	}
	// Root node has no parent — should return NodeNone.
	if parent.NodeID() != dom.NodeNone {
		t.Errorf("root parent = %d, want NodeNone", parent.NodeID())
	}
}

func TestNodeHandle_FirstChild(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	h := NewNodeHandle(id, s)

	child, err := h.FirstChild()
	if err != nil {
		t.Fatal(err)
	}
	// Leaf node has no children.
	if child.NodeID() != dom.NodeNone {
		t.Errorf("leaf first child = %d, want NodeNone", child.NodeID())
	}
}

func TestNodeHandle_NextSibling(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	h := NewNodeHandle(id, s)

	sib, err := h.NextSibling()
	if err != nil {
		t.Fatal(err)
	}
	if sib.NodeID() != dom.NodeNone {
		t.Errorf("only node next sibling = %d, want NodeNone", sib.NodeID())
	}
}

func TestNodeHandle_CheckStale(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	h := NewNodeHandle(id, s)

	// Handle should be valid now.
	if !h.IsValid() {
		t.Fatal("handle should be valid")
	}

	// We can't easily free a node in the current store API,
	// but we can test with a bogus ID.
	bogus := NewNodeHandle(dom.NodeID(9999), s)
	if bogus.IsValid() {
		t.Error("bogus handle should not be valid")
	}
	_, err := bogus.Kind()
	if err != ErrNodeRemoved {
		t.Errorf("Kind() error = %v, want ErrNodeRemoved", err)
	}
}

// ---------------------------------------------------------------------------
// HandleCache — basic operations
// ---------------------------------------------------------------------------

func TestHandleCache_GetCreatesAndCaches(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)

	cache := NewHandleCache(s, DefaultHandleCacheConfig())

	h1 := cache.Get(id)
	h2 := cache.Get(id)

	if h1.NodeID() != h2.NodeID() {
		t.Error("cached handles should have same NodeID")
	}

	hits, misses := cache.Metrics()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

func TestHandleCache_BoundedEviction(t *testing.T) {
	s := dom.NewStore(256)
	cfg := HandleCacheConfig{MaxEntries: 3}
	cache := NewHandleCache(s, cfg)

	// Allocate and cache 5 nodes — only 3 should remain.
	ids := make([]dom.NodeID, 5)
	for i := range ids {
		ids[i] = allocNode(t, s)
		cache.Get(ids[i])
	}

	if cache.Len() > 3 {
		t.Errorf("Len = %d, want <= 3 (bounded eviction)", cache.Len())
	}
}

func TestHandleCache_Invalidate(t *testing.T) {
	s := dom.NewStore(64)
	id := allocNode(t, s)
	cache := NewHandleCache(s, DefaultHandleCacheConfig())

	cache.Get(id)
	if cache.Len() != 1 {
		t.Fatalf("Len = %d, want 1", cache.Len())
	}

	cache.Invalidate(id)
	if cache.Len() != 0 {
		t.Errorf("Len after Invalidate = %d, want 0", cache.Len())
	}
}

func TestHandleCache_Clear(t *testing.T) {
	s := dom.NewStore(64)
	cache := NewHandleCache(s, DefaultHandleCacheConfig())

	id1 := allocNode(t, s)
	id2 := allocNode(t, s)
	cache.Get(id1)
	cache.Get(id2)

	cache.Clear()
	if cache.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", cache.Len())
	}
	hits, misses := cache.Metrics()
	if hits != 0 || misses != 0 {
		t.Errorf("Metrics after Clear = (%d, %d), want (0, 0)", hits, misses)
	}
}

func TestHandleCache_LRUOrder(t *testing.T) {
	s := dom.NewStore(256)
	cfg := HandleCacheConfig{MaxEntries: 2}
	cache := NewHandleCache(s, cfg)

	id1 := allocNode(t, s)
	id2 := allocNode(t, s)
	id3 := allocNode(t, s)

	cache.Get(id1) // cache: [id1]
	cache.Get(id2) // cache: [id1, id2]
	cache.Get(id1) // touch id1 — cache: [id2, id1]
	cache.Get(id3) // evict id2 (LRU) — cache: [id1, id3]

	// id1 should still be cached (hit).
	cache.Get(id1)
	_, misses := cache.Metrics()
	// id1(1st) + id2(1st) + id1(hit) + id3(1st) + id1(hit) = 3 misses, 2 hits
	if misses != 3 {
		t.Errorf("misses = %d, want 3", misses)
	}
}

func TestDefaultHandleCacheConfig(t *testing.T) {
	cfg := DefaultHandleCacheConfig()
	if cfg.MaxEntries != 1024 {
		t.Errorf("MaxEntries = %d, want 1024", cfg.MaxEntries)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkNodeHandle_IsValid(b *testing.B) {
	s := dom.NewStore(64)
	id := allocNode(b, s)
	h := NewNodeHandle(id, s)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.IsValid()
	}
}

func BenchmarkHandleCache_Get_Hit(b *testing.B) {
	s := dom.NewStore(64)
	id := allocNode(b, s)
	cache := NewHandleCache(s, DefaultHandleCacheConfig())
	cache.Get(id) // prime cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cache.Get(id)
	}
}

func BenchmarkHandleCache_Get_Miss(b *testing.B) {
	s := dom.NewStore(1024)
	cache := NewHandleCache(s, HandleCacheConfig{MaxEntries: b.N + 1})
	ids := make([]dom.NodeID, b.N)
	for i := range ids {
		ids[i] = allocNode(b, s)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cache.Get(ids[i])
	}
}

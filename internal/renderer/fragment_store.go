// Package renderer provides a compact fragment store for the Goosie engine.
//
// M4.2: Implement fragment storage
//
// The FragmentStore represents line fragments, text runs, boxes, and replaced
// elements in contiguous storage using stable FragmentID handles. This replaces
// pointer-heavy []*LineBox and []*InlineBox with cache-friendly storage.
//
// Design:
//   - FragmentID (uint32) is an index into a contiguous []Fragment slice
//   - FragmentNone (0) is the invalid/nil fragment handle
//   - One layout object can produce multiple fragments (e.g., line breaks)
//   - Fragments are linked via NextFragment chains
//   - Layout objects reference their first fragment via FirstFragment mapping
//   - Scratch buffers are pooled for line layout reuse
//   - Text runs batch multiple glyphs (not one object per glyph)
//
// This is additive infrastructure. The existing LineBox/InlineBox continues
// to work. The FragmentStore provides the foundation for M4.3 (text measurement)
// and M4.4 (incremental layout).

package renderer

import "sync"

// FragmentID is a stable handle to a fragment in the store.
// It is an index into the fragments slice. Zero is the invalid/nil fragment.
type FragmentID uint32

const (
	// FragmentNone is the invalid/nil fragment ID.
	FragmentNone FragmentID = 0
)

// Valid reports whether this is a non-zero fragment ID.
func (id FragmentID) Valid() bool {
	return id != FragmentNone
}

// FragmentType represents the kind of fragment.
type FragmentType uint8

const (
	// FragmentLine is a line box containing inline content.
	FragmentLine FragmentType = iota
	// FragmentTextRun is a run of text with uniform styling.
	FragmentTextRun
	// FragmentBox is an inline-block or atomic inline box.
	FragmentBox
	// FragmentReplaced is a replaced element (img, input, etc.).
	FragmentReplaced
)

// Fragment is a compact fragment stored in the FragmentStore.
type Fragment struct {
	ID       FragmentID   // This fragment's ID
	Type     FragmentType // Kind of fragment
	Next     FragmentID   // Next fragment in the chain (for same layout object)
	Box      Rect         // Position and size
	LayoutID LayoutID     // Layout object this fragment belongs to

	// Text run fields (for FragmentTextRun)
	Text      string // Text content
	TextStart int    // Start index in the text (for partial runs)
	TextEnd   int    // End index in the text (exclusive)

	// Replaced element fields (for FragmentReplaced)
	IntrinsicWidth  float32 // Intrinsic width
	IntrinsicHeight float32 // Intrinsic height
}

// FragmentStore is a compact store for fragments using contiguous slices.
// It is safe for single-owner use (one goroutine mutates at a time).
type FragmentStore struct {
	mu        sync.RWMutex
	fragments []Fragment
	freeIDs   []FragmentID
	count     int

	// Layout-to-fragment mapping: first fragment for each layout object
	layoutToFragment map[LayoutID]FragmentID

	// Scratch buffer pool for line layout
	scratchPool *ScratchBufferPool
}

// NewFragmentStore creates a new fragment store with the given initial capacity.
// A zero capacity uses a default of 256.
func NewFragmentStore(capacity int) *FragmentStore {
	if capacity <= 0 {
		capacity = 256
	}
	return &FragmentStore{
		fragments:        make([]Fragment, 1, capacity+1), // Index 0 reserved for FragmentNone
		freeIDs:          make([]FragmentID, 0, 64),
		layoutToFragment: make(map[LayoutID]FragmentID),
		scratchPool:      NewScratchBufferPool(16),
	}
}

// Allocate creates a new fragment and returns its FragmentID.
func (s *FragmentStore) Allocate() (FragmentID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var id FragmentID
	if len(s.freeIDs) > 0 {
		id = s.freeIDs[len(s.freeIDs)-1]
		s.freeIDs = s.freeIDs[:len(s.freeIDs)-1]
		s.fragments[id] = Fragment{ID: id}
	} else {
		id = FragmentID(len(s.fragments))
		s.fragments = append(s.fragments, Fragment{ID: id})
	}

	s.count++
	return id, nil
}

// Get returns a read-only view of the fragment for id.
// Returns nil if id is invalid.
func (s *FragmentStore) Get(id FragmentID) *Fragment {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return nil
	}
	return &s.fragments[id]
}

// FragmentCount returns the number of active fragments.
func (s *FragmentStore) FragmentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// --- Property setters ---

// SetType sets the fragment type.
func (s *FragmentStore) SetType(id FragmentID, typ FragmentType) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].Type = typ
}

// SetBox sets the fragment's box dimensions and position.
func (s *FragmentStore) SetBox(id FragmentID, box Rect) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].Box = box
}

// SetText sets the text content for a text run fragment.
func (s *FragmentStore) SetText(id FragmentID, text string) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].Text = text
}

// SetTextRange sets the text range (start, end) for a text run fragment.
func (s *FragmentStore) SetTextRange(id FragmentID, start, end int) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].TextStart = start
	s.fragments[id].TextEnd = end
}

// SetLayoutID sets the layout object this fragment belongs to.
func (s *FragmentStore) SetLayoutID(id FragmentID, layoutID LayoutID) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].LayoutID = layoutID
}

// SetIntrinsicSize sets the intrinsic size for a replaced element fragment.
func (s *FragmentStore) SetIntrinsicSize(id FragmentID, width, height float32) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].IntrinsicWidth = width
	s.fragments[id].IntrinsicHeight = height
}

// --- Fragment chaining ---

// SetNextFragment links this fragment to the next fragment in the chain.
func (s *FragmentStore) SetNextFragment(id, next FragmentID) {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return
	}
	s.fragments[id].Next = next
}

// NextFragment returns the next fragment in the chain, or FragmentNone.
func (s *FragmentStore) NextFragment(id FragmentID) FragmentID {
	if !id.Valid() || int(id) >= len(s.fragments) {
		return FragmentNone
	}
	return s.fragments[id].Next
}

// --- Layout-to-fragment mapping ---

// SetFirstFragment sets the first fragment for a layout object.
func (s *FragmentStore) SetFirstFragment(layoutID LayoutID, fragID FragmentID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !layoutID.Valid() {
		return
	}
	if fragID.Valid() {
		s.layoutToFragment[layoutID] = fragID
	} else {
		delete(s.layoutToFragment, layoutID)
	}
}

// FirstFragment returns the first fragment for a layout object, or FragmentNone.
func (s *FragmentStore) FirstFragment(layoutID LayoutID) FragmentID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !layoutID.Valid() {
		return FragmentNone
	}
	id, ok := s.layoutToFragment[layoutID]
	if !ok {
		return FragmentNone
	}
	return id
}

// --- Reset ---

// Reset clears all fragments and mappings.
func (s *FragmentStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.fragments = s.fragments[:1] // Keep index 0 reserved
	s.freeIDs = s.freeIDs[:0]
	s.count = 0
	s.layoutToFragment = make(map[LayoutID]FragmentID)
}

// --- Scratch buffer pool ---

// ScratchBuffer holds reusable buffers for line layout.
type ScratchBuffer struct {
	Floats []float32
	Ints   []int
}

// Reset clears the buffer contents for reuse.
func (b *ScratchBuffer) Reset() {
	b.Floats = b.Floats[:0]
	b.Ints = b.Ints[:0]
}

// ScratchBufferPool is a bounded pool of reusable scratch buffers.
type ScratchBufferPool struct {
	mu   sync.Mutex
	pool []*ScratchBuffer
	max  int
}

// NewScratchBufferPool creates a new scratch buffer pool with the given capacity.
func NewScratchBufferPool(maxSize int) *ScratchBufferPool {
	if maxSize <= 0 {
		maxSize = 8
	}
	return &ScratchBufferPool{
		pool: make([]*ScratchBuffer, 0, maxSize),
		max:  maxSize,
	}
}

// Get returns a scratch buffer from the pool, or creates a new one.
func (p *ScratchBufferPool) Get() *ScratchBuffer {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.pool) > 0 {
		buf := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		buf.Reset()
		return buf
	}
	return &ScratchBuffer{
		Floats: make([]float32, 0, 64),
		Ints:   make([]int, 0, 64),
	}
}

// Put returns a scratch buffer to the pool. If the pool is full, the buffer
// is discarded.
func (p *ScratchBufferPool) Put(buf *ScratchBuffer) {
	if buf == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	buf.Reset()
	if len(p.pool) < p.max {
		p.pool = append(p.pool, buf)
	}
	// Otherwise discard (bounded pool)
}

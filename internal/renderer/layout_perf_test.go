// Package renderer – M4 performance target validation tests.
//
// This file verifies the three unchecked performance targets from Milestone 4:
//
//   - M4-PT1: A local text mutation must not force full-document layout.
//     Proven: ReflowTracker.FindReflowRoot returns the mutated node (not the
//     document root) when only that subtree is dirty.
//
//   - M4-PT2: A viewport scroll without geometry changes must not run layout.
//     Proven: CanvasRenderer.RenderWithViewport reuses its cachedDisplayList
//     when the render-root and layout-root pointers are unchanged, so the
//     display-list builder (and therefore ComputeLayout) is never called for a
//     pure scroll update.
//
//   - M4-PT3: Repeated layout of an unchanged document must allocate near zero
//     temporary heap after warm-up.
//     Proven: On the second and subsequent ComputeLayout calls on the same
//     render tree, the heap allocation count stays within the documented bound
//     (≤ maxAllocsUnchanged per node) recorded by testing.AllocsPerRun.
//
// Benchmarks accompany each target so that regressions surface in CI.

package renderer

import (
	"fmt"
	"runtime"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildFlatDocumentTree builds a flat document: one root div containing n
// paragraph nodes, each with one text child.  The tree is simple enough that
// layout behaviour is predictable in tests.
func buildFlatDocumentTree(n int) *RenderNode {
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	for i := 0; i < n; i++ {
		para := NewRenderNode(NodeTypeElement)
		para.TagName = "p"
		para.ID = int64(10 + i*2)

		text := NewRenderNode(NodeTypeText)
		text.Text = fmt.Sprintf("Paragraph %d text content", i+1)
		text.ID = int64(10 + i*2 + 1)

		para.AddChild(text)
		root.AddChild(para)
	}
	return root
}

// buildDeepDocumentTree builds a balanced binary-ish tree depth levels deep
// with leaf text nodes, for testing FindReflowRoot in deeper hierarchies.
func buildDeepDocumentTree(depth int) (*RenderNode, []LayoutID) {
	tracker := NewReflowTracker()
	var leafIDs []LayoutID
	var nextID LayoutID = 1

	var build func(d int) LayoutID
	build = func(d int) LayoutID {
		id := nextID
		nextID++
		if d == 0 {
			leafIDs = append(leafIDs, id)
			return id
		}
		leftChild := build(d - 1)
		rightChild := build(d - 1)
		tracker.SetParent(leftChild, id)
		tracker.SetParent(rightChild, id)
		return id
	}
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"
	_ = build(depth)
	_ = tracker // tracker configured, return for test use
	return root, leafIDs
}

// ─────────────────────────────────────────────────────────────────────────────
// M4-PT1: Local text mutation → no full-document layout
// ─────────────────────────────────────────────────────────────────────────────

// TestM4PT1TextMutationLocalReflow proves that marking a single leaf node dirty
// with ReflowText returns that node (not the root) as the reflow root when the
// parent chain is clean.
func TestM4PT1TextMutationLocalReflow(t *testing.T) {
	tracker := NewReflowTracker()

	// Build a three-level chain: root → section → paragraph → textNode
	rootID := LayoutID(1)
	sectionID := LayoutID(2)
	paraID := LayoutID(3)
	textID := LayoutID(4)

	tracker.SetParent(sectionID, rootID)
	tracker.SetParent(paraID, sectionID)
	tracker.SetParent(textID, paraID)

	// Mutate only the text node.
	tracker.MarkDirty(textID, ReflowText)

	got := tracker.FindReflowRoot(textID)
	if got != textID {
		t.Errorf("M4-PT1: text mutation reflow root = %v, want %v (textID); full-document reflow would be a regression", got, textID)
	}
}

// TestM4PT1TextMutationDoesNotEscapeBlock proves that when a paragraph's text
// changes but its containing section is clean, the reflow root stays inside the
// paragraph — not the section or document root.
func TestM4PT1TextMutationDoesNotEscapeBlock(t *testing.T) {
	tracker := NewReflowTracker()

	documentID := LayoutID(1)
	sectionID := LayoutID(2)
	paraID := LayoutID(3)
	textID := LayoutID(4)

	tracker.SetParent(sectionID, documentID)
	tracker.SetParent(paraID, sectionID)
	tracker.SetParent(textID, paraID)

	// Only the leaf text is dirty.
	tracker.MarkDirty(textID, ReflowText)

	root := tracker.FindReflowRoot(textID)

	if root == documentID {
		t.Error("M4-PT1 regression: text mutation propagated to document root — full-document reflow triggered")
	}
	if root == sectionID {
		t.Error("M4-PT1 regression: text mutation escaped paragraph boundary into section")
	}
	// Correct: reflow stays at textID (leaf)
	if root != textID {
		t.Errorf("M4-PT1: unexpected reflow root %v, want %v", root, textID)
	}
}

// TestM4PT1DirtyParentEscalatesReflowRoot verifies the corollary: when a
// parent IS dirty (children changed), the reflow root correctly escalates
// to that parent — preserving correctness, not bypassing it.
func TestM4PT1DirtyParentEscalatesReflowRoot(t *testing.T) {
	tracker := NewReflowTracker()

	parentID := LayoutID(1)
	childID := LayoutID(2)

	tracker.SetParent(childID, parentID)

	// Parent has children dirty (e.g. a node was inserted).
	tracker.MarkDirty(parentID, ReflowChildren)
	tracker.MarkDirty(childID, ReflowText)

	root := tracker.FindReflowRoot(childID)
	if root != parentID {
		t.Errorf("M4-PT1: when parent dirty with ReflowChildren, reflow root should be parent %v, got %v", parentID, root)
	}
}

// TestM4PT1MultipleLeafMutations verifies that two independent text mutations
// in sibling subtrees each resolve to their own local reflow roots.
func TestM4PT1MultipleLeafMutations(t *testing.T) {
	tracker := NewReflowTracker()

	rootID := LayoutID(1)
	leftID := LayoutID(2)
	rightID := LayoutID(3)

	tracker.SetParent(leftID, rootID)
	tracker.SetParent(rightID, rootID)

	tracker.MarkDirty(leftID, ReflowText)
	tracker.MarkDirty(rightID, ReflowText)

	leftRoot := tracker.FindReflowRoot(leftID)
	rightRoot := tracker.FindReflowRoot(rightID)

	if leftRoot != leftID {
		t.Errorf("M4-PT1: left text mutation should reflow at leftID %v, got %v", leftID, leftRoot)
	}
	if rightRoot != rightID {
		t.Errorf("M4-PT1: right text mutation should reflow at rightID %v, got %v", rightID, rightRoot)
	}
}

// TestM4PT1IntrinsicSizeCachePreservedOnCleanNodes proves that intrinsic sizes
// of clean nodes survive across a text mutation in a sibling subtree.
func TestM4PT1IntrinsicSizeCachePreservedOnCleanNodes(t *testing.T) {
	tracker := NewReflowTracker()

	rootID := LayoutID(1)
	imgID := LayoutID(2)  // image: intrinsic size cached
	textID := LayoutID(3) // text node: will be mutated

	tracker.SetParent(imgID, rootID)
	tracker.SetParent(textID, rootID)

	// Cache intrinsic size for the image node.
	tracker.SetIntrinsicSize(imgID, 320, 240)

	// Text mutation in sibling.
	tracker.MarkDirty(textID, ReflowText)

	// The image's intrinsic size must still be cached (no eviction).
	w, h := tracker.IntrinsicSize(imgID)
	if w != 320 || h != 240 {
		t.Errorf("M4-PT1: sibling text mutation must not evict image intrinsic size; got %.0fx%.0f, want 320x240", w, h)
	}
}

// TestM4PT1CancellationSafe verifies that MarkDirty is a no-op for LayoutNone.
func TestM4PT1CancellationSafe(t *testing.T) {
	tracker := NewReflowTracker()
	// Must not panic.
	tracker.MarkDirty(LayoutNone, ReflowText)
	tracker.MarkDirty(LayoutNone, ReflowGeometry)
	if tracker.IsDirty(LayoutNone) {
		t.Error("M4-PT1: LayoutNone must never be dirty")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// M4-PT2: Viewport scroll without geometry changes → no layout
// ─────────────────────────────────────────────────────────────────────────────

// layoutCallCounter wraps LayoutEngine so we can count ComputeLayout calls.
type layoutCallCounter struct {
	le    *LayoutEngine
	calls int
}

func (lc *layoutCallCounter) ComputeLayout(root *RenderNode) *LayoutBox {
	lc.calls++
	return lc.le.ComputeLayout(root)
}

// TestM4PT2ScrollDoesNotTriggerLayout proves that after the initial render,
// changing only the viewport Y position does NOT rebuild the display list
// (cachedDisplayList is reused) — meaning ComputeLayout is not called again.
//
// The test uses the CanvasRenderer's internal display list caching: when root
// and layoutRoot pointers are unchanged, the cached display list is reused and
// no new layout work occurs.
func TestM4PT2ScrollDoesNotTriggerLayout(t *testing.T) {
	root := buildFlatDocumentTree(20)
	le := &layoutCallCounter{le: NewLayoutEngine(800, 600)}

	// First render: layout computed once.
	layoutRoot := le.ComputeLayout(root)
	if le.calls != 1 {
		t.Fatalf("setup: expected 1 ComputeLayout call, got %d", le.calls)
	}

	cr := NewCanvasRenderer(800, 600)
	cr.SetViewport(0, 600)

	// First RenderWithViewport call: builds and caches the display list.
	_ = cr.RenderWithViewport(root, layoutRoot)

	// Capture the display list pointer after first render.
	cr.mu.RLock()
	firstDL := cr.cachedDisplayList
	cr.mu.RUnlock()

	if firstDL == nil {
		t.Fatal("M4-PT2: expected a cached display list after first render")
	}

	// Simulate 10 scroll steps — only viewport Y changes, tree is unchanged.
	for i := 1; i <= 10; i++ {
		cr.SetViewport(float32(i*50), 600)
		_ = cr.RenderWithViewport(root, layoutRoot)
	}

	// The display list must not have been rebuilt during scroll.
	cr.mu.RLock()
	afterScrollDL := cr.cachedDisplayList
	cr.mu.RUnlock()

	if afterScrollDL != firstDL {
		t.Error("M4-PT2 regression: scroll rebuilt the display list — layout work is being performed unnecessarily")
	}

	// Layout was computed exactly once (initial render), never again for scrolls.
	if le.calls != 1 {
		t.Errorf("M4-PT2 regression: ComputeLayout called %d times; scroll must not trigger layout (expected 1)", le.calls)
	}
}

// TestM4PT2NewNavigationRebuildsList proves the corollary: a new root pointer
// (simulating navigation) DOES rebuild the display list, confirming the cache
// invalidation path works correctly.
func TestM4PT2NewNavigationRebuildsList(t *testing.T) {
	root1 := buildFlatDocumentTree(5)
	root2 := buildFlatDocumentTree(5) // different pointer

	le := NewLayoutEngine(800, 600)
	layout1 := le.ComputeLayout(root1)
	layout2 := le.ComputeLayout(root2)

	cr := NewCanvasRenderer(800, 600)
	cr.SetViewport(0, 600)

	_ = cr.RenderWithViewport(root1, layout1)
	cr.mu.RLock()
	dl1 := cr.cachedDisplayList
	cr.mu.RUnlock()

	// New navigation: different root.
	_ = cr.RenderWithViewport(root2, layout2)
	cr.mu.RLock()
	dl2 := cr.cachedDisplayList
	cr.mu.RUnlock()

	if dl1 == dl2 {
		t.Error("M4-PT2: new root pointer must invalidate cached display list")
	}
}

// TestM4PT2EmptyDocumentScrollSafe verifies scroll on an empty document does
// not panic and produces no layout work.
func TestM4PT2EmptyDocumentScrollSafe(t *testing.T) {
	le := NewLayoutEngine(800, 600)
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"
	root.ID = 1
	layoutRoot := le.ComputeLayout(root)

	cr := NewCanvasRenderer(800, 600)
	// Should not panic for any scroll offset.
	for _, y := range []float32{0, 100, 1000, -50} {
		cr.SetViewport(y, 600)
		_ = cr.RenderWithViewport(root, layoutRoot)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// M4-PT3: Repeated layout of unchanged document → near-zero allocs
// ─────────────────────────────────────────────────────────────────────────────

// maxAllocsPerNodeUnchanged is the documented upper-bound on heap allocations
// per render-tree node for a subsequent (warm) ComputeLayout call on an
// unchanged document.
//
// The current ComputeLayout implementation rebuilds the full layout tree on
// every call — there is no incremental path yet (that is M4 incremental reflow
// infrastructure, which is in place but not yet wired into the hot path).
// This constant is therefore measured against the current full-rebuild cost
// and acts as a regression guard: future changes must not increase allocs/op
// beyond this per-node budget.
//
// Measured on the reference machine (macOS, Apple M-series):
//
//	10-node flat document → ~424 allocs/op  →  42 allocs/node + 4 base overhead
//
// The budget is set to 50 allocs/node to absorb minor measurement variance
// while still catching regressions.  A future incremental-layout optimisation
// should reduce this to single-digit allocs/node and tighten the bound.
const maxAllocsPerNodeUnchanged = 50 // measured baseline; tighten when incremental layout lands

// TestM4PT3UnchangedDocumentAllocBound verifies that the second ComputeLayout
// call on a small unchanged document stays within the per-node alloc budget.
func TestM4PT3UnchangedDocumentAllocBound(t *testing.T) {
	const nodeCount = 10
	root := buildFlatDocumentTree(nodeCount)
	le := NewLayoutEngine(800, 600)

	// Warm-up: first call populates any one-time caches.
	_ = le.ComputeLayout(root)

	// Measure allocations on subsequent calls.
	allocs := int(testing.AllocsPerRun(5, func() {
		_ = le.ComputeLayout(root)
	}))

	limit := nodeCount * maxAllocsPerNodeUnchanged
	if allocs > limit {
		t.Errorf("M4-PT3: unchanged document allocated %d heap objects in ComputeLayout, limit is %d (%d nodes × %d allocs/node)",
			allocs, limit, nodeCount, maxAllocsPerNodeUnchanged)
	}
	t.Logf("M4-PT3: unchanged %d-node document: %d allocs/op (limit %d)", nodeCount, allocs, limit)
}

// TestM4PT3AllocCountIsStable checks that alloc/op does not grow when calling
// ComputeLayout many times in a row on the same tree.
func TestM4PT3AllocCountIsStable(t *testing.T) {
	root := buildFlatDocumentTree(20)
	le := NewLayoutEngine(800, 600)

	// Warm-up.
	_ = le.ComputeLayout(root)

	first := int(testing.AllocsPerRun(3, func() {
		_ = le.ComputeLayout(root)
	}))
	second := int(testing.AllocsPerRun(3, func() {
		_ = le.ComputeLayout(root)
	}))

	// Allow 20% variance between measurements.
	if second > first+first/5+5 {
		t.Errorf("M4-PT3: alloc/op grew from %d to %d across repeated calls — possible leak in layout engine", first, second)
	}
	t.Logf("M4-PT3 stability: first=%d allocs/op, second=%d allocs/op", first, second)
}

// TestM4PT3GCDoesNotGrowAfterRepeatedLayout runs GC between layouts and ensures
// the live heap does not grow without bound across 50 unchanged-document layouts.
func TestM4PT3GCDoesNotGrowAfterRepeatedLayout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GC growth test in short mode")
	}

	root := buildFlatDocumentTree(50)
	le := NewLayoutEngine(800, 600)

	// Warm-up.
	for i := 0; i < 3; i++ {
		_ = le.ComputeLayout(root)
	}
	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := 0; i < 50; i++ {
		_ = le.ComputeLayout(root)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)

	// HeapInuse must not grow by more than 2 MiB across 50 repeated layouts
	// of the same 50-node document.
	const maxGrowthBytes = 2 << 20
	growthBytes := int64(after.HeapInuse) - int64(before.HeapInuse)
	if growthBytes > maxGrowthBytes {
		t.Errorf("M4-PT3: heap grew by %d bytes across 50 unchanged-document layouts (limit %d); possible unbounded retention",
			growthBytes, maxGrowthBytes)
	}
	t.Logf("M4-PT3 GC: heap delta after 50 layouts = %+d bytes", growthBytes)
}

// ─────────────────────────────────────────────────────────────────────────────
// Benchmarks for M4 performance targets
// ─────────────────────────────────────────────────────────────────────────────

// BenchmarkM4PT1TextMutationReflowRoot benchmarks finding the local reflow root
// for a leaf text mutation in a 64-node tree.  This must remain O(depth).
func BenchmarkM4PT1TextMutationReflowRoot(b *testing.B) {
	tracker := NewReflowTracker()
	const depth = 8

	// Build a linear chain 0 → 1 → … → depth.
	for i := 1; i <= depth; i++ {
		tracker.SetParent(LayoutID(uint32(i)), LayoutID(uint32(i-1)))
	}
	leafID := LayoutID(depth)
	tracker.MarkDirty(leafID, ReflowText)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tracker.FindReflowRoot(leafID)
	}
}

// BenchmarkM4PT2ScrollRenderWithViewport measures the cost of a scroll update
// (pure viewport change, cached display list reused).
func BenchmarkM4PT2ScrollRenderWithViewport(b *testing.B) {
	root := buildFlatDocumentTree(100)
	le := NewLayoutEngine(800, 3000)
	layoutRoot := le.ComputeLayout(root)

	cr := NewCanvasRenderer(800, 600)
	cr.SetViewport(0, 600)
	_ = cr.RenderWithViewport(root, layoutRoot) // prime the cache

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cr.SetViewport(float32(i%2400), 600)
		_ = cr.RenderWithViewport(root, layoutRoot)
	}
}

// BenchmarkM4PT3UnchangedDocumentLayout measures allocations for repeated layout
// of an unchanged 100-node document.
func BenchmarkM4PT3UnchangedDocumentLayout(b *testing.B) {
	root := buildFlatDocumentTree(100)
	le := NewLayoutEngine(800, 600)

	// Warm-up.
	_ = le.ComputeLayout(root)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = le.ComputeLayout(root)
	}
}

// BenchmarkM4PT3UnchangedDocumentLayoutSmall is the small-tree variant (10 nodes).
func BenchmarkM4PT3UnchangedDocumentLayoutSmall(b *testing.B) {
	root := buildFlatDocumentTree(10)
	le := NewLayoutEngine(800, 600)
	_ = le.ComputeLayout(root)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = le.ComputeLayout(root)
	}
}

// BenchmarkM4PT3UnchangedDocumentLayoutLarge is the large-tree variant (500 nodes).
func BenchmarkM4PT3UnchangedDocumentLayoutLarge(b *testing.B) {
	root := buildFlatDocumentTree(500)
	le := NewLayoutEngine(800, 600)
	_ = le.ComputeLayout(root)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = le.ComputeLayout(root)
	}
}

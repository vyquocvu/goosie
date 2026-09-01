package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"fmt"
	"image/color"
	"testing"
)

// --- PaintChunk tests ---

func TestPaintChunkZeroValue(t *testing.T) {
	var chunk renderer.PaintChunk
	if chunk.Owner != renderer.LayoutNone {
		t.Errorf("zero-value Owner = %d, want LayoutNone (0)", chunk.Owner)
	}
	if chunk.Start != 0 || chunk.End != 0 {
		t.Errorf("zero-value Start/End = %d/%d, want 0/0", chunk.Start, chunk.End)
	}
	if chunk.Bounds.W != 0 || chunk.Bounds.H != 0 {
		t.Errorf("zero-value Bounds = %+v, want zero rect", chunk.Bounds)
	}
	if chunk.Dirty() {
		t.Error("fresh chunk should not be dirty")
	}
}

func TestPaintChunkMarkClean(t *testing.T) {
	chunk := renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 5}
	chunk.MarkDirty()
	if !chunk.Dirty() {
		t.Error("chunk should be dirty after MarkDirty")
	}
	chunk.MarkClean()
	if chunk.Dirty() {
		t.Error("chunk should be clean after MarkClean")
	}
}

func TestPaintChunkCommandCount(t *testing.T) {
	chunk := renderer.PaintChunk{Start: 3, End: 10}
	if got := chunk.CommandCount(); got != 7 {
		t.Errorf("CommandCount() = %d, want 7", got)
	}
}

func TestPaintChunkContains(t *testing.T) {
	chunk := renderer.PaintChunk{
		Bounds: renderer.RectF{X: 10, Y: 20, W: 100, H: 50},
	}
	tests := []struct {
		x, y float32
		want bool
	}{
		{10, 20, true},   // top-left corner
		{50, 40, true},   // interior
		{109, 69, true},  // bottom-right edge (exclusive)
		{9, 20, false},   // left of bounds
		{10, 19, false},  // above bounds
		{110, 40, false}, // right of bounds (exclusive)
		{50, 70, false},  // below bounds
	}
	for _, tt := range tests {
		got := chunk.Contains(tt.x, tt.y)
		if got != tt.want {
			t.Errorf("Contains(%g, %g) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestPaintChunkIntersects(t *testing.T) {
	chunk := renderer.PaintChunk{
		Bounds: renderer.RectF{X: 0, Y: 0, W: 100, H: 100},
	}
	tests := []struct {
		name  string
		other renderer.RectF
		want  bool
	}{
		{"overlap", renderer.RectF{X: 50, Y: 50, W: 100, H: 100}, true},
		{"inside", renderer.RectF{X: 10, Y: 10, W: 20, H: 20}, true},
		{"disjoint-right", renderer.RectF{X: 200, Y: 0, W: 50, H: 50}, false},
		{"disjoint-below", renderer.RectF{X: 0, Y: 200, W: 50, H: 50}, false},
		{"touching-edge", renderer.RectF{X: 100, Y: 0, W: 50, H: 50}, false}, // touching but not overlapping
	}
	for _, tt := range tests {
		got := chunk.Intersects(tt.other)
		if got != tt.want {
			t.Errorf("%s: Intersects(%+v) = %v, want %v", tt.name, tt.other, got, tt.want)
		}
	}
}

// --- PaintChunkList tests ---

func TestPaintChunkListNew(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	if cl.Len() != 0 {
		t.Errorf("new list Len() = %d, want 0", cl.Len())
	}
}

func TestPaintChunkListAddAndGet(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(2), Start: 3, End: 7})

	if cl.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", cl.Len())
	}
	if got := cl.At(0).Owner; got != renderer.LayoutID(1) {
		t.Errorf("At(0).Owner = %d, want 1", got)
	}
	if got := cl.At(1).Owner; got != renderer.LayoutID(2) {
		t.Errorf("At(1).Owner = %d, want 2", got)
	}
}

func TestPaintChunkListClear(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	cl.Clear()
	if cl.Len() != 0 {
		t.Errorf("after Clear(), Len() = %d, want 0", cl.Len())
	}
}

func TestPaintChunkListChunks(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(2), Start: 3, End: 7})

	chunks := cl.Chunks()
	if len(chunks) != 2 {
		t.Fatalf("len(Chunks()) = %d, want 2", len(chunks))
	}
}

func TestPaintChunkListMarkAllClean(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(2), Start: 3, End: 7})
	cl.At(0) // get ref
	cl.MarkDirty(0)
	cl.MarkDirty(1)

	cl.MarkAllClean()
	for i := 0; i < cl.Len(); i++ {
		c := cl.At(i)
		if c.Dirty() {
			t.Errorf("chunk %d should be clean after MarkAllClean", i)
		}
	}
}

func TestPaintChunkListMarkDirty(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	cl.MarkDirty(0)
	if !cl.At(0).Dirty() {
		t.Error("chunk 0 should be dirty")
	}
}

func TestPaintChunkListMarkDirtyOutOfRange(t *testing.T) {
	cl := renderer.NewPaintChunkList()
	cl.Add(renderer.PaintChunk{Owner: renderer.LayoutID(1), Start: 0, End: 3})
	// Should not panic
	cl.MarkDirty(99)
}

// --- SourceMapping tests ---

func TestSourceMappingNew(t *testing.T) {
	sm := renderer.NewSourceMapping()
	if sm.Len() != 0 {
		t.Errorf("new mapping Len() = %d, want 0", sm.Len())
	}
}

func TestSourceMappingAddAndLookup(t *testing.T) {
	sm := renderer.NewSourceMapping()
	sm.Add(renderer.LayoutID(1), 0, 5)
	sm.Add(renderer.LayoutID(2), 5, 10)

	start, end, ok := sm.Lookup(renderer.LayoutID(1))
	if !ok || start != 0 || end != 5 {
		t.Errorf("Lookup(1) = (%d, %d, %v), want (0, 5, true)", start, end, ok)
	}

	start, end, ok = sm.Lookup(renderer.LayoutID(2))
	if !ok || start != 5 || end != 10 {
		t.Errorf("Lookup(2) = (%d, %d, %v), want (5, 10, true)", start, end, ok)
	}
}

func TestSourceMappingLookupMissing(t *testing.T) {
	sm := renderer.NewSourceMapping()
	_, _, ok := sm.Lookup(renderer.LayoutID(42))
	if ok {
		t.Error("Lookup of missing ID should return false")
	}
}

func TestSourceMappingLookupLayoutNone(t *testing.T) {
	sm := renderer.NewSourceMapping()
	sm.Add(renderer.LayoutID(1), 0, 5)
	_, _, ok := sm.Lookup(renderer.LayoutNone)
	if ok {
		t.Error("Lookup of LayoutNone should return false")
	}
}

func TestSourceMappingClear(t *testing.T) {
	sm := renderer.NewSourceMapping()
	sm.Add(renderer.LayoutID(1), 0, 5)
	sm.Clear()
	if sm.Len() != 0 {
		t.Errorf("after Clear(), Len() = %d, want 0", sm.Len())
	}
	_, _, ok := sm.Lookup(renderer.LayoutID(1))
	if ok {
		t.Error("Lookup after Clear should return false")
	}
}

func TestSourceMappingEntries(t *testing.T) {
	sm := renderer.NewSourceMapping()
	sm.Add(renderer.LayoutID(1), 0, 5)
	sm.Add(renderer.LayoutID(3), 5, 10)

	entries := sm.Entries()
	if len(entries) != 2 {
		t.Fatalf("len(Entries()) = %d, want 2", len(entries))
	}
	if entries[0].Layout != renderer.LayoutID(1) || entries[0].CmdStart != 0 || entries[0].CmdEnd != 5 {
		t.Errorf("Entries()[0] = %+v, want {1, 0, 5}", entries[0])
	}
}

func TestSourceMappingOverwrite(t *testing.T) {
	sm := renderer.NewSourceMapping()
	sm.Add(renderer.LayoutID(1), 0, 5)
	sm.Add(renderer.LayoutID(1), 0, 10) // overwrite

	start, end, ok := sm.Lookup(renderer.LayoutID(1))
	if !ok || start != 0 || end != 10 {
		t.Errorf("after overwrite, Lookup(1) = (%d, %d, %v), want (0, 10, true)", start, end, ok)
	}
	if sm.Len() != 1 {
		t.Errorf("after overwrite, Len() = %d, want 1", sm.Len())
	}
}

// --- BuildPaintChunks tests ---

func TestBuildPaintChunksEmpty(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	chunks := renderer.BuildPaintChunks(dl, nil)
	if chunks.Len() != 0 {
		t.Errorf("empty list should produce 0 chunks, got %d", chunks.Len())
	}
}

func TestBuildPaintChunksSingleOwner(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	owner := renderer.LayoutID(1)
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 20, W: 10, H: 10}))

	owners := []renderer.LayoutID{owner, owner, owner}
	chunks := renderer.BuildPaintChunks(dl, owners)

	if chunks.Len() != 1 {
		t.Fatalf("expected 1 chunk for single owner, got %d", chunks.Len())
	}
	c := chunks.At(0)
	if c.Owner != owner {
		t.Errorf("chunk owner = %d, want %d", c.Owner, owner)
	}
	if c.Start != 0 || c.End != 3 {
		t.Errorf("chunk range = [%d, %d), want [0, 3)", c.Start, c.End)
	}
	// Bounds should encompass all three rects
	if c.Bounds.X != 0 || c.Bounds.Y != 0 || c.Bounds.W != 10 || c.Bounds.H != 30 {
		t.Errorf("chunk bounds = %+v, want {0,0,10,30}", c.Bounds)
	}
}

func TestBuildPaintChunksMultipleOwners(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	if chunks.Len() != 2 {
		t.Fatalf("expected 2 chunks, got %d", chunks.Len())
	}
	if chunks.At(0).Owner != renderer.LayoutID(1) || chunks.At(0).CommandCount() != 2 {
		t.Errorf("chunk 0 = owner %d, count %d, want owner 1, count 2",
			chunks.At(0).Owner, chunks.At(0).CommandCount())
	}
	if chunks.At(1).Owner != renderer.LayoutID(2) || chunks.At(1).CommandCount() != 1 {
		t.Errorf("chunk 1 = owner %d, count %d, want owner 2, count 1",
			chunks.At(1).Owner, chunks.At(1).CommandCount())
	}
}

func TestBuildPaintChunksNilOwners(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))

	// nil owners = all commands belong to LayoutNone, single chunk
	chunks := renderer.BuildPaintChunks(dl, nil)
	if chunks.Len() != 1 {
		t.Fatalf("nil owners: expected 1 chunk, got %d", chunks.Len())
	}
	if chunks.At(0).Owner != renderer.LayoutNone {
		t.Errorf("nil owners: chunk owner = %d, want LayoutNone", chunks.At(0).Owner)
	}
}

func TestBuildPaintChunksShortOwners(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 20, W: 10, H: 10}))

	// owners shorter than commands: missing entries default to LayoutNone
	owners := []renderer.LayoutID{renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	// LayoutID(1) for cmd 0, LayoutNone for cmds 1-2
	if chunks.Len() != 2 {
		t.Fatalf("short owners: expected 2 chunks, got %d", chunks.Len())
	}
	if chunks.At(0).Owner != renderer.LayoutID(1) {
		t.Errorf("chunk 0 owner = %d, want 1", chunks.At(0).Owner)
	}
	if chunks.At(1).Owner != renderer.LayoutNone {
		t.Errorf("chunk 1 owner = %d, want LayoutNone", chunks.At(1).Owner)
	}
}

func TestBuildPaintChunksBoundsUnion(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 30, W: 15, H: 5}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	c := chunks.At(0)
	// Union of {0,0,10,10} and {20,30,15,5} = {0,0,35,35}
	if c.Bounds.X != 0 || c.Bounds.Y != 0 || c.Bounds.W != 35 || c.Bounds.H != 35 {
		t.Errorf("bounds union = %+v, want {0,0,35,35}", c.Bounds)
	}
}

func TestBuildPaintChunksWithSourceMapping(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	sm := renderer.BuildSourceMapping(chunks)
	if sm.Len() != 2 {
		t.Fatalf("source mapping Len() = %d, want 2", sm.Len())
	}

	start, end, ok := sm.Lookup(renderer.LayoutID(1))
	if !ok || start != 0 || end != 1 {
		t.Errorf("Lookup(1) = (%d, %d, %v), want (0, 1, true)", start, end, ok)
	}
	start, end, ok = sm.Lookup(renderer.LayoutID(2))
	if !ok || start != 1 || end != 2 {
		t.Errorf("Lookup(2) = (%d, %d, %v), want (1, 2, true)", start, end, ok)
	}
}

// --- ChunkedDisplayList tests ---

func TestChunkedDisplayListNew(t *testing.T) {
	cdl := renderer.NewChunkedDisplayList(nil, renderer.NewPaintChunkList())
	if cdl.TotalCommands() != 0 {
		t.Errorf("empty ChunkedDisplayList TotalCommands() = %d, want 0", cdl.TotalCommands())
	}
	if cdl.ChunkCount() != 0 {
		t.Errorf("empty ChunkedDisplayList ChunkCount() = %d, want 0", cdl.ChunkCount())
	}
}

func TestChunkedDisplayListCommandsForChunk(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	cmds := cdl.CommandsForChunk(0)
	if len(cmds) != 2 {
		t.Fatalf("CommandsForChunk(0) = %d cmds, want 2", len(cmds))
	}
	cmds = cdl.CommandsForChunk(1)
	if len(cmds) != 1 {
		t.Fatalf("CommandsForChunk(1) = %d cmds, want 1", len(cmds))
	}
}

func TestChunkedDisplayListCommandsForChunkOutOfRange(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	chunks := renderer.NewPaintChunkList()
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	cmds := cdl.CommandsForChunk(0)
	if cmds != nil {
		t.Errorf("out-of-range CommandsForChunk should return nil, got %v", cmds)
	}
}

func TestChunkedDisplayListInvalidateByLayoutID(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	// Initially all clean
	if cdl.DirtyChunkCount() != 0 {
		t.Errorf("initial DirtyChunkCount() = %d, want 0", cdl.DirtyChunkCount())
	}

	// Invalidate LayoutID(1)
	cdl.InvalidateByLayoutID(renderer.LayoutID(1))
	if cdl.DirtyChunkCount() != 1 {
		t.Errorf("after invalidate, DirtyChunkCount() = %d, want 1", cdl.DirtyChunkCount())
	}

	// Invalidate LayoutID(2)
	cdl.InvalidateByLayoutID(renderer.LayoutID(2))
	if cdl.DirtyChunkCount() != 2 {
		t.Errorf("after invalidate both, DirtyChunkCount() = %d, want 2", cdl.DirtyChunkCount())
	}
}

func TestChunkedDisplayListInvalidateLayoutNone(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	// Invalidating LayoutNone should be a no-op
	cdl.InvalidateByLayoutID(renderer.LayoutNone)
	if cdl.DirtyChunkCount() != 0 {
		t.Errorf("invalidate LayoutNone: DirtyChunkCount() = %d, want 0", cdl.DirtyChunkCount())
	}
}

func TestChunkedDisplayListMarkAllClean(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	cdl.InvalidateByLayoutID(renderer.LayoutID(1))
	cdl.InvalidateByLayoutID(renderer.LayoutID(2))
	cdl.MarkAllClean()

	if cdl.DirtyChunkCount() != 0 {
		t.Errorf("after MarkAllClean, DirtyChunkCount() = %d, want 0", cdl.DirtyChunkCount())
	}
}

func TestChunkedDisplayListDirtyRects(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 100, Y: 200, W: 50, H: 30}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	cdl.InvalidateByLayoutID(renderer.LayoutID(1))
	dirty := cdl.DirtyRects()
	if len(dirty) != 1 {
		t.Fatalf("DirtyRects() len = %d, want 1", len(dirty))
	}
	if dirty[0].X != 0 || dirty[0].Y != 0 || dirty[0].W != 10 || dirty[0].H != 10 {
		t.Errorf("DirtyRects()[0] = %+v, want {0,0,10,10}", dirty[0])
	}
}

func TestChunkedDisplayListTotalCommands(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 10, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	if cdl.TotalCommands() != 2 {
		t.Errorf("TotalCommands() = %d, want 2", cdl.TotalCommands())
	}
}

func TestChunkedDisplayListFindChunkByLayoutID(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	idx, ok := cdl.FindChunkByLayoutID(renderer.LayoutID(2))
	if !ok || idx != 1 {
		t.Errorf("FindChunkByLayoutID(2) = (%d, %v), want (1, true)", idx, ok)
	}

	_, ok = cdl.FindChunkByLayoutID(renderer.LayoutID(99))
	if ok {
		t.Error("FindChunkByLayoutID(99) should return false")
	}

	_, ok = cdl.FindChunkByLayoutID(renderer.LayoutNone)
	if ok {
		t.Error("FindChunkByLayoutID(LayoutNone) should return false")
	}
}

// --- Chunk reuse tests ---

func TestChunkedDisplayListReuseUnchanged(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 40, Y: 0, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2), renderer.LayoutID(3)}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	// Only invalidate LayoutID(2)
	cdl.InvalidateByLayoutID(renderer.LayoutID(2))

	// Chunks 0 and 2 should be reusable (clean)
	reused := 0
	for i := 0; i < cdl.ChunkCount(); i++ {
		ch := cdl.Chunks().At(i)
		if !ch.Dirty() {
			reused++
		}
	}
	if reused != 2 {
		t.Errorf("expected 2 reusable chunks, got %d", reused)
	}
}

// --- Edge cases ---

func TestBuildPaintChunksNonContiguousSameOwner(t *testing.T) {
	// Same owner appearing non-contiguously creates separate chunks
	dl := renderer.NewDisplayCommandList()
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 20, Y: 0, W: 10, H: 10}))
	dl.Add(makeRectCmd(renderer.RectF{X: 0, Y: 20, W: 10, H: 10}))

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(2), renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	// Should produce 3 chunks: owner1, owner2, owner1
	if chunks.Len() != 3 {
		t.Fatalf("expected 3 chunks for non-contiguous owner, got %d", chunks.Len())
	}
	if chunks.At(0).Owner != renderer.LayoutID(1) || chunks.At(2).Owner != renderer.LayoutID(1) {
		t.Error("non-contiguous same owner should produce separate chunks")
	}
}

func TestBuildPaintChunksTextCommandBounds(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{
		Kind: renderer.CmdText,
		Text: renderer.TextCommand{Bounds: renderer.RectF{X: 5, Y: 10, W: 100, H: 20}},
	})

	owners := []renderer.LayoutID{renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	c := chunks.At(0)
	if c.Bounds.X != 5 || c.Bounds.Y != 10 || c.Bounds.W != 100 || c.Bounds.H != 20 {
		t.Errorf("text command bounds = %+v, want {5,10,100,20}", c.Bounds)
	}
}

func TestBuildPaintChunksImageCommandBounds(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{
		Kind:  renderer.CmdImage,
		Image: renderer.ImageCommand{Bounds: renderer.RectF{X: 0, Y: 0, W: 200, H: 150}},
	})

	owners := []renderer.LayoutID{renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	c := chunks.At(0)
	if c.Bounds.W != 200 || c.Bounds.H != 150 {
		t.Errorf("image command bounds = %+v, want W=200 H=150", c.Bounds)
	}
}

func TestBuildPaintChunksPushPopCommandsNoBounds(t *testing.T) {
	dl := renderer.NewDisplayCommandList()
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdPushClip, Clip: renderer.ClipCommand{Bounds: renderer.RectF{X: 0, Y: 0, W: 100, H: 100}}})
	dl.Add(makeRectCmd(renderer.RectF{X: 10, Y: 10, W: 50, H: 50}))
	dl.Add(renderer.DisplayCommand{Kind: renderer.CmdPopClip})

	owners := []renderer.LayoutID{renderer.LayoutID(1), renderer.LayoutID(1), renderer.LayoutID(1)}
	chunks := renderer.BuildPaintChunks(dl, owners)

	c := chunks.At(0)
	// PushClip bounds should be included in union
	if c.CommandCount() != 3 {
		t.Errorf("expected 3 commands in chunk, got %d", c.CommandCount())
	}
}

// --- helper ---

func makeRectCmd(r renderer.RectF) renderer.DisplayCommand {
	return renderer.DisplayCommand{
		Kind: renderer.CmdRect,
		Rect: renderer.RectCommand{Bounds: r, Color: color.RGBA{R: 255, A: 255}},
	}
}

// --- Benchmarks ---

func BenchmarkBuildPaintChunks(b *testing.B) {
	sizes := []int{10, 100, 1000}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("cmds=%d", n), func(b *testing.B) {
			dl := renderer.NewDisplayCommandList()
			owners := make([]renderer.LayoutID, n)
			for i := 0; i < n; i++ {
				dl.Add(makeRectCmd(renderer.RectF{X: float32(i), Y: 0, W: 10, H: 10}))
				owners[i] = renderer.LayoutID(i%10 + 1) // 10 distinct owners
			}
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				renderer.BuildPaintChunks(dl, owners)
			}
		})
	}
}

func BenchmarkBuildPaintChunksSingleOwner(b *testing.B) {
	n := 1000
	dl := renderer.NewDisplayCommandList()
	owners := make([]renderer.LayoutID, n)
	for i := 0; i < n; i++ {
		dl.Add(makeRectCmd(renderer.RectF{X: float32(i), Y: 0, W: 10, H: 10}))
		owners[i] = renderer.LayoutID(1)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		renderer.BuildPaintChunks(dl, owners)
	}
}

func BenchmarkChunkedDisplayListInvalidate(b *testing.B) {
	n := 1000
	dl := renderer.NewDisplayCommandList()
	owners := make([]renderer.LayoutID, n)
	for i := 0; i < n; i++ {
		dl.Add(makeRectCmd(renderer.RectF{X: float32(i), Y: 0, W: 10, H: 10}))
		owners[i] = renderer.LayoutID(i + 1)
	}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cdl.InvalidateByLayoutID(renderer.LayoutID(500))
		cdl.MarkAllClean()
	}
}

func BenchmarkChunkedDisplayListDirtyRects(b *testing.B) {
	n := 1000
	dl := renderer.NewDisplayCommandList()
	owners := make([]renderer.LayoutID, n)
	for i := 0; i < n; i++ {
		dl.Add(makeRectCmd(renderer.RectF{X: float32(i), Y: 0, W: 10, H: 10}))
		owners[i] = renderer.LayoutID(i + 1)
	}
	chunks := renderer.BuildPaintChunks(dl, owners)
	cdl := renderer.NewChunkedDisplayList(dl, chunks)

	// Mark 10% dirty
	for i := 0; i < n; i += 10 {
		cdl.InvalidateByLayoutID(renderer.LayoutID(i + 1))
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cdl.DirtyRects()
	}
}

func BenchmarkSourceMappingBuild(b *testing.B) {
	n := 1000
	dl := renderer.NewDisplayCommandList()
	owners := make([]renderer.LayoutID, n)
	for i := 0; i < n; i++ {
		dl.Add(makeRectCmd(renderer.RectF{X: float32(i), Y: 0, W: 10, H: 10}))
		owners[i] = renderer.LayoutID(i + 1)
	}
	chunks := renderer.BuildPaintChunks(dl, owners)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		renderer.BuildSourceMapping(chunks)
	}
}

func BenchmarkSourceMappingLookup(b *testing.B) {
	sm := renderer.NewSourceMapping()
	for i := 1; i <= 1000; i++ {
		sm.Add(renderer.LayoutID(i), i-1, i)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sm.Lookup(renderer.LayoutID(500))
	}
}

func BenchmarkPaintChunkContains(b *testing.B) {
	chunk := renderer.PaintChunk{Bounds: renderer.RectF{X: 0, Y: 0, W: 100, H: 100}}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		chunk.Contains(50, 50)
	}
}

func BenchmarkPaintChunkIntersects(b *testing.B) {
	chunk := renderer.PaintChunk{Bounds: renderer.RectF{X: 0, Y: 0, W: 100, H: 100}}
	other := renderer.RectF{X: 50, Y: 50, W: 100, H: 100}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		chunk.Intersects(other)
	}
}

package renderer

// paint_chunk.go — M5.2 Paint Chunks
//
// Paint chunks group DisplayCommand values by stable layout ownership.
// This enables:
//   - Reuse of unchanged chunks across frames
//   - Rebuilding only paint-dirty chunks
//   - Source-to-display mappings for developer tools
//
// Design:
//   - PaintChunk is a value type with LayoutID owner, command range, bounds, dirty flag
//   - PaintChunkList stores chunks in contiguous []PaintChunk slice
//   - SourceMapping maps LayoutID → command range for dev tools
//   - ChunkedDisplayList combines DisplayCommandList + PaintChunkList with reuse support

// PaintChunk groups a contiguous range of DisplayCommand values owned by one layout object.
//
// The Owner field is the LayoutID that produced these commands. Start and End
// are indices into the DisplayCommandList. Bounds is the union of all command
// bounds in this chunk. The dirty flag tracks whether the chunk needs rebuilding.
type PaintChunk struct {
	Owner  LayoutID
	Start  int   // inclusive index into DisplayCommandList
	End    int   // exclusive index into DisplayCommandList
	Bounds RectF // union of all command bounds in this chunk
	dirty  bool  // true if chunk needs rebuilding
}

// Dirty reports whether this chunk needs to be rebuilt.
func (c PaintChunk) Dirty() bool {
	return c.dirty
}

// MarkDirty sets the dirty flag.
func (c *PaintChunk) MarkDirty() {
	c.dirty = true
}

// MarkClean clears the dirty flag.
func (c *PaintChunk) MarkClean() {
	c.dirty = false
}

// CommandCount returns the number of commands in this chunk.
func (c PaintChunk) CommandCount() int {
	return c.End - c.Start
}

// Contains reports whether point (x, y) lies within the chunk bounds.
func (c PaintChunk) Contains(x, y float32) bool {
	return c.Bounds.Contains(x, y)
}

// Intersects reports whether the chunk bounds overlap with the given rectangle.
func (c PaintChunk) Intersects(r RectF) bool {
	return c.Bounds.Intersects(r)
}

// PaintChunkList is a contiguous slice of PaintChunk values.
// Chunks are stored in display order matching the command order in DisplayCommandList.
type PaintChunkList struct {
	chunks []PaintChunk
}

// NewPaintChunkList creates an empty paint chunk list.
func NewPaintChunkList() *PaintChunkList {
	return &PaintChunkList{chunks: make([]PaintChunk, 0)}
}

// Add appends a chunk to the list.
func (cl *PaintChunkList) Add(chunk PaintChunk) {
	cl.chunks = append(cl.chunks, chunk)
}

// Clear removes all chunks.
func (cl *PaintChunkList) Clear() {
	cl.chunks = cl.chunks[:0]
}

// Len returns the number of chunks.
func (cl *PaintChunkList) Len() int {
	return len(cl.chunks)
}

// At returns the chunk at index i. Panics if out of range.
func (cl *PaintChunkList) At(i int) PaintChunk {
	return cl.chunks[i]
}

// Chunks returns the underlying slice for iteration.
func (cl *PaintChunkList) Chunks() []PaintChunk {
	return cl.chunks
}

// MarkDirty marks the chunk at index i as dirty. No-op if out of range.
func (cl *PaintChunkList) MarkDirty(i int) {
	if i >= 0 && i < len(cl.chunks) {
		cl.chunks[i].dirty = true
	}
}

// MarkAllClean clears the dirty flag on all chunks.
func (cl *PaintChunkList) MarkAllClean() {
	for i := range cl.chunks {
		cl.chunks[i].dirty = false
	}
}

// SourceMapping maps LayoutID to command ranges for developer tools.
// This allows inspecting which display commands belong to which layout object.
type SourceMapping struct {
	entries []sourceEntry
}

type sourceEntry struct {
	Layout   LayoutID
	CmdStart int
	CmdEnd   int
}

// NewSourceMapping creates an empty source mapping.
func NewSourceMapping() *SourceMapping {
	return &SourceMapping{entries: make([]sourceEntry, 0)}
}

// Add records a command range for a layout ID. Overwrites any previous entry.
func (sm *SourceMapping) Add(layout LayoutID, start, end int) {
	if layout == LayoutNone {
		return
	}
	// Overwrite existing entry
	for i := range sm.entries {
		if sm.entries[i].Layout == layout {
			sm.entries[i].CmdStart = start
			sm.entries[i].CmdEnd = end
			return
		}
	}
	sm.entries = append(sm.entries, sourceEntry{Layout: layout, CmdStart: start, CmdEnd: end})
}

// Lookup returns the command range for a layout ID.
// Returns (0, 0, false) if not found or if layout is LayoutNone.
func (sm *SourceMapping) Lookup(layout LayoutID) (start, end int, ok bool) {
	if layout == LayoutNone {
		return 0, 0, false
	}
	for i := range sm.entries {
		if sm.entries[i].Layout == layout {
			return sm.entries[i].CmdStart, sm.entries[i].CmdEnd, true
		}
	}
	return 0, 0, false
}

// Clear removes all entries.
func (sm *SourceMapping) Clear() {
	sm.entries = sm.entries[:0]
}

// Len returns the number of entries.
func (sm *SourceMapping) Len() int {
	return len(sm.entries)
}

// Entries returns a copy of all entries for iteration.
func (sm *SourceMapping) Entries() []sourceEntry {
	out := make([]sourceEntry, len(sm.entries))
	copy(out, sm.entries)
	return out
}

// BuildPaintChunks groups display commands by layout ownership.
//
// The owners slice must have one LayoutID per command. If owners is nil or
// shorter than the command list, missing entries default to LayoutNone.
// Consecutive commands with the same owner are grouped into one chunk.
// Non-contiguous same-owner commands produce separate chunks (preserving
// display order for correct painting).
func BuildPaintChunks(dl *DisplayCommandList, owners []LayoutID) *PaintChunkList {
	chunks := NewPaintChunkList()
	if dl == nil || dl.Len() == 0 {
		return chunks
	}

	cmds := dl.Commands()
	n := len(cmds)

	getOwner := func(i int) LayoutID {
		if owners != nil && i < len(owners) {
			return owners[i]
		}
		return LayoutNone
	}

	// Start first chunk
	chunkStart := 0
	chunkOwner := getOwner(0)
	chunkBounds := commandBounds(cmds[0])

	for i := 1; i < n; i++ {
		owner := getOwner(i)
		if owner == chunkOwner {
			// Extend current chunk
			unionBounds(&chunkBounds, commandBounds(cmds[i]))
		} else {
			// Flush current chunk, start new one
			chunks.Add(PaintChunk{
				Owner:  chunkOwner,
				Start:  chunkStart,
				End:    i,
				Bounds: chunkBounds,
			})
			chunkStart = i
			chunkOwner = owner
			chunkBounds = commandBounds(cmds[i])
		}
	}

	// Flush last chunk
	chunks.Add(PaintChunk{
		Owner:  chunkOwner,
		Start:  chunkStart,
		End:    n,
		Bounds: chunkBounds,
	})

	return chunks
}

// BuildSourceMapping creates a SourceMapping from a PaintChunkList.
func BuildSourceMapping(chunks *PaintChunkList) *SourceMapping {
	sm := NewSourceMapping()
	if chunks == nil {
		return sm
	}
	for i := 0; i < chunks.Len(); i++ {
		c := chunks.At(i)
		if c.Owner != LayoutNone {
			sm.Add(c.Owner, c.Start, c.End)
		}
	}
	return sm
}

// ChunkedDisplayList combines a DisplayCommandList with PaintChunkList.
// It supports invalidation by LayoutID and chunk reuse.
type ChunkedDisplayList struct {
	commands *DisplayCommandList
	chunks   *PaintChunkList
}

// NewChunkedDisplayList creates a chunked display list.
func NewChunkedDisplayList(cmds *DisplayCommandList, chunks *PaintChunkList) *ChunkedDisplayList {
	if cmds == nil {
		cmds = NewDisplayCommandList()
	}
	if chunks == nil {
		chunks = NewPaintChunkList()
	}
	return &ChunkedDisplayList{commands: cmds, chunks: chunks}
}

// Commands returns the underlying DisplayCommandList.
func (cdl *ChunkedDisplayList) Commands() *DisplayCommandList {
	return cdl.commands
}

// Chunks returns the underlying PaintChunkList.
func (cdl *ChunkedDisplayList) Chunks() *PaintChunkList {
	return cdl.chunks
}

// TotalCommands returns the total number of display commands.
func (cdl *ChunkedDisplayList) TotalCommands() int {
	return cdl.commands.Len()
}

// ChunkCount returns the number of paint chunks.
func (cdl *ChunkedDisplayList) ChunkCount() int {
	return cdl.chunks.Len()
}

// CommandsForChunk returns the display commands for chunk at index i.
// Returns nil if i is out of range.
func (cdl *ChunkedDisplayList) CommandsForChunk(i int) []DisplayCommand {
	if i < 0 || i >= cdl.chunks.Len() {
		return nil
	}
	c := cdl.chunks.At(i)
	return cdl.commands.Commands()[c.Start:c.End]
}

// InvalidateByLayoutID marks all chunks owned by layout as dirty.
// No-op if layout is LayoutNone or not found.
func (cdl *ChunkedDisplayList) InvalidateByLayoutID(layout LayoutID) {
	if layout == LayoutNone {
		return
	}
	for i := 0; i < cdl.chunks.Len(); i++ {
		if cdl.chunks.chunks[i].Owner == layout {
			cdl.chunks.chunks[i].dirty = true
		}
	}
}

// MarkAllClean clears the dirty flag on all chunks.
func (cdl *ChunkedDisplayList) MarkAllClean() {
	cdl.chunks.MarkAllClean()
}

// DirtyChunkCount returns the number of dirty chunks.
func (cdl *ChunkedDisplayList) DirtyChunkCount() int {
	count := 0
	for i := 0; i < cdl.chunks.Len(); i++ {
		if cdl.chunks.chunks[i].dirty {
			count++
		}
	}
	return count
}

// DirtyRects returns the bounds of all dirty chunks.
func (cdl *ChunkedDisplayList) DirtyRects() []RectF {
	var rects []RectF
	for i := 0; i < cdl.chunks.Len(); i++ {
		c := cdl.chunks.chunks[i]
		if c.dirty {
			rects = append(rects, c.Bounds)
		}
	}
	return rects
}

// FindChunkByLayoutID returns the chunk index for a layout ID.
// Returns (0, false) if not found or if layout is LayoutNone.
func (cdl *ChunkedDisplayList) FindChunkByLayoutID(layout LayoutID) (int, bool) {
	if layout == LayoutNone {
		return 0, false
	}
	for i := 0; i < cdl.chunks.Len(); i++ {
		if cdl.chunks.chunks[i].Owner == layout {
			return i, true
		}
	}
	return 0, false
}

// --- helpers ---

// commandBounds extracts the bounds from a DisplayCommand.
func commandBounds(cmd DisplayCommand) RectF {
	switch cmd.Kind {
	case CmdRect:
		return cmd.Rect.Bounds
	case CmdBorder:
		return cmd.Border.Bounds
	case CmdText:
		return cmd.Text.Bounds
	case CmdImage:
		return cmd.Image.Bounds
	case CmdPushClip:
		return cmd.Clip.Bounds
	case CmdPopClip:
		return RectF{} // no bounds
	case CmdPushTransform:
		return RectF{} // transform has no intrinsic bounds
	case CmdPopTransform:
		return RectF{}
	case CmdPushOpacity:
		return RectF{}
	case CmdPopOpacity:
		return RectF{}
	case CmdPushStackingContext:
		return RectF{}
	case CmdPopStackingContext:
		return RectF{}
	default:
		return RectF{}
	}
}

// unionBounds expands dst to include src.
func unionBounds(dst *RectF, src RectF) {
	if src.W == 0 && src.H == 0 {
		return // skip zero-size bounds
	}
	if dst.W == 0 && dst.H == 0 {
		*dst = src
		return
	}
	// Compute union
	x0 := dst.X
	if src.X < x0 {
		x0 = src.X
	}
	y0 := dst.Y
	if src.Y < y0 {
		y0 = src.Y
	}
	x1 := dst.X + dst.W
	if src.X+src.W > x1 {
		x1 = src.X + src.W
	}
	y1 := dst.Y + dst.H
	if src.Y+src.H > y1 {
		y1 = src.Y + src.H
	}
	dst.X = x0
	dst.Y = y0
	dst.W = x1 - x0
	dst.H = y1 - y0
}

package renderer

import (
	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

type MutationInvalidation struct {
	NodeID int64
	Flags  DirtyFlag
}

func (r *Renderer) ApplyMutationBatch(batch []MutationInvalidation) int {
	if len(batch) == 0 {
		return 0
	}
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	if r.currentRenderTree == nil {
		return 0
	}
	if r.incremental == nil {
		width, height := r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight
		r.incremental = NewIncrementalLayoutEngine(width, height)
	}
	applied := 0
	hasLayout := false
	for _, mutation := range batch {
		if mutation.NodeID == 0 || mutation.Flags == DirtyNone {
			continue
		}
		node := findRenderNodeByIDRoot(r.currentRenderTree, mutation.NodeID)
		if node == nil {
			continue
		}
		r.incremental.InvalidateNode(node, mutation.Flags)
		if mutation.Flags&DirtyStyle != 0 {
			// Recompute computed styles for the mutated subtree before any
			// relayout so the rebuilt boxes read fresh styling (PR10).
			// Without this, class/style= mutations keep the styling from the
			// original full render until the next reparse.
			r.recomputeSubtreeStyles(node)
		}
		applied++
		if mutation.Flags&DirtyLayout != 0 || mutation.Flags&DirtySubtree != 0 {
			hasLayout = true
		}
	}
	if applied > 0 {
		r.dirty = true
		if hasLayout {
			r.currentLayoutTree = r.incremental.RecomputeDirtyFromPrevious(r.currentLayoutTree, r.currentRenderTree)
		}
		// Mutation values were applied in place, so the canvas renderer's
		// pointer-identity display-list cache would repaint stale content on
		// the next UpdateViewport/present. Drop the cache so the next
		// present rebuilds commands from the updated trees.
		r.canvasRenderer.mu.Lock()
		r.canvasRenderer.cachedDisplayList = nil
		r.canvasRenderer.cachedRenderRoot = nil
		r.canvasRenderer.cachedLayoutRoot = nil
		r.canvasRenderer.mu.Unlock()
	}
	return applied
}

// recomputeSubtreeStyles resets and re-applies computed styles for a
// mutated node's subtree so class/style attribute mutations produce fresh
// styling on the typed path without a full reparse (PR10). The caller must
// hold treeMu. When no stylesheet is loaded (a renderer that never went
// through a Build* pass), styles are left as-is to mirror the build-time
// behavior of not styling an un-built tree.
func (r *Renderer) recomputeSubtreeStyles(node *RenderNode) {
	if node == nil {
		return
	}
	r.stylesheetMu.RLock()
	sheet := r.stylesheet
	r.stylesheetMu.RUnlock()
	if sheet == nil {
		return
	}
	resetSubtreeStyles(node)
	sm := NewStyleManagerWithViewport(sheet, r.layoutEngine.canvasWidth, r.layoutEngine.canvasHeight)
	sm.ApplyStyles(node)
}

// resetSubtreeStyles clears the computed style and raw declaration map of a
// node and all its descendants so a style re-apply starts from a clean
// slate. ApplyStyles merges into existing computed styles, so without a
// reset a removed class or style= declaration would keep its stale styling.
func resetSubtreeStyles(node *RenderNode) {
	if node == nil {
		return
	}
	node.ComputedStyle = nil
	node.Styles = nil
	for _, child := range node.Children {
		resetSubtreeStyles(child)
	}
}

// PresentFromMutationBatch triggers a dirty paint using the renderer's
// chunked display list and forwards the raster output to the supplied
// Fyne adapter when one is configured. Returns true when a paint
// happened. The function is the typed-mutation entry point called by
// the JS sink after ApplyMutationBatch + InvalidatePaintChunks.
func (r *Renderer) PresentFromMutationBatch(adapter *FyneAdapter) bool {
	if r == nil || r.chunkedDisplay == nil || r.layoutEngine == nil {
		return false
	}
	width := int(r.layoutEngine.canvasWidth)
	height := int(r.layoutEngine.canvasHeight)
	if width <= 0 || height <= 0 {
		return false
	}
	commands := convertDisplayCommands(r.chunkedDisplay.commands.Commands())
	painter, err := NewIncrementalPainter(width, height)
	if err != nil || painter == nil {
		return false
	}
	defer painter.Close()
	chunks := r.chunkedDisplay.chunks.Chunks()
	img, err := painter.PaintDirty(chunks, commands)
	if err != nil || img == nil {
		return false
	}
	r.chunkedDisplay.MarkAllClean()
	painter.Present(adapter)
	return true
}

// convertDisplayCommands converts the renderer's DisplayCommand slice into
// the backend-neutral raster.DisplayCmd slice consumed by IncrementalPainter.
func convertDisplayCommands(in []DisplayCommand) []raster.DisplayCmd {
	out := make([]raster.DisplayCmd, 0, len(in))
	for _, cmd := range in {
		out = append(out, toRasterDisplayCmd(cmd))
	}
	return out
}

// toRasterDisplayCmd converts a single DisplayCommand into the raster
// backend command shape. The conversion is intentionally minimal: only the
// rect/clip/opacity/text/image fields that the dirty paint path uses today
// are propagated. Other fields are no-ops for the current test surface.
func toRasterDisplayCmd(cmd DisplayCommand) raster.DisplayCmd {
	rect := frame.Rect{X: cmd.Rect.Bounds.X, Y: cmd.Rect.Bounds.Y, W: cmd.Rect.Bounds.W, H: cmd.Rect.Bounds.H}
	if rect == (frame.Rect{}) {
		rect = frame.Rect{X: cmd.Border.Bounds.X, Y: cmd.Border.Bounds.Y, W: cmd.Border.Bounds.W, H: cmd.Border.Bounds.H}
	}
	return raster.DisplayCmd{Rect: rect, Color: frame.White}
}

type DirtyFlag uint8

const (
	// DirtyNone means no recomputation needed
	DirtyNone DirtyFlag = 0
	// DirtyLayout means layout needs to be recomputed
	DirtyLayout DirtyFlag = 1 << 0
	// DirtyPaint means paint needs to be recomputed
	DirtyPaint DirtyFlag = 1 << 1
	// DirtyStyle means style needs to be recomputed
	DirtyStyle DirtyFlag = 1 << 2
	// DirtySubtree means the entire subtree is dirty
	DirtySubtree DirtyFlag = 1 << 3
)

func (r *Renderer) InvalidatePaintChunks(nodeIDs []int64) int {
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	if r.chunkedDisplay == nil {
		return 0
	}
	if r.incremental == nil {
		return 0
	}
	invalidated := 0
	for _, id := range nodeIDs {
		if id == 0 {
			continue
		}
		if r.chunkedDisplay.InvalidateByLayoutIDCount(LayoutID(uint32(id))) > 0 {
			invalidated++
		}
	}
	return invalidated
}

// InvalidationTracker tracks which nodes need recomputation
type InvalidationTracker struct {
	dirtyNodes map[int64]DirtyFlag // nodeID -> dirty flags
}

// NewInvalidationTracker creates a new invalidation tracker
func NewInvalidationTracker() *InvalidationTracker {
	return &InvalidationTracker{
		dirtyNodes: make(map[int64]DirtyFlag),
	}
}

// MarkDirty marks a node as dirty with the specified flags
func (it *InvalidationTracker) MarkDirty(nodeID int64, flags DirtyFlag) {
	currentFlags := it.dirtyNodes[nodeID]
	it.dirtyNodes[nodeID] = currentFlags | flags
}

// IsDirty checks if a node has any dirty flags
func (it *InvalidationTracker) IsDirty(nodeID int64) bool {
	return it.dirtyNodes[nodeID] != DirtyNone
}

// GetDirtyFlags returns the dirty flags for a node
func (it *InvalidationTracker) GetDirtyFlags(nodeID int64) DirtyFlag {
	return it.dirtyNodes[nodeID]
}

// ClearDirty removes dirty flags for a node
func (it *InvalidationTracker) ClearDirty(nodeID int64) {
	delete(it.dirtyNodes, nodeID)
}

// GetDirtyNodes returns all node IDs that are dirty
func (it *InvalidationTracker) GetDirtyNodes() []int64 {
	nodes := make([]int64, 0, len(it.dirtyNodes))
	for nodeID := range it.dirtyNodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// ClearAll removes all dirty flags
func (it *InvalidationTracker) ClearAll() {
	it.dirtyNodes = make(map[int64]DirtyFlag)
}

// PropagateInvalidation propagates invalidation up the tree
// When a node is marked dirty, its ancestors may also need updating
func (it *InvalidationTracker) PropagateInvalidation(node *RenderNode, flags DirtyFlag) {
	if node == nil {
		return
	}

	// Mark this node dirty
	it.MarkDirty(node.ID, flags)

	// If layout is dirty, parent's layout may need updating too
	if flags&DirtyLayout != 0 {
		if node.Parent != nil {
			it.PropagateInvalidation(node.Parent, DirtyLayout)
		}
	}

	// If subtree is dirty, mark all children recursively
	if flags&DirtySubtree != 0 {
		it.markSubtreeDirty(node)
	}
}

// markSubtreeDirty recursively marks all nodes in a subtree as dirty
func (it *InvalidationTracker) markSubtreeDirty(node *RenderNode) {
	if node == nil {
		return
	}

	it.MarkDirty(node.ID, DirtyLayout|DirtyPaint)

	for _, child := range node.Children {
		it.markSubtreeDirty(child)
	}
}

// IncrementalLayoutEngine extends LayoutEngine with incremental layout support
type IncrementalLayoutEngine struct {
	*LayoutEngine
	invalidation *InvalidationTracker
}

// NewIncrementalLayoutEngine creates a layout engine with invalidation tracking
func NewIncrementalLayoutEngine(width, height float32) *IncrementalLayoutEngine {
	return &IncrementalLayoutEngine{
		LayoutEngine: NewLayoutEngine(width, height),
		invalidation: NewInvalidationTracker(),
	}
}

// InvalidateNode marks a node as needing relayout
func (ile *IncrementalLayoutEngine) InvalidateNode(node *RenderNode, flags DirtyFlag) {
	ile.invalidation.PropagateInvalidation(node, flags)

	// Walk up parent pointers to find any table ancestor and invalidate its cached column widths
	curr := node
	for curr != nil {
		if curr.TagName == "table" {
			globalTableColumnCache.Invalidate(curr.ID)
		}
		curr = curr.Parent
	}
}

// ComputeIncrementalLayout performs incremental layout, only recomputing dirty subtrees
func (ile *IncrementalLayoutEngine) RecomputeDirty(root *RenderNode, previousLayout *LayoutBox) *LayoutBox {
	if root == nil {
		return nil
	}
	if previousLayout == nil {
		return ile.LayoutEngine.ComputeLayout(root)
	}
	dirtyNodes := ile.invalidation.GetDirtyNodes()
	if len(dirtyNodes) == 0 {
		return previousLayout
	}
	return ile.recomputeSubtrees(root, previousLayout, dirtyNodes)
}

func findRenderNodeByIDRoot(node *RenderNode, id int64) *RenderNode {
	if node == nil || id == 0 {
		return nil
	}
	if node.ID == id {
		return node
	}
	for _, child := range node.Children {
		if result := findRenderNodeByIDRoot(child, id); result != nil {
			return result
		}
	}
	return nil
}

func (ile *IncrementalLayoutEngine) recomputeSubtrees(root *RenderNode, previous *LayoutBox, dirtyNodes []int64) *LayoutBox {
	visited := make(map[int64]bool, len(dirtyNodes))
	for _, id := range dirtyNodes {
		if visited[id] {
			continue
		}
		node := findRenderNodeByIDRoot(root, id)
		if node == nil {
			continue
		}
		rootID := ile.findReflowRoot(node)
		if rootID == 0 {
			continue
		}
		rootNode := findRenderNodeByIDRoot(root, rootID)
		if rootNode == nil {
			continue
		}
		ile.rebuildSubtree(rootNode)
		visited[rootID] = true
	}
	ile.invalidation.ClearAll()
	return previous
}

func (ile *IncrementalLayoutEngine) findReflowRoot(node *RenderNode) int64 {
	current := node
	for current != nil {
		if ile.invalidation.IsDirty(current.ID) {
			return current.ID
		}
		current = current.Parent
	}
	return node.ID
}

func (ile *IncrementalLayoutEngine) rebuildSubtree(node *RenderNode) {
	if node == nil {
		return
	}
	le := ile.LayoutEngine
	le.nodeMapMu.Lock()
	if le.nodeMap == nil {
		le.nodeMap = make(map[int64]*LayoutBox)
	}
	le.nodeMapMu.Unlock()
	_ = le.buildLayoutBox(node, 0, 0, le.canvasWidth, nil,
		NewInlineLayoutEngine(le.fontMetrics, le.defaultFontSize),
		NewFlexLayoutEngine(le.fontMetrics),
		NewGridLayoutEngine(le.fontMetrics),
	)
}

func (ile *IncrementalLayoutEngine) RecomputeDirtyFromPrevious(previous *LayoutBox, root *RenderNode) *LayoutBox {
	if previous == nil || root == nil {
		return ile.RecomputeDirty(root, previous)
	}
	dirtyNodes := ile.invalidation.GetDirtyNodes()
	if len(dirtyNodes) == 0 {
		return previous
	}
	for _, id := range dirtyNodes {
		node := findRenderNodeByIDRoot(root, id)
		if node == nil {
			continue
		}
		ile.rebuildSubtree(node)
	}
	ile.invalidation.ClearAll()
	return previous
}
func (ile *IncrementalLayoutEngine) ComputeIncrementalLayout(root *RenderNode, previousLayout *LayoutBox) *LayoutBox {
	if root == nil {
		return nil
	}

	// If no nodes are dirty, return the previous layout
	dirtyNodes := ile.invalidation.GetDirtyNodes()
	if len(dirtyNodes) == 0 && previousLayout != nil {
		return previousLayout
	}

	// For now, do a full recompute if any node is dirty
	// A more sophisticated implementation would only recompute dirty subtrees
	layoutRoot := ile.LayoutEngine.ComputeLayout(root)

	// Clear dirty flags after layout
	ile.invalidation.ClearAll()

	return layoutRoot
}

// IsNodeDirty checks if a node needs recomputation
func (ile *IncrementalLayoutEngine) IsNodeDirty(nodeID int64) bool {
	return ile.invalidation.IsDirty(nodeID)
}

// GetInvalidationTracker returns the invalidation tracker (for testing)
func (ile *IncrementalLayoutEngine) GetInvalidationTracker() *InvalidationTracker {
	return ile.invalidation
}

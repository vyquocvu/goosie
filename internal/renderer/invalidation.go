package renderer

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
		applied++
		if mutation.Flags&DirtyLayout != 0 || mutation.Flags&DirtySubtree != 0 {
			hasLayout = true
		}
	}
	if applied > 0 {
		r.dirty = true
		if hasLayout {
			r.currentLayoutTree = r.incremental.RecomputeDirty(r.currentRenderTree, r.currentLayoutTree)
		}
	}
	return applied
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
	le.nodeMap = make(map[int64]*LayoutBox)
	le.nodeMapMu.Unlock()
	_ = le.buildLayoutBox(node, 0, 0, le.canvasWidth, nil,
		NewInlineLayoutEngine(le.fontMetrics, le.defaultFontSize),
		NewFlexLayoutEngine(le.fontMetrics),
		NewGridLayoutEngine(le.fontMetrics),
	)
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

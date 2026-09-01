package renderer

import (
	"strconv"
	"sync"
)

// NodeIDLookup converts between the JS polyfill's __goosie_id strings and the
// renderer's stable RenderNode IDs.
type NodeIDLookup struct {
	mu         sync.RWMutex
	byGoosieID map[string]int64
	byRenderID map[int64]string
}

func NewNodeIDLookup() *NodeIDLookup {
	return &NodeIDLookup{
		byGoosieID: make(map[string]int64),
		byRenderID: make(map[int64]string),
	}
}

func (l *NodeIDLookup) Bind(goosieID string, renderID int64) {
	if l == nil || goosieID == "" || renderID == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byGoosieID[goosieID] = renderID
	l.byRenderID[renderID] = goosieID
}

func (l *NodeIDLookup) Lookup(goosieID string) int64 {
	if l == nil || goosieID == "" {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.byGoosieID[goosieID]
}

func (l *NodeIDLookup) Reverse(renderID int64) string {
	if l == nil || renderID == 0 {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.byRenderID[renderID]
}

func (l *NodeIDLookup) ForgetRenderID(renderID int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if goosieID, ok := l.byRenderID[renderID]; ok {
		delete(l.byGoosieID, goosieID)
		delete(l.byRenderID, renderID)
	}
}

func (l *NodeIDLookup) Snapshot(root *RenderNode) {
	if l == nil || root == nil {
		return
	}
	byG := make(map[string]int64)
	byR := make(map[int64]string)
	var walk func(*RenderNode)
	walk = func(node *RenderNode) {
		if node == nil {
			return
		}
		if goosieID, ok := node.Attrs["__goosie_id"]; ok && goosieID != "" {
			if _, err := strconv.ParseInt(goosieID, 10, 64); err == nil {
				byG[goosieID] = node.ID
				byR[node.ID] = goosieID
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)

	l.mu.Lock()
	l.byGoosieID = byG
	l.byRenderID = byR
	l.mu.Unlock()
}

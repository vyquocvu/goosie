// Package dom provides a compatibility adapter for the compact DOM store.
//
// M2.5: Compatibility Adapter
//
// This file provides a temporary adapter that converts the compact NodeID-based
// DOM store back to *html.Node trees for unmigrated consumers (renderer, JS runtime,
// cmd/browser). This is migration-only infrastructure and must be removed before
// Milestone 5 exit.
//
// Usage metrics are tracked via atomic counters to detect remaining use during
// the migration period.
package dom

import (
	"golang.org/x/net/html"
	"sync/atomic"
)

// adapterUsageCount tracks the number of ToHTMLNode calls for migration metrics.
var adapterUsageCount atomic.Int64

// NodeAdapter converts compact Store subtrees to *html.Node trees.
// Deprecated: Migration-only adapter. Remove before Milestone 5 exit.
type NodeAdapter struct {
	store *Store
}

// NewNodeAdapter creates a new adapter for the given store.
// Deprecated: Migration-only adapter. Remove before Milestone 5 exit.
func NewNodeAdapter(store *Store) *NodeAdapter {
	return &NodeAdapter{store: store}
}

// ToHTMLNode converts the subtree rooted at id to a *html.Node tree.
// Returns nil if id is invalid or stale.
// Deprecated: Migration-only adapter. Remove before Milestone 5 exit.
func (a *NodeAdapter) ToHTMLNode(id NodeID) *html.Node {
	if id == NodeNone || int(id) >= len(a.store.nodes) {
		return nil
	}

	rec := &a.store.nodes[id]
	if rec.Kind == 0 {
		// Stale handle.
		return nil
	}

	// Track usage for migration metrics (once per top-level call).
	adapterUsageCount.Add(1)

	return a.convertNode(id)
}

// convertNode recursively converts a store node to *html.Node.
// Unlike ToHTMLNode, this does not increment the usage counter.
func (a *NodeAdapter) convertNode(id NodeID) *html.Node {
	rec := &a.store.nodes[id]

	// Convert based on node kind.
	var n *html.Node
	switch rec.Kind {
	case NodeKindElement:
		n = &html.Node{
			Type: html.ElementNode,
			Data: rec.Name.String(),
		}
		// Convert attributes.
		if rec.AttrCount > 0 {
			start := rec.AttrStart
			end := start + uint32(rec.AttrCount)
			if end <= uint32(len(a.store.attrs)) {
				n.Attr = make([]html.Attribute, rec.AttrCount)
				for i, attr := range a.store.attrs[start:end] {
					n.Attr[i] = html.Attribute{
						Key: attr.Name.String(),
						Val: attr.Value.String(),
					}
				}
			}
		}
	case NodeKindText:
		text, _ := a.store.Text(id)
		n = &html.Node{
			Type: html.TextNode,
			Data: text,
		}
	case NodeKindComment:
		text, _ := a.store.Text(id)
		n = &html.Node{
			Type: html.CommentNode,
			Data: text,
		}
	case NodeKindDocument:
		n = &html.Node{
			Type: html.DocumentNode,
		}
	case NodeKindDoctype:
		n = &html.Node{
			Type: html.DoctypeNode,
		}
	default:
		return nil
	}

	// Recursively convert children.
	for child := rec.FirstChild; child != NodeNone; child = a.store.nodes[child].NextSibling {
		childNode := a.convertNode(child)
		if childNode != nil {
			n.AppendChild(childNode)
		}
	}

	return n
}

// AdapterUsageCount returns the number of ToHTMLNode calls since program start.
// Deprecated: Migration-only metric. Remove before Milestone 5 exit.
func AdapterUsageCount() int64 {
	return adapterUsageCount.Load()
}

// ResetAdapterMetrics resets the usage counter for testing.
// Deprecated: Migration-only metric. Remove before Milestone 5 exit.
func ResetAdapterMetrics() {
	adapterUsageCount.Store(0)
}

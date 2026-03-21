package renderer

import (
	"github.com/vyquocvu/goosie/internal/css"
)

// StyleResolver walks the DOM and matches parsed CSS rules to RenderNode objects.
type StyleResolver struct {
	stylesheet *css.StyleSheet
}

// NewStyleResolver creates a new StyleResolver with the given stylesheet.
func NewStyleResolver(stylesheet *css.StyleSheet) *StyleResolver {
	return &StyleResolver{
		stylesheet: stylesheet,
	}
}

// Resolve walks the DOM tree and matches CSS rules to each node,
// storing the computed values in the node's Styles map.
func (sr *StyleResolver) Resolve(node *RenderNode) {
	sm := NewStyleManager(sr.stylesheet)
	sr.resolveRecursive(node, sm)
}

func (sr *StyleResolver) resolveRecursive(node *RenderNode, sm *StyleManager) {
	if node == nil {
		return
	}

	if node.Styles == nil {
		node.Styles = make(map[string]string)
	}

	if sr.stylesheet != nil {
		sm.applyMatchingRules(sr.stylesheet, node)
	}

	// Recursively resolve for children
	for _, child := range node.Children {
		sr.resolveRecursive(child, sm)
	}
}

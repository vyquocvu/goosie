package renderer

import (
"fmt"
"testing"

"golang.org/x/net/html"
"strings"
)

func TestLayoutDebug003(t *testing.T) {
htmlContent := `<!DOCTYPE html><html><body><ul><li>Unordered 1</li><li>Unordered 2</li></ul><ol><li>Ordered 1</li><li>Ordered 2</li></ol><dl><dt>Term</dt><dd>Definition</dd></dl></body></html>`

doc, _ := html.Parse(strings.NewReader(htmlContent))
bodyNode := findBodyNode(doc)
renderTree := BuildRenderTree(bodyNode)

fmt.Printf("RenderTree root: %s (block=%v, children=%d)\n", renderTree.TagName, renderTree.IsBlock(), len(renderTree.Children))
for i, child := range renderTree.Children {
fmt.Printf("  Child %d: %s (block=%v, children=%d)\n", i, child.TagName, child.IsBlock(), len(child.Children))
}

le := NewLayoutEngine(1280, 800)
hasInlineBody := le.hasInlineContent(renderTree)
fmt.Printf("hasInlineContent(body) = %v\n", hasInlineBody)

layoutTree := le.ComputeLayout(renderTree)
fmt.Printf("Body Box: x=%.1f, y=%.1f, w=%.1f, h=%.1f\n", layoutTree.Box.X, layoutTree.Box.Y, layoutTree.Box.Width, layoutTree.Box.Height)
for i, child := range layoutTree.Children {
fmt.Printf("  Child %d (nodeID=%d) Box: x=%.1f, y=%.1f, w=%.1f, h=%.1f\n", i, child.NodeID, child.Box.X, child.Box.Y, child.Box.Width, child.Box.Height)
for j, gc := range child.Children {
fmt.Printf("    Grandchild %d (nodeID=%d) Box: x=%.1f, y=%.1f, w=%.1f, h=%.1f\n", j, gc.NodeID, gc.Box.X, gc.Box.Y, gc.Box.Width, gc.Box.Height)
}
}
}

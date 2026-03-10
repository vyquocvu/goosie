package renderer

import (
"fmt"
"testing"
"strings"

"golang.org/x/net/html"
)

func TestComprehensiveDebug(t *testing.T) {
tests := []struct{
name string
html string
}{
{"test_003", `<!DOCTYPE html><html><body><ul><li>Unordered 1</li><li>Unordered 2</li></ul><ol><li>Ordered 1</li><li>Ordered 2</li></ol><dl><dt>Term</dt><dd>Definition</dd></dl></body></html>`},
{"test_005", `<!DOCTYPE html><html><body><blockquote><p>This is a blockquote.</p></blockquote></body></html>`},
{"test_009", `<!DOCTYPE html><html><body><p style="line-height:1.0">Single line height<br>Second line</p><p style="line-height:2.0">Double line height<br>Second line</p></body></html>`},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
doc, _ := html.Parse(strings.NewReader(tt.html))
bodyNode := findBodyNode(doc)
renderTree := BuildRenderTree(bodyNode)

le := NewLayoutEngine(1280, 800)
layoutTree := le.ComputeLayout(renderTree)

fmt.Printf("%s: body.Box.Height=%.1f, children=%d\n", tt.name, layoutTree.Box.Height, len(layoutTree.Children))
for i, child := range layoutTree.Children {
nodeID := child.NodeID
_ = nodeID
fmt.Printf("  Child %d: Box(y=%.1f, h=%.1f), linebox_count=%d\n", i, child.Box.Y, child.Box.Height, len(child.LineBoxes))
for j, lb := range child.LineBoxes {
fmt.Printf("    LineBox %d: Y=%.1f, H=%.1f, boxes=%d\n", j, lb.Y, lb.Height, len(lb.InlineBoxes))
}
for j, gc := range child.Children {
fmt.Printf("    GrandChild %d: Box(y=%.1f, h=%.1f), linebox_count=%d\n", j, gc.Box.Y, gc.Box.Height, len(gc.LineBoxes))
}
}
})
}
}

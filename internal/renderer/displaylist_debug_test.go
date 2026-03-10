package renderer

import (
"fmt"
"testing"

"golang.org/x/net/html"
"strings"
)

func TestDisplayListDebug002(t *testing.T) {
htmlContent := `<!DOCTYPE html><html><body><p><b>Bold</b>, <strong>Strong</strong>, <i>Italic</i>, <em>Emphasized</em>, <u>Underline</u>, <strike>Strike</strike>, <s>Strikethrough</s>, <tt>Teletype</tt>, <code>Code</code>, <small>Small</small>, <sub>Sub</sub>, <sup>Sup</sup>.</p></body></html>`

doc, _ := html.Parse(strings.NewReader(htmlContent))
bodyNode := findBodyNode(doc)
renderTree := BuildRenderTree(bodyNode)

le := NewLayoutEngine(1280, 800)
layoutTree := le.ComputeLayout(renderTree)

fmt.Printf("Body Box: h=%.1f\n", layoutTree.Box.Height)

dlb := NewDisplayListBuilder()
dl := dlb.Build(layoutTree, renderTree)

fmt.Printf("DisplayList has %d commands:\n", len(dl.Commands))
for i, cmd := range dl.Commands {
switch cmd.Type {
case PaintText:
fmt.Printf("  [%d] PaintText: text=%q, box=(%.1f,%.1f,%.1f,%.1f)\n", i, cmd.Text, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
case PaintRect:
fmt.Printf("  [%d] PaintRect: box=(%.1f,%.1f,%.1f,%.1f)\n", i, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
default:
fmt.Printf("  [%d] Type=%d: box=(%.1f,%.1f,%.1f,%.1f)\n", i, cmd.Type, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
}
}
}

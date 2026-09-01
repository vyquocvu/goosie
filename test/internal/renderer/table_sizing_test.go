package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestTableColumnSpacerWidths(t *testing.T) {
	// Table with 3 columns:
	// Col 1: spacer width="20"
	// Col 2: content (auto)
	// Col 3: spacer width="20"
	content := `<!DOCTYPE html><html><body>
		<table width="800" cellspacing="0" cellpadding="0">
			<tr>
				<td width="20">Spacer</td>
				<td>Main content article text here</td>
				<td width="20">Spacer</td>
			</tr>
		</table>
	</body></html>`

	r := renderer.NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	renderRoot := renderer.BuildRenderTree(findBodyNode(doc))
	layoutRoot := r.LayoutEngine().ComputeLayout(renderRoot)

	// Find the table layout box
	var tableBox *renderer.LayoutBox
	var findTable func(b *renderer.LayoutBox)
	findTable = func(b *renderer.LayoutBox) {
		if b == nil {
			return
		}
		if b.Display == renderer.DisplayGrid && len(b.Children) == 3 {
			tableBox = b
			return
		}
		for _, c := range b.Children {
			findTable(c)
		}
	}
	findTable(layoutRoot)

	if tableBox == nil {
		t.Fatal("table box not found")
	}

	cell1 := tableBox.Children[0]
	cell2 := tableBox.Children[1]
	cell3 := tableBox.Children[2]

	// Col 1 should be 20px
	if cell1.Box.Width != 20.0 {
		t.Errorf("Expected cell 1 width 20.0, got %f", cell1.Box.Width)
	}
	// Col 3 should be 20px
	if cell3.Box.Width != 20.0 {
		t.Errorf("Expected cell 3 width 20.0, got %f", cell3.Box.Width)
	}
	// Col 2 should get the remaining width (800 - 40 = 760)
	if cell2.Box.Width != 760.0 {
		t.Errorf("Expected cell 2 width 760.0, got %f", cell2.Box.Width)
	}
}

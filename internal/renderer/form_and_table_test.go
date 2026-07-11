package renderer

import (
	"context"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/net/html"
)

func TestFormElementRendering(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<input placeholder="Enter text" />
				<button>Click me</button>
				<textarea placeholder="Enter more text"></textarea>
			</body>
		</html>
	`
	r := NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}
	renderTree := BuildRenderTree(findBodyNode(doc))
	obj := r.canvasRenderer.Render(renderTree)

	// The top-level object is a container, let's inspect its children
	topContainer, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected a container, but got %T", obj)
	}

	if len(topContainer.Objects) != 3 {
		t.Fatalf("Expected 3 objects, got %d", len(topContainer.Objects))
	}

	if _, ok := topContainer.Objects[0].(*widget.Entry); !ok {
		t.Errorf("Expected first object to be an Entry, but it was not")
	}
	if _, ok := topContainer.Objects[1].(*widget.Button); !ok {
		t.Errorf("Expected second object to be a Button, but it was not")
	}
	if _, ok := topContainer.Objects[2].(*widget.Entry); !ok {
		t.Errorf("Expected third object to be a MultiLineEntry, but it was not")
	}
}

func TestTableElementRendering(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<table>
					<tr>
						<td>Cell 1</td>
						<td>Cell 2</td>
					</tr>
					<tr>
						<td>Cell 3</td>
						<td>Cell 4</td>
					</tr>
				</table>
			</body>
		</html>
	`
	r := NewRenderer(800, 600)
	obj, err := r.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	// Result should be a container
	containerObj, ok := obj.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected a container, but got %T", obj)
	}

	// Inspect children
	// We expect paint commands for:
	// - Cell 1
	// - Cell 2
	// - Cell 3
	// - Cell 4
	// Plus potentially backgrounds/borders if any (none in this simple html)

	// Print children types for debugging
	t.Logf("Container has %d children", len(containerObj.Objects))
	foundCells := 0
	expectedTexts := []string{"Cell 1", "Cell 2", "Cell 3", "Cell 4"}

	for i, child := range containerObj.Objects {
		if textObj, ok := child.(*canvas.Text); ok {
			t.Logf("Child %d is canvas.Text: %s", i, textObj.Text)
			// Check if it matches one of our expected cells
			for _, expected := range expectedTexts {
				if textObj.Text == expected {
					foundCells++
				}
			}
		} else {
			t.Logf("Child %d is %T", i, child)
		}
	}

	if foundCells < 4 {
		t.Errorf("Expected at least 4 cell labels, found %d matching labels", foundCells)
	}
}

func TestTableColspanRowspan(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<table>
			<tr class="r1">
				<td class="cellA" colspan="2" rowspan="2">A</td>
				<td class="cellB" rowspan="3">B</td>
			</tr>
			<tr class="r2">
				<td class="cellC">C</td>
			</tr>
			<tr class="r3">
				<td class="cellD">D</td>
				<td class="cellE" colspan="2">E</td>
			</tr>
		</table>
	</body>
	</html>`

	_, layoutRoot, renderTree := runLayout(t, htmlStr, "")

	// Find the cell nodes in renderTree and verify their Grid coordinates on LayoutBox
	cellA := findNodeByClass(renderTree, "cellA")
	if cellA == nil {
		t.Fatal("cellA not found")
	}
	boxA := findLayoutBoxInTree(layoutRoot, cellA.ID)
	if boxA == nil {
		t.Fatal("LayoutBox for cellA not found")
	}

	if boxA.GridColumnStart != 1 || boxA.GridColumnEnd != 3 {
		t.Errorf("cellA: expected col 1-3, got %d-%d", boxA.GridColumnStart, boxA.GridColumnEnd)
	}
	if boxA.GridRowStart != 1 || boxA.GridRowEnd != 3 {
		t.Errorf("cellA: expected row 1-3, got %d-%d", boxA.GridRowStart, boxA.GridRowEnd)
	}

	cellB := findNodeByClass(renderTree, "cellB")
	if cellB == nil {
		t.Fatal("cellB not found")
	}
	boxB := findLayoutBoxInTree(layoutRoot, cellB.ID)
	if boxB == nil {
		t.Fatal("LayoutBox for cellB not found")
	}
	if boxB.GridColumnStart != 3 || boxB.GridColumnEnd != 4 {
		t.Errorf("cellB: expected col 3-4, got %d-%d", boxB.GridColumnStart, boxB.GridColumnEnd)
	}
	if boxB.GridRowStart != 1 || boxB.GridRowEnd != 4 {
		t.Errorf("cellB: expected row 1-4, got %d-%d", boxB.GridRowStart, boxB.GridRowEnd)
	}

	cellC := findNodeByClass(renderTree, "cellC")
	if cellC == nil {
		t.Fatal("cellC not found")
	}
	boxC := findLayoutBoxInTree(layoutRoot, cellC.ID)
	if boxC == nil {
		t.Fatal("LayoutBox for cellC not found")
	}
	// Columns 1 and 2 occupied by cellA. Column 3 occupied by cellB.
	// cellC must be pushed to Column 4!
	if boxC.GridColumnStart != 4 || boxC.GridColumnEnd != 5 {
		t.Errorf("cellC: expected col 4-5, got %d-%d", boxC.GridColumnStart, boxC.GridColumnEnd)
	}
	if boxC.GridRowStart != 2 || boxC.GridRowEnd != 3 {
		t.Errorf("cellC: expected row 2-3, got %d-%d", boxC.GridRowStart, boxC.GridRowEnd)
	}

	cellD := findNodeByClass(renderTree, "cellD")
	if cellD == nil {
		t.Fatal("cellD not found")
	}
	boxD := findLayoutBoxInTree(layoutRoot, cellD.ID)
	if boxD == nil {
		t.Fatal("LayoutBox for cellD not found")
	}
	// Row 3: column 1 is free.
	if boxD.GridColumnStart != 1 || boxD.GridColumnEnd != 2 {
		t.Errorf("cellD: expected col 1-2, got %d-%d", boxD.GridColumnStart, boxD.GridColumnEnd)
	}
	if boxD.GridRowStart != 3 || boxD.GridRowEnd != 4 {
		t.Errorf("cellD: expected row 3-4, got %d-%d", boxD.GridRowStart, boxD.GridRowEnd)
	}

	cellE := findNodeByClass(renderTree, "cellE")
	if cellE == nil {
		t.Fatal("cellE not found")
	}
	boxE := findLayoutBoxInTree(layoutRoot, cellE.ID)
	if boxE == nil {
		t.Fatal("LayoutBox for cellE not found")
	}
	// Row 3: column 2 is free.
	if boxE.GridColumnStart != 2 || boxE.GridColumnEnd != 4 {
		t.Errorf("cellE: expected col 2-4, got %d-%d", boxE.GridColumnStart, boxE.GridColumnEnd)
	}
	if boxE.GridRowStart != 3 || boxE.GridRowEnd != 4 {
		t.Errorf("cellE: expected row 3-4, got %d-%d", boxE.GridRowStart, boxE.GridRowEnd)
	}
}

func TestTableSectionOrdering(t *testing.T) {
	// HTML with sections in out-of-order sequence (tfoot before tbody)
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<table>
			<thead>
				<tr class="h-row"><td class="header-cell">Header</td></tr>
			</thead>
			<tfoot>
				<tr class="f-row"><td class="footer-cell">Footer</td></tr>
			</tfoot>
			<tbody>
				<tr class="b-row"><td class="body-cell">Body</td></tr>
			</tbody>
		</table>
	</body>
	</html>`

	_, layoutRoot, renderTree := runLayout(t, htmlStr, "")

	headerCell := findNodeByClass(renderTree, "header-cell")
	bodyCell := findNodeByClass(renderTree, "body-cell")
	footerCell := findNodeByClass(renderTree, "footer-cell")

	if headerCell == nil || bodyCell == nil || footerCell == nil {
		t.Fatal("One of the cells was not found in the render tree")
	}

	boxHeader := findLayoutBoxInTree(layoutRoot, headerCell.ID)
	boxBody := findLayoutBoxInTree(layoutRoot, bodyCell.ID)
	boxFooter := findLayoutBoxInTree(layoutRoot, footerCell.ID)

	if boxHeader == nil || boxBody == nil || boxFooter == nil {
		t.Fatal("One of the layout boxes was not found")
	}

	// Visual order must be Header (row 1) -> Body (row 2) -> Footer (row 3)
	if boxHeader.GridRowStart != 1 {
		t.Errorf("Header row start: expected 1, got %d", boxHeader.GridRowStart)
	}
	if boxBody.GridRowStart != 2 {
		t.Errorf("Body row start: expected 2, got %d", boxBody.GridRowStart)
	}
	if boxFooter.GridRowStart != 3 {
		t.Errorf("Footer row start: expected 3, got %d", boxFooter.GridRowStart)
	}
}

func TestTableSpansLimits(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
	<html>
	<body>
		<table>
			<tr>
				<td class="clamped-cell" colspan="999" rowspan="999">Clamped</td>
			</tr>
		</table>
	</body>
	</html>`

	_, layoutRoot, renderTree := runLayout(t, htmlStr, "")

	cell := findNodeByClass(renderTree, "clamped-cell")
	if cell == nil {
		t.Fatal("clamped cell not found")
	}

	box := findLayoutBoxInTree(layoutRoot, cell.ID)
	if box == nil {
		t.Fatal("LayoutBox not found")
	}

	// Maximum span limits should be enforced (clamped to 100)
	expectedColEnd := box.GridColumnStart + 100
	expectedRowEnd := box.GridRowStart + 100

	if box.GridColumnEnd != expectedColEnd {
		t.Errorf("GridColumnEnd expected %d, got %d (not clamped)", expectedColEnd, box.GridColumnEnd)
	}
	if box.GridRowEnd != expectedRowEnd {
		t.Errorf("GridRowEnd expected %d, got %d (not clamped)", expectedRowEnd, box.GridRowEnd)
	}
}

// helper to find a LayoutBox by node ID in a tree
func findLayoutBoxInTree(root *LayoutBox, nodeID int64) *LayoutBox {
	if root == nil {
		return nil
	}
	if root.NodeID == nodeID {
		return root
	}
	for _, child := range root.Children {
		if found := findLayoutBoxInTree(child, nodeID); found != nil {
			return found
		}
	}
	return nil
}

package renderer

import (
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
	obj, err := r.RenderHTML(htmlContent)
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

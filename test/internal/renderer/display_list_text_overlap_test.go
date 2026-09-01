package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestDisplayListInlineTextOverlapPrevention(t *testing.T) {
	// A paragraph containing text split by an inline <span> or <a>
	// The prefix and suffix text belong to the same parent paragraph/text structure,
	// but must NOT be coalesced into a single command spanning across the inline element.
	content := `<!DOCTYPE html><html><body>
		<p id="target">Prefix text <a href="https://example.com" id="link">middle link</a> suffix text</p>
	</body></html>`

	r := renderer.NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parseHTML failed: %v", err)
	}
	body := findBodyNode(doc)
	renderRoot := renderer.BuildRenderTree(body)
	layoutRoot := r.LayoutEngine().ComputeLayout(renderRoot)

	dlb := renderer.NewDisplayListBuilder()
	displayList := dlb.Build(layoutRoot, renderRoot)

	var textCmds []string
	var linkCmds []string

	for _, cmd := range displayList.Commands {
		switch cmd.Type {
		case renderer.PaintText:
			textCmds = append(textCmds, cmd.Text)
		case renderer.PaintLink:
			linkCmds = append(linkCmds, cmd.LinkText)
		}
	}

	// Verify that prefix and suffix are separate text commands, and link is separate
	foundPrefix := false
	foundSuffix := false
	foundCombined := false

	for _, txt := range textCmds {
		if strings.Contains(txt, "Prefix text") {
			foundPrefix = true
		}
		if strings.Contains(txt, "suffix text") {
			foundSuffix = true
		}
		if strings.Contains(txt, "Prefix text") && strings.Contains(txt, "suffix text") {
			foundCombined = true
		}
	}

	if foundCombined {
		t.Fatalf("Prefix and suffix were merged across inline element: %v", textCmds)
	}
	if !foundPrefix {
		t.Errorf("Missing prefix text command in %v", textCmds)
	}
	if !foundSuffix {
		t.Errorf("Missing suffix text command in %v", textCmds)
	}
	if len(linkCmds) != 1 || linkCmds[0] != "middle link" {
		t.Errorf("Expected link command 'middle link', got %v", linkCmds)
	}
}

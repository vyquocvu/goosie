package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestDisplayListUsesInlineFragmentText_NoDuplicates(t *testing.T) {
	content := `<!DOCTYPE html><html><body>
		<h1>H1 Heading</h1>
		<p>Standard paragraph text.</p>
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

	type stats struct {
		original  string
		fullCount int
		total     int
	}

	perNode := map[int64]*stats{}

	var h1TextNodeID int64
	var h1Original string
	var pTextNodeID int64
	var pOriginal string

	var find func(n *renderer.RenderNode)
	find = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.Type == renderer.NodeTypeElement {
			if n.TagName == "h1" && len(n.Children) > 0 {
				for _, c := range n.Children {
					if c.Type == renderer.NodeTypeText {
						h1TextNodeID = c.ID
						h1Original = c.Text
					}
				}
			}
			if n.TagName == "p" && len(n.Children) > 0 {
				for _, c := range n.Children {
					if c.Type == renderer.NodeTypeText {
						pTextNodeID = c.ID
						pOriginal = c.Text
					}
				}
			}
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(renderRoot)

	perNode[h1TextNodeID] = &stats{original: h1Original}
	perNode[pTextNodeID] = &stats{original: pOriginal}

	for _, cmd := range displayList.Commands {
		if cmd.Type != renderer.PaintText {
			continue
		}
		s, ok := perNode[cmd.NodeID]
		if !ok {
			continue
		}
		s.total++
		if cmd.Text == s.original {
			s.fullCount++
		}
	}

	if s := perNode[h1TextNodeID]; s != nil {
		if s.total < 1 {
			t.Fatalf("h1 text commands missing")
		}
		if s.total > 1 && s.fullCount > 1 {
			t.Fatalf("duplicate full-text rendering detected for h1: total=%d fullCount=%d", s.total, s.fullCount)
		}
	}

	if s := perNode[pTextNodeID]; s != nil {
		if s.total < 1 {
			t.Fatalf("p text commands missing")
		}
		if s.total > 1 && s.fullCount > 1 {
			t.Fatalf("duplicate full-text rendering detected for p: total=%d fullCount=%d", s.total, s.fullCount)
		}
	}
}

func TestDisplayListLinkDoesNotPaintOverlappingText(t *testing.T) {
	content := `<!DOCTYPE html><html><body>
		<a href="https://example.com">Click me</a>
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

	linkCount := 0
	overlappingTextCount := 0
	for _, cmd := range displayList.Commands {
		switch cmd.Type {
		case renderer.PaintLink:
			if cmd.LinkURL == "https://example.com" && cmd.LinkText == "Click me" {
				linkCount++
			}
		case renderer.PaintText:
			if strings.TrimSpace(cmd.Text) == "Click me" {
				overlappingTextCount++
			}
		}
	}

	if linkCount != 1 {
		t.Fatalf("expected one clickable link command, got %d", linkCount)
	}
	if overlappingTextCount != 0 {
		t.Fatalf("expected link text to be painted only by PaintLink, got %d overlapping text commands", overlappingTextCount)
	}
}

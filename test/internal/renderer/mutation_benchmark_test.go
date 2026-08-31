package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/engine/testpages"
)

func BenchmarkMutationClassToggle(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}

	doc, err := html.Parse(strings.NewReader(page.HTML))
	if err != nil {
		b.Fatal(err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		b.Fatal("failed to build render tree")
	}

	sm := renderer.NewStyleManager(stylesheet)
	le := renderer.NewLayoutEngine(800, 600)

	target := findNodeByID(renderTree, "username")
	if target == nil {
		b.Fatal("target node 'username' not found")
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			target.SetAttribute("class", "mutated-class")
		} else {
			target.SetAttribute("class", "")
		}

		sm.ApplyStyles(renderTree)
		le.ComputeLayout(renderTree)
	}
}

func BenchmarkMutationAppendNode(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}

	doc, err := html.Parse(strings.NewReader(page.HTML))
	if err != nil {
		b.Fatal(err)
	}

	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		b.Fatal("failed to build render tree")
	}

	sm := renderer.NewStyleManager(nil)
	le := renderer.NewLayoutEngine(800, 600)

	target := findNodeByID(renderTree, "settings-form")
	if target == nil {
		b.Fatal("target node 'settings-form' not found")
	}

	newNode := renderer.NewRenderNode(renderer.NodeTypeElement)
	newNode.TagName = "p"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		target.AddChild(newNode)

		sm.ApplyStyles(renderTree)
		le.ComputeLayout(renderTree)

		// Revert addition to keep tree size stable
		target.Children = target.Children[:len(target.Children)-1]
		newNode.Parent = nil
	}
}

func BenchmarkMutationReplaceText(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}

	doc, err := html.Parse(strings.NewReader(page.HTML))
	if err != nil {
		b.Fatal(err)
	}

	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		b.Fatal("failed to build render tree")
	}

	sm := renderer.NewStyleManager(nil)
	le := renderer.NewLayoutEngine(800, 600)

	target := findNodeByID(renderTree, "username")
	if target == nil {
		b.Fatal("target node 'username' not found")
	}

	var textNode *renderer.RenderNode
	for _, child := range target.Children {
		if child.Type == renderer.NodeTypeText {
			textNode = child
			break
		}
	}
	if textNode == nil {
		textNode = renderer.NewRenderNode(renderer.NodeTypeText)
		target.AddChild(textNode)
	}

	shortText := "Short"
	longText := "This is a significantly longer paragraph of text that should wrap and change layout heights."

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			textNode.Text = longText
		} else {
			textNode.Text = shortText
		}

		sm.ApplyStyles(renderTree)
		le.ComputeLayout(renderTree)
	}
}

func BenchmarkMutationResizeViewport(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}

	doc, err := html.Parse(strings.NewReader(page.HTML))
	if err != nil {
		b.Fatal(err)
	}

	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	if renderTree == nil {
		b.Fatal("failed to build render tree")
	}

	sm := renderer.NewStyleManager(nil)
	sm.ApplyStyles(renderTree)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		width := float32(800)
		if i%2 == 0 {
			width = 1024
		}

		le := renderer.NewLayoutEngine(width, 600)
		le.ComputeLayout(renderTree)
	}
}

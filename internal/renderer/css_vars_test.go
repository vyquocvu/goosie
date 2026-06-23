package renderer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
)

func mustParseHTML(t *testing.T, htmlStr string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}
	return doc
}

func TestCSSCustomPropertyStorage(t *testing.T) {
	htmlStr := `<html><head><style>
		:root { --primary: #ff0000; --size: 16px; }
		div { color: var(--primary); font-size: var(--size); }
	</style></head><body><div id="target">hi</div></body></html>`

	doc := mustParseHTML(t, htmlStr)
	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))

	sm := NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)

	// Find the div
	var div *RenderNode
	for _, c := range renderTree.Children {
		if c.TagName == "div" {
			div = c
		}
	}
	assert.NotNil(t, div)
	// var() resolution is Task 2 — only verify the div was found and styled
	assert.NotNil(t, div.ComputedStyle)
	// CustomProperties should be inherited from :root
	assert.NotNil(t, div.ComputedStyle.CustomProperties, "CustomProperties should be inherited from :root")
	assert.Equal(t, "#ff0000", div.ComputedStyle.CustomProperties["--primary"])
}

func TestCSSCustomPropertyStorageOnly(t *testing.T) {
	htmlStr := `<html><head><style>
		:root { --primary: #ff0000; --size: 16px; }
	</style></head><body><div id="target">hi</div></body></html>`

	doc := mustParseHTML(t, htmlStr)
	stylesheet := extractAndParseCSS(doc)

	renderTree := BuildRenderTree(findBodyNode(doc))

	sm := NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)

	// Find the div
	var div *RenderNode
	for _, c := range renderTree.Children {
		if c.TagName == "div" {
			div = c
		}
	}
	assert.NotNil(t, div)
	// Custom properties should be inherited from :root -> html -> body -> div
	assert.NotNil(t, div.ComputedStyle.CustomProperties, "div should have inherited custom properties")
}

package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
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
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))

	sm := renderer.NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)

	// Find the div
	var div *renderer.RenderNode
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

	// Task 2: verify var() resolution actually applied the color
	// The div has color: var(--primary) which resolves to #ff0000 (red)
	r, g, b, _ := div.ComputedStyle.Color.RGBA()
	assert.Greater(t, r, uint32(0xAAAA), "color should be resolved to red from var(--primary)")
	assert.Less(t, g, uint32(0x1000), "green channel should be near 0")
	assert.Less(t, b, uint32(0x1000), "blue channel should be near 0")
}

func TestResolveVarTokens(t *testing.T) {
	style := &renderer.Style{
		CustomProperties: map[string]string{
			"--color": "red",
			"--size":  "16px",
			"--chain": "var(--color)",
		},
	}

	assert.Equal(t, "red", renderer.ResolveVarTokens("var(--color)", style))
	assert.Equal(t, "16px", renderer.ResolveVarTokens("var(--size)", style))
	assert.Equal(t, "1px solid red", renderer.ResolveVarTokens("1px solid var(--color)", style))
	assert.Equal(t, "blue", renderer.ResolveVarTokens("var(--missing, blue)", style))
	assert.Equal(t, "red", renderer.ResolveVarTokens("var(--chain)", style)) // chained var
	assert.Equal(t, "", renderer.ResolveVarTokens("var(--undefined)", style))

	// Malformed var() should not panic
	assert.NotPanics(t, func() {
		renderer.ResolveVarTokens("var(--color", style) // missing closing paren
	})
}

func TestCSSCustomPropertyStorageOnly(t *testing.T) {
	htmlStr := `<html><head><style>
		:root { --primary: #ff0000; --size: 16px; }
	</style></head><body><div id="target">hi</div></body></html>`

	doc := mustParseHTML(t, htmlStr)
	stylesheet := extractAndParseCSS(doc)

	renderTree := renderer.BuildRenderTree(findBodyNode(doc))

	sm := renderer.NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)

	// Find the div
	var div *renderer.RenderNode
	for _, c := range renderTree.Children {
		if c.TagName == "div" {
			div = c
		}
	}
	assert.NotNil(t, div)
	// Custom properties should be inherited from :root -> html -> body -> div
	assert.NotNil(t, div.ComputedStyle.CustomProperties, "div should have inherited custom properties")
}

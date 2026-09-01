package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// Helper to find a RenderNode by ID attribute
func findNodeByID(node *renderer.RenderNode, id string) *renderer.RenderNode {
	if node == nil {
		return nil
	}
	if val, ok := node.GetAttribute("id"); ok && val == id {
		return node
	}
	for _, child := range node.Children {
		if found := findNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

func TestMutationClassToggle(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					.foo {
						color: #000000;
						display: block;
					}
					.bar {
						color: #ff0000;
						display: flex;
					}
				</style>
			</head>
			<body>
				<div id="target" class="foo">Hello</div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	require.NotNil(t, renderTree)

	// Step 1: Initial Style and Layout
	sm := renderer.NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByID(renderTree, "target")
	require.NotNil(t, target)
	assert.Equal(t, "block", target.ComputedStyle.Display)

	le := renderer.NewLayoutEngine(800, 600)
	layoutTree := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree)

	// Locate layout box for target
	le.NodeMapMu().RLock()
	targetBox, ok := le.NodeMap()[target.ID]
	le.NodeMapMu().RUnlock()
	require.True(t, ok)
	assert.Equal(t, renderer.DisplayBlock, targetBox.Display)

	// Step 2: Mutation - Toggle Class
	target.SetAttribute("class", "bar")

	// Recalculate Style & Layout
	sm.ApplyStyles(renderTree)
	layoutTree2 := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree2)

	// Verify updated Style and Layout
	assert.Equal(t, "flex", target.ComputedStyle.Display)
	le.NodeMapMu().RLock()
	targetBox2, ok := le.NodeMap()[target.ID]
	le.NodeMapMu().RUnlock()
	require.True(t, ok)
	assert.Equal(t, renderer.DisplayFlex, targetBox2.Display)
}

func TestMutationAppendNode(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					#container {
						display: block;
						width: 200px;
					}
					p {
						display: block;
						height: 50px;
					}
				</style>
			</head>
			<body>
				<div id="container"></div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	require.NotNil(t, renderTree)

	sm := renderer.NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	le := renderer.NewLayoutEngine(800, 600)
	layoutTree := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree)

	container := findNodeByID(renderTree, "container")
	require.NotNil(t, container)
	assert.Empty(t, container.Children)

	// Step 2: Mutation - Append Node
	newParagraph := renderer.NewRenderNode(renderer.NodeTypeElement)
	newParagraph.TagName = "p"
	container.AddChild(newParagraph)

	// Recalculate Style & Layout
	sm.ApplyStyles(renderTree)
	layoutTree2 := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree2)

	// Verify container has a child and its layout is generated
	assert.Len(t, container.Children, 1)
	le.NodeMapMu().RLock()
	pBox, ok := le.NodeMap()[newParagraph.ID]
	le.NodeMapMu().RUnlock()
	require.True(t, ok)
	assert.Equal(t, renderer.DisplayBlock, pBox.Display)
	assert.Equal(t, float32(50.0), pBox.Box.Height)
}

func TestMutationReplaceText(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<p id="target">Short</p>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)

	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	require.NotNil(t, renderTree)

	sm := renderer.NewStyleManager(nil)
	sm.ApplyStyles(renderTree)

	le := renderer.NewLayoutEngine(800, 600)
	layoutTree := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree)

	target := findNodeByID(renderTree, "target")
	require.NotNil(t, target)
	require.Len(t, target.Children, 1)
	textNode := target.Children[0]
	assert.Equal(t, renderer.NodeTypeText, textNode.Type)
	assert.Equal(t, "Short", textNode.Text)

	// Step 2: Mutation - Replace Text
	textNode.Text = "This is a significantly longer paragraph of text that should wrap and change layout heights."

	// Recalculate Style & Layout
	sm.ApplyStyles(renderTree)
	layoutTree2 := le.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree2)

	// Verify text replaced
	assert.Equal(t, "This is a significantly longer paragraph of text that should wrap and change layout heights.", textNode.Text)
}

func TestMutationResizeViewport(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<style>
					body {
						margin: 0;
					}
					#target {
						display: block;
						width: 50%;
						height: 100px;
					}
				</style>
			</head>
			<body>
				<div id="target"></div>
			</body>
		</html>
	`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	require.NoError(t, err)

	stylesheet := extractAndParseCSS(doc)
	renderTree := renderer.BuildRenderTree(findBodyNode(doc))
	require.NotNil(t, renderTree)

	// Step 1: Layout at 800px width
	sm := renderer.NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	le1 := renderer.NewLayoutEngine(800, 600)
	layoutTree1 := le1.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree1)

	target := findNodeByID(renderTree, "target")
	require.NotNil(t, target)

	le1.NodeMapMu().RLock()
	targetBox1, ok := le1.NodeMap()[target.ID]
	le1.NodeMapMu().RUnlock()
	require.True(t, ok)
	assert.Equal(t, float32(400.0), targetBox1.Box.Width) // 50% of 800

	// Step 2: Mutation - Resize Viewport to 1000px width
	le2 := renderer.NewLayoutEngine(1000, 600)
	layoutTree2 := le2.ComputeLayout(renderTree)
	require.NotNil(t, layoutTree2)

	le2.NodeMapMu().RLock()
	targetBox2, ok := le2.NodeMap()[target.ID]
	le2.NodeMapMu().RUnlock()
	require.True(t, ok)
	assert.Equal(t, float32(500.0), targetBox2.Box.Width) // 50% of 1000
}

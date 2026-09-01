package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockInInlineStacksNestedDiv verifies that a block-level child following
// inline text starts on a new line (CSS block-in-inline anonymous blocks).
func TestBlockInInlineStacksNestedDiv(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	html := `<html><style>.parent{background-color:yellow;}.child{background-color:inherit;}</style><body><div class="parent">Parent<div class="child">Child</div></div></body></html>`
	require.NoError(t, r.BuildHTML(context.Background(), html))

	root := r.CurrentRenderTree()
	require.NotNil(t, root)
	var parent, child *renderer.RenderNode
	var find func(n *renderer.RenderNode)
	find = func(n *renderer.RenderNode) {
		if n.TagName == "div" {
			if parent == nil {
				parent = n
			} else if child == nil {
				child = n
			}
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(root)
	require.NotNil(t, parent, "parent div not found")
	require.NotNil(t, child, "child div not found")

	parentBox := renderer.FindLayoutBoxForNode(r.CurrentLayoutTree(), parent.ID)
	require.NotNil(t, parentBox, "parent div layout box not found")

	// The child div must have its own LayoutBox stacked below the first line.
	var childBox *renderer.LayoutBox
	var walk func(b *renderer.LayoutBox)
	walk = func(b *renderer.LayoutBox) {
		if b == nil {
			return
		}
		if b.NodeID == child.ID {
			childBox = b
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(parentBox)
	require.NotNil(t, childBox, "child div should have a block LayoutBox")

	// Parent's line boxes must contain "Parent" text above the child box.
	require.NotEmpty(t, parentBox.LineBoxes, "parent should have line boxes")
	firstLine := parentBox.LineBoxes[0]
	assert.Less(t, firstLine.Y+firstLine.Height, childBox.Box.Y+0.01,
		"first line should end above the child block box")

	// The child text must NOT appear on the first line.
	for _, ib := range firstLine.InlineBoxes {
		if ib.IsText {
			node := renderer.FindRenderNodeByIDRoot(parent, ib.NodeID)
			if node != nil {
				assert.NotContains(t, node.Text, "Child",
					"child text must not flow inline with parent text")
			}
		}
	}
}

func TestBlockInInlineMixedSequence(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	html := `<html><body><div>text1<p>block</p>text2<span>inline</span><div>block2</div>text3</div></body></html>`
	require.NoError(t, r.BuildHTML(context.Background(), html))

	var divNode *renderer.RenderNode
	var findDiv func(n *renderer.RenderNode)
	findDiv = func(n *renderer.RenderNode) {
		if n.TagName == "div" && divNode == nil && n != r.CurrentRenderTree() {
			divNode = n
		}
		for _, c := range n.Children {
			findDiv(c)
		}
	}
	findDiv(r.CurrentRenderTree())
	require.NotNil(t, divNode, "outer div not found")

	root := renderer.FindLayoutBoxForNode(r.CurrentLayoutTree(), divNode.ID)
	require.NotNil(t, root, "div layout box not found")

	// Three inline runs produce line boxes; two block children produce boxes.
	assert.GreaterOrEqual(t, len(root.LineBoxes), 3, "expected line boxes for three inline runs")
	assert.Len(t, root.Children, 2, "expected two block child boxes")

	// Blocks must stack below their preceding inline run.
	for i, b := range root.Children {
		if i == 0 {
			continue
		}
		prev := root.Children[i-1]
		assert.GreaterOrEqual(t, b.Box.Y, prev.Box.Y+prev.Box.Height-0.01,
			"block children must not overlap preceding blocks")
	}
}

func TestBlockInInlinePreservesInheritance(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	html := `<html><body><div style="color: red;">Parent<span style="color: blue;">inline</span><p style="color: green;">block</p></div></body></html>`
	require.NoError(t, r.BuildHTML(context.Background(), html))

	var p *renderer.RenderNode
	var find func(n *renderer.RenderNode)
	find = func(n *renderer.RenderNode) {
		if n.TagName == "p" && p == nil {
			p = n
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(r.CurrentRenderTree())
	require.NotNil(t, p)
	require.NotNil(t, p.ComputedStyle)

	green := color.RGBA{R: 0, G: 128, B: 0, A: 255}
	rr, gg, bb, aa := p.ComputedStyle.Color.RGBA()
	assert.Equal(t, green, color.RGBA{R: uint8(rr >> 8), G: uint8(gg >> 8), B: uint8(bb >> 8), A: uint8(aa >> 8)},
		"block child should keep its own computed color")
}

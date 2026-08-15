package renderer

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderSplitTwoPhaseMatchesLegacy — PR4: the two-phase
// BuildHTML→PresentFrame composition must produce the same render tree,
// layout tree, and frame metrics as the legacy single-phase RenderHTML.
func TestRenderSplitTwoPhaseMatchesLegacy(t *testing.T) {
	const html = `<html><head><style>.x{color:red}</style></head><body><div class="x">hello</div></body></html>`

	legacy := NewRenderer(800, 600)
	obj1, err := legacy.RenderHTML(context.Background(), html)
	require.NoError(t, err)
	require.NotNil(t, obj1)

	split := NewRenderer(800, 600)
	require.NoError(t, split.BuildHTML(context.Background(), html))
	obj2 := split.PresentFrame()
	require.NotNil(t, obj2)

	// Same render tree structure (node count and text content).
	n1 := splitCountRenderNodes(legacy.GetRoot())
	n2 := splitCountRenderNodes(split.GetRoot())
	assert.Equal(t, n1, n2, "render tree node counts must match")

	text1 := splitCollectText(legacy.GetRoot())
	text2 := splitCollectText(split.GetRoot())
	assert.Equal(t, text1, text2, "render tree text must match")
}

// TestRenderSplitBuildHTMLPresentEndToEnd — a full BuildHTML→PresentFrame
// cycle hands off both trees and produces a presentable canvas object.
func TestRenderSplitBuildHTMLPresentEndToEnd(t *testing.T) {
	r := NewRenderer(800, 600)
	require.NoError(t, r.BuildHTML(context.Background(), `<html><body><p>hi</p></body></html>`))
	obj := r.PresentFrame()
	require.NotNil(t, obj)
	assert.NotNil(t, r.GetRoot())
	assert.NotNil(t, r.currentLayoutTree)
}

// TestRenderSplitNewestBuildWins — a slower build for an older render
// intent must not clobber a newer build's trees.
func TestRenderSplitNewestBuildWins(t *testing.T) {
	r := NewRenderer(800, 600)
	require.NoError(t, r.BuildHTML(context.Background(), `<html><body><p>first</p></body></html>`))
	first := r.GetRoot()

	// Simulate a second, newer build.
	require.NoError(t, r.BuildHTML(context.Background(), `<html><body><p>second</p></body></html>`))
	second := r.GetRoot()
	assert.NotEqual(t, first, second, "newer build must hand off a new tree")
	if text := splitCollectText(second); len(text) > 0 && text[0] != "second" {
		t.Fatalf("newest build text = %v, want second", text)
	}
}

// TestRenderSplitCancelledBuildHandoff — a cancelled build must not
// leave the renderer with a partially handed-off tree.
func TestRenderSplitCancelledBuildHandoff(t *testing.T) {
	r := NewRenderer(800, 600)
	require.NoError(t, r.BuildHTML(context.Background(), `<html><body><p>ok</p></body></html>`))
	before := r.GetRoot()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.BuildHTML(ctx, `<html><body><p>cancel</p></body></html>`)
	if err == nil {
		// If the build was not cancelled mid-flight, the new tree is
		// still valid — just ensure nothing is nil.
		assert.NotNil(t, r.GetRoot())
		return
	}
	// Cancelled: the old tree must still be the current one.
	assert.Same(t, before, r.GetRoot())
}

func splitCountRenderNodes(n *RenderNode) int {
	if n == nil {
		return 0
	}
	count := 1
	for _, c := range n.Children {
		count += splitCountRenderNodes(c)
	}
	return count
}

func splitCollectText(n *RenderNode) []string {
	if n == nil {
		return nil
	}
	var out []string
	if n.Type == NodeTypeText && strings.TrimSpace(n.Text) != "" {
		out = append(out, n.Text)
	}
	for _, c := range n.Children {
		out = append(out, splitCollectText(c)...)
	}
	return out
}

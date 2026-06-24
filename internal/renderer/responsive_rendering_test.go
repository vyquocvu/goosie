package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderHTMLAppliesMediaQueriesForViewport(t *testing.T) {
	r := NewRenderer(1000, 700)
	r.SetTestingMode(true)

	_, err := r.RenderHTML(`
		<style>
			.desktop { display: none; }
			.mobile { display: block; }
			@media (min-width: 800px) {
				.desktop { display: block; }
				.mobile { display: none; }
			}
		</style>
		<body>
			<div class="desktop">desktop</div>
			<div class="mobile">mobile</div>
		</body>`)
	require.NoError(t, err)

	desktop := findRenderNodeByClass(r.currentRenderTree, "desktop")
	mobile := findRenderNodeByClass(r.currentRenderTree, "mobile")
	require.NotNil(t, desktop)
	require.NotNil(t, mobile)
	require.Equal(t, "block", desktop.ComputedStyle.Display)
	require.Equal(t, "none", mobile.ComputedStyle.Display)
}

func TestRenderHTMLAppliesNestedMediaQueriesOnlyWhenBothMatch(t *testing.T) {
	html := `
		<style>
			.narrow { display: none; }
			@media screen {
				@media (max-width: 600px) {
					.narrow { display: block; }
			}	}
		</style>
		<body><div class="narrow">narrow</div></body>`

	wide := NewRenderer(1000, 700)
	wide.SetTestingMode(true)
	_, err := wide.RenderHTML(html)
	require.NoError(t, err)
	require.Equal(t, "none", findRenderNodeByClass(wide.currentRenderTree, "narrow").ComputedStyle.Display)

	narrow := NewRenderer(500, 700)
	narrow.SetTestingMode(true)
	_, err = narrow.RenderHTML(html)
	require.NoError(t, err)
	require.Equal(t, "block", findRenderNodeByClass(narrow.currentRenderTree, "narrow").ComputedStyle.Display)
}

func findRenderNodeByClass(node *RenderNode, class string) *RenderNode {
	if node == nil {
		return nil
	}
	if value, ok := node.GetAttribute("class"); ok && value == class {
		return node
	}
	for _, child := range node.Children {
		if found := findRenderNodeByClass(child, class); found != nil {
			return found
		}
	}
	return nil
}

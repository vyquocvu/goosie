package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderHTMLAppliesMediaQueriesForViewport(t *testing.T) {
	r := renderer.NewRenderer(1000, 700)
	r.SetTestingMode(true)

	_, err := r.RenderHTML(context.Background(), `
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

	desktop := findRenderNodeByClass(r.CurrentRenderTree(), "desktop")
	mobile := findRenderNodeByClass(r.CurrentRenderTree(), "mobile")
	require.NotNil(t, desktop)
	require.NotNil(t, mobile)
	require.Equal(t, css.DisplayAtomBlock, desktop.ComputedStyle.Display)
	require.Equal(t, css.DisplayAtomNone, mobile.ComputedStyle.Display)
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

	wide := renderer.NewRenderer(1000, 700)
	wide.SetTestingMode(true)
	_, err := wide.RenderHTML(context.Background(), html)
	require.NoError(t, err)
	require.Equal(t, css.DisplayAtomNone, findRenderNodeByClass(wide.CurrentRenderTree(), "narrow").ComputedStyle.Display)

	narrow := renderer.NewRenderer(500, 700)
	narrow.SetTestingMode(true)
	_, err = narrow.RenderHTML(context.Background(), html)
	require.NoError(t, err)
	require.Equal(t, css.DisplayAtomBlock, findRenderNodeByClass(narrow.CurrentRenderTree(), "narrow").ComputedStyle.Display)
}

func findRenderNodeByClass(node *renderer.RenderNode, class string) *renderer.RenderNode {
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

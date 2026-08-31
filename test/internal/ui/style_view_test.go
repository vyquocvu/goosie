package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestComputedStyleView_NoStyle(t *testing.T) {
	view := ui.NewComputedStyleView()
	view.SetNode(&renderer.RenderNode{ID: 1, TagName: "div"})
	assert.Equal(t, 0, view.VisibleCount())
}

func TestComputedStyleView_ShowsProperties(t *testing.T) {
	view := ui.NewComputedStyleView()
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		ComputedStyle: &renderer.Style{
			Display:  "block",
			FontSize: 16,
			Color:    color.RGBA{R: 0, G: 0, B: 0, A: 255},
			Width:    "100px",
		},
	}
	view.SetNode(node)
	assert.Greater(t, view.VisibleCount(), 0)
}

func TestComputedStyleView_Filter(t *testing.T) {
	view := ui.NewComputedStyleView()
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		ComputedStyle: &renderer.Style{
			Display:  "block",
			FontSize: 16,
			Color:    color.RGBA{R: 0, G: 0, B: 0, A: 255},
			Width:    "100px",
		},
	}
	view.SetNode(node)

	allCount := view.VisibleCount()

	view.SetFilter("font")
	filteredCount := view.VisibleCount()
	assert.Less(t, filteredCount, allCount)
	assert.Greater(t, filteredCount, 0)
}

func TestComputedStyleView_FilterNoMatch(t *testing.T) {
	view := ui.NewComputedStyleView()
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		ComputedStyle: &renderer.Style{
			Display: "block",
		},
	}
	view.SetNode(node)
	view.SetFilter("zzznonexistent")
	assert.Equal(t, 0, view.VisibleCount())
}

func TestComputedStyleView_ClearsOnEmptyFilter(t *testing.T) {
	view := ui.NewComputedStyleView()
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		ComputedStyle: &renderer.Style{
			Display: "block",
			Width:   "100px",
			Height:  "200px",
		},
	}
	view.SetNode(node)
	allCount := view.VisibleCount()
	assert.Equal(t, 3, allCount)

	view.SetFilter("width")
	assert.Equal(t, 1, view.VisibleCount())

	view.SetFilter("")
	assert.Equal(t, allCount, view.VisibleCount())
}

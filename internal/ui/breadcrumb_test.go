package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestBreadcrumbBar_EmptyWhenNoNode(t *testing.T) {
	bar := NewBreadcrumbBar(nil)
	assert.Equal(t, 0, bar.Count())
}

func TestBreadcrumbBar_SingleNode(t *testing.T) {
	bar := NewBreadcrumbBar(nil)
	node := &renderer.RenderNode{ID: 1, TagName: "html"}

	bar.SetSelection(node, nil)

	assert.Equal(t, 1, bar.Count())
	assert.Equal(t, "html", bar.SegmentText(0))
}

func TestBreadcrumbBar_ShowsAncestorChain(t *testing.T) {
	bar := NewBreadcrumbBar(nil)

	html := &renderer.RenderNode{ID: 1, TagName: "html"}
	body := &renderer.RenderNode{ID: 2, TagName: "body", Parent: html}
	div := &renderer.RenderNode{ID: 3, TagName: "div", Parent: body}
	html.Children = append(html.Children, body)
	body.Children = append(body.Children, div)

	bar.SetSelection(div, nil)

	assert.Equal(t, 3, bar.Count())
	assert.Equal(t, "html", bar.SegmentText(0))
	assert.Equal(t, "body", bar.SegmentText(1))
	assert.Equal(t, "div", bar.SegmentText(2))
}

func TestBreadcrumbBar_ClearsOnNil(t *testing.T) {
	bar := NewBreadcrumbBar(nil)

	html := &renderer.RenderNode{ID: 1, TagName: "html"}
	body := &renderer.RenderNode{ID: 2, TagName: "body", Parent: html}
	html.Children = append(html.Children, body)

	bar.SetSelection(body, nil)
	assert.Equal(t, 2, bar.Count())

	bar.SetSelection(nil, nil)
	assert.Equal(t, 0, bar.Count())
}

func TestBreadcrumbBar_ShowsIdAndClass(t *testing.T) {
	bar := NewBreadcrumbBar(nil)

	div := &renderer.RenderNode{ID: 1, TagName: "div"}
	div.Attrs = map[string]string{"id": "main", "class": "container"}

	bar.SetSelection(div, nil)
	assert.Equal(t, 1, bar.Count())
	assert.Equal(t, "div#main.container", bar.SegmentText(0))
}

func TestBreadcrumbBar_ClickSelectsAncestor(t *testing.T) {
	var selectedID int64
	bar := NewBreadcrumbBar(func(node *renderer.RenderNode) {
		selectedID = node.ID
	})

	html := &renderer.RenderNode{ID: 1, TagName: "html"}
	body := &renderer.RenderNode{ID: 2, TagName: "body", Parent: html}
	div := &renderer.RenderNode{ID: 3, TagName: "div", Parent: body}
	html.Children = append(html.Children, body)
	body.Children = append(body.Children, div)

	bar.SetSelection(div, nil)
	bar.ClickSegment(1)

	assert.Equal(t, int64(2), selectedID)
}

func TestBreadcrumbBar_TruncatesLongChains(t *testing.T) {
	bar := NewBreadcrumbBar(nil)

	// Build a chain of 20 elements
	var prev *renderer.RenderNode
	var leaf *renderer.RenderNode
	for i := 0; i < 20; i++ {
		n := &renderer.RenderNode{ID: int64(i), TagName: "div", Parent: prev}
		if prev != nil {
			prev.Children = append(prev.Children, n)
		}
		prev = n
		leaf = n
	}

	bar.SetSelection(leaf, nil)
	assert.GreaterOrEqual(t, bar.Count(), 1)
}

func TestBreadcrumbBar_ContainerIsCanvasObject(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	bar := NewBreadcrumbBar(nil)
	w.SetContent(bar.CanvasObject())
}

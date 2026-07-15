package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type BreadcrumbBar struct {
	container *fyne.Container
	onSelect  func(node *renderer.RenderNode)
	ancestors []*renderer.RenderNode
}

func NewBreadcrumbBar(onSelect func(node *renderer.RenderNode)) *BreadcrumbBar {
	b := &BreadcrumbBar{
		container: container.NewHBox(),
		onSelect:  onSelect,
	}
	return b
}

func (b *BreadcrumbBar) SetSelection(node *renderer.RenderNode, _ *renderer.LayoutBox) {
	b.ancestors = b.buildAncestors(node)
	b.rebuild()
}

func (b *BreadcrumbBar) Count() int {
	return len(b.ancestors)
}

func (b *BreadcrumbBar) SegmentText(i int) string {
	if i < 0 || i >= len(b.ancestors) {
		return ""
	}
	return breadcrumbLabel(b.ancestors[i])
}

func (b *BreadcrumbBar) ClickSegment(i int) {
	if i < 0 || i >= len(b.ancestors) || b.onSelect == nil {
		return
	}
	b.onSelect(b.ancestors[i])
}

func (b *BreadcrumbBar) CanvasObject() fyne.CanvasObject {
	return b.container
}

func (b *BreadcrumbBar) buildAncestors(node *renderer.RenderNode) []*renderer.RenderNode {
	if node == nil {
		return nil
	}
	var chain []*renderer.RenderNode
	for n := node; n != nil; n = n.Parent {
		chain = append(chain, n)
	}
	// Reverse to get root-first order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func (b *BreadcrumbBar) rebuild() {
	b.container.Objects = nil
	for i, anode := range b.ancestors {
		if i > 0 {
			b.container.Add(widget.NewLabel(">"))
		}
		label := widget.NewHyperlink(breadcrumbLabel(anode), nil)
		idx := i
		label.OnTapped = func() {
			if b.onSelect != nil {
				b.onSelect(b.ancestors[idx])
			}
		}
		b.container.Add(label)
	}
	b.container.Refresh()
}

func breadcrumbLabel(node *renderer.RenderNode) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(node.TagName)
	if id, ok := node.Attrs["id"]; ok {
		b.WriteString("#" + id)
	}
	if cls, ok := node.Attrs["class"]; ok {
		parts := strings.Fields(cls)
		for _, p := range parts {
			b.WriteString("." + p)
		}
	}
	return b.String()
}

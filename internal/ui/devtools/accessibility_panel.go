package devtools

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type accessibilityPanel struct {
	fyne.Container
	label   *widget.Label
	content *fyne.Container
}

func newAccessibilityPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &accessibilityPanel{
		label:   widget.NewLabel("No accessibility data available."),
		content: container.NewVBox(),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil {
				p.RefreshFrom(ctx)
			}
		}
	})

	topBar := container.NewBorder(nil, nil, refreshBtn,
		widget.NewLabelWithStyle("Accessibility Tree", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	scrollContent := container.NewVScroll(p.content)
	outer := container.NewBorder(topBar, nil, nil, nil, scrollContent)
	p.Container = *outer
	return p
}

func (p *accessibilityPanel) RefreshFrom(ctx *TabContext) {
	if ctx == nil || ctx.Accessibility == nil {
		p.label.SetText("No accessibility data available.")
		return
	}
	tree := ctx.Accessibility.GetAccessibilityTree()
	if len(tree) == 0 {
		p.label.SetText("No accessibility nodes found.")
		return
	}
	formatted := formatA11yTree(tree, "")
	if formatted == "" {
		p.label.SetText("No accessibility data available.")
		return
	}
	p.label.SetText(formatted)
	p.label.TextStyle.Monospace = true
	p.label.Refresh()
}

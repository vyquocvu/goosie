package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type settingsPanel struct {
	fyne.Container
	label *widget.Label
}

func newSettingsPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &settingsPanel{
		label: widget.NewLabel("No settings available"),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil {
				p.refreshFrom(ctx)
			}
		}
	})

	topBar := container.NewBorder(nil, nil, refreshBtn,
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
	p.Container = *content
	return p
}

func (p *settingsPanel) RefreshFrom(ctx *TabContext) {
	if ctx != nil {
		p.refreshFrom(ctx)
	}
}

func (p *settingsPanel) refreshFrom(ctx *TabContext) {
	if ctx.Settings == nil {
		p.label.SetText("No settings provider available.")
		return
	}
	s := ctx.Settings
	var b strings.Builder
	b.WriteString("Browser Settings\n\n")
	b.WriteString(fmt.Sprintf("  Homepage:            %s\n", s.GetHomepage()))
	b.WriteString(fmt.Sprintf("  Default Search:     %s\n", s.GetDefaultSearchEngine()))
	b.WriteString(fmt.Sprintf("  JavaScript:         %s\n", map[bool]string{true: "Enabled", false: "Disabled"}[s.GetEnableJavaScript()]))
	b.WriteString(fmt.Sprintf("  Images:             %s\n", map[bool]string{true: "Enabled", false: "Disabled"}[s.GetEnableImages()]))
	b.WriteString("\nChanges are applied immediately and persisted\nto the profile on browser restart.\n")
	p.label.SetText(b.String())
	p.label.Refresh()
}

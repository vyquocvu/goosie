package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type securityPanel struct {
	fyne.Container
	label *widget.Label
}

func newSecurityPanel(activeTab func() *TabContext) fyne.CanvasObject {
	p := &securityPanel{
		label: widget.NewLabel("No security information"),
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
		widget.NewLabelWithStyle("Security", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
	p.Container = *content
	return p
}

func (p *securityPanel) RefreshFrom(ctx *TabContext) {
	if ctx != nil {
		p.refreshFrom(ctx)
	}
}

func (p *securityPanel) refreshFrom(ctx *TabContext) {
	var b strings.Builder
	b.WriteString("Current Page\n\n")

	if ctx.CurrentURL != "" {
		b.WriteString(fmt.Sprintf("  URL:     %s\n", ctx.CurrentURL))
		if strings.HasPrefix(ctx.CurrentURL, "https://") {
			b.WriteString("  Protocol: HTTPS (encrypted)\n")
		} else if strings.HasPrefix(ctx.CurrentURL, "http://") {
			b.WriteString("  Protocol: HTTP (unencrypted)\n")
		}
	} else {
		b.WriteString("  No page loaded.\n")
	}

	if ctx.SecuritySummary != "" {
		b.WriteString(fmt.Sprintf("  Summary: %s\n", ctx.SecuritySummary))
	}

	b.WriteString("\nCertificate\n\n")
	b.WriteString("  Certificate chain inspection is available\n")
	b.WriteString("  for HTTPS pages with TLS connections.\n")

	p.label.SetText(b.String())
	p.label.Refresh()
}

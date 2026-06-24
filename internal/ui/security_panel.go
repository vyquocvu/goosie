package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type SecurityPanel struct {
	container    *fyne.Container
	summaryLabel *widget.Label
}

func NewSecurityPanel() *SecurityPanel {
	panel := &SecurityPanel{summaryLabel: widget.NewLabel("No security information")}
	panel.container = container.NewBorder(widget.NewLabel("Security"), nil, nil, nil, panel.summaryLabel)
	return panel
}

func (p *SecurityPanel) SetSummary(summary goosienet.SecuritySummary) {
	state := "Not secure"
	if summary.Secure {
		state = "Secure"
	}
	p.summaryLabel.SetText(fmt.Sprintf("%s\n%s\nSubject: %s\nIssuer: %s\nError: %s", summary.URL, state, summary.Subject, summary.Issuer, summary.Error))
}
func (p *SecurityPanel) CanvasObject() fyne.CanvasObject { return p.container }

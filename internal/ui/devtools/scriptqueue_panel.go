package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type scriptQueuePanel struct {
	fyne.Container
	label *widget.Label
}

func newScriptQueuePanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	p := &scriptQueuePanel{
		label: widget.NewLabel("No JavaScript runtime available."),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil {
			p.label.SetText("No active tab.")
			return
		}
		rt := ctx.JSRuntime
		if rt == nil {
			p.label.SetText("No JavaScript runtime available.\n\nJavaScript task queue monitoring requires an active JS runtime on the current tab.")
			return
		}

		var b strings.Builder
		b.WriteString("JavaScript Task Queue\n\n")
		timers := rt.ActiveTimersCount()
		consoleCount := len(rt.GetConsoleMessages())
		errorCount := len(rt.GetJavaScriptErrors())

		b.WriteString(fmt.Sprintf("Active Timers (setTimeout/setInterval): %d\n", timers))
		b.WriteString(fmt.Sprintf("Console Messages:                      %d\n", consoleCount))
		b.WriteString(fmt.Sprintf("JavaScript Errors:                     %d\n", errorCount))
		b.WriteString(fmt.Sprintf("Script Running:                        %s\n", map[bool]string{true: "Yes", false: "No"}[rt.RunningScriptCount() > 0]))
		p.label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Script Task Queue", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
}

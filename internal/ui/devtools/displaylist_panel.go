package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type displayListPanel struct {
	fyne.Container
	label *widget.Label
}

func newDisplayListPanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	p := &displayListPanel{
		label: widget.NewLabel("No display list built yet."),
	}
	p.label.Wrapping = fyne.TextWrapWord

	refreshBtn := widget.NewButton("Refresh", func() {
		ctx := activeTab()
		if ctx == nil || ctx.Renderer == nil {
			p.label.SetText("No renderer available.")
			return
		}

		summary := ctx.Renderer.GetDisplayListSummary()
		cmds := ctx.Renderer.GetDisplayListCommands()
		if len(cmds) == 0 {
			p.label.SetText("No display list built yet.")
			return
		}

		total := 0
		for _, c := range summary {
			total += c
		}

		var b strings.Builder
		b.WriteString(fmt.Sprintf("Total: %d commands\n\n", total))
		for _, name := range displayListTypeOrder() {
			count, ok := summary[name]
			if !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("  %-12s %d\n", name+":", count))
		}

		b.WriteString("\n--- Command Details ---\n")
		for i, cmd := range cmds {
			line := fmt.Sprintf("%d. %s", i+1, cmd.Type.String())
			switch cmd.Type {
			case renderer.PaintText:
				txt := cmd.Text
				if len(txt) > 40 {
					txt = txt[:37] + "..."
				}
				line += fmt.Sprintf("  text=%q  font=%.0f  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					txt, cmd.FontSize, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintRect:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintImage:
				src := cmd.ImageSrc
				if len(src) > 40 {
					src = src[:37] + "..."
				}
				line += fmt.Sprintf("  src=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", src, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintLink:
				line += fmt.Sprintf("  url=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)", cmd.LinkURL, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintBorder:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  stroke=%.0f",
					cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.StrokeWidth)
			case renderer.PaintButton:
				line += fmt.Sprintf("  text=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.ButtonText, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintInput:
				line += fmt.Sprintf("  type=%s  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.InputType, cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PaintTextarea:
				line += fmt.Sprintf("  value=%s  placeholder=%s  pos=(%.0f,%.0f)  size=(%.0f×%.0f)",
					cmd.InputValue, cmd.Placeholder, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			case renderer.PushClip:
				line += fmt.Sprintf("  pos=(%.0f,%.0f)  size=(%.0f×%.0f)  overflow=%s",
					cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, cmd.ClipOverflow)
			case renderer.PopClip:
			}
			b.WriteString(line + "\n")
		}
		p.label.SetText(b.String())
	})

	topBar := container.NewBorder(nil, nil, refreshBtn, nil,
		widget.NewLabelWithStyle("Display List Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	return container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.label))
}

func displayListTypeOrder() []string {
	return []string{"Text", "Rect", "Image", "Link", "Border", "Button", "Input", "Textarea", "PushClip", "PopClip"}
}

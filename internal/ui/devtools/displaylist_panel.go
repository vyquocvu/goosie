package devtools

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// displayListPanel is the Display List inspector. The spec calls
// for a two-pane layout: a tree of paint commands on the left and
// per-command properties on the right. Clicking a command also
// outlines the corresponding region on the page canvas via the
// renderer's highlight hook. This is a significant step up from
// the earlier single-label dump because the actual data — number
// of commands, types, sizes, text — is large and a tree makes it
// browsable without losing context.
type displayListPanel struct {
	fyne.Container

	// Counts and labels summarising the display list.
	totalLabel   *widget.Label
	summaryList  *widget.List
	commandsList *widget.List

	// Right-pane detail view.
	detailText *widget.Label

	// Selection state: which command is highlighted, and a callback
	// the renderer wires up to outline the page rectangle.
	selectedIndex   int
	highlightTarget func(nodeID int)
}

// newDisplayListPanelContent builds the full Display List panel.
//
// The wiring assumes the renderer provided via TabContext implements
// the rendererProvider interface defined in dock.go — specifically
// GetDisplayListSummary, GetDisplayListCommands, and the highlight
// hook used to outline the selected command on the page canvas.
//
// Callers should pass an activeTab callback that returns the most
// recent TabContext so the panel can pull fresh display-list data on
// every Refresh.
//
// The function returns a fyne.CanvasObject; the underlying
// displayListPanel value is reachable via SetHighlightTarget which
// installs the renderer's per-command outline callback. The panel
// is intentionally not exported to keep its public surface area
// the same as the other devtools panels.
func newDisplayListPanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	p := &displayListPanel{
		totalLabel:      widget.NewLabel("No display list available."),
		detailText:      widget.NewLabel("Select a command to see its details."),
		selectedIndex:   -1,
		highlightTarget: nil,
	}
	p.detailText.Wrapping = fyne.TextWrapWord
	p.detailText.TextStyle.Monospace = true

	// Bindings make refresh cheap: Fyne only re-renders when the
	// underlying data actually changes.
	totalBinding := binding.NewString()
	summaryBinding := binding.NewStringList()
	commandsBinding := binding.NewStringList()
	p.totalLabel.Bind(totalBinding)

	// Refresh pulls fresh data from the active tab context and
	// pushes it into the bindings so all three widgets redraw.
	refresh := func() {
		ctx := activeTab()
		if ctx == nil || ctx.Renderer == nil {
			_ = totalBinding.Set("No renderer available.")
			_ = summaryBinding.Set(nil)
			_ = commandsBinding.Set(nil)
			return
		}

		summary := ctx.Renderer.GetDisplayListSummary()
		cmds := ctx.Renderer.GetDisplayListCommands()
		if len(cmds) == 0 {
			_ = totalBinding.Set("No display list built yet.")
			_ = summaryBinding.Set(nil)
			_ = commandsBinding.Set(nil)
			return
		}

		total := 0
		for _, c := range summary {
			total += c
		}
		_ = totalBinding.Set(fmt.Sprintf("Total: %d commands", total))

		// Summary: one row per command type, in displayListTypeOrder
		// so the order matches how the renderer emits them.
		summaryLines := []string{}
		for _, name := range displayListTypeOrder() {
			count, ok := summary[name]
			if !ok || count == 0 {
				continue
			}
			summaryLines = append(summaryLines, fmt.Sprintf("%-10s %d", name+":", count))
		}
		_ = summaryBinding.Set(summaryLines)

		// Per-command lines for the centre list.
		commandLines := make([]string, len(cmds))
		for i, cmd := range cmds {
			commandLines[i] = formatCommandLine(i, cmd)
		}
		_ = commandsBinding.Set(commandLines)
		p.selectedIndex = -1
		p.detailText.SetText("Select a command to see its details.")
	}

	// Summary list: read-only type counts. No selection handling.
	p.summaryList = widget.NewListWithData(summaryBinding,
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.TextStyle.Monospace = true
			return label
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			obj.(*widget.Label).Bind(item.(binding.String))
		},
	)

	// Commands list: one row per paint command. Selecting a row
	// shows the detail pane and triggers the highlight hook.
	p.commandsList = widget.NewListWithData(commandsBinding,
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.TextStyle.Monospace = true
			return label
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			obj.(*widget.Label).Bind(item.(binding.String))
		},
	)
	p.commandsList.OnSelected = func(id widget.ListItemID) {
		p.selectedIndex = id
		ctx := activeTab()
		if ctx == nil || ctx.Renderer == nil {
			return
		}
		cmds := ctx.Renderer.GetDisplayListCommands()
		if id < 0 || id >= len(cmds) {
			return
		}
		cmd := cmds[id]
		p.detailText.SetText(formatCommandDetail(id, cmd))
		if p.highlightTarget != nil && cmd.NodeID > 0 {
			p.highlightTarget(int(cmd.NodeID))
		}
	}

	refreshBtn := widget.NewButton("Refresh", refresh)
	clearBtn := widget.NewButton("Clear Highlight", func() {
		if p.highlightTarget != nil {
			p.highlightTarget(-1)
		}
	})

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(refreshBtn, clearBtn), nil,
		widget.NewLabelWithStyle("Display List Inspector", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	// Two-column layout: command list + summary on the left, detail
	// text on the right.
	leftPane := container.NewVSplit(
		container.NewBorder(widget.NewLabelWithStyle("Commands", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nil, nil, nil, p.commandsList),
		container.NewBorder(widget.NewLabelWithStyle("Type Summary", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nil, nil, nil, p.summaryList),
	)
	rightPane := container.NewBorder(
		widget.NewLabelWithStyle("Command Detail", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		nil, nil, nil,
		container.NewScroll(p.detailText),
	)

	// Outer layout: top bar with summary, then split between left
	// (commands + summary) and right (detail).
	body := container.NewHSplit(leftPane, rightPane)
	p.Container = *container.NewBorder(topBar, nil, nil, nil, body)

	// Kick off the first refresh so the panel is populated when
	// the user first opens it.
	refresh()
	return &p.Container
}

// SetHighlightTarget wires the panel to a callback the renderer
// invokes when the user clicks a command. The callback receives a
// node id (>= 0) to outline on the page canvas, or -1 to clear
// the highlight.
func (p *displayListPanel) SetHighlightTarget(fn func(nodeID int)) {
	p.highlightTarget = fn
}

// newDisplayListPanelWithHighlight is the public constructor for
// callers that want to wire the highlight hook up-front. It builds
// the same panel as newDisplayListPanelContent but lets the caller
// inject the renderer's outline callback. Tests use this; the dock
// uses newDisplayListPanelContent and wires the highlight later.
func newDisplayListPanelWithHighlight(activeTab func() *TabContext, highlight func(nodeID int)) fyne.CanvasObject {
	obj := newDisplayListPanelContent(activeTab)
	if obj == nil {
		return nil
	}
	if p, ok := obj.(*displayListPanel); ok {
		p.SetHighlightTarget(highlight)
	}
	return obj
}

// formatCommandLine returns a single short summary line for the
// commands list. The format is intentionally compact so the
// user can scan many commands at once without scrolling.
func formatCommandLine(idx int, cmd renderer.PaintCommand) string {
	head := fmt.Sprintf("%3d. %-9s", idx+1, cmd.Type.String())
	switch cmd.Type {
	case renderer.PaintText:
		txt := cmd.Text
		if len(txt) > 32 {
			txt = txt[:29] + "..."
		}
		return head + fmt.Sprintf(" %q", txt)
	case renderer.PaintImage:
		src := cmd.ImageSrc
		if len(src) > 32 {
			src = src[:29] + "..."
		}
		return head + fmt.Sprintf(" %s", src)
	case renderer.PaintLink:
		return head + fmt.Sprintf(" %s", cmd.LinkURL)
	default:
		return head
	}
}

// formatCommandDetail produces the right-pane text for the selected
// command. The text is multi-line and monospaced so each property
// aligns in a way that's easy to scan.
func formatCommandDetail(idx int, cmd renderer.PaintCommand) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Command #%d\n", idx+1)
	fmt.Fprintf(&b, "  Type:      %s\n", cmd.Type)
	if cmd.NodeID > 0 {
		fmt.Fprintf(&b, "  NodeID:    %d\n", cmd.NodeID)
	}
	fmt.Fprintf(&b, "  Position:  (%.0f, %.0f)\n", cmd.Box.X, cmd.Box.Y)
	fmt.Fprintf(&b, "  Size:      %.0f × %.0f\n", cmd.Box.Width, cmd.Box.Height)
	switch cmd.Type {
	case renderer.PaintText:
		fmt.Fprintf(&b, "  Font size: %.0f\n", cmd.FontSize)
		fmt.Fprintf(&b, "  Text:      %s\n", cmd.Text)
	case renderer.PaintImage:
		fmt.Fprintf(&b, "  Source:    %s\n", cmd.ImageSrc)
	case renderer.PaintLink:
		fmt.Fprintf(&b, "  URL:       %s\n", cmd.LinkURL)
		fmt.Fprintf(&b, "  Text:      %s\n", cmd.LinkText)
	case renderer.PaintBorder:
		fmt.Fprintf(&b, "  Stroke:    %.0f\n", cmd.StrokeWidth)
	case renderer.PaintButton:
		fmt.Fprintf(&b, "  Text:      %s\n", cmd.ButtonText)
	case renderer.PaintInput, renderer.PaintTextarea:
		fmt.Fprintf(&b, "  Type:      %s\n", cmd.InputType)
		fmt.Fprintf(&b, "  Value:     %s\n", cmd.InputValue)
		fmt.Fprintf(&b, "  Placeholder: %s\n", cmd.Placeholder)
	case renderer.PushClip:
		fmt.Fprintf(&b, "  Overflow:  %s\n", cmd.ClipOverflow)
	}
	return b.String()
}

// displayListTypeOrder returns the order in which paint command
// types are listed in the summary. Kept as a function (rather than
// a package-level variable) so tests can mutate it without
// affecting the production rendering order.
func displayListTypeOrder() []string {
	return []string{"Text", "Rect", "Image", "Link", "Border", "Button", "Input", "Textarea", "PushClip", "PopClip"}
}
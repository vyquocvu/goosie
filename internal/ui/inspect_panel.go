package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// InspectPanel represents the element inspector panel
type InspectPanel struct {
	container      *fyne.Container
	elementInfo    *widget.RichText
	closeButton    *widget.Button
	onClose        func()
	selectedNode   *renderer.RenderNode
	selectedLayout *renderer.LayoutBox
}

// NewInspectPanel creates a new inspect panel
func NewInspectPanel(onClose func()) *InspectPanel {
	panel := &InspectPanel{
		onClose: onClose,
	}

	// Create close button
	panel.closeButton = widget.NewButton("X", func() {
		if panel.onClose != nil {
			panel.onClose()
		}
	})

	// Create element info display
	panel.elementInfo = widget.NewRichText()
	panel.elementInfo.Wrapping = fyne.TextWrapWord
	panel.elementInfo.Scroll = container.ScrollBoth

	// Create scroll container for element info
	infoScroll := container.NewScroll(panel.elementInfo)
	infoScroll.SetMinSize(fyne.NewSize(300, 200))

	// Create top bar with controls
	topBar := container.NewBorder(
		nil, nil,
		container.NewHBox(panel.closeButton),
		widget.NewLabel("Element Inspector"),
	)

	// Create main container
	panel.container = container.NewBorder(
		topBar, nil, nil, nil,
		infoScroll,
	)

	return panel
}

// GetContainer returns the inspect panel's container
func (ip *InspectPanel) GetContainer() *fyne.Container {
	return ip.container
}

// SetElement sets the element to inspect
func (ip *InspectPanel) SetElement(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	ip.selectedNode = node
	ip.selectedLayout = layout
	ip.updateElementInfo()
}

// Clear clears the inspected element
func (ip *InspectPanel) Clear() {
	ip.selectedNode = nil
	ip.selectedLayout = nil
	ip.elementInfo.ParseMarkdown("No element selected. Hover over an element to inspect it.")
}

// updateElementInfo updates the displayed element information
func (ip *InspectPanel) updateElementInfo() {
	if ip.selectedNode == nil {
		ip.elementInfo.ParseMarkdown("No element selected.")
		return
	}

	var info strings.Builder

	// Element tag/type
	if ip.selectedNode.Type == renderer.NodeTypeElement {
		info.WriteString(fmt.Sprintf("**Tag:** `<%s>`\n\n", ip.selectedNode.TagName))
	} else if ip.selectedNode.Type == renderer.NodeTypeText {
		info.WriteString("**Type:** Text Node\n\n")
	}

	// Node ID
	info.WriteString(fmt.Sprintf("**Node ID:** %d\n\n", ip.selectedNode.ID))

	// Attributes
	if len(ip.selectedNode.Attrs) > 0 {
		info.WriteString("**Attributes:**\n")
		for key, value := range ip.selectedNode.Attrs {
			info.WriteString(fmt.Sprintf("- `%s` = `%s`\n", key, value))
		}
		info.WriteString("\n")
	}

	// Text content (for text nodes or elements with text)
	if ip.selectedNode.Type == renderer.NodeTypeText {
		text := strings.TrimSpace(ip.selectedNode.Text)
		if text != "" {
			// Truncate long text
			if len(text) > 100 {
				text = text[:100] + "..."
			}
			info.WriteString(fmt.Sprintf("**Text:** `%s`\n\n", text))
		}
	}

	// Layout information
	if ip.selectedLayout != nil {
		info.WriteString("**Layout:**\n")
		info.WriteString(fmt.Sprintf("- Position: (%.1f, %.1f)\n", ip.selectedLayout.Box.X, ip.selectedLayout.Box.Y))
		info.WriteString(fmt.Sprintf("- Size: %.1f × %.1f\n", ip.selectedLayout.Box.Width, ip.selectedLayout.Box.Height))
		info.WriteString(fmt.Sprintf("- Display: %s\n", ip.selectedLayout.Display))
		info.WriteString("\n")
	}

	// Computed styles
	if ip.selectedNode.ComputedStyle != nil {
		style := ip.selectedNode.ComputedStyle
		hasStyles := false
		info.WriteString("**Computed Styles:**\n")

		if style.Color != nil {
			info.WriteString(fmt.Sprintf("- Color: %v\n", style.Color))
			hasStyles = true
		}
		if style.BackgroundColor != nil {
			info.WriteString(fmt.Sprintf("- Background: %v\n", style.BackgroundColor))
			hasStyles = true
		}
		if style.FontSize > 0 {
			info.WriteString(fmt.Sprintf("- Font Size: %.1fpx\n", style.FontSize))
			hasStyles = true
		}
		if style.FontWeight != "" {
			info.WriteString(fmt.Sprintf("- Font Weight: %s\n", style.FontWeight))
			hasStyles = true
		}
		if style.FontFamily != "" {
			info.WriteString(fmt.Sprintf("- Font Family: %s\n", style.FontFamily))
			hasStyles = true
		}
		if style.Display != "" {
			info.WriteString(fmt.Sprintf("- Display: %s\n", style.Display))
			hasStyles = true
		}
		if style.Width != "" {
			info.WriteString(fmt.Sprintf("- Width: %s\n", style.Width))
			hasStyles = true
		}
		if style.Height != "" {
			info.WriteString(fmt.Sprintf("- Height: %s\n", style.Height))
			hasStyles = true
		}

		// Box model
		if style.MarginTop != "" || style.MarginRight != "" || style.MarginBottom != "" || style.MarginLeft != "" {
			info.WriteString(fmt.Sprintf("- Margin: %s %s %s %s\n", style.MarginTop, style.MarginRight, style.MarginBottom, style.MarginLeft))
			hasStyles = true
		}
		if style.PaddingTop != "" || style.PaddingRight != "" || style.PaddingBottom != "" || style.PaddingLeft != "" {
			info.WriteString(fmt.Sprintf("- Padding: %s %s %s %s\n", style.PaddingTop, style.PaddingRight, style.PaddingBottom, style.PaddingLeft))
			hasStyles = true
		}

		if !hasStyles {
			info.WriteString("(none)\n")
		}
		info.WriteString("\n")
	}

	// Children count
	if len(ip.selectedNode.Children) > 0 {
		info.WriteString(fmt.Sprintf("**Children:** %d\n\n", len(ip.selectedNode.Children)))
	}

	ip.elementInfo.ParseMarkdown(info.String())
}

// CanvasObject returns the underlying canvas object for the panel
func (ip *InspectPanel) CanvasObject() fyne.CanvasObject {
	return ip.container
}


package renderer

import (
	"image/color"
	"sort"
	"strings"
)

// PaintCommandType represents the type of paint command
type PaintCommandType int

const (
	// PaintText represents a text paint command
	PaintText PaintCommandType = iota
	// PaintRect represents a rectangle paint command
	PaintRect
	// PaintImage represents an image paint command
	PaintImage
	// PaintLink represents a link paint command
	PaintLink
	// PaintBorder represents a border paint command
	PaintBorder
	// PaintButton represents a button paint command
	PaintButton
	// PushClip represents a command to start a clipping region
	PushClip
	// PopClip represents a command to end a clipping region
	PopClip
)

// PaintCommand represents a single paint operation
type PaintCommand struct {
	Type   PaintCommandType
	NodeID int64       // ID of the node this command is for
	Node   *RenderNode // Direct reference to the render node
	Box    Rect        // Position and size for the command

	// Text-specific fields
	Text     string
	FontSize float32
	Bold     bool
	Italic   bool

	// Rectangle-specific fields
	FillColor   color.Color
	StrokeColor color.Color
	StrokeWidth float32

	// Image-specific fields
	ImageSrc string
	ImageAlt string

	// Link-specific fields
	LinkURL  string
	LinkText string

	// Button-specific fields
	ButtonText string
	OnClick    string // onclick attribute value

	// Border-specific fields
	BorderTopWidth    float32
	BorderRightWidth  float32
	BorderBottomWidth float32
	BorderLeftWidth   float32
	BorderTopColor    color.Color
	BorderRightColor  color.Color
	BorderBottomColor color.Color
	BorderLeftColor   color.Color
	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	// Clip-specific fields
	ClipOverflow string // "hidden", "scroll", "auto"
}

// DisplayList represents a list of paint commands
type DisplayList struct {
	Commands []*PaintCommand
}

// NewDisplayList creates a new display list
func NewDisplayList() *DisplayList {
	return &DisplayList{
		Commands: make([]*PaintCommand, 0),
	}
}

// AddCommand adds a paint command to the display list
func (dl *DisplayList) AddCommand(cmd *PaintCommand) {
	dl.Commands = append(dl.Commands, cmd)
}

// Clear removes all commands from the display list
func (dl *DisplayList) Clear() {
	dl.Commands = make([]*PaintCommand, 0)
}

// DisplayListBuilder builds a display list from a layout tree and render tree
type DisplayListBuilder struct {
	defaultFontSize float32
	fontMetrics     *FontMetrics
}

// NewDisplayListBuilder creates a new display list builder
func NewDisplayListBuilder() *DisplayListBuilder {
	defaultSize := float32(16.0)
	return &DisplayListBuilder{
		defaultFontSize: defaultSize,
		fontMetrics:     NewFontMetrics(defaultSize),
	}
}

// Build builds a display list from a layout tree and render tree
func (dlb *DisplayListBuilder) Build(layoutRoot *LayoutBox, renderRoot *RenderNode) *DisplayList {
	displayList := NewDisplayList()

	if layoutRoot == nil || renderRoot == nil {
		return displayList
	}

	// Build a map of render nodes by ID for quick lookup
	renderMap := dlb.buildRenderMap(renderRoot)

	// Walk the layout tree and generate paint commands
	dlb.buildRecursive(layoutRoot, renderMap, displayList)

	return displayList
}

// buildRenderMap builds a map of render nodes indexed by their ID
func (dlb *DisplayListBuilder) buildRenderMap(root *RenderNode) map[int64]*RenderNode {
	nodeMap := make(map[int64]*RenderNode)
	dlb.buildRenderMapRecursive(root, nodeMap)
	return nodeMap
}

// buildRenderMapRecursive recursively builds the render node map
func (dlb *DisplayListBuilder) buildRenderMapRecursive(node *RenderNode, nodeMap map[int64]*RenderNode) {
	if node == nil {
		return
	}

	nodeMap[node.ID] = node

	for _, child := range node.Children {
		dlb.buildRenderMapRecursive(child, nodeMap)
	}
}

// buildRecursive recursively builds paint commands for a layout box
func (dlb *DisplayListBuilder) buildRecursive(layoutBox *LayoutBox, renderMap map[int64]*RenderNode, displayList *DisplayList) {
	if layoutBox == nil {
		return
	}

	// Get the corresponding render node
	renderNode, exists := renderMap[layoutBox.NodeID]
	if !exists {
		return
	}

	// Add border paint command if the element has borders
	dlb.addBorderCommand(layoutBox, renderNode, displayList)

	// Special handling for button elements - they should be rendered as buttons, not as text
	if renderNode.Type == NodeTypeElement && renderNode.TagName == "button" {
		dlb.addElementCommand(layoutBox, renderNode, displayList)
		// Don't process children for buttons - the button text is extracted in addElementCommand
		// Process children layout boxes for nested elements if any
		for _, child := range layoutBox.Children {
			dlb.buildRecursive(child, renderMap, displayList)
		}
		return
	}

	// Check if this layout box has inline content (LineBoxes)
	if len(layoutBox.LineBoxes) > 0 {
		// Group inline boxes by NodeID to avoid duplicates
		processedNodes := make(map[int64]bool)

		// Process inline boxes from LineBoxes
		for _, lineBox := range layoutBox.LineBoxes {
			for _, inlineBox := range lineBox.InlineBoxes {
				// Skip if we've already processed this node
				if processedNodes[inlineBox.NodeID] {
					continue
				}
				processedNodes[inlineBox.NodeID] = true

				if inlineBox.IsText {
					// Get the render node for this inline box
					inlineRenderNode, inlineExists := renderMap[inlineBox.NodeID]
					if !inlineExists {
						continue
					}

					// Get text style from node hierarchy
					style := dlb.fontMetrics.GetTextStyleFromNode(inlineRenderNode)

					// Get font size from parent
					fontSize := dlb.defaultFontSize
					if inlineRenderNode.Parent != nil {
						fontSize = dlb.fontMetrics.GetFontSize(inlineRenderNode.Parent.TagName)
					}

					// Create paint command for the full text of the node
					// Use the layout box dimensions for the entire element
					cmd := &PaintCommand{
						Type:     PaintText,
						NodeID:   inlineBox.NodeID,
						Node:     inlineRenderNode,
						Box:      layoutBox.Box,
						Text:     inlineRenderNode.Text,
						FontSize: fontSize,
						Bold:     style.Bold,
						Italic:   style.Italic,
					}

					displayList.AddCommand(cmd)
				} else {
					// Handle inline-block elements if needed
					// For now, skip them as they should have their own LayoutBox
				}
			}
		}
	} else {
		// No inline content - generate paint command based on node type
		if renderNode.Type == NodeTypeText {
			dlb.addTextCommand(layoutBox, renderNode, displayList)
		} else if renderNode.Type == NodeTypeElement {
			dlb.addElementCommand(layoutBox, renderNode, displayList)
		}
	}

	// Check for overflow property
	isOverflow := false
	if renderNode.ComputedStyle != nil && (renderNode.ComputedStyle.Overflow == "hidden" || renderNode.ComputedStyle.Overflow == "scroll" || renderNode.ComputedStyle.Overflow == "auto") {
		isOverflow = true
		// Push clip command
		dlb.addPushClipCommand(layoutBox, renderNode, displayList)
	}

	// Process children
	// Create a copy of children to sort by z-index
	children := make([]*LayoutBox, len(layoutBox.Children))
	copy(children, layoutBox.Children)

	// Sort children by z-index
	sort.SliceStable(children, func(i, j int) bool {
		nodeI := renderMap[children[i].NodeID]
		nodeJ := renderMap[children[j].NodeID]

		zIndexI := 0
		if nodeI != nil && nodeI.ComputedStyle != nil {
			zIndexI = nodeI.ComputedStyle.ZIndex
		}

		zIndexJ := 0
		if nodeJ != nil && nodeJ.ComputedStyle != nil {
			zIndexJ = nodeJ.ComputedStyle.ZIndex
		}

		return zIndexI < zIndexJ
	})

	for _, child := range children {
		dlb.buildRecursive(child, renderMap, displayList)
	}

	// Pop clip command if needed
	if isOverflow {
		dlb.addPopClipCommand(layoutBox, renderNode, displayList)
	}
}

// addPushClipCommand adds a push clip command
func (dlb *DisplayListBuilder) addPushClipCommand(layoutBox *LayoutBox, renderNode *RenderNode, displayList *DisplayList) {
	cmd := &PaintCommand{
		Type:         PushClip,
		NodeID:       layoutBox.NodeID,
		Node:         renderNode,
		Box:          layoutBox.Box,
		ClipOverflow: renderNode.ComputedStyle.Overflow,
	}
	displayList.AddCommand(cmd)
}

// addPopClipCommand adds a pop clip command
func (dlb *DisplayListBuilder) addPopClipCommand(layoutBox *LayoutBox, renderNode *RenderNode, displayList *DisplayList) {
	cmd := &PaintCommand{
		Type:   PopClip,
		NodeID: layoutBox.NodeID,
		Node:   renderNode,
		Box:    layoutBox.Box,
	}
	displayList.AddCommand(cmd)
}

// addTextCommand adds a text paint command
func (dlb *DisplayListBuilder) addTextCommand(layoutBox *LayoutBox, renderNode *RenderNode, displayList *DisplayList) {
	text := renderNode.Text
	if text == "" {
		return
	}

	// Get text style from node hierarchy
	style := dlb.fontMetrics.GetTextStyleFromNode(renderNode)

	// Get font size from parent
	fontSize := dlb.defaultFontSize
	if renderNode.Parent != nil {
		fontSize = dlb.fontMetrics.GetFontSize(renderNode.Parent.TagName)
	}

	cmd := &PaintCommand{
		Type:     PaintText,
		NodeID:   layoutBox.NodeID,
		Node:     renderNode,
		Box:      layoutBox.Box,
		Text:     text,
		FontSize: fontSize,
		Bold:     style.Bold,
		Italic:   style.Italic,
	}

	displayList.AddCommand(cmd)
}

// addElementCommand adds paint commands for an element
func (dlb *DisplayListBuilder) addElementCommand(layoutBox *LayoutBox, renderNode *RenderNode, displayList *DisplayList) {
	// For link elements, add a link paint command
	if renderNode.TagName == "a" {
		href, hasHref := renderNode.GetAttribute("href")
		if hasHref && href != "" {
			// Extract link text from child text nodes
			linkText := dlb.extractText(renderNode)
			if linkText != "" {
				cmd := &PaintCommand{
					Type:     PaintLink,
					NodeID:   layoutBox.NodeID,
					Node:     renderNode,
					Box:      layoutBox.Box,
					LinkURL:  href,
					LinkText: linkText,
				}
				displayList.AddCommand(cmd)
			}
		}
		return
	}

	// For image elements, add a rectangle placeholder and text
	if renderNode.TagName == "img" {
		// Check visibility
		if renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Visibility == "hidden" {
			// Add transparent placeholder to maintain layout space
			cmd := &PaintCommand{
				Type:      PaintRect,
				NodeID:    layoutBox.NodeID,
				Node:      renderNode,
				Box:       layoutBox.Box,
				FillColor: color.Transparent,
			}
			displayList.AddCommand(cmd)
			return
		}

		// Add image info text if available
		src, _ := renderNode.GetAttribute("src")
		alt, _ := renderNode.GetAttribute("alt")

		if src != "" || alt != "" {
			textCmd := &PaintCommand{
				Type:     PaintImage,
				NodeID:   layoutBox.NodeID,
				Node:     renderNode,
				Box:      layoutBox.Box,
				ImageSrc: src,
				ImageAlt: alt,
			}
			displayList.AddCommand(textCmd)
		}
		return
	}

	// For button elements, add a button paint command
	if renderNode.TagName == "button" {
		buttonText := dlb.extractText(renderNode)
		onclick, _ := renderNode.GetAttribute("onclick")

		cmd := &PaintCommand{
			Type:       PaintButton,
			NodeID:     layoutBox.NodeID,
			Node:       renderNode,
			Box:        layoutBox.Box,
			ButtonText: buttonText,
			OnClick:    onclick,
		}
		displayList.AddCommand(cmd)
		return
	}

	// For other elements, we primarily rely on their children for rendering
	// but we could add background colors, borders, etc. here in the future
}

// extractText extracts text content from a render node
func (dlb *DisplayListBuilder) extractText(node *RenderNode) string {
	if node == nil {
		return ""
	}

	if node.Type == NodeTypeText {
		return strings.TrimSpace(node.Text)
	}

	var result strings.Builder
	for _, child := range node.Children {
		text := dlb.extractText(child)
		if text != "" {
			if result.Len() > 0 {
				result.WriteString(" ")
			}
			result.WriteString(text)
		}
	}

	return strings.TrimSpace(result.String())
}

// addBorderCommand adds border paint commands for an element
func (dlb *DisplayListBuilder) addBorderCommand(layoutBox *LayoutBox, renderNode *RenderNode, displayList *DisplayList) {
	// Check if any border is present
	hasBorder := false

	// Check if any border width is set and style is not "none" or empty
	if (layoutBox.BorderTopWidth > 0 && layoutBox.BorderTopStyle != "" && layoutBox.BorderTopStyle != "none") ||
		(layoutBox.BorderRightWidth > 0 && layoutBox.BorderRightStyle != "" && layoutBox.BorderRightStyle != "none") ||
		(layoutBox.BorderBottomWidth > 0 && layoutBox.BorderBottomStyle != "" && layoutBox.BorderBottomStyle != "none") ||
		(layoutBox.BorderLeftWidth > 0 && layoutBox.BorderLeftStyle != "" && layoutBox.BorderLeftStyle != "none") {
		hasBorder = true
	}

	if !hasBorder {
		return
	}

	// Create border paint command
	cmd := &PaintCommand{
		Type:   PaintBorder,
		NodeID: layoutBox.NodeID,
		Node:   renderNode,
		Box:    layoutBox.Box,

		BorderTopWidth:    layoutBox.BorderTopWidth,
		BorderRightWidth:  layoutBox.BorderRightWidth,
		BorderBottomWidth: layoutBox.BorderBottomWidth,
		BorderLeftWidth:   layoutBox.BorderLeftWidth,

		BorderTopStyle:    layoutBox.BorderTopStyle,
		BorderRightStyle:  layoutBox.BorderRightStyle,
		BorderBottomStyle: layoutBox.BorderBottomStyle,
		BorderLeftStyle:   layoutBox.BorderLeftStyle,

		BorderTopColor:    layoutBox.BorderTopColor,
		BorderRightColor:  layoutBox.BorderRightColor,
		BorderBottomColor: layoutBox.BorderBottomColor,
		BorderLeftColor:   layoutBox.BorderLeftColor,
	}

	displayList.AddCommand(cmd)
}

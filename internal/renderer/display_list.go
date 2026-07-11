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
	// PaintInput represents a native form input paint command
	PaintInput
	// PaintTextarea represents a native textarea paint command
	PaintTextarea
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
	Text          string
	FontSize      float32
	Color         color.Color // Text color from CSS
	Bold          bool
	Italic        bool
	Underline     bool
	Strikethrough bool

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

	// Input-specific fields
	InputType   string
	InputValue  string
	Placeholder string

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

// YBand represents a horizontal band of the display list at a given Y-range.
// Commands between cmdStart and cmdEnd fall within this Y range.
type YBand struct {
	YStart   float32
	YEnd     float32
	CmdStart int
	CmdEnd   int
}

// DisplayList represents a list of paint commands
type DisplayList struct {
	Commands []*PaintCommand
	YBands   []YBand // Spatial index — commands grouped by Y-range (~200px bands)
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

// SortByZIndex reorders PaintCommands so lower z-index paints before higher z-index.
func SortByZIndex(dl *DisplayList) {
	sort.SliceStable(dl.Commands, func(i, j int) bool {
		return zIndexOf(dl.Commands[i]) < zIndexOf(dl.Commands[j])
	})
}

func zIndexOf(cmd *PaintCommand) int {
	if cmd.Node != nil && cmd.Node.ComputedStyle != nil {
		return cmd.Node.ComputedStyle.ZIndex
	}
	return 0
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

const yBandHeight = float32(200)

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

	// Build spatial Y-band index for viewport culling
	buildYBands(displayList)

	return displayList
}

// buildYBands partitions display list leaf commands into spatial Y-bands for
// efficient viewport culling. Each band groups ~200px of vertical space so
// that RenderWithViewport can skip entire groups of off-screen commands.
func buildYBands(dl *DisplayList) {
	if len(dl.Commands) == 0 {
		return
	}

	// Find Y range of non-clip commands
	minY := float32(0)
	maxY := float32(0)
	first := true
	for _, cmd := range dl.Commands {
		if cmd.Type == PushClip || cmd.Type == PopClip {
			continue
		}
		cmdBottom := cmd.Box.Y + cmd.Box.Height
		if first {
			minY = cmd.Box.Y
			maxY = cmdBottom
			first = false
		} else {
			if cmd.Box.Y < minY {
				minY = cmd.Box.Y
			}
			if cmdBottom > maxY {
				maxY = cmdBottom
			}
		}
	}
	if first {
		return
	}

	bandH := yBandHeight
	numBands := int((maxY-minY)/bandH) + 1
	bands := make([]YBand, numBands)
	for b := 0; b < numBands; b++ {
		bands[b] = YBand{
			YStart:   minY + float32(b)*bandH,
			YEnd:     minY + float32(b+1)*bandH,
			CmdStart: -1,
			CmdEnd:   -1,
		}
	}

	currentBand := 0
	for i, cmd := range dl.Commands {
		if cmd.Type == PushClip || cmd.Type == PopClip {
			continue
		}
		for currentBand < numBands-1 && cmd.Box.Y >= bands[currentBand+1].YStart {
			if bands[currentBand].CmdEnd < 0 {
				bands[currentBand].CmdEnd = i
			}
			currentBand++
		}
		if bands[currentBand].CmdStart < 0 {
			bands[currentBand].CmdStart = i
		}
		bands[currentBand].CmdEnd = i + 1
	}

	// Fill empty trailing bands with their predecessor's CmdEnd (marks them as empty)
	for b := numBands - 1; b >= 0; b-- {
		if bands[b].CmdStart < 0 {
			if b == 0 {
				bands[b].CmdStart = 0
				bands[b].CmdEnd = 0
			} else {
				bands[b].CmdStart = bands[b-1].CmdEnd
				bands[b].CmdEnd = bands[b-1].CmdEnd
			}
		}
	}

	dl.YBands = bands
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

	// Skip elements with display:none (already excluded from layout tree, but guard here too)
	if renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Display == "none" {
		return
	}

	// For visibility:hidden, skip paint commands but still process children (they maintain space in layout)
	isHidden := renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Visibility == "hidden"

	// Paint background color if present (drawn behind borders and content)
	if !isHidden && renderNode.Type == NodeTypeElement && layoutBox.BackgroundColor != nil && layoutBox.BackgroundColor != color.Transparent {
		if renderNode.TagName != "button" {
			cmd := &PaintCommand{
				Type:      PaintRect,
				NodeID:    layoutBox.NodeID,
				Node:      renderNode,
				Box:       layoutBox.Box,
				FillColor: layoutBox.BackgroundColor,
			}
			displayList.AddCommand(cmd)
		}
	}

	// Add border paint command if the element has borders (skip if hidden)
	if !isHidden {
		dlb.addBorderCommand(layoutBox, renderNode, displayList)
	}

	// Special handling for form elements - they should be rendered as native controls, not as text
	if !isHidden && renderNode.Type == NodeTypeElement && (renderNode.TagName == "button" || renderNode.TagName == "input" || renderNode.TagName == "textarea") {
		dlb.addElementCommand(layoutBox, renderNode, displayList)
		// Don't process children for form inputs/textareas/buttons - their values/texts are extracted in addElementCommand
		for _, child := range layoutBox.Children {
			dlb.buildRecursive(child, renderMap, displayList)
		}
		return
	}

	// Check if this layout box has inline content (LineBoxes)
	// (skip rendering text if element is hidden)
	if !isHidden && len(layoutBox.LineBoxes) > 0 {
		for _, lineBox := range layoutBox.LineBoxes {
			// Coalesce inline text fragments with the same NodeID per line
			type textAccum struct {
				node      *RenderNode
				text      strings.Builder
				box       Rect // top-left of first fragment
				fontSize  float32
				color     color.Color
				bold      bool
				italic    bool
				underline bool
				strike    bool
			}
			seen := make(map[int64]*textAccum)
			order := make([]int64, 0) // preserve insertion order

			for _, inlineBox := range lineBox.InlineBoxes {
				if !inlineBox.IsText {
					continue
				}
				inlineRenderNode, inlineExists := renderMap[inlineBox.NodeID]
				if !inlineExists {
					continue
				}

				accum, exists := seen[inlineBox.NodeID]
				if !exists {
					style := dlb.fontMetrics.GetTextStyleFromNode(inlineRenderNode)
					// Get font size from computed style (prefer this over tag heuristic)
					fontSize := dlb.defaultFontSize
					var textColor color.Color
					if inlineRenderNode.ComputedStyle != nil && inlineRenderNode.ComputedStyle.FontSize > 0 {
						fontSize = inlineRenderNode.ComputedStyle.FontSize
						textColor = inlineRenderNode.ComputedStyle.Color
					} else if inlineRenderNode.Parent != nil {
						if inlineRenderNode.Parent.ComputedStyle != nil && inlineRenderNode.Parent.ComputedStyle.FontSize > 0 {
							fontSize = inlineRenderNode.Parent.ComputedStyle.FontSize
							textColor = inlineRenderNode.Parent.ComputedStyle.Color
						} else {
							fontSize = dlb.fontMetrics.GetFontSize(inlineRenderNode.Parent.TagName)
						}
					}
					accum = &textAccum{
						node:      inlineRenderNode,
						box:       Rect{X: lineBox.X + inlineBox.X, Y: lineBox.Y + inlineBox.Y, Width: inlineBox.Width, Height: inlineBox.Height},
						fontSize:  fontSize,
						color:     textColor,
						bold:      style.Bold,
						italic:    style.Italic,
						underline: inlineRenderNode.ComputedStyle != nil && strings.Contains(inlineRenderNode.ComputedStyle.TextDecoration, "underline"),
						strike:    inlineRenderNode.ComputedStyle != nil && strings.Contains(inlineRenderNode.ComputedStyle.TextDecoration, "line-through"),
					}
					seen[inlineBox.NodeID] = accum
					order = append(order, inlineBox.NodeID)
				} else {
					// Extend width to include this fragment on the same line
					accum.box.Width = (inlineBox.X + inlineBox.Width) - (accum.box.X - lineBox.X)
				}
				accum.text.WriteString(inlineBox.Text)
			}

			for _, nodeID := range order {
				accum := seen[nodeID]
				text := strings.TrimSpace(accum.text.String())
				if text == "" {
					continue
				}
				if linkNode, href, ok := dlb.linkAncestor(accum.node); ok {
					cmd := &PaintCommand{
						Type:     PaintLink,
						NodeID:   linkNode.ID,
						Node:     linkNode,
						Box:      accum.box,
						LinkURL:  href,
						LinkText: text,
					}
					displayList.AddCommand(cmd)
					continue
				}
				cmd := &PaintCommand{
					Type:          PaintText,
					NodeID:        nodeID,
					Node:          accum.node,
					Box:           accum.box,
					Text:          text,
					FontSize:      accum.fontSize,
					Color:         accum.color,
					Bold:          accum.bold,
					Italic:        accum.italic,
					Underline:     accum.underline,
					Strikethrough: accum.strike,
				}
				displayList.AddCommand(cmd)
			}
		}
	} else if !isHidden {
		// No inline content - generate paint command based on node type
		if renderNode.Type == NodeTypeText {
			if _, _, ok := dlb.linkAncestor(renderNode); ok {
				return
			}
			dlb.addTextCommand(layoutBox, renderNode, displayList)
		} else if renderNode.Type == NodeTypeElement {
			dlb.addElementCommand(layoutBox, renderNode, displayList)
			if dlb.isLinkWithHref(renderNode) {
				return
			}
		}
	} else if isHidden && renderNode.Type == NodeTypeElement && renderNode.TagName == "img" {
		// For visibility:hidden images, still add transparent placeholder to preserve layout space
		// This is correct CSS behavior: visibility:hidden elements occupy space but are invisible
		dlb.addElementCommand(layoutBox, renderNode, displayList)
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
		// If this box used inline layout (LineBoxes), text node children were already
		// rendered as inline fragments above. Skip them to avoid double rendering.
		if len(layoutBox.LineBoxes) > 0 {
			childNode := renderMap[child.NodeID]
			if childNode != nil && childNode.Type == NodeTypeText {
				continue
			}
		}
		dlb.buildRecursive(child, renderMap, displayList)
	}

	// Pop clip command if needed
	if isOverflow {
		dlb.addPopClipCommand(layoutBox, renderNode, displayList)
	}
}

func (dlb *DisplayListBuilder) isLinkWithHref(node *RenderNode) bool {
	if node == nil || node.Type != NodeTypeElement || node.TagName != "a" {
		return false
	}
	href, ok := node.GetAttribute("href")
	return ok && href != ""
}

func (dlb *DisplayListBuilder) linkAncestor(node *RenderNode) (*RenderNode, string, bool) {
	for current := node; current != nil; current = current.Parent {
		if dlb.isLinkWithHref(current) {
			href, _ := current.GetAttribute("href")
			return current, href, true
		}
	}
	return nil, "", false
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

	// Get font size and color - prefer computed style values
	fontSize := dlb.defaultFontSize
	var textColor color.Color
	if renderNode.Parent != nil {
		if renderNode.Parent.ComputedStyle != nil && renderNode.Parent.ComputedStyle.FontSize > 0 {
			fontSize = renderNode.Parent.ComputedStyle.FontSize
			textColor = renderNode.Parent.ComputedStyle.Color
		} else {
			fontSize = dlb.fontMetrics.GetFontSize(renderNode.Parent.TagName)
		}
	}

	cmd := &PaintCommand{
		Type:          PaintText,
		NodeID:        layoutBox.NodeID,
		Node:          renderNode,
		Box:           layoutBox.Box,
		Text:          text,
		FontSize:      fontSize,
		Color:         textColor,
		Bold:          style.Bold,
		Italic:        style.Italic,
		Underline:     renderNode.ComputedStyle != nil && strings.Contains(renderNode.ComputedStyle.TextDecoration, "underline"),
		Strikethrough: renderNode.ComputedStyle != nil && strings.Contains(renderNode.ComputedStyle.TextDecoration, "line-through"),
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

	// For input elements, add an input paint command
	if renderNode.TagName == "input" {
		inputType, _ := renderNode.GetAttribute("type")
		inputValue, _ := renderNode.GetAttribute("value")
		placeholder, _ := renderNode.GetAttribute("placeholder")

		cmd := &PaintCommand{
			Type:        PaintInput,
			NodeID:      layoutBox.NodeID,
			Node:        renderNode,
			Box:         layoutBox.Box,
			InputType:   inputType,
			InputValue:  inputValue,
			Placeholder: placeholder,
		}
		displayList.AddCommand(cmd)
		return
	}

	// For textarea elements, add a textarea paint command
	if renderNode.TagName == "textarea" {
		inputValue := dlb.extractText(renderNode)
		placeholder, _ := renderNode.GetAttribute("placeholder")

		cmd := &PaintCommand{
			Type:        PaintTextarea,
			NodeID:      layoutBox.NodeID,
			Node:        renderNode,
			Box:         layoutBox.Box,
			InputValue:  inputValue,
			Placeholder: placeholder,
		}
		displayList.AddCommand(cmd)
		return
	}
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

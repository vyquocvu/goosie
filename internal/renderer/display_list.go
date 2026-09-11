package renderer

import (
	"image/color"
	"sort"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
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
	// PaintBackgroundImage represents a background image paint command
	PaintBackgroundImage
	// PushClip represents a command to start a clipping region
	PushClip
	// PopClip represents a command to end a clipping region
	PopClip
)

// commandNames maps PaintCommandType values to human-readable labels for
// debugging and display list inspection.
var commandNames = map[PaintCommandType]string{
	PaintText:            "Text",
	PaintRect:            "Rect",
	PaintImage:           "Image",
	PaintLink:            "Link",
	PaintBorder:          "Border",
	PaintButton:          "Button",
	PaintInput:           "Input",
	PaintTextarea:        "Textarea",
	PaintBackgroundImage: "BackgroundImage",
	PushClip:             "PushClip",
	PopClip:              "PopClip",
}

// String returns a human-readable label for the paint command type.
func (t PaintCommandType) String() string {
	if name, ok := commandNames[t]; ok {
		return name
	}
	return "Unknown"
}

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
	Commands      []*PaintCommand
	FixedCommands []*PaintCommand // Fixed and sticky commands anchored to viewport
	YBands        []YBand         // Spatial index — commands grouped by Y-range (~200px bands)
	Height        float32         // Document layout height
}

// NewDisplayList creates a new display list
func NewDisplayList() *DisplayList {
	return &DisplayList{
		Commands:      make([]*PaintCommand, 0),
		FixedCommands: make([]*PaintCommand, 0),
	}
}

// AddCommand adds a paint command to the display list
func (dl *DisplayList) AddCommand(cmd *PaintCommand) {
	dl.Commands = append(dl.Commands, cmd)
}

// Clear removes all commands from the display list
func (dl *DisplayList) Clear() {
	dl.Commands = make([]*PaintCommand, 0)
	dl.FixedCommands = nil
	dl.YBands = nil
}

// SortByZIndex reorders PaintCommands so lower z-index paints before higher z-index.
// It also rebuilds dl.YBands so the spatial index reflects the post-sort command slice.
func SortByZIndex(dl *DisplayList) {
	if dl == nil {
		return
	}
	sort.SliceStable(dl.Commands, func(i, j int) bool {
		return zIndexOf(dl.Commands[i]) < zIndexOf(dl.Commands[j])
	})
	buildYBands(dl)
}

func zIndexOf(cmd *PaintCommand) int {
	if cmd == nil || cmd.Node == nil {
		return 0
	}
	if cmd.Node.ComputedStyle != nil && cmd.Node.ComputedStyle.ZIndex != 0 {
		return cmd.Node.ComputedStyle.ZIndex
	}
	for n := cmd.Node.Parent; n != nil; n = n.Parent {
		if n.ComputedStyle != nil && n.ComputedStyle.ZIndex != 0 {
			return n.ComputedStyle.ZIndex
		}
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
	displayList.Height = layoutRoot.Box.Height

	// Build a map of render nodes by ID for quick lookup
	renderMap := dlb.buildRenderMap(renderRoot)

	// Walk the layout tree and generate paint commands
	dlb.buildRecursive(layoutRoot, renderMap, displayList)

	// Sort commands by z-index and build spatial Y-band index for viewport culling
	SortByZIndex(displayList)

	return displayList
}

// buildYBands partitions display list leaf commands into spatial Y-bands for
// efficient viewport culling. Each band groups ~200px of vertical space so
// that RenderWithViewport can skip entire groups of off-screen commands.
// Fixed and sticky commands are collected into dl.FixedCommands and are NOT
// indexed into YBands to avoid expanding all band intervals to [0, N].
func buildYBands(dl *DisplayList) {
	if dl == nil || len(dl.Commands) == 0 {
		if dl != nil {
			dl.YBands = nil
			dl.FixedCommands = nil
		}
		return
	}

	// Populate FixedCommands in stable z-index order (dl.Commands is already sorted by SortByZIndex)
	dl.FixedCommands = nil
	for _, cmd := range dl.Commands {
		if cmd != nil && isFixedOrSticky(cmd.Node) {
			dl.FixedCommands = append(dl.FixedCommands, cmd)
		}
	}

	// Find Y range of in-flow non-clip commands
	minY := float32(0)
	maxY := float32(0)
	first := true
	for _, cmd := range dl.Commands {
		if cmd == nil || cmd.Type == PushClip || cmd.Type == PopClip || isFixedOrSticky(cmd.Node) {
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
	if first || maxY <= minY {
		dl.YBands = nil
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

	// Index in-flow commands into all bands they intersect
	for i, cmd := range dl.Commands {
		if cmd == nil || cmd.Type == PushClip || cmd.Type == PopClip || isFixedOrSticky(cmd.Node) {
			continue
		}

		cmdTop := cmd.Box.Y
		cmdBottom := cmd.Box.Y + cmd.Box.Height

		startBand := int((cmdTop - minY) / bandH)
		endBand := int((cmdBottom - minY) / bandH)

		if startBand < 0 {
			startBand = 0
		}
		if startBand >= numBands {
			startBand = numBands - 1
		}
		if endBand < 0 {
			endBand = 0
		}
		if endBand >= numBands {
			endBand = numBands - 1
		}
		if endBand < startBand {
			endBand = startBand
		}

		for b := startBand; b <= endBand; b++ {
			if bands[b].CmdStart < 0 || i < bands[b].CmdStart {
				bands[b].CmdStart = i
			}
			if bands[b].CmdEnd < 0 || i+1 > bands[b].CmdEnd {
				bands[b].CmdEnd = i + 1
			}
		}
	}

	// Cleanly mark empty bands
	for b := 0; b < numBands; b++ {
		if bands[b].CmdStart < 0 {
			bands[b].CmdStart = -1
			bands[b].CmdEnd = -1
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
	if renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Display == css.DisplayAtomNone {
		return
	}

	// For visibility:hidden, skip paint commands but still process children (they maintain space in layout)
	isHidden := renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Visibility == css.VisibilityAtomHidden

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

	// Paint background image if present (drawn after background color, before borders and content)
	if !isHidden && renderNode.Type == NodeTypeElement && (layoutBox.BackgroundImage != "" || renderNode.BackgroundImageData != nil || (renderNode.ComputedStyle != nil && renderNode.ComputedStyle.BackgroundImage != "")) {
		if renderNode.TagName != "button" {
			bgBox := layoutBox.Box
			// If this is the body or html tag, ensure it covers at least the layout box bounds
			cmd := &PaintCommand{
				Type:   PaintBackgroundImage,
				NodeID: layoutBox.NodeID,
				Node:   renderNode,
				Box:    bgBox,
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
			// Coalesce contiguous inline text fragments with the same NodeID per line
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

			var currentAccum *textAccum
			isFirstInLine := true

			flushAccum := func(isLastInLine bool) {
				if currentAccum == nil {
					return
				}
				rawText := currentAccum.text.String()
				if strings.TrimSpace(rawText) != "" {
					text := rawText
					if isFirstInLine {
						text = strings.TrimLeft(text, " ")
					}
					if isLastInLine {
						text = strings.TrimRight(text, " ")
					}
					if text != "" {
						if linkNode, href, ok := dlb.linkAncestor(currentAccum.node); ok {
							cmd := &PaintCommand{
								Type:     PaintLink,
								NodeID:   linkNode.ID,
								Node:     linkNode,
								Box:      currentAccum.box,
								LinkURL:  href,
								LinkText: text,
							}
							displayList.AddCommand(cmd)
						} else {
							cmd := &PaintCommand{
								Type:          PaintText,
								NodeID:        currentAccum.node.ID,
								Node:          currentAccum.node,
								Box:           currentAccum.box,
								Text:          text,
								FontSize:      currentAccum.fontSize,
								Color:         currentAccum.color,
								Bold:          currentAccum.bold,
								Italic:        currentAccum.italic,
								Underline:     currentAccum.underline,
								Strikethrough: currentAccum.strike,
							}
							displayList.AddCommand(cmd)
						}
						isFirstInLine = false
					}
				}
				currentAccum = nil
			}

			for _, inlineBox := range lineBox.InlineBoxes {
				if !inlineBox.IsText {
					flushAccum(false)
					inlineRenderNode, inlineExists := renderMap[inlineBox.NodeID]
					if inlineExists {
						// Create a temporary layout box with the absolute coordinates of the inline box
						tempBox := &LayoutBox{
							NodeID: inlineBox.NodeID,
							Box:    Rect{X: lineBox.X + inlineBox.X, Y: lineBox.Y + inlineBox.Y, Width: inlineBox.Width, Height: inlineBox.Height},
						}
						dlb.addElementCommand(tempBox, inlineRenderNode, displayList)

						// If this inline element is wrapped in a link, we should also add a link command
						if linkNode, href, ok := dlb.linkAncestor(inlineRenderNode); ok {
							linkCmd := &PaintCommand{
								Type:     PaintLink,
								NodeID:   linkNode.ID,
								Node:     linkNode,
								Box:      tempBox.Box,
								LinkURL:  href,
								LinkText: "",
							}
							displayList.AddCommand(linkCmd)
						} else if inlineRenderNode.TagName == "a" {
							href, hasHref := inlineRenderNode.GetAttribute("href")
							if hasHref && href != "" {
								linkCmd := &PaintCommand{
									Type:     PaintLink,
									NodeID:   inlineRenderNode.ID,
									Node:     inlineRenderNode,
									Box:      tempBox.Box,
									LinkURL:  href,
									LinkText: "",
								}
								displayList.AddCommand(linkCmd)
							}
						}
					}
					continue
				}

				inlineRenderNode, inlineExists := renderMap[inlineBox.NodeID]
				if !inlineExists {
					continue
				}

				if currentAccum != nil && currentAccum.node.ID == inlineBox.NodeID {
					// Extend width to include this fragment on the same line
					currentAccum.box.Width = (inlineBox.X + inlineBox.Width) - (currentAccum.box.X - lineBox.X)
					currentAccum.text.WriteString(inlineBox.Text)
				} else {
					flushAccum(false)

					style := dlb.fontMetrics.GetTextStyleFromNode(inlineRenderNode)
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

					currentAccum = &textAccum{
						node:      inlineRenderNode,
						box:       Rect{X: lineBox.X + inlineBox.X, Y: lineBox.Y + inlineBox.Y, Width: inlineBox.Width, Height: inlineBox.Height},
						fontSize:  fontSize,
						color:     textColor,
						bold:      style.Bold,
						italic:    style.Italic,
						underline: inlineRenderNode.ComputedStyle != nil && inlineRenderNode.ComputedStyle.TextDecoration == css.TextDecorationAtomUnderline,
						strike:    inlineRenderNode.ComputedStyle != nil && inlineRenderNode.ComputedStyle.TextDecoration == css.TextDecorationAtomLineThrough,
					}
					currentAccum.text.WriteString(inlineBox.Text)
				}
			}

			flushAccum(true)
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
	if renderNode.ComputedStyle != nil && (renderNode.ComputedStyle.Overflow == css.OverflowAtomHidden || renderNode.ComputedStyle.Overflow == css.OverflowAtomScroll || renderNode.ComputedStyle.Overflow == css.OverflowAtomAuto) {
		isOverflow = true
		// Push clip command
		dlb.addPushClipCommand(layoutBox, renderNode, displayList)
	}

	// Process children — sort by z-index only when at least one child has a
	// non-zero z-index (the common case is all-zero, so this avoids an
	// allocation and sort on every element).
	if !hasNonZeroZIndex(layoutBox, renderMap) {
		for _, child := range layoutBox.Children {
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
	} else {
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
			if len(layoutBox.LineBoxes) > 0 {
				childNode := renderMap[child.NodeID]
				if childNode != nil && childNode.Type == NodeTypeText {
					continue
				}
			}
			dlb.buildRecursive(child, renderMap, displayList)
		}
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
		ClipOverflow: renderNode.ComputedStyle.Overflow.String(),
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
		Underline:     renderNode.ComputedStyle != nil && renderNode.ComputedStyle.TextDecoration == css.TextDecorationAtomUnderline,
		Strikethrough: renderNode.ComputedStyle != nil && renderNode.ComputedStyle.TextDecoration == css.TextDecorationAtomLineThrough,
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

	// For image and svg elements, add an image paint command
	if renderNode.TagName == "img" || renderNode.TagName == "svg" {
		// Check visibility
		if renderNode.ComputedStyle != nil && renderNode.ComputedStyle.Visibility == css.VisibilityAtomHidden {
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
		if renderNode.TagName == "svg" {
			src = "inline-svg"
		}

		if src != "" || alt != "" || renderNode.ImageData != nil {
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
		if strings.EqualFold(strings.TrimSpace(inputType), "hidden") {
			return
		}
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
	if node.ComputedStyle != nil && node.ComputedStyle.Display == css.DisplayAtomNone {
		return ""
	}
	switch node.TagName {
	case "style", "script", "noscript", "template":
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

// hasNonZeroZIndex reports whether any direct child of layoutBox has a
// non-zero z-index. It is used to skip the children copy+sort in the
// common case where no child participates in z-ordering.
func hasNonZeroZIndex(layoutBox *LayoutBox, renderMap map[int64]*RenderNode) bool {
	for _, child := range layoutBox.Children {
		if node, ok := renderMap[child.NodeID]; ok && node != nil {
			if node.ComputedStyle != nil && node.ComputedStyle.ZIndex != 0 {
				return true
			}
		}
	}
	return false
}

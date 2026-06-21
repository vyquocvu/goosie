package renderer

import (
	"image/color"
	"strings"
)

// LayoutEngine handles layout calculations for render nodes
type LayoutEngine struct {
	canvasWidth  float32
	canvasHeight float32

	// Default font sizes for headings and text
	defaultFontSize float32
	lineHeight      float32

	// nodeMap maps RenderNode IDs to their corresponding LayoutBoxes
	nodeMap map[int64]*LayoutBox

	// fontMetrics provides accurate text measurement
	fontMetrics *FontMetrics

	// inlineLayoutEngine handles inline layout
	inlineLayoutEngine *InlineLayoutEngine

	// flexLayoutEngine handles flexbox layout
	flexLayoutEngine *FlexLayoutEngine

	// gridLayoutEngine handles grid layout
	gridLayoutEngine *GridLayoutEngine
}

// NewLayoutEngine creates a new layout engine
func NewLayoutEngine(width, height float32) *LayoutEngine {
	defaultSize := float32(16.0)
	fontMetrics := NewFontMetrics(defaultSize)
	return &LayoutEngine{
		canvasWidth:        width,
		canvasHeight:       height,
		defaultFontSize:    defaultSize,
		lineHeight:         1.5,
		nodeMap:            make(map[int64]*LayoutBox),
		fontMetrics:        fontMetrics,
		inlineLayoutEngine: NewInlineLayoutEngine(fontMetrics, defaultSize),
		flexLayoutEngine:   NewFlexLayoutEngine(fontMetrics),
		gridLayoutEngine:   NewGridLayoutEngine(fontMetrics),
	}
}

// Layout performs layout calculations on the render tree and returns a layout tree
// This is the new API that produces a separate layout tree
func (le *LayoutEngine) ComputeLayout(root *RenderNode) *LayoutBox {
	if root == nil {
		return nil
	}

	// Clear previous mappings
	le.nodeMap = make(map[int64]*LayoutBox)

	// Build layout tree from render tree
	layoutRoot := le.buildLayoutBox(root, 0, 0, le.canvasWidth)

	return layoutRoot
}

// buildLayoutBox creates a LayoutBox for a RenderNode and computes its layout
func (le *LayoutEngine) buildLayoutBox(node *RenderNode, x, y, availableWidth float32) *LayoutBox {
	if node == nil {
		return nil
	}

	layoutBox := NewLayoutBox(node.ID)
	le.nodeMap[node.ID] = layoutBox

	// Determine display type from computed style
	if node.ComputedStyle != nil && node.ComputedStyle.Display != "" {
		switch node.ComputedStyle.Display {
		case "block":
			layoutBox.Display = DisplayBlock
		case "inline":
			layoutBox.Display = DisplayInline
		case "none":
			layoutBox.Display = DisplayNone
			return nil // Don't layout non-displayed elements
		case "flex":
			layoutBox.Display = DisplayFlex
		case "grid":
			layoutBox.Display = DisplayGrid
		case "inline-block":
			layoutBox.Display = DisplayInlineBlock
		default:
			layoutBox.Display = DisplayInline // Default for unknown values
		}
	} else if node.Type == NodeTypeElement {
		if node.IsBlock() {
			layoutBox.Display = DisplayBlock
		} else {
			layoutBox.Display = DisplayInline
		}
	} else {
		layoutBox.Display = DisplayInline // Text nodes are inline
	}

	// Apply box model properties from computed style
	le.applyBoxModel(node, layoutBox)

	// Compute layout
	var currentY float32
	if node.TagName == "table" {
		layoutBox.Display = DisplayGrid
		currentY = le.buildTableLayoutBox(node, layoutBox, x, y, availableWidth)
	} else {
		currentY = le.computeLayoutBox(node, layoutBox, x, y, availableWidth)
	}

	// Update height based on children
	// currentY tracks the bottom edge of content/padding
	// layoutBox.Box.Y is the top edge
	calculatedHeight := currentY - layoutBox.Box.Y

	// Check for explicit height
	if node.ComputedStyle != nil && node.ComputedStyle.Height != "" && node.ComputedStyle.Height != "auto" {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}
		explicitHeight := parseLengthWithViewport(node.ComputedStyle.Height, fontSize, le.canvasWidth, le.canvasHeight, le.canvasHeight)
		if explicitHeight > 0 {
			// Include padding and borders if box-sizing is content-box (default)
			layoutBox.Box.Height = explicitHeight + layoutBox.PaddingTop + layoutBox.PaddingBottom + layoutBox.BorderTopWidth + layoutBox.BorderBottomWidth
		} else {
			layoutBox.Box.Height = calculatedHeight
		}
	} else if node.TagName == "img" {
		// For img elements, fall back to HTML height attribute if CSS height is not set
		if hAttr, ok := node.GetAttribute("height"); ok && hAttr != "" {
			if v := parseLength(hAttr, le.defaultFontSize); v > 0 {
				layoutBox.Box.Height = v
			} else {
				layoutBox.Box.Height = calculatedHeight
			}
		} else {
			layoutBox.Box.Height = calculatedHeight
		}
	} else {
		layoutBox.Box.Height = calculatedHeight
	}

	if layoutBox.Box.Height < 0 {
		layoutBox.Box.Height = 0
	}

	// Apply CSS positioning: copy position value and override coordinates for absolute/fixed
	if node.ComputedStyle != nil && node.ComputedStyle.Position != "" {
		layoutBox.Position = node.ComputedStyle.Position
		pos := node.ComputedStyle.Position
		if pos == "absolute" || pos == "fixed" {
			fontSize := le.defaultFontSize
			if node.ComputedStyle.FontSize > 0 {
				fontSize = node.ComputedStyle.FontSize
			}

			var ancestorX, ancestorY float32
			containerWidth := le.canvasWidth
			containerHeight := le.canvasHeight

			if pos == "absolute" {
				curr := node.Parent
				for curr != nil {
					if curr.ComputedStyle != nil && curr.ComputedStyle.Position != "" && curr.ComputedStyle.Position != "static" {
						if ancestorBox, ok := le.nodeMap[curr.ID]; ok {
							ancestorX = ancestorBox.Box.X
							ancestorY = ancestorBox.Box.Y
							containerWidth = ancestorBox.Box.Width
							containerHeight = ancestorBox.Box.Height
							break
						}
					}
					curr = curr.Parent
				}
			}

			// Store old coordinates to calculate delta
			oldX := layoutBox.Box.X
			oldY := layoutBox.Box.Y

			newY := oldY
			newX := oldX

			if node.ComputedStyle.Top != "" && node.ComputedStyle.Top != "auto" {
				newY = ancestorY + parseLength(node.ComputedStyle.Top, fontSize)
			} else if node.ComputedStyle.Bottom != "" && node.ComputedStyle.Bottom != "auto" {
				newY = ancestorY + containerHeight - layoutBox.Box.Height - parseLength(node.ComputedStyle.Bottom, fontSize)
			}

			if node.ComputedStyle.Left != "" && node.ComputedStyle.Left != "auto" {
				newX = ancestorX + parseLength(node.ComputedStyle.Left, fontSize)
			} else if node.ComputedStyle.Right != "" && node.ComputedStyle.Right != "auto" {
				newX = ancestorX + containerWidth - layoutBox.Box.Width - parseLength(node.ComputedStyle.Right, fontSize)
			}

			deltaX := newX - oldX
			deltaY := newY - oldY

			if deltaX != 0 || deltaY != 0 {
				layoutBox.Box.X = newX
				layoutBox.Box.Y = newY
				le.shiftLayoutBox(layoutBox, deltaX, deltaY)
			}
		}
	}

	return layoutBox
}

// shiftLayoutBox recursively offsets the coordinates of a layout box, its children, and its inline line boxes.
func (le *LayoutEngine) shiftLayoutBox(box *LayoutBox, deltaX, deltaY float32) {
	if box == nil || (deltaX == 0 && deltaY == 0) {
		return
	}

	for _, child := range box.Children {
		child.Box.X += deltaX
		child.Box.Y += deltaY
		le.shiftLayoutBox(child, deltaX, deltaY)
	}

	for _, line := range box.LineBoxes {
		line.X += deltaX
		line.Y += deltaY
	}
}

// buildTableLayoutBox creates a LayoutBox for a table node and computes its layout
func (le *LayoutEngine) buildTableLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32) float32 {
	// x, y and availableWidth already account for margins from computeLayoutBox
	// availableWidth here is the border-box width

	// Reduce available width by borders (if we supported them fully) - for now just assume availableWidth is content width
	contentWidth := availableWidth

	// Ensure we don't proceed with negative width
	if contentWidth < 0 {
		contentWidth = 0
	}

	layoutBox.Box.X = x
	layoutBox.Box.Y = y
	layoutBox.Box.Width = contentWidth

	// 1. Flatten table to find cells and max columns
	maxCols := 0

	// Helper to traverse
	var traverse func(n *RenderNode)
	traverse = func(n *RenderNode) {
		for _, child := range n.Children {
			if child.TagName == "tr" {
				colCount := 0
				for _, cell := range child.Children {
					if cell.TagName == "td" || cell.TagName == "th" {
						colCount++
					}
				}
				if colCount > maxCols {
					maxCols = colCount
				}
			} else if child.TagName == "thead" || child.TagName == "tbody" || child.TagName == "tfoot" {
				traverse(child)
			}
		}
	}
	traverse(node)

	if maxCols == 0 {
		return y + layoutBox.PaddingTop + layoutBox.PaddingBottom
	}

	// 2. Set grid template columns
	var colsBuilder strings.Builder
	for i := 0; i < maxCols; i++ {
		if i > 0 {
			colsBuilder.WriteString(" ")
		}
		colsBuilder.WriteString("auto")
	}
	layoutBox.GridTemplateColumns = colsBuilder.String()

	// 3. Create layout boxes for cells
	currentRow := 1

	var buildCells func(n *RenderNode)
	buildCells = func(n *RenderNode) {
		for _, child := range n.Children {
			if child.TagName == "tr" {
				currentCol := 1
				// Try to get row background color
				var trBgColor color.Color
				if child.ComputedStyle != nil {
					trBgColor = child.ComputedStyle.BackgroundColor
				}

				for _, cell := range child.Children {
					if cell.TagName == "td" || cell.TagName == "th" {
						// Create cell box
						cellBox := le.buildLayoutBox(cell, 0, 0, contentWidth)

						if cellBox != nil {
							cellBox.GridColumnStart = currentCol
							cellBox.GridColumnEnd = currentCol + 1
							cellBox.GridRowStart = currentRow
							cellBox.GridRowEnd = currentRow + 1

							// Transmit TR background to cell if cell has none
							if trBgColor != nil && (cellBox.BackgroundColor == nil || cellBox.BackgroundColor == color.Transparent) {
								cellBox.BackgroundColor = trBgColor
							}

							layoutBox.AddChild(cellBox)
						}
						currentCol++
					}
				}
				currentRow++
			} else if child.TagName == "thead" || child.TagName == "tbody" || child.TagName == "tfoot" {
				buildCells(child)
			}
		}
	}
	buildCells(node)

	// 4. Run grid layout
	le.gridLayoutEngine.LayoutTable(layoutBox)

	// Calculate height
	maxY := y + layoutBox.PaddingTop
	for _, child := range layoutBox.Children {
		childBottom := child.Box.Y + child.Box.Height + child.MarginBottom
		if childBottom > maxY {
			maxY = childBottom
		}
	}

	// Ensure we account for explicit height if set (ignoring for now to allow auto-height)

	return maxY + layoutBox.PaddingBottom
}

// applyBoxModel applies box model properties (margin, padding, border) from computed style to layout box
func (le *LayoutEngine) applyBoxModel(node *RenderNode, layoutBox *LayoutBox) {
	if node.ComputedStyle == nil {
		return
	}

	// Get font size for em/rem calculations
	fontSize := le.defaultFontSize
	if node.ComputedStyle.FontSize > 0 {
		fontSize = node.ComputedStyle.FontSize
	}

	// Apply margins
	layoutBox.MarginTop = parseLength(node.ComputedStyle.MarginTop, fontSize)
	layoutBox.MarginRight = parseLength(node.ComputedStyle.MarginRight, fontSize)
	layoutBox.MarginBottom = parseLength(node.ComputedStyle.MarginBottom, fontSize)
	layoutBox.MarginLeft = parseLength(node.ComputedStyle.MarginLeft, fontSize)

	// Apply padding
	layoutBox.PaddingTop = parseLength(node.ComputedStyle.PaddingTop, fontSize)
	layoutBox.PaddingRight = parseLength(node.ComputedStyle.PaddingRight, fontSize)
	layoutBox.PaddingBottom = parseLength(node.ComputedStyle.PaddingBottom, fontSize)
	layoutBox.PaddingLeft = parseLength(node.ComputedStyle.PaddingLeft, fontSize)

	// Apply borders
	layoutBox.BorderTopWidth = parseLength(node.ComputedStyle.BorderTopWidth, fontSize)
	layoutBox.BorderRightWidth = parseLength(node.ComputedStyle.BorderRightWidth, fontSize)
	layoutBox.BorderBottomWidth = parseLength(node.ComputedStyle.BorderBottomWidth, fontSize)
	layoutBox.BorderLeftWidth = parseLength(node.ComputedStyle.BorderLeftWidth, fontSize)

	layoutBox.BorderTopStyle = node.ComputedStyle.BorderTopStyle
	layoutBox.BorderRightStyle = node.ComputedStyle.BorderRightStyle
	layoutBox.BorderBottomStyle = node.ComputedStyle.BorderBottomStyle
	layoutBox.BorderLeftStyle = node.ComputedStyle.BorderLeftStyle

	layoutBox.BorderTopColor = node.ComputedStyle.BorderTopColor
	layoutBox.BorderRightColor = node.ComputedStyle.BorderRightColor
	layoutBox.BorderBottomColor = node.ComputedStyle.BorderBottomColor
	layoutBox.BorderLeftColor = node.ComputedStyle.BorderLeftColor
	
	// Apply background color
	layoutBox.BackgroundColor = node.ComputedStyle.BackgroundColor
}

// computeLayoutBox computes the layout for a single box
func (le *LayoutEngine) computeLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32) float32 {
	// Account for margins
	marginLeft := layoutBox.MarginLeft
	marginRight := layoutBox.MarginRight

	// Check for explicit width
	explicitWidth := float32(-1)
	if node.ComputedStyle != nil && node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}
		explicitWidth = parseLengthWithViewport(node.ComputedStyle.Width, fontSize, le.canvasWidth, le.canvasHeight, availableWidth)
	}
	// For img elements, fall back to HTML width attribute if CSS width is not set
	if node.TagName == "img" && explicitWidth < 0 {
		if wAttr, ok := node.GetAttribute("width"); ok && wAttr != "" {
			if v := parseLength(wAttr, le.defaultFontSize); v > 0 {
				explicitWidth = v
			}
		}
	}

	// Handle margin: auto for block-level elements
	if node.IsBlock() && explicitWidth >= 0 && explicitWidth < availableWidth {
		if node.ComputedStyle != nil && (node.ComputedStyle.MarginLeft == "auto" || node.ComputedStyle.MarginRight == "auto") {
			remainingSpace := availableWidth - explicitWidth
			if node.ComputedStyle.MarginLeft == "auto" && node.ComputedStyle.MarginRight == "auto" {
				// Center
				marginLeft = remainingSpace / 2
				marginRight = remainingSpace / 2
			} else if node.ComputedStyle.MarginLeft == "auto" {
				// Align right
				marginLeft = remainingSpace
				marginRight = 0
			} else {
				// Align left (default)
				marginLeft = 0
				marginRight = remainingSpace
			}
			// Update layout box margins
			layoutBox.MarginLeft = marginLeft
			layoutBox.MarginRight = marginRight
		}
	}

	x += marginLeft
	y += layoutBox.MarginTop

	// Calculate width
	width := availableWidth - (marginLeft + marginRight)
	if explicitWidth >= 0 {
		width = explicitWidth
	}

	if width < 0 {
		width = 0
	}

	layoutBox.Box.X = x
	layoutBox.Box.Y = y
	layoutBox.Box.Width = width

	currentY := y

	if node.Type == NodeTypeText {
		// Layout text node
		currentY = le.computeTextLayout(node, layoutBox, x, y, width)
	} else if node.Type == NodeTypeElement {
		// Layout element node
		currentY = le.computeElementLayout(node, layoutBox, x, y, width)
	}

	return currentY
}

// computeTextLayout computes layout for text nodes
func (le *LayoutEngine) computeTextLayout(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32) float32 {
	// Get font size from computed style
	fontSize := le.defaultFontSize
	if node.Parent != nil && node.Parent.ComputedStyle != nil && node.Parent.ComputedStyle.FontSize > 0 {
		fontSize = node.Parent.ComputedStyle.FontSize
	}

	// Get text style from parent hierarchy
	style := le.fontMetrics.GetTextStyleFromNode(node)

	// Calculate text dimensions using font metrics
	text := strings.TrimSpace(node.Text)
	if text == "" {
		layoutBox.Box.Height = 0
		return y
	}

	// Measure text with wrapping
	letterSpacing := float32(0)
	if node.Parent != nil && node.Parent.ComputedStyle != nil {
		letterSpacing = node.Parent.ComputedStyle.LetterSpacing
	}
	metrics := le.fontMetrics.MeasureTextWithWrapping(text, fontSize, style, letterSpacing, availableWidth)

	layoutBox.Box.Height = metrics.Height

	return y + metrics.Height
}

// computeElementLayout computes layout for element nodes
func (le *LayoutEngine) computeElementLayout(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32) float32 {
	currentY := y

	// Add border top offset before padding
	currentY += layoutBox.BorderTopWidth

	// Add padding to the starting position
	currentY += layoutBox.PaddingTop
	childX := x + layoutBox.BorderLeftWidth + layoutBox.PaddingLeft

	// Reduce available width by horizontal borders and padding
	contentWidth := availableWidth - layoutBox.BorderLeftWidth - layoutBox.PaddingLeft - layoutBox.PaddingRight - layoutBox.BorderRightWidth
	if contentWidth < 0 {
		contentWidth = 0
	}

	// Layout children
	childY := currentY

	// Check if this block element contains inline content
	// Block elements like p, div can contain inline content
	if layoutBox.Display == DisplayFlex {
		// Use flexbox layout engine for flex containers
		le.flexLayoutEngine.LayoutFlexContainer(node, layoutBox, le.buildLayoutBox)

		// Calculate childY based on laid out flex items
		for _, child := range layoutBox.Children {
			endY := child.Box.Y + child.Box.Height + child.MarginBottom
			if endY > childY {
				childY = endY
			}
		}
	} else if layoutBox.Display == DisplayGrid {
		// Use grid layout engine for grid containers
		le.gridLayoutEngine.LayoutGridContainer(node, layoutBox, le.buildLayoutBox)

		// Calculate childY based on laid out items (similar to Block/Flex)
		// Grid layout sets height on parentBox too, but let's ensure childY reflects content
		for _, child := range layoutBox.Children {
			endY := child.Box.Y + child.Box.Height + child.MarginBottom
			if endY > childY {
				childY = endY
			}
		}
	} else if node.IsBlock() && le.hasInlineContent(node) {
		// Use inline layout for the children
		wsMode := le.whiteSpaceModeForNode(node)
		lines, totalHeight := le.inlineLayoutEngine.LayoutInlineContent(
			node, childX, currentY, contentWidth, wsMode,
		)

		// Store line boxes in the layout box
		layoutBox.LineBoxes = lines

		// DO NOT create child LayoutBox instances for inline boxes
		// The LineBoxes contain all the information needed for rendering
		// However, we still need to populate nodeMap for GetLayoutBox to work
		processedNodeIDs := make(map[int64]bool)
		for _, line := range lines {
			for _, inlineBox := range line.InlineBoxes {
				if !processedNodeIDs[inlineBox.NodeID] {
					processedNodeIDs[inlineBox.NodeID] = true
					// Map the inline node ID to the parent layout box
					// This allows GetLayoutBox to find a box for inline nodes
					le.nodeMap[inlineBox.NodeID] = layoutBox
				}
			}
		}

		childY = currentY + totalHeight
	} else if node.IsBlock() {
		// Block elements: stack children vertically (when no inline content)
		// Check if element has intrinsic dimensions (e.g. input, button, textarea)
		if node.TagName == "input" {
			// Default height for input
			inputHeight := float32(30) + layoutBox.PaddingTop + layoutBox.PaddingBottom
			childY = currentY + inputHeight
		} else if node.TagName == "button" && !le.hasInlineContent(node) {
			// Default height for empty button
			buttonHeight := float32(30) + layoutBox.PaddingTop + layoutBox.PaddingBottom
			childY = currentY + buttonHeight
		} else if node.TagName == "textarea" {
			// Default height for textarea
			textareaHeight := float32(60) + layoutBox.PaddingTop + layoutBox.PaddingBottom
			childY = currentY + textareaHeight
		} else {
			for _, child := range node.Children {
				childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth)
				if childLayoutBox != nil {
					layoutBox.AddChild(childLayoutBox)
					if childLayoutBox.Position != "absolute" && childLayoutBox.Position != "fixed" {
						childY = childLayoutBox.Box.Y + childLayoutBox.Box.Height + childLayoutBox.MarginBottom
					}
				}
			}
		}
	} else {
		// Inline elements: use inline layout engine
		if le.hasInlineContent(node) {
			wsMode := le.whiteSpaceModeForNode(node)
			lines, totalHeight := le.inlineLayoutEngine.LayoutInlineContent(
				node, childX, currentY, contentWidth, wsMode,
			)

			// Store line boxes in the layout box
			layoutBox.LineBoxes = lines

			// DO NOT create child LayoutBox instances for inline boxes
			// The LineBoxes contain all the information needed for rendering
			// However, we still need to populate nodeMap for GetLayoutBox to work
			processedNodeIDs := make(map[int64]bool)
			for _, line := range lines {
				for _, inlineBox := range line.InlineBoxes {
					if !processedNodeIDs[inlineBox.NodeID] {
						processedNodeIDs[inlineBox.NodeID] = true
						// Map the inline node ID to the parent layout box
						// This allows GetLayoutBox to find a box for inline nodes
						le.nodeMap[inlineBox.NodeID] = layoutBox
					}
				}
			}

			childY = currentY + totalHeight
		} else {
			// Check if element has intrinsic dimensions (e.g. input, button, textarea)
			// These might have no children (void tags or empty) but need rendering size
			if node.TagName == "input" {
				// Default height for input
				inputHeight := float32(30) + layoutBox.PaddingTop + layoutBox.PaddingBottom
				childY = currentY + inputHeight
			} else if node.TagName == "button" {
				// Default height for button if empty (though usually has text)
				buttonHeight := float32(30) + layoutBox.PaddingTop + layoutBox.PaddingBottom
				childY = currentY + buttonHeight
			} else if node.TagName == "textarea" {
				// Default height for textarea
				textareaHeight := float32(60) + layoutBox.PaddingTop + layoutBox.PaddingBottom
				childY = currentY + textareaHeight
			} else {
				// Fallback for empty inline elements using Block layout (e.g. empty div)
				for _, child := range node.Children {
					childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth)
					if childLayoutBox != nil {
						layoutBox.AddChild(childLayoutBox)
						if childLayoutBox.Position != "absolute" && childLayoutBox.Position != "fixed" {
							childY = childLayoutBox.Box.Y + childLayoutBox.Box.Height + childLayoutBox.MarginBottom
						}
					}
				}
			}
		}
	}

	// Add bottom padding
	childY += layoutBox.PaddingBottom

	// Add border bottom offset after padding
	childY += layoutBox.BorderBottomWidth

	return childY
}

// GetLayoutBox returns the LayoutBox for a given RenderNode ID
func (le *LayoutEngine) GetLayoutBox(nodeID int64) *LayoutBox {
	return le.nodeMap[nodeID]
}

// HitTest performs hit testing on the layout tree
// Returns the node ID of the deepest layout box containing the point (x, y)
// Returns 0 if no box contains the point
func (le *LayoutEngine) HitTest(layoutRoot *LayoutBox, x, y float32) int64 {
	if layoutRoot == nil {
		return 0
	}

	return le.hitTestRecursive(layoutRoot, x, y)
}

// hitTestRecursive recursively searches for the deepest box containing (x, y)
func (le *LayoutEngine) hitTestRecursive(box *LayoutBox, x, y float32) int64 {
	if !box.Contains(x, y) {
		return 0
	}

	// Check children first (depth-first search for deepest match)
	for _, child := range box.Children {
		if hitID := le.hitTestRecursive(child, x, y); hitID != 0 {
			return hitID
		}
	}

	// If no child contains the point, return this box's node ID
	return box.NodeID
}

// Layout performs layout calculations on the render tree (deprecated - use ComputeLayout)
// Kept for backward compatibility
func (le *LayoutEngine) Layout(root *RenderNode) {
	if root == nil {
		return
	}

	// Start layout from top-left with full canvas width
	le.layoutNode(root, 0, 0, le.canvasWidth)
}

// layoutNode performs layout calculation for a single node and its children
func (le *LayoutEngine) layoutNode(node *RenderNode, x, y, availableWidth float32) float32 {
	if node == nil {
		return y
	}

	currentY := y

	if node.Type == NodeTypeText {
		// Layout text node
		currentY = le.layoutTextNode(node, x, y, availableWidth)
	} else if node.Type == NodeTypeElement {
		// Layout element node
		currentY = le.layoutElementNode(node, x, y, availableWidth)
	}

	return currentY
}

// layoutTextNode handles layout for text nodes
func (le *LayoutEngine) layoutTextNode(node *RenderNode, x, y, availableWidth float32) float32 {
	// Get font size from parent element
	fontSize := le.defaultFontSize
	if node.Parent != nil {
		fontSize = le.fontMetrics.GetFontSize(node.Parent.TagName)
	}

	// Get text style from parent hierarchy
	style := le.fontMetrics.GetTextStyleFromNode(node)

	// Calculate text dimensions using font metrics
	text := strings.TrimSpace(node.Text)
	if text == "" {
		return y
	}

	// Measure text with wrapping
	letterSpacing := float32(0)
	if node.Parent != nil && node.Parent.ComputedStyle != nil {
		letterSpacing = node.Parent.ComputedStyle.LetterSpacing
	}
	metrics := le.fontMetrics.MeasureTextWithWrapping(text, fontSize, style, letterSpacing, availableWidth)

	node.Box.X = x
	node.Box.Y = y
	node.Box.Width = availableWidth
	node.Box.Height = metrics.Height

	return y + metrics.Height
}

// layoutElementNode handles layout for element nodes (legacy path used by layoutNode)
func (le *LayoutEngine) layoutElementNode(node *RenderNode, x, y, availableWidth float32) float32 {
	node.Box.X = x
	node.Box.Y = y
	node.Box.Width = availableWidth

	currentY := y
	childY := currentY

	if node.IsBlock() {
		for _, child := range node.Children {
			childY = le.layoutNode(child, x, childY, availableWidth)
		}
	} else {
		childX := x
		for _, child := range node.Children {
			if child.Type == NodeTypeText {
				childY = le.layoutTextNode(child, childX, currentY, availableWidth-childX+x)
			} else {
				childY = le.layoutNode(child, childX, childY, availableWidth)
			}
		}
	}

	node.Box.Height = childY - currentY
	return childY
}

// getFontSize returns the font size for an element (delegates to fontMetrics)
func (le *LayoutEngine) getFontSize(tagName string) float32 {
	return le.fontMetrics.GetFontSize(tagName)
}


// whiteSpaceModeForNode selects white space handling based on element type
func (le *LayoutEngine) whiteSpaceModeForNode(node *RenderNode) WhiteSpaceMode {
	if node == nil {
		return WhiteSpaceNormal
	}
	switch node.TagName {
	case "pre":
		return WhiteSpacePre
	case "code":
		return WhiteSpaceNoWrap
	default:
		return WhiteSpaceNormal
	}
}

// hasInlineContent checks if a node has inline content (text or inline children)
func (le *LayoutEngine) hasInlineContent(node *RenderNode) bool {
	return le.hasInlineContentRecursive(node)
}

// hasInlineContentRecursive recursively checks for inline content
func (le *LayoutEngine) hasInlineContentRecursive(node *RenderNode) bool {
	for _, child := range node.Children {
		// Skip display:none elements
		if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
			continue
		}
		// Skip absolute/fixed positioned elements from inline content check
		if child.ComputedStyle != nil && (child.ComputedStyle.Position == "absolute" || child.ComputedStyle.Position == "fixed") {
			continue
		}

		if child.Type == NodeTypeText {
			// Check if text is not empty after trimming
			if strings.TrimSpace(child.Text) != "" {
				return true
			}
		} else if !child.IsBlock() {
			// Inline element - check its children too
			if le.hasInlineContentRecursive(child) {
				return true
			}
		}
	}
	return false
}

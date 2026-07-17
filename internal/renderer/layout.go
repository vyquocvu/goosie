package renderer

import (
	"image/color"
	"strconv"
	"strings"
	"sync"
)

// LayoutEngine handles layout calculations for render nodes
type LayoutEngine struct {
	mu           sync.Mutex
	canvasWidth  float32
	canvasHeight float32

	// Default font sizes for headings and text
	defaultFontSize float32
	lineHeight      float32

	// nodeMap maps RenderNode IDs to their corresponding LayoutBoxes
	nodeMap   map[int64]*LayoutBox
	nodeMapMu sync.RWMutex

	// fontMetrics provides accurate text measurement
	fontMetrics *FontMetrics

	// We create layout engines per compute cycle to be fully thread safe.
	// We no longer keep them as state.
}

// NewLayoutEngine creates a new layout engine
func NewLayoutEngine(width, height float32) *LayoutEngine {
	defaultSize := float32(16.0)
	fontMetrics := NewFontMetrics(defaultSize)
	return &LayoutEngine{
		canvasWidth:     width,
		canvasHeight:    height,
		defaultFontSize: defaultSize,
		lineHeight:      1.5,
		nodeMap:         make(map[int64]*LayoutBox),
		fontMetrics:     fontMetrics,
	}
}

// Layout performs layout calculations on the render tree and returns a layout tree
// This is the new API that produces a separate layout tree
func (le *LayoutEngine) ComputeLayout(root *RenderNode) *LayoutBox {
	le.mu.Lock()
	defer le.mu.Unlock()

	if root == nil {
		return nil
	}

	// Clear previous mappings
	le.nodeMapMu.Lock()
	le.nodeMap = make(map[int64]*LayoutBox)
	le.nodeMapMu.Unlock()

	// Build layout tree from render tree
	inlineLayoutEngine := NewInlineLayoutEngine(le.fontMetrics, le.defaultFontSize)
	flexLayoutEngine := NewFlexLayoutEngine(le.fontMetrics)
	gridLayoutEngine := NewGridLayoutEngine(le.fontMetrics)
	layoutRoot := le.buildLayoutBox(root, 0, 0, le.canvasWidth, nil, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)

	return layoutRoot
}

// buildLayoutBox creates a LayoutBox for a RenderNode and computes its layout
func (le *LayoutEngine) buildLayoutBox(node *RenderNode, x, y, availableWidth float32, floatCtx *FloatContext, inlineLayoutEngine *InlineLayoutEngine, flexLayoutEngine *FlexLayoutEngine, gridLayoutEngine *GridLayoutEngine) *LayoutBox {
	if node == nil {
		return nil
	}

	layoutBox := NewLayoutBox(node.ID)
	le.nodeMapMu.Lock()
	le.nodeMap[node.ID] = layoutBox
	le.nodeMapMu.Unlock()

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
		currentY = le.buildTableLayoutBox(node, layoutBox, x, y, availableWidth, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
	} else {
		currentY = le.computeLayoutBox(node, layoutBox, x, y, availableWidth, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
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
			if node.ComputedStyle.BoxSizing == "border-box" {
				layoutBox.Box.Height = explicitHeight
			} else {
				layoutBox.Box.Height = explicitHeight + layoutBox.PaddingTop + layoutBox.PaddingBottom + layoutBox.BorderTopWidth + layoutBox.BorderBottomWidth
			}
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

	// Apply min-height and max-height constraints
	if node.ComputedStyle != nil {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}

		if node.ComputedStyle.MinHeight != "" {
			minH := parseLengthWithViewport(node.ComputedStyle.MinHeight, fontSize, le.canvasWidth, le.canvasHeight, le.canvasHeight)
			if node.ComputedStyle.BoxSizing != "border-box" {
				minH += layoutBox.PaddingTop + layoutBox.PaddingBottom + layoutBox.BorderTopWidth + layoutBox.BorderBottomWidth
			}
			if layoutBox.Box.Height < minH {
				layoutBox.Box.Height = minH
			}
		}

		if node.ComputedStyle.MaxHeight != "" && node.ComputedStyle.MaxHeight != "none" {
			maxH := parseLengthWithViewport(node.ComputedStyle.MaxHeight, fontSize, le.canvasWidth, le.canvasHeight, le.canvasHeight)
			if node.ComputedStyle.BoxSizing != "border-box" {
				maxH += layoutBox.PaddingTop + layoutBox.PaddingBottom + layoutBox.BorderTopWidth + layoutBox.BorderBottomWidth
			}
			if layoutBox.Box.Height > maxH {
				layoutBox.Box.Height = maxH
			}
		}
	}

	if layoutBox.Box.Height < 0 {
		layoutBox.Box.Height = 0
	}

	// Apply CSS positioning: copy position value and override coordinates for absolute/fixed
	if node.ComputedStyle != nil {
		layoutBox.Position = node.ComputedStyle.Position
		layoutBox.Float = node.ComputedStyle.Float
		layoutBox.Clear = node.ComputedStyle.Clear
	}
	if node.ComputedStyle != nil && node.ComputedStyle.Position != "" {
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
		} else if pos == "relative" {
			fontSize := le.defaultFontSize
			if node.ComputedStyle.FontSize > 0 {
				fontSize = node.ComputedStyle.FontSize
			}

			var dx, dy float32
			if node.ComputedStyle.Top != "" && node.ComputedStyle.Top != "auto" {
				dy = parseLength(node.ComputedStyle.Top, fontSize)
			} else if node.ComputedStyle.Bottom != "" && node.ComputedStyle.Bottom != "auto" {
				dy = -parseLength(node.ComputedStyle.Bottom, fontSize)
			}

			if node.ComputedStyle.Left != "" && node.ComputedStyle.Left != "auto" {
				dx = parseLength(node.ComputedStyle.Left, fontSize)
			} else if node.ComputedStyle.Right != "" && node.ComputedStyle.Right != "auto" {
				dx = -parseLength(node.ComputedStyle.Right, fontSize)
			}

			if dx != 0 || dy != 0 {
				layoutBox.Box.X += dx
				layoutBox.Box.Y += dy
				le.shiftLayoutBox(layoutBox, dx, dy)
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
func (le *LayoutEngine) buildTableLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext, inlineLayoutEngine *InlineLayoutEngine, flexLayoutEngine *FlexLayoutEngine, gridLayoutEngine *GridLayoutEngine) float32 {
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

	// 1. Gather all rows in correct visual order: thead -> tbody/direct-tr -> tfoot
	var rows []*RenderNode
	gatherRows := func(parent *RenderNode, targetTags []string) {
		for _, child := range parent.Children {
			isTarget := false
			for _, tag := range targetTags {
				if child.TagName == tag {
					isTarget = true
					break
				}
			}
			if isTarget {
				if child.TagName == "tr" {
					rows = append(rows, child)
				} else {
					for _, subChild := range child.Children {
						if subChild.TagName == "tr" {
							rows = append(rows, subChild)
						}
					}
				}
			}
		}
	}

	gatherRows(node, []string{"thead"})
	gatherRows(node, []string{"tbody", "tr"})
	gatherRows(node, []string{"tfoot"})

	if len(rows) == 0 {
		return y + layoutBox.PaddingTop + layoutBox.PaddingBottom
	}

	// 2. Map cells to coordinates, respecting colspan and rowspan
	maxCols := 0
	occupied := make(map[int]map[int]bool)
	isOccupied := func(row, col int) bool {
		if cols, ok := occupied[row]; ok {
			return cols[col]
		}
		return false
	}
	markOccupied := func(row, col int) {
		if _, ok := occupied[row]; !ok {
			occupied[row] = make(map[int]bool)
		}
		occupied[row][col] = true
	}

	for r, rowNode := range rows {
		currentRow := r + 1
		currentCol := 1

		// Try to get row background color
		var trBgColor color.Color
		if rowNode.ComputedStyle != nil {
			trBgColor = rowNode.ComputedStyle.BackgroundColor
		}

		for _, cell := range rowNode.Children {
			if cell.TagName == "td" || cell.TagName == "th" {
				// Find next unoccupied column
				for isOccupied(currentRow, currentCol) {
					currentCol++
				}

				// Parse colspan and rowspan, with a clamp of 100 to prevent OOM
				colspan := 1
				if attr, ok := cell.GetAttribute("colspan"); ok {
					if v, err := strconv.Atoi(attr); err == nil && v > 0 {
						if v > 100 {
							v = 100
						}
						colspan = v
					}
				}

				rowspan := 1
				if attr, ok := cell.GetAttribute("rowspan"); ok {
					if v, err := strconv.Atoi(attr); err == nil && v > 0 {
						if v > 100 {
							v = 100
						}
						rowspan = v
					}
				}

				// Mark occupied cells
				for dr := 0; dr < rowspan; dr++ {
					for dc := 0; dc < colspan; dc++ {
						markOccupied(currentRow+dr, currentCol+dc)
					}
				}

				colStart := currentCol
				colEnd := currentCol + colspan
				rowStart := currentRow
				rowEnd := currentRow + rowspan

				if colEnd-1 > maxCols {
					maxCols = colEnd - 1
				}

				// Create cell box
				cellBox := le.buildLayoutBox(cell, 0, 0, contentWidth, nil, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
				if cellBox != nil {
					cellBox.GridColumnStart = colStart
					cellBox.GridColumnEnd = colEnd
					cellBox.GridRowStart = rowStart
					cellBox.GridRowEnd = rowEnd

					// Transmit TR background to cell if cell has none
					if trBgColor != nil && (cellBox.BackgroundColor == nil || cellBox.BackgroundColor == color.Transparent) {
						cellBox.BackgroundColor = trBgColor
					}

					layoutBox.AddChild(cellBox)
				}

				currentCol += colspan
			}
		}
	}

	if maxCols == 0 {
		return y + layoutBox.PaddingTop + layoutBox.PaddingBottom
	}

	// 3. Set grid template columns
	var colsBuilder strings.Builder
	for i := 0; i < maxCols; i++ {
		if i > 0 {
			colsBuilder.WriteString(" ")
		}
		colsBuilder.WriteString("auto")
	}
	layoutBox.GridTemplateColumns = colsBuilder.String()

	// 4. Run grid layout
	gridLayoutEngine.LayoutTable(layoutBox)

	// Calculate height
	maxY := y + layoutBox.PaddingTop
	for _, child := range layoutBox.Children {
		childBottom := child.Box.Y + child.Box.Height + child.MarginBottom
		if childBottom > maxY {
			maxY = childBottom
		}
	}

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

	// Apply margins (use parseLengthWithViewport to support vh/vw/% units)
	layoutBox.MarginTop = parseLengthWithViewport(node.ComputedStyle.MarginTop, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginRight = parseLengthWithViewport(node.ComputedStyle.MarginRight, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginBottom = parseLengthWithViewport(node.ComputedStyle.MarginBottom, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginLeft = parseLengthWithViewport(node.ComputedStyle.MarginLeft, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	// parseLengthWithViewport returns -1 for "auto" or unsupported; treat as 0 for box model
	if layoutBox.MarginTop < 0 {
		layoutBox.MarginTop = 0
	}
	if layoutBox.MarginRight < 0 {
		layoutBox.MarginRight = 0
	}
	if layoutBox.MarginBottom < 0 {
		layoutBox.MarginBottom = 0
	}
	if layoutBox.MarginLeft < 0 {
		layoutBox.MarginLeft = 0
	}

	// Apply padding (use parseLengthWithViewport to support vh/vw/% units)
	layoutBox.PaddingTop = parseLengthWithViewport(node.ComputedStyle.PaddingTop, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.PaddingRight = parseLengthWithViewport(node.ComputedStyle.PaddingRight, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.PaddingBottom = parseLengthWithViewport(node.ComputedStyle.PaddingBottom, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.PaddingLeft = parseLengthWithViewport(node.ComputedStyle.PaddingLeft, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	if layoutBox.PaddingTop < 0 {
		layoutBox.PaddingTop = 0
	}
	if layoutBox.PaddingRight < 0 {
		layoutBox.PaddingRight = 0
	}
	if layoutBox.PaddingBottom < 0 {
		layoutBox.PaddingBottom = 0
	}
	if layoutBox.PaddingLeft < 0 {
		layoutBox.PaddingLeft = 0
	}

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
func (le *LayoutEngine) computeLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext, inlineLayoutEngine *InlineLayoutEngine, flexLayoutEngine *FlexLayoutEngine, gridLayoutEngine *GridLayoutEngine) float32 {
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
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = explicitWidth
		} else {
			width = explicitWidth + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	} else if node.ComputedStyle != nil && (node.ComputedStyle.Float == "left" || node.ComputedStyle.Float == "right" || node.ComputedStyle.Display == "inline-block") {
		// Float or Inline-Block with auto width: compute shrink-to-fit width
		contentW := float32(0)
		if le.hasInlineContent(node) {
			wsMode := le.whiteSpaceModeForNode(node)
			tempILE := NewInlineLayoutEngine(le.fontMetrics, le.defaultFontSize)
			lines, _ := tempILE.LayoutInlineContent(node, 0, 0, availableWidth, wsMode, nil)
			for _, line := range lines {
				if line.Width > contentW {
					contentW = line.Width
				}
			}
		} else {
			for _, child := range node.Children {
				if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
					continue
				}
				if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
					continue
				}
				childBox := le.buildLayoutBox(child, 0, 0, availableWidth, nil, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
				if childBox != nil {
					childW := childBox.Box.Width + childBox.MarginLeft + childBox.MarginRight
					if childW > contentW {
						contentW = childW
					}
				}
			}
		}

		if node.ComputedStyle.BoxSizing == "border-box" {
			width = contentW
		} else {
			width = contentW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}

		// Ensure width doesn't exceed availableWidth - (marginLeft + marginRight)
		maxWidth := availableWidth - (marginLeft + marginRight)
		if width > maxWidth {
			width = maxWidth
		}
	} else if node.TagName == "input" {
		defaultW := float32(150)
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = defaultW
		} else {
			width = defaultW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	} else if node.TagName == "textarea" {
		defaultW := float32(200)
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = defaultW
		} else {
			width = defaultW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	} else if node.TagName == "button" {
		defaultW := float32(80)
		if text := le.extractButtonText(node); text != "" {
			style := le.fontMetrics.GetTextStyleFromNode(node)
			letterSpacing := float32(0)
			if node.ComputedStyle != nil {
				letterSpacing = node.ComputedStyle.LetterSpacing
			}
			metrics := le.fontMetrics.MeasureText(text, le.defaultFontSize, style, letterSpacing)
			defaultW = metrics.Width + 20 // 20px padding/buffer
		}
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = defaultW
		} else {
			width = defaultW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	}

	// Apply min-width and max-width constraints
	if node.ComputedStyle != nil {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}

		if node.ComputedStyle.MinWidth != "" {
			minW := parseLengthWithViewport(node.ComputedStyle.MinWidth, fontSize, le.canvasWidth, le.canvasHeight, availableWidth)
			if node.ComputedStyle.BoxSizing != "border-box" {
				minW += layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
			}
			if width < minW {
				width = minW
			}
		}

		if node.ComputedStyle.MaxWidth != "" && node.ComputedStyle.MaxWidth != "none" {
			maxW := parseLengthWithViewport(node.ComputedStyle.MaxWidth, fontSize, le.canvasWidth, le.canvasHeight, availableWidth)
			if node.ComputedStyle.BoxSizing != "border-box" {
				maxW += layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
			}
			if width > maxW {
				width = maxW
			}
		}
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
		currentY = le.computeElementLayout(node, layoutBox, x, y, width, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
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
func (le *LayoutEngine) computeElementLayout(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext, inlineLayoutEngine *InlineLayoutEngine, flexLayoutEngine *FlexLayoutEngine, gridLayoutEngine *GridLayoutEngine) float32 {
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

	// Establish a BFC (independent FloatContext) if needed
	if floatCtx == nil || establishesBFC(node, layoutBox) {
		floatCtx = NewFloatContext(childX, currentY, contentWidth)
	}
	floatStart := len(floatCtx.floats)

	// Layout children
	childY := currentY

	// Check if this block element contains inline content
	// Block elements like p, div can contain inline content
	if layoutBox.Display == DisplayFlex {
		// Use Adapter to build child layout boxes with nil float context since flex items establish new BFC
		buildLayoutBoxAdapter := func(child *RenderNode, cx, cy, cw float32) *LayoutBox {
			return le.buildLayoutBox(child, cx, cy, cw, nil, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
		}
		// Use flexbox layout engine for flex containers
		flexLayoutEngine.LayoutFlexContainer(node, layoutBox, buildLayoutBoxAdapter)

		// Calculate childY based on laid out flex items
		for _, child := range layoutBox.Children {
			endY := child.Box.Y + child.Box.Height + child.MarginBottom
			if endY > childY {
				childY = endY
			}
		}
	} else if layoutBox.Display == DisplayGrid {
		// Use Adapter to build child layout boxes with nil float context since grid items establish new BFC
		buildLayoutBoxAdapter := func(child *RenderNode, cx, cy, cw float32) *LayoutBox {
			return le.buildLayoutBox(child, cx, cy, cw, nil, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
		}
		// Use grid layout engine for grid containers
		gridLayoutEngine.LayoutGridContainer(node, layoutBox, buildLayoutBoxAdapter)

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
		lines, totalHeight := inlineLayoutEngine.LayoutInlineContent(
			node, childX, currentY, contentWidth, wsMode, floatCtx,
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
			// Default content height for input is 30px
			childY = currentY + 30
		} else if node.TagName == "button" && !le.hasInlineContent(node) {
			// Default content height for empty button is 30px
			childY = currentY + 30
		} else if node.TagName == "textarea" {
			// Default content height for textarea is 60px
			childY = currentY + 60
		} else {
			var lastChild *LayoutBox
			for _, child := range node.Children {
				if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
					continue
				}
				// Skip whitespace-only text nodes in block children stacking
				if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
					continue
				}

				// 1. Handle clear
				if child.ComputedStyle != nil && child.ComputedStyle.Clear != "" {
					childY = floatCtx.ClearFloat(child.ComputedStyle.Clear, childY)
				}

				// 2. Handle float
				if child.ComputedStyle != nil && (child.ComputedStyle.Float == "left" || child.ComputedStyle.Float == "right") {
					floatDir := child.ComputedStyle.Float

					childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
					if childLayoutBox != nil {
						fx, fy := floatCtx.PlaceFloat(childLayoutBox, floatDir, childY, childX, contentWidth)
						dx := fx - childLayoutBox.Box.X
						dy := fy - childLayoutBox.Box.Y
						childLayoutBox.Box.X = fx
						childLayoutBox.Box.Y = fy
						le.shiftLayoutBox(childLayoutBox, dx, dy)

						floatCtx.AddFloat(childLayoutBox, floatDir)
						layoutBox.AddChild(childLayoutBox)
					}
					continue
				}

				// 3. Normal in-flow block elements: stack vertically with margin collapse
				nextChildY := childY
				if lastChild != nil {
					isBlock1 := lastChild.Display == DisplayBlock || lastChild.Display == DisplayFlex || lastChild.Display == DisplayGrid
					isBlock2 := child.IsBlock()
					if isBlock1 && isBlock2 &&
						lastChild.Position != "absolute" && lastChild.Position != "fixed" && lastChild.Float == "" &&
						child.ComputedStyle != nil && child.ComputedStyle.Position != "absolute" && child.ComputedStyle.Position != "fixed" && child.ComputedStyle.Float == "" {

						fontSize := le.defaultFontSize
						if child.ComputedStyle.FontSize > 0 {
							fontSize = child.ComputedStyle.FontSize
						}
						childMarginTop := parseLength(child.ComputedStyle.MarginTop, fontSize)
						collapsedMargin := maxFloat32(lastChild.MarginBottom, childMarginTop)

						lastChildBottom := lastChild.Box.Y + lastChild.Box.Height
						nextChildY = lastChildBottom + collapsedMargin - childMarginTop
					}
				}

				childLayoutBox := le.buildLayoutBox(child, childX, nextChildY, contentWidth, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
				if childLayoutBox != nil {
					layoutBox.AddChild(childLayoutBox)
					if childLayoutBox.Position != "absolute" && childLayoutBox.Position != "fixed" {
						childY = childLayoutBox.Box.Y + childLayoutBox.Box.Height + childLayoutBox.MarginBottom
						lastChild = childLayoutBox
					}
				}
			}
		}
	} else {
		// Inline elements: use inline layout engine
		if le.hasInlineContent(node) {
			wsMode := le.whiteSpaceModeForNode(node)
			lines, totalHeight := inlineLayoutEngine.LayoutInlineContent(
				node, childX, currentY, contentWidth, wsMode, floatCtx,
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
						le.nodeMapMu.Lock()
						le.nodeMap[inlineBox.NodeID] = layoutBox
						le.nodeMapMu.Unlock()
					}
				}
			}

			childY = currentY + totalHeight
		} else {
			// Check if element has intrinsic dimensions (e.g. input, button, textarea)
			// These might have no children (void tags or empty) but need rendering size
			if node.TagName == "input" {
				// Default content height for input is 30px
				childY = currentY + 30
			} else if node.TagName == "button" {
				// Default content height for button is 30px
				childY = currentY + 30
			} else if node.TagName == "textarea" {
				// Default content height for textarea is 60px
				childY = currentY + 60
			} else {
				// Fallback for empty inline elements using Block layout (e.g. empty div)
				for _, child := range node.Children {
					childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth, floatCtx, inlineLayoutEngine, flexLayoutEngine, gridLayoutEngine)
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

	// Enclose floats if BFC or overflow is not visible
	if node.ComputedStyle != nil && (node.ComputedStyle.Overflow == "hidden" || node.ComputedStyle.Overflow == "auto" || node.ComputedStyle.Overflow == "scroll") {
		childY = floatCtx.ClearFloat("both", childY)
	}
	for _, f := range floatCtx.floats[floatStart:] {
		if bottom := f.Box.Y + f.Box.Height; bottom > childY {
			childY = bottom
		}
	}

	// Add bottom padding
	childY += layoutBox.PaddingBottom

	// Add border bottom offset after padding
	childY += layoutBox.BorderBottomWidth

	return childY
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minFloat32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func establishesBFC(node *RenderNode, layoutBox *LayoutBox) bool {
	if node.ComputedStyle == nil {
		return false
	}
	// display: flow-root
	if node.ComputedStyle.Display == "flow-root" {
		return true
	}
	// overflow other than visible
	if node.ComputedStyle.Overflow != "" && node.ComputedStyle.Overflow != "visible" {
		return true
	}
	// float other than none
	if node.ComputedStyle.Float != "" && node.ComputedStyle.Float != "none" {
		return true
	}
	// position: absolute or fixed
	if node.ComputedStyle.Position == "absolute" || node.ComputedStyle.Position == "fixed" {
		return true
	}
	// display: inline-block, flex, grid
	disp := layoutBox.Display
	if disp == DisplayInlineBlock || disp == DisplayFlex || disp == DisplayGrid {
		return true
	}
	return false
}

// GetLayoutBox returns the LayoutBox for a given RenderNode ID
func (le *LayoutEngine) GetLayoutBox(nodeID int64) *LayoutBox {
	le.nodeMapMu.RLock()
	defer le.nodeMapMu.RUnlock()
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

// extractButtonText extracts text content recursively from a render node
func (le *LayoutEngine) extractButtonText(node *RenderNode) string {
	if node == nil {
		return ""
	}
	if node.Type == NodeTypeText {
		return strings.TrimSpace(node.Text)
	}
	var sb strings.Builder
	for _, child := range node.Children {
		t := le.extractButtonText(child)
		if t != "" {
			if sb.Len() > 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(t)
		}
	}
	return sb.String()
}

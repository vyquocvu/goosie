package renderer

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"

	imageloader "github.com/vyquocvu/goosie/internal/image"
)

// LayoutEngine handles layout calculations for render nodes
type LayoutEngine struct {
	mu           sync.Mutex
	canvasWidth  float32
	canvasHeight float32

	defaultFontSize float32
	lineHeight      float32

	nodeMap   map[int64]*LayoutBox
	nodeMapMu sync.RWMutex

	fontMetrics *FontMetrics

	// Sub-engines are stateless between computations, so one set is created
	// per engine and reused across ComputeLayout calls. All uses are
	// serialized by mu (rebuildSubtree also takes mu).
	inlineEngine *InlineLayoutEngine
	flexEngine   *FlexLayoutEngine
	gridEngine   *GridLayoutEngine
}

func NewLayoutEngine(width, height float32) *LayoutEngine {
	defaultSize := float32(16.0)
	fontMetrics := NewFontMetrics(defaultSize)
	le := &LayoutEngine{
		canvasWidth:     width,
		canvasHeight:    height,
		defaultFontSize: defaultSize,
		lineHeight:      1.5,
		nodeMap:         make(map[int64]*LayoutBox),
		fontMetrics:     fontMetrics,
		inlineEngine:    NewInlineLayoutEngine(fontMetrics, defaultSize),
		flexEngine:      NewFlexLayoutEngine(fontMetrics),
		gridEngine:      NewGridLayoutEngine(fontMetrics),
	}
	le.flexEngine.minContentFn = le.minContentSize
	return le
}

// ComputeLayout lays out the render tree and returns a layout tree. Layout
// always recomputes in full: callers mutate render trees in place (tests and
// the typed-mutation path), so no same-pointer caching is done here. Skipping
// work for unchanged documents is the caller's (or the incremental engine's)
// responsibility.
func (le *LayoutEngine) ComputeLayout(root *RenderNode) *LayoutBox {
	le.mu.Lock()
	defer le.mu.Unlock()

	if root == nil {
		return nil
	}

	le.nodeMapMu.Lock()
	clear(le.nodeMap)
	le.nodeMapMu.Unlock()

	return le.buildLayoutBox(root, 0, 0, le.canvasWidth, nil)
}

// layoutEnginePool recycles LayoutEngines across build cycles. Engines are
// per-cycle for thread safety (concurrent builds never share one), but their
// maps, sub-engines, and FontMetrics measurement caches are worth reusing.
var layoutEnginePool = sync.Pool{
	New: func() any { return NewLayoutEngine(0, 0) },
}

// getLayoutEngine returns a pooled engine reset for the given canvas size.
func getLayoutEngine(width, height float32) *LayoutEngine {
	le := layoutEnginePool.Get().(*LayoutEngine)
	le.reset(width, height)
	return le
}

// putLayoutEngine returns an engine to the pool, dropping cached references.
func putLayoutEngine(le *LayoutEngine) {
	if le == nil {
		return
	}
	le.reset(0, 0)
	layoutEnginePool.Put(le)
}

// reset reinitializes an engine (pool get/put and size changes). It drops
// all box mappings so a pooled engine retains nothing from the previous
// document.
func (le *LayoutEngine) reset(width, height float32) {
	le.mu.Lock()
	le.canvasWidth = width
	le.canvasHeight = height
	le.mu.Unlock()
	le.nodeMapMu.Lock()
	if le.nodeMap != nil {
		clear(le.nodeMap)
	} else {
		le.nodeMap = make(map[int64]*LayoutBox)
	}
	le.nodeMapMu.Unlock()
}

// buildLayoutBox creates a LayoutBox for a RenderNode and computes its layout
func (le *LayoutEngine) buildLayoutBox(node *RenderNode, x, y, availableWidth float32, floatCtx *FloatContext) *LayoutBox {
	if node == nil {
		return nil
	}

	layoutBox := NewLayoutBox(node.ID)
	le.nodeMapMu.Lock()
	le.nodeMap[node.ID] = layoutBox
	le.nodeMapMu.Unlock()

	if node.ComputedStyle != nil && node.ComputedStyle.Display != "" {
		switch node.ComputedStyle.Display {
		case "block":
			layoutBox.Display = DisplayBlock
		case "inline":
			layoutBox.Display = DisplayInline
		case "none":
			layoutBox.Display = DisplayNone
			return nil
		case "flex":
			layoutBox.Display = DisplayFlex
		case "grid":
			layoutBox.Display = DisplayGrid
		case "inline-block":
			layoutBox.Display = DisplayInlineBlock
		default:
			layoutBox.Display = DisplayInline
		}
	} else if node.Type == NodeTypeElement {
		if node.IsBlock() {
			layoutBox.Display = DisplayBlock
		} else {
			layoutBox.Display = DisplayInline
		}
	} else {
		layoutBox.Display = DisplayInline
	}

	le.applyBoxModel(node, layoutBox)

	var currentY float32
	if node.TagName == "table" {
		layoutBox.Display = DisplayGrid
		currentY = le.buildTableLayoutBox(node, layoutBox, x, y, availableWidth, floatCtx)
	} else {
		currentY = le.computeLayoutBox(node, layoutBox, x, y, availableWidth, floatCtx)
	}

	calculatedHeight := currentY - layoutBox.Box.Y

	if node.ComputedStyle != nil && node.ComputedStyle.Height != "" && node.ComputedStyle.Height != "auto" {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}
		explicitHeight := parseLengthWithViewport(node.ComputedStyle.Height, fontSize, le.canvasWidth, le.canvasHeight, le.canvasHeight)
		if explicitHeight >= 0 {
			if node.ComputedStyle.BoxSizing == "border-box" {
				layoutBox.Box.Height = explicitHeight
			} else {
				layoutBox.Box.Height = explicitHeight + layoutBox.PaddingTop + layoutBox.PaddingBottom + layoutBox.BorderTopWidth + layoutBox.BorderBottomWidth
			}
		} else {
			layoutBox.Box.Height = calculatedHeight
		}
	} else if node.TagName == "img" {
		// For img elements, fall back to HTML height attribute if CSS height is not set,
		// then to the image's intrinsic height once it is loaded.
		if hAttr, ok := node.GetAttribute("height"); ok && hAttr != "" {
			if v := parseLength(hAttr, le.defaultFontSize); v > 0 {
				layoutBox.Box.Height = v
			} else {
				layoutBox.Box.Height = calculatedHeight
			}
		} else if node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
			layoutBox.Box.Height = float32(node.ImageData.Height)
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

func (le *LayoutEngine) buildTableLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext) float32 {
	contentWidth := availableWidth
	if wAttr, ok := node.GetAttribute("width"); ok && wAttr != "" {
		if strings.HasSuffix(wAttr, "%") {
			if pct, err := strconv.ParseFloat(strings.TrimSuffix(wAttr, "%"), 32); err == nil && pct > 0 {
				contentWidth = availableWidth * float32(pct) / 100.0
			}
		} else {
			if val, err := strconv.ParseFloat(strings.TrimSuffix(wAttr, "px"), 32); err == nil && val > 0 {
				contentWidth = float32(val)
			}
		}
	}
	if csAttr, ok := node.GetAttribute("cellspacing"); ok {
		if v, err := strconv.Atoi(csAttr); err == nil && v >= 0 {
			layoutBox.Gap = float32(v)
		}
	}
	var cellPadding float32 = -1
	if cpAttr, ok := node.GetAttribute("cellpadding"); ok {
		if v, err := strconv.Atoi(cpAttr); err == nil && v >= 0 {
			cellPadding = float32(v)
		}
	}

	if contentWidth < 0 {
		contentWidth = 0
	}

	layoutBox.Box.X = x
	layoutBox.Box.Y = y
	layoutBox.Box.Width = contentWidth

	// Gather all rows in correct visual order: thead -> tbody/direct-tr -> tfoot
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

	colSpecs := make(map[int]string)

	for r, rowNode := range rows {
		currentRow := r + 1
		currentCol := 1

		var trBgColor color.Color
		if rowNode.ComputedStyle != nil {
			trBgColor = rowNode.ComputedStyle.BackgroundColor
		}

		for _, cell := range rowNode.Children {
			if cell.TagName == "td" || cell.TagName == "th" {
				for isOccupied(currentRow, currentCol) {
					currentCol++
				}

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

				if colspan == 1 {
					colIdx := colStart - 1
					cellWidthStr := ""
					if w, ok := cell.GetAttribute("width"); ok && w != "" {
						cellWidthStr = w
					} else if cell.ComputedStyle != nil && cell.ComputedStyle.Width != "" && cell.ComputedStyle.Width != "auto" {
						cellWidthStr = cell.ComputedStyle.Width
					}
					if cellWidthStr != "" {
						if strings.HasSuffix(cellWidthStr, "%") {
							if _, err := strconv.ParseFloat(strings.TrimSuffix(cellWidthStr, "%"), 32); err == nil {
								colSpecs[colIdx] = cellWidthStr
							}
						} else {
							if v, err := strconv.ParseFloat(strings.TrimSuffix(cellWidthStr, "px"), 32); err == nil && v > 0 {
								colSpecs[colIdx] = fmt.Sprintf("%dpx", int(v))
							}
						}
					}
				}

				cellBox := le.buildLayoutBox(cell, 0, 0, contentWidth, nil)
				if cellBox != nil {
					cellBox.GridColumnStart = colStart
					cellBox.GridColumnEnd = colEnd
					cellBox.GridRowStart = rowStart
					cellBox.GridRowEnd = rowEnd

					if cellPadding >= 0 && cellBox.PaddingTop == 0 && cellBox.PaddingBottom == 0 && cellBox.PaddingLeft == 0 && cellBox.PaddingRight == 0 {
						cellBox.PaddingTop = cellPadding
						cellBox.PaddingBottom = cellPadding
						cellBox.PaddingLeft = cellPadding
						cellBox.PaddingRight = cellPadding
					}

					if trBgColor != nil && (cellBox.BackgroundColor == nil || cellBox.BackgroundColor == color.Transparent) {
						cellBox.BackgroundColor = trBgColor
					}

					layoutBox.AddChild(cellBox)
				}

				currentCol += colspan
			}
		}
	}

	var colsBuilder strings.Builder
	for i := 0; i < maxCols; i++ {
		if i > 0 {
			colsBuilder.WriteString(" ")
		}
		if spec, ok := colSpecs[i]; ok && spec != "" {
			colsBuilder.WriteString(spec)
		} else {
			colsBuilder.WriteString("auto")
		}
	}
	layoutBox.GridTemplateColumns = colsBuilder.String()

	le.gridEngine.LayoutTable(layoutBox)

	maxY := y + layoutBox.PaddingTop
	for _, child := range layoutBox.Children {
		childBottom := child.Box.Y + child.Box.Height + child.MarginBottom
		if childBottom > maxY {
			maxY = childBottom
		}
	}

	return maxY + layoutBox.PaddingBottom
}

func (le *LayoutEngine) applyBoxModel(node *RenderNode, layoutBox *LayoutBox) {
	if node.ComputedStyle == nil {
		return
	}

	fontSize := le.defaultFontSize
	if node.ComputedStyle.FontSize > 0 {
		fontSize = node.ComputedStyle.FontSize
	}

	layoutBox.MarginTop = parseLengthWithViewport(node.ComputedStyle.MarginTop, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginRight = parseLengthWithViewport(node.ComputedStyle.MarginRight, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginBottom = parseLengthWithViewport(node.ComputedStyle.MarginBottom, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	layoutBox.MarginLeft = parseLengthWithViewport(node.ComputedStyle.MarginLeft, fontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
	if node.ComputedStyle.MarginTop == "" || node.ComputedStyle.MarginTop == "auto" {
		layoutBox.MarginTop = 0
	}
	if node.ComputedStyle.MarginRight == "" || node.ComputedStyle.MarginRight == "auto" {
		layoutBox.MarginRight = 0
	}
	if node.ComputedStyle.MarginBottom == "" || node.ComputedStyle.MarginBottom == "auto" {
		layoutBox.MarginBottom = 0
	}
	if node.ComputedStyle.MarginLeft == "" || node.ComputedStyle.MarginLeft == "auto" {
		layoutBox.MarginLeft = 0
	}

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

	layoutBox.BackgroundColor = node.ComputedStyle.BackgroundColor
	layoutBox.BackgroundImage = node.ComputedStyle.BackgroundImage
	layoutBox.BackgroundRepeat = node.ComputedStyle.BackgroundRepeat
	layoutBox.BackgroundPosition = node.ComputedStyle.BackgroundPosition
	layoutBox.BackgroundSize = node.ComputedStyle.BackgroundSize
	layoutBox.BackgroundAttachment = node.ComputedStyle.BackgroundAttachment
	layoutBox.BackgroundImageData = node.BackgroundImageData
	if layoutBox.BackgroundImageData == nil && node.ComputedStyle.BackgroundImageData != nil {
		layoutBox.BackgroundImageData = node.ComputedStyle.BackgroundImageData
	}
}

func (le *LayoutEngine) computeLayoutBox(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext) float32 {
	marginLeft := layoutBox.MarginLeft
	marginRight := layoutBox.MarginRight

	explicitWidth := float32(-1)
	if node.ComputedStyle != nil && node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
		fontSize := le.defaultFontSize
		if node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}
		explicitWidth = parseLengthWithViewport(node.ComputedStyle.Width, fontSize, le.canvasWidth, le.canvasHeight, availableWidth)
	}
	if node.TagName == "img" && explicitWidth < 0 {
		if wAttr, ok := node.GetAttribute("width"); ok && wAttr != "" {
			if v := parseLength(wAttr, le.defaultFontSize); v > 0 {
				explicitWidth = v
			}
		}
	}

	fontSize := le.defaultFontSize
	if node.ComputedStyle != nil && node.ComputedStyle.FontSize > 0 {
		fontSize = node.ComputedStyle.FontSize
	}

	// Determine the used border-box width for margin:auto centering. In CSS,
	// margin:auto distributes the remaining space around the full box
	// (content + padding + border), not just the specified content width.
	usedBoxWidth := float32(-1)
	if explicitWidth >= 0 {
		usedBoxWidth = explicitWidth
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing != "border-box" {
			usedBoxWidth += layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	} else if node.ComputedStyle != nil && node.ComputedStyle.MaxWidth != "" && node.ComputedStyle.MaxWidth != "none" {
		maxW := parseLengthWithViewport(node.ComputedStyle.MaxWidth, fontSize, le.canvasWidth, le.canvasHeight, availableWidth)
		if maxW >= 0 {
			usedBoxWidth = maxW
			if node.ComputedStyle.BoxSizing != "border-box" {
				usedBoxWidth += layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
			}
		}
	}

	if node.IsBlock() && usedBoxWidth >= 0 && usedBoxWidth < availableWidth {
		if node.ComputedStyle != nil && (node.ComputedStyle.MarginLeft == "auto" || node.ComputedStyle.MarginRight == "auto") {
			remainingSpace := availableWidth - usedBoxWidth
			if node.ComputedStyle.MarginLeft == "auto" && node.ComputedStyle.MarginRight == "auto" {
				marginLeft = remainingSpace / 2
				marginRight = remainingSpace / 2
			} else if node.ComputedStyle.MarginLeft == "auto" {
				marginLeft = remainingSpace
				marginRight = 0
			} else {
				marginLeft = 0
				marginRight = remainingSpace
			}
			layoutBox.MarginLeft = marginLeft
			layoutBox.MarginRight = marginRight
		}
	}

	x += marginLeft
	y += layoutBox.MarginTop

	width := availableWidth - (marginLeft + marginRight)
	if explicitWidth >= 0 {
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = explicitWidth
		} else {
			width = explicitWidth + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}
	} else if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		width = float32(node.ImageData.Width)
	} else if node.ComputedStyle != nil && (node.ComputedStyle.Float == "left" || node.ComputedStyle.Float == "right" || node.ComputedStyle.Display == "inline-block") {
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
				childW := le.measureMaxContentWidth(child)
				if childW > contentW {
					contentW = childW
				}
			}
		}

		if node.ComputedStyle.BoxSizing == "border-box" {
			width = contentW
		} else {
			width = contentW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}

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
			defaultW = metrics.Width + 20
		}
		if node.ComputedStyle != nil && node.ComputedStyle.BoxSizing == "border-box" {
			width = defaultW
		} else {
			width = defaultW + layoutBox.PaddingLeft + layoutBox.PaddingRight + layoutBox.BorderLeftWidth + layoutBox.BorderRightWidth
		}

		if width > availableWidth-marginLeft-marginRight {
			width = availableWidth - marginLeft - marginRight
		}
	}

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
		currentY = le.computeTextLayout(node, layoutBox, x, y, width)
	} else if node.Type == NodeTypeElement {
		currentY = le.computeElementLayout(node, layoutBox, x, y, width, floatCtx)
	}

	return currentY
}

func (le *LayoutEngine) computeTextLayout(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32) float32 {
	fontSize := le.defaultFontSize
	if node.Parent != nil && node.Parent.ComputedStyle != nil && node.Parent.ComputedStyle.FontSize > 0 {
		fontSize = node.Parent.ComputedStyle.FontSize
	}

	style := le.fontMetrics.GetTextStyleFromNode(node)

	text := strings.TrimSpace(node.Text)
	if text == "" {
		layoutBox.Box.Height = 0
		return y
	}

	letterSpacing := float32(0)
	if node.Parent != nil && node.Parent.ComputedStyle != nil {
		letterSpacing = node.Parent.ComputedStyle.LetterSpacing
	}
	metrics := le.fontMetrics.MeasureTextWithWrapping(text, fontSize, style, letterSpacing, availableWidth)

	layoutBox.Box.Height = metrics.Height

	return y + metrics.Height
}

func (le *LayoutEngine) computeElementLayout(node *RenderNode, layoutBox *LayoutBox, x, y, availableWidth float32, floatCtx *FloatContext) float32 {
	currentY := y + layoutBox.BorderTopWidth + layoutBox.PaddingTop
	childX := x + layoutBox.BorderLeftWidth + layoutBox.PaddingLeft

	contentWidth := availableWidth - layoutBox.BorderLeftWidth - layoutBox.PaddingLeft - layoutBox.PaddingRight - layoutBox.BorderRightWidth
	if contentWidth < 0 {
		contentWidth = 0
	}

	if floatCtx == nil || establishesBFC(node, layoutBox) {
		floatCtx = NewFloatContext(childX, currentY, contentWidth)
	}
	floatStart := len(floatCtx.floats)

	childY := currentY

	if layoutBox.Display == DisplayFlex {
		buildLayoutBoxAdapter := func(child *RenderNode, cx, cy, cw float32) *LayoutBox {
			return le.buildLayoutBox(child, cx, cy, cw, nil)
		}
		le.flexEngine.LayoutFlexContainer(node, layoutBox, buildLayoutBoxAdapter)

		for _, child := range layoutBox.Children {
			endY := child.Box.Y + child.Box.Height + child.MarginBottom
			if endY > childY {
				childY = endY
			}
		}
	} else if layoutBox.Display == DisplayGrid {
		buildLayoutBoxAdapter := func(child *RenderNode, cx, cy, cw float32) *LayoutBox {
			return le.buildLayoutBox(child, cx, cy, cw, nil)
		}
		le.gridEngine.LayoutGridContainer(node, layoutBox, buildLayoutBoxAdapter)

		for _, child := range layoutBox.Children {
			endY := child.Box.Y + child.Box.Height + child.MarginBottom
			if endY > childY {
				childY = endY
			}
		}
	} else if node.IsBlock() && le.hasBlockChildren(node) && le.hasInlineContent(node) {
		// Mixed block and inline content: lay out consecutive inline runs as
		// anonymous blocks (line boxes) and stack block-level children between
		// them, per CSS block-in-inline model.
		childY = le.layoutBlockAndInline(node, layoutBox, childX, currentY, contentWidth, floatCtx)
	} else if node.IsBlock() && le.hasBlockChildren(node) {
		if node.TagName == "input" {
			childY = currentY + 30
		} else if node.TagName == "button" && !le.hasInlineContent(node) {
			childY = currentY + 30
		} else if node.TagName == "textarea" {
			childY = currentY + 60
		} else {
			var lastChild *LayoutBox
			for _, child := range node.Children {
				if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
					continue
				}
				if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
					continue
				}

				if child.ComputedStyle != nil && child.ComputedStyle.Clear != "" {
					childY = floatCtx.ClearFloat(child.ComputedStyle.Clear, childY)
				}

				if child.ComputedStyle != nil && (child.ComputedStyle.Float == "left" || child.ComputedStyle.Float == "right") {
					le.layoutFloatedChild(child, layoutBox, childX, childY, contentWidth, floatCtx)
					continue
				}

				newY, childBox, inFlow := le.stackBlockChild(child, layoutBox, lastChild, childX, childY, contentWidth, floatCtx)
				if inFlow {
					childY = newY
					lastChild = childBox
				}
			}
		}
	} else {
		if le.hasInlineContent(node) {
			wsMode := le.whiteSpaceModeForNode(node)
			lines, totalHeight := le.inlineEngine.LayoutInlineContent(
				node, childX, currentY, contentWidth, wsMode, floatCtx,
			)

			layoutBox.LineBoxes = lines

			processedNodeIDs := make(map[int64]bool)
			for _, line := range lines {
				for _, inlineBox := range line.InlineBoxes {
					if !processedNodeIDs[inlineBox.NodeID] {
						processedNodeIDs[inlineBox.NodeID] = true
						le.nodeMapMu.Lock()
						le.nodeMap[inlineBox.NodeID] = layoutBox
						le.nodeMapMu.Unlock()
					}
				}
			}

			childY = currentY + totalHeight
		} else {
			if node.TagName == "input" {
				childY = currentY + 30
			} else if node.TagName == "button" {
				childY = currentY + 30
			} else if node.TagName == "textarea" {
				childY = currentY + 60
			} else {
				for _, child := range node.Children {
					childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth, floatCtx)
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

	if node.ComputedStyle != nil && (node.ComputedStyle.Overflow == "hidden" || node.ComputedStyle.Overflow == "auto" || node.ComputedStyle.Overflow == "scroll") {
		childY = floatCtx.ClearFloat("both", childY)
	}
	for _, f := range floatCtx.floats[floatStart:] {
		if bottom := f.Box.Y + f.Box.Height; bottom > childY {
			childY = bottom
		}
	}

	childY += layoutBox.PaddingBottom
	childY += layoutBox.BorderBottomWidth

	return childY
}

// layoutFloatedChild builds a LayoutBox for a floated child, places it via
// the float context, and attaches it to the parent box. Shared by the pure
// block and mixed block/inline stacking paths.
func (le *LayoutEngine) layoutFloatedChild(child *RenderNode, layoutBox *LayoutBox, childX, childY, contentWidth float32, floatCtx *FloatContext) {
	floatDir := child.ComputedStyle.Float

	childLayoutBox := le.buildLayoutBox(child, childX, childY, contentWidth, floatCtx)
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
}

// stackBlockChild builds a LayoutBox for an in-flow block child, stacking it
// vertically with margin collapse against the previous block box. The child
// is attached to layoutBox whenever its box could be built. Returns the
// advanced Y, the child's box, and whether the box participates in flow
// (in-flow boxes advance the stacking position; positioned boxes do not).
func (le *LayoutEngine) stackBlockChild(child *RenderNode, layoutBox *LayoutBox, lastChild *LayoutBox, childX, childY, contentWidth float32, floatCtx *FloatContext) (float32, *LayoutBox, bool) {
	// Collapse margins with the previous sibling block box.
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
			collapsedMargin := collapseMargins(lastChild.MarginBottom, childMarginTop)

			lastChildBottom := lastChild.Box.Y + lastChild.Box.Height
			nextChildY = lastChildBottom + collapsedMargin - childMarginTop
		}
	}

	childLayoutBox := le.buildLayoutBox(child, childX, nextChildY, contentWidth, floatCtx)
	if childLayoutBox == nil {
		return childY, nil, false
	}
	layoutBox.AddChild(childLayoutBox)
	if childLayoutBox.Position == "absolute" || childLayoutBox.Position == "fixed" {
		return childY, childLayoutBox, false
	}
	return childLayoutBox.Box.Y + childLayoutBox.Box.Height + childLayoutBox.MarginBottom, childLayoutBox, true
}

// layoutBlockAndInline lays out a block container whose children mix inline
// runs (text, inline elements, inline-blocks) with block-level children.
// Consecutive inline siblings are grouped into an anonymous block and laid
// out as line boxes; block-level children are stacked between the runs with
// float, clear, and margin handling mirroring the pure-block path.
func (le *LayoutEngine) layoutBlockAndInline(node *RenderNode, layoutBox *LayoutBox, childX, currentY, contentWidth float32, floatCtx *FloatContext) float32 {
	wsMode := le.whiteSpaceModeForNode(node)
	childY := currentY
	var lastChild *LayoutBox
	var run []*RenderNode

	flushRun := func() {
		if len(run) == 0 {
			return
		}
		// Whitespace-only runs between blocks must not create empty lines.
		hasContent := false
		for _, rn := range run {
			if rn.Type == NodeTypeElement || strings.TrimSpace(rn.Text) != "" {
				hasContent = true
				break
			}
		}
		if hasContent {
			lines, totalHeight := le.inlineEngine.LayoutInlineChildren(
				run, childX, childY, contentWidth, wsMode, floatCtx,
			)
			layoutBox.LineBoxes = append(layoutBox.LineBoxes, lines...)
			for _, line := range lines {
				for _, inlineBox := range line.InlineBoxes {
					le.nodeMapMu.Lock()
					le.nodeMap[inlineBox.NodeID] = layoutBox
					le.nodeMapMu.Unlock()
				}
			}
			childY += totalHeight
		}
		run = run[:0]
	}

	for _, child := range node.Children {
		if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
			continue
		}

		isBlockChild := child.Type == NodeTypeElement && child.IsBlock()
		if !isBlockChild {
			run = append(run, child)
			continue
		}

		flushRun()

		if child.ComputedStyle != nil && child.ComputedStyle.Clear != "" {
			childY = floatCtx.ClearFloat(child.ComputedStyle.Clear, childY)
		}

		if child.ComputedStyle != nil && (child.ComputedStyle.Float == "left" || child.ComputedStyle.Float == "right") {
			le.layoutFloatedChild(child, layoutBox, childX, childY, contentWidth, floatCtx)
			continue
		}

		// Normal in-flow block element: stack vertically with margin collapse
		// against the previous block box (line runs do not participate).
		newY, childBox, inFlow := le.stackBlockChild(child, layoutBox, lastChild, childX, childY, contentWidth, floatCtx)
		if inFlow {
			childY = newY
			lastChild = childBox
		}
	}

	flushRun()

	return childY
}

func collapseMargins(m1, m2 float32) float32 {
	if m1 >= 0 && m2 >= 0 {
		return max(m1, m2)
	}
	if m1 < 0 && m2 < 0 {
		return min(m1, m2)
	}
	return m1 + m2
}

func establishesBFC(node *RenderNode, layoutBox *LayoutBox) bool {
	if node.ComputedStyle == nil {
		return false
	}
	if node.ComputedStyle.Display == "flow-root" {
		return true
	}
	if node.ComputedStyle.Overflow != "" && node.ComputedStyle.Overflow != "visible" {
		return true
	}
	if node.ComputedStyle.Float != "" && node.ComputedStyle.Float != "none" {
		return true
	}
	if node.ComputedStyle.Position == "absolute" || node.ComputedStyle.Position == "fixed" {
		return true
	}
	disp := layoutBox.Display
	if disp == DisplayInlineBlock || disp == DisplayFlex || disp == DisplayGrid {
		return true
	}
	return false
}

func (le *LayoutEngine) GetLayoutBox(nodeID int64) *LayoutBox {
	le.nodeMapMu.RLock()
	defer le.nodeMapMu.RUnlock()
	return le.nodeMap[nodeID]
}

// HitTest returns the node ID of the deepest layout box containing (x, y), or 0.
func (le *LayoutEngine) HitTest(layoutRoot *LayoutBox, x, y float32) int64 {
	if layoutRoot == nil {
		return 0
	}

	return le.hitTestRecursive(layoutRoot, x, y)
}

func (le *LayoutEngine) hitTestRecursive(box *LayoutBox, x, y float32) int64 {
	if !box.Contains(x, y) {
		return 0
	}

	for _, child := range box.Children {
		if hitID := le.hitTestRecursive(child, x, y); hitID != 0 {
			return hitID
		}
	}

	return box.NodeID
}

// Layout performs layout calculations on the render tree (deprecated - use ComputeLayout)
func (le *LayoutEngine) Layout(root *RenderNode) {
	if root == nil {
		return
	}

	le.layoutNode(root, 0, 0, le.canvasWidth)
}

func (le *LayoutEngine) layoutNode(node *RenderNode, x, y, availableWidth float32) float32 {
	if node == nil {
		return y
	}

	currentY := y

	if node.Type == NodeTypeText {
		currentY = le.layoutTextNode(node, x, y, availableWidth)
	} else if node.Type == NodeTypeElement {
		currentY = le.layoutElementNode(node, x, y, availableWidth)
	}

	return currentY
}

func (le *LayoutEngine) layoutTextNode(node *RenderNode, x, y, availableWidth float32) float32 {
	fontSize := le.defaultFontSize
	if node.Parent != nil {
		fontSize = le.fontMetrics.GetFontSize(node.Parent.TagName)
	}

	style := le.fontMetrics.GetTextStyleFromNode(node)

	text := strings.TrimSpace(node.Text)
	if text == "" {
		return y
	}

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

func (le *LayoutEngine) getFontSize(tagName string) float32 {
	return le.fontMetrics.GetFontSize(tagName)
}

// whiteSpaceModeForNode selects white space handling. The CSS white-space
// property wins when present (it is inherited, so descendants pick it up);
// the tag fallback only covers unstyled documents and matches browser UA
// defaults — notably <code> wraps by default (white-space: normal).
func (le *LayoutEngine) whiteSpaceModeForNode(node *RenderNode) WhiteSpaceMode {
	if node == nil {
		return WhiteSpaceNormal
	}
	if node.ComputedStyle != nil && node.ComputedStyle.WhiteSpace != "" {
		if mode, ok := whiteSpaceModeFromCSS(node.ComputedStyle.WhiteSpace); ok {
			return mode
		}
	}
	if node.TagName == "pre" {
		return WhiteSpacePre
	}
	return WhiteSpaceNormal
}

func whiteSpaceModeFromCSS(value string) (WhiteSpaceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "normal":
		return WhiteSpaceNormal, true
	case "nowrap":
		return WhiteSpaceNoWrap, true
	case "pre":
		return WhiteSpacePre, true
	case "pre-wrap", "break-spaces":
		return WhiteSpacePreWrap, true
	case "pre-line":
		return WhiteSpacePreLine, true
	}
	return WhiteSpaceNormal, false
}

func (le *LayoutEngine) hasBlockChildren(node *RenderNode) bool {
	if node == nil {
		return false
	}
	for _, child := range node.Children {
		if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
			continue
		}
		if child.Type == NodeTypeElement && child.IsBlock() {
			return true
		}
	}
	return false
}

func (le *LayoutEngine) hasInlineContent(node *RenderNode) bool {
	return le.hasInlineContentRecursive(node)
}

// measureMaxContentWidth computes the max-content width of a node: the width it
// needs to lay out all its content on a single line without wrapping. This is
// used for shrink-to-fit sizing of floats and inline-blocks whose children are
// block-level boxes.
func (le *LayoutEngine) measureMaxContentWidth(node *RenderNode) float32 {
	if node == nil {
		return 0
	}
	if node.Type == NodeTypeText {
		return 0
	}
	if node.ComputedStyle != nil && node.ComputedStyle.Display == "none" {
		return 0
	}

	if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		return float32(node.ImageData.Width)
	}

	var contentW float32
	if le.hasInlineContent(node) {
		wsMode := le.whiteSpaceModeForNode(node)
		tempILE := NewInlineLayoutEngine(le.fontMetrics, le.defaultFontSize)
		lines, _ := tempILE.LayoutInlineContent(node, 0, 0, 1e6, wsMode, nil)
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
			childW := le.measureMaxContentWidth(child)
			// Float or inline-level children are placed side by side; block
			// children stack, so the max-content width is the sum of floats
			// on a line or the widest stacked block.
			if child.ComputedStyle != nil && (child.ComputedStyle.Float == "left" || child.ComputedStyle.Float == "right") {
				contentW += childW
			} else if childW > contentW {
				contentW = childW
			}
		}
	}

	// Include the box model (padding/border) for the node being measured.
	if node.ComputedStyle != nil {
		box := NewLayoutBox(node.ID)
		le.applyBoxModel(node, box)
		contentW += box.PaddingLeft + box.PaddingRight + box.BorderLeftWidth + box.BorderRightWidth
	}
	return contentW
}

// minContentSize computes the min-content width of a node: the narrowest width
// its content can occupy without overflowing (the widest unbreakable segment,
// e.g. the longest word or a fixed-size child). It does NOT clamp by the node's
// own explicit width - that is the caller's "specified size suggestion" job.
// Box model padding/border are included.
func (le *LayoutEngine) minContentSize(node *RenderNode) float32 {
	if node == nil {
		return 0
	}
	if node.ComputedStyle != nil && node.ComputedStyle.Display == "none" {
		return 0
	}
	if node.Type == NodeTypeText {
		return 0
	}
	if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		box := NewLayoutBox(node.ID)
		le.applyBoxModel(node, box)
		return float32(node.ImageData.Width) + box.PaddingLeft + box.PaddingRight + box.BorderLeftWidth + box.BorderRightWidth
	}

	var contentW float32
	if le.hasInlineContent(node) {
		contentW = le.widestInlineSegment(node)
	} else {
		for _, child := range node.Children {
			if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
				continue
			}
			if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
				continue
			}
			childW := le.minContentContribution(child)
			if child.ComputedStyle != nil && (child.ComputedStyle.Float == "left" || child.ComputedStyle.Float == "right") {
				contentW += childW
			} else if childW > contentW {
				contentW = childW
			}
		}
	}

	box := NewLayoutBox(node.ID)
	le.applyBoxModel(node, box)
	contentW += box.PaddingLeft + box.PaddingRight + box.BorderLeftWidth + box.BorderRightWidth
	return contentW
}

// minContentContribution returns how much of a node's width a parent must
// reserve: the max of the node's own content-based min-content size and its
// explicit width, plus margins.
func (le *LayoutEngine) minContentContribution(node *RenderNode) float32 {
	contentW := le.minContentSize(node)
	if node.ComputedStyle != nil && node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
		// Percentage widths are indefinite during intrinsic sizing and must
		// not inflate the min-content contribution (a width: 100% child does
		// not force its parent to be viewport-wide).
		if !strings.HasSuffix(node.ComputedStyle.Width, "%") {
			w := parseLengthWithViewport(node.ComputedStyle.Width, le.defaultFontSize, le.canvasWidth, le.canvasHeight, le.canvasWidth)
			if w > contentW {
				contentW = w
			}
		}
	}
	if node.ComputedStyle != nil {
		box := NewLayoutBox(node.ID)
		le.applyBoxModel(node, box)
		contentW += box.MarginLeft + box.MarginRight
	}
	return contentW
}

// widestInlineSegment returns the width of the widest unbreakable inline
// segment in a node's subtree: the widest word, inline image, or replaced
// element, which governs the min-content width of inline content.
func (le *LayoutEngine) widestInlineSegment(node *RenderNode) float32 {
	if node == nil {
		return 0
	}
	if node.ComputedStyle != nil && node.ComputedStyle.Display == "none" {
		return 0
	}
	if node.Type == NodeTypeText {
		if strings.TrimSpace(node.Text) == "" {
			return 0
		}
		fontSize := le.defaultFontSize
		if node.ComputedStyle != nil && node.ComputedStyle.FontSize > 0 {
			fontSize = node.ComputedStyle.FontSize
		}
		style := le.fontMetrics.GetTextStyleFromNode(node)
		letterSpacing := float32(0)
		if node.ComputedStyle != nil {
			letterSpacing = node.ComputedStyle.LetterSpacing
		}
		var widest float32
		for _, word := range strings.Fields(node.Text) {
			m := le.fontMetrics.MeasureText(word, fontSize, style, letterSpacing)
			if m.Width > widest {
				widest = m.Width
			}
		}
		return widest
	}
	if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		return float32(node.ImageData.Width)
	}
	// Replaced-style elements contribute fixed intrinsic widths, not flowing
	// text. Calling minContentSize here is circular — minContentSize calls
	// back into widestInlineSegment whenever the node has inline children
	// (a text-bearing button or textarea), which recursed until stack
	// overflow on github.com. Buttons and textareas measure their own text
	// through the generic child loop below instead.
	if node.TagName == "input" {
		return float32(150) // matches the form-control default width in computeElementLayout
	}
	if node.TagName == "svg" {
		return 0
	}
	var widest float32
	for _, c := range node.Children {
		if w := le.widestInlineSegment(c); w > widest {
			widest = w
		}
	}
	return widest
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
			if child.TagName == "img" || child.TagName == "svg" || child.TagName == "input" || child.TagName == "button" || child.TagName == "textarea" {
				// Replaced elements (img, svg, form controls) are inline content:
				// they have intrinsic dimensions that the inline layout engine sizes.
				return true
			}
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

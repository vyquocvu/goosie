package renderer

import (
	"sort"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

// FlexLayoutEngine handles flexbox layout calculations
type FlexLayoutEngine struct {	fontMetrics *FontMetrics
	// minContentFn measures a node's min-content size along the main axis.
	// Used to compute the automatic minimum size (min-width:auto / min-height:auto)
	// of flex items so they don't collapse below their content.
	minContentFn func(node *RenderNode) float32
}

// NewFlexLayoutEngine creates a new flex layout engine
func NewFlexLayoutEngine(fm *FontMetrics) *FlexLayoutEngine {
	return &FlexLayoutEngine{
		fontMetrics: fm,
	}
}

// flexItem represents a flex item during layout calculation
type flexItem struct {
	node       *RenderNode
	layoutBox  *LayoutBox
	mainSize   float32 // Size along main axis
	crossSize  float32 // Size along cross axis
	minMainSize float32 // Automatic minimum size along main axis (min-width:auto floor)
	flexGrow   float32
	flexShrink float32
	flexBasis  float32
	basisSet   bool // true when flex-basis was explicitly specified (including 0)
	order      int
}

// LayoutFlexContainer performs flexbox layout on a container's children
func (fle *FlexLayoutEngine) LayoutFlexContainer(
	container *RenderNode,
	parentBox *LayoutBox,
	buildLayoutBox func(node *RenderNode, x, y, width float32) *LayoutBox,
) {
	if container == nil || len(container.Children) == 0 {
		return
	}

	// Get flex container properties from computed style
	direction := "row"
	justifyContent := "flex-start"
	alignItems := "stretch"
	gap := float32(0)

	if container.ComputedStyle != nil {
		if container.ComputedStyle.FlexDirection != "" {
			direction = container.ComputedStyle.FlexDirection
		}
		if container.ComputedStyle.JustifyContent != "" {
			justifyContent = container.ComputedStyle.JustifyContent
		}
		if container.ComputedStyle.AlignItems != "" {
			alignItems = container.ComputedStyle.AlignItems
		}
		if container.ComputedStyle.Gap != "" {
			gap = parseLength(container.ComputedStyle.Gap, 16)
		}
	}

	// Determine main and cross axis dimensions
	isRow := direction == "row" || direction == "row-reverse"
	isReverse := direction == "row-reverse" || direction == "column-reverse"

	mainAxisSize := parentBox.Box.Width - parentBox.PaddingLeft - parentBox.PaddingRight
	crossAxisSize := parentBox.Box.Height - parentBox.PaddingTop - parentBox.PaddingBottom
	mainAxisDefinite := true
	if !isRow {
		mainAxisSize, crossAxisSize = crossAxisSize, mainAxisSize
		// For column direction with no explicit height, the container is
		// shrink-wrapped by its content: flex-grow has no free space to
		// distribute, so keep the main axis indefinite (no 10000 fallback
		// which would inflate growing items like a flex-grow:1 paragraph).
		if mainAxisSize <= 0 {
			mainAxisSize = 10000 // Large value, will be shrunk by content
			mainAxisDefinite = false
		}
	}

	// Build flex items
	items := fle.buildFlexItems(container, buildLayoutBox, parentBox, isRow)
	if len(items) == 0 {
		return
	}

	// Sort by order property
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].order < items[j].order
	})

	// Reverse if needed
	if isReverse {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}

	// Calculate total main axis size and remaining space
	totalMainSize := float32(0)
	totalGaps := gap * float32(len(items)-1)
	for _, item := range items {
		totalMainSize += item.mainSize
	}
	totalMainSize += totalGaps

	remainingSpace := mainAxisSize - totalMainSize


	// Distribute remaining space (flex-grow/shrink). For an indefinite column
	// main axis (auto height), the container shrink-wraps its content so there
	// is no free space to grow into.
	if remainingSpace > 0 {
		if mainAxisDefinite {
			fle.distributeGrow(items, remainingSpace)
		}
	} else if remainingSpace < 0 {
		fle.distributeShrink(items, -remainingSpace)
	}

	// Position items along main axis based on justify-content.
	// For an indefinite column main axis (auto height) the container
	// shrink-wraps its content, so there is no free space to distribute and
	// justify-content is a no-op. Using the 10000px fallback here would push
	// flex-end/center items ~10000px down (e.g. Google's doodle homepage).
	posAxisSize := mainAxisSize
	if !mainAxisDefinite {
		posAxisSize = totalMainSize
	}
	mainAxisPositions := fle.calculateMainAxisPositions(items, posAxisSize, gap, justifyContent)

	// Position items along cross axis based on align-items
	contentX := parentBox.Box.X + parentBox.PaddingLeft
	contentY := parentBox.Box.Y + parentBox.PaddingTop

	for i, item := range items {
		var x, y float32

		if isRow {
			x = contentX + mainAxisPositions[i]
			y = contentY + fle.calculateCrossAxisOffset(item.crossSize, crossAxisSize, alignItems, item)
		} else {
			x = contentX + fle.calculateCrossAxisOffset(item.crossSize, crossAxisSize, alignItems, item)
			y = contentY + mainAxisPositions[i]
		}

		// Re-layout the item with its resolved flex size so its internal
		// content flows at the final width/height rather than the container's
		// full available size. Without this, a shrunken item keeps content laid
		// out at the container width (e.g. main at 850px still containing
		// 1100px-wide children).
		resolvedMainSize := item.mainSize
		resolvedCrossSize := item.crossSize
		if alignItems == "stretch" && item.crossSize == 0 {
			resolvedCrossSize = crossAxisSize
		}
		var newBox *LayoutBox
		if isRow {
			if absDiff(resolvedMainSize, item.layoutBox.Box.Width) > 0.5 {
				newBox = buildLayoutBox(item.node, x, y, resolvedMainSize)
			}
		} else {
			if absDiff(resolvedMainSize, item.layoutBox.Box.Height) > 0.5 {
				newBox = buildLayoutBox(item.node, x, y, resolvedCrossSize)
			}
		}
		if newBox != nil {
			item.layoutBox = newBox
		}

		if isRow {
			deltaX := x - item.layoutBox.Box.X
			deltaY := y - item.layoutBox.Box.Y
			shiftLayoutBoxTree(item.layoutBox, deltaX, deltaY)
			item.layoutBox.Box.Width = resolvedMainSize
			if alignItems == "stretch" && item.crossSize == 0 {
				item.layoutBox.Box.Height = resolvedCrossSize
			}
		} else {
			deltaX := x - item.layoutBox.Box.X
			deltaY := y - item.layoutBox.Box.Y
			shiftLayoutBoxTree(item.layoutBox, deltaX, deltaY)
			if alignItems == "stretch" && item.crossSize == 0 {
				item.layoutBox.Box.Width = resolvedCrossSize
			}
			item.layoutBox.Box.Height = resolvedMainSize
		}

		parentBox.AddChild(item.layoutBox)
	}
}

func absDiff(a, b float32) float32 {
	if a > b {
		return a - b
	}
	return b - a
}

// buildFlexItems creates flex items from container children
func (fle *FlexLayoutEngine) buildFlexItems(
	container *RenderNode,
	buildLayoutBox func(node *RenderNode, x, y, width float32) *LayoutBox,
	parentBox *LayoutBox,
	isRow bool,
) []*flexItem {
	items := make([]*flexItem, 0, len(container.Children))

	for _, child := range container.Children {
		if child.ComputedStyle != nil && child.ComputedStyle.Display == css.DisplayAtomNone {
			continue
		}
		if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
			continue
		}

		// Children are always laid out at the container's content width
		// (main axis for rows, cross axis for columns). Using mainAxisSize
		// directly would give column items a width equal to the container's
		// height fallback (e.g. 10000px), breaking their inline layout.
		availWidth := parentBox.Box.Width - parentBox.PaddingLeft - parentBox.PaddingRight
		childBox := buildLayoutBox(child, 0, 0, availWidth)
		if childBox == nil {
			continue
		}

		item := &flexItem{
			node:       child,
			layoutBox:  childBox,
			flexGrow:   0,
			flexShrink: 1, // Default shrink is 1
			order:      0,
		}

		// Get flex properties from computed style
		if child.ComputedStyle != nil {
			item.flexGrow = child.ComputedStyle.FlexGrow
			if child.ComputedStyle.FlexShrink > 0 {
				item.flexShrink = child.ComputedStyle.FlexShrink
			}
			item.order = child.ComputedStyle.Order

			// Parse flex-basis: "auto" (or empty) means fall back to width/height;
			// an explicit length (including 0) overrides the natural size.
			if child.ComputedStyle.FlexBasis != "" && child.ComputedStyle.FlexBasis != "auto" {
				item.flexBasis = parseLength(child.ComputedStyle.FlexBasis, 16)
				item.basisSet = true
			}
		}

		// Calculate main and cross sizes
		if isRow {
			if item.basisSet {
				item.mainSize = item.flexBasis
			} else {
				item.mainSize = childBox.Box.Width
			}
			item.crossSize = childBox.Box.Height
			item.minMainSize = fle.automaticMinMainSize(child, childBox)
		} else {
			if item.basisSet {
				item.mainSize = item.flexBasis
			} else {
				item.mainSize = childBox.Box.Height
			}
			item.crossSize = childBox.Box.Width
			item.minMainSize = fle.automaticMinMainSize(child, childBox)
		}


		// Apply the automatic minimum size so items don't collapse below their
		// content (min-width/min-height: auto behavior).
		if item.mainSize < item.minMainSize {
			item.mainSize = item.minMainSize
		}

		items = append(items, item)
	}

	return items
}

// automaticMinMainSize computes the automatic minimum main size of a flex item.
// Per css-flexbox §4.5, the automatic minimum size (min-width:auto, the default)
// is the smaller of the content size suggestion (min-content size) and the
// specified size suggestion (the item's width/height when definite). An explicit
// min-width/min-height overrides it entirely.
func (fle *FlexLayoutEngine) automaticMinMainSize(node *RenderNode, childBox *LayoutBox) float32 {
	if node.ComputedStyle == nil {
		return 0
	}

	// An explicit min-width takes precedence over the automatic minimum.
	explicitMin := node.ComputedStyle.MinWidth
	if explicitMin != "" && explicitMin != "auto" {
		if minW := parseLength(explicitMin, fle.fontMetrics.defaultFontSize); minW >= 0 {
			return minW
		}
	}

	// Automatic minimum: min(specified size suggestion, content size suggestion).
	contentSuggestion := float32(0)
	if fle.minContentFn != nil {
		contentSuggestion = fle.minContentFn(node)
	}
	specifiedSuggestion := float32(-1)
	if node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
		if w := parseLength(node.ComputedStyle.Width, fle.fontMetrics.defaultFontSize); w >= 0 {
			specifiedSuggestion = w
		}
	}

	min := contentSuggestion
	if specifiedSuggestion >= 0 && specifiedSuggestion < min {
		min = specifiedSuggestion
	}

	// Clamp by the maximum main size if it's definite.
	if node.ComputedStyle.MaxWidth != "" && node.ComputedStyle.MaxWidth != "none" {
		if maxW := parseLength(node.ComputedStyle.MaxWidth, fle.fontMetrics.defaultFontSize); maxW >= 0 && maxW < min {
			min = maxW
		}
	}
	return min
}

// distributeGrow distributes positive remaining space based on flex-grow
func (fle *FlexLayoutEngine) distributeGrow(items []*flexItem, remainingSpace float32) {
	totalGrow := float32(0)
	for _, item := range items {
		totalGrow += item.flexGrow
	}

	if totalGrow <= 0 {
		return
	}

	for _, item := range items {
		if item.flexGrow > 0 {
			item.mainSize += (item.flexGrow / totalGrow) * remainingSpace
		}
	}
}

// distributeShrink distributes negative remaining space based on flex-shrink
func (fle *FlexLayoutEngine) distributeShrink(items []*flexItem, overflow float32) {
	totalShrink := float32(0)
	for _, item := range items {
		shrinkable := item.mainSize - item.minMainSize
		if shrinkable < 0 {
			shrinkable = 0
		}
		totalShrink += item.flexShrink * shrinkable
	}

	if totalShrink <= 0 {
		return
	}

	for _, item := range items {
		if item.flexShrink > 0 {
			shrinkable := item.mainSize - item.minMainSize
			if shrinkable < 0 {
				shrinkable = 0
			}
			shrinkAmount := (item.flexShrink * shrinkable / totalShrink) * overflow
			item.mainSize -= shrinkAmount
			if item.mainSize < item.minMainSize {
				item.mainSize = item.minMainSize
			}
		}
	}
}

// calculateMainAxisPositions calculates positions along main axis based on justify-content
func (fle *FlexLayoutEngine) calculateMainAxisPositions(
	items []*flexItem,
	mainAxisSize float32,
	gap float32,
	justifyContent string,
) []float32 {
	positions := make([]float32, len(items))
	if len(items) == 0 {
		return positions
	}

	totalItemSize := float32(0)
	for _, item := range items {
		totalItemSize += item.mainSize
	}
	totalGaps := gap * float32(len(items)-1)
	freeSpace := mainAxisSize - totalItemSize - totalGaps

	var startOffset float32
	var spaceBetween float32

	switch justifyContent {
	case "flex-end":
		startOffset = freeSpace
		spaceBetween = gap
	case "center":
		startOffset = freeSpace / 2
		spaceBetween = gap
	case "space-between":
		startOffset = 0
		if len(items) > 1 {
			spaceBetween = freeSpace / float32(len(items)-1)
		} else {
			spaceBetween = 0
		}
	case "space-around":
		unitSpace := freeSpace / float32(len(items))
		startOffset = unitSpace / 2
		spaceBetween = unitSpace + gap
	case "space-evenly":
		unitSpace := freeSpace / float32(len(items)+1)
		startOffset = unitSpace
		spaceBetween = unitSpace + gap
	default: // flex-start
		startOffset = 0
		spaceBetween = gap
	}

	position := startOffset
	for i, item := range items {
		positions[i] = position
		position += item.mainSize + spaceBetween
	}

	return positions
}

// calculateCrossAxisOffset calculates offset along cross axis based on align-items
func (fle *FlexLayoutEngine) calculateCrossAxisOffset(
	itemCrossSize float32,
	containerCrossSize float32,
	alignItems string,
	item *flexItem,
) float32 {
	// Check for align-self override
	alignSelf := alignItems
	if item.node.ComputedStyle != nil && item.node.ComputedStyle.AlignSelf != "" {
		alignSelf = item.node.ComputedStyle.AlignSelf
	}

	switch alignSelf {
	case "flex-end":
		return containerCrossSize - itemCrossSize
	case "center":
		return (containerCrossSize - itemCrossSize) / 2
	case "stretch":
		return 0 // Item will be stretched to fill container
	case "baseline":
		return 0 // Baseline alignment not fully implemented
	default: // flex-start
		return 0
	}
}

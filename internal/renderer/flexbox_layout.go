package renderer

import (
	"sort"
	"strings"
)

// FlexLayoutEngine handles flexbox layout calculations
type FlexLayoutEngine struct {
	fontMetrics *FontMetrics
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
	flexGrow   float32
	flexShrink float32
	flexBasis  float32
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
	if !isRow {
		mainAxisSize, crossAxisSize = crossAxisSize, mainAxisSize
		// For column direction with no explicit height, use a large value and let content determine size
		if mainAxisSize <= 0 {
			mainAxisSize = 10000 // Large value, will be shrunk by content
		}
	}

	// Build flex items
	items := fle.buildFlexItems(container, buildLayoutBox, parentBox, mainAxisSize, isRow)
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

	// Distribute remaining space (flex-grow/shrink)
	if remainingSpace > 0 {
		fle.distributeGrow(items, remainingSpace)
	} else if remainingSpace < 0 {
		fle.distributeShrink(items, -remainingSpace)
	}

	// Position items along main axis based on justify-content
	mainAxisPositions := fle.calculateMainAxisPositions(items, mainAxisSize, gap, justifyContent)

	// Position items along cross axis based on align-items
	contentX := parentBox.Box.X + parentBox.PaddingLeft
	contentY := parentBox.Box.Y + parentBox.PaddingTop

	for i, item := range items {
		var x, y float32

		if isRow {
			x = contentX + mainAxisPositions[i]
			y = contentY + fle.calculateCrossAxisOffset(item.crossSize, crossAxisSize, alignItems, item)
			item.layoutBox.Box.X = x
			item.layoutBox.Box.Y = y
			item.layoutBox.Box.Width = item.mainSize
			if alignItems == "stretch" && item.crossSize == 0 {
				item.layoutBox.Box.Height = crossAxisSize
			}
		} else {
			x = contentX + fle.calculateCrossAxisOffset(item.crossSize, crossAxisSize, alignItems, item)
			y = contentY + mainAxisPositions[i]
			item.layoutBox.Box.X = x
			item.layoutBox.Box.Y = y
			if alignItems == "stretch" && item.crossSize == 0 {
				item.layoutBox.Box.Width = crossAxisSize
			}
			item.layoutBox.Box.Height = item.mainSize
		}

		parentBox.AddChild(item.layoutBox)
	}
}

// buildFlexItems creates flex items from container children
func (fle *FlexLayoutEngine) buildFlexItems(
	container *RenderNode,
	buildLayoutBox func(node *RenderNode, x, y, width float32) *LayoutBox,
	parentBox *LayoutBox,
	mainAxisSize float32,
	isRow bool,
) []*flexItem {
	items := make([]*flexItem, 0, len(container.Children))

	for _, child := range container.Children {
		if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
			continue
		}
		if child.Type == NodeTypeText && strings.TrimSpace(child.Text) == "" {
			continue
		}

		// Build the child layout box to get its natural size
		childBox := buildLayoutBox(child, 0, 0, mainAxisSize)
		if childBox == nil {
			continue
		}

		item := &flexItem{
			node:       child,
			layoutBox:  childBox,
			flexGrow:   0,
			flexShrink: 1, // Default shrink is 1
			flexBasis:  0, // 0 means auto
			order:      0,
		}

		// Get flex properties from computed style
		if child.ComputedStyle != nil {
			item.flexGrow = child.ComputedStyle.FlexGrow
			if child.ComputedStyle.FlexShrink > 0 {
				item.flexShrink = child.ComputedStyle.FlexShrink
			}
			item.order = child.ComputedStyle.Order

			// Parse flex-basis
			if child.ComputedStyle.FlexBasis != "" && child.ComputedStyle.FlexBasis != "auto" {
				item.flexBasis = parseLength(child.ComputedStyle.FlexBasis, 16)
			}
		}

		// Calculate main and cross sizes
		if isRow {
			if item.flexBasis > 0 {
				item.mainSize = item.flexBasis
			} else {
				item.mainSize = childBox.Box.Width
			}
			item.crossSize = childBox.Box.Height
		} else {
			if item.flexBasis > 0 {
				item.mainSize = item.flexBasis
			} else {
				item.mainSize = childBox.Box.Height
			}
			item.crossSize = childBox.Box.Width
		}

		items = append(items, item)
	}

	return items
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
		totalShrink += item.flexShrink * item.mainSize
	}

	if totalShrink <= 0 {
		return
	}

	for _, item := range items {
		if item.flexShrink > 0 {
			shrinkAmount := (item.flexShrink * item.mainSize / totalShrink) * overflow
			item.mainSize -= shrinkAmount
			if item.mainSize < 0 {
				item.mainSize = 0
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

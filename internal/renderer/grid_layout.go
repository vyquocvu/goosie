package renderer

import (
	"strconv"
	"strings"
)

// GridLayoutEngine handles grid layout calculations
type GridLayoutEngine struct {
	fontMetrics *FontMetrics
}

// NewGridLayoutEngine creates a new grid layout engine
func NewGridLayoutEngine(fm *FontMetrics) *GridLayoutEngine {
	return &GridLayoutEngine{
		fontMetrics: fm,
	}
}

// LayoutGridContainer performs grid layout on a container's children
func (gle *GridLayoutEngine) LayoutGridContainer(
	container *RenderNode,
	parentBox *LayoutBox,
	buildLayoutBox func(node *RenderNode, x, y, width float32) *LayoutBox,
) {
	if container == nil || len(container.Children) == 0 {
		return
	}

	// 1. Parse Grid Properties
	gridTemplateColumns := parseTrackList(container.ComputedStyle.GridTemplateColumns)
	gridTemplateRows := parseTrackList(container.ComputedStyle.GridTemplateRows)
	
	columnGap := float32(0)
	rowGap := float32(0)
	
	if container.ComputedStyle.Gap != "" {
		gap := parseLength(container.ComputedStyle.Gap, 16)
		columnGap = gap
		rowGap = gap
	}
	if container.ComputedStyle.ColumnGap != "" {
		columnGap = parseLength(container.ComputedStyle.ColumnGap, 16)
	}
	if container.ComputedStyle.RowGap != "" {
		rowGap = parseLength(container.ComputedStyle.RowGap, 16)
	}
	
	parentBox.Gap = columnGap 

	// 2. Determine container dimensions
	containerWidth := parentBox.Box.Width - parentBox.PaddingLeft - parentBox.PaddingRight
	
	// 3. Resolve explicit grid tracks
	if len(gridTemplateColumns) == 0 {
		gridTemplateColumns = []TrackSize{{Type: TrackTypeAuto, Value: 0}}
	}
	// For rows, start with what's defined

	// 4. Place items into the grid
	items := make([]*gridItem, 0, len(container.Children))
	maxColumnIndex := len(gridTemplateColumns)
	maxRowIndex := len(gridTemplateRows)
	if maxRowIndex == 0 { maxRowIndex = 1 }

	for _, child := range container.Children {
		if child.ComputedStyle != nil && child.ComputedStyle.Display == "none" {
			continue
		}
		
		item := &gridItem{
			node: child,
		}
		
		// Parse item placement
		if child.ComputedStyle != nil {
			var err error
			item.colStart, item.colEnd, err = parseGridPlacement(
				child.ComputedStyle.GridColumnStart,
				child.ComputedStyle.GridColumnEnd,
			)
			if err != nil {
				item.colStart = 0 
				item.colEnd = 0   
			}
			
			item.rowStart, item.rowEnd, err = parseGridPlacement(
				child.ComputedStyle.GridRowStart,
				child.ComputedStyle.GridRowEnd,
			)
			if err != nil {
				item.rowStart = 0
				item.rowEnd = 0
			}
		} else {
			item.colStart = 0
			item.colEnd = 0
			item.rowStart = 0
			item.rowEnd = 0
		}
		
		items = append(items, item)
	}

	// Simple Auto-Placement Algorithm
	occupied := make(map[int]map[int]bool)
	
	nextRow := 1
	nextCol := 1
	
	for _, item := range items {
		// If both are auto, place in next available cell
		if item.colStart == 0 && item.rowStart == 0 {
			for {
				if !isOccupied(occupied, nextRow, nextCol) {
					item.colStart = nextCol
					item.colEnd = nextCol + 1
					item.rowStart = nextRow
					item.rowEnd = nextRow + 1
					
					markOccupied(occupied, nextRow, nextCol)
					
					// Advance pointer
					nextCol++
					if nextCol > len(gridTemplateColumns) {
						nextCol = 1
						nextRow++
					}
					break
				}
				nextCol++
				if nextCol > len(gridTemplateColumns) {
					nextCol = 1
					nextRow++
				}
			}
		} else {
			// Logic for fixed placement (simplified)
			if item.colStart != 0 && item.rowStart == 0 {
				// Fixed col, find row
				r := 1
				for {
					if !isOccupied(occupied, r, item.colStart) {
						item.rowStart = r
						item.rowEnd = r + 1
						if item.colEnd == 0 { item.colEnd = item.colStart + 1 }
						markOccupied(occupied, r, item.colStart)
						break
					}
					r++
				}
			} else if item.rowStart != 0 && item.colStart == 0 {
				// Fixed row, find col
				c := 1
				for {
					if !isOccupied(occupied, item.rowStart, c) {
						item.colStart = c
						item.colEnd = c + 1
						if item.rowEnd == 0 { item.rowEnd = item.rowStart + 1 }
						markOccupied(occupied, item.rowStart, c)
						break
					}
					c++
				}
			} else {
				// Fully defined
				if item.colEnd == 0 { item.colEnd = item.colStart + 1 }
				if item.rowEnd == 0 { item.rowEnd = item.rowStart + 1 }
				markOccupied(occupied, item.rowStart, item.colStart)
			}
		}

		// Update max dimensions
		if item.colEnd-1 > maxColumnIndex {
			maxColumnIndex = item.colEnd - 1
		}
		if item.rowEnd-1 > maxRowIndex {
			maxRowIndex = item.rowEnd - 1
		}
	}

	// 5. Expand tracks for implicit items
	for len(gridTemplateColumns) < maxColumnIndex {
		gridTemplateColumns = append(gridTemplateColumns, TrackSize{Type: TrackTypeAuto, Value: 0})
	}
	for len(gridTemplateRows) < maxRowIndex {
		gridTemplateRows = append(gridTemplateRows, TrackSize{Type: TrackTypeAuto, Value: 0})
	}

	// 6. Calculate Track Sizes
	colWidths := gle.calculateTrackSizes(gridTemplateColumns, containerWidth, columnGap)
	
	// Pre-layout items to determine heights for auto rows
	for _, item := range items {
		colStartIdx := item.colStart - 1
		colEndIdx := item.colEnd - 1
		
		// Safety check
		if colStartIdx < 0 { colStartIdx = 0 }
		if colEndIdx > len(colWidths) { colEndIdx = len(colWidths) }
		
		itemWidth := float32(0)
		for i := colStartIdx; i < colEndIdx; i++ {
			itemWidth += colWidths[i]
		}
		// Add gaps between spanned tracks
		if colEndIdx > colStartIdx + 1 {
			itemWidth += float32(colEndIdx - colStartIdx - 1) * columnGap
		}
		
		// Build box to get content height
		item.layoutBox = buildLayoutBox(item.node, 0, 0, itemWidth)
		
		if item.layoutBox != nil {
			item.layoutBox.GridColumnStart = item.colStart
			item.layoutBox.GridColumnEnd = item.colEnd
			item.layoutBox.GridRowStart = item.rowStart
			item.layoutBox.GridRowEnd = item.rowEnd
		}
	}
	
	// Now calculate row heights based on item heights
	rowHeights := gle.calculateRowHeights(gridTemplateRows, items, parentBox.Box.Height, rowGap) 
	
	// 7. Position items
	// Calculate accumulated positions
	colPositions := make([]float32, len(colWidths)+1)
	currentX := parentBox.Box.X + parentBox.PaddingLeft
	for i, w := range colWidths {
		colPositions[i] = currentX
		currentX += w + columnGap
	}
	colPositions[len(colWidths)] = currentX 
	
	rowPositions := make([]float32, len(rowHeights)+1)
	currentY := parentBox.Box.Y + parentBox.PaddingTop
	for i, h := range rowHeights {
		rowPositions[i] = currentY
		currentY += h + rowGap
	}
	rowPositions[len(rowHeights)] = currentY
	
	// Assign final geometry
	for _, item := range items {
		if item.layoutBox == nil { continue }
		
		cStart := item.colStart - 1
		// cEnd := item.colEnd - 1
		rStart := item.rowStart - 1
		// rEnd := item.rowEnd - 1
		
		// Calculate width again or just take from cache
		// Correct logic: layoutbox width is content width. 
		// If stretched, we force it? For now, we trust buildLayoutBox.
		
		// Set position
		if cStart >= 0 && cStart < len(colPositions) {
			item.layoutBox.Box.X = colPositions[cStart]
		}
		if rStart >= 0 && rStart < len(rowPositions) {
			item.layoutBox.Box.Y = rowPositions[rStart]
		}
		
		parentBox.AddChild(item.layoutBox)
	}
	
	// Update container height
	if len(rowHeights) > 0 {
		totalH := float32(0)
		for _, h := range rowHeights {
			totalH += h
		}
		totalH += float32(len(rowHeights)-1) * rowGap
		
		// This sets the content height of the grid container
		// We add padding, but usually the outer layout (buildLayoutBox caller) handles adding padding to total height?
		// In layout.go: layoutBox.Box.Height = currentY - (y + layoutBox.MarginTop)
		// Here we set the childY effectively.
		// parentBox.Box.Height should reflect content size + padding.
		// LayoutEngine expects us to return the CHILD Y... wait. 
		// LayoutFlexContainer returns nothing, it modifies parentBox directly?
		// No, let's check layout.go usage.
		// In computeElementLayout:
		// le.flexLayoutEngine.LayoutFlexContainer(node, layoutBox, le.buildLayoutBox)
		// then it iterates children to find max Y.
		
		// So we just need to set children Y correctly.
		// We don't need to perform layoutBox.Box.Height assignment, layout.go does it based on children?
		// Wait, flex layout does:
		// for child in children: childY = endY...
		// So we are good if we enable that loop for grid too.
	}
}

// calculateTrackSizes resolves track sizes to pixels
func (gle *GridLayoutEngine) calculateTrackSizes(tracks []TrackSize, availableSpace float32, gap float32) []float32 {
	sizes := make([]float32, len(tracks))
	
	remainingSpace := availableSpace
	totalGapSpace := float32(0)
	if len(tracks) > 1 {
		totalGapSpace = float32(len(tracks)-1) * gap
	}
	remainingSpace -= totalGapSpace
	
	totalFr := float32(0)
	
	for i, t := range tracks {
		if t.Type == TrackTypePx {
			sizes[i] = t.Value
			remainingSpace -= t.Value
		} else if t.Type == TrackTypePercent {
			px := (t.Value / 100.0) * availableSpace
			sizes[i] = px
			remainingSpace -= px
		} else if t.Type == TrackTypeFr {
			totalFr += t.Value
		}
	}
	
	if totalFr > 0 && remainingSpace > 0 {
		frUnit := remainingSpace / totalFr
		for i, t := range tracks {
			if t.Type == TrackTypeFr {
				sizes[i] = t.Value * frUnit
			}
		}
	} else if totalFr > 0 {
		for i, t := range tracks {
			if t.Type == TrackTypeFr {
				sizes[i] = 0
			}
		}
	}
	
	// Handle auto tracks (simplified)
	autoCount := float32(0)
	for _, t := range tracks { if t.Type == TrackTypeAuto { autoCount++ } }
	
	if autoCount > 0 {
		if totalFr == 0 && remainingSpace > 0 {
			autoWidth := remainingSpace / autoCount
			for i, t := range tracks {
				if t.Type == TrackTypeAuto {
					sizes[i] = autoWidth
				}
			}
		} else {
			// fallback
			for i, t := range tracks {
				if t.Type == TrackTypeAuto && sizes[i] == 0 {
					sizes[i] = 100 // default fallback
				}
			}
		}
	}
	
	return sizes
}

// calculateRowHeights calculates row heights based on template and content
func (gle *GridLayoutEngine) calculateRowHeights(tracks []TrackSize, items []*gridItem, containerHeight float32, gap float32) []float32 {
	heights := make([]float32, len(tracks))
	
	for i, t := range tracks {
		if t.Type == TrackTypePx {
			heights[i] = t.Value
		} else if t.Type == TrackTypePercent {
			if containerHeight > 0 {
				heights[i] = (t.Value / 100.0) * containerHeight
			} else {
				// treat as auto if container height indefinite?
				// Fallthrough to auto logic below
			}
		}
	}
	
	// Auto/Content heights
	for i, t := range tracks {
		if t.Type == TrackTypeAuto || (t.Type == TrackTypePercent && containerHeight <= 0) {
			maxH := float32(0)
			rowIdx := i + 1 
			
			for _, item := range items {
				if item.rowStart <= rowIdx && item.rowEnd > rowIdx {
					// Single row span for simplicity
					if item.rowEnd - item.rowStart == 1 {
						if item.layoutBox != nil && item.layoutBox.Box.Height > maxH {
							maxH = item.layoutBox.Box.Height
						}
					}
				}
			}
			// ensure min height
			if maxH == 0 { maxH = 20 } // minimal row height?
			heights[i] = maxH
		}
	}
	
	return heights
}


// --- Helper Types & Functions ---

type TrackType int
const (
	TrackTypePx TrackType = iota
	TrackTypePercent
	TrackTypeFr
	TrackTypeAuto
)

type TrackSize struct {
	Type  TrackType
	Value float32
}

type gridItem struct {
	node      *RenderNode
	layoutBox *LayoutBox
	colStart  int
	colEnd    int
	rowStart  int
	rowEnd    int
}

func parseTrackList(value string) []TrackSize {
	if value == "" {
		return nil
	}
	parts := strings.Fields(value)
	tracks := make([]TrackSize, 0, len(parts))
	
	for _, part := range parts {
		if part == "auto" {
			tracks = append(tracks, TrackSize{Type: TrackTypeAuto})
		} else if strings.HasSuffix(part, "fr") {
			val, _ := strconv.ParseFloat(strings.TrimSuffix(part, "fr"), 32)
			tracks = append(tracks, TrackSize{Type: TrackTypeFr, Value: float32(val)})
		} else if strings.HasSuffix(part, "%") {
			val, _ := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 32)
			tracks = append(tracks, TrackSize{Type: TrackTypePercent, Value: float32(val)})
		} else {
			val := parseLength(part, 16) 
			tracks = append(tracks, TrackSize{Type: TrackTypePx, Value: val})
		}
	}
	return tracks
}

func parseGridPlacement(startVal, endVal string) (int, int, error) {
	start := 0
	end := 0 
	
	// Handle "span X" future logic here
	
	if s, err := strconv.Atoi(startVal); err == nil {
		start = s
	}
	
	if e, err := strconv.Atoi(endVal); err == nil {
		end = e
	}
	
	if start == 0 && end != 0 {
		start = end - 1
	}
	
	if start != 0 && end == 0 {
		end = start + 1
	}
	
	return start, end, nil
}

func isOccupied(occupied map[int]map[int]bool, row, col int) bool {
	if r, ok := occupied[row]; ok {
		return r[col]
	}
	return false
}

func markOccupied(occupied map[int]map[int]bool, row, col int) {
	if _, ok := occupied[row]; !ok {
		occupied[row] = make(map[int]bool)
	}
	occupied[row][col] = true
}

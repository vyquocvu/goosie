package renderer

// FloatEntry represents a placed float's bounds and side.
type FloatEntry struct {
	Box  Rect
	Side string // "left" or "right"
}

// FloatContext tracks active float boxes in a layout context.
type FloatContext struct {
	containerX     float32
	containerY     float32
	containerWidth float32
	floats         []FloatEntry
}

// NewFloatContext creates a new FloatContext.
func NewFloatContext(containerX, containerY, containerWidth float32) *FloatContext {
	return &FloatContext{
		containerX:     containerX,
		containerY:     containerY,
		containerWidth: containerWidth,
		floats:         make([]FloatEntry, 0),
	}
}

// PlaceFloat positions a new float element, constrained by the containing block's X and width.
// For float: left, it finds the leftmost available position at or below currentY.
// For float: right, it finds the rightmost available position at or below currentY.
// If there are existing floats that overlap with the new float, it pushes the Y coordinate down.
func (fc *FloatContext) PlaceFloat(box *LayoutBox, floatDir string, currentY float32, containingBlockX, containingBlockWidth float32) (float32, float32) {
	width := box.Box.Width + box.MarginLeft + box.MarginRight
	height := box.Box.Height + box.MarginTop + box.MarginBottom

	targetY := currentY
	for {
		leftOffset, bfcAvailableWidth := fc.GetAvailableWidth(targetY, height)

		bfcLeft := fc.containerX + leftOffset
		bfcRight := fc.containerX + leftOffset + bfcAvailableWidth

		leftBoundary := maxFloat32(containingBlockX, bfcLeft)
		rightBoundary := minFloat32(containingBlockX+containingBlockWidth, bfcRight)

		availableWidth := rightBoundary - leftBoundary
		if availableWidth < 0 {
			availableWidth = 0
		}

		// If width fits, place it
		if width <= availableWidth {
			var x float32
			if floatDir == "left" {
				x = leftBoundary + box.MarginLeft
			} else { // "right"
				x = rightBoundary - width + box.MarginLeft
			}
			return x, targetY + box.MarginTop
		}

		// Otherwise, find the lowest bottom of any active float that overlaps the targetY range,
		// and move targetY there to try again.
		lowestY := targetY + 1.0 // progress by at least 1px
		foundOverlap := false
		for _, f := range fc.floats {
			// If float overlaps vertically with [targetY, targetY + height]
			if f.Box.Y < targetY+height && f.Box.Y+f.Box.Height > targetY {
				if f.Box.Y+f.Box.Height > lowestY {
					lowestY = f.Box.Y + f.Box.Height
					foundOverlap = true
				}
			}
		}
		if !foundOverlap {
			// If no overlapping floats but still doesn't fit, we have to place it here anyway
			var x float32
			if floatDir == "left" {
				x = leftBoundary + box.MarginLeft
			} else {
				x = rightBoundary - width + box.MarginLeft
			}
			return x, targetY + box.MarginTop
		}
		targetY = lowestY
	}
}

// GetAvailableWidth computes the available horizontal span at a given Y position and height,
// accounting for any left and right floats that intersect that vertical range.
func (fc *FloatContext) GetAvailableWidth(y, height float32) (float32, float32) {
	leftBoundary := float32(0.0)
	rightBoundary := fc.containerWidth

	for _, f := range fc.floats {
		// Vertically overlapping?
		if f.Box.Y < y+height && f.Box.Y+f.Box.Height > y {
			// Shift boundaries relative to containerX
			fx := f.Box.X - fc.containerX
			fWidth := f.Box.Width

			if f.Side == "left" {
				rightOfFloat := fx + fWidth
				if rightOfFloat > leftBoundary {
					leftBoundary = rightOfFloat
				}
			} else if f.Side == "right" {
				leftOfFloat := fx
				if leftOfFloat < rightBoundary {
					rightBoundary = leftOfFloat
				}
			}
		}
	}

	available := rightBoundary - leftBoundary
	if available < 0 {
		available = 0
	}
	return leftBoundary, available
}

// AddFloat registers a placed float's layout box (including margins) into the exclusion tracking.
func (fc *FloatContext) AddFloat(box *LayoutBox, floatDir string) {
	// Include margins in the layout box exclusion rect
	entry := FloatEntry{
		Box: Rect{
			X:      box.Box.X - box.MarginLeft,
			Y:      box.Box.Y - box.MarginTop,
			Width:  box.Box.Width + box.MarginLeft + box.MarginRight,
			Height: box.Box.Height + box.MarginTop + box.MarginBottom,
		},
		Side: floatDir,
	}
	fc.floats = append(fc.floats, entry)
}

// ClearFloat moves the current Y position down past any active left, right, or both floats.
func (fc *FloatContext) ClearFloat(clearType string, currentY float32) float32 {
	if clearType == "none" || clearType == "" {
		return currentY
	}
	maxY := currentY
	for _, f := range fc.floats {
		if (clearType == "left" && f.Side == "left") ||
			(clearType == "right" && f.Side == "right") ||
			(clearType == "both") {
			bottom := f.Box.Y + f.Box.Height
			if bottom > maxY {
				maxY = bottom
			}
		}
	}
	return maxY
}

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

// PlaceFloat positions a new float element at or below currentY within the containing block.
func (fc *FloatContext) PlaceFloat(box *LayoutBox, floatDir string, currentY float32, containingBlockX, containingBlockWidth float32) (float32, float32) {
	width := box.Box.Width + box.MarginLeft + box.MarginRight
	height := box.Box.Height + box.MarginTop + box.MarginBottom

	targetY := currentY
	for {
		leftOffset, bfcAvailableWidth := fc.GetAvailableWidth(targetY, height)

		bfcLeft := fc.containerX + leftOffset
		bfcRight := fc.containerX + leftOffset + bfcAvailableWidth

		leftBoundary := max(containingBlockX, bfcLeft)
		rightBoundary := min(containingBlockX+containingBlockWidth, bfcRight)

		availableWidth := max(0, rightBoundary-leftBoundary)

		if width <= availableWidth {
			var x float32
			if floatDir == "left" {
				x = leftBoundary + box.MarginLeft
			} else {
				x = rightBoundary - width + box.MarginLeft
			}
			return x, targetY + box.MarginTop
		}

		lowestY := targetY + 1.0
		foundOverlap := false
		for _, f := range fc.floats {
			if f.Box.Y < targetY+height && f.Box.Y+f.Box.Height > targetY {
				if f.Box.Y+f.Box.Height > lowestY {
					lowestY = f.Box.Y + f.Box.Height
					foundOverlap = true
				}
			}
		}
		if !foundOverlap {
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

// GetAvailableWidth computes the available horizontal span at a given Y position and height.
func (fc *FloatContext) GetAvailableWidth(y, height float32) (float32, float32) {
	leftBoundary := float32(0.0)
	rightBoundary := fc.containerWidth

	for _, f := range fc.floats {
		if f.Box.Y < y+height && f.Box.Y+f.Box.Height > y {
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

	available := max(0, rightBoundary-leftBoundary)
	return leftBoundary, available
}

// AddFloat registers a placed float's layout box into exclusion tracking.
func (fc *FloatContext) AddFloat(box *LayoutBox, floatDir string) {
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

// ClearFloat moves the current Y position down past active floats matching clearType.
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

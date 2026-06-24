package renderer

import "strings"

// OverflowMode represents CSS overflow behavior.
type OverflowMode string

const (
	OverflowVisible OverflowMode = "visible"
	OverflowHidden  OverflowMode = "hidden"
	OverflowScroll  OverflowMode = "scroll"
	OverflowAuto    OverflowMode = "auto"
)

// ParseOverflowMode converts a string to OverflowMode.
func ParseOverflowMode(value string) OverflowMode {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "hidden":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	default:
		return OverflowVisible
	}
}

// ClipRegion represents a rectangular clipping region.
type ClipRegion struct {
	X, Y, Width, Height float32
	Enabled             bool
}

// NewClipRegion creates a new ClipRegion.
func NewClipRegion(x, y, width, height float32) ClipRegion {
	return ClipRegion{
		X:       x,
		Y:       y,
		Width:   width,
		Height:  height,
		Enabled: true,
	}
}

// Contains checks if a point is within the ClipRegion.
func (c ClipRegion) Contains(x, y float32) bool {
	if !c.Enabled {
		return true
	}
	return x >= c.X && x <= c.X+c.Width && y >= c.Y && y <= c.Y+c.Height
}

// Intersect computes the intersection of two ClipRegions.
func (c ClipRegion) Intersect(other ClipRegion) ClipRegion {
	if !c.Enabled {
		return other
	}
	if !other.Enabled {
		return c
	}

	x1 := c.X
	if other.X > x1 {
		x1 = other.X
	}

	y1 := c.Y
	if other.Y > y1 {
		y1 = other.Y
	}

	x2 := c.X + c.Width
	oX2 := other.X + other.Width
	if oX2 < x2 {
		x2 = oX2
	}

	y2 := c.Y + c.Height
	oY2 := other.Y + other.Height
	if oY2 < y2 {
		y2 = oY2
	}

	width := x2 - x1
	height := y2 - y1
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	return ClipRegion{
		X:       x1,
		Y:       y1,
		Width:   width,
		Height:  height,
		Enabled: true,
	}
}

// ScrollableContainer represents layout information for a scrollable box.
type ScrollableContainer struct {
	ContentWidth           float32
	ContentHeight          float32
	ViewportWidth          float32
	ViewportHeight         float32
	ScrollX                float32
	ScrollY                float32
	OverflowX              OverflowMode
	OverflowY              OverflowMode
	HasHorizontalScrollbar bool
	HasVerticalScrollbar   bool
}

// ShouldClipContent returns true if the content of this element should be clipped.
func ShouldClipContent(overflowMode OverflowMode) bool {
	return overflowMode == OverflowHidden || overflowMode == OverflowScroll || overflowMode == OverflowAuto
}

// NeedsScrollbar returns true if a scrollbar should be rendered for the given sizes.
func NeedsScrollbar(overflowMode OverflowMode, contentSize, viewportSize float32) bool {
	if overflowMode == OverflowScroll {
		return true
	}
	if overflowMode == OverflowAuto {
		return contentSize > viewportSize
	}
	return false
}

// ComputeTextOverflow cuts the text and appends ellipsis if it overflows the maxWidth.
func ComputeTextOverflow(text string, maxWidth float32, mode string, measureFunc func(string) float32) string {
	if mode != "ellipsis" || maxWidth <= 0 {
		return text
	}
	if measureFunc(text) <= maxWidth {
		return text
	}

	ellipsis := "..."
	ellipsisWidth := measureFunc(ellipsis)
	if ellipsisWidth >= maxWidth {
		return ""
	}

	// Simple binary search or scan back to find where ellipsis fits
	runes := []rune(text)
	low := 0
	high := len(runes)
	best := ""

	for low <= high {
		mid := (low + high) / 2
		candidate := string(runes[:mid]) + ellipsis
		if measureFunc(candidate) <= maxWidth {
			best = candidate
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	return best
}

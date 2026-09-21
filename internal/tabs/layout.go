package tabs

import "github.com/vyquocvu/goosie/internal/frame"

const (
	TabBarHeight   = 36
	TabWidth       = 200
	TabGap         = 1
	LeftMargin     = 8
	NewTabBtnSize  = 32
	CloseBtnSize   = 16
	CloseBtnMargin = 8
)

// TabRect returns the bounding rect for the tab at index idx given scrollOffset
// and total tabCount. contentWidth is the window width (unused for basic layout).
func TabRect(idx int, scrollOffset, tabCount, contentWidth int32) frame.Rect {
	x := int32(LeftMargin) + int32(idx)*(TabWidth+TabGap) - scrollOffset
	return frame.Rect4(x, 0, x+TabWidth, TabBarHeight)
}

// NewTabButtonRect returns the "+" button rect positioned after the last tab.
func NewTabButtonRect(tabCount int32, scrollOffset, contentWidth int32) frame.Rect {
	lastX := int32(LeftMargin) + tabCount*(TabWidth+TabGap) - TabGap - scrollOffset
	x := lastX + TabGap
	y := int32((TabBarHeight - NewTabBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(NewTabBtnSize), y+int32(NewTabBtnSize))
}

// CloseButtonRect returns the close button rect within a tab.
func CloseButtonRect(tabRect frame.Rect) frame.Rect {
	x := tabRect.X1 - int32(CloseBtnSize) - int32(CloseBtnMargin)
	y := int32((TabBarHeight - CloseBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(CloseBtnSize), y+int32(CloseBtnSize))
}

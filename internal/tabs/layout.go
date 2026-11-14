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
	MinTabWidth    = 80
	ChevronReserve = 56
	ChevronSize    = 24
)

func TabWidthFor(tabCount, contentWidth int32) int32 {
	if tabCount <= 0 {
		return TabWidth
	}
	avail := contentWidth - int32(LeftMargin) - int32(NewTabBtnSize) - int32(TabGap)
	w := avail/tabCount - int32(TabGap)
	if w > TabWidth {
		return TabWidth
	}
	if w < MinTabWidth {
		return MinTabWidth
	}
	return w
}

func TabRect(idx int, scrollOffset, tabCount, contentWidth int32) frame.Rect {
	w := TabWidthFor(tabCount, contentWidth)
	x := int32(LeftMargin) + int32(idx)*(w+int32(TabGap)) - scrollOffset
	return frame.Rect4(x, 0, x+w, TabBarHeight)
}

func NewTabButtonRect(tabCount int32, scrollOffset, contentWidth int32) frame.Rect {
	w := TabWidthFor(tabCount, contentWidth)
	lastX := int32(LeftMargin) + tabCount*(w+int32(TabGap)) - int32(TabGap) - scrollOffset
	x := lastX + int32(TabGap)
	y := int32((TabBarHeight - NewTabBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(NewTabBtnSize), y+int32(NewTabBtnSize))
}

func stripTotal(tabCount, contentWidth int32) int32 {
	if tabCount <= 0 {
		return 0
	}
	w := TabWidthFor(tabCount, contentWidth)
	return int32(LeftMargin) + tabCount*(w+int32(TabGap)) + int32(NewTabBtnSize)
}

func ChevronsShown(tabCount, contentWidth int32) bool {
	return stripTotal(tabCount, contentWidth) > contentWidth
}

func ChevronRects(tabCount, contentWidth int32) (left, right frame.Rect, ok bool) {
	if !ChevronsShown(tabCount, contentWidth) {
		return frame.Rect{}, frame.Rect{}, false
	}
	y0 := int32(TabBarHeight-ChevronSize) / 2
	rx0 := contentWidth - 4 - ChevronSize
	lx0 := rx0 - 4 - ChevronSize
	return frame.Rect4(lx0, y0, lx0+ChevronSize, y0+ChevronSize),
		frame.Rect4(rx0, y0, rx0+ChevronSize, y0+ChevronSize), true
}

func TabScrollPage(tabCount, contentWidth int32) int32 {
	w := TabWidthFor(tabCount, contentWidth)
	vis := contentWidth - int32(LeftMargin)
	if ChevronsShown(tabCount, contentWidth) {
		vis -= ChevronReserve
	}
	step := vis - w
	if step < w {
		step = w
	}
	if step < 1 {
		step = 1
	}
	return step
}

func StripMaxOffset(tabCount, contentWidth int32) int32 {
	if !ChevronsShown(tabCount, contentWidth) {
		return 0
	}
	return stripTotal(tabCount, contentWidth) - (contentWidth - ChevronReserve)
}

func EnsureTabVisible(idx int, tabCount, contentWidth, current int32) int32 {
	max := StripMaxOffset(tabCount, contentWidth)
	if max <= 0 {
		return 0
	}
	reserve := int32(0)
	if ChevronsShown(tabCount, contentWidth) {
		reserve = ChevronReserve
	}
	r := TabRect(idx, 0, tabCount, contentWidth)
	off := current
	if r.X0-off < int32(LeftMargin) {
		off = r.X0 - int32(LeftMargin)
	}
	if r.X1-off > contentWidth-reserve-int32(NewTabBtnSize)-int32(TabGap) {
		off = r.X1 - (contentWidth - reserve - int32(NewTabBtnSize) - int32(TabGap))
	}
	if off < 0 {
		return 0
	}
	if off > max {
		return max
	}
	return off
}

func CloseButtonRect(tabRect frame.Rect) frame.Rect {
	x := tabRect.X1 - int32(CloseBtnSize) - int32(CloseBtnMargin)
	y := int32((TabBarHeight - CloseBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(CloseBtnSize), y+int32(CloseBtnSize))
}

const DragThreshold = 6

func DropIndexAt(x, tabCount, contentWidth, scrollOffset int32) int {
	if tabCount <= 0 {
		return 0
	}
	w := TabWidthFor(tabCount, contentWidth)
	pitch := w + int32(TabGap)
	slot := (x - int32(LeftMargin) + scrollOffset) / pitch
	if slot < 0 {
		return 0
	}
	if slot > tabCount-1 {
		return int(tabCount - 1)
	}
	return int(slot)
}

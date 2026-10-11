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
	// MinTabWidth is the narrowest a tab shrinks under strip pressure: wide
	// enough for an ellipsis plus the close affordance. Beyond it the strip
	// scrolls under the chevron controls; every tab stays reachable.
	MinTabWidth = 72
	// ChevronReserve is the strip's right end kept for the two scroll
	// chevrons once tabs overflow: two 24px buttons, a 4px gap between them,
	// and a 4px right margin.
	ChevronReserve = 56
	ChevronSize    = 24
)

// TabWidthFor shares contentWidth across tabCount tabs so the whole strip,
// including the new-tab button, fits: full TabWidth while it fits, shrinking
// uniformly to MinTabWidth under pressure. Drawing (DrawTabBar) and hit
// testing (the host's tab-bar click handler) both derive rects from this one
// function, so a press always lands on the tab the pixels show.
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

// TabRect returns the bounding rect for the tab at index idx given scrollOffset
// and total tabCount. contentWidth is the logical tab-strip width the tabs
// shrink to fit; scrollOffset shifts the strip under the chevron controls.
func TabRect(idx int, scrollOffset, tabCount, contentWidth int32) frame.Rect {
	w := TabWidthFor(tabCount, contentWidth)
	x := int32(LeftMargin) + int32(idx)*(w+int32(TabGap)) - scrollOffset
	return frame.Rect4(x, 0, x+w, TabBarHeight)
}

// NewTabButtonRect returns the "+" button rect positioned after the last tab.
func NewTabButtonRect(tabCount int32, scrollOffset, contentWidth int32) frame.Rect {
	w := TabWidthFor(tabCount, contentWidth)
	lastX := int32(LeftMargin) + tabCount*(w+int32(TabGap)) - int32(TabGap) - scrollOffset
	x := lastX + int32(TabGap)
	y := int32((TabBarHeight - NewTabBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(NewTabBtnSize), y+int32(NewTabBtnSize))
}

// stripTotal is the strip's full logical width: tabs plus the new-tab button.
// Chevrons overlay the right end rather than shrinking tabs further, so the
// total is independent of scrolling.
func stripTotal(tabCount, contentWidth int32) int32 {
	if tabCount <= 0 {
		return 0
	}
	w := TabWidthFor(tabCount, contentWidth)
	return int32(LeftMargin) + tabCount*(w+int32(TabGap)) + int32(NewTabBtnSize)
}

// ChevronsShown reports whether a strip of tabCount tabs over contentWidth
// logical pixels needs its scroll controls: exactly when shrinking to
// MinTabWidth still does not fit.
func ChevronsShown(tabCount, contentWidth int32) bool {
	return stripTotal(tabCount, contentWidth) > contentWidth
}

// ChevronRects returns the left/right scroll-button rects pinned to the
// strip's right end, or ok=false when the strip fits and has no controls.
// Draw and hit-testing share these rects so a press lands on the control the
// pixels show.
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

// TabScrollPage is one chevron click of travel: the visible strip minus one
// tab of context, floored at one tab so a narrow strip still moves.
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

// StripMaxOffset is the legal scroll range for a strip of tabCount tabs over
// contentWidth logical pixels: zero while shrinking fits everything, and once
// chevrons show, the offset that parks the new-tab button just left of them.
func StripMaxOffset(tabCount, contentWidth int32) int32 {
	if !ChevronsShown(tabCount, contentWidth) {
		return 0
	}
	return stripTotal(tabCount, contentWidth) - (contentWidth - ChevronReserve)
}

// EnsureTabVisible returns the strip offset keeping idx on screen: the offset
// moves just enough to show a clipped tab, keeps the new-tab button reachable
// past the last tab, and never leaves [0, StripMaxOffset]. Pure so draw,
// hit-testing and tab switching share one definition of visible.
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

// CloseButtonRect returns the close button rect within a tab.
func CloseButtonRect(tabRect frame.Rect) frame.Rect {
	x := tabRect.X1 - int32(CloseBtnSize) - int32(CloseBtnMargin)
	y := int32((TabBarHeight - CloseBtnSize) / 2)
	return frame.Rect4(x, y, x+int32(CloseBtnSize), y+int32(CloseBtnSize))
}

// DragThreshold is how far a press must travel (logical pixels) before it
// becomes a tab drag rather than a click.
const DragThreshold = 6

// DropIndexAt maps a strip X to the insertion slot whose center is nearest,
// clamped to [0, tabCount-1]. scrollOffset shifts into content coordinates
// so draw, hit-testing and switching share one definition of the slot.
func DropIndexAt(x, tabCount, contentWidth, scrollOffset int32) int {
	if tabCount <= 0 {
		return 0
	}
	w := TabWidthFor(tabCount, contentWidth)
	pitch := w + int32(TabGap)
	// Slot i spans [LeftMargin + i*pitch, LeftMargin + (i+1)*pitch): the
	// boundary falls on tab edges, splitting the gap evenly enough that a
	// press on a tab maps to its own slot. Negative numerators truncate
	// toward zero in Go, which is harmless: every negative result clamps
	// to 0 below.
	slot := (x - int32(LeftMargin) + scrollOffset) / pitch
	if slot < 0 {
		return 0
	}
	if slot > tabCount-1 {
		return int(tabCount - 1)
	}
	return int(slot)
}

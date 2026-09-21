package tabs

import "github.com/vyquocvu/goosie/internal/frame"

var (
	tabBarBg       = frame.RGB(235, 235, 235)
	activeTabBg    = frame.RGB(255, 255, 255)
	inactiveTabBg  = frame.RGB(220, 220, 220)
	tabBorderColor = frame.RGB(180, 180, 180)
	tabTextColor   = frame.RGB(80, 80, 80)
	tabTextActive  = frame.RGB(30, 30, 30)
	newTabBtnColor = frame.RGB(120, 120, 120)
	closeBtnColor  = frame.RGB(140, 140, 140)
	activeHighlight = frame.RGB(70, 140, 220)
)

// DrawTabBar renders the tab bar onto the top of buf.
func DrawTabBar(buf *frame.Bitmap, mgr *TabManager, scrollOffset int32) {
	if buf == nil || buf.Empty() {
		return
	}
	w := int32(buf.W)
	barRect := frame.Rect4(0, 0, w, TabBarHeight)
	buf.FillRect(barRect, tabBarBg, nil)

	tabsList := mgr.Tabs()
	activeIdx := mgr.ActiveIndex()

	for i, tab := range tabsList {
		r := TabRect(i, scrollOffset, int32(len(tabsList)), w)
		if r.X1 <= 0 || r.X0 >= w {
			continue
		}
		bg := inactiveTabBg
		textColor := tabTextColor
		if i == activeIdx {
			bg = activeTabBg
			textColor = tabTextActive
		}
		buf.FillRect(r, bg, nil)

		if i == activeIdx {
			buf.FillRect(frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+2), activeHighlight, nil)
		}

		buf.FillRect(frame.Rect4(r.X0, r.Y0, r.X0+1, r.Y1), tabBorderColor, nil)
		buf.FillRect(frame.Rect4(r.X1-1, r.Y0, r.X1, r.Y1), tabBorderColor, nil)
		buf.FillRect(frame.Rect4(r.X0, r.Y0, r.X1, r.Y0+1), tabBorderColor, nil)

		if i != activeIdx {
			buf.FillRect(frame.Rect4(r.X0, r.Y1-1, r.X1, r.Y1), tabBorderColor, nil)
		}

		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		drawTabText(buf, title, r, textColor)

		closeR := CloseButtonRect(r)
		drawCloseButton(buf, closeR, closeBtnColor)
	}

	buf.FillRect(frame.Rect4(0, TabBarHeight-1, w, TabBarHeight), tabBorderColor, nil)

	if len(tabsList) > 0 {
		btnR := NewTabButtonRect(int32(len(tabsList)), scrollOffset, w)
		if btnR.X0 < w {
			drawNewTabButton(buf, btnR, newTabBtnColor)
		}
	}
}

func drawTabText(buf *frame.Bitmap, text string, tabRect frame.Rect, color frame.Color) {
	textX := tabRect.X0 + 12
	textY := tabRect.Y0 + (TabBarHeight-6)/2 + 1
	maxW := tabRect.W() - CloseBtnSize - CloseBtnMargin - 24
	if maxW < 0 {
		maxW = 0
	}
	textW := int32(len(text)) * 5
	if textW > maxW {
		textW = maxW
	}
	if textW > 0 {
		buf.FillRect(frame.Rect4(textX, textY, textX+textW, textY+6), color, nil)
	}
}

func drawCloseButton(buf *frame.Bitmap, r frame.Rect, c frame.Color) {
	size := r.W()
	if size < 4 {
		return
	}
	inset := int32(4)
	x0 := r.X0 + inset
	y0 := r.Y0 + inset
	x1 := r.X1 - inset
	y1 := r.Y1 - inset
	if x1 <= x0 || y1 <= y0 {
		return
	}
	for i := int32(0); x0+i < x1 && y0+i < y1; i++ {
		buf.FillRect(frame.Rect4(x0+i, y0+i, x0+i+1, y0+i+1), c, nil)
		buf.FillRect(frame.Rect4(x1-1-i, y0+i, x1-i, y0+i+1), c, nil)
	}
}

func drawNewTabButton(buf *frame.Bitmap, r frame.Rect, c frame.Color) {
	cx := (r.X0 + r.X1) / 2
	cy := (r.Y0 + r.Y1) / 2
	half := int32(7)
	// Horizontal bar
	buf.FillRect(frame.Rect4(cx-half, cy, cx+half+1, cy+1), c, nil)
	// Vertical bar
	buf.FillRect(frame.Rect4(cx, cy-half, cx+1, cy+half+1), c, nil)
}

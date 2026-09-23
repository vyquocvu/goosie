package tabs

import (
	"math"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

var (
	tabBarBg       = frame.RGB(230, 230, 230)
	activeTabBg    = frame.RGB(255, 255, 255)
	tabHairline    = frame.RGB(214, 214, 214)
	tabTextColor   = frame.RGB(95, 95, 95)
	tabTextActive  = frame.RGB(35, 35, 35)
	newTabBtnColor = frame.RGB(120, 120, 120)
	closeBtnColor  = frame.RGB(145, 145, 145)
	// tabRadius rounds the top corners of the active tab. The toolbar below is
	// the same white, so the open tab reads as connected to the page chrome.
	tabRadius = int32(8)
	titleSize = int32(14)
)

// DrawTabBar renders the tab bar onto the top of buf. Layout is computed in
// logical pixels and multiplied by scale, so a HiDPI window gets a chrome of
// the same physical size a 1x window shows, not a shrunken one.
func DrawTabBar(buf *frame.Bitmap, mgr *TabManager, scrollOffset int32, fonts *raster.Fonts, scale int32) {
	if buf == nil || buf.Empty() {
		return
	}
	if scale < 1 {
		scale = 1
	}
	w := int32(buf.W) / scale
	barH := int32(TabBarHeight) * scale
	buf.FillRect(frame.Rect4(0, 0, int32(buf.W), barH), tabBarBg, nil)
	buf.FillRect(frame.Rect4(0, barH-scale, int32(buf.W), barH), tabHairline, nil)

	tabsList := mgr.Tabs()
	activeIdx := mgr.ActiveIndex()

	for i, tab := range tabsList {
		lr := TabRect(i, scrollOffset, int32(len(tabsList)), w)
		r := scaleRect(lr, scale)
		if r.X1 <= 0 || r.X0 >= int32(buf.W) {
			continue
		}
		textColor := tabTextColor
		if i == activeIdx {
			fillRoundedTop(buf, r, tabRadius*scale, activeTabBg)
			textColor = tabTextActive
		}

		title := tab.Title
		if title == "" {
			title = "New Tab"
		}
		drawTabText(buf, title, r, textColor, fonts, scale)

		drawCloseButton(buf, scaleRect(CloseButtonRect(lr), scale), closeBtnColor, scale)
	}

	if len(tabsList) > 0 {
		btnR := scaleRect(NewTabButtonRect(int32(len(tabsList)), scrollOffset, w), scale)
		if btnR.X0 < int32(buf.W) {
			drawNewTabButton(buf, btnR, newTabBtnColor, scale)
		}
	}
}

func scaleRect(r frame.Rect, scale int32) frame.Rect {
	return frame.Rect4(r.X0*scale, r.Y0*scale, r.X1*scale, r.Y1*scale)
}

// fillRoundedTop fills r with its two top corners rounded, leaving the bottom
// edge square so the tab meets the toolbar without a seam.
func fillRoundedTop(buf *frame.Bitmap, r frame.Rect, radius int32, c frame.Color) {
	if radius > r.H() {
		radius = r.H()
	}
	for dy := int32(0); dy < radius; dy++ {
		d := radius - dy
		cut := radius - int32(math.Sqrt(float64(radius*radius-d*d)))
		buf.FillRect(frame.Rect4(r.X0+cut, r.Y0+dy, r.X1-cut, r.Y0+dy+1), c, nil)
	}
	buf.FillRect(frame.Rect4(r.X0, r.Y0+radius, r.X1, r.Y1), c, nil)
}

func drawTabText(buf *frame.Bitmap, text string, tabRect frame.Rect, color frame.Color, fonts *raster.Fonts, scale int32) {
	if fonts == nil {
		return
	}
	size := titleSize * scale
	textX := tabRect.X0 + 12*scale
	baseline := tabRect.Y0 + (tabRect.H()-size)/2 + size*4/5
	maxW := tabRect.W() - (CloseBtnSize+CloseBtnMargin)*scale - 24*scale
	if maxW < 0 {
		maxW = 0
	}

	penX := textX
	runes := []rune(text)
	for i, rn := range runes {
		g := fonts.Glyph(size, rn, frame.FontSlot{})
		if !g.Ok || g.Mask == nil {
			penX += g.Advance
			continue
		}

		if penX+g.Advance-textX > maxW {
			if i > 0 && len(runes) > 1 {
				for _, dot := range "..." {
					dg := fonts.Glyph(size, dot, frame.FontSlot{})
					if dg.Ok && dg.Mask != nil {
						origin := frame.Point{X: penX + dg.Bounds.X0, Y: baseline + dg.Bounds.Y0}
						buf.BlitMask(dg.Mask, origin, color, buf.Bounds(), nil)
						penX += dg.Advance
					}
				}
			}
			break
		}

		origin := frame.Point{X: penX + g.Bounds.X0, Y: baseline + g.Bounds.Y0}
		buf.BlitMask(g.Mask, origin, color, buf.Bounds(), nil)
		penX += g.Advance
	}
}

func drawCloseButton(buf *frame.Bitmap, r frame.Rect, c frame.Color, scale int32) {
	size := r.W()
	if size < 4*scale {
		return
	}
	inset := int32(4) * scale
	x0 := r.X0 + inset
	y0 := r.Y0 + inset
	x1 := r.X1 - inset
	y1 := r.Y1 - inset
	if x1 <= x0 || y1 <= y0 {
		return
	}
	for i := int32(0); x0+i < x1 && y0+i < y1; i++ {
		buf.FillRect(frame.Rect4(x0+i, y0+i, x0+i+scale, y0+i+scale), c, nil)
		buf.FillRect(frame.Rect4(x1-scale-i, y0+i, x1-i, y0+i+scale), c, nil)
	}
}

func drawNewTabButton(buf *frame.Bitmap, r frame.Rect, c frame.Color, scale int32) {
	cx := (r.X0 + r.X1) / 2
	cy := (r.Y0 + r.Y1) / 2
	half := int32(7) * scale
	buf.FillRect(frame.Rect4(cx-half, cy, cx+half+scale, cy+scale), c, nil)
	buf.FillRect(frame.Rect4(cx, cy-half, cx+scale, cy+half+scale), c, nil)
}

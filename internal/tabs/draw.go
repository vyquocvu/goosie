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
	n := int32(len(tabsList))
	dragID, dragX, dragging := mgr.Dragging()

	for i, tab := range tabsList {
		// The dragged tab leaves its slot and rides the pointer, drawn
		// last on top.
		if dragging && tab.ID() == dragID {
			continue
		}
		lr := TabRect(i, scrollOffset, n, w)
		r := scaleRect(lr, scale)
		if r.X1 <= 0 || r.X0 >= int32(buf.W) {
			continue
		}
		drawTabBody(buf, tab, lr, r, i == activeIdx, fonts, scale)
	}

	// The dragged tab follows the pointer, clamped to the strip, with an
	// insertion line at the slot it would land in.
	if dragging {
		tw := TabWidthFor(n, w)
		dx := dragX
		if dx < int32(LeftMargin)+tw/2 {
			dx = int32(LeftMargin) + tw/2
		}
		if dx > w-tw/2 {
			dx = w - tw/2
		}
		idx := DropIndexAt(dx, n, w, scrollOffset)
		bx := int32(LeftMargin) + int32(idx)*(tw+int32(TabGap)) - scrollOffset
		lx := bx * scale
		buf.FillRect(frame.Rect4(lx-1*scale, 0, lx+1*scale, int32(TabBarHeight)*scale), tabTextActive, nil)
		for _, tab := range tabsList {
			if tab.ID() != dragID {
				continue
			}
			lr := frame.Rect4(dx-tw/2, 0, dx+tw-tw/2, TabBarHeight)
			r := scaleRect(lr, scale)
			drawTabBody(buf, tab, lr, r, true, fonts, scale)
		}
	}

	if len(tabsList) > 0 {
		btnR := scaleRect(NewTabButtonRect(int32(len(tabsList)), scrollOffset, w), scale)
		if btnR.X0 < int32(buf.W) {
			drawNewTabButton(buf, btnR, newTabBtnColor, scale)
		}
	}

	// Scroll chevrons overlay the strip's right end once tabs overflow. The
	// zone is repainted opaque first: clipped tabs sliding underneath must
	// not bleed through the controls.
	if left, right, ok := ChevronRects(int32(len(tabsList)), w); ok {
		lr := scaleRect(frame.Rect4(left.X0, 0, right.X1, TabBarHeight), scale)
		buf.FillRect(frame.Rect4(lr.X0, 0, lr.X1, int32(TabBarHeight)*scale), tabBarBg, nil)
		drawChevron(buf, scaleRect(left, scale), -1, newTabBtnColor, scale)
		drawChevron(buf, scaleRect(right, scale), 1, newTabBtnColor, scale)
	}
}

// drawTabBody paints one tab: background, title text and close button.
// lr is the logical rect (for text layout and the close button), r is the
// scaled device rect.
func drawTabBody(buf *frame.Bitmap, tab *Tab, lr, r frame.Rect, active bool, fonts *raster.Fonts, scale int32) {
	textColor := tabTextColor
	if active {
		fillRoundedTop(buf, r, tabRadius*scale, activeTabBg)
		textColor = tabTextActive
	}

	title := tab.Title()
	if title == "" {
		title = "New Tab"
	}
	drawTabText(buf, title, r, textColor, fonts, scale)

	drawCloseButton(buf, scaleRect(CloseButtonRect(lr), scale), closeBtnColor, scale)
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
	rad := frame.Corners{
		TL: frame.Radius{X: float32(radius), Y: float32(radius)},
		TR: frame.Radius{X: float32(radius), Y: float32(radius)},
	}
	buf.FillRounded(r, rad, c, nil)
}

func tabBaseline(tabRect frame.Rect, size int32, fonts *raster.Fonts) int32 {
	if fonts != nil {
		asc, desc, _ := fonts.LineMetrics(size, frame.FontSlot{})
		if asc+desc > 0 {
			if desc < 0 {
				desc = -desc
			}
			return tabRect.Y0 + (tabRect.H()-(asc+desc))/2 + asc
		}
	}
	return tabRect.Y0 + (tabRect.H()-size)/2 + size*4/5
}

func drawTabText(buf *frame.Bitmap, text string, tabRect frame.Rect, color frame.Color, fonts *raster.Fonts, scale int32) {
	if fonts == nil {
		return
	}
	size := titleSize * scale
	textX := tabRect.X0 + 12*scale
	baseline := tabBaseline(tabRect, size, fonts)
	maxW := tabRect.W() - (CloseBtnSize+CloseBtnMargin)*scale - 24*scale
	if maxW < 0 {
		maxW = 0
	}

	var penFixed int32
	penX := textX
	runes := []rune(text)
	for i, rn := range runes {
		g := fonts.Glyph(size, rn, frame.FontSlot{})
		advFixed := fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
		if !g.Ok || g.Mask == nil {
			penFixed += advFixed
			penX = textX + (penFixed+32)>>6
			continue
		}

		penX = textX + (penFixed+32)>>6
		if penX+g.Advance-textX > maxW {
			if i > 0 && len(runes) > 1 {
				for _, dot := range "..." {
					dg := fonts.Glyph(size, dot, frame.FontSlot{})
					dadv := fonts.GlyphAdvanceFixed(size, dot, frame.FontSlot{})
					dx := textX + (penFixed+32)>>6
					if dg.Ok && dg.Mask != nil {
						origin := frame.Point{X: dx + dg.Bounds.X0, Y: baseline + dg.Bounds.Y0}
						buf.BlitMask(dg.Mask, origin, color, buf.Bounds(), nil)
					}
					penFixed += dadv
				}
			}
			break
		}

		origin := frame.Point{X: penX + g.Bounds.X0, Y: baseline + g.Bounds.Y0}
		buf.BlitMask(g.Mask, origin, color, buf.Bounds(), nil)
		penFixed += advFixed
	}
}

func blendDot(buf *frame.Bitmap, x, y int, c frame.Color, cov float64) {
	if cov <= 0 {
		return
	}
	if cov > 1 {
		cov = 1
	}
	buf.Set(x, y, frame.BlendOver(buf.At(x, y), frame.ScaleColor(c, float32(cov))))
}

func drawLineAA(buf *frame.Bitmap, x0, y0, x1, y1, thick float64, c frame.Color) {
	minX := int(math.Floor(math.Min(x0, x1) - thick/2 - 1))
	maxX := int(math.Ceil(math.Max(x0, x1) + thick/2 + 1))
	minY := int(math.Floor(math.Min(y0, y1) - thick/2 - 1))
	maxY := int(math.Ceil(math.Max(y0, y1) + thick/2 + 1))
	dx := x1 - x0
	dy := y1 - y0
	len2 := dx*dx + dy*dy
	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			fx := float64(px) + 0.5
			fy := float64(py) + 0.5
			t := 0.0
			if len2 > 0 {
				t = ((fx-x0)*dx + (fy-y0)*dy) / len2
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
			}
			ddx := fx - (x0 + t*dx)
			ddy := fy - (y0 + t*dy)
			dist := math.Sqrt(ddx*ddx + ddy*ddy)
			blendDot(buf, px, py, c, thick/2+0.5-dist)
		}
	}
}

func drawCloseButton(buf *frame.Bitmap, r frame.Rect, c frame.Color, scale int32) {
	size := r.W()
	if size < 4*scale {
		return
	}
	inset := float64(4*scale) + 0.5
	x0 := float64(r.X0) + inset
	y0 := float64(r.Y0) + inset
	x1 := float64(r.X1) - inset
	y1 := float64(r.Y1) - inset
	if x1 <= x0 || y1 <= y0 {
		return
	}
	thick := float64(scale) * 1.6
	if thick < 1.5 {
		thick = 1.5
	}
	drawLineAA(buf, x0, y0, x1, y1, thick, c)
	drawLineAA(buf, x1, y0, x0, y1, thick, c)
}

// drawChevron paints one scroll control: a ‹ (dir -1) or › (dir +1) centered
// in r, in device pixels. Two stroked segments, the same AA technique as the
// close button's cross.
func drawChevron(buf *frame.Bitmap, r frame.Rect, dir int32, c frame.Color, scale int32) {
	cx := float64(r.X0+r.X1) / 2
	cy := float64(r.Y0+r.Y1) / 2
	arm := 7 * float64(scale)
	half := 6 * float64(scale)
	thick := float64(scale) * 1.6
	if thick < 1.5 {
		thick = 1.5
	}
	// The tip leads toward dir: ‹ (dir -1) points left, › (dir +1) right.
	tipX := cx + float64(dir)*arm
	endX := cx - float64(dir)*arm
	drawLineAA(buf, tipX, cy, endX, cy-half, thick, c)
	drawLineAA(buf, tipX, cy, endX, cy+half, thick, c)
}

func drawNewTabButton(buf *frame.Bitmap, r frame.Rect, c frame.Color, scale int32) {
	cx := float64(r.X0+r.X1) / 2
	cy := float64(r.Y0+r.Y1) / 2
	half := 7 * float64(scale)
	thick := float64(scale) * 1.6
	if thick < 1.5 {
		thick = 1.5
	}
	drawLineAA(buf, cx-half, cy, cx+half, cy, thick, c)
	drawLineAA(buf, cx, cy-half, cx, cy+half, thick, c)
}

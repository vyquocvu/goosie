package main

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/raster"
)

const (
	menuItemHeight = 26
	menuWidth      = 180
	menuPadding    = 4
)

// ContextMenuItem represents an option in the popup context menu.
type ContextMenuItem struct {
	Label     string
	Action    string
	Data      string
	Separator bool
	Disabled  bool
}

// ContextMenu represents an in-viewport context menu popup.
type ContextMenu struct {
	Visible bool
	X, Y    int32
	W, H    int32
	Items   []ContextMenuItem
	Hover   int
}

// NewContextMenu creates a new empty context menu.
func NewContextMenu() *ContextMenu {
	return &ContextMenu{Hover: -1}
}

// Show populates and opens the context menu at (x, y) with boundary clamping.
func (cm *ContextMenu) Show(x, y, maxW, maxH int32, items []ContextMenuItem, scale int32) {
	if scale < 1 {
		scale = 1
	}
	cm.Items = items
	cm.Hover = -1
	cm.W = int32(menuWidth) * scale

	totalH := int32(menuPadding*2) * scale
	for _, it := range items {
		if it.Separator {
			totalH += 6 * scale
		} else {
			totalH += int32(menuItemHeight) * scale
		}
	}
	cm.H = totalH

	// Clamp to viewport bounds
	if x+cm.W > maxW {
		x = maxW - cm.W - 4*scale
	}
	if y+cm.H > maxH {
		y = maxH - cm.H - 4*scale
	}
	if x < 4*scale {
		x = 4 * scale
	}
	if y < 4*scale {
		y = 4 * scale
	}
	cm.X = x
	cm.Y = y
	cm.Visible = true
}

// Hide closes the context menu.
func (cm *ContextMenu) Hide() {
	cm.Visible = false
	cm.Hover = -1
	cm.Items = nil
}

// Rect returns the bounding box of the context menu.
func (cm *ContextMenu) Rect() frame.Rect {
	if !cm.Visible {
		return frame.Rect{}
	}
	return frame.Rect4(cm.X, cm.Y, cm.X+cm.W, cm.Y+cm.H)
}

// UpdateHover updates the hovered item based on pointer position.
func (cm *ContextMenu) UpdateHover(pos frame.Point, scale int32) bool {
	if !cm.Visible || scale < 1 {
		return false
	}
	if pos.X < cm.X || pos.X >= cm.X+cm.W || pos.Y < cm.Y || pos.Y >= cm.Y+cm.H {
		if cm.Hover != -1 {
			cm.Hover = -1
			return true
		}
		return false
	}

	curY := cm.Y + int32(menuPadding)*scale
	oldHover := cm.Hover
	cm.Hover = -1

	for i, it := range cm.Items {
		var itemH int32
		if it.Separator {
			itemH = 6 * scale
		} else {
			itemH = int32(menuItemHeight) * scale
		}
		if pos.Y >= curY && pos.Y < curY+itemH {
			if !it.Separator && !it.Disabled {
				cm.Hover = i
			}
			break
		}
		curY += itemH
	}
	return cm.Hover != oldHover
}

// HitItem checks if pos clicks on an active context menu item.
func (cm *ContextMenu) HitItem(pos frame.Point, scale int32) (ContextMenuItem, bool) {
	if !cm.Visible || scale < 1 {
		return ContextMenuItem{}, false
	}
	if pos.X < cm.X || pos.X >= cm.X+cm.W || pos.Y < cm.Y || pos.Y >= cm.Y+cm.H {
		return ContextMenuItem{}, false
	}

	curY := cm.Y + int32(menuPadding)*scale
	for _, it := range cm.Items {
		var itemH int32
		if it.Separator {
			itemH = 6 * scale
		} else {
			itemH = int32(menuItemHeight) * scale
		}
		if pos.Y >= curY && pos.Y < curY+itemH {
			if !it.Separator && !it.Disabled {
				return it, true
			}
			return ContextMenuItem{}, false
		}
		curY += itemH
	}
	return ContextMenuItem{}, false
}

// Draw renders the context menu on buf.
func (cm *ContextMenu) Draw(buf *frame.Bitmap, fonts *raster.Fonts, scale int32) {
	if !cm.Visible || scale < 1 {
		return
	}
	// Menu card background
	bgRect := frame.Rect4(cm.X, cm.Y, cm.X+cm.W, cm.Y+cm.H)
	borderColor := frame.RGB(200, 204, 210)
	bgColor := frame.RGB(255, 255, 255)
	textColor := frame.RGB(32, 33, 36)
	hoverBgColor := frame.RGB(232, 240, 254)
	hoverTextColor := frame.RGB(26, 115, 232)
	sepColor := frame.RGB(226, 230, 235)
	disabledTextColor := frame.RGB(160, 165, 175)

	// Draw border & background
	buf.FillRect(bgRect, borderColor, nil)
	innerRect := frame.Rect4(cm.X+scale, cm.Y+scale, cm.X+cm.W-scale, cm.Y+cm.H-scale)
	buf.FillRect(innerRect, bgColor, nil)

	curY := cm.Y + int32(menuPadding)*scale
	fontSize := float32(12 * scale)

	for i, it := range cm.Items {
		if it.Separator {
			buf.FillRect(frame.Rect4(cm.X+int32(menuPadding)*scale, curY+2*scale, cm.X+cm.W-int32(menuPadding)*scale, curY+3*scale), sepColor, nil)
			curY += 6 * scale
			continue
		}

		itemH := int32(menuItemHeight) * scale
		itemRect := frame.Rect4(cm.X+scale, curY, cm.X+cm.W-scale, curY+itemH)

		tColor := textColor
		if it.Disabled {
			tColor = disabledTextColor
		} else if i == cm.Hover {
			buf.FillRect(itemRect, hoverBgColor, nil)
			tColor = hoverTextColor
		}

		// Draw label
		if fonts != nil && it.Label != "" {
			textX := cm.X + 12*scale
			textY := curY + (itemH-int32(fontSize))/2 + int32(fontSize)*4/5
			label := it.Label
			if len(label) > 24 {
				label = label[:21] + "..."
			}
			drawMenuText(buf, fonts, label, textX, textY, int32(fontSize), tColor)
		}

		curY += itemH
	}
}

func drawMenuText(buf *frame.Bitmap, fonts *raster.Fonts, text string, x, y int32, size int32, color frame.Color) {
	if fonts == nil || text == "" {
		return
	}
	var penFixed int32
	for _, rn := range text {
		g := fonts.Glyph(size, rn, frame.FontSlot{})
		advFixed := fonts.GlyphAdvanceFixed(size, rn, frame.FontSlot{})
		if g.Ok && g.Mask != nil {
			origin := frame.Point{X: x + (penFixed+32)>>6 + g.Bounds.X0, Y: y + g.Bounds.Y0}
			buf.BlitMask(g.Mask, origin, color, buf.Bounds(), nil)
		}
		penFixed += advFixed
	}
}

// BuildContextMenuItems builds the context menu items list depending on what
// was clicked under (contentX, contentY).
func BuildContextMenuItems(linkURL, imageURL, selText, focusedText string, isEditable, canBack, canForward bool) []ContextMenuItem {
	var items []ContextMenuItem

	if linkURL != "" {
		items = append(items, ContextMenuItem{Label: "Open in New Tab", Action: "open_new_tab", Data: linkURL})
		items = append(items, ContextMenuItem{Label: "Copy Link Address", Action: "copy_link", Data: linkURL})
		items = append(items, ContextMenuItem{Separator: true})
	}

	if imageURL != "" {
		items = append(items, ContextMenuItem{Label: "Open Image in New Tab", Action: "open_image_tab", Data: imageURL})
		items = append(items, ContextMenuItem{Label: "Copy Image Address", Action: "copy_image", Data: imageURL})
		items = append(items, ContextMenuItem{Separator: true})
	}

	if selText != "" {
		trimSel := strings.TrimSpace(selText)
		items = append(items, ContextMenuItem{Label: "Copy", Action: "copy", Data: trimSel})
		if len(trimSel) > 0 {
			items = append(items, ContextMenuItem{Label: "Search Google", Action: "search_google", Data: trimSel})
		}
		items = append(items, ContextMenuItem{Separator: true})
	} else if isEditable {
		items = append(items, ContextMenuItem{Label: "Cut", Action: "cut"})
		items = append(items, ContextMenuItem{Label: "Copy", Action: "copy", Data: focusedText})
		items = append(items, ContextMenuItem{Label: "Paste", Action: "paste"})
		items = append(items, ContextMenuItem{Label: "Select All", Action: "select_all"})
		items = append(items, ContextMenuItem{Separator: true})
	}

	// Navigation & Page items
	items = append(items, ContextMenuItem{Label: "Back", Action: "back", Disabled: !canBack})
	items = append(items, ContextMenuItem{Label: "Forward", Action: "forward", Disabled: !canForward})
	items = append(items, ContextMenuItem{Label: "Reload", Action: "reload"})
	items = append(items, ContextMenuItem{Separator: true})
	items = append(items, ContextMenuItem{Label: "Inspect Element", Action: "inspect"})
	items = append(items, ContextMenuItem{Label: "Print to PDF", Action: "print_pdf"})

	return items
}

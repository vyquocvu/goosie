package debug

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// Overlay draws debug visualization on top of the rendered frame.
// It shows damage rects, tile boundaries, hover areas, and other debug info.
type Overlay struct {
	Enabled       bool
	DrawTiles     bool // Draw tile grid boundaries
	DrawDamage    bool // Draw damage rects
	DrawHover     bool // Draw hovered element
	DrawFocus     bool // Draw focused element
	DrawSelection bool // Draw selection rects

	tileColor      frame.Color
	damageColor    frame.Color
	hoverColor     frame.Color
	focusColor     frame.Color
	selectionColor frame.Color
}

// NewOverlay returns a debug overlay with default colors.
func NewOverlay(enabled bool) *Overlay {
	return &Overlay{
		Enabled:        enabled,
		DrawTiles:      true,
		DrawDamage:     true,
		DrawHover:      true,
		DrawFocus:      true,
		DrawSelection:  true,
		tileColor:      frame.RGBA(100, 100, 100, 80), // Gray, semi-transparent
		damageColor:    frame.RGBA(255, 0, 0, 120),    // Red, semi-transparent
		hoverColor:     frame.RGBA(0, 255, 255, 100),  // Cyan, semi-transparent
		focusColor:     frame.RGBA(255, 255, 0, 120),  // Yellow, semi-transparent
		selectionColor: frame.RGBA(0, 100, 255, 100),  // Blue, semi-transparent
	}
}

// Draw renders debug overlays onto the display list.
// scale is the device pixel ratio for coordinate transformation.
func (o *Overlay) Draw(list *paint.List, scale float32, viewport frame.Rect, damage []frame.Rect, hoverRect, focusRect, selectionRect frame.Rect) {
	if !o.Enabled || list == nil {
		return
	}

	// Draw tile grid
	if o.DrawTiles {
		o.drawTileGrid(list, scale, viewport)
	}

	// Draw damage rects
	if o.DrawDamage && len(damage) > 0 {
		o.drawRects(list, scale, damage, o.damageColor, "DAMAGE")
	}

	// Draw hover rect
	if o.DrawHover && !hoverRect.Empty() {
		o.drawRect(list, scale, hoverRect, o.hoverColor, "HOVER")
	}

	// Draw focus rect
	if o.DrawFocus && !focusRect.Empty() {
		o.drawRect(list, scale, focusRect, o.focusColor, "FOCUS")
	}

	// Draw selection rect
	if o.DrawSelection && !selectionRect.Empty() {
		o.drawRect(list, scale, selectionRect, o.selectionColor, "SELECT")
	}
}

// drawTileGrid draws the 256px tile boundaries.
func (o *Overlay) drawTileGrid(list *paint.List, scale float32, viewport frame.Rect) {
	tileSize := float32(frame.TileSize) * scale

	// Vertical lines
	for x := float32(viewport.X0); x <= float32(viewport.X1); x += tileSize {
		list.Append(paint.DisplayCmd{
			Kind:  paint.CmdFill,
			Rect:  frame.Rect4(int32(x), int32(viewport.Y0), int32(x)+1, int32(viewport.Y1)),
			Color: o.tileColor,
		})
	}

	// Horizontal lines
	for y := float32(viewport.Y0); y <= float32(viewport.Y1); y += tileSize {
		list.Append(paint.DisplayCmd{
			Kind:  paint.CmdFill,
			Rect:  frame.Rect4(int32(viewport.X0), int32(y), int32(viewport.X1), int32(y)+1),
			Color: o.tileColor,
		})
	}
}

// drawRects draws multiple rects with a label.
func (o *Overlay) drawRects(list *paint.List, scale float32, rects []frame.Rect, col frame.Color, label string) {
	for _, r := range rects {
		o.drawRect(list, scale, r, col, label)
	}
}

// drawRect draws a single rect with border and semi-transparent fill.
func (o *Overlay) drawRect(list *paint.List, scale float32, r frame.Rect, col frame.Color, label string) {
	if r.Empty() {
		return
	}

	// Scale coordinates
	x0 := int32(float32(r.X0) * scale)
	y0 := int32(float32(r.Y0) * scale)
	x1 := int32(float32(r.X1) * scale)
	y1 := int32(float32(r.Y1) * scale)

	// Fill (semi-transparent)
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x0, y0, x1, y1),
		Color: col,
	})

	// Border (2px, opaque) - extract RGB from col and set alpha to 255
	// frame.Color is uint32: 0xRRGGBBAA
	borderCol := frame.Color((uint32(col) & 0xFFFFFF00) | 0xFF)
	borderWidth := int32(2)

	// Top
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x0, y0, x1, y0+borderWidth),
		Color: borderCol,
	})
	// Bottom
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x0, y1-borderWidth, x1, y1),
		Color: borderCol,
	})
	// Left
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x0, y0, x0+borderWidth, y1),
		Color: borderCol,
	})
	// Right
	list.Append(paint.DisplayCmd{
		Kind:  paint.CmdFill,
		Rect:  frame.Rect4(x1-borderWidth, y0, x1, y1),
		Color: borderCol,
	})
}

// HoverRect computes the bounding rect of a hovered node in the arena.
func HoverRect(arena interface{}, nodeID uint32) frame.Rect {
	// Type assertion to avoid import cycle
	type arenaGetter interface {
		GetObjectRect(nodeID uint32) frame.Rect
	}
	if getter, ok := arena.(arenaGetter); ok {
		return getter.GetObjectRect(nodeID)
	}
	return frame.Rect{}
}

// FocusRect computes the bounding rect of the focused element.
func FocusRect(arena interface{}, focusNodeID uint32) frame.Rect {
	return HoverRect(arena, focusNodeID)
}

// SelectionRect computes the bounding rect of the current selection.
func SelectionRect(arena interface{}, selectionBounds frame.Rect) frame.Rect {
	return selectionBounds
}

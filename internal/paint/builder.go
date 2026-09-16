package paint

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// Builder walks a layout arena and emits display commands into a List.
//
// The builder is the seam between layout and paint: layout produces positioned
// boxes with styles, and the builder translates each box into the drawing
// operations that paint understands. The builder runs after layout is complete;
// it does not mutate the arena.
type Builder struct {
	list  *List
	arena *layout.Arena
	scale float32
}

// NewBuilder returns a builder that will emit commands into list.
func NewBuilder(list *List, arena *layout.Arena, scale float32) *Builder {
	return &Builder{list: list, arena: arena, scale: scale}
}

// Build walks the arena starting at root and appends display commands to the
// list. The root object's position is taken as the origin; a page with a
// scrolled viewport passes the scroll offset as root.X/Y before calling Build.
func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root)
}

func (b *Builder) paintBox(id layout.ObjectID) {
	obj := b.arena.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}
	if obj.Style != nil {
		x0, y0, x1, y1 := obj.BorderRect()
		rect := frame.RectF4(x0, y0, x1, y1).ToDevice(b.scale)
		if obj.Style.BackgroundColor.A > 0 {
			b.list.Append(DisplayCmd{
				Kind:  CmdFill,
				Rect:  rect,
				Color: convertColor(obj.Style.BackgroundColor),
			})
		}
		if obj.Style.Display == style.DisplayBlock || obj.Style.Display == style.DisplayInline {
			b.paintBorders(obj, rect)
		}
		if obj.Node != nil && obj.Node.Type == 2 {
			b.paintText(obj, rect)
		}
	}
	for kid := obj.FirstKid; kid != 0; kid = b.arena.Get(kid).NextSibling {
		b.paintBox(kid)
	}
}

func (b *Builder) paintBorders(obj *layout.Object, rect frame.Rect) {
	s := obj.Style
	if s == nil {
		return
	}
	border := BorderSpec{}
	if s.BorderTopWidth > 0 {
		border.Top = SideSpec{Width: int32(s.BorderTopWidth), Color: convertColor(s.BorderTopColor)}
	}
	if s.BorderRightWidth > 0 {
		border.Right = SideSpec{Width: int32(s.BorderRightWidth), Color: convertColor(s.BorderRightColor)}
	}
	if s.BorderBottomWidth > 0 {
		border.Bottom = SideSpec{Width: int32(s.BorderBottomWidth), Color: convertColor(s.BorderBottomColor)}
	}
	if s.BorderLeftWidth > 0 {
		border.Left = SideSpec{Width: int32(s.BorderLeftWidth), Color: convertColor(s.BorderLeftColor)}
	}
	if border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0 {
		b.list.Append(DisplayCmd{
			Kind:   CmdBorder,
			Rect:   rect,
			Border: border,
		})
	}
}

func (b *Builder) paintText(obj *layout.Object, rect frame.Rect) {
	if obj.Node == nil || obj.Node.DataContent == "" {
		return
	}
	if rect.Empty() {
		return
	}
	s := obj.Style
	if s == nil {
		return
	}
	fontSize := int32(s.FontSize * b.scale)
	if fontSize <= 0 {
		return
	}
	text := obj.Node.DataContent
	runes := []rune(text)
	if len(runes) == 0 {
		return
	}
	glyphs := make([]GlyphRun, 0, len(runes))
	x := rect.X0
	y := rect.Y0 + int32(s.FontSize*b.scale*0.8)
	advance := fontSize / 2
	if obj.W > 0 {
		advance = int32(obj.W*b.scale) / int32(len(runes))
	}
	for _, r := range runes {
		glyphs = append(glyphs, GlyphRun{
			Rune: r,
			X:    x,
			Y:    y,
			Size: fontSize,
		})
		x += advance
	}
	b.list.Append(DisplayCmd{
		Kind: CmdText,
		Rect: rect,
		Text: TextRun{
			Glyphs: glyphs,
			Color:  convertColor(s.Color),
		},
	})
}

func convertColor(c css.Color) frame.Color {
	return frame.RGBA(c.R, c.G, c.B, c.A)
}

package paint

import (
	"image"

	"github.com/vyquocvu/goosie/internal/frame"
)

type CmdKind uint8

const (
	CmdFill CmdKind = iota
	CmdBorder
	CmdText
	CmdImage
	CmdGradient
	CmdRadialGradient
)

func (k CmdKind) String() string {
	switch k {
	case CmdFill:
		return "fill"
	case CmdBorder:
		return "border"
	case CmdText:
		return "text"
	case CmdImage:
		return "image"
	case CmdGradient:
		return "gradient"
	case CmdRadialGradient:
		return "radial-gradient"
	}
	return "unknown"
}

type SideSpec struct {
	Width int32
	Color frame.Color
}

type BorderSpec struct {
	Top, Right, Bottom, Left SideSpec
}

func (b BorderSpec) Empty() bool {
	return b.Top.Width <= 0 && b.Right.Width <= 0 && b.Bottom.Width <= 0 && b.Left.Width <= 0
}

type GlyphRun struct {
	Rune rune
	X, Y int32
	Size int32
	Slot frame.FontSlot
}

type TextRun struct {
	Glyphs []GlyphRun
	Color  frame.Color
}

func (t TextRun) Bounds(scale int32) frame.Rect {
	if len(t.Glyphs) == 0 {
		return frame.Rect{}
	}
	r := frame.Rect{X0: t.Glyphs[0].X, Y0: t.Glyphs[0].Y - t.Glyphs[0].Size,
		X1: t.Glyphs[0].X + t.Glyphs[0].Size*scale, Y1: t.Glyphs[0].Y}
	for _, g := range t.Glyphs[1:] {
		gx0, gy0 := g.X, g.Y-g.Size
		if gx0 < r.X0 {
			r.X0 = gx0
		}
		if gy0 < r.Y0 {
			r.Y0 = gy0
		}
		gx1, gy1 := g.X+g.Size*scale, g.Y
		if gx1 > r.X1 {
			r.X1 = gx1
		}
		if gy1 > r.Y1 {
			r.Y1 = gy1
		}
	}
	return r
}

type ImageSpec struct {
	Src    image.Image
	SrcBox image.Rectangle
}

type DisplayCmd struct {
	Kind           CmdKind
	Rect           frame.Rect
	Color          frame.Color
	Border         BorderSpec
	Radius         frame.Corners
	Opacity        float32
	Mask           image.Image
	Text           TextRun
	Image          ImageSpec
	Gradient       frame.LinearGradient
	RadialGradient frame.RadialGradient
	Filter         string
	BackdropFilter string

	Clip frame.Rect

	Z uint32

	Seq int32
}

func (d DisplayCmd) Bounds() (frame.Rect, bool) {
	switch d.Kind {
	case CmdText:
		r := d.Text.Bounds(1)
		return r, !r.Empty()
	case CmdBorder:
		if d.Border.Empty() {
			return frame.Rect{}, false
		}
		return d.Rect, !d.Rect.Empty()
	case CmdFill, CmdImage:
		return d.Rect, !d.Rect.Empty() && (d.Kind == CmdImage || d.Color.A() > 0)
	case CmdGradient:
		return d.Rect, !d.Rect.Empty() && !d.Gradient.Empty()
	case CmdRadialGradient:
		return d.Rect, !d.Rect.Empty() && !d.RadialGradient.Empty()
	}
	return frame.Rect{}, false
}

func (d DisplayCmd) Intersects(r frame.Rect) bool {
	b, ok := d.Bounds()
	return ok && b.Intersects(r)
}

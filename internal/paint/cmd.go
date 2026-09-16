package paint

import (
	"image"

	"github.com/vyquocvu/goosie/internal/frame"
)

// CmdKind selects which union member of a DisplayCmd is meaningful. An enum
// plus one struct keeps the display list a flat slice of equal-sized values,
// which is what makes it slab-allocatable and cheap for workers to scan. The
// alternative - one Go type per command behind an interface - allocates per
// command and forces a type assertion in the rasterizer's inner loop.
type CmdKind uint8

const (
	// CmdFill paints a solid rect.
	CmdFill CmdKind = iota
	// CmdBorder paints up to four solid edge strips around a rect.
	CmdBorder
	// CmdText draws positioned glyph runs.
	CmdText
	// CmdImage draws a decoded image scaled into a rect.
	CmdImage
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
	}
	return "unknown"
}

// SideSpec is one edge of a border. Width is in device pixels and Color is
// premultiplied, so the rasterizer needs neither the scale factor nor a
// conversion step.
type SideSpec struct {
	Width int32
	Color frame.Color
}

// BorderSpec holds the four edges of a box border. Only solid styles exist in
// M1: dashed and dotted borders are a later milestone, and carrying a style
// field nobody reads would invite a half-implemented path.
type BorderSpec struct {
	Top, Right, Bottom, Left SideSpec
}

// Empty reports whether the border paints nothing, which lets the rasterizer
// skip a command the style pass already knows is a no-op.
func (b BorderSpec) Empty() bool {
	return b.Top.Width <= 0 && b.Right.Width <= 0 && b.Bottom.Width <= 0 && b.Left.Width <= 0
}

// GlyphRun is one glyph at a position fixed by the producer. The rasterizer
// performs mask lookup and blit only; it never measures, breaks lines, or
// resolves a font. That is the whole point of pushing text positioning into
// layout: invariant 4 says no string-to-length parsing at paint time, and a
// positioned glyph run is what that looks like as a type.
//
// X and Y are device pixels in the layer's content space. Size is the pixel
// size, already scaled by the device ratio, so a face lookup is keyed by
// (rune, size) alone.
type GlyphRun struct {
	Rune rune
	X, Y int32
	Size int32
}

// TextRun is a string of glyphs sharing a color. Glyphs are owned by the
// producer's slab and are frozen along with the list, exactly like the commands
// that reference them.
type TextRun struct {
	Glyphs []GlyphRun
	Color  frame.Color
}

// Bounds returns the device-space box the run occupies. It is derived from the
// glyph positions and sizes rather than stored, so a producer cannot publish a
// run whose bounds contradict its glyphs.
func (t TextRun) Bounds(scale int32) frame.Rect {
	if len(t.Glyphs) == 0 {
		return frame.Rect{}
	}
	r := frame.Rect{X0: t.Glyphs[0].X, Y0: t.Glyphs[0].Y - t.Glyphs[0].Size,
		X1: t.Glyphs[0].X + 1, Y1: t.Glyphs[0].Y}
	for _, g := range t.Glyphs[1:] {
		gx0, gy0 := g.X, g.Y-g.Size
		if gx0 < r.X0 {
			r.X0 = gx0
		}
		if gy0 < r.Y0 {
			r.Y0 = gy0
		}
		// Advance width is unknown here, so the extent is estimated from the
		// pixel size. scale lets a producer that knows the real advance tighten
		// it; the rasterizer only needs an over-approximation, since a too-large
		// bound costs one extra tile and a too-small one clips a glyph.
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

// ImageSpec names the pixels to draw. Src is read-only for the frame's
// lifetime; v2 never mutates a decoded image in place, which is what allows the
// same image to appear in several frozen lists at once.
type ImageSpec struct {
	Src    image.Image
	SrcBox image.Rectangle
}

// DisplayCmd is one drawing operation in paint order. It is a value, not an
// interface implementation, so a display list is one contiguous slab and a
// worker scanning it never dereferences a pointer it does not already own.
//
// Rect is in layer content space. Text and Image commands also carry their own
// detail; unused fields stay zero, which costs a little memory and buys a
// uniform slice.
type DisplayCmd struct {
	Kind   CmdKind
	Rect   frame.Rect
	Color  frame.Color
	Border BorderSpec
	// Opacity multiplies the command's coverage. Zero means unset, that is
	// opaque, so a producer only writes the field when it is not 1; an element
	// that is fully transparent is dropped before it reaches a list at all.
	Opacity float32
	Text    TextRun
	Image   ImageSpec

	// Z orders this command within its layer. The list is sorted once per
	// content version, never per frame or per tile; see List.SortStable.
	Z uint32
}

// Bounds returns the box the command touches in content space, or false when it
// paints nothing. Callers use it for tile selection, so it must over-approximate
// rather than clip a glyph.
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
	}
	return frame.Rect{}, false
}

// Intersects reports whether the command can affect a tile covering r. Fully
// conservative: it is fine for this to say yes to a command that turns out to
// paint nothing inside the tile, and wrong to say no to one that does.
func (d DisplayCmd) Intersects(r frame.Rect) bool {
	b, ok := d.Bounds()
	return ok && b.Intersects(r)
}

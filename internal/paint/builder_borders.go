package paint

import (
	"image"
	"math"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (b *Builder) paintBorders(obj *layout.Object, rect frame.Rect, radius frame.Corners, opacity float32) {
	s := obj.Style
	if s == nil {
		return
	}
	if b.paintBorderImage(obj, rect, opacity) {
		return
	}
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = b.borderSide(obj.BorderTop, s.BorderTopColor, s.BorderTopStyle)
	}
	if obj.BorderRight > 0 {
		rightColour, rightStyle := s.BorderRightColor, s.BorderRightStyle
		if obj.CollapsedBorderRight != nil {
			rightColour = obj.CollapsedBorderRight.BorderRightColor
			rightStyle = obj.CollapsedBorderRight.BorderRightStyle
		}
		border.Right = b.borderSide(obj.BorderRight, rightColour, rightStyle)
	}
	if obj.BorderBottom > 0 {
		bottomColour, bottomStyle := s.BorderBottomColor, s.BorderBottomStyle
		if obj.CollapsedBorderBottom != nil {
			bottomColour = obj.CollapsedBorderBottom.BorderBottomColor
			bottomStyle = obj.CollapsedBorderBottom.BorderBottomStyle
		}
		border.Bottom = b.borderSide(obj.BorderBottom, bottomColour, bottomStyle)
	}
	if obj.BorderLeft > 0 {
		border.Left = b.borderSide(obj.BorderLeft, s.BorderLeftColor, s.BorderLeftStyle)
	}
	if border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0 {
		b.list.Append(DisplayCmd{
			Kind:    CmdBorder,
			Rect:    rect,
			Border:  border,
			Radius:  radius,
			Opacity: opacity,
		})
	}
}

func (b *Builder) paintInlineBorderFragments(obj *layout.Object, opacity float32) {
	s := obj.Style
	if s == nil {
		return
	}
	rtl := s.Direction == "rtl"
	n := len(obj.InlineFragments)
	for i, frag := range obj.InlineFragments {
		rect := frag.ToDevice(b.scale)
		b.paintBorderFragment(obj, rect, opacity, i == 0, i == n-1, rtl)
	}
}

func (b *Builder) paintBorderFragment(obj *layout.Object, rect frame.Rect, opacity float32, first, last, rtl bool) {
	s := obj.Style
	drawLeft := (first && !rtl) || (last && rtl)
	drawRight := (last && !rtl) || (first && rtl)
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = b.borderSide(obj.BorderTop, s.BorderTopColor, s.BorderTopStyle)
	}
	if obj.BorderBottom > 0 {
		border.Bottom = b.borderSide(obj.BorderBottom, s.BorderBottomColor, s.BorderBottomStyle)
	}
	if drawLeft && obj.BorderLeft > 0 {
		border.Left = b.borderSide(obj.BorderLeft, s.BorderLeftColor, s.BorderLeftStyle)
	}
	if drawRight && obj.BorderRight > 0 {
		border.Right = b.borderSide(obj.BorderRight, s.BorderRightColor, s.BorderRightStyle)
	}
	if border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0 {
		b.list.Append(DisplayCmd{
			Kind:    CmdBorder,
			Rect:    rect,
			Border:  border,
			Opacity: opacity,
		})
	}
}

func (b *Builder) borderSide(width float32, colour css.Color, styleName string) SideSpec {
	deviceWidth := int32(width * b.scale)
	if deviceWidth <= 0 {
		deviceWidth = 1
	}
	return SideSpec{Width: deviceWidth, Color: convertColor(colour), Style: borderStyle(styleName)}
}

func borderStyle(name string) BorderStyle {
	switch name {
	case "inset":
		return BorderStyleInset
	case "outset":
		return BorderStyleOutset
	case "ridge":
		return BorderStyleRidge
	case "groove":
		return BorderStyleGroove
	default:
		return BorderStyleSolid
	}
}

func (b *Builder) paintOutline(s *style.ComputedStyle, rect frame.Rect, opacity float32) {
	if s == nil || s.OutlineWidth <= 0 {
		return
	}
	if s.OutlineStyle == "" || s.OutlineStyle == "none" || s.OutlineStyle == "hidden" {
		return
	}
	thick := int32(s.OutlineWidth * b.scale)
	if thick <= 0 {
		thick = 1
	}
	off := int32(s.OutlineOffset * b.scale)
	outer := frame.Rect4(rect.X0-off-thick, rect.Y0-off-thick, rect.X1+off+thick, rect.Y1+off+thick)
	inner := frame.Rect4(rect.X0-off, rect.Y0-off, rect.X1+off, rect.Y1+off)
	if outer.Empty() || inner.Empty() {
		return
	}
	c := s.OutlineColor
	if c.A == 0 {
		c = s.Color
	}
	color := convertColor(c)
	strip := func(x0, y0, x1, y1 int32) {
		r := frame.Rect4(x0, y0, x1, y1)
		if r.Empty() {
			return
		}
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: r, Color: color, Opacity: opacity})
	}
	strip(outer.X0, outer.Y0, outer.X1, inner.Y0)
	strip(outer.X0, inner.Y1, outer.X1, outer.Y1)
	strip(outer.X0, inner.Y0, inner.X0, inner.Y1)
	strip(inner.X1, inner.Y0, outer.X1, inner.Y1)
}

func (b *Builder) paintBorderImage(obj *layout.Object, rect frame.Rect, opacity float32) bool {
	s := obj.Style
	if s == nil || s.BorderImage.Source == "" {
		return false
	}
	src, ok := obj.BorderImage.(image.Image)
	if !ok || src == nil {
		return false
	}
	sb := src.Bounds()
	sw, sh := float32(sb.Dx()), float32(sb.Dy())
	if sw <= 0 || sh <= 0 {
		return false
	}
	bi := s.BorderImage
	cut := [4]float32{}
	for i := range cut {
		v := bi.Slice[i]
		if bi.SlicePct[i] {
			if i == 0 || i == 2 {
				v = v * sh / 100
			} else {
				v = v * sw / 100
			}
		}
		if v < 0 {
			v = 0
		}
		cut[i] = v
	}
	if cut[0]+cut[2] > sh && sh > 0 {
		k := sh / (cut[0] + cut[2])
		cut[0], cut[2] = cut[0]*k, cut[2]*k
	}
	if cut[1]+cut[3] > sw && sw > 0 {
		k := sw / (cut[1] + cut[3])
		cut[1], cut[3] = cut[1]*k, cut[3]*k
	}
	bw := obj.Style.UsedBorderWidths()
	ox := [4]float32{}
	if bi.HasOutset {
		for i := range ox {
			if bi.OutsetN[i] {
				ox[i] = bi.Outset[i] * bw[i]
			} else {
				ox[i] = bi.Outset[i]
			}
		}
	}
	areaW := obj.W + obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight + ox[1] + ox[3]
	areaH := obj.H + obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom + ox[0] + ox[2]
	w := [4]float32{}
	for i := range w {
		if !bi.HasWidth {
			w[i] = bw[i]
			continue
		}
		switch {
		case bi.WidthPct[i]:
			if i == 0 || i == 2 {
				w[i] = bi.Width[i] * areaH / 100
			} else {
				w[i] = bi.Width[i] * areaW / 100
			}
		case bi.WidthU[i] == css.BiWidthAuto:
			w[i] = cut[i]
		case bi.WidthU[i] == css.BiWidthNumber:
			w[i] = bi.Width[i] * bw[i]
		default:
			w[i] = bi.Width[i]
		}
		if w[i] < 0 {
			w[i] = 0
		}
	}
	toDev := func(v float32) int32 { return int32(v*b.scale + 0.5) }
	ax0, ay0 := rect.X0-toDev(ox[3]), rect.Y0-toDev(ox[0])
	ax1, ay1 := rect.X1+toDev(ox[1]), rect.Y1+toDev(ox[2])
	dw := [4]int32{toDev(w[0]), toDev(w[1]), toDev(w[2]), toDev(w[3])}
	sc := func(f float32) int { return int(f + 0.5) }
	cells := []biCell{
		{sb.Min.X, sb.Min.Y, sb.Min.X + sc(cut[3]), sb.Min.Y + sc(cut[0]), ax0, ay0, ax0 + dw[3], ay0 + dw[0]},
		{sb.Max.X - sc(cut[1]), sb.Min.Y, sb.Max.X, sb.Min.Y + sc(cut[0]), ax1 - dw[1], ay0, ax1, ay0 + dw[0]},
		{sb.Min.X, sb.Max.Y - sc(cut[2]), sb.Min.X + sc(cut[3]), sb.Max.Y, ax0, ay1 - dw[2], ax0 + dw[3], ay1},
		{sb.Max.X - sc(cut[1]), sb.Max.Y - sc(cut[2]), sb.Max.X, sb.Max.Y, ax1 - dw[1], ay1 - dw[2], ax1, ay1},
	}
	topSrc := [4]int{sb.Min.X + sc(cut[3]), sb.Min.Y, sb.Max.X - sc(cut[1]), sb.Min.Y + sc(cut[0])}
	botSrc := [4]int{sb.Min.X + sc(cut[3]), sb.Max.Y - sc(cut[2]), sb.Max.X - sc(cut[1]), sb.Max.Y}
	leftSrc := [4]int{sb.Min.X, sb.Min.Y + sc(cut[0]), sb.Min.X + sc(cut[3]), sb.Max.Y - sc(cut[2])}
	rightSrc := [4]int{sb.Max.X - sc(cut[1]), sb.Min.Y + sc(cut[0]), sb.Max.X, sb.Max.Y - sc(cut[2])}
	for _, tc := range biTileCells(topSrc, bi.RepeatX, float64(ax0+dw[3]), float64(ax1-dw[1]), ay0, ay0+dw[0], true) {
		cells = append(cells, tc)
	}
	for _, tc := range biTileCells(botSrc, bi.RepeatX, float64(ax0+dw[3]), float64(ax1-dw[1]), ay1-dw[2], ay1, true) {
		cells = append(cells, tc)
	}
	for _, tc := range biTileCells(leftSrc, bi.RepeatY, float64(ay0+dw[0]), float64(ay1-dw[2]), ax0, ax0+dw[3], false) {
		cells = append(cells, tc)
	}
	for _, tc := range biTileCells(rightSrc, bi.RepeatY, float64(ay0+dw[0]), float64(ay1-dw[2]), ax1-dw[1], ax1, false) {
		cells = append(cells, tc)
	}
	if bi.Fill {
		cells = append(cells, biCell{
			sb.Min.X + sc(cut[3]), sb.Min.Y + sc(cut[0]), sb.Max.X - sc(cut[1]), sb.Max.Y - sc(cut[2]),
			ax0 + dw[3], ay0 + dw[0], ax1 - dw[1], ay1 - dw[2],
		})
	}
	for _, c := range cells {
		if c.sx1 <= c.sx0 || c.sy1 <= c.sy0 || c.dx1 <= c.dx0 || c.dy1 <= c.dy0 {
			continue
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdImage,
			Rect:    frame.Rect4(c.dx0, c.dy0, c.dx1, c.dy1),
			Image:   ImageSpec{Src: src, SrcBox: image.Rect(c.sx0, c.sy0, c.sx1, c.sy1)},
			Opacity: opacity,
		})
	}
	return true
}

func corners(s *style.ComputedStyle, r frame.Rect, scale float32) frame.Corners {
	return s.UsedBorderRadius(float32(r.W()), float32(r.H()), scale)
}

func uniformCorners(r float32) frame.Corners {
	rad := frame.Radius{X: r, Y: r}
	return frame.Corners{TL: rad, TR: rad, BR: rad, BL: rad}
}

type biCell struct {
	sx0, sy0, sx1, sy1 int
	dx0, dy0, dx1, dy1 int32
}

func biTileCells(src [4]int, mode css.BorderImageRepeat, span0, span1 float64, cross0, cross1 int32, horizontal bool) []biCell {
	var out []biCell
	emit := func(s0, s1 int, d0, d1 float64) {
		if d1 > d0 {
			if d0 < span0 {
				s0 += int((span0 - d0) / (d1 - d0) * float64(s1-s0))
				d0 = span0
			}
			if d1 > span1 {
				s1 -= int((d1 - span1) / (d1 - d0) * float64(s1-s0))
				d1 = span1
			}
		}
		if s1 <= s0 || d1 <= d0 {
			return
		}
		if horizontal {
			out = append(out, biCell{sx0: s0, sy0: src[1], sx1: s1, sy1: src[3], dx0: int32(d0 + 0.5), dy0: cross0, dx1: int32(d1 + 0.5), dy1: cross1})
		} else {
			out = append(out, biCell{sx0: src[0], sy0: s0, sx1: src[2], sy1: s1, dx0: cross0, dy0: int32(d0 + 0.5), dx1: cross1, dy1: int32(d1 + 0.5)})
		}
	}
	srcLen := src[2] - src[0]
	axis0 := src[0]
	if !horizontal {
		srcLen = src[3] - src[1]
		axis0 = src[1]
	}
	span := span1 - span0
	if srcLen <= 0 || span <= 0 {
		return nil
	}
	crossLen := float64(cross1 - cross0)
	srcCross := src[3] - src[1]
	if !horizontal {
		srcCross = src[2] - src[0]
	}
	tile := float64(srcLen)
	if srcCross > 0 && crossLen > 0 {
		tile = float64(srcLen) * crossLen / float64(srcCross)
	}
	if tile <= 0 {
		return nil
	}
	mid := (span0 + span1) / 2
	switch mode {
	case css.BiRepeatRepeat:
		i0 := int(math.Floor((span0 - (mid - tile/2)) / tile))
		i1 := int(math.Ceil((span1 - (mid - tile/2)) / tile))
		for i := i0; i < i1; i++ {
			d0 := mid - tile/2 + float64(i)*tile
			emit(axis0, axis0+srcLen, d0, d0+tile)
		}
	case css.BiRepeatRound:
		n := int(span/tile + 0.5)
		if n < 1 {
			n = 1
		}
		each := span / float64(n)
		for i := 0; i < n; i++ {
			emit(axis0, axis0+srcLen, span0+float64(i)*each, span0+float64(i+1)*each)
		}
	case css.BiRepeatSpace:
		n := int(span / tile)
		if n < 1 {
			n = 1
		}
		gap := (span - float64(n)*tile) / float64(n+1)
		for i := 0; i < n; i++ {
			d := span0 + gap + float64(i)*(tile+gap)
			emit(axis0, axis0+srcLen, d, d+tile)
		}
	default:
		emit(axis0, axis0+srcLen, span0, span1)
	}
	return out
}

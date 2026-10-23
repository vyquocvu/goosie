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
	// A decoded border-image replaces normal border rendering entirely; the
	// border widths still reserve layout space underneath it.
	if b.paintBorderImage(obj, rect, opacity) {
		return
	}
	// The widths come off the box, not the style: layout is what decides which
	// edges a box actually draws, and a collapsed table border is exactly the
	// case where the two disagree. A nonzero width always paints at least one
	// device pixel: truncating 0.1px to zero would erase the hairlines the
	// small-values tests require to be visible.
	toDevice := func(cssPx float32) int32 {
		if cssPx <= 0 {
			return 0
		}
		// Floor, with a one-pixel floor for nonzero widths: 1.9px snaps
		// to 1px (never rounds up to 2px), while 0.1px still paints its
		// hairline instead of vanishing.
		if w := int32(cssPx * b.scale); w > 0 {
			return w
		}
		return 1
	}
	border := BorderSpec{}
	if obj.BorderTop > 0 {
		border.Top = SideSpec{Width: toDevice(obj.BorderTop), Color: convertColor(s.BorderTopColor)}
	}
	if obj.BorderRight > 0 {
		border.Right = SideSpec{Width: toDevice(obj.BorderRight), Color: convertColor(s.BorderRightColor)}
	}
	if obj.BorderBottom > 0 {
		border.Bottom = SideSpec{Width: toDevice(obj.BorderBottom), Color: convertColor(s.BorderBottomColor)}
	}
	if obj.BorderLeft > 0 {
		border.Left = SideSpec{Width: toDevice(obj.BorderLeft), Color: convertColor(s.BorderLeftColor)}
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

// paintBorderImage nine-slices a decoded border-image source over the
// border area. It reports whether it painted: without a source, a slice, or
// decoded pixels the caller falls back to normal borders.
//
// Slice edges cut the source (percentages of its dimensions, numbers in
// source px, scaled down together when they overflow); widths size the
// destination ring (auto reads the slice intrinsics, numbers multiply the
// border widths, lengths and percentages are absolute); outset grows the
// ring past the border box. Corners scale to their cells; edges stretch
// along their axis in this pass (repeat/round/space tile in the next).
// The middle paints only with slice fill.
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
	// Source cuts: top/bottom against the height, left/right against the
	// width (percentages) or in source px (numbers).
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
	// Overflowing cuts scale down together, preserving their ratios.
	if cut[0]+cut[2] > sh && sh > 0 {
		k := sh / (cut[0] + cut[2])
		cut[0], cut[2] = cut[0]*k, cut[2]*k
	}
	if cut[1]+cut[3] > sw && sw > 0 {
		k := sw / (cut[1] + cut[3])
		cut[1], cut[3] = cut[1]*k, cut[3]*k
	}
	// Destination widths per side: top, right, bottom, left. Outset is
	// area-independent (lengths, or numbers times the border widths), so the
	// area resolves first and percentage widths read it after. Number widths
	// multiply the *used* border widths (zero under none/hidden style).
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
	// Border-image area in CSS px: the border box grown by the outset.
	// obj.W/H are content-box; padding and border complete the border box
	// the percentage widths key off.
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
			// Auto reads the slice intrinsics: the cut's own size.
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
	// Device-space destination area and widths.
	toDev := func(v float32) int32 { return int32(v*b.scale + 0.5) }
	ax0, ay0 := rect.X0-toDev(ox[3]), rect.Y0-toDev(ox[0])
	ax1, ay1 := rect.X1+toDev(ox[1]), rect.Y1+toDev(ox[2])
	dw := [4]int32{toDev(w[0]), toDev(w[1]), toDev(w[2]), toDev(w[3])}
	sc := func(f float32) int { return int(f + 0.5) }
	cells := []biCell{
		// Corners.
		{sb.Min.X, sb.Min.Y, sb.Min.X + sc(cut[3]), sb.Min.Y + sc(cut[0]), ax0, ay0, ax0 + dw[3], ay0 + dw[0]},
		{sb.Max.X - sc(cut[1]), sb.Min.Y, sb.Max.X, sb.Min.Y + sc(cut[0]), ax1 - dw[1], ay0, ax1, ay0 + dw[0]},
		{sb.Min.X, sb.Max.Y - sc(cut[2]), sb.Min.X + sc(cut[3]), sb.Max.Y, ax0, ay1 - dw[2], ax0 + dw[3], ay1},
		{sb.Max.X - sc(cut[1]), sb.Max.Y - sc(cut[2]), sb.Max.X, sb.Max.Y, ax1 - dw[1], ay1 - dw[2], ax1, ay1},
	}
	// Edges tile along their axis per the repeat modes; the cross axis
	// always stretches to the destination width.
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
	// A fully degenerate image (every cell empty) still replaces the border:
	// per CSS an empty source region paints nothing, but the border style
	// underneath must not show through either.
	return true
}

// corners resolves a box's declared radii against the device rect they round. A
// percentage is the share of the side it runs along that the cascade could not
// compute, and radii that overflow their side scale down together, which is what
// turns an oversized value like 999px into a pill.
func corners(s *style.ComputedStyle, r frame.Rect, scale float32) frame.Corners {
	if s == nil {
		return frame.Corners{}
	}
	declared := s.BorderRadius
	// A declared percentage is a negative sentinel, so the check for "nothing was
	// declared" has to be against zero rather than against a positive length.
	if declared == ([4][2]float32{}) {
		return frame.Corners{}
	}
	w, h := float64(r.W()), float64(r.H())
	resolve := func(v float32, side float64) float64 {
		if v < -2 {
			return float64(-2-v) * side / 100
		}
		if v <= 0 {
			return 0
		}
		return float64(v) * float64(scale)
	}
	var c [4][2]float64
	for i := 0; i < 4; i++ {
		c[i][0] = resolve(declared[i][0], w)
		c[i][1] = resolve(declared[i][1], h)
	}
	fit := func(sum, side float64) float64 {
		if sum <= side || sum <= 0 {
			return 1
		}
		return side / sum
	}
	k := fit(c[0][0]+c[1][0], w)
	if v := fit(c[3][0]+c[2][0], w); v < k {
		k = v
	}
	if v := fit(c[0][1]+c[3][1], h); v < k {
		k = v
	}
	if v := fit(c[1][1]+c[2][1], h); v < k {
		k = v
	}
	rad := func(i int) frame.Radius {
		return frame.Radius{X: float32(c[i][0] * k), Y: float32(c[i][1] * k)}
	}
	return frame.Corners{TL: rad(0), TR: rad(1), BR: rad(2), BL: rad(3)}
}

func uniformCorners(r float32) frame.Corners {
	rad := frame.Radius{X: r, Y: r}
	return frame.Corners{TL: rad, TR: rad, BR: rad, BL: rad}
}

// biCell is one nine-slice piece: a source sub-rectangle scaled into a
// destination rect, both as integers (source px, device px).
type biCell struct {
	sx0, sy0, sx1, sy1 int
	dx0, dy0, dx1, dy1 int32
}

// biTileCells lays one border-image edge strip: src is the source strip,
// mode the axis repeat, span the destination interval along the axis, and
// cross the destination interval across it (for a horizontal edge, x-span
// and y-cross). The tile keeps its intrinsic axis size; the cross axis
// stretches to the destination width, which preserves the source aspect
// the way round-trip scaling through the cross ratio does.
//
// Tiles phase from the span center, the way border-image counts from the
// middle of each side: a tile far larger than its span shows its middle,
// and small tiles repeat symmetrically outward, clipped to the span.
// Stretch covers the span in one piece; repeat clips the overhanging
// tiles; round rescales to a whole count; space distributes whole tiles
// with the first flush to the start and the last flush to the end (a lone
// tile centers unscathed).
func biTileCells(src [4]int, mode css.BorderImageRepeat, span0, span1 float64, cross0, cross1 int32, horizontal bool) []biCell {
	var out []biCell
	emit := func(s0, s1 int, d0, d1 float64) {
		// Clip to the span, keeping source and destination proportional:
		// edge tiles never paint into the corners' cells.
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
	// The tile keeps the source aspect: the strip scaled by the cross-axis
	// ratio (destination width over source cross size). A 50px strip on a
	// 100px border tiles at 100px; a square slice on equal widths tiles at
	// its own size.
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
		// Tiles march outward from the centered tile, clipped to the span.
		// Every tile shows the whole strip; only its visible slice maps.
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
		// Space distributes evenly between AND around the tiles: n+1
		// equal gaps, so a lone tile centers and two tiles sit at
		// third-points rather than flush with the span ends.
		gap := (span - float64(n)*tile) / float64(n+1)
		for i := 0; i < n; i++ {
			d := span0 + gap + float64(i)*(tile+gap)
			emit(axis0, axis0+srcLen, d, d+tile)
		}
	default: // BiRepeatStretch
		emit(axis0, axis0+srcLen, span0, span1)
	}
	return out
}

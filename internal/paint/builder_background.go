package paint

import (
	"image"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

const maxBgTiles = 4096

// BGTileSize resolves one background tile's drawn size for paint and for
// the engine's post-layout vector rasterization, which must agree exactly.
// noRatio marks a source with no intrinsic ratio (no usable dimensions and
// no viewBox): cover and contain fill the positioning area instead of ratio
// math. Otherwise negotiation is standard against the natural size.
//
// Auto always tiles at the natural size: the decoder bakes the aspect
// handling (meet centering, none stretching) into the raster, so paint
// places it 1:1. Only cover/contain scale, and only lengths resolve axes;
// an auto axis follows the ratio, falling back to the natural dimension.
func BGTileSize(s *style.ComputedStyle, areaW, areaH, natW, natH float32, noRatio bool) (float32, float32) {
	tileW, tileH := natW, natH
	switch s.BackgroundSize {
	case style.BgSizeContain:
		if noRatio {
			tileW, tileH = areaW, areaH
			break
		}
		k := areaW / natW
		if h := areaH / natH; h < k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeCover:
		if noRatio {
			tileW, tileH = areaW, areaH
			break
		}
		k := areaW / natW
		if h := areaH / natH; h > k {
			k = h
		}
		tileW, tileH = natW*k, natH*k
	case style.BgSizeLength:
		wAuto := !s.BgSizeWPct && s.BgSizeW < 0
		hAuto := !s.BgSizeHPct && s.BgSizeH < 0
		if s.BgSizeWPct {
			tileW = areaW * s.BgSizeW
		} else if !wAuto {
			tileW = s.BgSizeW
		}
		if s.BgSizeHPct {
			tileH = areaH * s.BgSizeH
		} else if !hAuto {
			tileH = s.BgSizeH
		}
		switch {
		case wAuto && hAuto:
			tileW, tileH = natW, natH
		case wAuto:
			// Width auto resolves from the height through the ratio;
			// without one it keeps the natural width: only the height
			// axis defaults to the positioning area (see hAuto).
			if !noRatio && tileH > 0 && natW > 0 && natH > 0 {
				tileW = tileH * natW / natH
			} else {
				tileW = natW
			}
		case hAuto:
			// Height auto: preserve the image aspect ratio, or fill the
			// positioning area height when there is no ratio at all.
			if !noRatio && tileW > 0 && natW > 0 && natH > 0 {
				tileH = tileW * natH / natW
			} else if noRatio {
				tileH = areaH
			} else {
				tileH = natH
			}
		}
	}
	return tileW, tileH
}

// paintBackground draws a box's CSS background-image. The repeat modes tile the
// image across the border box; no-repeat places it once. background-size selects
// the tile's drawn size and background-position selects its origin. A tile that
// overflows the box edge is clipped by shrinking both its destination rect and
// the source sub-rect that maps into it, so paint never spills onto a neighbour.
func (b *Builder) paintBackground(obj *layout.Object, opacity float32) {
	src, ok := obj.BgImage.(image.Image)
	if !ok || src == nil {
		return
	}
	s := obj.Style
	if s == nil {
		return
	}
	sb := src.Bounds()
	natW, natH := float32(sb.Dx()), float32(sb.Dy())
	if natW <= 0 || natH <= 0 {
		return
	}
	tiles := bgLayerTiles(s, obj, natW, natH, obj.BgNoRatio)
	// A border-area clip shows only the ring between the border box and the
	// padding box: each tile's visible slice is cut against the inner box
	// before source mapping, so every piece keeps its own pixels.
	var ring *[4]float32
	if s.BackgroundClip == style.BgBoxBorderArea {
		px0, py0, px1, py1 := obj.BgAreaRect(style.BgBoxPadding)
		ring = &[4]float32{px0, py0, px1, py1}
	}
	for _, t := range tiles {
		pieces := [][4]float64{{t.vx0, t.vy0, t.vx1, t.vy1}}
		if ring != nil {
			pieces = splitSlice([4]float64{t.vx0, t.vy0, t.vx1, t.vy1}, ring)
		}
		for _, pc := range pieces {
			vx0, vy0, vx1, vy1 := pc[0], pc[1], pc[2], pc[3]
			// Map the visible destination slice back to the image's source
			// sub-rectangle, at the tile's scale.
			sx0 := int32(float64(sb.Min.X) + (vx0-float64(t.cx))/float64(t.tw)*float64(sb.Dx()))
			sx1 := int32(float64(sb.Min.X) + (vx1-float64(t.cx))/float64(t.tw)*float64(sb.Dx()))
			sy0 := int32(float64(sb.Min.Y) + (vy0-float64(t.cy))/float64(t.th)*float64(sb.Dy()))
			sy1 := int32(float64(sb.Min.Y) + (vy1-float64(t.cy))/float64(t.th)*float64(sb.Dy()))
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			dst := frame.RectF4(float32(vx0), float32(vy0), float32(vx1), float32(vy1)).ToDevice(b.scale)
			if dst.Empty() {
				continue
			}
			b.list.Append(DisplayCmd{
				Kind:    CmdImage,
				Rect:    dst,
				Image:   ImageSpec{Src: src, SrcBox: image.Rect(int(sx0), int(sy0), int(sx1), int(sy1))},
				Opacity: opacity,
			})
		}
	}
}

// paintGradientTiles paints one gradient layer through the background
// tile machinery: gradients carry no intrinsic size, so the positioning
// area is both the size they resolve auto axes against and the natural
// size negotiation starts from. Each tile maps the ramp to itself, which
// is what makes repeat tile a gradient instead of stretching one.
func (b *Builder) paintGradientTiles(obj *layout.Object, clipRect frame.Rect, radius frame.Corners, g frame.LinearGradient, isRadial bool, rg frame.RadialGradient) {
	s := obj.Style
	if s == nil {
		return
	}
	px0, py0, px1, py1 := obj.BgAreaRect(s.BackgroundOrigin)
	areaW, areaH := px1-px0, py1-py0
	if areaW <= 0 || areaH <= 0 {
		return
	}
	for _, t := range bgLayerTiles(s, obj, areaW, areaH, true) {
		dst := frame.RectF4(float32(t.vx0), float32(t.vy0), float32(t.vx1), float32(t.vy1)).ToDevice(b.scale)
		if dst.Empty() {
			continue
		}
		if isRadial {
			b.list.Append(DisplayCmd{
				Kind:           CmdRadialGradient,
				Rect:           dst,
				RadialGradient: rg,
				Radius:         radius,
			})
		} else {
			b.list.Append(DisplayCmd{
				Kind:     CmdGradient,
				Rect:     dst,
				Gradient: g,
				Radius:   radius,
			})
		}
	}
}

// bgTile is one painted tile: cx/cy is the tile origin and tw/th its size
// (for mapping back to source pixels); vx0..vy1 is the visible slice after
// clipping, in CSS px.
type bgTile struct {
	cx, cy, tw, th     float32
	vx0, vy0, vx1, vy1 float64
}

// bgLayerTiles lays one background layer's tiles: size from BGTileSize
// against the origin box, round rescaling, positions from background-
// position, spans covering the clip box (space stays in the origin box).
// Every returned tile is non-empty and clipped to the clip box.
func bgLayerTiles(s *style.ComputedStyle, obj *layout.Object, natW, natH float32, noRatio bool) []bgTile {
	// Tiles position within the origin box and paint inside the clip box.
	px0, py0, px1, py1 := obj.BgAreaRect(s.BackgroundOrigin)
	cx0, cy0, cx1, cy1 := obj.BgAreaRect(s.BackgroundClip)
	if px1 < px0 {
		px0, px1 = px1, px0
	}
	if py1 < py0 {
		py0, py1 = py1, py0
	}
	if cx1 < cx0 {
		cx0, cx1 = cx1, cx0
	}
	if cy1 < cy0 {
		cy0, cy1 = cy1, cy0
	}
	areaW, areaH := px1-px0, py1-py0
	if areaW <= 0 || areaH <= 0 {
		return nil
	}
	ax0, ay0 := px0, py0

	tileW, tileH := BGTileSize(s, areaW, areaH, natW, natH, noRatio)
	if tileW <= 0 || tileH <= 0 {
		return nil
	}

	rx, ry := bgRepeatAxis(s.BackgroundRepeat, true), bgRepeatAxis(s.BackgroundRepeatY, false)
	// Round rescales the tile so a whole number fits the positioning area.
	if rx == style.BgRepeatRound {
		tileW = bgRoundTile(areaW, tileW)
	}
	if ry == style.BgRepeatRound {
		tileH = bgRoundTile(areaH, tileH)
	}

	ox := bgOrigin(s.BackgroundPosXMode, s.BackgroundPosX, s.BgPosXPct, ax0, areaW, tileW)
	oy := bgOrigin(s.BackgroundPosYMode, s.BackgroundPosY, s.BgPosYPct, ay0, areaH, tileH)

	// Repeating tiles cover the whole clip box, phased from the
	// positioning origin: a padding-box origin on a bordered box still
	// repeats under the transparent border. Space distributes within the
	// positioning area itself.
	sx0, sx1 := cx0, cx1
	sy0, sy1 := cy0, cy1
	if rx == style.BgRepeatSpace {
		sx0, sx1 = ax0, px1
	}
	if ry == style.BgRepeatSpace {
		sy0, sy1 = ay0, py1
	}
	xs := bgAxisSpans(rx, ox, tileW, sx0, sx1, areaW)
	ys := bgAxisSpans(ry, oy, tileH, sy0, sy1, areaH)
	if len(xs)*len(ys) == 0 || len(xs)*len(ys) > maxBgTiles {
		return nil
	}
	var out []bgTile
	for _, cx := range xs {
		for _, cy := range ys {
			vx0, vy0 := float64(cx), float64(cy)
			vx1, vy1 := float64(cx+tileW), float64(cy+tileH)
			// Painting clips to the clip box, which may extend past the
			// positioning area the tiles phase from.
			if float32(vx0) < cx0 {
				vx0 = float64(cx0)
			}
			if float32(vy0) < cy0 {
				vy0 = float64(cy0)
			}
			if float32(vx1) > cx1 {
				vx1 = float64(cx1)
			}
			if float32(vy1) > cy1 {
				vy1 = float64(cy1)
			}
			if vx1 <= vx0 || vy1 <= vy0 {
				continue
			}
			out = append(out, bgTile{cx: cx, cy: cy, tw: tileW, th: tileH, vx0: vx0, vy0: vy0, vx1: vx1, vy1: vy1})
		}
	}
	return out
}

// bgOrigin resolves one background-position axis into a CSS-px coordinate where
// a tile of the given size begins within a span of `areaLen` starting at `base`.
// A percentage aligns `length` of the image with `length` of the free space; a
// length is a plain offset from the start edge. The three/four-value offsets
// measure from their own keyword edge instead: `right 25px` sits 25px left of
// the end, and a percentage offset resolves against the free space.
func bgOrigin(mode style.BgPosMode, length float32, isPct bool, base, areaLen, tileSize float32) float32 {
	free := areaLen - tileSize
	switch mode {
	case style.BgPosCenter:
		return base + free/2
	case style.BgPosEnd:
		return base + free
	case style.BgPosLength:
		if isPct {
			return base + free*length
		}
		return base + length
	case style.BgPosStartOffset:
		if isPct {
			return base + free*length
		}
		return base + length
	case style.BgPosEndOffset:
		if isPct {
			return base + free - free*length
		}
		return base + free - length
	default: // BgPosStart
		return base
	}
}

// bgRepeatAxis normalizes one repeat mode for the axis being laid out: the
// legacy repeat-x/repeat-y spellings repeat only on their own axis, while
// every other mode applies wherever it is stored.
func bgRepeatAxis(mode style.BgRepeat, isX bool) style.BgRepeat {
	switch mode {
	case style.BgRepeatRepeat:
		return style.BgRepeatRepeat
	case style.BgRepeatRepeatX:
		if isX {
			return style.BgRepeatRepeat
		}
		return style.BgRepeatNoRepeat
	case style.BgRepeatRepeatY:
		if !isX {
			return style.BgRepeatRepeat
		}
		return style.BgRepeatNoRepeat
	default:
		return mode
	}
}

// bgRoundTile rescales a tile so a whole number fits the positioning area:
// the count is the nearest whole number of tiles, at least one, and the
// tile stretches or squeezes to exactly fill the area with that count.
func bgRoundTile(areaLen, tileLen float32) float32 {
	if tileLen <= 0 || areaLen <= 0 {
		return tileLen
	}
	n := int(areaLen/tileLen + 0.5)
	if n < 1 {
		n = 1
	}
	return areaLen / float32(n)
}

// bgAxisSpans lists the start coordinate of every tile along one axis. A
// repeating (or round, which repeats its rescaled tile) axis phases from
// the origin and fills the whole span; space lays whole unclipped tiles
// flush with both edges and spreads the leftover evenly, centering a lone
// tile; anything else contributes only the single placed tile.
func bgAxisSpans(mode style.BgRepeat, origin, size, lo, hi, areaLen float32) []float32 {
	switch mode {
	case style.BgRepeatSpace:
		if size <= 0 {
			return nil
		}
		n := int(areaLen / size)
		if n <= 1 {
			// One tile (or none fitting): center it in the area.
			return []float32{lo + (areaLen-size)/2}
		}
		if n > maxBgTiles {
			n = maxBgTiles
		}
		gap := (areaLen - float32(n)*size) / float32(n-1)
		out := make([]float32, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, lo+float32(i)*(size+gap))
		}
		return out
	case style.BgRepeatRepeat, style.BgRepeatRound:
		start := origin
		// Walk the phase back to the edge so tiling covers the whole area.
		for i := 0; start > lo && i < maxBgTiles; i++ {
			start -= size
		}
		var out []float32
		for x := start; x < hi && len(out) < maxBgTiles; x += size {
			out = append(out, x)
		}
		return out
	default:
		return []float32{origin}
	}
}

// appendBgFill emits one background-color fill. A border-area clip paints
// only the ring between the border box and the padding box (square corners;
// rounded ones keep the full border box because the rasterizer has no
// rounded hole-punch).
func (b *Builder) appendBgFill(obj *layout.Object, clipRect frame.Rect, radius frame.Corners, color frame.Color, opacity float32) {
	// A mask-image multiplies the background fill's coverage (icon pattern:
	// solid colour showing through a glyph-shaped mask). The mask stretches
	// over the fill rect; mask-size/position/repeat are deferred.
	var mask image.Image
	if m, ok := obj.MaskImage.(image.Image); ok && m != nil {
		if b := m.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
			mask = m
		}
	}
	if obj.Style.BackgroundClip == style.BgBoxBorderArea && radius.Empty() {
		px0, py0, px1, py1 := obj.BgAreaRect(style.BgBoxPadding)
		inner := frame.RectF4(px0, py0, px1, py1).ToDevice(b.scale)
		for _, r := range subtractRect(clipRect, inner) {
			b.list.Append(DisplayCmd{
				Kind:    CmdFill,
				Rect:    r,
				Color:   color,
				Opacity: opacity,
				Mask:    mask,
			})
		}
		return
	}
	b.list.Append(DisplayCmd{
		Kind:    CmdFill,
		Rect:    clipRect,
		Color:   color,
		Radius:  radius,
		Opacity: opacity,
		Mask:    mask,
	})
}

func splitSlice(v [4]float64, inner *[4]float32) [][4]float64 {
	ix0, iy0 := float64(inner[0]), float64(inner[1])
	if ix0 < v[0] {
		ix0 = v[0]
	}
	if iy0 < v[1] {
		iy0 = v[1]
	}
	ix1, iy1 := float64(inner[2]), float64(inner[3])
	if ix1 > v[2] {
		ix1 = v[2]
	}
	if iy1 > v[3] {
		iy1 = v[3]
	}
	if ix0 >= ix1 || iy0 >= iy1 {
		return [][4]float64{v}
	}
	var out [][4]float64
	for _, r := range [][4]float64{
		{v[0], v[1], v[2], iy0},
		{v[0], iy1, v[2], v[3]},
		{v[0], iy0, ix0, iy1},
		{ix1, iy0, v[2], iy1},
	} {
		if r[2] > r[0] && r[3] > r[1] {
			out = append(out, r)
		}
	}
	return out
}

// subtractRect cuts inner out of outer: the up-to-four axis-aligned strips
// that cover outer minus inner. A non-overlapping inner returns outer whole;
// an empty result returns nothing.
func subtractRect(outer, inner frame.Rect) []frame.Rect {
	ix0, iy0 := outer.X0, outer.Y0
	if inner.X0 > ix0 {
		ix0 = inner.X0
	}
	if inner.Y0 > iy0 {
		iy0 = inner.Y0
	}
	ix1, iy1 := outer.X1, outer.Y1
	if inner.X1 < ix1 {
		ix1 = inner.X1
	}
	if inner.Y1 < iy1 {
		iy1 = inner.Y1
	}
	if ix0 >= ix1 || iy0 >= iy1 {
		if outer.Empty() {
			return nil
		}
		return []frame.Rect{outer}
	}
	var out []frame.Rect
	// Top and bottom span the full width; left and right fill between them.
	for _, r := range []frame.Rect{
		frame.Rect4(outer.X0, outer.Y0, outer.X1, iy0),
		frame.Rect4(outer.X0, iy1, outer.X1, outer.Y1),
		frame.Rect4(outer.X0, iy0, ix0, iy1),
		frame.Rect4(ix1, iy0, outer.X1, iy1),
	} {
		if !r.Empty() {
			out = append(out, r)
		}
	}
	return out
}

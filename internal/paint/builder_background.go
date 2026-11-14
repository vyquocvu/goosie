package paint

import (
	"image"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

const maxBgTiles = 4096

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
			if !noRatio && tileH > 0 && natW > 0 && natH > 0 {
				tileW = tileH * natW / natH
			} else {
				tileW = natW
			}
		case hAuto:
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

func (b *Builder) paintGradientTiles(obj *layout.Object, clipRect frame.Rect, radius frame.Corners, sg *style.Gradient, srg *style.RadialGradient, opacity float32) {
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
		w, h := float32(t.vx1-t.vx0), float32(t.vy1-t.vy0)
		if srg != nil {
			rx, ry, cx, cy := radialRadii(*srg, w, h)
			rg := radialGradient(*srg, rx, ry, cx, cy, opacity)
			b.list.Append(DisplayCmd{
				Kind:           CmdRadialGradient,
				Rect:           dst,
				RadialGradient: rg,
				Radius:         radius,
			})
		} else {
			g := gradient(*sg, gradientLineLength(sg.Angle, w, h), opacity)
			b.list.Append(DisplayCmd{
				Kind:     CmdGradient,
				Rect:     dst,
				Gradient: g,
				Radius:   radius,
			})
		}
	}
}

type bgTile struct {
	cx, cy, tw, th     float32
	vx0, vy0, vx1, vy1 float64
}

func bgLayerTiles(s *style.ComputedStyle, obj *layout.Object, natW, natH float32, noRatio bool) []bgTile {
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
	if rx == style.BgRepeatRound {
		tileW = bgRoundTile(areaW, tileW)
	}
	if ry == style.BgRepeatRound {
		tileH = bgRoundTile(areaH, tileH)
	}

	ox := bgOrigin(s.BackgroundPosXMode, s.BackgroundPosX, s.BgPosXPct, ax0, areaW, tileW)
	oy := bgOrigin(s.BackgroundPosYMode, s.BackgroundPosY, s.BgPosYPct, ay0, areaH, tileH)

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
	default:
		return base
	}
}

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

func bgAxisSpans(mode style.BgRepeat, origin, size, lo, hi, areaLen float32) []float32 {
	switch mode {
	case style.BgRepeatSpace:
		if size <= 0 {
			return nil
		}
		n := int(areaLen / size)
		if n <= 1 {
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

func (b *Builder) appendBgFill(obj *layout.Object, clipRect frame.Rect, radius frame.Corners, color frame.Color, opacity float32) {
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

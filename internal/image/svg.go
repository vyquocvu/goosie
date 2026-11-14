package image

import (
	"bytes"
	"encoding/xml"
	"fmt"
	gimage "image"
	"math"
	"strconv"
	"strings"
)

const (
	maxSVGElements  = 8192
	maxSVGDepth     = 64
	defaultSVGSize  = 300
	defaultSVGSizeH = 150
)

type svgColor struct {
	r, g, b, a float64
	none       bool
}

type svgState struct {
	fill, stroke svgColor
	opacity      float64
	strokeW      float64
	ctm          [6]float64
}

func IsSVG(data []byte) bool {
	return isSVGData(data)
}

func isSVGData(data []byte) bool {
	t := bytes.TrimSpace(data)
	if len(t) == 0 || t[0] != '<' {
		return false
	}
	dec := xml.NewDecoder(bytes.NewReader(t))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == "svg"
		}
	}
}

func rejectSVGAttackSurface(data []byte) error {
	lower := bytes.ToLower(data)
	if bytes.Contains(lower, []byte("<!doctype")) || bytes.Contains(lower, []byte("<!entity")) {
		return fmt.Errorf("image: svg with DOCTYPE/ENTITY declarations refused")
	}
	return nil
}

func svgLength(s string, pctBase float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '+' || s[i] == '-') {
		i++
	}
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(strings.TrimSpace(s[i:])) {
	case "", "px":
		return f, true
	case "%":
		return f * pctBase / 100, true
	case "pt":
		return f * 96.0 / 72.0, true
	case "pc":
		return f * 16.0, true
	case "in":
		return f * 96.0, true
	case "cm":
		return f * 96.0 / 2.54, true
	case "mm":
		return f * 96.0 / 25.4, true
	case "em", "ex", "rem":
		return f * 16.0, true
	}
	return 0, false
}

func svgColorOf(s string, opacity float64) (svgColor, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return svgColor{}, false
	}
	if strings.EqualFold(s, "none") {
		return svgColor{none: true}, true
	}
	if strings.HasPrefix(s, "url(") {
		return svgColor{}, false
	}
	var r, g, b float64
	var a = 1.0
	if strings.EqualFold(s, "transparent") {
		return svgColor{a: 0}, true
	}
	switch {
	case strings.HasPrefix(s, "#"):
		hex := s[1:]
		var rv, gv, bv, av uint64 = 0, 0, 0, 255
		switch len(hex) {
		case 3:
			_, err := fmt.Sscanf(hex, "%1x%1x%1x", &rv, &gv, &bv)
			if err != nil {
				return svgColor{}, false
			}
			rv, gv, bv = rv*17, gv*17, bv*17
		case 6, 8:
			_, err := fmt.Sscanf(hex, "%2x%2x%2x", &rv, &gv, &bv)
			if err != nil {
				return svgColor{}, false
			}
			if len(hex) == 8 {
				var rest string
				rest = hex[6:]
				var av2 uint64
				if _, err := fmt.Sscanf(rest, "%2x", &av2); err != nil {
					return svgColor{}, false
				}
				av = av2
			}
		default:
			return svgColor{}, false
		}
		r, g, b, a = float64(rv)/255, float64(gv)/255, float64(bv)/255, float64(av)/255
	case strings.HasPrefix(strings.ToLower(s), "rgb(") || strings.HasPrefix(strings.ToLower(s), "rgba("):
		inner := s[strings.Index(s, "(")+1 : len(s)-1]
		parts := strings.Split(inner, ",")
		if len(parts) < 3 || len(parts) > 4 {
			return svgColor{}, false
		}
		nums := make([]float64, len(parts))
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if strings.HasSuffix(p, "%") {
				f, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
				if err != nil {
					return svgColor{}, false
				}
				nums[i] = f / 100
				continue
			}
			f, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return svgColor{}, false
			}
			if i < 3 {
				nums[i] = f / 255
			} else {
				nums[i] = f
			}
		}
		r, g, b = nums[0], nums[1], nums[2]
		if len(nums) == 4 {
			a = nums[3]
		}
	default:
		var ok bool
		r, g, b, ok = svgKeyword(s)
		if !ok {
			return svgColor{}, false
		}
	}
	a *= opacity
	if a <= 0 {
		return svgColor{none: true}, true
	}
	if a > 1 {
		a = 1
	}
	return svgColor{r: r * a, g: g * a, b: b * a, a: a}, true
}

func svgKeyword(s string) (float64, float64, float64, bool) {
	switch strings.ToLower(s) {
	case "black":
		return 0, 0, 0, true
	case "white":
		return 1, 1, 1, true
	case "red":
		return 1, 0, 0, true
	case "green":
		return 0, 0.50196, 0, true
	case "blue":
		return 0, 0, 1, true
	case "yellow":
		return 1, 1, 0, true
	case "cyan", "aqua":
		return 0, 1, 1, true
	case "magenta", "fuchsia":
		return 1, 0, 1, true
	case "gray", "grey":
		return 0.50196, 0.50196, 0.50196, true
	case "silver":
		return 0.75294, 0.75294, 0.75294, true
	case "maroon":
		return 0.50196, 0, 0, true
	case "olive":
		return 0.50196, 0.50196, 0, true
	case "lime":
		return 0, 1, 0, true
	case "teal":
		return 0, 0.50196, 0.50196, true
	case "navy":
		return 0, 0, 0.50196, true
	case "purple":
		return 0.50196, 0, 0.50196, true
	case "orange":
		return 1, 0.64706, 0, true
	case "transparent":
		return 0, 0, 0, false
	}
	return 0, 0, 0, false
}

func svgAttr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func rejectDegenerateViewBox(el xml.StartElement) error {
	if vb := strings.Fields(svgAttr(el, "viewBox")); len(vb) == 4 {
		vw, vwErr := strconv.ParseFloat(vb[2], 64)
		vh, vhErr := strconv.ParseFloat(vb[3], 64)
		if vwErr == nil && vhErr == nil && (vw <= 0 || vh <= 0) {
			return fmt.Errorf("image: svg viewBox %q has no area", svgAttr(el, "viewBox"))
		}
	}
	return nil
}

func svgViewport(el xml.StartElement) (int, int, error) {
	if err := rejectDegenerateViewBox(el); err != nil {
		return 0, 0, err
	}
	w, wOk := svgLength(svgAttr(el, "width"), 0)
	h, hOk := svgLength(svgAttr(el, "height"), 0)
	if (!wOk || w <= 0) || (!hOk || h <= 0) {
		vb := strings.Fields(svgAttr(el, "viewBox"))
		if len(vb) == 4 {
			if vw, err := strconv.ParseFloat(vb[2], 64); err == nil && vw > 0 {
				if vh, err := strconv.ParseFloat(vb[3], 64); err == nil && vh > 0 {
					if (!wOk || w <= 0) && hOk && h > 0 {
						w = h * vw / vh
					} else if (!hOk || h <= 0) && wOk && w > 0 {
						h = w * vh / vw
					} else if (!wOk || w <= 0) && (!hOk || h <= 0) {
						w, h = vw, vh
					}
				}
			}
		}
	}
	if w <= 0 {
		w = defaultSVGSize
	}
	if h <= 0 {
		h = defaultSVGSizeH
	}
	wi, hi := int(math.Ceil(w)), int(math.Ceil(h))
	if wi <= 0 || hi <= 0 || wi > MaxImageDimension || hi > MaxImageDimension {
		return 0, 0, fmt.Errorf("image: svg dimensions %dx%d exceed admission limit %d", wi, hi, MaxImageDimension)
	}
	if int64(wi) > MaxDecodedImagePixels/int64(hi) {
		return 0, 0, fmt.Errorf("image: svg pixel count %dx%d exceeds limit %d", wi, hi, MaxDecodedImagePixels)
	}
	return wi, hi, nil
}

func svgMapping(el xml.StartElement, w, h int) (sx, sy, ox, oy float64) {
	sx, sy = 1, 1
	vb := strings.Fields(svgAttr(el, "viewBox"))
	if len(vb) == 4 {
		minx, _ := strconv.ParseFloat(vb[0], 64)
		miny, _ := strconv.ParseFloat(vb[1], 64)
		vw, _ := strconv.ParseFloat(vb[2], 64)
		vh, _ := strconv.ParseFloat(vb[3], 64)
		if vw > 0 && vh > 0 {
			if strings.TrimSpace(svgAttr(el, "preserveAspectRatio")) == "none" {
				sx, sy = float64(w)/vw, float64(h)/vh
			} else {
				s := math.Min(float64(w)/vw, float64(h)/vh)
				sx, sy = s, s
			}
			ox = -minx * sx
			oy = -miny * sy
			if sx == sy {
				ox += (float64(w) - vw*sx) / 2
				oy += (float64(h) - vh*sy) / 2
			}
		}
	}
	return sx, sy, ox, oy
}

func svgTransform(st svgState, s string) svgState {
	s = strings.TrimSpace(s)
	if s == "" {
		return st
	}
	mul := func(m [6]float64) {
		a, b, c, d, e, f := st.ctm[0], st.ctm[1], st.ctm[2], st.ctm[3], st.ctm[4], st.ctm[5]
		st.ctm = [6]float64{
			a*m[0] + c*m[1],
			b*m[0] + d*m[1],
			a*m[2] + c*m[3],
			b*m[2] + d*m[3],
			a*m[4] + c*m[5] + e,
			b*m[4] + d*m[5] + f,
		}
	}
	for len(s) > 0 {
		par := strings.Index(s, "(")
		if par < 0 {
			break
		}
		name := strings.TrimSpace(s[:par])
		rest := s[par+1:]
		end := strings.Index(rest, ")")
		if end < 0 {
			break
		}
		args := strings.FieldsFunc(rest[:end], func(c rune) bool { return c == ',' || c == ' ' || c == '\t' || c == '\n' })
		nums := make([]float64, 0, len(args))
		for _, a := range args {
			if a == "" {
				continue
			}
			if f, err := strconv.ParseFloat(a, 64); err == nil {
				nums = append(nums, f)
			}
		}
		switch strings.ToLower(name) {
		case "translate":
			if len(nums) > 0 {
				ty := 0.0
				if len(nums) > 1 {
					ty = nums[1]
				}
				mul([6]float64{1, 0, 0, 1, nums[0], ty})
			}
		case "scale":
			if len(nums) > 0 {
				sy := nums[0]
				if len(nums) > 1 {
					sy = nums[1]
				}
				mul([6]float64{nums[0], 0, 0, sy, 0, 0})
			}
		case "rotate":
			if len(nums) > 0 {
				rad := nums[0] * math.Pi / 180
				cos, sin := math.Cos(rad), math.Sin(rad)
				if len(nums) == 3 {
					cx, cy := nums[1], nums[2]
					mul([6]float64{1, 0, 0, 1, cx, cy})
					mul([6]float64{cos, sin, -sin, cos, 0, 0})
					mul([6]float64{1, 0, 0, 1, -cx, -cy})
				} else {
					mul([6]float64{cos, sin, -sin, cos, 0, 0})
				}
			}
		case "skewx":
			if len(nums) > 0 {
				mul([6]float64{1, 0, math.Tan(nums[0] * math.Pi / 180), 1, 0, 0})
			}
		case "skewy":
			if len(nums) > 0 {
				mul([6]float64{1, math.Tan(nums[0] * math.Pi / 180), 0, 1, 0, 0})
			}
		case "matrix":
			if len(nums) == 6 {
				mul([6]float64{nums[0], nums[1], nums[2], nums[3], nums[4], nums[5]})
			}
		}
		s = strings.TrimSpace(rest[end+1:])
		if s == "" {
			break
		}
		if s[0] == ',' {
			s = strings.TrimSpace(s[1:])
		}
	}
	return st
}

func svgOpacity(s string) float64 {
	if s = strings.TrimSpace(s); s == "" {
		return 1
	} else if strings.HasSuffix(s, "%") {
		if f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64); err == nil {
			return clamp01(f / 100)
		}
		return 1
	} else if f, err := strconv.ParseFloat(s, 64); err == nil {
		return clamp01(f)
	}
	return 1
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func svgStrokeWidth(s string) float64 {
	if f, ok := svgLength(s, 0); ok && f >= 0 {
		return f
	}
	return 1
}

func svgPoints(s string) [][2]float64 {
	var out [][2]float64
	s = strings.ReplaceAll(s, ",", " ")
	f := strings.Fields(s)
	for i := 0; i+1 < len(f); i += 2 {
		x, err1 := strconv.ParseFloat(f[i], 64)
		y, err2 := strconv.ParseFloat(f[i+1], 64)
		if err1 != nil || err2 != nil {
			return nil
		}
		out = append(out, [2]float64{x, y})
	}
	if len(out) < 1 {
		return nil
	}
	return out
}

func svgPaint(st svgState) (fill, stroke svgColor, sw float64) {
	return st.fill, st.stroke, st.strokeW
}

func svgElementState(parent svgState, el xml.StartElement) svgState {
	st := parent
	st.opacity *= svgOpacity(svgAttr(el, "opacity"))
	if v := svgAttr(el, "fill"); v != "" {
		if strings.HasPrefix(v, "url(") {
			st.fill = svgColor{none: true}
		} else if c, ok := svgColorOf(v, svgOpacity(svgAttr(el, "fill-opacity"))); ok {
			st.fill = c
		}
	} else if v := svgAttr(el, "fill-opacity"); v != "" {
		st.fill.a *= svgOpacity(v)
	}
	if v := svgAttr(el, "stroke"); v != "" {
		if strings.HasPrefix(v, "url(") {
			st.stroke = svgColor{none: true}
		} else if c, ok := svgColorOf(v, svgOpacity(svgAttr(el, "stroke-opacity"))); ok {
			st.stroke = c
		}
	} else if v := svgAttr(el, "stroke-opacity"); v != "" {
		st.stroke.a *= svgOpacity(v)
	}
	if v := svgAttr(el, "stroke-width"); v != "" {
		st.strokeW = svgStrokeWidth(v)
	}
	return svgTransform(st, svgAttr(el, "transform"))
}

type svgCanvas struct {
	px   *gimage.RGBA
	w, h int
}

func (st svgState) invert() ([6]float64, bool) {
	a, b, c, d, e, f := st.ctm[0], st.ctm[1], st.ctm[2], st.ctm[3], st.ctm[4], st.ctm[5]
	det := a*d - b*c
	if det == 0 {
		return [6]float64{}, false
	}
	return [6]float64{d / det, -b / det, -c / det, a / det, (c*f - d*e) / det, (b*e - a*f) / det}, true
}

func (st svgState) devScale() float64 {
	return (st.devScaleX() + st.devScaleY()) / 2
}

func (st svgState) devScaleX() float64 {
	return math.Hypot(st.ctm[0], st.ctm[1])
}

func (st svgState) devScaleY() float64 {
	return math.Hypot(st.ctm[2], st.ctm[3])
}

func (c *svgCanvas) devBox(st svgState, pts ...[2]float64) (float64, float64, float64, float64, bool) {
	return c.devBoxPad(st, 0, pts...)
}

func (c *svgCanvas) devBoxPad(st svgState, pad float64, pts ...[2]float64) (float64, float64, float64, float64, bool) {
	a, b, cc, d, e, f := st.ctm[0], st.ctm[1], st.ctm[2], st.ctm[3], st.ctm[4], st.ctm[5]
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		dx := a*p[0] + cc*p[1] + e
		dy := b*p[0] + d*p[1] + f
		x0, y0 = math.Min(x0, dx), math.Min(y0, dy)
		x1, y1 = math.Max(x1, dx), math.Max(y1, dy)
	}
	x0 -= pad
	y0 -= pad
	x1 += pad
	y1 += pad
	if x0 >= x1 || y0 >= y1 {
		return 0, 0, 0, 0, false
	}
	x0 = math.Max(0, math.Floor(x0))
	y0 = math.Max(0, math.Floor(y0))
	x1 = math.Min(float64(c.w), math.Ceil(x1))
	y1 = math.Min(float64(c.h), math.Ceil(y1))
	if x0 >= x1 || y0 >= y1 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

func svgBlend(dst []uint8, sr, sg, sb, sa float64) {
	if sa <= 0 {
		return
	}
	if sa >= 1 {
		dst[0], dst[1], dst[2], dst[3] = uint8(sr*255+0.5), uint8(sg*255+0.5), uint8(sb*255+0.5), 255
		return
	}
	dr, dg, db, da := float64(dst[0])/255, float64(dst[1])/255, float64(dst[2])/255, float64(dst[3])/255
	inv := 1 - sa
	dst[0] = uint8((sr+dr*inv)*255 + 0.5)
	dst[1] = uint8((sg+dg*inv)*255 + 0.5)
	dst[2] = uint8((sb+db*inv)*255 + 0.5)
	dst[3] = uint8((sa+da*inv)*255 + 0.5)
}

func (c *svgCanvas) paintMask(st svgState, x0, y0, x1, y1 float64, cover func(sx, sy float64) bool, col svgColor) {
	if col.none || col.a <= 0 {
		return
	}
	inv, ok := st.invert()
	if !ok {
		return
	}
	ix0, iy0 := int(x0), int(y0)
	ix1, iy1 := int(x1), int(y1)
	if ix0 < 0 {
		ix0 = 0
	}
	if iy0 < 0 {
		iy0 = 0
	}
	if ix1 > c.w {
		ix1 = c.w
	}
	if iy1 > c.h {
		iy1 = c.h
	}
	if ix0 >= ix1 || iy0 >= iy1 {
		return
	}
	r, g, b, al := col.r*col.a, col.g*col.a, col.b*col.a, col.a
	opaque := al >= 1
	for y := iy0; y < iy1; y++ {
		row := c.px.Pix[y*c.px.Stride:]
		dy := float64(y) + 0.5
		for x := ix0; x < ix1; x++ {
			dx := float64(x) + 0.5
			sx := inv[0]*dx + inv[2]*dy + inv[4]
			sy := inv[1]*dx + inv[3]*dy + inv[5]
			if !cover(sx, sy) {
				continue
			}
			px := row[x*4:]
			if opaque {
				px[0], px[1], px[2], px[3] = uint8(r*255+0.5), uint8(g*255+0.5), uint8(b*255+0.5), 255
			} else {
				svgBlend(px, r, g, b, al)
			}
		}
	}
}

func (c *svgCanvas) paintRect(st svgState, x, y, w, h float64, col svgColor) {
	if w <= 0 || h <= 0 {
		return
	}
	x0, y0, x1, y1, ok := c.devBox(st, [2]float64{x, y}, [2]float64{x + w, y}, [2]float64{x, y + h}, [2]float64{x + w, y + h})
	if !ok {
		return
	}
	c.paintMask(st, x0, y0, x1, y1, func(sx, sy float64) bool {
		return sx >= x && sx < x+w && sy >= y && sy < y+h
	}, col)
}

func (c *svgCanvas) paintRectStroke(st svgState, x, y, w, h, sw float64, stroke, fill svgColor) {
	c.paintRect(st, x-sw/2, y-sw/2, w+sw, h+sw, stroke)
	c.paintRect(st, x+sw/2, y+sw/2, w-sw, h-sw, fill)
}

func (c *svgCanvas) paintEllipse(st svgState, cx, cy, rx, ry float64, col svgColor) {
	if rx <= 0 || ry <= 0 {
		return
	}
	x0, y0, x1, y1, ok := c.devBox(st, [2]float64{cx - rx, cy - ry}, [2]float64{cx + rx, cy - ry}, [2]float64{cx - rx, cy + ry}, [2]float64{cx + rx, cy + ry})
	if !ok {
		return
	}
	c.paintMask(st, x0, y0, x1, y1, func(sx, sy float64) bool {
		dx := (sx - cx) / rx
		dy := (sy - cy) / ry
		return dx*dx+dy*dy <= 1
	}, col)
}

func (c *svgCanvas) paintEllipseStroke(st svgState, cx, cy, rx, ry, sw float64, stroke, fill svgColor) {
	s := st.devScale()
	if s <= 0 {
		s = 1
	}
	d := sw / 2 / s
	c.paintEllipse(st, cx, cy, rx+d, ry+d, stroke)
	c.paintEllipse(st, cx, cy, rx-d, ry-d, fill)
}

func segDist(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func (c *svgCanvas) paintPoly(st svgState, pts [][2]float64, closed bool, fill, stroke svgColor, sw float64) {
	if len(pts) == 0 {
		return
	}
	s := st.devScale()
	if s <= 0 {
		s = 1
	}
	half := sw / 2 / s
	halo := sw * s / 2
	x0, y0, x1, y1, ok := c.devBoxPad(st, halo, pts...)
	if !ok {
		return
	}
	segs := len(pts) - 1
	if closed {
		segs = len(pts)
	}
	if !stroke.none && sw > 0 {
		c.paintMask(st, x0, y0, x1, y1, func(sx, sy float64) bool {
			for i := 0; i < segs; i++ {
				a := pts[i]
				b := pts[(i+1)%len(pts)]
				if segDist(sx, sy, a[0], a[1], b[0], b[1]) <= half {
					return true
				}
			}
			return false
		}, stroke)
	}
	if !fill.none && closed && len(pts) >= 3 {
		c.paintMask(st, x0, y0, x1, y1, func(sx, sy float64) bool {
			inside := false
			for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
				xi, yi := pts[i][0], pts[i][1]
				xj, yj := pts[j][0], pts[j][1]
				if (yi > sy) != (yj > sy) && sx < (xj-xi)*(sy-yi)/(yj-yi)+xi {
					inside = !inside
				}
			}
			return inside
		}, fill)
	}
}

func (c *svgCanvas) dim(st svgState, el xml.StartElement, name string, isX bool) float64 {
	s := svgAttr(el, name)
	base := 0.0
	if strings.Contains(s, "%") {
		sc := st.devScaleX()
		if !isX {
			sc = st.devScaleY()
		}
		if sc <= 0 {
			sc = 1
		}
		if isX {
			base = float64(c.w) / sc
		} else {
			base = float64(c.h) / sc
		}
	}
	v, _ := svgLength(s, base)
	return v
}

func (c *svgCanvas) dimR(st svgState, el xml.StartElement) float64 {
	s := svgAttr(el, "r")
	base := 0.0
	if strings.Contains(s, "%") {
		sc := st.devScale()
		if sc <= 0 {
			sc = 1
		}
		base = math.Hypot(float64(c.w), float64(c.h)) / math.Sqrt2 / sc
	}
	v, _ := svgLength(s, base)
	return v
}

func (c *svgCanvas) paintShape(st svgState, el xml.StartElement) {
	if st.opacity <= 0 {
		return
	}
	fill, stroke, sw := svgPaint(st)
	fill.a *= st.opacity
	stroke.a *= st.opacity
	switch el.Name.Local {
	case "rect":
		x := c.dim(st, el, "x", true)
		y := c.dim(st, el, "y", false)
		w := c.dim(st, el, "width", true)
		h := c.dim(st, el, "height", false)
		if w <= 0 || h <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintRect(st, x, y, w, h, fill)
		} else {
			c.paintRectStroke(st, x, y, w, h, sw, stroke, fill)
		}
	case "circle":
		cx := c.dim(st, el, "cx", true)
		cy := c.dim(st, el, "cy", false)
		r := c.dimR(st, el)
		if r <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintEllipse(st, cx, cy, r, r, fill)
		} else {
			c.paintEllipseStroke(st, cx, cy, r, r, sw, stroke, fill)
		}
	case "ellipse":
		cx := c.dim(st, el, "cx", true)
		cy := c.dim(st, el, "cy", false)
		rx := c.dim(st, el, "rx", true)
		ry := c.dim(st, el, "ry", false)
		if rx <= 0 || ry <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintEllipse(st, cx, cy, rx, ry, fill)
		} else {
			c.paintEllipseStroke(st, cx, cy, rx, ry, sw, stroke, fill)
		}
	case "line":
		x1 := c.dim(st, el, "x1", true)
		y1 := c.dim(st, el, "y1", false)
		x2 := c.dim(st, el, "x2", true)
		y2 := c.dim(st, el, "y2", false)
		c.paintPoly(st, [][2]float64{{x1, y1}, {x2, y2}}, false, svgColor{none: true}, stroke, sw)
	case "polyline":
		pts := svgPoints(svgAttr(el, "points"))
		if len(pts) < 2 {
			return
		}
		c.paintPoly(st, pts, false, svgColor{none: true}, stroke, sw)
	case "polygon":
		pts := svgPoints(svgAttr(el, "points"))
		if len(pts) < 3 {
			return
		}
		c.paintPoly(st, pts, true, fill, stroke, sw)
	case "path":
		for _, sub := range svgPath(svgAttr(el, "d")) {
			if len(sub.pts) < 2 {
				continue
			}
			if sub.closed {
				if len(sub.pts) >= 3 {
					c.paintPoly(st, sub.pts, true, fill, stroke, sw)
				} else {
					c.paintPoly(st, sub.pts, false, svgColor{none: true}, stroke, sw)
				}
				continue
			}
			if !fill.none && len(sub.pts) >= 3 {
				closed := append(append([][2]float64{}, sub.pts...), sub.pts[0])
				c.paintPoly(st, closed, true, fill, svgColor{none: true}, sw)
			}
			c.paintPoly(st, sub.pts, false, svgColor{none: true}, stroke, sw)
		}
	}
}

func svgRootState(el xml.StartElement, w, h int) svgState {
	sx, sy, ox, oy := svgMapping(el, w, h)
	return svgState{
		fill:    svgColor{r: 0, g: 0, b: 0, a: 1},
		stroke:  svgColor{none: true},
		opacity: 1,
		strokeW: 1,
		ctm:     [6]float64{sx, 0, 0, sy, ox, oy},
	}
}

func parseSVG(data []byte, cv *svgCanvas, forceW, forceH int) (w, h int, err error) {
	if err := rejectSVGAttackSurface(data); err != nil {
		return 0, 0, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	dec.Entity = map[string]string{}
	nElements := 0
	depth := 0
	haveRoot := false
	var stack []svgState
	skipDepth := -1
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			nElements++
			if nElements > maxSVGElements {
				return 0, 0, fmt.Errorf("image: svg element count exceeds limit %d", maxSVGElements)
			}
			depth++
			if depth > maxSVGDepth {
				return 0, 0, fmt.Errorf("image: svg nesting depth exceeds limit %d", maxSVGDepth)
			}
			if skipDepth >= 0 {
				continue
			}
			name := t.Name.Local
			if !haveRoot {
				if name != "svg" {
					return 0, 0, fmt.Errorf("image: not an svg document")
				}
				haveRoot = true
				var err error
				if forceW > 0 && forceH > 0 {
					if err := rejectDegenerateViewBox(t); err != nil {
						return 0, 0, err
					}
					w, h = forceW, forceH
				} else {
					w, h, err = svgViewport(t)
					if err != nil {
						return 0, 0, err
					}
				}
				if cv != nil {
					cv.w, cv.h = w, h
					cv.px = gimage.NewRGBA(gimage.Rect(0, 0, w, h))
				}
				base := svgRootState(t, w, h)
				stack = append(stack, svgElementState(base, t))
				continue
			}
			parent := stack[len(stack)-1]
			switch name {
			case "g", "a":
				stack = append(stack, svgElementState(parent, t))
			case "rect", "circle", "ellipse", "line", "polyline", "polygon", "path":
				st := svgElementState(parent, t)
				stack = append(stack, st)
				if cv != nil {
					cv.paintShape(st, t)
				}
			case "title", "desc", "defs", "mask", "clipPath", "pattern",
				"linearGradient", "radialGradient", "filter", "text", "tspan",
				"image", "use", "foreignObject", "script", "style", "animate",
				"animateTransform", "animateMotion", "set", "switch",
				"symbol", "marker", "view":
				skipDepth = depth
			default:
				stack = append(stack, svgElementState(parent, t))
			}
		case xml.EndElement:
			if skipDepth >= 0 {
				if depth == skipDepth {
					skipDepth = -1
				}
				depth--
				continue
			}
			depth--
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if !haveRoot {
		return 0, 0, fmt.Errorf("image: not an svg document")
	}
	return w, h, nil
}

func svgProbeDims(data []byte) (int, int, error) {
	w, h, err := parseSVG(data, nil, 0, 0)
	if err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

func decodeSVG(data []byte) (gimage.Image, error) {
	cv := &svgCanvas{}
	w, h, err := parseSVG(data, cv, 0, 0)
	if err != nil {
		return nil, err
	}
	if cv.px == nil || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("image: svg produced no raster")
	}
	return cv.px, nil
}

func DecodeSVGAt(data []byte, w, h int) (gimage.Image, error) {
	if w <= 0 || h <= 0 || w > MaxImageDimension || h > MaxImageDimension {
		return nil, fmt.Errorf("image: svg raster size %dx%d outside admission", w, h)
	}
	if int64(w) > MaxDecodedImagePixels/int64(h) {
		return nil, fmt.Errorf("image: svg raster pixel count %dx%d exceeds limit %d", w, h, MaxDecodedImagePixels)
	}
	cv := &svgCanvas{}
	if _, _, err := parseSVG(data, cv, w, h); err != nil {
		return nil, err
	}
	if cv.px == nil {
		return nil, fmt.Errorf("image: svg produced no raster")
	}
	return cv.px, nil
}

func SVGPreserveNone(data []byte) bool {
	if err := rejectSVGAttackSurface(data); err != nil {
		return false
	}
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "svg" {
				return false
			}
			par := strings.ToLower(strings.TrimSpace(svgAttr(start, "preserveAspectRatio")))
			return par == "none" || strings.HasPrefix(par, "none ")
		}
	}
}

func SVGIntrinsicKind(data []byte) (hasDims, hasRatio bool) {
	if err := rejectSVGAttackSurface(data); err != nil {
		return false, false
	}
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return false, false
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "svg" {
				return false, false
			}
			w, wok := svgLength(svgAttr(start, "width"), 0)
			h, hok := svgLength(svgAttr(start, "height"), 0)
			hasDims = wok && hok && w > 0 && h > 0
			vb := strings.Fields(svgAttr(start, "viewBox"))
			if len(vb) == 4 {
				if vw, err := strconv.ParseFloat(vb[2], 64); err == nil && vw > 0 {
					if vh, err := strconv.ParseFloat(vb[3], 64); err == nil && vh > 0 {
						hasRatio = true
					}
				}
			}
			return hasDims, hasRatio
		}
	}
}

func SVGDefersBothSides(data []byte) bool {
	if err := rejectSVGAttackSurface(data); err != nil {
		return false
	}
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "svg" {
				return false
			}
			w, wok := svgLength(svgAttr(start, "width"), 0)
			h, hok := svgLength(svgAttr(start, "height"), 0)
			if wok && w > 0 || hok && h > 0 {
				return false
			}
			vb := strings.Fields(svgAttr(start, "viewBox"))
			if len(vb) == 4 {
				if vw, err := strconv.ParseFloat(vb[2], 64); err == nil && vw > 0 {
					if vh, err := strconv.ParseFloat(vb[3], 64); err == nil && vh > 0 {
						return false
					}
				}
			}
			return true
		}
	}
}

func SVGRatio(data []byte) (rw, rh float64, ok bool) {
	if err := rejectSVGAttackSurface(data); err != nil {
		return 0, 0, false
	}
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.Strict = true
	for {
		tok, err := dec.Token()
		if err != nil {
			return 0, 0, false
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "svg" {
				return 0, 0, false
			}
			vb := strings.Fields(svgAttr(start, "viewBox"))
			if len(vb) != 4 {
				return 0, 0, false
			}
			vw, err := strconv.ParseFloat(vb[2], 64)
			if err != nil || vw <= 0 {
				return 0, 0, false
			}
			vh, err := strconv.ParseFloat(vb[3], 64)
			if err != nil || vh <= 0 {
				return 0, 0, false
			}
			return vw, vh, true
		}
	}
}

func svgConfig(data []byte) (gimage.Config, error) {
	w, h, err := svgProbeDims(data)
	if err != nil {
		return gimage.Config{}, err
	}
	return gimage.Config{Width: w, Height: h}, nil
}

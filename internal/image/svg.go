package image

// Static SVG subset for <img> and CSS image contexts.
//
// What this answers: a standalone SVG document (including data: URIs) with a
// width/height or viewBox, painted from flat shapes - rect, circle, ellipse,
// line, polyline, polygon - with fill/stroke colors and opacity, nested <g>
// fill/stroke inheritance, and translate/scale transforms. Paint order is
// document order over transparency, the way an <img> composites.
//
// What it deliberately does not answer, and never will in an image decoder:
// script, animation, external references (use, image, href, style sheets),
// embedded HTML, text (no font engine is reachable from this package by the
// import rules), paths, gradients/patterns (url() fills paint transparent),
// filters, masks, clipping, and non-uniform preserveAspectRatio modes beyond
// the default meet. Unsupported elements are skipped with their subtrees, so
// a <defs> block never paints and a <path> leaves a hole rather than junk.
//
// Security posture: the input is untrusted bytes. Anything resembling an
// entity attack surface ("<!DOCTYPE", "<!ENTITY", case-insensitive) is refused
// before parsing; element count and nesting depth are capped; the raster is
// bounded by the same MaxImageDimension/MaxDecodedImagePixels admission as
// every other format. There is no script engine, no network, and no font
// loading reachable from here by construction.

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
	// maxSVGElements caps the tags in one document: a static icon holds
	// dozens, and anything past thousands is an attack, not artwork.
	maxSVGElements = 8192
	// maxSVGDepth caps nesting for the same reason.
	maxSVGDepth = 64
	// defaultSVGSize is the CSS replaced-element fallback (CSS Images §5):
	// 300x150 when neither dimensions nor a viewBox answer.
	defaultSVGSize  = 300
	defaultSVGSizeH = 150
)

// svgColor is one sRGBA paint stop: unpremultiplied 0..1 channels with an
// own alpha (fill-opacity baked in). The cumulative opacity chain in the state
// multiplies in once, at draw time.
type svgColor struct {
	r, g, b, a float64
	none       bool // "none" (or fully transparent): paints nothing
}

// svgState is the inherited paint context <g> elements stack. The ctm maps
// shape (viewBox) units to device px; it starts as the viewBox mapping and
// every transform multiplies into it, so translate/scale/rotate/skew/matrix
// all compose exactly.
type svgState struct {
	fill, stroke svgColor
	opacity      float64    // cumulative opacity product down the chain
	strokeW      float64    // inherited stroke-width in shape units
	ctm          [6]float64 // a b c d e f: dx = a*x+c*y+e, dy = b*x+d*y+f
}

// isSVGData sniffs standalone SVG markup: optional XML prolog, comments and
// whitespace, then a root <svg> element. Anything else is not ours.
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

// rejectSVGAttackSurface refuses entity-declaration tricks before parsing:
// internal-subset entities are how billion-laughs payloads smuggle
// exponential expansion into an otherwise_plain XML document, and a static
// <img> SVG never needs a DOCTYPE at all.
func rejectSVGAttackSurface(data []byte) error {
	lower := bytes.ToLower(data)
	if bytes.Contains(lower, []byte("<!doctype")) || bytes.Contains(lower, []byte("<!entity")) {
		return fmt.Errorf("image: svg with DOCTYPE/ENTITY declarations refused")
	}
	return nil
}

// svgLength parses a CSS/SVG length to px. pctBase answers "%" (a viewport
// fraction); em/ex fall back to the 16px root the cascade leaves alone.
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

// svgColorOf parses a paint value: "none", #rgb/#rrggbb/#rrggbbaa, rgb()/rgba(),
// or one of the 16 basic CSS color keywords. url() references (gradients,
// patterns) and anything else report ok=false and paint transparent, which is
// the honest rendering of an unsupported fill.
func svgColorOf(s string, opacity float64) (svgColor, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		// Unspecified: the caller applies the spec default (black fill).
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

// svgKeyword maps the 16 basic CSS color keywords (CSS Color §4). Anything
// beyond them in an SVG fill is rare enough to stay transparent.
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

// svgAttr reads one attribute from a start element.
func svgAttr(el xml.StartElement, name string) string {
	for _, a := range el.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// svgViewport resolves the raster size: width/height win, else the viewBox,
// else the 300x150 replaced-element fallback. Anything non-positive or past
// the admission limits fails before a pixel is allocated.
func svgViewport(el xml.StartElement) (int, int, error) {
	w, wOk := svgLength(svgAttr(el, "width"), 0)
	h, hOk := svgLength(svgAttr(el, "height"), 0)
	if !wOk || !hOk || w <= 0 || h <= 0 {
		vb := strings.Fields(svgAttr(el, "viewBox"))
		if len(vb) == 4 {
			if vw, err := strconv.ParseFloat(vb[2], 64); err == nil && vw > 0 {
				if vh, err := strconv.ParseFloat(vb[3], 64); err == nil && vh > 0 {
					w, h = vw, vh
				}
			}
		}
	}
	if w <= 0 || h <= 0 {
		w, h = defaultSVGSize, defaultSVGSizeH
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

// svgMapping maps viewBox units to device px under preserveAspectRatio="meet"
// (the default): uniform scale, centered. "none" stretches; anything else is
// treated as meet and documented as such.
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

// svgTransform folds translate/scale/rotate/skew/matrix into the state's
// affine matrix by exact composition. Skew and rotate are fully supported:
// coverage tests run in shape space through the inverse map, so no axis
// assumption leaks into rasterization.
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

// svgOpacity parses an opacity/fill-opacity/stroke-opacity value, defaulting
// to 1 and clamping into range.
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

// svgStrokeWidth parses stroke-width in viewBox units; unparseable means the
// spec default 1.
func svgStrokeWidth(s string) float64 {
	if f, ok := svgLength(s, 0); ok && f >= 0 {
		return f
	}
	return 1
}

// svgPoints parses a points="x,y ..." list.
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

// svgPaint resolves the fill/stroke the state already folded: fill defaults
// to spec-black, stroke to none.
func svgPaint(st svgState) (fill, stroke svgColor, sw float64) {
	return st.fill, st.stroke, st.strokeW
}

// svgElementState folds one element's presentation attributes and transform
// over its parent state. Opacity accumulates down the chain and applies once
// at draw; an unparseable paint value keeps the inherited one (the
// declaration is dropped, not transparent): a url() gradient degrades to
// whatever flat paint surrounds it rather than a hole.
func svgElementState(parent svgState, el xml.StartElement) svgState {
	st := parent
	st.opacity *= svgOpacity(svgAttr(el, "opacity"))
	// Paint alphas stay definition-local (their own fill-opacity only); the
	// cumulative chain multiplies once at draw, so inherited paints pick up
	// every opacity above them exactly once.
	if v := svgAttr(el, "fill"); v != "" {
		if strings.HasPrefix(v, "url(") {
			// Gradients/patterns are unsupported: transparent signals the
			// hole honestly instead of inheriting an unrelated flat paint
			// (at the root that would be spec-black).
			st.fill = svgColor{none: true}
		} else if c, ok := svgColorOf(v, svgOpacity(svgAttr(el, "fill-opacity"))); ok {
			st.fill = c
		}
	} else if v := svgAttr(el, "fill-opacity"); v != "" {
		// A lone fill-opacity scales the inherited fill.
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

// svgCanvas is the raster target: premultiplied RGBA, painter's order.
type svgCanvas struct {
	px   *gimage.RGBA
	w, h int
}

// invert returns the shape-space map of a device point. A singular matrix
// (zero scale) collapses the shape to nothing and reports false.
func (st svgState) invert() ([6]float64, bool) {
	a, b, c, d, e, f := st.ctm[0], st.ctm[1], st.ctm[2], st.ctm[3], st.ctm[4], st.ctm[5]
	det := a*d - b*c
	if det == 0 {
		return [6]float64{}, false
	}
	return [6]float64{d / det, -b / det, -c / det, a / det, (c*f - d*e) / det, (b*e - a*f) / det}, true
}

// devScale is the device px per shape unit, for stroke widths: the mean of
// the matrix column norms, exact under uniform scale.
func (st svgState) devScale() float64 {
	a, b, c, d := st.ctm[0], st.ctm[1], st.ctm[2], st.ctm[3]
	return (math.Hypot(a, b) + math.Hypot(c, d)) / 2
}

// devBox maps shape-space corners to a clipped device bbox.
func (c *svgCanvas) devBox(st svgState, pts ...[2]float64) (float64, float64, float64, float64, bool) {
	return c.devBoxPad(st, 0, pts...)
}

// devBoxPad is devBox expanded by pad device px first: strokes and
// zero-area geometry (a horizontal line has no height) still own pixels.
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

// svgBlend composites one premultiplied src pixel src-over dst in place.
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

// paintMask fills device pixels whose inverse-mapped center passes cover.
// col carries definition-local rgb with an already-effective alpha (the
// caller folded the opacity chain); blending is premultiplied src-over.
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
	// Defensive clamp: stroke halos expand past the clipped geometry box.
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

// The painters below evaluate coverage in shape space, so rotation and skew
// need no special casing: the inverse map already unrotated the pixel.

// paintRect fills the axis-aligned rect; rx/ry (rounded corners) are drawn
// square, a documented limitation.
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

// paintRectStroke strokes a rect by painting the outer rect in stroke color
// and the inset interior in fill color: exact under any affine map because
// both passes share it.
func (c *svgCanvas) paintRectStroke(st svgState, x, y, w, h, sw float64, stroke, fill svgColor) {
	c.paintRect(st, x-sw/2, y-sw/2, w+sw, h+sw, stroke)
	c.paintRect(st, x+sw/2, y+sw/2, w-sw, h-sw, fill)
}

// paintEllipse fills (or ring-strokes) an ellipse; a circle is rx == ry.
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

// paintEllipseStroke strokes an ellipse as an outer ellipse in stroke color
// with the inner ellipse filled over it.
func (c *svgCanvas) paintEllipseStroke(st svgState, cx, cy, rx, ry, sw float64, stroke, fill svgColor) {
	s := st.devScale()
	if s <= 0 {
		s = 1
	}
	d := sw / 2 / s
	c.paintEllipse(st, cx, cy, rx+d, ry+d, stroke)
	c.paintEllipse(st, cx, cy, rx-d, ry-d, fill)
}

// segDist is the distance from p to segment ab.
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

// paintPoly strokes (and optionally even-odd fills) a point list; closed
// joins the last point to the first for polygons.
func (c *svgCanvas) paintPoly(st svgState, pts [][2]float64, closed bool, fill, stroke svgColor, sw float64) {
	if len(pts) == 0 {
		return
	}
	s := st.devScale()
	if s <= 0 {
		s = 1
	}
	// Stroke tolerance lives in shape space; under uniform scale it equals
	// the device half-width exactly. The bbox pads by the device halo so
	// zero-area geometry (a straight horizontal line) still owns pixels.
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

// paintShape dispatches one element, folding the cumulative opacity into the
// resolved paints exactly once. It reports false for elements with no paint
// of their own (groups are expanded by the walker, the rest skipped).
func (c *svgCanvas) paintShape(st svgState, el xml.StartElement) {
	if st.opacity <= 0 {
		return
	}
	fill, stroke, sw := svgPaint(st)
	fill.a *= st.opacity
	stroke.a *= st.opacity
	switch el.Name.Local {
	case "rect":
		x, _ := svgLength(svgAttr(el, "x"), 0)
		y, _ := svgLength(svgAttr(el, "y"), 0)
		w, _ := svgLength(svgAttr(el, "width"), 0)
		h, _ := svgLength(svgAttr(el, "height"), 0)
		if w <= 0 || h <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintRect(st, x, y, w, h, fill)
		} else {
			c.paintRectStroke(st, x, y, w, h, sw, stroke, fill)
		}
	case "circle":
		cx, _ := svgLength(svgAttr(el, "cx"), 0)
		cy, _ := svgLength(svgAttr(el, "cy"), 0)
		r, _ := svgLength(svgAttr(el, "r"), 0)
		if r <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintEllipse(st, cx, cy, r, r, fill)
		} else {
			c.paintEllipseStroke(st, cx, cy, r, r, sw, stroke, fill)
		}
	case "ellipse":
		cx, _ := svgLength(svgAttr(el, "cx"), 0)
		cy, _ := svgLength(svgAttr(el, "cy"), 0)
		rx, _ := svgLength(svgAttr(el, "rx"), 0)
		ry, _ := svgLength(svgAttr(el, "ry"), 0)
		if rx <= 0 || ry <= 0 {
			return
		}
		if stroke.none || sw <= 0 {
			c.paintEllipse(st, cx, cy, rx, ry, fill)
		} else {
			c.paintEllipseStroke(st, cx, cy, rx, ry, sw, stroke, fill)
		}
	case "line":
		x1, _ := svgLength(svgAttr(el, "x1"), 0)
		y1, _ := svgLength(svgAttr(el, "y1"), 0)
		x2, _ := svgLength(svgAttr(el, "x2"), 0)
		y2, _ := svgLength(svgAttr(el, "y2"), 0)
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
	}
	// path, text, image, use, foreignObject, filter, mask, clipPath and
	// everything else: skipped with their subtrees by the walker.
}

// svgRootState builds the initial paint state from the viewBox mapping:
// identity user transform, opaque, spec-default black fill, no stroke,
// width-1 stroke for when a stroke appears.
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

// parseSVG parses a standalone SVG document with admission guards. Shapes
// rasterize onto cv as encountered (document order); every pushed state pops
// exactly once, including shapes and the root.
func parseSVG(data []byte, cv *svgCanvas) (w, h int, err error) {
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
				w, h, err = svgViewport(t)
				if err != nil {
					return 0, 0, err
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
				// <a> without link semantics paints its children; hrefs are
				// never followed.
				stack = append(stack, svgElementState(parent, t))
			case "rect", "circle", "ellipse", "line", "polyline", "polygon":
				st := svgElementState(parent, t)
				stack = append(stack, st)
				if cv != nil {
					cv.paintShape(st, t)
				}
			case "title", "desc", "defs", "mask", "clipPath", "pattern",
				"linearGradient", "radialGradient", "filter", "text", "tspan",
				"image", "use", "foreignObject", "script", "style", "animate",
				"animateTransform", "animateMotion", "set", "path", "switch",
				"symbol", "marker", "view":
				// Unsupported or metadata: skip the subtree. Script never
				// runs, external references never resolve, animation never
				// advances - there is no clock, network, or JS here.
				skipDepth = depth
			default:
				// Unknown elements paint nothing but their children may hold
				// supported shapes (e.g. extension wrappers): inherit state.
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

// svgProbeDims returns the raster size without painting.
func svgProbeDims(data []byte) (int, int, error) {
	w, h, err := parseSVG(data, nil)
	if err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

// decodeSVG rasterizes a standalone SVG document to premultiplied RGBA.
func decodeSVG(data []byte) (gimage.Image, error) {
	cv := &svgCanvas{}
	w, h, err := parseSVG(data, cv)
	if err != nil {
		return nil, err
	}
	if cv.px == nil || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("image: svg produced no raster")
	}
	return cv.px, nil
}

// svgConfig adapts SVG dimensions to an image.Config for Probe.
func svgConfig(data []byte) (gimage.Config, error) {
	w, h, err := svgProbeDims(data)
	if err != nil {
		return gimage.Config{}, err
	}
	return gimage.Config{Width: w, Height: h}, nil
}

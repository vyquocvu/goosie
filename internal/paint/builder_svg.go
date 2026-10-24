package paint

import (
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// svgNamespace is the NSSVG constant value (defined in internal/dom).
// We duplicate it here to avoid importing dom from paint, which would
// violate the import boundary rules.
const svgNamespace = 1

// svgMapping maps user-space coordinates from a root's viewBox into its CSS
// viewport under the default preserveAspectRatio="xMidYMid meet". Without a
// usable viewBox it is the identity at the viewport origin.
type svgMapping struct {
	ox, oy float32 // viewport origin, CSS px
	ux, uy float32 // viewBox min-x, min-y
	s      float32 // user units to CSS px
}

func newSVGMapping(viewBox string, x0, y0, w, h float32) svgMapping {
	m := svgMapping{ox: x0, oy: y0, s: 1}
	f := strings.Fields(viewBox)
	if len(f) != 4 {
		return m
	}
	minX, err1 := strconv.ParseFloat(f[0], 32)
	minY, err2 := strconv.ParseFloat(f[1], 32)
	vbW, err3 := strconv.ParseFloat(f[2], 32)
	vbH, err4 := strconv.ParseFloat(f[3], 32)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || vbW <= 0 || vbH <= 0 {
		return m
	}
	s := w / float32(vbW)
	if sy := h / float32(vbH); sy < s {
		s = sy
	}
	m.s = s
	m.ux, m.uy = float32(minX), float32(minY)
	m.ox = x0 + (w-float32(vbW)*s)/2
	m.oy = y0 + (h-float32(vbH)*s)/2
	return m
}

func (m svgMapping) point(x, y float32) (float32, float32) {
	return m.ox + (x-m.ux)*m.s, m.oy + (y-m.uy)*m.s
}

func (m svgMapping) length(v float32) float32 { return v * m.s }

// paintSVGShape draws an SVG shape element (rect, circle, ellipse, line,
// polygon, polyline, path) using the element's geometry attributes and
// presentation style. Shapes are rasterized into fill/stroke commands.
func (b *Builder) paintSVGShape(obj *layout.Object, opacity float32) {
	if obj.Node == nil || obj.Node.Namespace != svgNamespace { // NSSVG
		return
	}
	// Locate enclosing <svg> element to obtain viewport origin and bounds.
	svgObj := obj
	for svgObj != nil && (svgObj.Node == nil || svgObj.Node.Data != "svg") {
		if svgObj.Parent == 0 {
			svgObj = nil
			break
		}
		svgObj = b.arena.Get(svgObj.Parent)
	}
	if svgObj == nil {
		return
	}
	x0, y0, x1, y1 := svgObj.ContentRect()
	if x1 <= x0 || y1 <= y0 {
		return
	}
	scale := b.scale
	tag := obj.Node.Data
	m := newSVGMapping(svgObj.Node.GetAttribute("viewBox"), x0, y0, x1-x0, y1-y0)

	// Parse fill color from style or attribute.
	fillColor := b.parseSVGPaint(obj, "fill", "black")
	strokeColor := b.parseSVGPaint(obj, "stroke", "")
	strokeWidth := b.parseSVGLength(obj, "stroke-width", 1) * m.length(1) * scale

	switch tag {
	case "rect":
		b.paintSVGRect(obj, m, fillColor, strokeColor, strokeWidth, opacity)
	case "circle":
		b.paintSVGCircle(obj, m, fillColor, strokeColor, strokeWidth, opacity)
	case "ellipse":
		b.paintSVGEllipse(obj, m, fillColor, strokeColor, strokeWidth, opacity)
	case "line":
		b.paintSVGLine(obj, m, strokeColor, strokeWidth, opacity)
	case "polygon", "polyline":
		b.paintSVGPolygon(obj, m, fillColor, strokeColor, strokeWidth, tag == "polyline", opacity)
	case "path":
		b.paintSVGPath(obj, m, fillColor, strokeColor, strokeWidth, opacity)
	}
}

// parseSVGPaint returns the fill/stroke color for an SVG element. It checks
// presentation attributes, inline style declarations, and currentColor.
func (b *Builder) parseSVGPaint(obj *layout.Object, attr, fallback string) frame.Color {
	if obj.Node == nil {
		return 0
	}
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		if styleAttr := obj.Node.GetAttribute("style"); styleAttr != "" {
			for _, decl := range strings.Split(styleAttr, ";") {
				parts := strings.SplitN(decl, ":", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[0]) == attr {
					v = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}
	if v == "" {
		v = fallback
	}
	if v == "" || v == "none" {
		return 0
	}
	if v == "currentColor" {
		if obj.Style != nil {
			return convertColor(obj.Style.Color)
		}
		return frame.RGB(0, 0, 0)
	}
	return parseSVGColor(v)
}

// parseSVGLength parses an SVG length attribute, returning the value in
// user units (pixels).
func (b *Builder) parseSVGLength(obj *layout.Object, attr string, fallback float32) float32 {
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		return fallback
	}
	// Strip units if present (px, em, etc.) - we treat everything as px.
	v = strings.TrimSuffix(v, "px")
	v = strings.TrimSuffix(v, "em")
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

// parseSVGColor parses a CSS color string using css.ParseColor.
func parseSVGColor(s string) frame.Color {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return 0
	}
	if c, ok := css.ParseColor(s); ok {
		return frame.RGBA(c.R, c.G, c.B, c.A)
	}
	return 0
}

func (b *Builder) paintSVGRect(obj *layout.Object, m svgMapping, fill, stroke frame.Color, strokeW float32, opacity float32) {
	x := b.parseSVGLengthAttr(obj, "x", 0)
	y := b.parseSVGLengthAttr(obj, "y", 0)
	w := b.parseSVGLengthAttr(obj, "width", 0)
	h := b.parseSVGLengthAttr(obj, "height", 0)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", rx)

	scale := b.scale
	x0, y0 := m.point(x, y)
	x1, y1 := m.point(x+w, y+h)
	rect := frame.RectF4(x0, y0, x1, y1).ToDevice(scale)
	if rect.Empty() {
		return
	}

	if fill.A() > 0 {
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: fill, Opacity: opacity})
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokeRect(rect, stroke, strokeW, m.length(rx)*scale, m.length(ry)*scale, opacity)
	}
}

func (b *Builder) paintSVGCircle(obj *layout.Object, m svgMapping, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", 0)
	cy := b.parseSVGLengthAttr(obj, "cy", 0)
	r := b.parseSVGLengthAttr(obj, "r", 0)
	if r <= 0 {
		return
	}
	scale := b.scale
	px, py := m.point(cx, cy)
	cxp := px * scale
	cyp := py * scale
	rp := m.length(r) * scale

	if fill.A() > 0 {
		b.paintSVGFilledCircle(cxp, cyp, rp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedCircle(cxp, cyp, rp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGEllipse(obj *layout.Object, m svgMapping, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", 0)
	cy := b.parseSVGLengthAttr(obj, "cy", 0)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", 0)
	if rx <= 0 || ry <= 0 {
		return
	}
	scale := b.scale
	px, py := m.point(cx, cy)
	cxp := px * scale
	cyp := py * scale
	rxp := m.length(rx) * scale
	ryp := m.length(ry) * scale

	if fill.A() > 0 {
		b.paintSVGFilledEllipse(cxp, cyp, rxp, ryp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedEllipse(cxp, cyp, rxp, ryp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGLine(obj *layout.Object, m svgMapping, stroke frame.Color, strokeW float32, opacity float32) {
	x1 := b.parseSVGLengthAttr(obj, "x1", 0)
	y1 := b.parseSVGLengthAttr(obj, "y1", 0)
	x2 := b.parseSVGLengthAttr(obj, "x2", 0)
	y2 := b.parseSVGLengthAttr(obj, "y2", 0)
	if stroke.A() <= 0 || strokeW <= 0 {
		return
	}
	scale := b.scale
	x1, y1 = m.point(x1, y1)
	x2, y2 = m.point(x2, y2)
	b.paintSVGLineSegment(x1*scale, y1*scale, x2*scale, y2*scale, stroke, strokeW, opacity)
}

func (b *Builder) paintSVGPolygon(obj *layout.Object, m svgMapping, fill, stroke frame.Color, strokeW float32, open bool, opacity float32) {
	pointsStr := obj.Node.GetAttribute("points")
	if pointsStr == "" {
		return
	}
	points := parseSVGPoints(pointsStr)
	if len(points) < 2 {
		return
	}
	scale := b.scale
	// Map user units through the viewBox, then to device pixels.
	for i := range points {
		x, y := m.point(points[i][0], points[i][1])
		points[i][0] = x * scale
		points[i][1] = y * scale
	}

	if !open && fill.A() > 0 {
		b.paintSVGFilledPolygon(points, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedPolygon(points, stroke, strokeW, open, opacity)
	}
}

func (b *Builder) paintSVGPath(obj *layout.Object, m svgMapping, fill, stroke frame.Color, strokeW float32, opacity float32) {
	d := obj.Node.GetAttribute("d")
	if d == "" {
		return
	}
	segments := parseSVGPath(d)
	if len(segments) == 0 {
		return
	}
	scale := b.scale
	// Map user units through the viewBox, then to device pixels.
	for i := range segments {
		for j := 0; j+1 < len(segments[i]); j += 2 {
			x, y := m.point(segments[i][j], segments[i][j+1])
			segments[i][j] = x * scale
			segments[i][j+1] = y * scale
		}
	}

	if fill.A() > 0 {
		b.paintSVGFilledPath(segments, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedPath(segments, stroke, strokeW, opacity)
	}
}

func (b *Builder) parseSVGLengthAttr(obj *layout.Object, attr string, fallback float32) float32 {
	if obj.Node == nil {
		return fallback
	}
	v := obj.Node.GetAttribute(attr)
	if v == "" {
		return fallback
	}
	v = strings.TrimSuffix(v, "px")
	v = strings.TrimSuffix(v, "em")
	v = strings.TrimSuffix(v, "%")
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

func (b *Builder) paintSVGStrokeRect(rect frame.Rect, color frame.Color, width, rx, ry float32, opacity float32) {
	sw := int32(math.Ceil(float64(width)))
	if sw < 1 {
		sw = 1
	}
	w := rect.W()
	h := rect.H()
	if w <= 0 || h <= 0 {
		return
	}
	if sw*2 >= w || sw*2 >= h {
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: color, Opacity: opacity})
		return
	}
	// Top
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0, rect.X1, rect.Y0+sw), Color: color, Opacity: opacity})
	// Bottom
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y1-sw, rect.X1, rect.Y1), Color: color, Opacity: opacity})
	// Left
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X0, rect.Y0+sw, rect.X0+sw, rect.Y1-sw), Color: color, Opacity: opacity})
	// Right
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: frame.Rect4(rect.X1-sw, rect.Y0+sw, rect.X1, rect.Y1-sw), Color: color, Opacity: opacity})
	_ = rx
	_ = ry
}

func (b *Builder) paintSVGFilledCircle(cx, cy, r float32, color frame.Color, opacity float32) {
	ri := int32(r)
	for dy := -ri; dy <= ri; dy++ {
		for dx := -ri; dx <= ri; dx++ {
			if float32(dx*dx)+float32(dy*dy) <= r*r {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGStrokedCircle(cx, cy, r float32, color frame.Color, width float32, opacity float32) {
	ri := int32(r)
	hw := width / 2
	for dy := -ri; dy <= ri; dy++ {
		for dx := -ri; dx <= ri; dx++ {
			dist := float32(dx*dx) + float32(dy*dy)
			outerR := r + hw
			innerR := r - hw
			if dist <= outerR*outerR && dist >= innerR*innerR {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGFilledEllipse(cx, cy, rx, ry float32, color frame.Color, opacity float32) {
	rxi := int32(rx)
	ryi := int32(ry)
	for dy := -ryi; dy <= ryi; dy++ {
		for dx := -rxi; dx <= rxi; dx++ {
			if float32(dx*dx)/(rx*rx)+float32(dy*dy)/(ry*ry) <= 1 {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGStrokedEllipse(cx, cy, rx, ry float32, color frame.Color, width float32, opacity float32) {
	rxi := int32(rx)
	ryi := int32(ry)
	hw := width / 2
	for dy := -ryi; dy <= ryi; dy++ {
		for dx := -rxi; dx <= rxi; dx++ {
			dist := float32(dx*dx)/(rx*rx) + float32(dy*dy)/(ry*ry)
			outerScale := 1 + hw/rx
			innerScale := 1 - hw/rx
			if dist <= outerScale*outerScale && dist >= innerScale*innerScale {
				b.list.Append(DisplayCmd{
					Kind:    CmdFill,
					Rect:    frame.Rect4(int32(cx)+dx, int32(cy)+dy, int32(cx)+dx+1, int32(cy)+dy+1),
					Color:   color,
					Opacity: opacity,
				})
			}
		}
	}
}

func (b *Builder) paintSVGLineSegment(x1, y1, x2, y2 float32, color frame.Color, width float32, opacity float32) {
	// Bresenham-like line drawing with thickness.
	dx := x2 - x1
	dy := y2 - y1
	steps := int32(math.Sqrt(float64(dx*dx + dy*dy)))
	if steps < 1 {
		steps = 1
	}
	hw := width / 2
	for i := int32(0); i <= steps; i++ {
		t := float32(i) / float32(steps)
		px := x1 + dx*t
		py := y1 + dy*t
		// Draw a small square at each step for thickness.
		size := int32(width)
		if size < 1 {
			size = 1
		}
		b.list.Append(DisplayCmd{
			Kind:    CmdFill,
			Rect:    frame.Rect4(int32(px)-int32(hw), int32(py)-int32(hw), int32(px)+int32(hw)+1, int32(py)+int32(hw)+1),
			Color:   color,
			Opacity: opacity,
		})
		_ = size
	}
}

func (b *Builder) paintSVGFilledPolygon(points [][2]float32, color frame.Color, opacity float32) {
	if len(points) < 3 {
		return
	}
	// Find bounding box.
	minX, minY := points[0][0], points[0][1]
	maxX, maxY := minX, minY
	for _, p := range points[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	// Scanline fill.
	for y := int32(minY); y <= int32(maxY); y++ {
		var intersections []float32
		n := len(points)
		for i := 0; i < n; i++ {
			j := (i + 1) % n
			yi, yj := points[i][1], points[j][1]
			xi, xj := points[i][0], points[j][0]
			if (yi <= float32(y) && yj > float32(y)) || (yj <= float32(y) && yi > float32(y)) {
				t := (float32(y) - yi) / (yj - yi)
				intersections = append(intersections, xi+t*(xj-xi))
			}
		}
		// Sort intersections.
		for i := 0; i < len(intersections); i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[j] < intersections[i] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}
		// Fill between pairs.
		for i := 0; i+1 < len(intersections); i += 2 {
			x0 := int32(intersections[i])
			x1 := int32(intersections[i+1])
			b.list.Append(DisplayCmd{
				Kind:    CmdFill,
				Rect:    frame.Rect4(x0, y, x1+1, y+1),
				Color:   color,
				Opacity: opacity,
			})
		}
	}
}

func (b *Builder) paintSVGStrokedPolygon(points [][2]float32, color frame.Color, width float32, open bool, opacity float32) {
	n := len(points)
	end := n
	if open {
		end = n - 1
	}
	for i := 0; i < end; i++ {
		j := (i + 1) % n
		b.paintSVGLineSegment(points[i][0], points[i][1], points[j][0], points[j][1], color, width, opacity)
	}
}

func (b *Builder) paintSVGFilledPath(segments [][]float32, color frame.Color, opacity float32) {
	// Each subpath is an independent region: joining them into one polygon
	// would draw connecting edges between shapes that don't touch.
	for _, seg := range segments {
		points := make([][2]float32, 0, len(seg)/2)
		for i := 0; i+1 < len(seg); i += 2 {
			points = append(points, [2]float32{seg[i], seg[i+1]})
		}
		if len(points) >= 3 {
			b.paintSVGFilledPolygon(points, color, opacity)
		}
	}
}

func (b *Builder) paintSVGStrokedPath(segments [][]float32, color frame.Color, width float32, opacity float32) {
	for _, seg := range segments {
		for i := 0; i+3 < len(seg); i += 2 {
			b.paintSVGLineSegment(seg[i], seg[i+1], seg[i+2], seg[i+3], color, width, opacity)
		}
	}
}

// parseSVGPoints parses an SVG points attribute value into coordinate pairs.
func parseSVGPoints(s string) [][2]float32 {
	var points [][2]float32
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", " ")
	fields := strings.Fields(s)
	for i := 0; i+1 < len(fields); i += 2 {
		x, err1 := strconv.ParseFloat(fields[i], 32)
		y, err2 := strconv.ParseFloat(fields[i+1], 32)
		if err1 == nil && err2 == nil {
			points = append(points, [2]float32{float32(x), float32(y)})
		}
	}
	return points
}

// parseSVGPath flattens path data into moveto-delimited subpaths of user-space
// points. Supported commands: M/m L/l H/h V/v Z/z, cubics C/c S/s, quadratics
// Q/q T/t and elliptical arcs A/a, with implicit command repetition. Curves are
// subdivided into line segments. The flattening lives here rather than reusing
// internal/image's path decoder because paint may not import it (see archtest).
func parseSVGPath(d string) [][]float32 {
	p := &svgPathScanner{d: d}
	var out [][]float32
	var cur, start [2]float32
	var seg []float32
	haveSub := false
	var lastC, lastQ [2]float32
	haveC, haveQ := false, false

	flush := func() {
		if len(seg) > 0 {
			out = append(out, seg)
		}
		seg = nil
		haveSub = false
	}
	add := func(pt [2]float32) { seg = append(seg, pt[0], pt[1]) }

	var cmd byte
	for {
		p.skipSep()
		if p.eof() {
			break
		}
		c := p.d[p.i]
		if isSVGPathCmd(c) {
			cmd = c
			p.i++
			if cmd == 'Z' || cmd == 'z' {
				if haveSub {
					add(start)
					flush()
					cur = start
				}
				haveC, haveQ = false, false
				cmd = 0
				continue
			}
		} else if cmd == 0 {
			break
		}
		switch cmd {
		case 'M', 'm':
			x, ok1 := p.number()
			y, ok2 := p.number()
			if !ok1 || !ok2 {
				return out
			}
			if cmd == 'm' {
				x += cur[0]
				y += cur[1]
			}
			flush()
			cur = [2]float32{x, y}
			start = cur
			add(cur)
			haveSub = true
			if cmd == 'm' {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
			haveC, haveQ = false, false
		case 'L', 'l':
			x, ok1 := p.number()
			y, ok2 := p.number()
			if !ok1 || !ok2 {
				return out
			}
			if cmd == 'l' {
				x += cur[0]
				y += cur[1]
			}
			cur = [2]float32{x, y}
			add(cur)
			haveSub = true
			haveC, haveQ = false, false
		case 'H', 'h':
			x, ok := p.number()
			if !ok {
				return out
			}
			if cmd == 'h' {
				x += cur[0]
			}
			cur[0] = x
			add(cur)
			haveSub = true
			haveC, haveQ = false, false
		case 'V', 'v':
			y, ok := p.number()
			if !ok {
				return out
			}
			if cmd == 'v' {
				y += cur[1]
			}
			cur[1] = y
			add(cur)
			haveSub = true
			haveC, haveQ = false, false
		case 'C', 'c':
			var v [6]float32
			for k := range v {
				n, ok := p.number()
				if !ok {
					return out
				}
				v[k] = n
			}
			p1 := [2]float32{v[0], v[1]}
			p2 := [2]float32{v[2], v[3]}
			p3 := [2]float32{v[4], v[5]}
			if cmd == 'c' {
				p1[0] += cur[0]
				p1[1] += cur[1]
				p2[0] += cur[0]
				p2[1] += cur[1]
				p3[0] += cur[0]
				p3[1] += cur[1]
			}
			for _, q := range flattenSVGCubic(cur, p1, p2, p3) {
				add(q)
			}
			lastC, haveC, haveQ = p2, true, false
			cur = p3
			haveSub = true
		case 'S', 's':
			var v [4]float32
			for k := range v {
				n, ok := p.number()
				if !ok {
					return out
				}
				v[k] = n
			}
			p1 := cur
			if haveC {
				p1 = [2]float32{2*cur[0] - lastC[0], 2*cur[1] - lastC[1]}
			}
			p2 := [2]float32{v[0], v[1]}
			p3 := [2]float32{v[2], v[3]}
			if cmd == 's' {
				p2[0] += cur[0]
				p2[1] += cur[1]
				p3[0] += cur[0]
				p3[1] += cur[1]
			}
			for _, q := range flattenSVGCubic(cur, p1, p2, p3) {
				add(q)
			}
			lastC, haveC, haveQ = p2, true, false
			cur = p3
			haveSub = true
		case 'Q', 'q':
			var v [4]float32
			for k := range v {
				n, ok := p.number()
				if !ok {
					return out
				}
				v[k] = n
			}
			p1 := [2]float32{v[0], v[1]}
			p2 := [2]float32{v[2], v[3]}
			if cmd == 'q' {
				p1[0] += cur[0]
				p1[1] += cur[1]
				p2[0] += cur[0]
				p2[1] += cur[1]
			}
			for _, q := range flattenSVGQuad(cur, p1, p2) {
				add(q)
			}
			lastQ, haveQ, haveC = p1, true, false
			cur = p2
			haveSub = true
		case 'T', 't':
			x, ok1 := p.number()
			y, ok2 := p.number()
			if !ok1 || !ok2 {
				return out
			}
			p1 := cur
			if haveQ {
				p1 = [2]float32{2*cur[0] - lastQ[0], 2*cur[1] - lastQ[1]}
			}
			p2 := [2]float32{x, y}
			if cmd == 't' {
				p2[0] += cur[0]
				p2[1] += cur[1]
			}
			for _, q := range flattenSVGQuad(cur, p1, p2) {
				add(q)
			}
			lastQ, haveQ, haveC = p1, true, false
			cur = p2
			haveSub = true
		case 'A', 'a':
			var v [7]float32
			for k := range v {
				n, ok := p.number()
				if !ok {
					return out
				}
				v[k] = n
			}
			end := [2]float32{v[5], v[6]}
			if cmd == 'a' {
				end[0] += cur[0]
				end[1] += cur[1]
			}
			for _, q := range flattenSVGArc(cur, v[0], v[1], v[2], v[3] != 0, v[4] != 0, end) {
				add(q)
			}
			haveC, haveQ = false, false
			cur = end
			haveSub = true
		default:
			return out
		}
	}
	flush()
	return out
}

func isSVGPathCmd(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's',
		'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

// svgPathScanner reads path data: whitespace and commas separate values, and a
// sign or decimal point can start the next number without a separator.
type svgPathScanner struct {
	d string
	i int
}

func (p *svgPathScanner) eof() bool { return p.i >= len(p.d) }

func (p *svgPathScanner) skipSep() {
	for p.i < len(p.d) {
		switch p.d[p.i] {
		case ' ', '\t', '\n', '\r', ',', '\f':
			p.i++
			continue
		}
		break
	}
}

func (p *svgPathScanner) number() (float32, bool) {
	p.skipSep()
	if p.i >= len(p.d) {
		return 0, false
	}
	start := p.i
	if p.d[p.i] == '-' || p.d[p.i] == '+' {
		p.i++
	}
	digits := false
	for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
		p.i++
		digits = true
	}
	if p.i < len(p.d) && p.d[p.i] == '.' {
		// A second dot starts the next number ("1.5.25" is 1.5 then .25).
		p.i++
		for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
			p.i++
			digits = true
		}
	}
	if !digits {
		return 0, false
	}
	if p.i < len(p.d) && (p.d[p.i] == 'e' || p.d[p.i] == 'E') {
		j := p.i + 1
		if j < len(p.d) && (p.d[j] == '-' || p.d[j] == '+') {
			j++
		}
		k := j
		for k < len(p.d) && p.d[k] >= '0' && p.d[k] <= '9' {
			k++
		}
		if k > j {
			p.i = k
		}
	}
	v, err := strconv.ParseFloat(p.d[start:p.i], 32)
	if err != nil {
		return 0, false
	}
	return float32(v), true
}

// flattenSVGCubic subdivides a cubic Bézier into line segments, excluding the
// start point and ending exactly on p3.
func flattenSVGCubic(p0, p1, p2, p3 [2]float32) [][2]float32 {
	const n = 12
	out := make([][2]float32, 0, n)
	for i := 1; i <= n; i++ {
		t := float32(i) / n
		mt := 1 - t
		out = append(out, [2]float32{
			mt*mt*mt*p0[0] + 3*mt*mt*t*p1[0] + 3*mt*t*t*p2[0] + t*t*t*p3[0],
			mt*mt*mt*p0[1] + 3*mt*mt*t*p1[1] + 3*mt*t*t*p2[1] + t*t*t*p3[1],
		})
	}
	return out
}

// flattenSVGQuad subdivides a quadratic Bézier into line segments.
func flattenSVGQuad(p0, p1, p2 [2]float32) [][2]float32 {
	const n = 10
	out := make([][2]float32, 0, n)
	for i := 1; i <= n; i++ {
		t := float32(i) / n
		mt := 1 - t
		out = append(out, [2]float32{
			mt*mt*p0[0] + 2*mt*t*p1[0] + t*t*p2[0],
			mt*mt*p0[1] + 2*mt*t*p1[1] + t*t*p2[1],
		})
	}
	return out
}

// flattenSVGArc converts an endpoint-parameterized elliptical arc to line
// segments via center parametrization (SVG 1.1 §8.5, F.6.5 correction).
func flattenSVGArc(p0 [2]float32, rx, ry, rot float32, large, sweep bool, p1 [2]float32) [][2]float32 {
	x0, y0 := float64(p0[0]), float64(p0[1])
	x1, y1 := float64(p1[0]), float64(p1[1])
	rxf, ryf := math.Abs(float64(rx)), math.Abs(float64(ry))
	if rxf == 0 || ryf == 0 {
		return [][2]float32{p1}
	}
	phi := float64(rot) * math.Pi / 180
	dx := (x0 - x1) / 2
	dy := (y0 - y1) / 2
	x1p := math.Cos(phi)*dx + math.Sin(phi)*dy
	y1p := -math.Sin(phi)*dx + math.Cos(phi)*dy
	lam := x1p*x1p/(rxf*rxf) + y1p*y1p/(ryf*ryf)
	if lam > 1 {
		s := math.Sqrt(lam)
		rxf *= s
		ryf *= s
	}
	num := rxf*rxf*y1p*y1p + ryf*ryf*x1p*x1p
	fac := 0.0
	if num > 0 {
		fac = math.Sqrt(math.Max(0, (rxf*rxf*ryf*ryf-num)/num))
	}
	if large == sweep {
		fac = -fac
	}
	cxp := fac * rxf * y1p / ryf
	cyp := fac * -ryf * x1p / rxf
	cx := math.Cos(phi)*cxp - math.Sin(phi)*cyp + (x0+x1)/2
	cy := math.Sin(phi)*cxp + math.Cos(phi)*cyp + (y0+y1)/2
	angle := func(px, py float64) float64 {
		return math.Atan2((py-cyp)/ryf, (px-cxp)/rxf)
	}
	t1 := angle(x0, y0)
	dth := angle(x1, y1) - t1
	for dth > math.Pi {
		dth -= 2 * math.Pi
	}
	for dth < -math.Pi {
		dth += 2 * math.Pi
	}
	if !sweep && dth > 0 {
		dth -= 2 * math.Pi
	} else if sweep && dth < 0 {
		dth += 2 * math.Pi
	}
	steps := int(math.Abs(dth) / (math.Pi / 16))
	if steps < 4 {
		steps = 4
	}
	if steps > 64 {
		steps = 64
	}
	out := make([][2]float32, 0, steps)
	for i := 1; i <= steps; i++ {
		a := t1 + dth*float64(i)/float64(steps)
		out = append(out, [2]float32{
			float32(cx + rxf*math.Cos(a)*math.Cos(phi) - ryf*math.Sin(a)*math.Sin(phi)),
			float32(cy + rxf*math.Cos(a)*math.Sin(phi) + ryf*math.Sin(a)*math.Cos(phi)),
		})
	}
	return out
}

func isSVGShape(tag string) bool {
	switch tag {
	case "rect", "circle", "ellipse", "line", "polygon", "polyline", "path":
		return true
	}
	return false
}

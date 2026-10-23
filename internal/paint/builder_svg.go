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

	// Parse fill color from style or attribute.
	fillColor := b.parseSVGPaint(obj, "fill", "black")
	strokeColor := b.parseSVGPaint(obj, "stroke", "")
	strokeWidth := b.parseSVGLength(obj, "stroke-width", 1) * scale

	switch tag {
	case "rect":
		b.paintSVGRect(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "circle":
		b.paintSVGCircle(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "ellipse":
		b.paintSVGEllipse(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
	case "line":
		b.paintSVGLine(obj, x0, y0, x1, y1, strokeColor, strokeWidth, opacity)
	case "polygon", "polyline":
		b.paintSVGPolygon(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, tag == "polyline", opacity)
	case "path":
		b.paintSVGPath(obj, x0, y0, x1, y1, fillColor, strokeColor, strokeWidth, opacity)
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

func (b *Builder) paintSVGRect(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	x := b.parseSVGLengthAttr(obj, "x", 0)
	y := b.parseSVGLengthAttr(obj, "y", 0)
	w := b.parseSVGLengthAttr(obj, "width", bx1-bx0)
	h := b.parseSVGLengthAttr(obj, "height", by1-by0)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", rx)

	scale := b.scale
	rect := frame.RectF4(bx0+x, by0+y, bx0+x+w, by0+y+h).ToDevice(scale)
	if rect.Empty() {
		return
	}

	if fill.A() > 0 {
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: rect, Color: fill, Opacity: opacity})
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokeRect(rect, stroke, strokeW, rx*scale, ry*scale, opacity)
	}
}

func (b *Builder) paintSVGCircle(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", (bx1-bx0)/2)
	cy := b.parseSVGLengthAttr(obj, "cy", (by1-by0)/2)
	r := b.parseSVGLengthAttr(obj, "r", 0)
	if r <= 0 {
		return
	}
	scale := b.scale
	cxp := (bx0 + cx) * scale
	cyp := (by0 + cy) * scale
	rp := r * scale

	if fill.A() > 0 {
		b.paintSVGFilledCircle(cxp, cyp, rp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedCircle(cxp, cyp, rp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGEllipse(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	cx := b.parseSVGLengthAttr(obj, "cx", (bx1-bx0)/2)
	cy := b.parseSVGLengthAttr(obj, "cy", (by1-by0)/2)
	rx := b.parseSVGLengthAttr(obj, "rx", 0)
	ry := b.parseSVGLengthAttr(obj, "ry", 0)
	if rx <= 0 || ry <= 0 {
		return
	}
	scale := b.scale
	cxp := (bx0 + cx) * scale
	cyp := (by0 + cy) * scale
	rxp := rx * scale
	ryp := ry * scale

	if fill.A() > 0 {
		b.paintSVGFilledEllipse(cxp, cyp, rxp, ryp, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedEllipse(cxp, cyp, rxp, ryp, stroke, strokeW, opacity)
	}
}

func (b *Builder) paintSVGLine(obj *layout.Object, bx0, by0, bx1, by1 float32, stroke frame.Color, strokeW float32, opacity float32) {
	x1 := b.parseSVGLengthAttr(obj, "x1", 0)
	y1 := b.parseSVGLengthAttr(obj, "y1", 0)
	x2 := b.parseSVGLengthAttr(obj, "x2", bx1-bx0)
	y2 := b.parseSVGLengthAttr(obj, "y2", 0)
	if stroke.A() <= 0 || strokeW <= 0 {
		return
	}
	scale := b.scale
	b.paintSVGLineSegment((bx0+x1)*scale, (by0+y1)*scale, (bx0+x2)*scale, (by0+y2)*scale, stroke, strokeW, opacity)
}

func (b *Builder) paintSVGPolygon(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, open bool, opacity float32) {
	pointsStr := obj.Node.GetAttribute("points")
	if pointsStr == "" {
		return
	}
	points := parseSVGPoints(pointsStr)
	if len(points) < 2 {
		return
	}
	scale := b.scale
	// Offset points to the SVG box origin and scale to device pixels.
	for i := range points {
		points[i][0] = (bx0 + points[i][0]) * scale
		points[i][1] = (by0 + points[i][1]) * scale
	}

	if !open && fill.A() > 0 {
		b.paintSVGFilledPolygon(points, fill, opacity)
	}
	if stroke.A() > 0 && strokeW > 0 {
		b.paintSVGStrokedPolygon(points, stroke, strokeW, open, opacity)
	}
}

func (b *Builder) paintSVGPath(obj *layout.Object, bx0, by0, bx1, by1 float32, fill, stroke frame.Color, strokeW float32, opacity float32) {
	d := obj.Node.GetAttribute("d")
	if d == "" {
		return
	}
	segments := parseSVGPath(d)
	if len(segments) == 0 {
		return
	}
	scale := b.scale
	// Offset to SVG box origin and scale to device pixels.
	for i := range segments {
		for j := range segments[i] {
			if j%2 == 0 {
				segments[i][j] = (bx0 + segments[i][j]) * scale
			} else {
				segments[i][j] = (by0 + segments[i][j]) * scale
			}
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
	// Flatten path to polygon points and fill.
	var points [][2]float32
	for _, seg := range segments {
		for i := 0; i+1 < len(seg); i += 2 {
			points = append(points, [2]float32{seg[i], seg[i+1]})
		}
	}
	if len(points) >= 3 {
		b.paintSVGFilledPolygon(points, color, opacity)
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

// parseSVGPath parses a simplified SVG path data string into line segments.
// Supports M, L, H, V, Z commands.
func parseSVGPath(d string) [][]float32 {
	var segments [][]float32
	var current []float32
	var cx, cy float32

	d = strings.TrimSpace(d)
	i := 0
	for i < len(d) {
		cmd := d[i]
		i++
		// Skip whitespace.
		for i < len(d) && (d[i] == ' ' || d[i] == ',') {
			i++
		}

		switch cmd {
		case 'M', 'm':
			if len(current) > 0 {
				segments = append(segments, current)
			}
			current = nil
			// Parse coordinates.
			coords := parsePathCoords(d, &i)
			if cmd == 'm' && len(segments) > 0 {
				// Relative move.
				prev := segments[len(segments)-1]
				if len(prev) >= 2 {
					cx = prev[len(prev)-2]
					cy = prev[len(prev)-1]
				}
			}
			for j := 0; j+1 < len(coords); j += 2 {
				x, y := coords[j], coords[j+1]
				if cmd == 'm' {
					x += cx
					y += cy
				}
				current = append(current, x, y)
				cx, cy = x, y
			}
		case 'L', 'l':
			coords := parsePathCoords(d, &i)
			for j := 0; j+1 < len(coords); j += 2 {
				x, y := coords[j], coords[j+1]
				if cmd == 'l' {
					x += cx
					y += cy
				}
				current = append(current, x, y)
				cx, cy = x, y
			}
		case 'H', 'h':
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			start := i
			for i < len(d) && (d[i] >= '0' && d[i] <= '9' || d[i] == '.' || d[i] == '-' || d[i] == 'e' || d[i] == 'E' || d[i] == '+') {
				i++
			}
			if i > start {
				x, err := strconv.ParseFloat(d[start:i], 32)
				if err == nil {
					if cmd == 'h' {
						x += float64(cx)
					}
					current = append(current, float32(x), cy)
					cx = float32(x)
				}
			}
		case 'V', 'v':
			for i < len(d) && (d[i] == ' ' || d[i] == ',') {
				i++
			}
			start := i
			for i < len(d) && (d[i] >= '0' && d[i] <= '9' || d[i] == '.' || d[i] == '-' || d[i] == 'e' || d[i] == 'E' || d[i] == '+') {
				i++
			}
			if i > start {
				y, err := strconv.ParseFloat(d[start:i], 32)
				if err == nil {
					if cmd == 'v' {
						y += float64(cy)
					}
					current = append(current, cx, float32(y))
					cy = float32(y)
				}
			}
		case 'Z', 'z':
			if len(current) >= 2 {
				current = append(current, current[0], current[1])
			}
		}
	}
	if len(current) > 0 {
		segments = append(segments, current)
	}
	return segments
}

func parsePathCoords(d string, i *int) []float32 {
	var coords []float32
	for *i < len(d) {
		for *i < len(d) && (d[*i] == ' ' || d[*i] == ',') {
			*i++
		}
		if *i >= len(d) {
			break
		}
		// Check if next char is a command letter.
		c := d[*i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			break
		}
		start := *i
		for *i < len(d) && (d[*i] >= '0' && d[*i] <= '9' || d[*i] == '.' || d[*i] == '-' || d[*i] == 'e' || d[*i] == 'E' || d[*i] == '+') {
			*i++
		}
		if *i > start {
			f, err := strconv.ParseFloat(d[start:*i], 32)
			if err == nil {
				coords = append(coords, float32(f))
			}
		} else {
			*i++
		}
	}
	return coords
}

func isSVGShape(tag string) bool {
	switch tag {
	case "rect", "circle", "ellipse", "line", "polygon", "polyline", "path":
		return true
	}
	return false
}

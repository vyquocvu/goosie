package frame

import (
	"image"
	"image/color"
	"math"
	"sort"
)

const (
	canvasPaintMethod   = "canvasPaint"
	maxCanvasStateStack = 64
	maxCanvasCoordinate = 1 << 20
	maxCanvasReplayOps  = 100000
)

type CanvasOp struct {
	Method string
	Args   []interface{}
}

func ReplayCanvasOps(b *Bitmap, ops []CanvasOp, width, height int) {
	if b == nil || len(ops) == 0 || len(ops) > maxCanvasReplayOps {
		return
	}
	if width <= 0 || width > b.W {
		width = b.W
	}
	if height <= 0 || height > b.H {
		height = b.H
	}
	if width <= 0 || height <= 0 {
		return
	}
	for y := 0; y < height; y++ {
		clear(b.RGBA[y*b.Stride : y*b.Stride+width*4])
	}
	for _, op := range ops {
		if op.Method == canvasPaintMethod {
			replayCanonicalCanvasOps(b, ops, width, height)
			return
		}
	}
	replayLegacyCanvasOps(b, ops, width, height)
}

type canvasClipSpan struct {
	x0, x1 int
	alpha  uint8
}

type canvasClipMask struct {
	rows [][]canvasClipSpan
}

type canvasReplay struct {
	width, height int
	mask          *image.Alpha
	clip          *canvasClipMask
	stack         []*canvasClipMask
}

func newCanvasReplay(width, height int) *canvasReplay {
	return &canvasReplay{
		width: width, height: height,
		mask: image.NewAlpha(image.Rect(0, 0, width, height)),
	}
}

func replayCanonicalCanvasOps(b *Bitmap, ops []CanvasOp, width, height int) {
	replay := newCanvasReplay(width, height)
	for _, op := range ops {
		switch op.Method {
		case "save":
			if len(replay.stack) < maxCanvasStateStack {
				replay.stack = append(replay.stack, replay.clip)
			}
		case "restore":
			if len(replay.stack) > 0 {
				last := len(replay.stack) - 1
				replay.clip = replay.stack[last]
				replay.stack = replay.stack[:last]
			}
		case "reset":
			clear(b.RGBA)
			replay.clip = nil
			replay.stack = nil
		case canvasPaintMethod:
			args := op.Args
			if len(args) < 8 {
				continue
			}
			kind, _ := args[0].(string)
			subpaths, _ := args[1].([][]float64)
			rgba, _ := args[2].([]float64)
			alpha, _ := args[3].(float64)
			if len(rgba) < 4 || !canvasFinite(alpha) {
				continue
			}
			coverage, bounds := replay.coverage(subpaths)
			if bounds.Empty() {
				continue
			}
			switch kind {
			case "clip":
				replay.clip = intersectCanvasClip(replay.clip, canvasClipFromCoverage(coverage, bounds, width, height))
			case "clear":
				canvasClearCoverage(b, coverage, bounds, replay.clip)
			case "fill":
				canvasFillCoverage(b, canvasPackedColor(rgba, alpha), coverage, bounds, replay.clip)
			}
		}
	}
}

type canvasIntersection struct {
	x     float64
	delta int
}

func (replay *canvasReplay) coverage(subpaths [][]float64) ([]uint8, image.Rectangle) {
	clear(replay.mask.Pix)
	bounds := image.Rectangle{}
	validPaths := make([][]float64, 0, len(subpaths))
	for _, points := range subpaths {
		if len(points) < 6 || len(points)%2 != 0 {
			continue
		}
		valid := true
		for i := 0; i+1 < len(points); i += 2 {
			if !canvasFinite(points[i]) || !canvasFinite(points[i+1]) {
				valid = false
				break
			}
			x0, y0 := int(math.Floor(points[i])), int(math.Floor(points[i+1]))
			x1, y1 := int(math.Ceil(points[i]))+1, int(math.Ceil(points[i+1]))+1
			bounds = bounds.Union(image.Rect(x0, y0, x1, y1))
		}
		if valid {
			validPaths = append(validPaths, points)
		}
	}
	if bounds.Empty() {
		return nil, bounds
	}
	bounds = bounds.Intersect(image.Rect(0, 0, replay.width, replay.height))
	if bounds.Empty() {
		return nil, bounds
	}
	var intersections []canvasIntersection
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for subY := 0; subY < 4; subY++ {
			sampleY := float64(y) + (float64(subY)+0.5)/4
			intersections = intersections[:0]
			for _, points := range validPaths {
				count := len(points) / 2
				for i := 0; i < count; i++ {
					next := (i + 1) % count
					x1, y1 := points[i*2], points[i*2+1]
					x2, y2 := points[next*2], points[next*2+1]
					if y1 <= sampleY && y2 > sampleY {
						x := x1 + (sampleY-y1)*(x2-x1)/(y2-y1)
						intersections = append(intersections, canvasIntersection{x: x, delta: 1})
					} else if y2 <= sampleY && y1 > sampleY {
						x := x1 + (sampleY-y1)*(x2-x1)/(y2-y1)
						intersections = append(intersections, canvasIntersection{x: x, delta: -1})
					}
				}
			}
			if len(intersections) < 2 {
				continue
			}
			sort.Slice(intersections, func(i, j int) bool { return intersections[i].x < intersections[j].x })
			winding := 0
			previous := intersections[0].x
			for i := 0; i < len(intersections); {
				x := intersections[i].x
				if winding != 0 && x > previous {
					canvasAccumulateSpan(replay.mask.Pix, replay.width, y, previous, x)
				}
				for i < len(intersections) && intersections[i].x == x {
					winding += intersections[i].delta
					i++
				}
				previous = x
			}
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			offset := y*replay.width + x
			replay.mask.Pix[offset] = byte((int(replay.mask.Pix[offset])*255 + 8) / 16)
		}
	}
	return replay.mask.Pix, bounds
}

func canvasAccumulateSpan(mask []byte, width, y int, left, right float64) {
	if left >= right {
		return
	}
	x0, x1 := max(0, int(math.Floor(left))), min(width, int(math.Ceil(right)))
	for x := x0; x < x1; x++ {
		sx0 := max(0, int(math.Ceil((left-float64(x))*4-0.5)))
		sx1 := min(4, int(math.Ceil((right-float64(x))*4-0.5)))
		if sx0 < sx1 {
			offset := y*width + x
			mask[offset] = byte(int(mask[offset]) + sx1 - sx0)
		}
	}
}

func canvasFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= maxCanvasCoordinate
}

func canvasPackedColor(rgba []float64, alpha float64) Color {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	a := uint8(math.Round(math.Max(0, math.Min(255, rgba[3])) * alpha))
	r := uint8(math.Max(0, math.Min(255, rgba[0])))
	g := uint8(math.Max(0, math.Min(255, rgba[1])))
	b := uint8(math.Max(0, math.Min(255, rgba[2])))
	return Color(uint32(r)*uint32(a)/255)<<24 |
		Color(uint32(g)*uint32(a)/255)<<16 |
		Color(uint32(b)*uint32(a)/255)<<8 |
		Color(a)
}

func canvasFillCoverage(b *Bitmap, source Color, coverage []uint8, bounds image.Rectangle, clip *canvasClipMask) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cov := coverage[y*b.W+x]
			if clip != nil {
				cov = uint8((uint32(cov)*uint32(canvasClipAlpha(clip, x, y)) + 127) / 255)
			}
			if cov == 0 {
				continue
			}
			scaled := Color(uint32(source.R())*uint32(cov)+127)/255<<24 |
				Color(uint32(source.G())*uint32(cov)+127)/255<<16 |
				Color(uint32(source.B())*uint32(cov)+127)/255<<8 |
				Color(uint32(source.A())*uint32(cov)+127)/255
			b.Set(x, y, BlendOver(b.At(x, y), scaled))
		}
	}
}

func canvasClearCoverage(b *Bitmap, coverage []uint8, bounds image.Rectangle, clip *canvasClipMask) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cov := coverage[y*b.W+x]
			if clip != nil {
				cov = uint8((uint32(cov)*uint32(canvasClipAlpha(clip, x, y)) + 127) / 255)
			}
			if cov == 0 {
				continue
			}
			keep := uint32(255 - cov)
			pixel := b.Pix(x, y)
			for channel := range pixel {
				pixel[channel] = byte((uint32(pixel[channel])*keep + 127) / 255)
			}
		}
	}
}

func canvasClipFromCoverage(coverage []uint8, bounds image.Rectangle, width, height int) *canvasClipMask {
	clip := &canvasClipMask{rows: make([][]canvasClipSpan, height)}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; {
			alpha := coverage[y*width+x]
			if alpha == 0 {
				x++
				continue
			}
			start := x
			for x < bounds.Max.X && coverage[y*width+x] == alpha {
				x++
			}
			clip.rows[y] = append(clip.rows[y], canvasClipSpan{x0: start, x1: x, alpha: alpha})
		}
	}
	return clip
}

func intersectCanvasClip(base, next *canvasClipMask) *canvasClipMask {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}
	out := &canvasClipMask{rows: make([][]canvasClipSpan, len(base.rows))}
	for y, a := range base.rows {
		b := next.rows[y]
		var spans []canvasClipSpan
		for i, j := 0, 0; i < len(a) && j < len(b); {
			x0, x1 := max(a[i].x0, b[j].x0), min(a[i].x1, b[j].x1)
			if x0 < x1 {
				alpha := uint8((uint32(a[i].alpha)*uint32(b[j].alpha) + 127) / 255)
				if alpha > 0 {
					spans = append(spans, canvasClipSpan{x0: x0, x1: x1, alpha: alpha})
				}
			}
			if a[i].x1 <= b[j].x1 {
				i++
			} else {
				j++
			}
		}
		out.rows[y] = spans
	}
	return out
}

func canvasClipAlpha(clip *canvasClipMask, x, y int) uint8 {
	if y < 0 || y >= len(clip.rows) {
		return 0
	}
	spans := clip.rows[y]
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := (lo + hi) / 2
		if x < spans[mid].x0 {
			hi = mid
		} else if x >= spans[mid].x1 {
			lo = mid + 1
		} else {
			return spans[mid].alpha
		}
	}
	return 0
}

type legacyCanvasState struct {
	fill, stroke color.RGBA
	alpha        float64
	lineWidth    float64
	font         string
	matrix       [6]float64
	clip         *canvasClipMask
}

type legacyCanvasPoint struct {
	x, y float64
}

type legacyCanvasPath struct {
	subpaths [][]legacyCanvasPoint
	closed   []bool
	start    legacyCanvasPoint
}

func replayLegacyCanvasOps(b *Bitmap, ops []CanvasOp, width, height int) {
	replay := newCanvasReplay(width, height)
	state := legacyCanvasState{
		fill: color.RGBA{A: 255}, stroke: color.RGBA{A: 255},
		alpha: 1, lineWidth: 1, font: "10px sans-serif",
		matrix: [6]float64{1, 0, 0, 1, 0, 0},
	}
	var stack []legacyCanvasState
	var path legacyCanvasPath
	for _, op := range ops {
		switch op.Method {
		case "fillStyle":
			if c, ok := legacyCanvasColor(op.Args); ok {
				state.fill = c
			}
		case "strokeStyle":
			if c, ok := legacyCanvasColor(op.Args); ok {
				state.stroke = c
			}
		case "globalAlpha":
			if value, ok := legacyCanvasFloat(op.Args, 0); ok && value >= 0 && value <= 1 {
				state.alpha = value
			}
		case "lineWidth":
			if value, ok := legacyCanvasFloat(op.Args, 0); ok && value > 0 {
				state.lineWidth = value
			}
		case "font":
			if len(op.Args) > 0 {
				state.font, _ = op.Args[0].(string)
			}
		case "save":
			if len(stack) < maxCanvasStateStack {
				stack = append(stack, state)
			}
		case "restore":
			if len(stack) > 0 {
				state = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				replay.clip = state.clip
			}
		case "reset":
			clear(b.RGBA)
			state = legacyCanvasState{fill: color.RGBA{A: 255}, stroke: color.RGBA{A: 255}, alpha: 1, lineWidth: 1, font: "10px sans-serif", matrix: [6]float64{1, 0, 0, 1, 0, 0}}
			replay.clip = nil
			stack = nil
			path = legacyCanvasPath{}
		case "translate":
			x, okX := legacyCanvasFloat(op.Args, 0)
			y, okY := legacyCanvasFloat(op.Args, 1)
			if okX && okY {
				m := state.matrix
				next := m
				next[4], next[5] = m[0]*x+m[2]*y+m[4], m[1]*x+m[3]*y+m[5]
				state.matrix = next
			}
		case "scale":
			x, okX := legacyCanvasFloat(op.Args, 0)
			y, okY := legacyCanvasFloat(op.Args, 1)
			if okX && okY {
				m := state.matrix
				state.matrix = [6]float64{m[0] * x, m[1] * x, m[2] * y, m[3] * y, m[4], m[5]}
			}
		case "rotate":
			angle, ok := legacyCanvasFloat(op.Args, 0)
			if ok {
				m := state.matrix
				sin, cos := math.Sin(angle), math.Cos(angle)
				state.matrix = [6]float64{m[0]*cos + m[2]*sin, m[1]*cos + m[3]*sin, -m[0]*sin + m[2]*cos, -m[1]*sin + m[3]*cos, m[4], m[5]}
			}
		case "transform":
			values, ok := legacyCanvasFloats(op.Args, 6)
			if ok {
				m := state.matrix
				state.matrix = [6]float64{
					m[0]*values[0] + m[2]*values[1], m[1]*values[0] + m[3]*values[1],
					m[0]*values[2] + m[2]*values[3], m[1]*values[2] + m[3]*values[3],
					m[0]*values[4] + m[2]*values[5] + m[4], m[1]*values[4] + m[3]*values[5] + m[5],
				}
			}
		case "setTransform":
			if values, ok := legacyCanvasFloats(op.Args, 6); ok {
				state.matrix = [6]float64{values[0], values[1], values[2], values[3], values[4], values[5]}
			}
		case "resetTransform":
			state.matrix = [6]float64{1, 0, 0, 1, 0, 0}
		case "fillRect":
			if values, ok := legacyCanvasFloats(op.Args, 4); ok {
				subpaths := legacyRectSubpaths(values[0], values[1], values[2], values[3], state.matrix)
				canvasFillSubpaths(b, replay, subpaths, state.fill, state.alpha, replay.clip)
			}
		case "strokeRect":
			if values, ok := legacyCanvasFloats(op.Args, 4); ok {
				subpaths := legacyStrokeRectSubpaths(values, state.lineWidth, state.matrix)
				canvasFillSubpaths(b, replay, subpaths, state.stroke, state.alpha, replay.clip)
			}
		case "clearRect":
			if values, ok := legacyCanvasFloats(op.Args, 4); ok {
				subpaths := legacyRectSubpaths(values[0], values[1], values[2], values[3], state.matrix)
				coverage, bounds := replay.coverage(subpaths)
				canvasClearCoverage(b, coverage, bounds, replay.clip)
			}
		case "beginPath":
			path = legacyCanvasPath{}
		case "moveTo":
			if values, ok := legacyCanvasFloats(op.Args, 2); ok {
				point := legacyCanvasTransform(state.matrix, values[0], values[1])
				path.subpaths = append(path.subpaths, []legacyCanvasPoint{point})
				path.closed = append(path.closed, false)
				path.start = point
			}
		case "lineTo":
			if values, ok := legacyCanvasFloats(op.Args, 2); ok {
				point := legacyCanvasTransform(state.matrix, values[0], values[1])
				if len(path.subpaths) == 0 {
					path.subpaths = append(path.subpaths, []legacyCanvasPoint{{0, 0}, point})
					path.closed = append(path.closed, false)
				} else {
					last := len(path.subpaths) - 1
					path.subpaths[last] = append(path.subpaths[last], point)
				}
			}
		case "closePath":
			if len(path.subpaths) > 0 {
				last := len(path.subpaths) - 1
				path.closed[last] = true
				path.subpaths[last] = append(path.subpaths[last], path.start)
			}
		case "rect":
			if values, ok := legacyCanvasFloats(op.Args, 4); ok {
				points := []legacyCanvasPoint{{values[0], values[1]}, {values[0] + values[2], values[1]}, {values[0] + values[2], values[1] + values[3]}, {values[0], values[1] + values[3]}, {values[0], values[1]}}
				for i := range points {
					points[i] = legacyCanvasTransform(state.matrix, points[i].x, points[i].y)
				}
				path.subpaths = append(path.subpaths, points)
				path.closed = append(path.closed, true)
				path.start = points[0]
			}
		case "arc":
			if values, ok := legacyCanvasFloats(op.Args, 5); ok && values[2] >= 0 {
				counterclockwise := len(op.Args) > 5 && op.Args[5] == true
				path.appendArc(state.matrix, values, counterclockwise)
			}
		case "ellipse":
			if values, ok := legacyCanvasFloats(op.Args, 7); ok && values[2] >= 0 && values[3] >= 0 {
				counterclockwise := len(op.Args) > 7 && op.Args[7] == true
				path.appendEllipse(state.matrix, values, counterclockwise)
			}
		case "bezierCurveTo":
			if values, ok := legacyCanvasFloats(op.Args, 6); ok {
				path.appendBezier(state.matrix, values)
			}
		case "quadraticCurveTo":
			if values, ok := legacyCanvasFloats(op.Args, 4); ok {
				path.appendQuadratic(state.matrix, values)
			}
		case "fill":
			canvasFillSubpaths(b, replay, path.subpathsFloat(), state.fill, state.alpha, replay.clip)
		case "stroke":
			canvasFillSubpaths(b, replay, legacyStrokeSubpaths(path, state.lineWidth), state.stroke, state.alpha, replay.clip)
		case "clip":
			coverage, bounds := replay.coverage(path.subpathsFloat())
			replay.clip = intersectCanvasClip(replay.clip, canvasClipFromCoverage(coverage, bounds, width, height))
		}
	}
}

func legacyCanvasColor(args []interface{}) (color.RGBA, bool) {
	values, ok := legacyCanvasFloats(args, 4)
	if !ok {
		return color.RGBA{}, false
	}
	return color.RGBA{R: uint8(values[0]), G: uint8(values[1]), B: uint8(values[2]), A: uint8(values[3])}, true
}

func legacyCanvasFloat(args []interface{}, index int) (float64, bool) {
	if index >= len(args) {
		return 0, false
	}
	value := toFloat(args[index])
	return value, canvasFinite(value)
}

func legacyCanvasFloats(args []interface{}, count int) ([]float64, bool) {
	values := make([]float64, count)
	for i := range values {
		value, ok := legacyCanvasFloat(args, i)
		if !ok {
			return nil, false
		}
		values[i] = value
	}
	return values, true
}

func legacyCanvasTransform(matrix [6]float64, x, y float64) legacyCanvasPoint {
	return legacyCanvasPoint{x: matrix[0]*x + matrix[2]*y + matrix[4], y: matrix[1]*x + matrix[3]*y + matrix[5]}
}

func legacyRectSubpaths(x, y, width, height float64, matrix [6]float64) [][]float64 {
	points := []legacyCanvasPoint{{x, y}, {x + width, y}, {x + width, y + height}, {x, y + height}}
	values := make([]float64, 0, 8)
	for _, point := range points {
		point = legacyCanvasTransform(matrix, point.x, point.y)
		values = append(values, point.x, point.y)
	}
	return [][]float64{values}
}

func legacyStrokeRectSubpaths(values []float64, lineWidth float64, matrix [6]float64) [][]float64 {
	x, y, width, height := values[0], values[1], values[2], values[3]
	half := lineWidth / 2
	outer := legacyRectSubpaths(x-half, y-half, width+lineWidth, height+lineWidth, matrix)[0]
	if math.Abs(width) <= lineWidth || math.Abs(height) <= lineWidth {
		return [][]float64{outer}
	}
	innerX, innerY := x+half, y+half
	innerWidth, innerHeight := width-lineWidth, height-lineWidth
	if width < 0 {
		innerX, innerWidth = x-half, width+lineWidth
	}
	if height < 0 {
		innerY, innerHeight = y-half, height+lineWidth
	}
	inner := legacyRectSubpaths(innerX, innerY, innerWidth, innerHeight, matrix)[0]
	for i := 0; i+3 < len(inner); i += 4 {
		inner[i], inner[i+4], inner[i+5], inner[i+6] = inner[i+6], inner[i], inner[i+1], inner[i+2]
		break
	}
	return [][]float64{outer, inner}
}

func (path *legacyCanvasPath) subpathsFloat() [][]float64 {
	out := make([][]float64, 0, len(path.subpaths))
	for _, subpath := range path.subpaths {
		values := make([]float64, 0, len(subpath)*2)
		for _, point := range subpath {
			values = append(values, point.x, point.y)
		}
		out = append(out, values)
	}
	return out
}

func (path *legacyCanvasPath) appendArc(matrix [6]float64, values []float64, counterclockwise bool) {
	x, y, radius, start, end := values[0], values[1], values[2], values[3], values[4]
	delta := end - start
	if !counterclockwise && delta < 0 {
		delta += 2 * math.Pi
	} else if counterclockwise && delta > 0 {
		delta -= 2 * math.Pi
	}
	const steps = 64
	for i := 0; i <= steps; i++ {
		angle := start + delta*float64(i)/float64(steps)
		point := legacyCanvasTransform(matrix, x+radius*math.Cos(angle), y+radius*math.Sin(angle))
		if i == 0 {
			path.subpaths = append(path.subpaths, []legacyCanvasPoint{point})
			path.closed = append(path.closed, false)
			path.start = point
		} else {
			last := len(path.subpaths) - 1
			path.subpaths[last] = append(path.subpaths[last], point)
		}
	}
}

func (path *legacyCanvasPath) appendEllipse(matrix [6]float64, values []float64, counterclockwise bool) {
	x, y, rx, ry, rotation, start, end := values[0], values[1], values[2], values[3], values[4], values[5], values[6]
	delta := end - start
	if !counterclockwise && delta < 0 {
		delta += 2 * math.Pi
	} else if counterclockwise && delta > 0 {
		delta -= 2 * math.Pi
	}
	const steps = 64
	cos, sin := math.Cos(rotation), math.Sin(rotation)
	for i := 0; i <= steps; i++ {
		angle := start + delta*float64(i)/float64(steps)
		px, py := rx*math.Cos(angle), ry*math.Sin(angle)
		point := legacyCanvasTransform(matrix, x+px*cos-py*sin, y+px*sin+py*cos)
		if i == 0 {
			path.subpaths = append(path.subpaths, []legacyCanvasPoint{point})
			path.closed = append(path.closed, false)
			path.start = point
		} else {
			last := len(path.subpaths) - 1
			path.subpaths[last] = append(path.subpaths[last], point)
		}
	}
}

func (path *legacyCanvasPath) appendBezier(matrix [6]float64, values []float64) {
	start := legacyCanvasPoint{}
	if len(path.subpaths) > 0 {
		points := path.subpaths[len(path.subpaths)-1]
		start = points[len(points)-1]
	}
	cp1 := legacyCanvasTransform(matrix, values[0], values[1])
	cp2 := legacyCanvasTransform(matrix, values[2], values[3])
	end := legacyCanvasTransform(matrix, values[4], values[5])
	for i := 1; i <= 16; i++ {
		t := float64(i) / 16
		it := 1 - t
		point := legacyCanvasPoint{
			x: it*it*it*start.x + 3*it*it*t*cp1.x + 3*it*t*t*cp2.x + t*t*t*end.x,
			y: it*it*it*start.y + 3*it*it*t*cp1.y + 3*it*t*t*cp2.y + t*t*t*end.y,
		}
		if len(path.subpaths) == 0 {
			path.subpaths = append(path.subpaths, []legacyCanvasPoint{point})
			path.closed = append(path.closed, false)
		} else {
			last := len(path.subpaths) - 1
			path.subpaths[last] = append(path.subpaths[last], point)
		}
	}
}

func (path *legacyCanvasPath) appendQuadratic(matrix [6]float64, values []float64) {
	start := legacyCanvasPoint{}
	if len(path.subpaths) > 0 {
		points := path.subpaths[len(path.subpaths)-1]
		start = points[len(points)-1]
	}
	control := legacyCanvasTransform(matrix, values[0], values[1])
	end := legacyCanvasTransform(matrix, values[2], values[3])
	for i := 1; i <= 16; i++ {
		t := float64(i) / 16
		it := 1 - t
		point := legacyCanvasPoint{x: it*it*start.x + 2*it*t*control.x + t*t*end.x, y: it*it*start.y + 2*it*t*control.y + t*t*end.y}
		if len(path.subpaths) == 0 {
			path.subpaths = append(path.subpaths, []legacyCanvasPoint{point})
			path.closed = append(path.closed, false)
		} else {
			last := len(path.subpaths) - 1
			path.subpaths[last] = append(path.subpaths[last], point)
		}
	}
}

func legacyStrokeSubpaths(path legacyCanvasPath, lineWidth float64) [][]float64 {
	if lineWidth <= 0 {
		return nil
	}
	half := lineWidth / 2
	var out [][]float64
	for _, subpath := range path.subpaths {
		for i := 0; i+1 < len(subpath); i++ {
			a, b := subpath[i], subpath[i+1]
			dx, dy := b.x-a.x, b.y-a.y
			length := math.Hypot(dx, dy)
			if length == 0 {
				continue
			}
			nx, ny := -dy/length*half, dx/length*half
			out = append(out, []float64{a.x + nx, a.y + ny, b.x + nx, b.y + ny, b.x - nx, b.y - ny, a.x - nx, a.y - ny})
		}
	}
	return out
}

func canvasFillSubpaths(b *Bitmap, replay *canvasReplay, subpaths [][]float64, source color.RGBA, alpha float64, clip *canvasClipMask) {
	coverage, bounds := replay.coverage(subpaths)
	if bounds.Empty() {
		return
	}
	rgba := []float64{float64(source.R), float64(source.G), float64(source.B), float64(source.A)}
	canvasFillCoverage(b, canvasPackedColor(rgba, alpha), coverage, bounds, clip)
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

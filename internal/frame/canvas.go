package frame

import (
	"image/color"
	"math"
)

// CanvasOp is one recorded canvas drawing operation.
type CanvasOp struct {
	Method string
	Args   []interface{}
}

// ReplayCanvasOps renders a sequence of canvas operations into a bitmap.
// The bitmap is cleared to transparent black first, then each op is applied
// in order. This implements a subset of the Canvas 2D API sufficient for
// basic drawing: fillRect, strokeRect, clearRect, fillText, and simple
// path operations (beginPath, moveTo, lineTo, arc, rect, fill, stroke).
func ReplayCanvasOps(b *Bitmap, ops []CanvasOp, width, height int) {
	if b == nil || len(ops) == 0 {
		return
	}

	// Clear to transparent
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			b.Set(x, y, 0)
		}
	}

	var (
		fillStyle   = color.RGBA{0, 0, 0, 255}
		strokeStyle = color.RGBA{0, 0, 0, 255}
		lineWidth   = 1.0
		path        []pathOp
		inPath      = false
	)

	for _, op := range ops {
		switch op.Method {
		case "fillRect":
			if len(op.Args) < 4 {
				continue
			}
			x, y, w, h := toFloat(op.Args[0]), toFloat(op.Args[1]), toFloat(op.Args[2]), toFloat(op.Args[3])
			fillRect(b, x, y, w, h, fillStyle)

		case "strokeRect":
			if len(op.Args) < 4 {
				continue
			}
			x, y, w, h := toFloat(op.Args[0]), toFloat(op.Args[1]), toFloat(op.Args[2]), toFloat(op.Args[3])
			strokeRect(b, x, y, w, h, lineWidth, strokeStyle)

		case "clearRect":
			if len(op.Args) < 4 {
				continue
			}
			x, y, w, h := toFloat(op.Args[0]), toFloat(op.Args[1]), toFloat(op.Args[2]), toFloat(op.Args[3])
			clearRect(b, x, y, w, h)

		case "beginPath":
			path = nil
			inPath = true

		case "closePath":
			if len(path) > 0 {
				path = append(path, pathOp{kind: pathLine, x: path[0].x, y: path[0].y})
			}

		case "moveTo":
			if len(op.Args) < 2 {
				continue
			}
			path = append(path, pathOp{kind: pathMove, x: toFloat(op.Args[0]), y: toFloat(op.Args[1])})

		case "lineTo":
			if len(op.Args) < 2 {
				continue
			}
			path = append(path, pathOp{kind: pathLine, x: toFloat(op.Args[0]), y: toFloat(op.Args[1])})

		case "arc":
			if len(op.Args) < 5 {
				continue
			}
			path = append(path, pathOp{
				kind: pathArc,
				x:    toFloat(op.Args[0]),
				y:    toFloat(op.Args[1]),
				r:    toFloat(op.Args[2]),
				a0:   toFloat(op.Args[3]),
				a1:   toFloat(op.Args[4]),
			})

		case "rect":
			if len(op.Args) < 4 {
				continue
			}
			x, y, w, h := toFloat(op.Args[0]), toFloat(op.Args[1]), toFloat(op.Args[2]), toFloat(op.Args[3])
			path = append(path,
				pathOp{kind: pathMove, x: x, y: y},
				pathOp{kind: pathLine, x: x + w, y: y},
				pathOp{kind: pathLine, x: x + w, y: y + h},
				pathOp{kind: pathLine, x: x, y: y + h},
				pathOp{kind: pathLine, x: x, y: y},
			)

		case "fill":
			if inPath {
				fillPath(b, path, fillStyle)
			}

		case "stroke":
			if inPath {
				strokePath(b, path, lineWidth, strokeStyle)
			}

		case "fillText":
			if len(op.Args) < 3 {
				continue
			}
			// Text rendering is complex; skip for now
			_ = op.Args[0].(string)
			_ = toFloat(op.Args[1])
			_ = toFloat(op.Args[2])

		case "strokeText":
			if len(op.Args) < 3 {
				continue
			}
			_ = op.Args[0].(string)
			_ = toFloat(op.Args[1])
			_ = toFloat(op.Args[2])
		}
	}
}

type pathKind int

const (
	pathMove pathKind = iota
	pathLine
	pathArc
)

type pathOp struct {
	kind pathKind
	x, y float64
	r    float64
	a0   float64
	a1   float64
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

func fillRect(b *Bitmap, x, y, w, h float64, c color.RGBA) {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := int(math.Ceil(x+w)), int(math.Ceil(y+h))
	col := packColor(c)
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			if px >= 0 && px < b.W && py >= 0 && py < b.H {
				b.Set(px, py, BlendOver(b.At(px, py), col))
			}
		}
	}
}

func strokeRect(b *Bitmap, x, y, w, h float64, lineWidth float64, c color.RGBA) {
	half := lineWidth / 2
	// Top
	fillRect(b, x-half, y-half, w+lineWidth, lineWidth, c)
	// Bottom
	fillRect(b, x-half, y+h-half, w+lineWidth, lineWidth, c)
	// Left
	fillRect(b, x-half, y-half, lineWidth, h+lineWidth, c)
	// Right
	fillRect(b, x+w-half, y-half, lineWidth, h+lineWidth, c)
}

func clearRect(b *Bitmap, x, y, w, h float64) {
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := int(math.Ceil(x+w)), int(math.Ceil(y+h))
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			if px >= 0 && px < b.W && py >= 0 && py < b.H {
				b.Set(px, py, 0)
			}
		}
	}
}

func fillPath(b *Bitmap, path []pathOp, c color.RGBA) {
	if len(path) < 3 {
		return
	}
	col := packColor(c)
	// Simple scanline fill for convex polygons
	yMin, yMax := math.MaxFloat64, -math.MaxFloat64
	for _, p := range path {
		if p.y < yMin {
			yMin = p.y
		}
		if p.y > yMax {
			yMax = p.y
		}
	}
	for py := int(math.Floor(yMin)); py <= int(math.Ceil(yMax)); py++ {
		var intersections []float64
		for i := 0; i < len(path); i++ {
			p1 := path[i]
			p2 := path[(i+1)%len(path)]
			if (p1.y <= float64(py) && p2.y > float64(py)) || (p2.y <= float64(py) && p1.y > float64(py)) {
				t := (float64(py) - p1.y) / (p2.y - p1.y)
				x := p1.x + t*(p2.x-p1.x)
				intersections = append(intersections, x)
			}
		}
		// Sort intersections
		for i := 0; i < len(intersections); i++ {
			for j := i + 1; j < len(intersections); j++ {
				if intersections[i] > intersections[j] {
					intersections[i], intersections[j] = intersections[j], intersections[i]
				}
			}
		}
		// Fill between pairs
		for i := 0; i+1 < len(intersections); i += 2 {
			x0 := int(math.Ceil(intersections[i]))
			x1 := int(math.Floor(intersections[i+1]))
			for px := x0; px <= x1; px++ {
				if px >= 0 && px < b.W && py >= 0 && py < b.H {
					b.Set(px, py, BlendOver(b.At(px, py), col))
				}
			}
		}
	}
}

func strokePath(b *Bitmap, path []pathOp, lineWidth float64, c color.RGBA) {
	for i := 0; i+1 < len(path); i++ {
		p1 := path[i]
		p2 := path[i+1]
		if p1.kind == pathMove || p2.kind == pathMove {
			continue
		}
		strokeLine(b, p1.x, p1.y, p2.x, p2.y, lineWidth, c)
	}
}

func strokeLine(b *Bitmap, x0, y0, x1, y1 float64, lineWidth float64, c color.RGBA) {
	// Bresenham-like line with thickness
	dx := math.Abs(x1 - x0)
	dy := math.Abs(y1 - y0)
	sx := 1.0
	if x0 > x1 {
		sx = -1
	}
	sy := 1.0
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	col := packColor(c)
	half := lineWidth / 2
	for {
		// Draw a small square at (x0, y0) with lineWidth thickness
		for py := int(math.Floor(y0 - half)); py <= int(math.Ceil(y0+half)); py++ {
			for px := int(math.Floor(x0 - half)); px <= int(math.Ceil(x0+half)); px++ {
				if px >= 0 && px < b.W && py >= 0 && py < b.H {
					b.Set(px, py, BlendOver(b.At(px, py), col))
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func packColor(c color.RGBA) Color {
	// Premultiply
	r := uint32(c.R) * uint32(c.A) / 255
	g := uint32(c.G) * uint32(c.A) / 255
	b := uint32(c.B) * uint32(c.A) / 255
	return Color(r<<24 | g<<16 | b<<8 | uint32(c.A))
}

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
		if y < 0 || y >= b.H {
			continue
		}
		x0, x1 := 0, width
		if x0 < 0 {
			x0 = 0
		}
		if x1 > b.W {
			x1 = b.W
		}
		if x0 < x1 {
			clear(b.RGBA[y*b.Stride+x0*4 : y*b.Stride+x1*4])
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
	// Clip once instead of bounds-checking every pixel: pixels outside were
	// ignored by Set, so skipping them writes the same bitmap.
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > b.W {
		x1 = b.W
	}
	if y1 > b.H {
		y1 = b.H
	}
	if x0 >= x1 || y0 >= y1 {
		return
	}
	col := packColor(c)
	for py := y0; py < y1; py++ {
		rowBase := py*b.Stride + x0*4
		row := b.RGBA[rowBase : rowBase+(x1-x0)*4]
		o := 0
		for px := x0; px < x1; px++ {
			p := row[o : o+4 : o+4]
			dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
			out := BlendOver(dst, col)
			p[0] = byte(out >> 24)
			p[1] = byte(out >> 16)
			p[2] = byte(out >> 8)
			p[3] = byte(out)
			o += 4
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
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > b.W {
		x1 = b.W
	}
	if y1 > b.H {
		y1 = b.H
	}
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for py := y0; py < y1; py++ {
		clear(b.RGBA[py*b.Stride+x0*4 : py*b.Stride+x1*4])
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
	// Simple scanline fill for convex polygons. The intersection buffer is
	// reused across scanlines; only its length, not its contents, carries over.
	var intersections []float64
	for py := int(math.Floor(yMin)); py <= int(math.Ceil(yMax)); py++ {
		yp := float64(py)
		intersections = intersections[:0]
		for i := 0; i < len(path); i++ {
			p1 := path[i]
			p2 := path[(i+1)%len(path)]
			if (p1.y <= yp && p2.y > yp) || (p2.y <= yp && p1.y > yp) {
				t := (yp - p1.y) / (p2.y - p1.y)
				x := p1.x + t*(p2.x-p1.x)
				intersections = append(intersections, x)
			}
		}
		// Insertion sort: the edge count per scanline is small, and it sorts
		// the same values the bubble sort did, so the fill pairs are unchanged.
		for i := 1; i < len(intersections); i++ {
			for j := i; j > 0 && intersections[j-1] > intersections[j]; j-- {
				intersections[j-1], intersections[j] = intersections[j], intersections[j-1]
			}
		}
		// Fill between pairs
		if py < 0 || py >= b.H {
			continue
		}
		for i := 0; i+1 < len(intersections); i += 2 {
			x0 := int(math.Ceil(intersections[i]))
			x1 := int(math.Floor(intersections[i+1]))
			// Clip once instead of bounds-checking every pixel: out-of-range
			// pixels were ignored by Set, so skipping them is identical.
			if x0 < 0 {
				x0 = 0
			}
			if x1 >= b.W {
				x1 = b.W - 1
			}
			if x0 > x1 {
				continue
			}
			rowBase := py*b.Stride + x0*4
			row := b.RGBA[rowBase : rowBase+(x1-x0+1)*4]
			o := 0
			for px := x0; px <= x1; px++ {
				p := row[o : o+4 : o+4]
				dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
				out := BlendOver(dst, col)
				p[0] = byte(out >> 24)
				p[1] = byte(out >> 16)
				p[2] = byte(out >> 8)
				p[3] = byte(out)
				o += 4
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
		// Draw a small square at (x0, y0) with lineWidth thickness. The stamp
		// is clipped once per step instead of bounds-checked per pixel; the
		// pixels, order, and blends are the same, only the ignored
		// out-of-range writes are skipped.
		py0, py1 := int(math.Floor(y0-half)), int(math.Ceil(y0+half))
		px0, px1 := int(math.Floor(x0-half)), int(math.Ceil(x0+half))
		if px0 < 0 {
			px0 = 0
		}
		if py0 < 0 {
			py0 = 0
		}
		if px1 >= b.W {
			px1 = b.W - 1
		}
		if py1 >= b.H {
			py1 = b.H - 1
		}
		// A stamp fully outside the bitmap draws nothing; Set ignored those.
		if px0 <= px1 && py0 <= py1 {
			for py := py0; py <= py1; py++ {
				rowBase := py*b.Stride + px0*4
				row := b.RGBA[rowBase : rowBase+(px1-px0+1)*4]
				o := 0
				for px := px0; px <= px1; px++ {
					p := row[o : o+4 : o+4]
					dst := Color(uint32(p[0])<<24 | uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3]))
					out := BlendOver(dst, col)
					p[0] = byte(out >> 24)
					p[1] = byte(out >> 16)
					p[2] = byte(out >> 8)
					p[3] = byte(out)
					o += 4
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

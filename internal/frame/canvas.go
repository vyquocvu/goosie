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

		case "bezierCurveTo":
			if len(op.Args) < 6 {
				continue
			}
			cp1x, cp1y := toFloat(op.Args[0]), toFloat(op.Args[1])
			cp2x, cp2y := toFloat(op.Args[2]), toFloat(op.Args[3])
			x, y := toFloat(op.Args[4]), toFloat(op.Args[5])
			x0, y0 := 0.0, 0.0
			if len(path) > 0 {
				x0, y0 = path[len(path)-1].x, path[len(path)-1].y
			}
			steps := 16
			for s := 1; s <= steps; s++ {
				t := float64(s) / float64(steps)
				it := 1.0 - t
				bx := it*it*it*x0 + 3*it*it*t*cp1x + 3*it*t*t*cp2x + t*t*t*x
				by := it*it*it*y0 + 3*it*it*t*cp1y + 3*it*t*t*cp2y + t*t*t*y
				path = append(path, pathOp{kind: pathLine, x: bx, y: by})
			}

		case "quadraticCurveTo":
			if len(op.Args) < 4 {
				continue
			}
			cpx, cpy := toFloat(op.Args[0]), toFloat(op.Args[1])
			x, y := toFloat(op.Args[2]), toFloat(op.Args[3])
			x0, y0 := 0.0, 0.0
			if len(path) > 0 {
				x0, y0 = path[len(path)-1].x, path[len(path)-1].y
			}
			steps := 16
			for s := 1; s <= steps; s++ {
				t := float64(s) / float64(steps)
				it := 1.0 - t
				bx := it*it*x0 + 2*it*t*cpx + t*t*x
				by := it*it*y0 + 2*it*t*cpy + t*t*y
				path = append(path, pathOp{kind: pathLine, x: bx, y: by})
			}

		case "ellipse":
			if len(op.Args) < 7 {
				continue
			}
			cx, cy := toFloat(op.Args[0]), toFloat(op.Args[1])
			rx, ry := toFloat(op.Args[2]), toFloat(op.Args[3])
			rot := toFloat(op.Args[4])
			a0, a1 := toFloat(op.Args[5]), toFloat(op.Args[6])
			ccw := false
			if len(op.Args) >= 8 {
				if bVal, ok := op.Args[7].(bool); ok {
					ccw = bVal
				}
			}
			cosRot, sinRot := math.Cos(rot), math.Sin(rot)
			delta := a1 - a0
			if !ccw && delta < 0 {
				delta += 2 * math.Pi
			} else if ccw && delta > 0 {
				delta -= 2 * math.Pi
			}
			steps := 32
			for s := 0; s <= steps; s++ {
				t := a0 + delta*float64(s)/float64(steps)
				px := rx * math.Cos(t)
				py := ry * math.Sin(t)
				ex := cx + px*cosRot - py*sinRot
				ey := cy + px*sinRot + py*cosRot
				if s == 0 && len(path) == 0 {
					path = append(path, pathOp{kind: pathMove, x: ex, y: ey})
				} else {
					path = append(path, pathOp{kind: pathLine, x: ex, y: ey})
				}
			}

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
	ix0, iy0 := int(math.Round(x0)), int(math.Round(y0))
	ix1, iy1 := int(math.Round(x1)), int(math.Round(y1))
	dx := math.Abs(float64(ix1 - ix0))
	dy := math.Abs(float64(iy1 - iy0))
	sx := 1
	if ix0 > ix1 {
		sx = -1
	}
	sy := 1
	if iy0 > iy1 {
		sy = -1
	}
	err := dx - dy
	col := packColor(c)
	half := int(math.Ceil(lineWidth / 2))
	if half < 1 {
		half = 1
	}
	for {
		py0, py1 := iy0-half, iy0+half
		px0, px1 := ix0-half, ix0+half
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
		if ix0 == ix1 && iy0 == iy1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			ix0 += sx
		}
		if e2 < dx {
			err += dx
			iy0 += sy
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

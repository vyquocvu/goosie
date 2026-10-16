package image

import (
	"math"
	"strconv"
)

// Static-SVG <path> support: the workhorse element of real-world icons and
// logos (the subset previously skipped <path> entirely, decoding every
// path-based SVG to a blank tile).
//
// What this covers: M/m L/l H/h V/v Z/z C/c S/s Q/q T/t A/a with implicit
// command repetition, per the SVG path microsyntax. Curves flatten to line
// segments (fixed subdivision, exact endpoints); arcs use endpoint
// parametrization. Fill uses the polygon even-odd rule below rather than
// nonzero: identical except on self-intersecting windings, which icons avoid.
// An unparseable segment ends the path instead of failing the document.

// pathSubpath is one moveto-delimited run of points plus whether a closepath
// joined its end back to its start.
type pathSubpath struct {
	pts    [][2]float64
	closed bool
}

func svgPath(d string) []pathSubpath {
	p := &pathParser{s: d}
	var out []pathSubpath
	var cur [2]float64
	var start [2]float64
	var pts [][2]float64
	haveSub := false
	// last cubic/quadratic control points for smooth commands.
	var lastC, lastQ [2]float64
	haveC, haveQ := false, false
	flush := func(closed bool) {
		if haveSub && len(pts) > 0 {
			out = append(out, pathSubpath{pts: pts, closed: closed})
		}
		pts = nil
		haveSub = false
	}
	cmd := byte(0)
	for {
		p.skipSep()
		if p.eof() {
			break
		}
		c := p.s[p.i]
		if isPathCmd(c) {
			cmd = c
			p.i++
			if cmd == 'Z' || cmd == 'z' {
				if haveSub {
					pts = append(pts, start)
				}
				flush(true)
				cur = start
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
			flush(false)
			rel := cmd == 'm'
			cur = [2]float64{x, y}
			start = cur
			pts = append(pts, cur)
			haveSub = true
			// Implicit linetos after the first pair, keeping the case.
			cmd = 'L'
			if rel {
				cmd = 'l'
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
			cur = [2]float64{x, y}
			pts = append(pts, cur)
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
			pts = append(pts, cur)
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
			pts = append(pts, cur)
			haveSub = true
			haveC, haveQ = false, false
		case 'C', 'c':
			var v [6]float64
			ok := true
			for k := 0; k < 6; k++ {
				v[k], ok = p.number()
				if !ok {
					return out
				}
			}
			var p0, p1, p2, p3 [2]float64
			p0 = cur
			if cmd == 'c' {
				p1 = [2]float64{cur[0] + v[0], cur[1] + v[1]}
				p2 = [2]float64{cur[0] + v[2], cur[1] + v[3]}
				p3 = [2]float64{cur[0] + v[4], cur[1] + v[5]}
			} else {
				p1 = [2]float64{v[0], v[1]}
				p2 = [2]float64{v[2], v[3]}
				p3 = [2]float64{v[4], v[5]}
			}
			for _, q := range flattenCubic(p0, p1, p2, p3) {
				pts = append(pts, q)
			}
			lastC = p2
			haveC = true
			haveQ = false
			cur = p3
			haveSub = true
		case 'S', 's':
			var v [4]float64
			ok := true
			for k := 0; k < 4; k++ {
				v[k], ok = p.number()
				if !ok {
					return out
				}
			}
			p0 := cur
			var p1 [2]float64
			if haveC {
				p1 = [2]float64{2*cur[0] - lastC[0], 2*cur[1] - lastC[1]}
			} else {
				p1 = cur
			}
			var p2, p3 [2]float64
			if cmd == 's' {
				p2 = [2]float64{cur[0] + v[0], cur[1] + v[1]}
				p3 = [2]float64{cur[0] + v[2], cur[1] + v[3]}
			} else {
				p2 = [2]float64{v[0], v[1]}
				p3 = [2]float64{v[2], v[3]}
			}
			for _, q := range flattenCubic(p0, p1, p2, p3) {
				pts = append(pts, q)
			}
			lastC = p2
			haveC = true
			haveQ = false
			cur = p3
			haveSub = true
		case 'Q', 'q':
			var v [4]float64
			ok := true
			for k := 0; k < 4; k++ {
				v[k], ok = p.number()
				if !ok {
					return out
				}
			}
			p0 := cur
			var p1, p2 [2]float64
			if cmd == 'q' {
				p1 = [2]float64{cur[0] + v[0], cur[1] + v[1]}
				p2 = [2]float64{cur[0] + v[2], cur[1] + v[3]}
			} else {
				p1 = [2]float64{v[0], v[1]}
				p2 = [2]float64{v[2], v[3]}
			}
			for _, q := range flattenQuad(p0, p1, p2) {
				pts = append(pts, q)
			}
			lastQ = p1
			haveQ = true
			haveC = false
			cur = p2
			haveSub = true
		case 'T', 't':
			x, ok1 := p.number()
			y, ok2 := p.number()
			if !ok1 || !ok2 {
				return out
			}
			p0 := cur
			var p1 [2]float64
			if haveQ {
				p1 = [2]float64{2*cur[0] - lastQ[0], 2*cur[1] - lastQ[1]}
			} else {
				p1 = cur
			}
			var p2 [2]float64
			if cmd == 't' {
				p2 = [2]float64{cur[0] + x, cur[1] + y}
			} else {
				p2 = [2]float64{x, y}
			}
			for _, q := range flattenQuad(p0, p1, p2) {
				pts = append(pts, q)
			}
			lastQ = p1
			haveQ = true
			haveC = false
			cur = p2
			haveSub = true
		case 'A', 'a':
			rx, ok1 := p.number()
			ry, ok2 := p.number()
			rot, ok3 := p.number()
			laf, ok4 := p.number()
			sf, ok5 := p.number()
			x, ok6 := p.number()
			y, ok7 := p.number()
			if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
				return out
			}
			var p1 [2]float64
			if cmd == 'a' {
				p1 = [2]float64{cur[0] + x, cur[1] + y}
			} else {
				p1 = [2]float64{x, y}
			}
			for _, q := range flattenArc(cur, rx, ry, rot, laf != 0, sf != 0, p1) {
				pts = append(pts, q)
			}
			haveC, haveQ = false, false
			cur = p1
			haveSub = true
		default:
			return out
		}
	}
	flush(false)
	return out
}

func isPathCmd(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's',
		'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

// pathParser scans path data: separators are whitespace and commas, and a
// sign or decimal point can start a new number without any separator
// ("M1-2.5.3" is three numbers after the command).
type pathParser struct {
	s string
	i int
}

func (p *pathParser) eof() bool { return p.i >= len(p.s) }

func (p *pathParser) skipSep() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == '\f' {
			p.i++
			continue
		}
		break
	}
}

func (p *pathParser) number() (float64, bool) {
	p.skipSep()
	if p.i >= len(p.s) {
		return 0, false
	}
	start := p.i
	if p.s[p.i] == '-' || p.s[p.i] == '+' {
		p.i++
	}
	digits := false
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
		digits = true
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		// A second dot starts the next number ("1.5.25" is 1.5 then .25).
		p.i++
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			digits = true
		}
	}
	if !digits {
		return 0, false
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		j := p.i + 1
		if j < len(p.s) && (p.s[j] == '-' || p.s[j] == '+') {
			j++
		}
		k := j
		for k < len(p.s) && p.s[k] >= '0' && p.s[k] <= '9' {
			k++
		}
		if k > j {
			p.i = k
		}
	}
	v, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// flattenCubic subdivides a cubic Bézier into line segments, endpoints exact.
func flattenCubic(p0, p1, p2, p3 [2]float64) [][2]float64 {
	const n = 12
	out := make([][2]float64, 0, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / n
		mt := 1 - t
		out = append(out, [2]float64{
			mt*mt*mt*p0[0] + 3*mt*mt*t*p1[0] + 3*mt*t*t*p2[0] + t*t*t*p3[0],
			mt*mt*mt*p0[1] + 3*mt*mt*t*p1[1] + 3*mt*t*t*p2[1] + t*t*t*p3[1],
		})
	}
	return out
}

// flattenQuad subdivides a quadratic Bézier into line segments.
func flattenQuad(p0, p1, p2 [2]float64) [][2]float64 {
	const n = 10
	out := make([][2]float64, 0, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / n
		mt := 1 - t
		out = append(out, [2]float64{
			mt*mt*p0[0] + 2*mt*t*p1[0] + t*t*p2[0],
			mt*mt*p0[1] + 2*mt*t*p1[1] + t*t*p2[1],
		})
	}
	return out
}

// flattenArc converts an endpoint-parameterized elliptical arc to line
// segments via center parametrization (SVG 1.1 §8.5, F.6.5 correction).
func flattenArc(p0 [2]float64, rx, ry, rot float64, large, sweep bool, p1 [2]float64) [][2]float64 {
	rx = math.Abs(rx)
	ry = math.Abs(ry)
	if rx == 0 || ry == 0 {
		return [][2]float64{p1}
	}
	phi := rot * math.Pi / 180
	dx := (p0[0] - p1[0]) / 2
	dy := (p0[1] - p1[1]) / 2
	x1p := math.Cos(phi)*dx + math.Sin(phi)*dy
	y1p := -math.Sin(phi)*dx + math.Cos(phi)*dy
	lam := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry)
	if lam > 1 {
		s := math.Sqrt(lam)
		rx *= s
		ry *= s
	}
	num := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	fac := 0.0
	if num > 0 {
		fac = math.Sqrt(math.Max(0, (rx*rx*ry*ry-num)/num))
	}
	if large == sweep {
		fac = -fac
	}
	cxp := fac * rx * y1p / ry
	cyp := fac * -ry * x1p / rx
	cx := math.Cos(phi)*cxp - math.Sin(phi)*cyp + (p0[0]+p1[0])/2
	cy := math.Sin(phi)*cxp + math.Cos(phi)*cyp + (p0[1]+p1[1])/2
	th := func(px, py float64) float64 {
		return math.Atan2((py-cyp)/ry, (px-cxp)/rx)
	}
	t1 := th(p0[0], p0[1])
	dth := th(p1[0], p1[1]) - t1
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
	out := make([][2]float64, 0, steps)
	for i := 1; i <= steps; i++ {
		a := t1 + dth*float64(i)/float64(steps)
		out = append(out, [2]float64{
			cx + rx*math.Cos(a)*math.Cos(phi) - ry*math.Sin(a)*math.Sin(phi),
			cy + rx*math.Cos(a)*math.Sin(phi) + ry*math.Sin(a)*math.Cos(phi),
		})
	}
	return out
}

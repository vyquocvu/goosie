package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

type GradientStop struct {
	At    float32
	Px    float32
	PxSet bool
	Color css.Color
}

type Gradient struct {
	Angle float32
	Stops []GradientStop
}

type RadialGradient struct {
	Shape   string
	Size    string
	Cx      float32
	Cy      float32
	CxPx    float32
	CyPx    float32
	CxPxSet bool
	CyPxSet bool
	Stops   []GradientStop
}

func (g Gradient) Empty() bool { return len(g.Stops) < 2 }

func (g RadialGradient) Empty() bool { return len(g.Stops) < 2 }

func parseBackground(v string) (css.Color, Gradient, RadialGradient) {
	return parseBackgroundWithScheme(v, "")
}

func parseBackgroundWithScheme(v, scheme string) (css.Color, Gradient, RadialGradient) {
	g := parseGradient(v)
	rg := parseRadialGradient(v)
	return parseBackgroundColorWithScheme(stripFunctions(v), scheme), g, rg
}

func parseGradient(v string) Gradient {
	lower := strings.ToLower(v)
	i := strings.Index(lower, "linear-gradient(")
	if i < 0 {
		return Gradient{}
	}
	args, ok := callArgs(v[i+len("linear-gradient("):])
	if !ok {
		return Gradient{}
	}
	return parseLinearGradientArgs(args)
}

func parseRadialGradient(v string) RadialGradient {
	lower := strings.ToLower(v)
	i := strings.Index(lower, "radial-gradient(")
	if i < 0 {
		return RadialGradient{}
	}
	args, ok := callArgs(v[i+len("radial-gradient("):])
	if !ok {
		return RadialGradient{}
	}
	return parseRadialGradientArgs(args)
}

func parseLinearGradientArgs(args string) Gradient {
	parts := splitArgs(args)
	if len(parts) == 0 {
		return Gradient{}
	}
	g := Gradient{Angle: 180}
	if len(parts) > 1 {
		if angle, ok := parseGradientAngle(parts[0]); ok {
			g.Angle = angle
			parts = parts[1:]
		}
	}
	stops := make([]GradientStop, 0, len(parts))
	for _, p := range parts {
		ss, ok := parseGradientStops(p)
		if !ok {
			return Gradient{}
		}
		stops = append(stops, ss...)
	}
	g.Stops = distributeStops(stops)
	return g
}

func parseRadialGradientArgs(args string) RadialGradient {
	parts := splitArgs(args)
	if len(parts) == 0 {
		return RadialGradient{}
	}
	g := RadialGradient{
		Shape: "ellipse",
		Size:  "farthest-corner",
		Cx:    0.5,
		Cy:    0.5,
	}
	header := strings.Fields(strings.ToLower(strings.TrimSpace(parts[0])))
	if len(header) == 0 || !isRadialHeaderToken(header[0]) {
		return radialWithStops(g, parts)
	}
	shape, size := "", ""
	i := 0
	for i < len(header) {
		tok := header[i]
		switch {
		case tok == "circle" || tok == "ellipse":
			if shape != "" {
				return RadialGradient{}
			}
			shape = tok
			i++
		case tok == "closest-side" || tok == "farthest-side" || tok == "closest-corner" || tok == "farthest-corner":
			if size != "" {
				return RadialGradient{}
			}
			size = tok
			i++
		case tok == "at":
			n := radialPosition(header[i+1:], &g)
			if n < 0 {
				return RadialGradient{}
			}
			i += 1 + n
			if i < len(header) {
				return RadialGradient{}
			}
		default:
			return RadialGradient{}
		}
	}
	if shape != "" {
		g.Shape = shape
	}
	if size != "" {
		g.Size = size
	}
	return radialWithStops(g, parts[1:])
}

func isRadialHeaderToken(tok string) bool {
	switch tok {
	case "circle", "ellipse", "at",
		"closest-side", "farthest-side", "closest-corner", "farthest-corner":
		return true
	}
	return false
}

func radialPosition(toks []string, g *RadialGradient) int {
	if len(toks) == 0 {
		return -1
	}
	c1, ok := radialPosComp(toks[0])
	if !ok {
		return -1
	}
	if len(toks) == 1 {
		axis := 0
		if c1.vertical {
			axis = 1
		}
		applyRadialComp(g, c1, axis)
		return 1
	}
	c2, ok := radialPosComp(toks[1])
	if !ok {
		return -1
	}
	ax1, ax2 := -1, -1
	switch {
	case c1.vertical:
		ax1 = 1
	case c1.horizontal:
		ax1 = 0
	}
	switch {
	case c2.vertical:
		ax2 = 1
	case c2.horizontal:
		ax2 = 0
	}
	if ax1 == -1 && ax2 == -1 {
		ax1, ax2 = 0, 1
	} else if ax1 == -1 {
		ax1 = 1 - ax2
	} else if ax2 == -1 {
		ax2 = 1 - ax1
	}
	if ax1 == ax2 {
		return -1
	}
	applyRadialComp(g, c1, ax1)
	applyRadialComp(g, c2, ax2)
	return 2
}

type radialComp struct {
	vertical   bool
	horizontal bool
	val        float32
	isPx       bool
}

func radialPosComp(tok string) (radialComp, bool) {
	switch tok {
	case "left":
		return radialComp{horizontal: true}, true
	case "right":
		return radialComp{horizontal: true, val: 1}, true
	case "top":
		return radialComp{vertical: true}, true
	case "bottom":
		return radialComp{vertical: true, val: 1}, true
	case "center":
		return radialComp{val: 0.5}, true
	}
	if strings.HasSuffix(tok, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(tok, "%"), 64)
		if err != nil {
			return radialComp{}, false
		}
		return radialComp{val: float32(n / 100)}, true
	}
	if strings.HasSuffix(tok, "px") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(tok, "px"), 64)
		if err != nil {
			return radialComp{}, false
		}
		return radialComp{val: float32(n), isPx: true}, true
	}
	if n, err := strconv.ParseFloat(tok, 64); err == nil && n == 0 {
		return radialComp{isPx: true}, true
	}
	return radialComp{}, false
}

func applyRadialComp(g *RadialGradient, c radialComp, axis int) {
	if c.isPx {
		if axis == 1 {
			g.CyPx, g.CyPxSet = c.val, true
		} else {
			g.CxPx, g.CxPxSet = c.val, true
		}
		return
	}
	if axis == 1 {
		g.Cy = c.val
	} else {
		g.Cx = c.val
	}
}

func radialWithStops(g RadialGradient, parts []string) RadialGradient {
	stops := make([]GradientStop, 0, len(parts))
	for _, p := range parts {
		ss, ok := parseGradientStops(p)
		if !ok {
			return RadialGradient{}
		}
		stops = append(stops, ss...)
	}
	g.Stops = distributeStops(stops)
	return g
}

func parseGradientAngle(s string) (float32, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "to ") {
		return gradientKeywordAngle(s)
	}
	for _, suffix := range []struct {
		name   string
		factor float64
	}{{"deg", 1}, {"turn", 360}, {"grad", 0.9}, {"rad", 180 / math.Pi}} {
		if strings.HasSuffix(s, suffix.name) {
			n, err := strconv.ParseFloat(strings.TrimSpace(s[:len(s)-len(suffix.name)]), 64)
			if err != nil {
				return 0, false
			}
			return float32(math.Mod(n*suffix.factor, 360)), true
		}
	}
	return 0, false
}

func gradientKeywordAngle(s string) (float32, bool) {
	fields := strings.Fields(s)
	if len(fields) != 2 && len(fields) != 3 {
		return 0, false
	}
	vertical, horizontal := "", ""
	for _, f := range fields[1:] {
		switch f {
		case "top", "bottom":
			if vertical != "" {
				return 0, false
			}
			vertical = f
		case "left", "right":
			if horizontal != "" {
				return 0, false
			}
			horizontal = f
		default:
			return 0, false
		}
	}
	switch vertical + " " + horizontal {
	case "top ", "bottom ":
		if vertical == "top" {
			return 0, true
		}
		return 180, true
	case " right", " left":
		if horizontal == "right" {
			return 90, true
		}
		return 270, true
	}
	const diag = float32(45)
	switch vertical + " " + horizontal {
	case "top right":
		return 90 - diag, true
	case "top left":
		return 270 + diag, true
	case "bottom right":
		return 90 + diag, true
	case "bottom left":
		return 270 - diag, true
	}
	return 0, false
}

type stopPos struct {
	at   float32
	px   float32
	isPx bool
}

func stopPosition(tok string) (p stopPos, is, legal bool) {
	if strings.HasSuffix(tok, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(tok, "%"), 64)
		if err != nil {
			return stopPos{}, false, false
		}
		return stopPos{at: float32(n / 100)}, true, true
	}
	if strings.HasSuffix(tok, "px") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(tok, "px"), 64)
		if err != nil {
			return stopPos{}, false, false
		}
		return stopPos{px: float32(n), isPx: true}, true, true
	}
	if n, err := strconv.ParseFloat(tok, 64); err == nil {
		if n == 0 {
			return stopPos{at: 0}, true, true
		}
		return stopPos{}, false, false
	}
	return stopPos{}, false, true
}

func parseGradientStop(s string) (GradientStop, bool) {
	stops, ok := parseGradientStops(s)
	if !ok || len(stops) != 1 {
		return GradientStop{}, false
	}
	return stops[0], true
}

func parseGradientStops(s string) ([]GradientStop, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	toks := splitOutsideParens(s)
	var pos []stopPos
	for len(toks) > 1 {
		last := toks[len(toks)-1]
		p, ok, legal := stopPosition(last)
		if !legal {
			return nil, false
		}
		if !ok {
			break
		}
		pos = append([]stopPos{p}, pos...)
		toks = toks[:len(toks)-1]
		if len(pos) == 2 {
			break
		}
	}
	c, ok := css.ParseColor(strings.Join(toks, " "))
	if !ok {
		return nil, false
	}
	stop := func(p stopPos) GradientStop {
		if p.isPx {
			return GradientStop{At: -1, Px: p.px, PxSet: true, Color: c}
		}
		return GradientStop{At: p.at, Color: c}
	}
	switch len(pos) {
	case 0:
		return []GradientStop{{Color: c, At: -1}}, true
	case 1:
		return []GradientStop{stop(pos[0])}, true
	default:
		lo, hi := pos[0], pos[1]
		if lo.isPx == hi.isPx {
			if lo.isPx {
				if hi.px < lo.px {
					hi.px = lo.px
				}
			} else if hi.at < lo.at {
				hi.at = lo.at
			}
		}
		return []GradientStop{stop(lo), stop(hi)}, true
	}
}

func distributeStops(stops []GradientStop) []GradientStop {
	for _, s := range stops {
		if s.PxSet {
			return stops
		}
	}
	n := len(stops)
	if n == 0 {
		return stops
	}
	pos := make([]float32, n)
	set := make([]bool, n)
	for i, s := range stops {
		if s.At >= 0 {
			pos[i], set[i] = s.At, true
		}
	}
	fillPositions(pos, set, 1)
	for i := range stops {
		stops[i].At = pos[i]
	}
	return stops
}

func fillPositions(pos []float32, set []bool, end float32) {
	n := len(pos)
	if n == 0 {
		return
	}
	if !set[0] {
		pos[0], set[0] = 0, true
	}
	if !set[n-1] {
		pos[n-1], set[n-1] = end, true
	}
	for i := 0; i < n; {
		if set[i] {
			i++
			continue
		}
		j := i
		for j < n && !set[j] {
			j++
		}
		lo, hi := pos[i-1], end
		if j < n {
			hi = pos[j]
		}
		for k := i; k < j; k++ {
			pos[k] = lo + (hi-lo)*float32(k-i+1)/float32(j-i+1)
		}
		i = j
	}
	for i := 1; i < n; i++ {
		if pos[i] < pos[i-1] {
			pos[i] = pos[i-1]
		}
	}
}

func StopsAlong(stops []GradientStop, lineLen float32) []GradientStop {
	any := false
	for _, s := range stops {
		if s.PxSet {
			any = true
			break
		}
	}
	if !any || lineLen <= 0 {
		return stops
	}
	n := len(stops)
	pos := make([]float32, n)
	set := make([]bool, n)
	for i, s := range stops {
		switch {
		case s.PxSet:
			pos[i], set[i] = s.Px, true
		case s.At >= 0:
			pos[i], set[i] = s.At*lineLen, true
		}
	}
	fillPositions(pos, set, lineLen)
	out := make([]GradientStop, n)
	copy(out, stops)
	for i := range out {
		out[i].At = pos[i] / lineLen
		out[i].Px, out[i].PxSet = 0, false
	}
	return out
}

func callArgs(s string) (string, bool) {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return s[:i], true
			}
		}
	}
	return "", false
}

func splitArgs(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

func splitOutsideParens(s string) []string {
	var out []string
	depth := 0
	start := 0
	flush := func(end int) {
		if end > start {
			out = append(out, s[start:end])
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t':
			if depth == 0 {
				flush(i)
				start = i + 1
			}
		}
	}
	flush(len(s))
	return out
}

func stripFunctions(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if isIdentStart(c) {
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			name := s[i:j]
			if j < len(s) && s[j] == '(' && isImageFunc(name) {
				depth := 0
				k := j
				for k < len(s) {
					if s[k] == '(' {
						depth++
					} else if s[k] == ')' {
						if depth--; depth == 0 {
							k++
							break
						}
					}
					k++
				}
				b.WriteByte(' ')
				i = k
				continue
			}
			b.WriteString(name)
			i = j
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func isIdentStart(c byte) bool {
	return c == '-' || c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || ('0' <= c && c <= '9')
}

func isImageFunc(name string) bool {
	switch strings.ToLower(name) {
	case "url", "src", "image", "image-set", "-webkit-image-set",
		"linear-gradient", "radial-gradient", "conic-gradient",
		"repeating-linear-gradient", "repeating-radial-gradient", "repeating-conic-gradient",
		"-webkit-linear-gradient", "-webkit-radial-gradient", "-webkit-gradient",
		"cross-fade", "cross-fade-url", "element", "paint":
		return true
	}
	return false
}

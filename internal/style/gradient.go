package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

// GradientStop is one colour of a ramp. At is its position along the gradient
// line as a fraction of that line's length, expressible only for a percentage
// position; a position written as a <length> cannot become a fraction until
// the box is laid out, so it rides on Px, PxSet through the cascade and paint
// resolves it against the gradient line it measures.
type GradientStop struct {
	At    float32
	Px    float32
	PxSet bool
	Color css.Color
}

// Gradient is a linear colour ramp: the direction its colours run across the
// box, and the colours themselves. Angle is CSS's, in degrees, where 0 points at
// the top of the box and the value grows clockwise, so the default is 180.
type Gradient struct {
	Angle float32
	Stops []GradientStop
}

// RadialGradient is a radial colour ramp: colours radiate from a centre point.
// Shape is "circle" or "ellipse" (default). Size is "closest-side",
// "farthest-side", "closest-corner", or "farthest-corner" (default).
// Cx, Cy are the centre as fractions of the box (0.5, 0.5 = centre).
type RadialGradient struct {
	Shape string  // "circle" or "ellipse"
	Size  string  // "closest-side", "farthest-side", "closest-corner", "farthest-corner"
	Cx    float32 // centre x as fraction (0..1)
	Cy    float32 // centre y as fraction (0..1)
	Stops []GradientStop
}

// Empty reports whether there is nothing to paint. A ramp needs two colours to
// blend between; a single stop is the flat background colour's job.
func (g Gradient) Empty() bool { return len(g.Stops) < 2 }

// Empty reports whether the radial gradient has nothing to paint.
func (g RadialGradient) Empty() bool { return len(g.Stops) < 2 }

// parseBackground reads the background shorthand. A gradient in it is the
// shorthand's image, and the colour is whatever is left once the function calls
// are out of the way — declaring a gradient alone leaves the colour transparent.
func parseBackground(v string) (css.Color, Gradient, RadialGradient) {
	return parseBackgroundWithScheme(v, "")
}

func parseBackgroundWithScheme(v, scheme string) (css.Color, Gradient, RadialGradient) {
	g := parseGradient(v)
	rg := parseRadialGradient(v)
	return parseBackgroundColorWithScheme(stripFunctions(v), scheme), g, rg
}

// parseGradient takes the first linear-gradient() or radial-gradient() in a value.
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

// parseRadialGradient takes the first radial-gradient() in a value.
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
	// Parse shape/size/position from the first parts.
	stopStart := 0
	for i, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		switch p {
		case "circle", "ellipse":
			g.Shape = p
			stopStart = i + 1
		case "closest-side", "farthest-side", "closest-corner", "farthest-corner":
			g.Size = p
			stopStart = i + 1
		case "center":
			g.Cx, g.Cy = 0.5, 0.5
			stopStart = i + 1
		case "top":
			g.Cy = 0
			stopStart = i + 1
		case "bottom":
			g.Cy = 1
			stopStart = i + 1
		case "left":
			g.Cx = 0
			stopStart = i + 1
		case "right":
			g.Cx = 1
			stopStart = i + 1
		default:
			// Check for position keywords combination or percentage.
			if strings.HasSuffix(p, "%") {
				// Could be a position percentage.
				break
			}
			// Must be a color stop, so we're done parsing the header.
			goto parseStops
		}
	}
parseStops:
	stops := make([]GradientStop, 0, len(parts)-stopStart)
	for _, p := range parts[stopStart:] {
		ss, ok := parseGradientStops(p)
		if !ok {
			return RadialGradient{}
		}
		stops = append(stops, ss...)
	}
	g.Stops = distributeStops(stops)
	return g
}

// parseGradientAngle reads the direction argument: a bare angle, or one of the
// "to …" keywords.
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

// gradientKeywordAngle turns a "to …" direction into an angle. A corner keyword
// aims the ramp at that corner, which for a box that is not square is not the
// 45deg it looks like: the gradient line is perpendicular to the line joining
// the two corners it runs between, so the angle carries the box's aspect ratio.
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
	// A corner keyword aims the ramp at that corner. The exact angle depends on
	// the box's proportions, which the cascade has no box to measure, so this is
	// the value a square box gets.
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

// stopPos is one parsed position token: either a fraction of the gradient
// line, or a length in px that cannot become a fraction until the laid-out
// box is measured at paint time.
type stopPos struct {
	at   float32
	px   float32
	isPx bool
}

// stopPosition reads the token that ends a colour stop. It returns the
// position, whether the token IS one, and whether it could have been one - a
// token that could have been and was not invalidates the whole ramp rather
// than being folded into the colour, which is what Chromium does with
// `#0a0 50`.
//
// A percentage is always a position. A bare number is one only when it is
// zero, because CSS lets a <length> drop its unit only at zero. A pixel
// length is a real position too, but the model cannot turn it into a
// fraction of the gradient line at cascade time - the line's length belongs
// to the laid-out box - so it rides on the stop (Px, PxSet) for paint to
// resolve; the gap this leaves for other units is recorded as
// GOOSIE-GRADIENTPX-001.
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

// parseGradientStop reads one stop: a colour and an optional percentage
// position.
func parseGradientStop(s string) (GradientStop, bool) {
	stops, ok := parseGradientStops(s)
	if !ok || len(stops) != 1 {
		return GradientStop{}, false
	}
	return stops[0], true
}

// parseGradientStops reads one stop, expanding a double position
// (color 25% 75%) into the solid band it denotes: two stops sharing the
// colour at each position. A position is a <length-percentage>, so it is
// written as a percentage, as a length with a unit, or - the only unitless
// form CSS allows - as a bare zero. Percentage and bare-zero positions become
// fractions here; pixel lengths ride on the stop unresolved, because the
// fraction they denote needs the laid-out gradient line. Single positions and
// bare colours behave as before; a bare non-zero number makes the stop, and
// so the whole ramp, invalid.
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
		// Unpositioned: distributeStops gives it an even share of the line.
		return []GradientStop{{Color: c, At: -1}}, true
	case 1:
		return []GradientStop{stop(pos[0])}, true
	default:
		lo, hi := pos[0], pos[1]
		// Two tokens of the same kind can be ordered here; a mixed pair -
		// `10px 50%` - waits for paint, which resolves the lengths first and
		// clamps the whole run afterwards.
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

// distributeStops fills in the positions the declaration left out. The ends
// anchor at 0 and 1, a run of unpositioned stops between two positioned ones
// splits the distance evenly, and positions may not run backwards. A ramp
// carrying a <length> position skips all of it: a fraction of the gradient
// line is not expressible until the box is measured, so paint resolves the
// whole run through StopsAlong instead.
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

// fillPositions fills unset positions between set ones: the ends anchor at 0
// and end, an unset run between two set positions splits the distance evenly,
// and the run may not go backwards. One algorithm serves both the cascade -
// fractions of the line, end 1 - and paint's resolution of length stops along
// the measured line, end lineLen.
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
		// pos[i-1] is set and pos[j] is, or is the end.
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

// StopsAlong resolves a ramp's length-written stop positions against the
// gradient line the painter measured: each px position becomes a fraction of
// that line, the stops the declaration left unpositioned are distributed as
// usual, and the run never goes backwards. A ramp without a length position
// comes back unchanged - the cascade already distributed those.
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

// callArgs returns what sits inside a function call whose opening parenthesis
// has already been consumed, stopping at the matching close.
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

// splitArgs breaks a function's argument list on the commas that separate its
// top-level arguments, leaving the commas inside rgb(…) alone.
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

// splitOutsideParens breaks a value on whitespace that is not inside a
// function call, so a colour and its position separate but rgba(0, 0, 0, .5)
// does not.
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

// stripFunctions removes the image-producing function calls from a background
// value so the remaining tokens can be read for the colour. It must NOT touch
// colour functions: `background: rgba(0,0,0,.4)` keeps its rgba(), while
// `url(...)`, `linear-gradient(...)` and friends are dropped. Only the named
// image functions are excised, balanced-paren included.
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

// isImageFunc reports whether a function name produces an image layer (and so
// must be stripped before reading the shorthand's colour). It receives the raw
// identifier; the comparison is case-insensitive.
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

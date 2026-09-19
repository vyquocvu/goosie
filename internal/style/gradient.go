package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

// GradientStop is one colour of a ramp. At is its position along the gradient
// line as a fraction of that line's length, so it is only expressible for a
// percentage position; a stop declared in pixels would need the box's size.
type GradientStop struct {
	At    float32
	Color css.Color
}

// Gradient is a linear colour ramp: the direction its colours run across the
// box, and the colours themselves. Angle is CSS's, in degrees, where 0 points at
// the top of the box and the value grows clockwise, so the default is 180.
type Gradient struct {
	Angle float32
	Stops []GradientStop
}

// Empty reports whether there is nothing to paint. A ramp needs two colours to
// blend between; a single stop is the flat background colour's job.
func (g Gradient) Empty() bool { return len(g.Stops) < 2 }

// parseBackground reads the background shorthand. A gradient in it is the
// shorthand's image, and the colour is whatever is left once the function calls
// are out of the way — declaring a gradient alone leaves the colour transparent.
func parseBackground(v string) (css.Color, Gradient) {
	g := parseGradient(v)
	return parseBackgroundColor(stripFunctions(v)), g
}

// parseGradient takes the first linear-gradient() in a value. Layered and
// radial images are a later milestone, and a box that declares them still gets
// its background colour.
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
		s, ok := parseGradientStop(p)
		if !ok {
			return Gradient{}
		}
		stops = append(stops, s)
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

// parseGradientStop reads one stop: a colour and an optional percentage
// position.
func parseGradientStop(s string) (GradientStop, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return GradientStop{}, false
	}
	toks := splitOutsideParens(s)
	at := float32(-1)
	if n := len(toks); n > 1 {
		last := toks[n-1]
		if strings.HasSuffix(last, "%") {
			p, err := strconv.ParseFloat(strings.TrimSuffix(last, "%"), 64)
			if err != nil {
				return GradientStop{}, false
			}
			at = float32(p / 100)
			toks = toks[:n-1]
		}
	}
	c, ok := css.ParseColor(strings.Join(toks, " "))
	if !ok {
		return GradientStop{}, false
	}
	if at < 0 {
		// Unpositioned: distributeStops gives it an even share of the line.
		return GradientStop{Color: c, At: -1}, true
	}
	return GradientStop{Color: c, At: at}, true
}

// distributeStops fills in the positions the declaration left out. The ends
// anchor at 0 and 1, a run of unpositioned stops between two positioned ones
// splits the distance evenly, and positions may not run backwards.
func distributeStops(stops []GradientStop) []GradientStop {
	n := len(stops)
	if n == 0 {
		return stops
	}
	if stops[0].At < 0 {
		stops[0].At = 0
	}
	if stops[n-1].At < 0 {
		stops[n-1].At = 1
	}
	for i := 0; i < n; {
		if stops[i].At >= 0 {
			i++
			continue
		}
		j := i
		for j < n && stops[j].At < 0 {
			j++
		}
		// stops[i-1] is positioned and stops[j] is, or is the end.
		lo, hi := stops[i-1].At, float32(1)
		if j < n {
			hi = stops[j].At
		}
		for k := i; k < j; k++ {
			stops[k].At = lo + (hi-lo)*float32(k-i+1)/float32(j-i+1)
		}
		i = j
	}
	for i := 1; i < n; i++ {
		if stops[i].At < stops[i-1].At {
			stops[i].At = stops[i-1].At
		}
	}
	return stops
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

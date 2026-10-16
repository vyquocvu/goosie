package css

import (
	"strings"
)

// TimingFunc is a CSS transition timing function.
type TimingFunc uint8

const (
	TimingEase TimingFunc = iota
	TimingLinear
	TimingEaseIn
	TimingEaseOut
	TimingEaseInOut
)

// ParseTimingFunc parses a CSS timing function keyword.
func ParseTimingFunc(s string) TimingFunc {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "linear":
		return TimingLinear
	case "ease-in":
		return TimingEaseIn
	case "ease-out":
		return TimingEaseOut
	case "ease-in-out":
		return TimingEaseInOut
	case "ease":
		return TimingEase
	}
	return TimingEase
}

// EvalTimingFunc evaluates the timing function at progress t (0..1), returning
// the eased progress value.
func (tf TimingFunc) Eval(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	switch tf {
	case TimingLinear:
		return t
	case TimingEaseIn:
		return t * t
	case TimingEaseOut:
		return t * (2 - t)
	case TimingEaseInOut:
		if t < 0.5 {
			return 2 * t * t
		}
		return -1 + (4-2*t)*t
	case TimingEase:
		// cubic-bezier(0.25, 0.1, 0.25, 1.0) approximation
		return cubicBezier(0.25, 0.1, 0.25, 1.0, t)
	}
	return t
}

// cubicBezier approximates a cubic-bezier timing function using Newton's method.
func cubicBezier(x1, y1, x2, y2, t float64) float64 {
	// Find the parametric t for the given x using Newton-Raphson.
	guess := t
	for i := 0; i < 8; i++ {
		x := bezier(x1, x2, guess) - t
		if abs64(x) < 1e-6 {
			break
		}
		dx := bezierDeriv(x1, x2, guess)
		if abs64(dx) < 1e-6 {
			break
		}
		guess -= x / dx
	}
	return bezier(y1, y2, guess)
}

func bezier(p1, p2, t float64) float64 {
	t2 := t * t
	t3 := t2 * t
	return 3*(1-t)*(1-t)*t*p1 + 3*(1-t)*t2*p2 + t3
}

func bezierDeriv(p1, p2, t float64) float64 {
	return 3*(1-t)*(1-t)*p1 + 6*(1-t)*t*(p2-p1) + 3*t*t*(1-p2)
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Transition is one parsed transition from the transition shorthand or
// longhand properties.
type Transition struct {
	Property string     // CSS property name, or "all", or "" for none
	Duration float64    // duration in seconds
	Timing   TimingFunc // timing function
	Delay    float64    // delay in seconds
}

// ParseTransitionShorthand parses the CSS `transition` shorthand value, which
// may contain comma-separated transitions. Each transition is:
//
//	<property> <duration> <timing-function> <delay>
//
// Duration and delay are time values (e.g. "0.3s", "300ms"). Only the duration
// is required; the rest default to "all", "ease", and "0s" respectively.
func ParseTransitionShorthand(value string) []Transition {
	value = strings.TrimSpace(value)
	if value == "" || strings.ToLower(value) == "none" {
		return nil
	}

	parts := splitTransitionParts(value)
	var transitions []Transition
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		transitions = append(transitions, parseOneTransition(part))
	}
	return transitions
}

// splitTransitionParts splits on commas that are outside parentheses, so
// `cubic-bezier(0.25, 0.1, 0.25, 1.0)` does not break into pieces.
func splitTransitionParts(s string) []string {
	var parts []string
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
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseOneTransition parses one transition entry: property duration [timing] [delay].
func parseOneTransition(s string) Transition {
	tr := Transition{
		Property: "all",
		Timing:   TimingEase,
	}

	tokens := splitTransitionTokens(s)
	var times []float64

	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if isTimingValue(lower) {
			if t, ok := parseTime(lower); ok {
				times = append(times, t)
			}
			continue
		}
		if isTimingFuncKeyword(lower) {
			tr.Timing = ParseTimingFunc(lower)
			continue
		}
		// Otherwise it is the property name.
		tr.Property = lower
	}

	if len(times) > 0 {
		tr.Duration = times[0]
	}
	if len(times) > 1 {
		tr.Delay = times[1]
	}

	return tr
}

// splitTransitionTokens splits a single transition entry on whitespace, keeping
// function calls like cubic-bezier(...) intact.
func splitTransitionTokens(s string) []string {
	var tokens []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '(':
			depth++
			if start < 0 {
				start = i
			}
		case ')':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				tokens = append(tokens, strings.TrimSpace(s[start:i+1]))
				start = -1
			}
		case ' ', '\t':
			if depth == 0 {
				if start >= 0 {
					tokens = append(tokens, strings.TrimSpace(s[start:i]))
					start = -1
				}
			} else if start < 0 {
				start = i
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, strings.TrimSpace(s[start:]))
	}
	return tokens
}

// isTimingValue reports whether a token looks like a CSS time value.
func isTimingValue(s string) bool {
	return strings.HasSuffix(s, "s") || strings.HasSuffix(s, "ms")
}

// isTimingFuncKeyword reports whether a token is a timing function keyword.
func isTimingFuncKeyword(s string) bool {
	switch s {
	case "ease", "linear", "ease-in", "ease-out", "ease-in-out":
		return true
	}
	return strings.HasPrefix(s, "cubic-bezier") || strings.HasPrefix(s, "steps")
}

// parseTime parses a CSS time value (e.g. "0.3s", "300ms") into seconds.
func parseTime(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "ms") {
		num := strings.TrimSpace(s[:len(s)-2])
		v, ok := parseFloat64(num)
		if !ok {
			return 0, false
		}
		return v / 1000, true
	}
	if strings.HasSuffix(s, "s") {
		num := strings.TrimSpace(s[:len(s)-1])
		v, ok := parseFloat64(num)
		if !ok {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// parseFloat64 is a simple float parser that avoids importing strconv in this file.
func parseFloat64(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	// Use the same approach as the token package.
	v := parseFloat(s)
	return v, v != 0 || s == "0" || s == "0.0" || s == ".0" || s == "0." ||
		strings.HasPrefix(s, "0.") || strings.HasPrefix(s, ".0") ||
		strings.HasPrefix(s, "-0") || strings.HasPrefix(s, "+0")
}

// ParseTransitionProperty parses the transition-property longhand.
func ParseTransitionProperty(value string) []string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "none" {
		return nil
	}
	if value == "all" {
		return []string{"all"}
	}
	var props []string
	for _, p := range strings.Split(value, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			props = append(props, p)
		}
	}
	return props
}

// ParseTransitionDuration parses the transition-duration longhand, which is a
// comma-separated list of time values.
func ParseTransitionDuration(value string) []float64 {
	var durations []float64
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if t, ok := parseTime(part); ok {
			durations = append(durations, t)
		}
	}
	return durations
}

// ParseTransitionTimingFunction parses the transition-timing-function longhand.
func ParseTransitionTimingFunction(value string) []TimingFunc {
	var funcs []TimingFunc
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			funcs = append(funcs, ParseTimingFunc(part))
		}
	}
	return funcs
}

// ParseTransitionDelay parses the transition-delay longhand.
func ParseTransitionDelay(value string) []float64 {
	var delays []float64
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if t, ok := parseTime(part); ok {
			delays = append(delays, t)
		}
	}
	return delays
}

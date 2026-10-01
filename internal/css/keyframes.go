package css

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// KeyframesRule represents a parsed @keyframes at-rule.
type KeyframesRule struct {
	Name  string         // the animation name
	Stops []KeyframeStop // sorted by Offset
}

// KeyframeStop is one keyframe within a @keyframes rule.
type KeyframeStop struct {
	Offset       float32           // 0.0 to 1.0 (from=0, to=1)
	Declarations map[string]string // property: value pairs
}

// ParseKeyframes parses the body of a @keyframes rule.
// Input is the content between the braces after "@keyframes name".
// Example input: "from { opacity: 0 } to { opacity: 1 }"
// Or: "0% { opacity: 0 } 50% { opacity: 0.5 } 100% { opacity: 1 }"
func ParseKeyframes(body string) ([]KeyframeStop, error) {
	var stops []KeyframeStop
	pos := 0
	for pos < len(body) {
		// Skip whitespace.
		for pos < len(body) && isKFWhitespace(body[pos]) {
			pos++
		}
		if pos >= len(body) {
			break
		}

		// Read selector(s) up to '{'.
		selStart := pos
		for pos < len(body) && body[pos] != '{' {
			pos++
		}
		if pos >= len(body) {
			break // no block found
		}
		selText := strings.TrimSpace(body[selStart:pos])
		pos++ // skip '{'

		// Read block content up to matching '}'.
		depth := 1
		blockStart := pos
		for pos < len(body) && depth > 0 {
			switch body[pos] {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth == 0 {
				break
			}
			pos++
		}
		blockText := body[blockStart:pos]
		if pos < len(body) {
			pos++ // skip '}'
		}

		if selText == "" {
			continue
		}

		// Parse offsets from selector text.
		offsets, err := parseKeyframeSelectors(selText)
		if err != nil {
			return nil, err
		}

		// Parse declarations from block text.
		decls := parseKeyframeDeclarations(blockText)

		// Create a stop for each offset, sharing the same declarations.
		for _, off := range offsets {
			// Copy declarations so each stop is independent.
			d := make(map[string]string, len(decls))
			for k, v := range decls {
				d[k] = v
			}
			stops = append(stops, KeyframeStop{
				Offset:       off,
				Declarations: d,
			})
		}
	}

	// Sort stops by offset.
	sort.Slice(stops, func(i, j int) bool {
		return stops[i].Offset < stops[j].Offset
	})

	return stops, nil
}

// isKFWhitespace reports whether c is CSS whitespace.
func isKFWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// parseKeyframeSelectors parses a comma-separated list of keyframe selectors
// and returns their offsets. Selectors can be "from", "to", or "N%".
func parseKeyframeSelectors(s string) ([]float32, error) {
	parts := strings.Split(s, ",")
	var offsets []float32
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		switch part {
		case "from":
			offsets = append(offsets, 0.0)
		case "to":
			offsets = append(offsets, 1.0)
		default:
			if !strings.HasSuffix(part, "%") {
				return nil, fmt.Errorf("invalid keyframe selector: %q", part)
			}
			numStr := strings.TrimSpace(part[:len(part)-1])
			v, ok := parseFloat64(numStr)
			if !ok {
				return nil, fmt.Errorf("invalid keyframe percentage: %q", part)
			}
			if v < 0 || v > 100 {
				return nil, fmt.Errorf("keyframe percentage out of range: %q", part)
			}
			offsets = append(offsets, float32(v/100))
		}
	}
	if len(offsets) == 0 {
		return nil, fmt.Errorf("empty keyframe selector")
	}
	return offsets, nil
}

// parseKeyframeDeclarations parses the declarations inside a keyframe block.
// Returns a map of property -> value.
func parseKeyframeDeclarations(s string) map[string]string {
	decls := make(map[string]string)
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colonIdx := strings.Index(part, ":")
		if colonIdx < 0 {
			continue
		}
		prop := strings.TrimSpace(part[:colonIdx])
		val := strings.TrimSpace(part[colonIdx+1:])
		if prop == "" || val == "" {
			continue
		}
		decls[strings.ToLower(prop)] = val
	}
	return decls
}

// AnimationDirection controls how @keyframes animations play.
type AnimationDirection int

const (
	AnimNormal           AnimationDirection = iota // forward
	AnimReverse                                    // backward
	AnimAlternate                                  // forward, then backward
	AnimAlternateReverse                           // backward, then forward
)

// ParseAnimationDirection parses a CSS animation-direction keyword.
func ParseAnimationDirection(s string) AnimationDirection {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "reverse":
		return AnimReverse
	case "alternate":
		return AnimAlternate
	case "alternate-reverse":
		return AnimAlternateReverse
	}
	return AnimNormal
}

// AnimationFillMode controls what happens before/after a @keyframes animation.
type AnimationFillMode int

const (
	FillNone      AnimationFillMode = iota
	FillForwards                    // holds last keyframe after completion
	FillBackwards                   // applies first keyframe during delay
	FillBoth                        // both forwards and backwards
)

// ParseAnimationFillMode parses a CSS animation-fill-mode keyword.
func ParseAnimationFillMode(s string) AnimationFillMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "forwards":
		return FillForwards
	case "backwards":
		return FillBackwards
	case "both":
		return FillBoth
	}
	return FillNone
}

// ParseDuration parses a CSS time value ("1s", "500ms") into a Go Duration.
// Returns 0 for unparseable values.
func ParseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "ms") {
		num := strings.TrimSpace(s[:len(s)-2])
		if v, ok := parseFloat64(num); ok {
			return time.Duration(v * float64(time.Millisecond))
		}
		return 0
	}
	if strings.HasSuffix(s, "s") {
		num := strings.TrimSpace(s[:len(s)-1])
		if v, ok := parseFloat64(num); ok {
			return time.Duration(v * float64(time.Second))
		}
		return 0
	}
	return 0
}

// ParseAnimationShorthand parses the CSS animation shorthand value.
// Format: name duration [timing] [delay] [itercount] [direction] [fillmode]
func ParseAnimationShorthand(value string) (
	name string,
	duration time.Duration,
	timing TimingFunc,
	delay time.Duration,
	iterCount int,
	direction AnimationDirection,
	fillMode AnimationFillMode,
) {
	timing = TimingEase
	iterCount = 1
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return
	}

	tokens := splitAnimTokens(value)
	timeIdx := 0
	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		switch lower {
		case "ease", "linear", "ease-in", "ease-out", "ease-in-out":
			timing = ParseTimingFunc(lower)
		case "infinite":
			iterCount = 0
		case "normal", "reverse", "alternate", "alternate-reverse":
			direction = ParseAnimationDirection(lower)
		case "forwards", "backwards", "both":
			fillMode = ParseAnimationFillMode(lower)
		default:
			if isTimeValue(lower) {
				d := ParseDuration(lower)
				if timeIdx == 0 {
					duration = d
				} else {
					delay = d
				}
				timeIdx++
			} else if isPositiveInt(lower) {
				if v, ok := parseFloat64(lower); ok {
					iterCount = int(v)
				}
			} else if name == "" {
				name = tok
			}
		}
	}
	return
}

// splitAnimTokens splits on whitespace outside parentheses.
func splitAnimTokens(s string) []string {
	var tokens []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		switch s[i] {
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
		case ' ', '\t', '\n', '\r':
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

// isTimeValue reports whether a token looks like a CSS time value.
func isTimeValue(s string) bool {
	return strings.HasSuffix(s, "ms") || strings.HasSuffix(s, "s")
}

// isPositiveInt reports whether s is a positive integer string.
func isPositiveInt(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if c < '0' || c > '9' {
			if i == 0 && c == '+' {
				continue
			}
			return false
		}
	}
	return true
}

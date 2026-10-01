package css

import (
	"strings"
)

// BoxShadow is one resolved box-shadow layer.
type BoxShadow struct {
	OffsetX float32
	OffsetY float32
	Blur    float32
	Spread  float32
	Color   Color
	Inset   bool
}

// TextShadow is one resolved text-shadow layer.
type TextShadow struct {
	OffsetX float32
	OffsetY float32
	Blur    float32
	Color   Color
}

// ParseBoxShadow parses a box-shadow property value into one or more shadow
// layers. Returns nil when the value is "none" or unparseable.
//
// Syntax: [inset? <offset-x> <offset-y> <blur-radius>? <spread-distance>? <color>?]#
func ParseBoxShadow(value string) []BoxShadow {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") || value == "" {
		return nil
	}
	layers := splitShadowLayers(value)
	var shadows []BoxShadow
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		s, ok := parseOneBoxShadow(layer)
		if !ok {
			continue
		}
		shadows = append(shadows, s)
	}
	return shadows
}

// ParseTextShadow parses a text-shadow property value into one or more shadow
// layers. Returns nil when the value is "none" or unparseable.
//
// Syntax: [<offset-x> <offset-y> <blur-radius>? <color>?]#
func ParseTextShadow(value string) []TextShadow {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") || value == "" {
		return nil
	}
	layers := splitShadowLayers(value)
	var shadows []TextShadow
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		s, ok := parseOneTextShadow(layer)
		if !ok {
			continue
		}
		shadows = append(shadows, s)
	}
	return shadows
}

// splitShadowLayers splits a shadow value on commas that are not inside
// parentheses (so rgb()/hsla() survive intact).
func splitShadowLayers(value string) []string {
	var layers []string
	depth := 0
	start := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				layers = append(layers, value[start:i])
				start = i + 1
			}
		}
	}
	layers = append(layers, value[start:])
	return layers
}

// parseOneBoxShadow parses a single box-shadow layer.
func parseOneBoxShadow(s string) (BoxShadow, bool) {
	var shadow BoxShadow
	tokens := splitShadowTokens(s)
	lengths := []float32{}
	colorFound := false

	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if lower == "inset" {
			shadow.Inset = true
			continue
		}
		if c, ok := ParseColor(lower); ok {
			shadow.Color = c
			colorFound = true
			continue
		}
		// Try to parse as a length.
		v := ParseValue(tok)
		if l := resolveShadowLength(v); l != 0 || (v.Type == ValueLength || v.Type == ValueNumber) {
			lengths = append(lengths, l)
			continue
		}
		// Unknown token - skip.
	}

	// Need at least offsetX and offsetY.
	if len(lengths) < 2 {
		return shadow, false
	}
	shadow.OffsetX = lengths[0]
	shadow.OffsetY = lengths[1]
	if len(lengths) >= 3 {
		shadow.Blur = lengths[2]
	}
	if len(lengths) >= 4 {
		shadow.Spread = lengths[3]
	}
	if !colorFound {
		// Default color is currentColor, represented as opaque black here.
		shadow.Color = Color{R: 0, G: 0, B: 0, A: 255}
	}
	return shadow, true
}

// parseOneTextShadow parses a single text-shadow layer.
func parseOneTextShadow(s string) (TextShadow, bool) {
	var shadow TextShadow
	tokens := splitShadowTokens(s)
	lengths := []float32{}
	colorFound := false

	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if c, ok := ParseColor(lower); ok {
			shadow.Color = c
			colorFound = true
			continue
		}
		v := ParseValue(tok)
		if l := resolveShadowLength(v); l != 0 || (v.Type == ValueLength || v.Type == ValueNumber) {
			lengths = append(lengths, l)
			continue
		}
	}

	if len(lengths) < 2 {
		return shadow, false
	}
	shadow.OffsetX = lengths[0]
	shadow.OffsetY = lengths[1]
	if len(lengths) >= 3 {
		shadow.Blur = lengths[2]
	}
	if !colorFound {
		shadow.Color = Color{R: 0, G: 0, B: 0, A: 255}
	}
	return shadow, true
}

// splitShadowTokens splits a shadow layer value on whitespace outside
// parentheses, keeping color function calls like rgb() intact.
func splitShadowTokens(s string) []string {
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

// resolveShadowLength converts a CSS value to a float32 pixel length for
// shadow offsets. Returns 0 for non-length values.
func resolveShadowLength(v Value) float32 {
	return v.ToLength()
}

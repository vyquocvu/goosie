package css

import (
	"strings"
)

type BoxShadow struct {
	OffsetX        float32
	OffsetY        float32
	Blur           float32
	Spread         float32
	Color          Color
	Inset          bool
	ColorIsCurrent bool
}

type TextShadow struct {
	OffsetX        float32
	OffsetY        float32
	Blur           float32
	Color          Color
	ColorIsCurrent bool
}

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
			return nil
		}
		s, ok := parseOneBoxShadow(layer)
		if !ok {
			return nil
		}
		shadows = append(shadows, s)
	}
	return shadows
}

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
			return nil
		}
		s, ok := parseOneTextShadow(layer)
		if !ok {
			return nil
		}
		shadows = append(shadows, s)
	}
	return shadows
}

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
		if lower == "currentcolor" {
			shadow.ColorIsCurrent = true
			colorFound = true
			continue
		}
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
		return shadow, false
	}

	if len(lengths) < 2 || len(lengths) > 4 {
		return shadow, false
	}
	if len(lengths) >= 3 && lengths[2] < 0 {
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
		shadow.ColorIsCurrent = true
	}
	return shadow, true
}

func parseOneTextShadow(s string) (TextShadow, bool) {
	var shadow TextShadow
	tokens := splitShadowTokens(s)
	lengths := []float32{}
	colorFound := false

	for _, tok := range tokens {
		lower := strings.ToLower(tok)
		if lower == "currentcolor" {
			shadow.ColorIsCurrent = true
			colorFound = true
			continue
		}
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
		return shadow, false
	}

	if len(lengths) < 2 || len(lengths) > 3 {
		return shadow, false
	}
	if len(lengths) >= 3 && lengths[2] < 0 {
		return shadow, false
	}
	shadow.OffsetX = lengths[0]
	shadow.OffsetY = lengths[1]
	if len(lengths) >= 3 {
		shadow.Blur = lengths[2]
	}
	if !colorFound {
		shadow.ColorIsCurrent = true
	}
	return shadow, true
}

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

func resolveShadowLength(v Value) float32 {
	return v.ToLength()
}

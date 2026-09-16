package css

import (
	"strings"
)

// Value represents a parsed CSS value.
type Value struct {
	Type    ValueType
	Num     float64
	Unit    string
	Str     string
	Color   Color
	Parts   []Value
	Func    string
}

// ValueType identifies the kind of CSS value.
type ValueType uint8

const (
	ValueKeyword ValueType = iota
	ValueNumber
	ValueLength
	ValuePercentage
	ValueColor
	ValueString
	ValueURL
	ValueList
	ValueFunc
	ValueInherit
	ValueInitial
	ValueUnset
)

// Color is a parsed CSS color in RGBA.
type Color struct {
	R, G, B, A uint8
}

// ParseValue parses a CSS value string into a Value.
func ParseValue(s string) Value {
	s = strings.TrimSpace(s)
	if s == "" {
		return Value{Type: ValueKeyword, Str: ""}
	}

	lower := strings.ToLower(s)
	switch lower {
	case "inherit":
		return Value{Type: ValueInherit}
	case "initial":
		return Value{Type: ValueInitial}
	case "unset":
		return Value{Type: ValueUnset}
	}

	if c, ok := ParseColor(s); ok {
		return Value{Type: ValueColor, Color: c}
	}

	if strings.HasPrefix(s, "url(") {
		inner := s[4:]
		if strings.HasSuffix(inner, ")") {
			inner = inner[:len(inner)-1]
		}
		inner = strings.TrimSpace(inner)
		inner = strings.Trim(inner, "\"'")
		return Value{Type: ValueURL, Str: inner}
	}

	if strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		inner := s[1:]
		if len(inner) > 0 && (inner[len(inner)-1] == '"' || inner[len(inner)-1] == '\'') {
			inner = inner[:len(inner)-1]
		}
		return Value{Type: ValueString, Str: inner}
	}

	if idx := strings.Index(s, "("); idx > 0 && strings.HasSuffix(s, ")") {
		fn := s[:idx]
		args := s[idx+1 : len(s)-1]
		var parts []Value
		for _, a := range splitArgs(args) {
			parts = append(parts, ParseValue(strings.TrimSpace(a)))
		}
		return Value{Type: ValueFunc, Func: fn, Parts: parts}
	}

	parts := strings.Fields(s)
	if len(parts) > 1 {
		var vals []Value
		for _, p := range parts {
			vals = append(vals, ParseValue(p))
		}
		return Value{Type: ValueList, Parts: vals}
	}

	tok := NewTokenizer(s)
	t := tok.Next()
	switch t.Type {
	case TokenNumber:
		return Value{Type: ValueNumber, Num: t.NumVal}
	case TokenPercentage:
		return Value{Type: ValuePercentage, Num: t.NumVal}
	case TokenDimension:
		return Value{Type: ValueLength, Num: t.NumVal, Unit: strings.ToLower(t.Unit)}
	case TokenIdent:
		return Value{Type: ValueKeyword, Str: strings.ToLower(t.Value)}
	}

	return Value{Type: ValueKeyword, Str: lower}
}

func splitArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
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

func ParseColor(s string) (Color, bool) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	if named, ok := namedColors[lower]; ok {
		return named, true
	}

	if strings.HasPrefix(s, "#") {
		return parseHex(s)
	}

	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		return parseRGB(s[4 : len(s)-1])
	}
	if strings.HasPrefix(s, "rgba(") && strings.HasSuffix(s, ")") {
		return parseRGBA(s[5 : len(s)-1])
	}

	return Color{}, false
}

func parseHex(s string) (Color, bool) {
	s = s[1:]
	switch len(s) {
	case 3:
		r := unhex(s[0]) * 17
		g := unhex(s[1]) * 17
		b := unhex(s[2]) * 17
		return Color{R: r, G: g, B: b, A: 255}, true
	case 4:
		r := unhex(s[0]) * 17
		g := unhex(s[1]) * 17
		b := unhex(s[2]) * 17
		a := unhex(s[3]) * 17
		return Color{R: r, G: g, B: b, A: a}, true
	case 6:
		r := unhex(s[0])<<4 | unhex(s[1])
		g := unhex(s[2])<<4 | unhex(s[3])
		b := unhex(s[4])<<4 | unhex(s[5])
		return Color{R: r, G: g, B: b, A: 255}, true
	case 8:
		r := unhex(s[0])<<4 | unhex(s[1])
		g := unhex(s[2])<<4 | unhex(s[3])
		b := unhex(s[4])<<4 | unhex(s[5])
		a := unhex(s[6])<<4 | unhex(s[7])
		return Color{R: r, G: g, B: b, A: a}, true
	}
	return Color{}, false
}

func unhex(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func parseRGB(s string) (Color, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return Color{}, false
	}
	r := clampByte(parseComponent(strings.TrimSpace(parts[0])))
	g := clampByte(parseComponent(strings.TrimSpace(parts[1])))
	b := clampByte(parseComponent(strings.TrimSpace(parts[2])))
	return Color{R: r, G: g, B: b, A: 255}, true
}

func parseRGBA(s string) (Color, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return Color{}, false
	}
	r := clampByte(parseComponent(strings.TrimSpace(parts[0])))
	g := clampByte(parseComponent(strings.TrimSpace(parts[1])))
	b := clampByte(parseComponent(strings.TrimSpace(parts[2])))
	a := clampByte(parseAlpha(strings.TrimSpace(parts[3])))
	return Color{R: r, G: g, B: b, A: a}, true
}

func parseComponent(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		return parseFloat(s[:len(s)-1]) / 100 * 255
	}
	return parseFloat(s)
}

func parseAlpha(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		return parseFloat(s[:len(s)-1]) / 100 * 255
	}
	return parseFloat(s) * 255
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

var namedColors = map[string]Color{
	"transparent": {0, 0, 0, 0},
	"black":       {0, 0, 0, 255},
	"white":       {255, 255, 255, 255},
	"red":         {255, 0, 0, 255},
	"green":       {0, 128, 0, 255},
	"blue":        {0, 0, 255, 255},
	"yellow":      {255, 255, 0, 255},
	"cyan":        {0, 255, 255, 255},
	"aqua":        {0, 255, 255, 255},
	"magenta":     {255, 0, 255, 255},
	"fuchsia":     {255, 0, 255, 255},
	"silver":      {192, 192, 192, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"maroon":      {128, 0, 0, 255},
	"olive":       {128, 128, 0, 255},
	"lime":        {0, 255, 0, 255},
	"teal":        {0, 128, 128, 255},
	"navy":        {0, 0, 128, 255},
	"orange":      {255, 165, 0, 255},
	"purple":      {128, 0, 128, 255},
	"pink":        {255, 192, 203, 255},
	"brown":       {165, 42, 42, 255},
	"coral":       {255, 127, 80, 255},
	"crimson":     {220, 20, 60, 255},
	"darkblue":    {0, 0, 139, 255},
	"darkgray":    {169, 169, 169, 255},
	"darkgrey":    {169, 169, 169, 255},
	"darkgreen":   {0, 100, 0, 255},
	"darkred":     {139, 0, 0, 255},
	"gold":        {255, 215, 0, 255},
	"indigo":      {75, 0, 130, 255},
	"ivory":       {255, 255, 240, 255},
	"khaki":       {240, 230, 140, 255},
	"lavender":    {230, 230, 250, 255},
	"lightblue":   {173, 216, 230, 255},
	"lightgray":   {211, 211, 211, 255},
	"lightgrey":   {211, 211, 211, 255},
	"lightgreen":  {144, 238, 144, 255},
	"lightyellow": {255, 255, 224, 255},
	"tomato":      {255, 99, 71, 255},
	"salmon":      {250, 128, 114, 255},
	"skyblue":     {135, 206, 235, 255},
	"slategray":   {112, 128, 144, 255},
	"snow":        {255, 250, 250, 255},
	"steelblue":   {70, 130, 180, 255},
	"tan":         {210, 180, 140, 255},
	"thistle":     {216, 191, 216, 255},
	"turquoise":   {64, 224, 208, 255},
	"violet":      {238, 130, 238, 255},
	"wheat":       {245, 222, 179, 255},
	"whitesmoke":  {245, 245, 245, 255},
	"yellowgreen": {154, 205, 50, 255},
}

// ToLength converts a value to a float32 length in pixels. Returns 0 for
// non-length values. Unit conversion uses CSS defaults: em/ex default to 16px,
// rem to 16px, pt to 1.333px, pc to 16px, in to 96px, cm to 37.795px, mm to
// 3.7795px.
func (v Value) ToLength() float32 {
	switch v.Type {
	case ValueLength:
		return toPixels(v.Num, v.Unit)
	case ValueNumber:
		return float32(v.Num)
	case ValuePercentage:
		return 0
	}
	return 0
}

func toPixels(num float64, unit string) float32 {
	switch unit {
	case "px", "":
		return float32(num)
	case "em", "rem", "ex":
		return float32(num * 16)
	case "pt":
		return float32(num * 96 / 72)
	case "pc":
		return float32(num * 16)
	case "in":
		return float32(num * 96)
	case "cm":
		return float32(num * 96 / 2.54)
	case "mm":
		return float32(num * 96 / 25.4)
	case "vw", "vh", "vmin", "vmax":
		return 0
	}
	return float32(num)
}

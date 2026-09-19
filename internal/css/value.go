package css

import (
	"math"
	"strings"
)

// Value represents a parsed CSS value.
type Value struct {
	Type  ValueType
	Num   float64
	Unit  string
	Str   string
	Color Color
	Parts []Value
	Func  string
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
		return Value{Type: ValueString, Str: DecodeEscapes(inner)}
	}

	if fn, ok := lengthFuncName(s); ok {
		// The expression stays as text: `em` and viewport terms can only be
		// resolved once the cascade knows this element's font size and the frame
		// it is laid out in, which is later than parsing. EvalFunc reads it back
		// at that point.
		inner := s[strings.Index(s, "(")+1 : len(s)-1]
		return Value{Type: ValueFunc, Func: fn, Str: strings.TrimSpace(inner)}
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

// lengthFuncName recognises the functions whose whole value is one length.
// They keep their argument text unevaluated, unlike rgb() or var(), because
// their arguments are expressions rather than lists of components.
func lengthFuncName(s string) (string, bool) {
	idx := strings.IndexByte(s, '(')
	if idx <= 0 || !strings.HasSuffix(s, ")") {
		return "", false
	}
	switch lower := strings.ToLower(s[:idx]); lower {
	case "calc", "min", "max", "clamp":
		// The trailing ')' has to close this function: `max(1px,2px) * 2` is an
		// expression around the call, not a call around the expression.
		depth := 0
		for i := idx; i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i != len(s)-1 {
					return "", false
				}
			}
		}
		return lower, true
	}
	return "", false
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

	if body, ok := colorFunctionArgs(s, "rgb", "rgba"); ok {
		return parseRGBLike(body)
	}
	if body, ok := colorFunctionArgs(s, "hsl", "hsla"); ok {
		return parseHSLLike(body)
	}

	return Color{}, false
}

// colorFunctionArgs returns the argument list of a color function named by any
// of the given prefixes. rgb()/hsl() take an alpha argument in CSS Color 4, so
// rgba()/hsla() are aliases rather than separate syntaxes.
func colorFunctionArgs(s string, prefixes ...string) (string, bool) {
	lower := strings.ToLower(s)
	if !strings.HasSuffix(lower, ")") {
		return "", false
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p+"(") {
			return s[len(p)+1 : len(s)-1], true
		}
	}
	return "", false
}

// colorComponents splits a color function's arguments into three components and
// an optional alpha. It accepts both the legacy comma form (`r, g, b, a`) and
// the CSS Color 4 space form (`r g b / a`) because the two differ only in
// separator; dropping either one silently discards the whole declaration.
func colorComponents(body string) (vals []string, alpha string, hasAlpha bool, ok bool) {
	if i := strings.IndexByte(body, '/'); i >= 0 {
		alpha, hasAlpha = strings.TrimSpace(body[i+1:]), true
		body = body[:i]
	}
	vals = strings.Fields(strings.ReplaceAll(body, ",", " "))
	if !hasAlpha && len(vals) == 4 {
		alpha, hasAlpha, vals = vals[3], true, vals[:3]
	}
	if hasAlpha && len(strings.Fields(alpha)) != 1 {
		return nil, "", false, false
	}
	return vals, alpha, hasAlpha, len(vals) == 3
}

func alphaComponent(s string, present bool) uint8 {
	if !present {
		return 255
	}
	return clampByte(parseAlpha(s))
}

func parseRGBLike(body string) (Color, bool) {
	vals, alpha, hasAlpha, ok := colorComponents(body)
	if !ok {
		return Color{}, false
	}
	return Color{
		R: clampByte(parseComponent(vals[0])),
		G: clampByte(parseComponent(vals[1])),
		B: clampByte(parseComponent(vals[2])),
		A: alphaComponent(alpha, hasAlpha),
	}, true
}

func parseHSLLike(body string) (Color, bool) {
	vals, alpha, hasAlpha, ok := colorComponents(body)
	if !ok {
		return Color{}, false
	}
	r, g, b := hslToRGB(parseHueAngle(vals[0]), parsePercent(vals[1]), parsePercent(vals[2]))
	return Color{R: r, G: g, B: b, A: alphaComponent(alpha, hasAlpha)}, true
}

// parseHueAngle accepts a bare number, which CSS Color 4 reads as degrees, as
// well as the angle units the space form makes common.
func parseHueAngle(s string) float64 {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, unit := range []string{"grad", "turn", "deg", "rad"} {
		if !strings.HasSuffix(lower, unit) {
			continue
		}
		v := parseFloat(s[:len(s)-len(unit)])
		switch unit {
		case "grad":
			return v * 360 / 400
		case "turn":
			return v * 360
		case "rad":
			return v * 180 / math.Pi
		}
		return v
	}
	return parseFloat(s)
}

func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		return parseFloat(s[:len(s)-1]) / 100
	}
	return parseFloat(s)
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

func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	h = h / 360.0
	if h < 0 {
		h = 0
	}
	if h > 1 {
		h = 1
	}

	var r, g, b float64
	if s == 0 {
		r = l
		g = l
		b = l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueToRGB(p, q, h+1.0/3.0)
		g = hueToRGB(p, q, h)
		b = hueToRGB(p, q, h-1.0/3.0)
	}

	return clampByte(r * 255), clampByte(g * 255), clampByte(b * 255)
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
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
	"aliceblue":            {240, 248, 255, 255},
	"antiquewhite":         {250, 235, 215, 255},
	"aquamarine":           {127, 255, 212, 255},
	"azure":                {240, 255, 255, 255},
	"beige":                {245, 245, 220, 255},
	"bisque":               {255, 228, 196, 255},
	"blanchedalmond":       {255, 235, 205, 255},
	"blueviolet":           {138, 43, 226, 255},
	"burlywood":            {222, 184, 135, 255},
	"cadetblue":            {95, 158, 160, 255},
	"chartreuse":           {127, 255, 0, 255},
	"chocolate":            {210, 105, 30, 255},
	"cornflowerblue":       {100, 149, 237, 255},
	"cornsilk":             {255, 248, 220, 255},
	"darkcyan":             {0, 139, 139, 255},
	"darkgoldenrod":        {184, 134, 11, 255},
	"darkkhaki":            {189, 183, 107, 255},
	"darkmagenta":          {139, 0, 139, 255},
	"darkolivegreen":       {85, 107, 47, 255},
	"darkorange":           {255, 140, 0, 255},
	"darkorchid":           {153, 50, 204, 255},
	"darksalmon":           {233, 150, 122, 255},
	"darkseagreen":         {143, 188, 143, 255},
	"darkslateblue":        {72, 61, 139, 255},
	"darkslategray":        {47, 79, 79, 255},
	"darkslategrey":        {47, 79, 79, 255},
	"darkturquoise":        {0, 206, 209, 255},
	"darkviolet":           {148, 0, 211, 255},
	"deeppink":             {255, 20, 147, 255},
	"deepskyblue":          {0, 191, 255, 255},
	"dimgray":              {105, 105, 105, 255},
	"dimgrey":              {105, 105, 105, 255},
	"dodgerblue":           {30, 144, 255, 255},
	"firebrick":            {178, 34, 34, 255},
	"floralwhite":          {255, 250, 240, 255},
	"forestgreen":          {34, 139, 34, 255},
	"gainsboro":            {220, 220, 220, 255},
	"ghostwhite":           {248, 248, 255, 255},
	"goldenrod":            {218, 165, 32, 255},
	"greenyellow":          {173, 255, 47, 255},
	"honeydew":             {240, 255, 240, 255},
	"hotpink":              {255, 105, 180, 255},
	"indianred":            {205, 92, 92, 255},
	"lavenderblush":        {255, 240, 245, 255},
	"lawngreen":            {124, 252, 0, 255},
	"lemonchiffon":         {255, 250, 205, 255},
	"lightcoral":           {240, 128, 128, 255},
	"lightcyan":            {224, 255, 255, 255},
	"lightgoldenrodyellow": {250, 250, 210, 255},
	"lightpink":            {255, 182, 193, 255},
	"lightsalmon":          {255, 160, 122, 255},
	"lightseagreen":        {32, 178, 170, 255},
	"lightskyblue":         {135, 206, 250, 255},
	"lightslategray":       {119, 136, 153, 255},
	"lightslategrey":       {119, 136, 153, 255},
	"lightsteelblue":       {176, 196, 222, 255},
	"limegreen":            {50, 205, 50, 255},
	"linen":                {250, 240, 230, 255},
	"mediumaquamarine":     {102, 205, 170, 255},
	"mediumblue":           {0, 0, 205, 255},
	"mediumorchid":         {186, 85, 211, 255},
	"mediumpurple":         {147, 112, 219, 255},
	"mediumseagreen":       {60, 179, 113, 255},
	"mediumslateblue":      {123, 104, 238, 255},
	"mediumspringgreen":    {0, 250, 154, 255},
	"mediumturquoise":      {72, 209, 204, 255},
	"mediumvioletred":      {199, 21, 133, 255},
	"midnightblue":         {25, 25, 112, 255},
	"mintcream":            {245, 255, 250, 255},
	"mistyrose":            {255, 228, 225, 255},
	"moccasin":             {255, 228, 181, 255},
	"navajowhite":          {255, 222, 173, 255},
	"oldlace":              {253, 245, 230, 255},
	"olivedrab":            {107, 142, 35, 255},
	"orangered":            {255, 69, 0, 255},
	"orchid":               {218, 112, 214, 255},
	"palegoldenrod":        {238, 232, 170, 255},
	"palegreen":            {152, 251, 152, 255},
	"paleturquoise":        {178, 234, 238, 255},
	"palevioletred":        {219, 112, 147, 255},
	"papayawhip":           {255, 239, 213, 255},
	"peachpuff":            {255, 218, 185, 255},
	"peru":                 {205, 133, 63, 255},
	"plum":                 {221, 160, 221, 255},
	"powderblue":           {176, 224, 230, 255},
	"rebeccapurple":        {102, 51, 153, 255},
	"rosybrown":            {188, 143, 143, 255},
	"royalblue":            {65, 105, 225, 255},
	"saddlebrown":          {139, 69, 19, 255},
	"sandybrown":           {244, 164, 96, 255},
	"seagreen":             {46, 139, 87, 255},
	"seashell":             {255, 245, 238, 255},
	"sienna":               {160, 82, 45, 255},
	"slateblue":            {106, 90, 205, 255},
	"slategrey":            {112, 128, 144, 255},
	"springgreen":          {0, 255, 127, 255},
	"transparent":          {0, 0, 0, 0},
	"black":                {0, 0, 0, 255},
	"white":                {255, 255, 255, 255},
	"red":                  {255, 0, 0, 255},
	"green":                {0, 128, 0, 255},
	"blue":                 {0, 0, 255, 255},
	"yellow":               {255, 255, 0, 255},
	"cyan":                 {0, 255, 255, 255},
	"aqua":                 {0, 255, 255, 255},
	"magenta":              {255, 0, 255, 255},
	"fuchsia":              {255, 0, 255, 255},
	"silver":               {192, 192, 192, 255},
	"gray":                 {128, 128, 128, 255},
	"grey":                 {128, 128, 128, 255},
	"maroon":               {128, 0, 0, 255},
	"olive":                {128, 128, 0, 255},
	"lime":                 {0, 255, 0, 255},
	"teal":                 {0, 128, 128, 255},
	"navy":                 {0, 0, 128, 255},
	"orange":               {255, 165, 0, 255},
	"purple":               {128, 0, 128, 255},
	"pink":                 {255, 192, 203, 255},
	"brown":                {165, 42, 42, 255},
	"coral":                {255, 127, 80, 255},
	"crimson":              {220, 20, 60, 255},
	"darkblue":             {0, 0, 139, 255},
	"darkgray":             {169, 169, 169, 255},
	"darkgrey":             {169, 169, 169, 255},
	"darkgreen":            {0, 100, 0, 255},
	"darkred":              {139, 0, 0, 255},
	"gold":                 {255, 215, 0, 255},
	"indigo":               {75, 0, 130, 255},
	"ivory":                {255, 255, 240, 255},
	"khaki":                {240, 230, 140, 255},
	"lavender":             {230, 230, 250, 255},
	"lightblue":            {173, 216, 230, 255},
	"lightgray":            {211, 211, 211, 255},
	"lightgrey":            {211, 211, 211, 255},
	"lightgreen":           {144, 238, 144, 255},
	"lightyellow":          {255, 255, 224, 255},
	"tomato":               {255, 99, 71, 255},
	"salmon":               {250, 128, 114, 255},
	"skyblue":              {135, 206, 235, 255},
	"slategray":            {112, 128, 144, 255},
	"snow":                 {255, 250, 250, 255},
	"steelblue":            {70, 130, 180, 255},
	"tan":                  {210, 180, 140, 255},
	"thistle":              {216, 191, 216, 255},
	"turquoise":            {64, 224, 208, 255},
	"violet":               {238, 130, 238, 255},
	"wheat":                {245, 222, 179, 255},
	"whitesmoke":           {245, 245, 245, 255},
	"yellowgreen":          {154, 205, 50, 255},
}

// ToLength converts a value to a float32 length in pixels. Returns 0 for
// non-length values. Unit conversion uses CSS defaults: em/ex default to 16px,
// rem to 16px, pt to 1.333px, pc to 16px, in to 96px, cm to 37.795px, mm to
// 3.7795px.
func (v Value) ToLength() float32 {
	switch v.Type {
	case ValueFunc:
		if f, ok := EvalFunc(v.Func, v.Str, 0, 0, 0); ok {
			return f
		}
		return 0
	case ValueLength:
		return toPixels(v.Num, v.Unit)
	case ValueNumber:
		return float32(v.Num)
	case ValuePercentage:
		return -2 - float32(v.Num)
	}
	return 0
}

// ToLengthWithEm is like ToLength but resolves em, rem, and ex units against
// the provided font-size rather than the hardcoded 16px default. Use this for
// all non-font-size lengths (margin, padding, top/right/bottom/left, etc.) so
// that e.g. "0.67em" on an h1 with font-size:32px correctly yields 21.44px
// instead of 10.72px.
func (v Value) ToLengthWithEm(fontSize float32) float32 {
	return v.ToLengthWithContext(fontSize, 1440, 900)
}

// ToLengthWithContext resolves length units with full context: font size for
// em/rem/ex, and viewport width/height for vw/vh/vmin/vmax.
func (v Value) ToLengthWithContext(em, vw, vh float32) float32 {
	if vw <= 0 {
		vw = 1440
	}
	if vh <= 0 {
		vh = 900
	}
	switch v.Type {
	case ValueFunc:
		if f, ok := EvalFunc(v.Func, v.Str, em, vw, vh); ok {
			return f
		}
		return 0
	case ValueLength:
		switch v.Unit {
		case "vw", "svw", "dvw", "lvw":
			return float32(v.Num) * vw / 100
		case "vh", "svh", "dvh", "lvh":
			return float32(v.Num) * vh / 100
		case "vmin":
			m := vw
			if vh < m {
				m = vh
			}
			return float32(v.Num) * m / 100
		case "vmax":
			m := vw
			if vh > m {
				m = vh
			}
			return float32(v.Num) * m / 100
		case "em", "ex":
			if em > 0 {
				return float32(v.Num) * em
			}
			return float32(v.Num) * 16
		case "ch":
			// One `ch` is the advance of the font's `0`. Every face this engine
			// draws digits at sits near half the font size, and the measure has to
			// follow the element's own size: `max-width: 60ch` on a 20px heading is
			// 600px, not the 60px an untreated unit suffix used to yield.
			if em <= 0 {
				em = 16
			}
			return float32(v.Num) * em * 0.5
		case "rem":
			// rem is relative to the root element's font size, not the element's
			// own. Resolving it against `em` (as this once did) inflates every rem
			// on a large-font element: an h1 with font-size:40px and margin-top:
			// 6.5rem got 260px instead of 104px. The root defaults to 16px, which
			// is what the overwhelming majority of documents leave it at.
			return float32(v.Num) * 16
		default:
			return toPixels(v.Num, v.Unit)
		}
	case ValueNumber:
		return float32(v.Num)
	case ValuePercentage:
		return -2 - float32(v.Num)
	}
	return 0
}

func toPixels(num float64, unit string) float32 {
	switch unit {
	case "px", "":
		return float32(num)
	case "em", "rem", "ex":
		return float32(num * 16)
	case "ch":
		return float32(num * 8)
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
	case "vw":
		return float32(num * 1440 / 100)
	case "vh":
		return float32(num * 900 / 100)
	case "vmin":
		return float32(num * 900 / 100)
	case "vmax":
		return float32(num * 1440 / 100)
	}
	return float32(num)
}

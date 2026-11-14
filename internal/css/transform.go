package css

import (
	"math"
	"strings"
)

type TransformFunc struct {
	Name string
	Args []float32
}

type TransformOrigin struct {
	X, Y   float32
	XIsPct bool
	YIsPct bool
}

func DefaultTransformOrigin() TransformOrigin {
	return TransformOrigin{X: 50, Y: 50, XIsPct: true, YIsPct: true}
}

func ParseTransform(value string) []TransformFunc {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil
	}

	var funcs []TransformFunc
	i := 0
	for i < len(value) {
		for i < len(value) && isTransformSpace(value[i]) {
			i++
		}
		if i >= len(value) {
			break
		}

		nameStart := i
		for i < len(value) && value[i] != '(' {
			i++
		}
		if i >= len(value) {
			break
		}
		name := strings.ToLower(strings.TrimSpace(value[nameStart:i]))

		i++
		depth := 1
		argsStart := i
		for i < len(value) && depth > 0 {
			switch value[i] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth > 0 {
				i++
			}
		}
		argsStr := value[argsStart:i]
		if i < len(value) {
			i++
		}

		fn := parseOneTransformFunc(name, argsStr)
		if fn.Name != "" {
			funcs = append(funcs, fn)
		}
	}
	return funcs
}

func isTransformSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func parseOneTransformFunc(name, argsStr string) TransformFunc {
	args := splitTransformArgs(argsStr)

	switch name {
	case "translate":
		tx := parseTransformLength(args, 0)
		ty := parseTransformLength(args, 1)
		return TransformFunc{Name: "translate", Args: []float32{tx, ty}}
	case "translatex":
		tx := parseTransformLength(args, 0)
		return TransformFunc{Name: "translate", Args: []float32{tx, 0}}
	case "translatey":
		ty := parseTransformLength(args, 0)
		return TransformFunc{Name: "translate", Args: []float32{0, ty}}
	case "rotate":
		angle := parseTransformAngle(args, 0)
		return TransformFunc{Name: "rotate", Args: []float32{angle}}
	case "scale":
		sx := parseTransformNumber(args, 0)
		sy := sx
		if len(args) > 1 {
			sy = parseTransformNumber(args, 1)
		}
		return TransformFunc{Name: "scale", Args: []float32{sx, sy}}
	case "scalex":
		sx := parseTransformNumber(args, 0)
		return TransformFunc{Name: "scale", Args: []float32{sx, 1}}
	case "scaley":
		sy := parseTransformNumber(args, 0)
		return TransformFunc{Name: "scale", Args: []float32{1, sy}}
	case "skew":
		ax := parseTransformAngle(args, 0)
		ay := parseTransformAngle(args, 1)
		return TransformFunc{Name: "skew", Args: []float32{ax, ay}}
	case "skewx":
		ax := parseTransformAngle(args, 0)
		return TransformFunc{Name: "skew", Args: []float32{ax, 0}}
	case "skewy":
		ay := parseTransformAngle(args, 0)
		return TransformFunc{Name: "skew", Args: []float32{0, ay}}
	case "matrix":
		if len(args) < 6 {
			return TransformFunc{}
		}
		m := make([]float32, 6)
		for i := 0; i < 6; i++ {
			m[i] = float32(parseFloat(strings.TrimSpace(args[i])))
		}
		return TransformFunc{Name: "matrix", Args: m}
	}
	return TransformFunc{}
}

func splitTransformArgs(s string) []string {
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
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func parseTransformLength(args []string, idx int) float32 {
	if idx >= len(args) {
		return 0
	}
	s := strings.TrimSpace(args[idx])
	if s == "" {
		return 0
	}
	return parseTransformLengthValue(s)
}

func parseTransformLengthValue(s string) float32 {
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "px") {
		return float32(parseFloat(s[:len(s)-2]))
	}
	if strings.HasSuffix(lower, "em") || strings.HasSuffix(lower, "rem") {
		return float32(parseFloat(s[:len(s)-2])) * 16
	}
	if strings.HasSuffix(lower, "%") {
		return float32(parseFloat(s[:len(s)-1])) / 100
	}
	return float32(parseFloat(s))
}

func parseTransformAngle(args []string, idx int) float32 {
	if idx >= len(args) {
		return 0
	}
	s := strings.TrimSpace(args[idx])
	if s == "" {
		return 0
	}
	return parseAngleValue(s)
}

func parseAngleValue(s string) float32 {
	lower := strings.ToLower(s)
	if fn, ok := lengthFuncName(s); ok {
		inner := s[strings.IndexByte(s, '(')+1 : len(s)-1]
		if v, ok := EvalFunc(fn, inner, 16, 1440, 900); ok {
			return v
		}
	}
	if strings.HasSuffix(lower, "grad") {
		return float32(parseFloat(s[:len(s)-4])) * math.Pi / 200
	}
	if strings.HasSuffix(lower, "turn") {
		return float32(parseFloat(s[:len(s)-4])) * 2 * math.Pi
	}
	if strings.HasSuffix(lower, "deg") {
		return float32(parseFloat(s[:len(s)-3])) * math.Pi / 180
	}
	if strings.HasSuffix(lower, "rad") {
		return float32(parseFloat(s[:len(s)-3]))
	}
	return float32(parseFloat(s)) * math.Pi / 180
}

func parseTransformNumber(args []string, idx int) float32 {
	if idx >= len(args) {
		return 1
	}
	s := strings.TrimSpace(args[idx])
	if s == "" {
		return 1
	}
	return float32(parseFloat(s))
}

func ParseTransformOrigin(value string) TransformOrigin {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultTransformOrigin()
	}

	parts := strings.Fields(value)
	origin := DefaultTransformOrigin()

	parseX := func(s string) {
		lower := strings.ToLower(s)
		switch lower {
		case "left":
			origin.X = 0
			origin.XIsPct = true
		case "center":
			origin.X = 50
			origin.XIsPct = true
		case "right":
			origin.X = 100
			origin.XIsPct = true
		default:
			if strings.HasSuffix(lower, "%") {
				origin.X = float32(parseFloat(s[:len(s)-1]))
				origin.XIsPct = true
			} else {
				origin.X = parseTransformLengthValue(s)
				origin.XIsPct = false
			}
		}
	}

	parseY := func(s string) {
		lower := strings.ToLower(s)
		switch lower {
		case "top":
			origin.Y = 0
			origin.YIsPct = true
		case "center":
			origin.Y = 50
			origin.YIsPct = true
		case "bottom":
			origin.Y = 100
			origin.YIsPct = true
		default:
			if strings.HasSuffix(lower, "%") {
				origin.Y = float32(parseFloat(s[:len(s)-1]))
				origin.YIsPct = true
			} else {
				origin.Y = parseTransformLengthValue(s)
				origin.YIsPct = false
			}
		}
	}

	if len(parts) >= 1 {
		parseX(parts[0])
	}
	if len(parts) >= 2 {
		parseY(parts[1])
	} else {
		lower := strings.ToLower(parts[0])
		switch lower {
		case "top", "bottom":
			origin.X = 50
			origin.XIsPct = true
		default:
			origin.Y = 50
			origin.YIsPct = true
		}
	}

	return origin
}

func (o TransformOrigin) ResolveOrigin(elementWidth, elementHeight float32) (float32, float32) {
	ox := o.X
	if o.XIsPct {
		ox = o.X / 100 * elementWidth
	}
	oy := o.Y
	if o.YIsPct {
		oy = o.Y / 100 * elementHeight
	}
	return ox, oy
}

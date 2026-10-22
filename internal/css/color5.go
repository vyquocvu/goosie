package css

import (
	"math"
	"strconv"
	"strings"
)

// parseColorMix implements CSS Color Module Level 5 §3 "Mixing Colors: the color-mix() function".
// Syntax: color-mix(in <color-interpolation-method>, <color-percentage>, <color-percentage>)
func parseColorMix(body string) (Color, bool) {
	parts := splitArgs(body)
	if len(parts) != 3 {
		return Color{}, false
	}

	// 1. Parse interpolation method: "in <space> [<hue-method>]"
	inMethod := strings.TrimSpace(parts[0])
	lowerIn := strings.ToLower(inMethod)
	if !strings.HasPrefix(lowerIn, "in ") && !strings.HasPrefix(lowerIn, "in\t") {
		return Color{}, false
	}
	specFields := strings.Fields(strings.TrimSpace(inMethod[3:]))
	if len(specFields) == 0 {
		return Color{}, false
	}
	space := strings.ToLower(specFields[0])
	hueMethod := "shorter"
	if len(specFields) >= 2 {
		h := strings.ToLower(specFields[1])
		switch h {
		case "shorter", "longer", "increasing", "decreasing":
			hueMethod = h
		}
	}

	// 2. Parse color percentages
	c1, p1, hasP1, ok1 := parseColorPercentage(parts[1])
	c2, p2, hasP2, ok2 := parseColorPercentage(parts[2])
	if !ok1 || !ok2 {
		return Color{}, false
	}

	// 3. Normalize percentages
	normP1, normP2, alphaScale, ok := normalizePercentages(p1, p2, hasP1, hasP2)
	if !ok {
		return Color{}, false
	}

	// 4. Mix colors in the target color space
	return mixInColorSpace(space, hueMethod, c1, c2, normP1, normP2, alphaScale)
}

func parseColorPercentage(s string) (col Color, pct float64, hasPct bool, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Color{}, 0, false, false
	}

	// Case 1: "<color> <percentage>" (percentage at end outside parens)
	lastSpace := -1
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t':
			if depth == 0 {
				lastSpace = i
			}
		}
	}
	if lastSpace > 0 {
		tail := strings.TrimSpace(s[lastSpace+1:])
		head := strings.TrimSpace(s[:lastSpace])
		if strings.HasSuffix(tail, "%") {
			p, err := strconv.ParseFloat(strings.TrimSuffix(tail, "%"), 64)
			if err == nil {
				c, ok := ParseColor(head)
				if ok {
					return c, p, true, true
				}
			}
		}
	}

	// Case 2: "<percentage> <color>" (percentage at start outside parens)
	firstSpace := -1
	depth = 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t':
			if depth == 0 && firstSpace < 0 {
				firstSpace = i
			}
		}
	}
	if firstSpace > 0 {
		head := strings.TrimSpace(s[:firstSpace])
		tail := strings.TrimSpace(s[firstSpace+1:])
		if strings.HasSuffix(head, "%") {
			p, err := strconv.ParseFloat(strings.TrimSuffix(head, "%"), 64)
			if err == nil {
				c, ok := ParseColor(tail)
				if ok {
					return c, p, true, true
				}
			}
		}
	}

	// Case 3: No percentage specified
	c, ok := ParseColor(s)
	if ok {
		return c, 0, false, true
	}
	return Color{}, 0, false, false
}

func normalizePercentages(p1, p2 float64, has1, has2 bool) (float64, float64, float64, bool) {
	if !has1 && !has2 {
		return 0.5, 0.5, 1.0, true
	}
	if has1 && !has2 {
		norm1 := p1 / 100.0
		norm2 := 1.0 - norm1
		return norm1, norm2, 1.0, true
	}
	if !has1 && has2 {
		norm2 := p2 / 100.0
		norm1 := 1.0 - norm2
		return norm1, norm2, 1.0, true
	}

	sum := p1 + p2
	if sum <= 0 {
		return 0, 0, 0, false
	}
	norm1 := p1 / sum
	norm2 := p2 / sum
	alphaScale := 1.0
	if sum < 100.0 {
		alphaScale = sum / 100.0
	}
	return norm1, norm2, alphaScale, true
}

func mixInColorSpace(space, hueMethod string, c1, c2 Color, p1, p2, alphaScale float64) (Color, bool) {
	a1 := float64(c1.A) / 255.0
	a2 := float64(c2.A) / 255.0
	outA := a1*p1 + a2*p2
	if outA <= 1e-6 {
		return Color{R: 0, G: 0, B: 0, A: 0}, true
	}
	finalAlpha := clampByte(outA * alphaScale * 255.0)

	switch space {
	case "srgb":
		r1, g1, b1 := float64(c1.R)/255.0, float64(c1.G)/255.0, float64(c1.B)/255.0
		r2, g2, b2 := float64(c2.R)/255.0, float64(c2.G)/255.0, float64(c2.B)/255.0
		r := mixPremul(r1, a1, r2, a2, p1, p2, outA)
		g := mixPremul(g1, a1, g2, a2, p1, p2, outA)
		b := mixPremul(b1, a1, b2, a2, p1, p2, outA)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: finalAlpha}, true

	case "srgb-linear":
		lr1, lg1, lb1 := srgbDecode(float64(c1.R)/255.0), srgbDecode(float64(c1.G)/255.0), srgbDecode(float64(c1.B)/255.0)
		lr2, lg2, lb2 := srgbDecode(float64(c2.R)/255.0), srgbDecode(float64(c2.G)/255.0), srgbDecode(float64(c2.B)/255.0)
		lr := mixPremul(lr1, a1, lr2, a2, p1, p2, outA)
		lg := mixPremul(lg1, a1, lg2, a2, p1, p2, outA)
		lb := mixPremul(lb1, a1, lb2, a2, p1, p2, outA)
		r := clip01(srgbEncode(lr))
		g := clip01(srgbEncode(lg))
		b := clip01(srgbEncode(lb))
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: finalAlpha}, true

	case "oklab":
		L1, a_1, b_1 := srgbToOklab(c1)
		L2, a_2, b_2 := srgbToOklab(c2)
		L := mixPremul(L1, a1, L2, a2, p1, p2, outA)
		a := mixPremul(a_1, a1, a_2, a2, p1, p2, outA)
		b := mixPremul(b_1, a1, b_2, a2, p1, p2, outA)
		r, g, bl := oklabToSRGB(L, a, b)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "oklch":
		L1, C1, h1 := srgbToOklch(c1)
		L2, C2, h2 := srgbToOklch(c2)
		L := mixPremul(L1, a1, L2, a2, p1, p2, outA)
		C := mixPremul(C1, a1, C2, a2, p1, p2, outA)
		h := interpolateHue(h1, h2, p1, p2, hueMethod)
		r, g, bl := oklchToSRGB(L, C, h)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "lab":
		L1, a_1, b_1 := srgbToLab(c1)
		L2, a_2, b_2 := srgbToLab(c2)
		L := mixPremul(L1, a1, L2, a2, p1, p2, outA)
		a := mixPremul(a_1, a1, a_2, a2, p1, p2, outA)
		b := mixPremul(b_1, a1, b_2, a2, p1, p2, outA)
		r, g, bl := labToSRGB(L, a, b)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "lch":
		L1, C1, h1 := srgbToLch(c1)
		L2, C2, h2 := srgbToLch(c2)
		L := mixPremul(L1, a1, L2, a2, p1, p2, outA)
		C := mixPremul(C1, a1, C2, a2, p1, p2, outA)
		h := interpolateHue(h1, h2, p1, p2, hueMethod)
		r, g, bl := lchToSRGB(L, C, h)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "hsl":
		h1, s1, l1 := rgbToHSL(c1.R, c1.G, c1.B)
		h2, s2, l2 := rgbToHSL(c2.R, c2.G, c2.B)
		s := mixPremul(s1, a1, s2, a2, p1, p2, outA)
		l := mixPremul(l1, a1, l2, a2, p1, p2, outA)
		h := interpolateHue(h1, h2, p1, p2, hueMethod)
		rr, gg, bb := hslToRGB(h, s, l)
		return Color{R: rr, G: gg, B: bb, A: finalAlpha}, true

	case "hwb":
		h1, w1, b1 := srgbToHWB(c1)
		h2, w2, b2 := srgbToHWB(c2)
		w := mixPremul(w1, a1, w2, a2, p1, p2, outA)
		b := mixPremul(b1, a1, b2, a2, p1, p2, outA)
		h := interpolateHue(h1, h2, p1, p2, hueMethod)
		rr, gg, bb := hwbToRGB(h, w, b)
		return Color{R: rr, G: gg, B: bb, A: finalAlpha}, true

	case "display-p3":
		p1r, p1g, p1b := srgbToDisplayP3(c1)
		p2r, p2g, p2b := srgbToDisplayP3(c2)
		p3r := mixPremul(p1r, a1, p2r, a2, p1, p2, outA)
		p3g := mixPremul(p1g, a1, p2g, a2, p1, p2, outA)
		p3b := mixPremul(p1b, a1, p2b, a2, p1, p2, outA)
		r, g, bl, ok := colorSpaceToSRGB("display-p3", p3r, p3g, p3b)
		if !ok {
			return Color{}, false
		}
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "xyz", "xyz-d65":
		x1, y1, z1 := srgbToXYZD65(c1)
		x2, y2, z2 := srgbToXYZD65(c2)
		x := mixPremul(x1, a1, x2, a2, p1, p2, outA)
		y := mixPremul(y1, a1, y2, a2, p1, p2, outA)
		z := mixPremul(z1, a1, z2, a2, p1, p2, outA)
		r, g, bl, ok := colorSpaceToSRGB("xyz-d65", x, y, z)
		if !ok {
			return Color{}, false
		}
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	case "xyz-d50":
		x1, y1, z1 := srgbToXYZD50(c1)
		x2, y2, z2 := srgbToXYZD50(c2)
		x := mixPremul(x1, a1, x2, a2, p1, p2, outA)
		y := mixPremul(y1, a1, y2, a2, p1, p2, outA)
		z := mixPremul(z1, a1, z2, a2, p1, p2, outA)
		r, g, bl, ok := colorSpaceToSRGB("xyz-d50", x, y, z)
		if !ok {
			return Color{}, false
		}
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(bl * 255.0), A: finalAlpha}, true

	default:
		return Color{}, false
	}
}

func mixPremul(v1, a1, v2, a2, p1, p2, outA float64) float64 {
	if a1 <= 1e-6 {
		return v2
	}
	if a2 <= 1e-6 {
		return v1
	}
	return (v1*a1*p1 + v2*a2*p2) / outA
}

func interpolateHue(h1, h2, p1, p2 float64, method string) float64 {
	if math.IsNaN(h1) {
		return h2
	}
	if math.IsNaN(h2) {
		return h1
	}
	h1 = math.Mod(h1, 360)
	if h1 < 0 {
		h1 += 360
	}
	h2 = math.Mod(h2, 360)
	if h2 < 0 {
		h2 += 360
	}

	switch method {
	case "longer":
		diff := h2 - h1
		if 0 < diff && diff < 180 {
			h1 += 360
		} else if -180 < diff && diff <= 0 {
			h2 += 360
		}
	case "increasing":
		if h2 < h1 {
			h2 += 360
		}
	case "decreasing":
		if h1 < h2 {
			h1 += 360
		}
	default: // "shorter"
		diff := h2 - h1
		if diff > 180 {
			h1 += 360
		} else if diff < -180 {
			h2 += 360
		}
	}

	res := h1*p1 + h2*p2
	res = math.Mod(res, 360)
	if res < 0 {
		res += 360
	}
	return res
}

// --- Space conversion helpers ---

func srgbToXYZD65(c Color) (float64, float64, float64) {
	lr := srgbDecode(float64(c.R) / 255.0)
	lg := srgbDecode(float64(c.G) / 255.0)
	lb := srgbDecode(float64(c.B) / 255.0)
	return linearSRGBToXYZD65(lr, lg, lb)
}

func linearSRGBToXYZD65(r, g, b float64) (float64, float64, float64) {
	return 0.41239079926595934*r + 0.357584339383878*g + 0.1804807884018343*b,
		0.21263900587151027*r + 0.715168678767756*g + 0.07219231536073371*b,
		0.01933081871559182*r + 0.1191947797946259*g + 0.9505321522496607*b
}

func srgbToXYZD50(c Color) (float64, float64, float64) {
	x, y, z := srgbToXYZD65(c)
	return bradfordD65ToD50(x, y, z)
}

func bradfordD65ToD50(x, y, z float64) (float64, float64, float64) {
	return 1.0478112*x + 0.0228866*y - 0.0501270*z,
		0.0295424*x + 0.9904844*y - 0.0170491*z,
		-0.0092345*x + 0.0150436*y + 0.7521316*z
}

func srgbToOklab(c Color) (float64, float64, float64) {
	lr := srgbDecode(float64(c.R) / 255.0)
	lg := srgbDecode(float64(c.G) / 255.0)
	lb := srgbDecode(float64(c.B) / 255.0)

	l := 0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb
	m := 0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb
	s := 0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb

	l_ := math.Cbrt(l)
	m_ := math.Cbrt(m)
	s_ := math.Cbrt(s)

	L := 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_
	a := 1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_
	b := 0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_
	return L, a, b
}

func srgbToOklch(c Color) (float64, float64, float64) {
	L, a, b := srgbToOklab(c)
	C := math.Hypot(a, b)
	h := math.Atan2(b, a) * 180.0 / math.Pi
	if h < 0 {
		h += 360.0
	}
	if C < 1e-5 {
		h = math.NaN()
	}
	return L, C, h
}

func srgbToLab(c Color) (float64, float64, float64) {
	x, y, z := srgbToXYZD50(c)
	const d50x, d50z = 0.96422, 0.82521
	fx := labF(x / d50x)
	fy := labF(y)
	fz := labF(z / d50z)
	L := 116.0*fy - 16.0
	a := 500.0 * (fx - fy)
	b := 200.0 * (fy - fz)
	return L, a, b
}

func labF(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Cbrt(t)
	}
	return t/(3.0*delta*delta) + 4.0/29.0
}

func srgbToLch(c Color) (float64, float64, float64) {
	L, a, b := srgbToLab(c)
	C := math.Hypot(a, b)
	h := math.Atan2(b, a) * 180.0 / math.Pi
	if h < 0 {
		h += 360.0
	}
	if C < 1e-5 {
		h = math.NaN()
	}
	return L, C, h
}

func rgbToHSL(r8, g8, b8 uint8) (float64, float64, float64) {
	r := float64(r8) / 255.0
	g := float64(g8) / 255.0
	b := float64(b8) / 255.0
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l := (max + min) / 2.0
	if max == min {
		return math.NaN(), 0, l
	}
	d := max - min
	var s float64
	if l > 0.5 {
		s = d / (2.0 - max - min)
	} else {
		s = d / (max + min)
	}
	var h float64
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6.0
		}
	case g:
		h = (b-r)/d + 2.0
	case b:
		h = (r-g)/d + 4.0
	}
	h *= 60.0
	return h, s, l
}

func srgbToHWB(c Color) (float64, float64, float64) {
	h, _, _ := rgbToHSL(c.R, c.G, c.B)
	w := math.Min(float64(c.R), math.Min(float64(c.G), float64(c.B))) / 255.0
	b := 1.0 - math.Max(float64(c.R), math.Max(float64(c.G), float64(c.B)))/255.0
	return h, w, b
}

func srgbToDisplayP3(c Color) (float64, float64, float64) {
	x, y, z := srgbToXYZD65(c)
	lr := 2.493496911941425*x - 0.9313836179191239*y - 0.40271078445071684*z
	lg := -0.8294889695615747*x + 1.7626640603183463*y + 0.02362468584294362*z
	lb := 0.03584583024378447*x - 0.07617238926804182*y + 0.9568845240076872*z
	return clip01(srgbEncode(lr)), clip01(srgbEncode(lg)), clip01(srgbEncode(lb))
}

// --- Relative Color Syntax (CSS Color 5 §4) ---

func isRelativeColor(body string) bool {
	trim := strings.TrimSpace(body)
	lower := strings.ToLower(trim)
	return strings.HasPrefix(lower, "from ") || strings.HasPrefix(lower, "from\t")
}

func parseRelativeColor(fn, body, colorScheme string) (Color, bool) {
	trim := strings.TrimSpace(body)
	lower := strings.ToLower(trim)
	if !strings.HasPrefix(lower, "from ") && !strings.HasPrefix(lower, "from\t") {
		return Color{}, false
	}
	afterFrom := strings.TrimSpace(trim[4:])

	// Extract origin color: read until first whitespace at depth 0
	depth := 0
	originEnd := -1
	for i := 0; i < len(afterFrom); i++ {
		switch afterFrom[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t':
			if depth == 0 {
				originEnd = i
				break
			}
		}
		if originEnd > 0 {
			break
		}
	}
	if originEnd < 0 {
		return Color{}, false
	}
	originColorStr := strings.TrimSpace(afterFrom[:originEnd])
	channelsStr := strings.TrimSpace(afterFrom[originEnd+1:])

	originColor, ok := ParseColorWithScheme(originColorStr, colorScheme)
	if !ok {
		return Color{}, false
	}

	// Check for "/ alpha"
	alphaExpr := ""
	hasAlpha := false
	depth = 0
	for i := 0; i < len(channelsStr); i++ {
		switch channelsStr[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '/':
			if depth == 0 {
				alphaExpr = strings.TrimSpace(channelsStr[i+1:])
				channelsStr = strings.TrimSpace(channelsStr[:i])
				hasAlpha = true
				break
			}
		}
		if hasAlpha {
			break
		}
	}

	chanExprs := splitTokensOutsideParens(channelsStr)

	fn = strings.ToLower(fn)
	vars := make(map[string]float64)
	originAlpha := float64(originColor.A) / 255.0
	vars["alpha"] = originAlpha
	vars["a"] = originAlpha

	switch fn {
	case "rgb", "rgba":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		vars["r"] = float64(originColor.R)
		vars["g"] = float64(originColor.G)
		vars["b"] = float64(originColor.B)
		r, ok1 := evalChannelExpr(chanExprs[0], vars)
		g, ok2 := evalChannelExpr(chanExprs[1], vars)
		b, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			av, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = av
		}
		return Color{R: clampByte(r), G: clampByte(g), B: clampByte(b), A: clampByte(a * 255.0)}, true

	case "hsl", "hsla":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		h, s, l := rgbToHSL(originColor.R, originColor.G, originColor.B)
		if math.IsNaN(h) {
			h = 0
		}
		vars["h"] = h
		vars["s"] = s * 100.0
		vars["l"] = l * 100.0
		hv, ok1 := evalChannelExpr(chanExprs[0], vars)
		sv, ok2 := evalChannelExpr(chanExprs[1], vars)
		lv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			av, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = av
		}
		rr, gg, bb := hslToRGB(hv, sv/100.0, lv/100.0)
		return Color{R: rr, G: gg, B: bb, A: clampByte(a * 255.0)}, true

	case "oklch":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		L, C, h := srgbToOklch(originColor)
		if math.IsNaN(h) {
			h = 0
		}
		vars["l"] = L
		vars["c"] = C
		vars["h"] = h
		lv, ok1 := evalChannelExpr(chanExprs[0], vars)
		cv, ok2 := evalChannelExpr(chanExprs[1], vars)
		hv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			av, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = av
		}
		r, g, b := oklchToSRGB(lv, cv, hv)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: clampByte(a * 255.0)}, true

	case "oklab":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		L, a_val, b_val := srgbToOklab(originColor)
		vars["l"] = L
		vars["a"] = a_val
		vars["b"] = b_val
		lv, ok1 := evalChannelExpr(chanExprs[0], vars)
		av, ok2 := evalChannelExpr(chanExprs[1], vars)
		bv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			alv, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = alv
		}
		r, g, b := oklabToSRGB(lv, av, bv)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: clampByte(a * 255.0)}, true

	case "lab":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		L, a_val, b_val := srgbToLab(originColor)
		vars["l"] = L
		vars["a"] = a_val
		vars["b"] = b_val
		lv, ok1 := evalChannelExpr(chanExprs[0], vars)
		av, ok2 := evalChannelExpr(chanExprs[1], vars)
		bv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			alv, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = alv
		}
		r, g, b := labToSRGB(lv, av, bv)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: clampByte(a * 255.0)}, true

	case "lch":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		L, C, h := srgbToLch(originColor)
		if math.IsNaN(h) {
			h = 0
		}
		vars["l"] = L
		vars["c"] = C
		vars["h"] = h
		lv, ok1 := evalChannelExpr(chanExprs[0], vars)
		cv, ok2 := evalChannelExpr(chanExprs[1], vars)
		hv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			alv, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = alv
		}
		r, g, b := lchToSRGB(lv, cv, hv)
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: clampByte(a * 255.0)}, true

	case "hwb":
		if len(chanExprs) != 3 {
			return Color{}, false
		}
		h, w, b := srgbToHWB(originColor)
		if math.IsNaN(h) {
			h = 0
		}
		vars["h"] = h
		vars["w"] = w * 100.0
		vars["b"] = b * 100.0
		hv, ok1 := evalChannelExpr(chanExprs[0], vars)
		wv, ok2 := evalChannelExpr(chanExprs[1], vars)
		bv, ok3 := evalChannelExpr(chanExprs[2], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			alv, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = alv
		}
		rr, gg, bb := hwbToRGB(hv, wv/100.0, bv/100.0)
		return Color{R: rr, G: gg, B: bb, A: clampByte(a * 255.0)}, true

	case "color":
		// color(from <color> <space> <c1> <c2> <c3> [ / <a> ])
		if len(chanExprs) != 4 {
			return Color{}, false
		}
		space := strings.ToLower(chanExprs[0])
		p1, p2, p3 := srgbToDisplayP3(originColor)
		vars["r"] = p1
		vars["g"] = p2
		vars["b"] = p3
		v1, ok1 := evalChannelExpr(chanExprs[1], vars)
		v2, ok2 := evalChannelExpr(chanExprs[2], vars)
		v3, ok3 := evalChannelExpr(chanExprs[3], vars)
		if !ok1 || !ok2 || !ok3 {
			return Color{}, false
		}
		a := originAlpha
		if hasAlpha {
			alv, ok := evalChannelExpr(alphaExpr, vars)
			if !ok {
				return Color{}, false
			}
			a = alv
		}
		r, g, b, ok := colorSpaceToSRGB(space, v1, v2, v3)
		if !ok {
			return Color{}, false
		}
		return Color{R: clampByte(r * 255.0), G: clampByte(g * 255.0), B: clampByte(b * 255.0), A: clampByte(a * 255.0)}, true

	default:
		return Color{}, false
	}
}

func evalChannelExpr(expr string, vars map[string]float64) (float64, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	if strings.EqualFold(expr, "none") {
		return 0, true
	}
	// Check direct variable match
	if v, ok := vars[strings.ToLower(expr)]; ok {
		return v, true
	}
	// Direct literal percent or number
	if strings.HasSuffix(expr, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(expr, "%"), 64)
		if err == nil {
			return f, true
		}
	}
	if strings.HasSuffix(expr, "deg") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(expr, "deg"), 64)
		if err == nil {
			return f, true
		}
	}
	if f, err := strconv.ParseFloat(expr, 64); err == nil {
		return f, true
	}

	// If calc(...)
	if strings.HasPrefix(strings.ToLower(expr), "calc(") && strings.HasSuffix(expr, ")") {
		inner := strings.TrimSpace(expr[5 : len(expr)-1])
		return evalCalcExpr(inner, vars)
	}

	return 0, false
}

func evalCalcExpr(s string, vars map[string]float64) (float64, bool) {
	tokens := tokenizeCalc(s)
	if len(tokens) == 0 {
		return 0, false
	}
	p := &calcParser{tokens: tokens, pos: 0, vars: vars}
	val, ok := p.parseExpr()
	if !ok || p.pos < len(tokens) {
		return 0, false
	}
	return val, true
}

func tokenizeCalc(s string) []string {
	var tokens []string
	i := 0
	for i < len(s) {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		if ch == '(' || ch == ')' || ch == '+' || ch == '-' || ch == '*' || ch == '/' {
			tokens = append(tokens, string(ch))
			i++
			continue
		}
		start := i
		for i < len(s) {
			c := s[i]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || c == '+' || c == '-' || c == '*' || c == '/' {
				break
			}
			i++
		}
		tokens = append(tokens, s[start:i])
	}
	return tokens
}

type calcParser struct {
	tokens []string
	pos    int
	vars   map[string]float64
}

func (p *calcParser) parseExpr() (float64, bool) {
	v, ok := p.parseTerm()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.tokens) {
		op := p.tokens[p.pos]
		if op != "+" && op != "-" {
			break
		}
		p.pos++
		rhs, ok := p.parseTerm()
		if !ok {
			return 0, false
		}
		if op == "+" {
			v += rhs
		} else {
			v -= rhs
		}
	}
	return v, true
}

func (p *calcParser) parseTerm() (float64, bool) {
	v, ok := p.parseFactor()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.tokens) {
		op := p.tokens[p.pos]
		if op != "*" && op != "/" {
			break
		}
		p.pos++
		rhs, ok := p.parseFactor()
		if !ok {
			return 0, false
		}
		if op == "*" {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, false
			}
			v /= rhs
		}
	}
	return v, true
}

func (p *calcParser) parseFactor() (float64, bool) {
	if p.pos >= len(p.tokens) {
		return 0, false
	}
	tok := p.tokens[p.pos]
	p.pos++

	if tok == "(" {
		v, ok := p.parseExpr()
		if !ok {
			return 0, false
		}
		if p.pos >= len(p.tokens) || p.tokens[p.pos] != ")" {
			return 0, false
		}
		p.pos++
		return v, true
	}
	if tok == "-" {
		v, ok := p.parseFactor()
		return -v, ok
	}
	if tok == "+" {
		return p.parseFactor()
	}

	if v, ok := p.vars[strings.ToLower(tok)]; ok {
		return v, true
	}

	clean := tok
	if strings.HasSuffix(clean, "%") {
		clean = strings.TrimSuffix(clean, "%")
	}
	if strings.HasSuffix(clean, "deg") {
		clean = strings.TrimSuffix(clean, "deg")
	}
	f, err := strconv.ParseFloat(clean, 64)
	if err == nil {
		return f, true
	}
	return 0, false
}

func splitTokensOutsideParens(s string) []string {
	var tokens []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '(':
			depth++
			if start == -1 {
				start = i
			}
		case ')':
			depth--
		case ' ', '\t':
			if depth == 0 && start != -1 {
				tokens = append(tokens, strings.TrimSpace(s[start:i]))
				start = -1
			}
		default:
			if start == -1 {
				start = i
			}
		}
	}
	if start != -1 {
		tokens = append(tokens, strings.TrimSpace(s[start:]))
	}
	return tokens
}

// parseLightDark implements CSS Color Module Level 5 §5 "light-dark() color function".
func parseLightDark(body, colorScheme string) (Color, bool) {
	parts := splitArgs(body)
	if len(parts) != 2 {
		return Color{}, false
	}
	choice := strings.TrimSpace(parts[0])
	if strings.EqualFold(strings.TrimSpace(colorScheme), "dark") {
		choice = strings.TrimSpace(parts[1])
	}
	return ParseColorWithScheme(choice, colorScheme)
}

package css

// CSS Color 4 predefined spaces, resolved one way into sRGB.
//
// Every function below answers "what sRGB does this color display as": the
// test and reference renders the WPT suite compares are both sRGB, so the
// engine converts each foreign space through XYZ (adapting D50 sources to
// D65) into linear sRGB, applies the sRGB transfer curve, and clips into
// gamut. Clipping is not the spec's Oklch chroma-reduction gamut mapping -
// an out-of-gamut color lands on the gamut boundary along its channel axes
// rather than along hue - but every WPT swatch asserted here sits close
// enough that the boundary point matches within tolerance, and a clipped
// color is honest rendering where the old code dropped the declaration and
// fell back to whatever came before it (usually red).
//
// Not here yet, by design: color-mix() and contrast-color(), which need the
// inverse direction (sRGB back into working spaces) and, for contrast, style
// query support in the cascade.

import (
	"math"
	"strings"
)

// parseColor4Like resolves hwb(), lab(), lch(), oklab() and oklch(). The
// caller already matched the function name; body holds its arguments.
func parseColor4Like(fn, body string) (Color, bool) {
	vals, alpha, hasAlpha, ok := colorComponents(body)
	if !ok {
		return Color{}, false
	}
	a := alphaComponent(alpha, hasAlpha)
	var r, g, b float64
	switch strings.ToLower(fn) {
	case "hwb":
		if len(vals) != 3 {
			return Color{}, false
		}
		rr, gg, bb := hwbToRGB(parseHueAngle(vals[0]), parsePercent(vals[1]), parsePercent(vals[2]))
		r, g, b = float64(rr)/255, float64(gg)/255, float64(bb)/255
	case "lab":
		if len(vals) != 3 {
			return Color{}, false
		}
		r, g, b = labToSRGB(parseLabLightness(vals[0]), parseFloat(vals[1]), parseFloat(vals[2]))
	case "lch":
		if len(vals) != 3 {
			return Color{}, false
		}
		r, g, b = lchToSRGB(parseLabLightness(vals[0]), parseFloat(vals[1]), parseHueAngle(vals[2]))
	case "oklab":
		if len(vals) != 3 {
			return Color{}, false
		}
		r, g, b = oklabToSRGB(parseOKLightness(vals[0]), parseFloat(vals[1]), parseFloat(vals[2]))
	case "oklch":
		if len(vals) != 3 {
			return Color{}, false
		}
		r, g, b = oklchToSRGB(parseOKLightness(vals[0]), parseFloat(vals[1]), parseHueAngle(vals[2]))
	default:
		return Color{}, false
	}
	return Color{R: clampByte(r * 255), G: clampByte(g * 255), B: clampByte(b * 255), A: a}, true
}

// parseColorFunc resolves color(<space> <c1> <c2> <c3> [/ alpha]).
func parseColorFunc(body string) (Color, bool) {
	alpha := ""
	hasAlpha := false
	if i := strings.IndexByte(body, '/'); i >= 0 {
		alpha, hasAlpha = strings.TrimSpace(body[i+1:]), true
		body = body[:i]
	}
	parts := strings.Fields(body)
	if hasAlpha && len(strings.Fields(alpha)) != 1 {
		return Color{}, false
	}
	if len(parts) != 4 {
		return Color{}, false
	}
	space, c1, c2, c3 := strings.ToLower(parts[0]), parts[1], parts[2], parts[3]
	a := alphaComponent(alpha, hasAlpha)
	num := func(s string) (float64, bool) {
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "%") {
			f, ok := parseFloat64(strings.TrimSuffix(s, "%"))
			if !ok {
				return 0, false
			}
			return f / 100, true
		}
		return parseFloat64(s)
	}
	v1, ok1 := num(c1)
	v2, ok2 := num(c2)
	v3, ok3 := num(c3)
	if !ok1 || !ok2 || !ok3 {
		return Color{}, false
	}
	r, g, b, ok := colorSpaceToSRGB(space, v1, v2, v3)
	if !ok {
		return Color{}, false
	}
	return Color{R: clampByte(r * 255), G: clampByte(g * 255), B: clampByte(b * 255), A: a}, true
}

// parseLabLightness reads an L in 0..100, as number or percent.
func parseLabLightness(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "none" {
		return 0
	}
	if strings.HasSuffix(s, "%") {
		return parseFloat(s[:len(s)-1])
	}
	return parseFloat(s)
}

// parseOKLightness reads an Oklab L in 0..1, as number or percent.
func parseOKLightness(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "none" {
		return 0
	}
	if strings.HasSuffix(s, "%") {
		return parseFloat(s[:len(s)-1]) / 100
	}
	return parseFloat(s)
}

// --- transfer curves and matrices (CSS Color 4, D65 unless noted) ---

func srgbEncode(linear float64) float64 {
	if linear <= 0.0031308 {
		return 12.92 * linear
	}
	return 1.055*math.Pow(linear, 1/2.4) - 0.055
}

func srgbDecode(encoded float64) float64 {
	if encoded <= 0.04045 {
		return encoded / 12.92
	}
	return math.Pow((encoded+0.055)/1.055, 2.4)
}

// xyzD65ToLinearSRGB is the CSS Color 4 XYZ-D65 to linear-sRGB matrix.
func xyzD65ToLinearSRGB(x, y, z float64) (float64, float64, float64) {
	return 3.2404542*x - 1.5371385*y - 0.4985314*z,
		-0.9692660*x + 1.8760108*y + 0.0415560*z,
		0.0556434*x - 0.2040259*y + 1.0572252*z
}

// bradfordD50ToD65 adapts a D50 tristimulus to D65.
func bradfordD50ToD65(x, y, z float64) (float64, float64, float64) {
	return 0.9555766*x - 0.0230393*y + 0.0631636*z,
		-0.0282895*x + 1.0099416*y + 0.0210077*z,
		0.0122982*x - 0.0204830*y + 1.3299098*z
}

func clip01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// hwbToRGB follows CSS Color 4 §10: pure hue, then whiten then blacken.
func hwbToRGB(hueDeg, w, b float64) (uint8, uint8, uint8) {
	rr, gg, bb := hslToRGB(hueDeg, 1, 0.5)
	r, g, bl := float64(rr)/255, float64(gg)/255, float64(bb)/255
	if w+b >= 1 {
		gray := w / (w + b)
		v := uint8(gray*255 + 0.5)
		return v, v, v
	}
	r = r*(1-w-b) + w
	g = g*(1-w-b) + w
	bl = bl*(1-w-b) + w
	return clampByte(r * 255), clampByte(g * 255), clampByte(bl * 255)
}

// labToSRGB converts CIE L*a*b* (D50) to gamma-encoded sRGB 0..1.
func labToSRGB(l, a, b float64) (float64, float64, float64) {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	const d50x, d50z = 0.96422, 0.82521
	x := d50x * labFInv(fx)
	y := labFInv(fy)
	z := d50z * labFInv(fz)
	x, y, z = bradfordD50ToD65(x, y, z)
	lr, lg, lb := xyzD65ToLinearSRGB(x, y, z)
	return clip01(srgbEncode(lr)), clip01(srgbEncode(lg)), clip01(srgbEncode(lb))
}

func labFInv(t float64) float64 {
	if t > 6.0/29.0 {
		return t * t * t
	}
	return 3 * 6.0 / 29.0 * 6.0 / 29.0 * (t - 4.0/29.0)
}

// lchToSRGB converts CIE L*C*h (D50) via Lab.
func lchToSRGB(l, c, hueDeg float64) (float64, float64, float64) {
	rad := hueDeg * math.Pi / 180
	return labToSRGB(l, c*math.Cos(rad), c*math.Sin(rad))
}

// oklabToSRGB converts Oklab to gamma-encoded sRGB 0..1.
func oklabToSRGB(l, a, b float64) (float64, float64, float64) {
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	l, m, s := l_*l_*l_, m_*m_*m_, s_*s_*s_
	lr := 4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	lg := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	lb := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return clip01(srgbEncode(lr)), clip01(srgbEncode(lg)), clip01(srgbEncode(lb))
}

// oklchToSRGB converts Oklch via Oklab.
func oklchToSRGB(l, c, hueDeg float64) (float64, float64, float64) {
	rad := hueDeg * math.Pi / 180
	return oklabToSRGB(l, c*math.Cos(rad), c*math.Sin(rad))
}

// colorSpaceToSRGB resolves a color() colorspace id with components already
// normalized (percentages as 0..1 fractions, per the space's reference
// range). Unknown spaces report false and the declaration drops, as before.
func colorSpaceToSRGB(space string, c1, c2, c3 float64) (float64, float64, float64, bool) {
	var x, y, z float64
	switch space {
	case "srgb":
		return clip01(c1), clip01(c2), clip01(c3), true
	case "srgb-linear":
		return clip01(srgbEncode(c1)), clip01(srgbEncode(c2)), clip01(srgbEncode(c3)), true
	case "display-p3":
		r, g, b := srgbDecode(clip01(c1)), srgbDecode(clip01(c2)), srgbDecode(clip01(c3))
		x = 0.4865709*r + 0.2656677*g + 0.1982173*b
		y = 0.2289746*r + 0.6917385*g + 0.0792869*b
		z = 0.0000000*r + 0.0451134*g + 1.0439444*b
	case "display-p3-linear":
		// Same primaries as display-p3, but the components arrive linear.
		x = 0.4865709*c1 + 0.2656677*c2 + 0.1982173*c3
		y = 0.2289746*c1 + 0.6917385*c2 + 0.0792869*c3
		z = 0.0000000*c1 + 0.0451134*c2 + 1.0439444*c3
	case "a98-rgb":
		r, g, b := math.Pow(clip01(c1), 563.0/256.0), math.Pow(clip01(c2), 563.0/256.0), math.Pow(clip01(c3), 563.0/256.0)
		x = 0.5766690*r + 0.1855580*g + 0.1882289*b
		y = 0.2973450*r + 0.6273636*g + 0.0752913*b
		z = 0.0270313*r + 0.0706892*g + 0.9911085*b
	case "prophoto-rgb":
		r, g, b := prophotoDecode(c1), prophotoDecode(c2), prophotoDecode(c3)
		x = 0.7976749*r + 0.1351917*g + 0.0313534*b
		y = 0.2880402*r + 0.7118741*g + 0.0000857*b
		z = 0.0000000*r + 0.0000000*g + 0.8252100*b
		x, y, z = bradfordD50ToD65(x, y, z)
	case "rec2020":
		r, g, b := rec2020Decode(c1), rec2020Decode(c2), rec2020Decode(c3)
		x = 0.6369580*r + 0.1446170*g + 0.1688809*b
		y = 0.2627000*r + 0.6779980*g + 0.0593020*b
		z = 0.0000000*r + 0.0280730*g + 1.0609850*b
	case "xyz", "xyz-d65":
		x, y, z = c1, c2, c3
	case "xyz-d50":
		x, y, z = bradfordD50ToD65(c1, c2, c3)
	default:
		return 0, 0, 0, false
	}
	lr, lg, lb := xyzD65ToLinearSRGB(x, y, z)
	return clip01(srgbEncode(lr)), clip01(srgbEncode(lg)), clip01(srgbEncode(lb)), true
}

// prophotoDecode inverts the ProPhoto RGB transfer curve.
func prophotoDecode(v float64) float64 {
	v = clip01(v)
	if v < 16.0/512.0 {
		return v / 16
	}
	return math.Pow(v, 1.8)
}

// rec2020Decode inverts the Rec.2020 OETF.
func rec2020Decode(v float64) float64 {
	v = clip01(v)
	if v < 0.08145 {
		return v / 4.5
	}
	return math.Pow((v+0.099296)/1.099296, 1/0.45)
}

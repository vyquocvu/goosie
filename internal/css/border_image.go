package css

import (
	"strings"
)

// BorderImageRepeat is one axis of border-image-repeat.
type BorderImageRepeat uint8

const (
	BiRepeatStretch BorderImageRepeat = iota
	BiRepeatRepeat
	BiRepeatRound
	BiRepeatSpace
)

// BorderImageWidthUnit says how one border-image-width edge resolves.
type BorderImageWidthUnit uint8

const (
	// BiWidthAuto uses the intrinsic size of the slice region.
	BiWidthAuto BorderImageWidthUnit = iota
	// BiWidthNumber multiplies the corresponding border-width.
	BiWidthNumber
	// BiWidthLength is an absolute CSS-px length.
	BiWidthLength
)

// BorderImage is one parsed border-image declaration set. Slice edges are
// numbers (source px) or percentages of the source dimensions; Width edges
// resolve against the border widths (number), absolute lengths, or the slice
// intrinsics (auto). Slice/Width/Outset follow CSS TRBL order.
type BorderImage struct {
	// Source is the url() payload; empty means no image (border-image-source:
	// none or unparseable).
	Source string
	// HasSlice reports an explicit border-image-slice (the initial 100%
	// behaves the same, but explicitness gates nothing here).
	HasSlice bool
	Slice    [4]float32
	// SlicePct marks percentage edges; number edges are source px. Fill
	// paints the middle region; without it the middle stays transparent.
	SlicePct [4]bool
	Fill     bool
	// HasWidth reports an explicit border-image-width (initial 1, i.e. one
	// border-width, paints the same when width is uniform).
	HasWidth bool
	Width    [4]float32
	WidthU   [4]BorderImageWidthUnit
	// WidthPct marks percentage edges, resolved against the border-image
	// area at paint time (stored alongside a Length unit).
	WidthPct  [4]bool
	HasOutset bool
	Outset    [4]float32
	// OutsetN marks number edges (multiples of the border width); length
	// edges are CSS px.
	OutsetN [4]bool
	// HasRepeat marks an explicit border-image-repeat: stretch is both the
	// zero value and the initial, so modes alone cannot tell "declared".
	HasRepeat bool
	RepeatX   BorderImageRepeat
	RepeatY   BorderImageRepeat
}

// ParseBorderImageSource reads border-image-source: none | url(...). Only
// url() payloads return a source; gradients and none return "".
func ParseBorderImageSource(value string) string {
	v := strings.TrimSpace(value)
	if strings.EqualFold(v, "none") || v == "" {
		return ""
	}
	if u, ok := borderImageURL(v); ok {
		return u
	}
	return ""
}

// borderImageURL returns the payload of the first url() in v.
func borderImageURL(v string) (string, bool) {
	lower := strings.ToLower(v)
	i := strings.Index(lower, "url(")
	if i < 0 {
		return "", false
	}
	rest := v[i+len("url("):]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return "", false
	}
	target := strings.Trim(strings.TrimSpace(rest[:end]), "\"'")
	if target == "" {
		return "", false
	}
	return target, true
}

// ParseBorderImageSlice reads border-image-slice: 1-4 numbers/percentages
// with an optional fill keyword in any position.
func ParseBorderImageSlice(value string) (BorderImage, bool) {
	var bi BorderImage
	toks := strings.Fields(value)
	if len(toks) == 0 || len(toks) > 5 {
		return bi, false
	}
	var nums []float32
	var pcts []bool
	fillSeen := false
	for _, tok := range toks {
		if strings.EqualFold(tok, "fill") {
			if fillSeen {
				return bi, false
			}
			fillSeen = true
			bi.Fill = true
			continue
		}
		if strings.HasSuffix(tok, "%") {
			f, ok := strictNumber(strings.TrimSuffix(tok, "%"))
			if !ok || f < 0 {
				return bi, false
			}
			nums = append(nums, f)
			pcts = append(pcts, true)
			continue
		}
		f, ok := strictNumber(tok)
		if !ok || f < 0 {
			return bi, false
		}
		nums = append(nums, f)
		pcts = append(pcts, false)
	}
	if len(nums) == 0 || len(nums) > 4 {
		return bi, false
	}
	n := expandBox(nums)
	p := expandBoxBools(pcts)
	bi.Slice = n
	bi.SlicePct = p
	bi.HasSlice = true
	return bi, true
}

// ParseBorderImageWidth reads border-image-width: 1-4 of length | number |
// percentage | auto. Percentages resolve against the border-image area;
// numbers multiply the border width.
func ParseBorderImageWidth(value string) (BorderImage, bool) {
	var bi BorderImage
	toks := strings.Fields(value)
	if len(toks) == 0 || len(toks) > 4 {
		return bi, false
	}
	var vals [4]float32
	var units [4]BorderImageWidthUnit
	var rawV []float32
	var rawU []BorderImageWidthUnit
	var rawP []bool
	for _, tok := range toks {
		if strings.EqualFold(tok, "auto") {
			rawV = append(rawV, 0)
			rawU = append(rawU, BiWidthAuto)
			rawP = append(rawP, false)
			continue
		}
		if strings.HasSuffix(tok, "%") {
			f, ok := strictNumber(strings.TrimSuffix(tok, "%"))
			if !ok || f < 0 {
				return bi, false
			}
			rawV = append(rawV, f)
			rawU = append(rawU, BiWidthLength)
			rawP = append(rawP, true)
			continue
		}
		v := ParseValue(tok)
		switch v.Type {
		case ValueLength:
			if v.Num < 0 {
				return bi, false
			}
			rawV = append(rawV, float32(v.Num))
			rawU = append(rawU, BiWidthLength)
			rawP = append(rawP, false)
		case ValueNumber:
			if v.Num < 0 {
				return bi, false
			}
			rawV = append(rawV, float32(v.Num))
			rawU = append(rawU, BiWidthNumber)
			rawP = append(rawP, false)
		default:
			return bi, false
		}
	}
	vals = expandBox(rawV)
	units = expandBoxUnits(rawU)
	bi.Width, bi.WidthU, bi.HasWidth = vals, units, true
	bi.WidthPct = expandBoxBools(rawP)
	return bi, true
}

// ParseBorderImageOutset reads border-image-outset: 1-4 lengths or numbers
// (multiples of the border width). Negatives are invalid.
func ParseBorderImageOutset(value string) (BorderImage, bool) {
	var bi BorderImage
	toks := strings.Fields(value)
	if len(toks) == 0 || len(toks) > 4 {
		return bi, false
	}
	var rawV []float32
	var rawN []bool
	for _, tok := range toks {
		v := ParseValue(tok)
		switch v.Type {
		case ValueLength:
			if v.Num < 0 {
				return bi, false
			}
			rawV = append(rawV, float32(v.Num))
			rawN = append(rawN, false)
		case ValueNumber:
			if v.Num < 0 {
				return bi, false
			}
			rawV = append(rawV, float32(v.Num))
			rawN = append(rawN, true)
		default:
			return bi, false
		}
	}
	bi.Outset = expandBox(rawV)
	bi.OutsetN = expandBoxBools(rawN)
	bi.HasOutset = true
	return bi, true
}

// ParseBorderImageRepeat reads border-image-repeat: 1-2 of stretch | repeat
// | round | space.
func ParseBorderImageRepeat(value string) (BorderImage, bool) {
	var bi BorderImage
	toks := strings.Fields(value)
	if len(toks) == 0 || len(toks) > 2 {
		return bi, false
	}
	modes := make([]BorderImageRepeat, 0, 2)
	for _, tok := range toks {
		switch strings.ToLower(tok) {
		case "stretch":
			modes = append(modes, BiRepeatStretch)
		case "repeat":
			modes = append(modes, BiRepeatRepeat)
		case "round":
			modes = append(modes, BiRepeatRound)
		case "space":
			modes = append(modes, BiRepeatSpace)
		default:
			return bi, false
		}
	}
	bi.RepeatX = modes[0]
	if len(modes) == 2 {
		bi.RepeatY = modes[1]
	} else {
		bi.RepeatY = modes[0]
	}
	bi.HasRepeat = true
	return bi, true
}

// ParseBorderImageShorthand splits border-image into its longhand sources:
// source and slice before the first slash, width between the slashes, outset
// after the second, and repeat keywords (which may sit in any section after
// the source). It returns the longhand value strings for the caller to parse.
func ParseBorderImageShorthand(value string) (source, slice, width, outset, repeat string, ok bool) {
	parts := splitTopLevel(value, '/')
	if len(parts) == 0 || len(parts) > 3 {
		return "", "", "", "", "", false
	}
	// Repeat keywords may appear in any section; pull them out first so the
	// section parsers see only their own grammar.
	var repToks []string
	strip := func(s string) string {
		var keep []string
		for _, tok := range strings.Fields(s) {
			switch strings.ToLower(tok) {
			case "stretch", "repeat", "round", "space":
				repToks = append(repToks, tok)
			default:
				keep = append(keep, tok)
			}
		}
		return strings.Join(keep, " ")
	}
	for i := range parts {
		parts[i] = strip(parts[i])
	}
	if len(repToks) > 2 {
		return "", "", "", "", "", false
	}
	repeat = strings.Join(repToks, " ")
	// The source is the url()/none/gradient token: the first token of the
	// first section that opens url( or names none.
	toks := strings.Fields(parts[0])
	srcIdx := -1
	for i, tok := range toks {
		l := strings.ToLower(tok)
		if strings.HasPrefix(l, "url(") || l == "none" || strings.HasPrefix(l, "linear-gradient(") || strings.HasPrefix(l, "radial-gradient(") {
			srcIdx = i
			break
		}
	}
	if srcIdx < 0 {
		return "", "", "", "", "", false
	}
	// A url() payload may itself contain spaces; rejoin from the url( token
	// through its closing paren.
	srcEnd := srcIdx
	if strings.HasPrefix(strings.ToLower(toks[srcIdx]), "url(") && !strings.Contains(toks[srcIdx], ")") {
		for srcEnd < len(toks) && !strings.Contains(toks[srcEnd], ")") {
			srcEnd++
		}
		if srcEnd >= len(toks) {
			return "", "", "", "", "", false
		}
	}
	source = strings.Join(toks[srcIdx:srcEnd+1], " ")
	rest := append(append([]string{}, toks[:srcIdx]...), toks[srcEnd+1:]...)
	slice = strings.Join(rest, " ")
	if len(parts) > 1 {
		width = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		outset = strings.TrimSpace(parts[2])
	}
	return source, slice, width, outset, repeat, true
}

// strictNumber parses a bare CSS number: no units, no trailing garbage.
// parseFloat64 accepts numeric prefixes ("10px" reads as 10), which is right
// for error-tolerant grammars but wrong for slice, where a length is invalid.
func strictNumber(s string) (float32, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c == '.' || c == '+' || c == '-' || c == 'e' || c == 'E' {
			continue
		}
		return 0, false
	}
	f, ok := parseFloat64(s)
	if !ok {
		return 0, false
	}
	return float32(f), true
}

// expandBox spreads 1-4 TRBL values the way all CSS box shorthands do.
func expandBox(v []float32) [4]float32 {
	var o [4]float32
	switch len(v) {
	case 1:
		o = [4]float32{v[0], v[0], v[0], v[0]}
	case 2:
		o = [4]float32{v[0], v[1], v[0], v[1]}
	case 3:
		o = [4]float32{v[0], v[1], v[2], v[1]}
	default:
		copy(o[:], v[:4])
	}
	return o
}

// expandBoxBools spreads 1-4 TRBL flags alongside expandBox.
func expandBoxBools(v []bool) [4]bool {
	var o [4]bool
	switch len(v) {
	case 1:
		o = [4]bool{v[0], v[0], v[0], v[0]}
	case 2:
		o = [4]bool{v[0], v[1], v[0], v[1]}
	case 3:
		o = [4]bool{v[0], v[1], v[2], v[1]}
	default:
		copy(o[:], v[:4])
	}
	return o
}

// expandBoxUnits spreads 1-4 TRBL width units alongside expandBox.
func expandBoxUnits(v []BorderImageWidthUnit) [4]BorderImageWidthUnit {
	var o [4]BorderImageWidthUnit
	switch len(v) {
	case 1:
		o = [4]BorderImageWidthUnit{v[0], v[0], v[0], v[0]}
	case 2:
		o = [4]BorderImageWidthUnit{v[0], v[1], v[0], v[1]}
	case 3:
		o = [4]BorderImageWidthUnit{v[0], v[1], v[2], v[1]}
	default:
		copy(o[:], v[:4])
	}
	return o
}

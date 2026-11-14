package css

import (
	"strings"
)

type BorderImageRepeat uint8

const (
	BiRepeatStretch BorderImageRepeat = iota
	BiRepeatRepeat
	BiRepeatRound
	BiRepeatSpace
)

type BorderImageWidthUnit uint8

const (
	BiWidthAuto BorderImageWidthUnit = iota
	BiWidthNumber
	BiWidthLength
)

type BorderImage struct {
	Source    string
	HasSlice  bool
	Slice     [4]float32
	SlicePct  [4]bool
	Fill      bool
	HasWidth  bool
	Width     [4]float32
	WidthU    [4]BorderImageWidthUnit
	WidthPct  [4]bool
	HasOutset bool
	Outset    [4]float32
	OutsetN   [4]bool
	HasRepeat bool
	RepeatX   BorderImageRepeat
	RepeatY   BorderImageRepeat
}

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

func borderImageURL(v string) (string, bool) {
	t, ok := ScanURL(v)
	if !ok {
		return "", false
	}
	u := URLPayload(t)
	if u == "" {
		return "", false
	}
	return u, true
}

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

func ParseBorderImageShorthand(value string) (source, slice, width, outset, repeat string, ok bool) {
	parts := splitTopLevel(value, '/')
	if len(parts) == 0 || len(parts) > 3 {
		return "", "", "", "", "", false
	}
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
	source = toks[srcIdx]
	rest := append([]string{}, toks[:srcIdx]...)
	if strings.HasPrefix(strings.ToLower(source), "url(") {
		joined := strings.Join(toks[srcIdx:], " ")
		t, ok := ScanURL(joined)
		if !ok {
			return "", "", "", "", "", false
		}
		source = joined[:t.End]
		rest = append(rest, strings.Fields(joined[t.End:])...)
	} else {
		rest = append(rest, toks[srcIdx+1:]...)
	}
	slice = strings.Join(rest, " ")
	if len(parts) > 1 {
		width = strings.TrimSpace(parts[1])
	}
	if len(parts) > 2 {
		outset = strings.TrimSpace(parts[2])
	}
	return source, slice, width, outset, repeat, true
}

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

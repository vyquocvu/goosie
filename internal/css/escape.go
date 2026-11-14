package css

import "strings"

func DecodeEscapes(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			sb.WriteByte(s[i])
			i++
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && i-start < 6 && isHex(s[i]) {
			i++
		}
		if i > start {
			codepoint := uint32(0)
			for j := start; j < i; j++ {
				codepoint = codepoint*16 + hexVal(s[j])
			}
			if i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\f') {
				i++
			}
			if codepoint == 0 || codepoint > 0x10FFFF || (codepoint >= 0xD800 && codepoint <= 0xDFFF) {
				sb.WriteRune(0xFFFD)
			} else {
				sb.WriteRune(rune(codepoint))
			}
		} else {
			sb.WriteByte(s[i])
			i++
		}
	}
	return sb.String()
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexVal(c byte) uint32 {
	switch {
	case c >= '0' && c <= '9':
		return uint32(c - '0')
	case c >= 'a' && c <= 'f':
		return uint32(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return uint32(c-'A') + 10
	}
	return 0
}

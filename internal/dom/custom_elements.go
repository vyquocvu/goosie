package dom

import "unicode/utf8"

// IsValidCustomElementName reports whether name satisfies the HTML Standard's
// custom-element-name production, including its reserved-name exclusions.
func IsValidCustomElementName(name string) bool {
	if len(name) == 0 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	hasHyphen := false
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == utf8.RuneError && size == 1 || !isPotentialCustomElementNameChar(r) {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			return false
		}
		if r == '-' {
			hasHyphen = true
		}
		i += size
	}
	if !hasHyphen {
		return false
	}
	switch name {
	case "annotation-xml", "color-profile", "font-face", "font-face-src", "font-face-uri", "font-face-format", "font-face-name", "missing-glyph":
		return false
	default:
		return true
	}
}

func isPotentialCustomElementNameChar(r rune) bool {
	return r != 0 && r != '\t' && r != '\n' && r != '\f' && r != '\r' && r != ' ' && r != '/' && r != '>'
}

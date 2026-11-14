package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

const maxImportDepth = 4

func expandCSSImports(linker CSSLinker, sheetURL, text string, seen map[string]bool, sub *subBudget, cache *cssTextCache, depth int, onMixed func(MixedContentKind, string)) string {
	if linker == nil || depth > maxImportDepth {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); {
		switch c := text[i]; c {
		case '/':
			if i+1 < len(text) && text[i+1] == '*' {
				if e := strings.Index(text[i+2:], "*/"); e >= 0 {
					b.WriteString(text[i : i+2+e+2])
					i += 2 + e + 2
					continue
				}
				b.WriteString(text[i:])
				return b.String()
			}
		case '"', '\'':
			j := stringEnd(text, i)
			b.WriteString(text[i:j])
			i = j
			continue
		case '@':
			if len(text) > i+7 && strings.EqualFold(text[i:i+7], "@import") {
				end := statementEnd(text, i)
				if end > 0 {
					if ref, ok := importRef(text[i:end]); ok {
						if abs, ok := resolveSubresourceURL(sheetURL, ref); ok && !seen[abs] && len(seen) < maxCSSImports {
							seen[abs] = true
							if sheet, ok := cssSheetText(linker, sheetURL, abs, sub, cache); ok {
								inner := expandCSSImports(linker, abs, absolutizeCSSURLs(sheet, abs), seen, sub, cache, depth+1, onMixed)
								if importUsable(inner) {
									b.WriteString(inner)
								}
							}
						} else if !ok && onMixed != nil {
							if bad, mixed := mixedContentURL(sheetURL, ref); mixed {
								onMixed(MixedContentStylesheet, bad)
							}
						}
					}
					i = end
					continue
				}
			}
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

func stringEnd(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case q:
			return j + 1
		}
	}
	return len(s)
}

func statementEnd(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '"', '\'':
			j = stringEnd(s, j) - 1
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				return j + 1
			}
		case '}':
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

func importRef(stmt string) (string, bool) {
	rest := strings.TrimSpace(stmt[len("@import"):])
	rest = strings.TrimRight(rest, ";")
	ref, remainder := "", rest
	if strings.HasPrefix(strings.ToLower(rest), "url(") {
		t, ok := css.ScanURL(rest)
		if !ok {
			return "", false
		}
		ref = css.URLPayload(t)
		remainder = strings.TrimSpace(rest[t.End:])
	} else if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
		end := stringEnd(rest, 0)
		ref = unquote(rest[:end])
		remainder = strings.TrimSpace(rest[end:])
	}
	if ref == "" {
		return "", false
	}
	if remainder != "" && remainder != "all" && remainder != "screen" {
		return "", false
	}
	return ref, true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return css.DecodeEscapes(s[1 : len(s)-1])
	}
	return css.DecodeEscapes(s)
}

func importUsable(text string) bool {
	b := &cssBudget{}
	return b.check(text, false) == nil
}

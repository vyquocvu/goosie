package engine

import "strings"

// maxImportDepth bounds one sheet's @import chain. Real themes nest one or two
// deep; a deeper chain is a cycle or an attack, and dropping the rest of it
// costs a few rules rather than the document.
const maxImportDepth = 4

// expandCSSImports replaces every @import statement in a fetched style sheet
// with the sheet it names, in place: imported rules cascade at the position of
// the statement, so a theme sheet spliced after a reset keeps overriding it.
//
// Statements carrying a media or layer condition are dropped without fetching,
// since the engine has no media evaluation for them: a page that ships its
// light and dark themes as conditioned imports then resolves to the
// unconditional one, which is what a default desktop browser loads too.
func expandCSSImports(linker CSSLinker, sheetURL, text string, seen map[string]bool, depth int) string {
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
						if abs, ok := resolveSheetURL(sheetURL, ref); ok && !seen[abs] {
							seen[abs] = true
							if sheet, err := linker(sheetURL, abs); err == nil {
								inner := expandCSSImports(linker, abs, absolutizeCSSURLs(sheet, abs), seen, depth+1)
								if importUsable(inner) {
									b.WriteString(inner)
								}
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

// stringEnd returns the index just past the quoted string starting at i.
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

// statementEnd returns the index just past the @import statement at i, or -1
// when it never closes. Parentheses and strings inside the statement are
// skipped so a url("a;b") does not end it early.
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

// importRef pulls the URL out of an @import statement and reports whether the
// statement applies at all. Anything after the URL is a media, layer or
// supports condition; `all` and `screen` match the desktop viewport the engine
// renders, and everything else is treated as not matching.
func importRef(stmt string) (string, bool) {
	rest := strings.TrimSpace(stmt[len("@import"):])
	rest = strings.TrimRight(rest, ";")
	ref, remainder := "", rest
	if strings.HasPrefix(strings.ToLower(rest), "url(") {
		close := strings.IndexByte(rest, ')')
		if close < 0 {
			return "", false
		}
		ref = unquote(strings.TrimSpace(rest[4:close]))
		remainder = strings.TrimSpace(rest[close+1:])
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
		return s[1 : len(s)-1]
	}
	return s
}

// importUsable reports whether an imported sheet can be spliced into its parent
// without taking the parent down with it. The per-sheet budget rejects some real
// sheets outright - Google's font CSS trips the numeric guard with its
// `unicode-range: U+0000-00FF` lists - and once such text sits inside another
// sheet the pair fails the same check, so a stylesheet the page can live
// without would cost every rule of the sheet that imported it.
func importUsable(text string) bool {
	b := &cssBudget{}
	return b.check(text, false) == nil
}

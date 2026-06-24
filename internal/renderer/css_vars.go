package renderer

import "strings"

// resolveVarTokens replaces all var(--name) and var(--name, fallback) tokens
// in a CSS value string using the custom properties from the given style.
func resolveVarTokens(value string, style *Style) string {
	if style == nil || !strings.Contains(value, "var(") {
		return value
	}
	return resolveVarInner(value, style, 0)
}

// resolveVarInner handles nested var() calls up to depth 10 to avoid infinite loops.
func resolveVarInner(value string, style *Style, depth int) string {
	if depth > 10 || !strings.Contains(value, "var(") {
		return value
	}
	result := ""
	remaining := value
	for {
		idx := strings.Index(remaining, "var(")
		if idx == -1 {
			result += remaining
			break
		}
		result += remaining[:idx]
		remaining = remaining[idx+4:] // skip "var("

		// Find matching closing paren (handles nested parens)
		depth2 := 1
		i := 0
		for i < len(remaining) && depth2 > 0 {
			if remaining[i] == '(' {
				depth2++
			} else if remaining[i] == ')' {
				depth2--
			}
			if depth2 > 0 {
				i++
			}
		}
		if i >= len(remaining) {
			// malformed var() with no closing paren — skip the rest
			break
		}
		inner := remaining[:i]
		remaining = remaining[i+1:] // skip closing ')'

		// Split inner into name and optional fallback at first comma not inside parens
		name, fallback := splitVarArgs(inner)
		name = strings.TrimSpace(name)

		var resolved string
		if style.CustomProperties != nil {
			if v, ok := style.CustomProperties[name]; ok {
				resolved = resolveVarInner(strings.TrimSpace(v), style, depth+1)
			}
		}
		if resolved == "" && fallback != "" {
			resolved = resolveVarInner(strings.TrimSpace(fallback), style, depth+1)
		}
		// Leave resolved as "" — caller will detect empty string and skip
		result += resolved
	}
	return result
}

// splitVarArgs splits "name, fallback" handling nested parens in the fallback.
func splitVarArgs(s string) (name, fallback string) {
	depth := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				return s[:i], s[i+1:]
			}
		}
	}
	return s, ""
}

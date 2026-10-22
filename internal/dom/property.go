package dom

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// CleanSyntax strips enclosing quotes from a syntax string, e.g. "'<color>'" -> "<color>".
func CleanSyntax(syntax string) string {
	syntax = strings.TrimSpace(syntax)
	if len(syntax) >= 2 {
		if (syntax[0] == '\'' && syntax[len(syntax)-1] == '\'') ||
			(syntax[0] == '"' && syntax[len(syntax)-1] == '"') {
			syntax = strings.TrimSpace(syntax[1 : len(syntax)-1])
		}
	}
	return syntax
}

// ValidatePropertySyntax checks whether a syntax descriptor conforms to the
// CSS Properties and Values API Level 1 grammar.
func ValidatePropertySyntax(syntax string) error {
	syntax = CleanSyntax(syntax)
	if syntax == "" {
		return fmt.Errorf("empty syntax descriptor")
	}
	if syntax == "*" {
		return nil
	}

	parts := strings.Split(syntax, "|")
	if len(parts) == 0 {
		return fmt.Errorf("invalid syntax descriptor %q", syntax)
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty alternation in syntax %q", syntax)
		}
		if err := validateSyntaxComponent(part); err != nil {
			return err
		}
	}
	return nil
}

func validateSyntaxComponent(comp string) error {
	base := comp
	if strings.HasSuffix(base, "+") || strings.HasSuffix(base, "#") {
		base = base[:len(base)-1]
	}

	if strings.HasPrefix(base, "<") && strings.HasSuffix(base, ">") {
		inner := strings.ToLower(base[1 : len(base)-1])
		switch inner {
		case "length", "number", "percentage", "length-percentage",
			"color", "image", "url", "integer", "angle", "time",
			"resolution", "transform-function", "transform-list", "custom-ident":
			return nil
		default:
			return fmt.Errorf("unknown data type <%s> in syntax", inner)
		}
	}

	// Ident token: must be a valid CSS ident and not a reserved keyword
	lower := strings.ToLower(base)
	switch lower {
	case "inherit", "initial", "unset", "revert", "revert-layer", "default":
		return fmt.Errorf("reserved CSS keyword %q not allowed as custom ident", base)
	}

	for i, r := range base {
		if i == 0 && (r == '-' || unicode.IsDigit(r)) {
			if r == '-' && len(base) > 1 && unicode.IsDigit(rune(base[1])) {
				return fmt.Errorf("invalid ident in syntax %q", comp)
			}
			if unicode.IsDigit(r) {
				return fmt.Errorf("ident cannot start with digit %q", comp)
			}
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return fmt.Errorf("invalid character in ident %q", comp)
		}
	}
	return nil
}

// MatchesPropertySyntax returns true if value conforms to the registered syntax.
func MatchesPropertySyntax(syntax, val string) bool {
	syntax = CleanSyntax(syntax)
	val = strings.TrimSpace(val)
	if syntax == "*" {
		return val != ""
	}
	if val == "" {
		return false
	}

	branches := strings.Split(syntax, "|")
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if matchesSingleBranch(branch, val) {
			return true
		}
	}
	return false
}

func matchesSingleBranch(branch, val string) bool {
	if strings.HasSuffix(branch, "+") {
		base := strings.TrimSpace(branch[:len(branch)-1])
		items := strings.Fields(val)
		if len(items) == 0 {
			return false
		}
		for _, item := range items {
			if !matchesBaseType(base, item) {
				return false
			}
		}
		return true
	}

	if strings.HasSuffix(branch, "#") {
		base := strings.TrimSpace(branch[:len(branch)-1])
		items := strings.Split(val, ",")
		if len(items) == 0 {
			return false
		}
		for _, item := range items {
			if !matchesBaseType(base, strings.TrimSpace(item)) {
				return false
			}
		}
		return true
	}

	return matchesBaseType(branch, val)
}

func matchesBaseType(base, val string) bool {
	base = strings.TrimSpace(base)
	val = strings.TrimSpace(val)

	if strings.HasPrefix(base, "<") && strings.HasSuffix(base, ">") {
		inner := strings.ToLower(base[1 : len(base)-1])
		switch inner {
		case "color":
			return isPlausibleColor(val)
		case "length":
			return isLengthValue(val)
		case "number":
			_, err := strconv.ParseFloat(val, 64)
			return err == nil
		case "integer":
			num, err := strconv.ParseFloat(val, 64)
			return err == nil && float64(int64(num)) == num && !strings.Contains(val, ".")
		case "percentage":
			if strings.HasSuffix(val, "%") {
				_, err := strconv.ParseFloat(val[:len(val)-1], 64)
				return err == nil
			}
			return false
		case "length-percentage":
			if strings.HasSuffix(val, "%") {
				_, err := strconv.ParseFloat(val[:len(val)-1], 64)
				return err == nil
			}
			return isLengthValue(val)
		case "angle":
			lower := strings.ToLower(val)
			for _, u := range []string{"deg", "rad", "grad", "turn"} {
				if strings.HasSuffix(lower, u) {
					_, err := strconv.ParseFloat(lower[:len(lower)-len(u)], 64)
					return err == nil
				}
			}
			return false
		case "time":
			lower := strings.ToLower(val)
			for _, u := range []string{"ms", "s"} {
				if strings.HasSuffix(lower, u) {
					_, err := strconv.ParseFloat(lower[:len(lower)-len(u)], 64)
					return err == nil
				}
			}
			return false
		case "resolution":
			lower := strings.ToLower(val)
			for _, u := range []string{"dpi", "dpcm", "dppx"} {
				if strings.HasSuffix(lower, u) {
					_, err := strconv.ParseFloat(lower[:len(lower)-len(u)], 64)
					return err == nil
				}
			}
			return false
		case "url":
			return strings.HasPrefix(strings.ToLower(val), "url(") && strings.HasSuffix(val, ")")
		case "image":
			lower := strings.ToLower(val)
			if strings.HasPrefix(lower, "url(") && strings.HasSuffix(val, ")") {
				return true
			}
			if strings.HasPrefix(lower, "linear-gradient(") ||
				strings.HasPrefix(lower, "radial-gradient(") ||
				strings.HasPrefix(lower, "conic-gradient(") {
				return strings.HasSuffix(val, ")")
			}
			return false
		case "transform-function", "transform-list":
			lower := strings.ToLower(val)
			return strings.Contains(lower, "translate") ||
				strings.Contains(lower, "scale") ||
				strings.Contains(lower, "rotate") ||
				strings.Contains(lower, "matrix") ||
				strings.Contains(lower, "skew")
		case "custom-ident":
			return isValidCustomIdent(val)
		}
	}

	// Literal keyword match
	return strings.EqualFold(base, val)
}

func isLengthValue(val string) bool {
	val = strings.TrimSpace(val)
	if val == "0" {
		return true
	}
	lower := strings.ToLower(val)
	if strings.HasPrefix(lower, "calc(") || strings.HasPrefix(lower, "min(") ||
		strings.HasPrefix(lower, "max(") || strings.HasPrefix(lower, "clamp(") {
		return strings.HasSuffix(val, ")")
	}

	units := []string{
		"cqmin", "cqmax", "vmin", "vmax",
		"cqw", "cqh", "cqi", "cqb",
		"rem", "px", "em", "ex", "ch", "vw", "vh",
		"pt", "pc", "in", "cm", "mm", "q",
	}
	for _, u := range units {
		if strings.HasSuffix(lower, u) {
			numPart := strings.TrimSpace(lower[:len(lower)-len(u)])
			if _, err := strconv.ParseFloat(numPart, 64); err == nil {
				return true
			}
		}
	}
	return false
}

func isValidCustomIdent(val string) bool {
	if val == "" {
		return false
	}
	lower := strings.ToLower(val)
	switch lower {
	case "inherit", "initial", "unset", "revert", "revert-layer", "default":
		return false
	}
	for i, r := range val {
		if i == 0 && (r == '-' || unicode.IsDigit(r)) {
			if r == '-' && len(val) > 1 && unicode.IsDigit(rune(val[1])) {
				return false
			}
			if unicode.IsDigit(r) {
				return false
			}
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func isPlausibleColor(val string) bool {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "#") {
		hex := val[1:]
		if len(hex) == 3 || len(hex) == 4 || len(hex) == 6 || len(hex) == 8 {
			for _, r := range hex {
				if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
					return false
				}
			}
			return true
		}
		return false
	}

	lower := strings.ToLower(val)
	colorFuncs := []string{
		"rgb(", "rgba(", "hsl(", "hsla(", "hwb(", "lab(", "oklab(", "lch(", "oklch(",
		"color(", "color-mix(", "light-dark(",
	}
	for _, fn := range colorFuncs {
		if strings.HasPrefix(lower, fn) && strings.HasSuffix(lower, ")") {
			return true
		}
	}

	namedColors := map[string]bool{
		"transparent": true, "currentcolor": true,
		"black": true, "silver": true, "gray": true, "white": true,
		"maroon": true, "red": true, "purple": true, "fuchsia": true,
		"green": true, "lime": true, "olive": true, "yellow": true,
		"navy": true, "blue": true, "teal": true, "aqua": true,
		"orange": true, "cyan": true, "magenta": true, "pink": true,
		"gold": true, "brown": true, "indigo": true, "violet": true,
	}
	return namedColors[lower]
}

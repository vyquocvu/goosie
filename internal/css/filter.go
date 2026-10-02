package css

import (
	"strings"
)

// FilterFunc represents one CSS filter function.
type FilterFunc struct {
	Name string  // e.g., "blur", "brightness"
	Arg  float32 // argument value (interpretation depends on function)
}

// ParseFilter parses a CSS filter property value.
// Supported functions: blur(), brightness(), contrast(), grayscale(), invert(),
// opacity(), saturate(), sepia().
func ParseFilter(s string) []FilterFunc {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return nil
	}

	var funcs []FilterFunc
	i := 0

	for i < len(s) {
		// Skip whitespace.
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}

		// Parse function name.
		start := i
		for i < len(s) && s[i] != '(' {
			i++
		}
		if i >= len(s) {
			break
		}
		name := strings.TrimSpace(s[start:i])
		i++ // skip '('

		// Parse argument.
		argStart := i
		depth := 1
		for i < len(s) && depth > 0 {
			if s[i] == '(' {
				depth++
			} else if s[i] == ')' {
				depth--
			}
			if depth > 0 {
				i++
			}
		}
		argStr := strings.TrimSpace(s[argStart:i])
		if i < len(s) {
			i++ // skip ')'
		}

		// Parse the argument value.
		arg := parseFilterArg(name, argStr)
		funcs = append(funcs, FilterFunc{Name: name, Arg: arg})
	}

	return funcs
}

// parseFilterArg parses the argument for a filter function.
func parseFilterArg(name, s string) float32 {
	// Most filter functions take a number with optional unit.
	// blur() takes a length (px).
	// Others take a number or percentage.

	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Strip units.
	if strings.HasSuffix(s, "px") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "%") {
		s = s[:len(s)-1]
		// Convert percentage to multiplier (100% = 1.0).
		v := parseFilterFloat(s)
		return v / 100
	}

	return parseFilterFloat(s)
}

// parseFilterFloat parses a float from a string.
func parseFilterFloat(s string) float32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	intPart := float32(0)
	fracPart := float32(0)
	inFrac := false
	fracDiv := float32(1)

	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			inFrac = true
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			digit := float32(s[i] - '0')
			if inFrac {
				fracDiv *= 10
				fracPart += digit / fracDiv
			} else {
				intPart = intPart*10 + digit
			}
		} else {
			break
		}
	}

	v := intPart + fracPart
	if negative {
		v = -v
	}
	return v
}

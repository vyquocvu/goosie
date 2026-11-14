package css

import (
	"strconv"
	"strings"
)

type FilterFunc struct {
	Name string
	Arg  float32
	Raw  string
}

func FormatFilter(funcs []FilterFunc) string {
	if len(funcs) == 0 {
		return "none"
	}
	var b strings.Builder
	for i, f := range funcs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(f.Name)
		b.WriteByte('(')
		if f.Raw != "" {
			b.WriteString(f.Raw)
		} else {
			b.WriteString(strconv.FormatFloat(float64(f.Arg), 'f', -1, 32))
		}
		b.WriteByte(')')
	}
	return b.String()
}

func ParseFilter(s string) []FilterFunc {
	s = strings.TrimSpace(s)
	if s == "" || s == "none" {
		return nil
	}

	var funcs []FilterFunc
	i := 0

	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}

		start := i
		for i < len(s) && s[i] != '(' {
			i++
		}
		if i >= len(s) {
			break
		}
		name := strings.TrimSpace(s[start:i])
		i++

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
			i++
		}

		arg := parseFilterArg(name, argStr)
		funcs = append(funcs, FilterFunc{Name: name, Arg: arg, Raw: argStr})
	}

	return funcs
}

func parseFilterArg(name, s string) float32 {

	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	if strings.HasSuffix(s, "px") {
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "%") {
		s = s[:len(s)-1]
		v := parseFilterFloat(s)
		return v / 100
	}

	return parseFilterFloat(s)
}

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

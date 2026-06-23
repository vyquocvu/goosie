package renderer

import (
	"math"
	"strconv"
	"strings"
)

// evalCalcExpr evaluates a CSS calc/min/max/clamp expression and returns px value.
// pct is the containing-block dimension used for % resolution.
func evalCalcExpr(expr string, fontSize, vw, vh, pct float32) float32 {
	expr = strings.TrimSpace(expr)
	lower := strings.ToLower(expr)
	if strings.HasPrefix(lower, "calc(") && strings.HasSuffix(expr, ")") {
		inner := expr[5 : len(expr)-1]
		return evalCalcAddSub(inner, fontSize, vw, vh, pct)
	}
	if strings.HasPrefix(lower, "min(") && strings.HasSuffix(expr, ")") {
		args := splitCalcCommas(expr[4 : len(expr)-1])
		best := float32(math.MaxFloat32)
		for _, a := range args {
			v := evalCalcExpr(strings.TrimSpace(a), fontSize, vw, vh, pct)
			if v < best {
				best = v
			}
		}
		return best
	}
	if strings.HasPrefix(lower, "max(") && strings.HasSuffix(expr, ")") {
		args := splitCalcCommas(expr[4 : len(expr)-1])
		best := float32(-math.MaxFloat32)
		for _, a := range args {
			v := evalCalcExpr(strings.TrimSpace(a), fontSize, vw, vh, pct)
			if v > best {
				best = v
			}
		}
		return best
	}
	if strings.HasPrefix(lower, "clamp(") && strings.HasSuffix(expr, ")") {
		args := splitCalcCommas(expr[6 : len(expr)-1])
		if len(args) == 3 {
			minV := evalCalcExpr(strings.TrimSpace(args[0]), fontSize, vw, vh, pct)
			val := evalCalcExpr(strings.TrimSpace(args[1]), fontSize, vw, vh, pct)
			maxV := evalCalcExpr(strings.TrimSpace(args[2]), fontSize, vw, vh, pct)
			if val < minV {
				return minV
			}
			if val > maxV {
				return maxV
			}
			return val
		}
	}
	return resolveSingleLength(expr, fontSize, vw, vh, pct)
}

func evalCalcAddSub(expr string, fontSize, vw, vh, pct float32) float32 {
	expr = strings.TrimSpace(expr)
	depth := 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depth++
		} else if c == '(' {
			depth--
		}
		if depth == 0 && (c == '+' || c == '-') && i > 0 {
			if i > 0 && expr[i-1] == ' ' {
				left := evalCalcAddSub(expr[:i-1], fontSize, vw, vh, pct)
				right := evalCalcMulDiv(strings.TrimSpace(expr[i+1:]), fontSize, vw, vh, pct)
				if c == '+' {
					return left + right
				}
				return left - right
			}
		}
	}
	return evalCalcMulDiv(expr, fontSize, vw, vh, pct)
}

func evalCalcMulDiv(expr string, fontSize, vw, vh, pct float32) float32 {
	expr = strings.TrimSpace(expr)
	depth := 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depth++
		} else if c == '(' {
			depth--
		}
		if depth == 0 && (c == '*' || c == '/') {
			left := evalCalcMulDiv(strings.TrimSpace(expr[:i]), fontSize, vw, vh, pct)
			right := resolveSingleLength(strings.TrimSpace(expr[i+1:]), fontSize, vw, vh, pct)
			if c == '*' {
				return left * right
			}
			if right != 0 {
				return left / right
			}
			return 0
		}
	}
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return evalCalcAddSub(expr[1:len(expr)-1], fontSize, vw, vh, pct)
	}
	return resolveSingleLength(expr, fontSize, vw, vh, pct)
}

func resolveSingleLength(s string, fontSize, vw, vh, pct float32) float32 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32); err == nil {
			return float32(v) / 100.0 * pct
		}
	}
	if strings.HasSuffix(s, "vw") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "vw"), 32); err == nil {
			return float32(v) / 100.0 * vw
		}
	}
	if strings.HasSuffix(s, "vh") {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(s, "vh"), 32); err == nil {
			return float32(v) / 100.0 * vh
		}
	}
	return parseLength(s, fontSize)
}

// splitCalcCommas splits at commas not inside parentheses.
func splitCalcCommas(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// isCalcExpr returns true if the value starts with calc(, min(, max(, or clamp(.
func isCalcExpr(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(v, "calc(") ||
		strings.HasPrefix(v, "min(") ||
		strings.HasPrefix(v, "max(") ||
		strings.HasPrefix(v, "clamp(")
}

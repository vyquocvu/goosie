package renderer

import (
	"math"
	"strconv"
	"strings"
)

// maxCalcDepth bounds evaluator recursion. Legitimate calc nesting is a
// handful of levels; the cap is a safety net against pathological input.
const maxCalcDepth = 24

// evalCalcExpr evaluates a CSS calc/min/max/clamp expression and returns px value.
// pct is the containing-block dimension used for % resolution.
func evalCalcExpr(expr string, fontSize, vw, vh, pct float32) float32 {
	return evalCalc(expr, fontSize, vw, vh, pct, 0)
}

func evalCalc(expr string, fontSize, vw, vh, pct float32, depth int) float32 {
	if depth > maxCalcDepth {
		return 0
	}
	expr = strings.TrimSpace(expr)
	lower := strings.ToLower(expr)
	switch {
	case strings.HasPrefix(lower, "calc("):
		if inner, ok := balancedInterior(expr); ok {
			return evalCalcAddSub(inner, fontSize, vw, vh, pct, depth+1)
		}
		return 0
	case strings.HasPrefix(lower, "min("):
		if inner, ok := balancedInterior(expr); ok {
			best := float32(math.MaxFloat32)
			for _, a := range splitCalcCommas(inner) {
				if v := evalCalc(strings.TrimSpace(a), fontSize, vw, vh, pct, depth+1); v < best {
					best = v
				}
			}
			return best
		}
		return 0
	case strings.HasPrefix(lower, "max("):
		if inner, ok := balancedInterior(expr); ok {
			best := float32(-math.MaxFloat32)
			for _, a := range splitCalcCommas(inner) {
				if v := evalCalc(strings.TrimSpace(a), fontSize, vw, vh, pct, depth+1); v > best {
					best = v
				}
			}
			return best
		}
		return 0
	case strings.HasPrefix(lower, "clamp("):
		if inner, ok := balancedInterior(expr); ok {
			args := splitCalcCommas(inner)
			if len(args) == 3 {
				minV := evalCalc(strings.TrimSpace(args[0]), fontSize, vw, vh, pct, depth+1)
				val := evalCalc(strings.TrimSpace(args[1]), fontSize, vw, vh, pct, depth+1)
				maxV := evalCalc(strings.TrimSpace(args[2]), fontSize, vw, vh, pct, depth+1)
				if val < minV {
					return minV
				}
				if val > maxV {
					return maxV
				}
				return val
			}
		}
		return 0
	}

	// Anything that still looks like a function call here is malformed
	// (unterminated or carrying trailing tokens, e.g. fragments produced by
	// shorthand splitting). Never hand it back to parseLength: isCalcExpr
	// matches on the prefix alone, so the same string would be redispatched
	// here forever (the github.com stack overflow). Evaluate the interior
	// best-effort instead.
	if isCalcExpr(expr) {
		if inner, ok := balancedInterior(expr); ok {
			return evalCalc(inner, fontSize, vw, vh, pct, depth+1)
		}
		return 0
	}
	return resolveSingleLength(expr, fontSize, vw, vh, pct)
}

// balancedInterior returns the content between the first '(' of expr and its
// MATCHING ')'. Unlike prefix+suffix checks this tolerates trailing tokens
// after the closing paren and rejects unterminated calls.
func balancedInterior(expr string) (string, bool) {
	open := strings.IndexByte(expr, '(')
	if open < 0 {
		return "", false
	}
	depth := 0
	for i := open; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return expr[open+1 : i], true
			}
		}
	}
	return "", false
}

func evalCalcAddSub(expr string, fontSize, vw, vh, pct float32, depth int) float32 {
	if depth > maxCalcDepth {
		return 0
	}
	expr = strings.TrimSpace(expr)
	depthBal := 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depthBal++
		} else if c == '(' {
			depthBal--
		}
		if depthBal == 0 && (c == '+' || c == '-') && i > 0 {
			if i > 0 && expr[i-1] == ' ' {
				left := evalCalcAddSub(expr[:i-1], fontSize, vw, vh, pct, depth+1)
				right := evalCalcMulDiv(strings.TrimSpace(expr[i+1:]), fontSize, vw, vh, pct, depth+1)
				if c == '+' {
					return left + right
				}
				return left - right
			}
		}
	}
	return evalCalcMulDiv(expr, fontSize, vw, vh, pct, depth+1)
}

func evalCalcMulDiv(expr string, fontSize, vw, vh, pct float32, depth int) float32 {
	if depth > maxCalcDepth {
		return 0
	}
	expr = strings.TrimSpace(expr)
	depthBal := 0
	for i := len(expr) - 1; i >= 0; i-- {
		c := expr[i]
		if c == ')' {
			depthBal++
		} else if c == '(' {
			depthBal--
		}
		if depthBal == 0 && (c == '*' || c == '/') {
			left := evalCalcMulDiv(strings.TrimSpace(expr[:i]), fontSize, vw, vh, pct, depth+1)
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
		return evalCalcAddSub(expr[1:len(expr)-1], fontSize, vw, vh, pct, depth+1)
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

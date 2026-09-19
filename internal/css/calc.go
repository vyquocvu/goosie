package css

import (
	"regexp"
	"strconv"
	"strings"
)

// EvalCalc resolves a CSS `calc()` expression to CSS pixels, with `em` and
// viewport units read from the same context ToLengthWithContext uses.
//
// Sums of lengths and scaling by unitless numbers are the whole grammar a
// property value needs in practice. A percentage is the one term that cannot be
// answered here: it resolves against the containing block, which a declaration
// does not know yet. An expression containing one therefore reports itself
// unsupported rather than returning a sum that silently dropped it.
func EvalCalc(expr string, em, vw, vh float32) (float32, bool) {
	return EvalFunc("calc", expr, em, vw, vh)
}

// EvalFunc resolves one of the value-bearing CSS functions - `calc()`,
// `min()`, `max()`, and `clamp(min, val, max)`. Each argument is its own
// expression in calc()'s grammar, so a clamp can nest a calc and a max can nest
// a clamp.
func EvalFunc(name, expr string, em, vw, vh float32) (float32, bool) {
	if em <= 0 {
		em = 16
	}
	if vw <= 0 {
		vw = 1440
	}
	if vh <= 0 {
		vh = 900
	}
	p := &calcExpr{em: em, vw: vw, vh: vh}
	return p.combine(name, splitArgs(expr))
}

// viewportTerm matches a viewport unit inside an expression's text.
var viewportTerm = regexp.MustCompile(`([0-9]*\.?[0-9]+)(svw|svh|dvw|dvh|lvw|lvh|vw|vh|vmin|vmax)`)

// ResolveViewportUnits rewrites the viewport terms in an expression as pixels,
// leaving every other term alone. A `calc()` only reaches its `em` terms once the
// cascade has settled the element's font size, and by then the frame's size is no
// longer in scope, so the two halves of the context are applied where each is
// known.
func ResolveViewportUnits(expr string, vw, vh float32) (string, bool) {
	found := false
	out := viewportTerm.ReplaceAllStringFunc(expr, func(m string) string {
		g := viewportTerm.FindStringSubmatch(m)
		n, err := strconv.ParseFloat(g[1], 32)
		if err != nil {
			return m
		}
		found = true
		v := Value{Type: ValueLength, Num: n, Unit: g[2]}
		return strconv.FormatFloat(float64(v.ToLengthWithContext(0, vw, vh)), 'f', -1, 32) + "px"
	})
	return out, found
}

func fold(args []string, em, vw, vh float32, smallest bool) (float32, bool) {
	best := float32(0)
	for i, a := range args {
		v, ok := evalSum(a, em, vw, vh)
		if !ok {
			return 0, false
		}
		if i == 0 || (smallest && v < best) || (!smallest && v > best) {
			best = v
		}
	}
	return best, true
}

func evalSum(expr string, em, vw, vh float32) (float32, bool) {
	p := &calcExpr{toks: calcTokens(expr), em: em, vw: vw, vh: vh}
	v, ok := p.sum()
	if !ok || p.pos != len(p.toks) {
		return 0, false
	}
	return v, true
}

// calcTokens splits an expression into numbers-with-units, function names,
// operators, and parentheses. Whitespace carries no meaning beyond separating
// tokens, so a multi-line expression with comments already stripped tokenizes
// the same as a single-line one.
func calcTokens(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if strings.IndexByte("()+-*/,", c) >= 0 {
			out = append(out, string(c))
			i++
			continue
		}
		j := i
		for j < len(s) && strings.IndexByte(" \t\n()+-*/,", s[j]) < 0 {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

type calcExpr struct {
	toks       []string
	pos        int
	em, vw, vh float32
}

func (p *calcExpr) sum() (float32, bool) {
	v, ok := p.product()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.toks) {
		op := p.toks[p.pos]
		if op != "+" && op != "-" {
			return v, true
		}
		p.pos++
		r, ok := p.product()
		if !ok {
			return 0, false
		}
		if op == "-" {
			v -= r
		} else {
			v += r
		}
	}
	return v, true
}

func (p *calcExpr) product() (float32, bool) {
	v, ok := p.factor()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.toks) {
		op := p.toks[p.pos]
		if op != "*" && op != "/" {
			return v, true
		}
		p.pos++
		r, ok := p.factor()
		if !ok {
			return 0, false
		}
		if op == "*" {
			v *= r
		} else if r != 0 {
			v /= r
		} else {
			return 0, false
		}
	}
	return v, true
}

func (p *calcExpr) factor() (float32, bool) {
	if p.pos >= len(p.toks) {
		return 0, false
	}
	sign := float32(1)
	if t := p.toks[p.pos]; t == "+" || t == "-" {
		if t == "-" {
			sign = -1
		}
		p.pos++
		if p.pos >= len(p.toks) {
			return 0, false
		}
	}
	if t := p.toks[p.pos]; t == "(" {
		p.pos++
		v, ok := p.sum()
		if !ok || p.pos >= len(p.toks) || p.toks[p.pos] != ")" {
			return 0, false
		}
		p.pos++
		return sign * v, true
	}
	// A bare word before a parenthesis opens another function, and its arguments
	// are evaluated in this same context so a nested `em` still means the
	// element's font.
	if p.pos+1 < len(p.toks) && p.toks[p.pos+1] == "(" && !isNumericToken(p.toks[p.pos]) {
		name := p.toks[p.pos]
		p.pos += 2
		v, ok := p.call(name)
		if !ok {
			return 0, false
		}
		return sign * v, true
	}
	tok := p.toks[p.pos]
	p.pos++
	v, ok := p.length(tok)
	if !ok {
		return 0, false
	}
	return sign * v, true
}

// call evaluates `calc()`, `min()`, `max()` or `clamp()` whose opening
// parenthesis the caller already consumed. Arguments are re-joined as text and
// recursed through evalSum, so a nested function is parsed by the same code.
func (p *calcExpr) call(name string) (float32, bool) {
	var args []string
	start, depth := p.pos, 0
	for p.pos < len(p.toks) {
		switch p.toks[p.pos] {
		case "(":
			depth++
		case ")":
			if depth == 0 {
				args = append(args, p.slice(start, p.pos))
				p.pos++
				return p.combine(name, args)
			}
			depth--
		case ",":
			if depth == 0 {
				args = append(args, p.slice(start, p.pos))
				start = p.pos + 1
			}
		}
		p.pos++
	}
	return 0, false
}

func (p *calcExpr) combine(name string, args []string) (float32, bool) {
	switch strings.ToLower(name) {
	case "calc":
		if len(args) != 1 {
			return 0, false
		}
		return evalSum(args[0], p.em, p.vw, p.vh)
	case "min":
		if len(args) < 1 {
			return 0, false
		}
		return fold(args, p.em, p.vw, p.vh, true)
	case "max":
		if len(args) < 1 {
			return 0, false
		}
		return fold(args, p.em, p.vw, p.vh, false)
	case "clamp":
		// clamp(a, b, c) is max(a, min(b, c)): the value, capped, then floored.
		if len(args) != 3 {
			return 0, false
		}
		v, ok := fold(args[1:], p.em, p.vw, p.vh, true)
		if !ok {
			return 0, false
		}
		lo, ok := evalSum(args[0], p.em, p.vw, p.vh)
		if !ok {
			return 0, false
		}
		if v < lo {
			v = lo
		}
		return v, true
	}
	return 0, false
}

func (p *calcExpr) slice(lo, hi int) string {
	return strings.Join(p.toks[lo:hi], " ")
}

func isNumericToken(tok string) bool {
	c := tok[0]
	return c >= '0' && c <= '9' || c == '.'
}

func (p *calcExpr) length(tok string) (float32, bool) {
	n := 0
	for n < len(tok) && (tok[n] >= '0' && tok[n] <= '9' || tok[n] == '.') {
		n++
	}
	if n == 0 {
		return 0, false
	}
	f, err := strconv.ParseFloat(tok[:n], 32)
	if err != nil {
		return 0, false
	}
	num := float32(f)
	switch strings.ToLower(tok[n:]) {
	case "px", "":
		return num, true
	case "em", "ex":
		return num * p.em, true
	case "ch":
		return num * p.em * 0.5, true
	case "rem":
		return num * 16, true
	case "pt":
		return num * 96 / 72, true
	case "pc":
		return num * 16, true
	case "in":
		return num * 96, true
	case "cm":
		return num * 96 / 2.54, true
	case "mm":
		return num * 96 / 25.4, true
	case "vw", "svw", "dvw", "lvw":
		return num * p.vw / 100, true
	case "vh", "svh", "dvh", "lvh":
		return num * p.vh / 100, true
	case "vmin":
		m := p.vw
		if p.vh < m {
			m = p.vh
		}
		return num * m / 100, true
	case "vmax":
		m := p.vw
		if p.vh > m {
			m = p.vh
		}
		return num * m / 100, true
	}
	return 0, false
}

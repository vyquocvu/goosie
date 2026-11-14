package css

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

func EvalCalc(expr string, em, vw, vh float32) (float32, bool) {
	return EvalFunc("calc", expr, em, vw, vh)
}

func EvalFuncWithRem(name, expr string, em, rem, vw, vh float32) (float32, bool) {
	if em <= 0 {
		em = 16
	}
	if rem <= 0 {
		rem = 16
	}
	if vw <= 0 {
		vw = 1440
	}
	if vh <= 0 {
		vh = 900
	}
	p := &calcExpr{em: em, rem: rem, vw: vw, vh: vh}
	return p.combine(name, splitArgs(expr))
}

func EvalFunc(name, expr string, em, vw, vh float32) (float32, bool) {
	return EvalFuncWithRem(name, expr, em, 16, vw, vh)
}

func EvalFuncWithBaseAndRem(name, expr string, em, rem, vw, vh, pctBase float32) (float32, bool) {
	if em <= 0 {
		em = 16
	}
	if rem <= 0 {
		rem = 16
	}
	if vw <= 0 {
		vw = 1440
	}
	if vh <= 0 {
		vh = 900
	}
	p := &calcExpr{em: em, rem: rem, vw: vw, vh: vh, pctBase: pctBase, allowPct: true}
	return p.combine(name, splitArgs(expr))
}

func EvalFuncWithBase(name, expr string, em, vw, vh, pctBase float32) (float32, bool) {
	return EvalFuncWithBaseAndRem(name, expr, em, 16, vw, vh, pctBase)
}

var viewportTerm = regexp.MustCompile(`([0-9]*\.?[0-9]+)(svmin|svmax|dvmin|dvmax|lvmin|lvmax|svw|svh|dvw|dvh|lvw|lvh|vi|vb|vw|vh|vmin|vmax)`)

var viewportTermHints = []string{"vw", "vh", "vi", "vb", "mi", "ma"}

func ResolveViewportUnits(expr string, vw, vh float32) (string, bool) {
	possible := false
	for _, h := range viewportTermHints {
		if strings.Contains(expr, h) {
			possible = true
			break
		}
	}
	if !possible {
		return expr, false
	}
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

var containerTerm = regexp.MustCompile(`([0-9]*\.?[0-9]+)(cqw|cqh|cqi|cqb|cqmin|cqmax)`)

var containerTermHints = []string{"cqw", "cqh", "cqi", "cqb", "cqmin", "cqmax"}

func ResolveContainerUnits(expr string, cqW, cqH float32) (string, bool) {
	possible := false
	for _, h := range containerTermHints {
		if strings.Contains(expr, h) {
			possible = true
			break
		}
	}
	if !possible {
		return expr, false
	}
	if cqW <= 0 {
		cqW = 1440
	}
	if cqH <= 0 {
		cqH = 900
	}
	cqmin := cqW
	if cqH < cqmin {
		cqmin = cqH
	}
	cqmax := cqW
	if cqH > cqmax {
		cqmax = cqH
	}
	found := false
	out := containerTerm.ReplaceAllStringFunc(expr, func(m string) string {
		g := containerTerm.FindStringSubmatch(m)
		n, err := strconv.ParseFloat(g[1], 32)
		if err != nil {
			return m
		}
		found = true
		var px float32
		switch g[2] {
		case "cqw", "cqi":
			px = float32(n) * cqW / 100
		case "cqh", "cqb":
			px = float32(n) * cqH / 100
		case "cqmin":
			px = float32(n) * cqmin / 100
		case "cqmax":
			px = float32(n) * cqmax / 100
		default:
			return m
		}
		return strconv.FormatFloat(float64(px), 'f', -1, 32) + "px"
	})
	return out, found
}

func (p *calcExpr) foldOne(args []string, smallest bool) (float32, bool) {
	best := float32(0)
	for i, a := range args {
		v, ok := p.evalSum(a)
		if !ok {
			return 0, false
		}
		if i == 0 || (smallest && v < best) || (!smallest && v > best) {
			best = v
		}
	}
	return best, true
}

func fold(args []string, em, vw, vh float32, smallest bool) (float32, bool) {
	p := &calcExpr{em: em, vw: vw, vh: vh}
	return p.foldOne(args, smallest)
}

func (p *calcExpr) evalSum(expr string) (float32, bool) {
	q := &calcExpr{toks: calcTokens(expr), em: p.em, rem: p.rem, vw: p.vw, vh: p.vh, pctBase: p.pctBase, allowPct: p.allowPct}
	v, ok := q.sum()
	if !ok || q.pos != len(q.toks) {
		return 0, false
	}
	return v, true
}

func evalSum(expr string, em, vw, vh float32) (float32, bool) {
	p := &calcExpr{em: em, rem: 16, vw: vw, vh: vh}
	return p.evalSum(expr)
}

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
	toks            []string
	pos             int
	em, rem, vw, vh float32
	pctBase         float32
	allowPct        bool
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
		return p.evalSum(args[0])
	case "min":
		if len(args) < 1 {
			return 0, false
		}
		return p.foldOne(args, true)
	case "max":
		if len(args) < 1 {
			return 0, false
		}
		return p.foldOne(args, false)
	case "clamp":
		if len(args) != 3 {
			return 0, false
		}
		v, ok := p.foldOne(args[1:], true)
		if !ok {
			return 0, false
		}
		lo, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		if v < lo {
			v = lo
		}
		return v, true
	case "sin":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		return float32(math.Sin(float64(v))), true
	case "cos":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		return float32(math.Cos(float64(v))), true
	case "tan":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		return float32(math.Tan(float64(v))), true
	case "asin":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok || v < -1 || v > 1 {
			return 0, false
		}
		return float32(math.Asin(float64(v))), true
	case "acos":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok || v < -1 || v > 1 {
			return 0, false
		}
		return float32(math.Acos(float64(v))), true
	case "atan":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		return float32(math.Atan(float64(v))), true
	case "atan2":
		if len(args) != 2 {
			return 0, false
		}
		y, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		x, ok := p.evalSum(args[1])
		if !ok {
			return 0, false
		}
		return float32(math.Atan2(float64(y), float64(x))), true
	case "pow":
		if len(args) != 2 {
			return 0, false
		}
		base, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		exp, ok := p.evalSum(args[1])
		if !ok {
			return 0, false
		}
		return float32(math.Pow(float64(base), float64(exp))), true
	case "sqrt":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok || v < 0 {
			return 0, false
		}
		return float32(math.Sqrt(float64(v))), true
	case "hypot":
		if len(args) < 1 {
			return 0, false
		}
		var sumSq float64
		for _, a := range args {
			v, ok := p.evalSum(a)
			if !ok {
				return 0, false
			}
			sumSq += float64(v) * float64(v)
		}
		return float32(math.Sqrt(sumSq)), true
	case "log":
		if len(args) == 1 {
			v, ok := p.evalSum(args[0])
			if !ok || v <= 0 {
				return 0, false
			}
			return float32(math.Log(float64(v))), true
		} else if len(args) == 2 {
			v, ok := p.evalSum(args[0])
			if !ok || v <= 0 {
				return 0, false
			}
			base, ok := p.evalSum(args[1])
			if !ok || base <= 0 || base == 1 {
				return 0, false
			}
			return float32(math.Log(float64(v)) / math.Log(float64(base))), true
		}
		return 0, false
	case "exp":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		return float32(math.Exp(float64(v))), true
	case "abs":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		if v < 0 {
			v = -v
		}
		return v, true
	case "sign":
		if len(args) != 1 {
			return 0, false
		}
		v, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		if v > 0 {
			return 1, true
		}
		if v < 0 {
			return -1, true
		}
		return 0, true
	case "round":
		argsToUse := args
		strat := "nearest"
		if len(args) > 0 {
			first := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(args[0])), " ", "")
			if first == "nearest" || first == "up" || first == "down" || first == "to-zero" {
				strat = first
				argsToUse = args[1:]
			}
		}
		if len(argsToUse) == 1 {
			v, ok := p.evalSum(argsToUse[0])
			if !ok {
				return 0, false
			}
			return roundWithStrategy(v, 1, strat), true
		} else if len(argsToUse) == 2 {
			v, ok := p.evalSum(argsToUse[0])
			if !ok {
				return 0, false
			}
			step, ok := p.evalSum(argsToUse[1])
			if !ok || step == 0 {
				return 0, false
			}
			return roundWithStrategy(v, step, strat), true
		}
		return 0, false
	case "mod":
		if len(args) != 2 {
			return 0, false
		}
		a, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		b, ok := p.evalSum(args[1])
		if !ok || b == 0 {
			return 0, false
		}
		return a - b*float32(math.Floor(float64(a/b))), true
	case "rem":
		if len(args) != 2 {
			return 0, false
		}
		a, ok := p.evalSum(args[0])
		if !ok {
			return 0, false
		}
		b, ok := p.evalSum(args[1])
		if !ok || b == 0 {
			return 0, false
		}
		return a - b*float32(math.Trunc(float64(a/b))), true
	}
	return 0, false
}

func roundWithStrategy(v, step float32, strat string) float32 {
	if step < 0 {
		step = -step
	}
	ratio := float64(v / step)
	switch strat {
	case "up":
		ratio = math.Ceil(ratio)
	case "down":
		ratio = math.Floor(ratio)
	case "to-zero":
		ratio = math.Trunc(ratio)
	default:
		ratio = math.Round(ratio)
	}
	return float32(ratio * float64(step))
}

func (p *calcExpr) slice(lo, hi int) string {
	return strings.Join(p.toks[lo:hi], " ")
}

func isNumericToken(tok string) bool {
	c := tok[0]
	return c >= '0' && c <= '9' || c == '.'
}

func (p *calcExpr) length(tok string) (float32, bool) {
	lower := strings.ToLower(tok)
	switch lower {
	case "pi":
		return float32(math.Pi), true
	case "e":
		return float32(math.E), true
	case "infinity", "inf":
		return float32(math.Inf(1)), true
	case "-infinity", "-inf":
		return float32(math.Inf(-1)), true
	}

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
	case "deg":
		return num * math.Pi / 180, true
	case "grad":
		return num * math.Pi / 200, true
	case "rad":
		return num, true
	case "turn":
		return num * 2 * math.Pi, true
	case "%":
		if !p.allowPct {
			return 0, false
		}
		return num * p.pctBase / 100, true
	case "em", "ex":
		return num * p.em, true
	case "ch":
		return num * p.em * 0.5, true
	case "rem":
		rem := p.rem
		if rem <= 0 {
			rem = 16
		}
		return num * rem, true
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
	case "vw", "svw", "dvw", "lvw", "vi", "cqw", "cqi":
		return num * p.vw / 100, true
	case "vh", "svh", "dvh", "lvh", "vb", "cqh", "cqb":
		return num * p.vh / 100, true
	case "vmin", "svmin", "dvmin", "lvmin", "cqmin":
		m := p.vw
		if p.vh < m {
			m = p.vh
		}
		return num * m / 100, true
	case "vmax", "svmax", "dvmax", "lvmax", "cqmax":
		m := p.vw
		if p.vh > m {
			m = p.vh
		}
		return num * m / 100, true
	}
	return 0, false
}

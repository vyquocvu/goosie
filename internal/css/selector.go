package css

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/dom"
)

func formatSel(sel Selector) string {
	var b strings.Builder
	for i, part := range sel.Parts {
		if i > 0 && part.Combinator != 0 {
			b.WriteByte(part.Combinator)
			b.WriteByte(' ')
		}
		for _, cond := range part.Conditions {
			switch cond.Type {
			case CondType:
				b.WriteString(cond.Value)
			case CondID:
				b.WriteByte('#')
				b.WriteString(cond.Value)
			case CondClass:
				b.WriteByte('.')
				b.WriteString(cond.Value)
			case CondPseudoClass:
				b.WriteByte(':')
				b.WriteString(cond.Pseudo)
				if cond.Pseudo == "has" && cond.compiled != nil && len(cond.compiled.has) > 0 {
					b.WriteByte('(')
					for h, arg := range cond.compiled.has {
						if h > 0 {
							b.WriteString(", ")
						}
						b.WriteString(formatSel(arg.sel))
					}
					b.WriteByte(')')
				}
			}
		}
	}
	return b.String()
}

type Selector struct {
	Parts []SelectorPart
}

type SelectorPart struct {
	Combinator byte
	Conditions []Condition
}

type Condition struct {
	Type            ConditionType
	Value           string
	Attr            string
	Op              string
	Pseudo          string
	CaseInsensitive bool
	compiled        *pseudoCompiled
	scope           *dom.Node
}

type pseudoCompiled struct {
	sels    []Selector
	has     []hasArg
	of      []Selector
	nthExpr string
	hasNth  bool
}

type hasArg struct {
	combinator byte
	sel        Selector
}

func (c *Condition) precompile() {
	if c.Type != CondPseudoClass || c.Pseudo == "" {
		return
	}
	switch c.Value {
	case "is", "where":
		var sels []Selector
		for _, selStr := range splitSelectorList(c.Pseudo) {
			if sel, ok := parseSelector(selStr); ok && !hasPseudoElement(sel) {
				sels = append(sels, sel)
			}
		}
		c.compiled = &pseudoCompiled{sels: sels}
	case "not", "host":
		var sels []Selector
		for _, selStr := range splitSelectorList(c.Pseudo) {
			sels = append(sels, ParseSelector(selStr))
		}
		c.compiled = &pseudoCompiled{sels: sels}
	case "has":
		c.compiled = &pseudoCompiled{has: compileHasArgs(c.Pseudo)}
	case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
		if _, of := splitNthOf(c.Pseudo); len(of) > 0 {
			pc := &pseudoCompiled{hasNth: true}
			pc.nthExpr, _ = splitNthOf(c.Pseudo)
			for _, selStr := range of {
				pc.of = append(pc.of, ParseSelector(selStr))
			}
			c.compiled = pc
		}
	}
}

func compileHasArgs(arg string) []hasArg {
	var out []hasArg
	for _, selStr := range splitSelectorList(arg) {
		selStr = strings.TrimSpace(selStr)
		if selStr == "" {
			continue
		}
		combinator := byte(' ')
		rest := selStr
		switch selStr[0] {
		case '>':
			combinator = '>'
			rest = strings.TrimSpace(selStr[1:])
		case '+':
			combinator = '+'
			rest = strings.TrimSpace(selStr[1:])
		case '~':
			combinator = '~'
			rest = strings.TrimSpace(selStr[1:])
		}
		if rest == "" {
			continue
		}
		out = append(out, hasArg{combinator: combinator, sel: ParseSelector(rest)})
	}
	return out
}

type ConditionType uint8

const (
	CondUniversal ConditionType = iota
	CondType
	CondClass
	CondID
	CondAttr
	CondPseudoClass
	CondPseudoElement
)

func (s Selector) Specificity() (a, b, c int) {
	for _, part := range s.Parts {
		for _, cond := range part.Conditions {
			switch cond.Type {
			case CondID:
				a++
			case CondClass, CondAttr:
				b++
			case CondPseudoClass:
				if cond.Value == "where" {
					continue
				}
				if cond.Value == "is" || cond.Value == "not" || cond.Value == "host" {
					if cond.Value == "host" {
						b++
					}
					var sels []Selector
					if cond.compiled != nil {
						sels = cond.compiled.sels
					} else if cond.Pseudo != "" {
						for _, selStr := range splitSelectorList(cond.Pseudo) {
							sels = append(sels, ParseSelector(selStr))
						}
					}
					maxA, maxB, maxC := 0, 0, 0
					for _, sel := range sels {
						sa, sb, sc := sel.Specificity()
						if CompareSpecificity(sa, sb, sc, maxA, maxB, maxC) > 0 {
							maxA, maxB, maxC = sa, sb, sc
						}
					}
					a += maxA
					b += maxB
					c += maxC
					continue
				}
				if cond.Value == "has" {
					var args []hasArg
					if cond.compiled != nil && len(cond.compiled.has) > 0 {
						args = cond.compiled.has
					} else if cond.Pseudo != "" {
						args = compileHasArgs(cond.Pseudo)
					}
					maxA, maxB, maxC := 0, 0, 0
					for _, arg := range args {
						sa, sb, sc := arg.sel.Specificity()
						if CompareSpecificity(sa, sb, sc, maxA, maxB, maxC) > 0 {
							maxA, maxB, maxC = sa, sb, sc
						}
					}
					a += maxA
					b += maxB
					c += maxC
					continue
				}
				b++
				switch cond.Value {
				case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
					if _, of := splitNthOf(cond.Pseudo); len(of) > 0 {
						maxA, maxB, maxC := 0, 0, 0
						for _, selStr := range of {
							sel := ParseSelector(selStr)
							sa, sb, sc := sel.Specificity()
							if CompareSpecificity(sa, sb, sc, maxA, maxB, maxC) > 0 {
								maxA, maxB, maxC = sa, sb, sc
							}
						}
						a += maxA
						b += maxB
						c += maxC
					}
				}
			case CondType, CondPseudoElement:
				c++
			}
		}
	}
	return
}

func CompareSpecificity(a1, b1, c1, a2, b2, c2 int) int {
	if a1 != a2 {
		if a1 > a2 {
			return 1
		}
		return -1
	}
	if b1 != b2 {
		if b1 > b2 {
			return 1
		}
		return -1
	}
	if c1 != c2 {
		if c1 > c2 {
			return 1
		}
		return -1
	}
	return 0
}

func ParseSelector(s string) Selector {
	sel, _ := parseSelector(s)
	return sel
}

func ParseSelectorList(s string) ([]Selector, error) {
	if s == "" {
		return nil, errors.New("css: the provided selector is empty")
	}
	alts, ok := splitSelectorListExact(s)
	if !ok || len(alts) == 0 {
		return nil, fmt.Errorf("css: %q is not a valid selector list", s)
	}
	out := make([]Selector, 0, len(alts))
	for _, alt := range alts {
		sel, ok := parseSelector(alt)
		if !ok {
			return nil, fmt.Errorf("css: %q is not a valid selector", s)
		}
		out = append(out, sel)
	}
	return out, nil
}

func parseSelector(s string) (Selector, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Selector{}, false
	}

	var parts []SelectorPart
	var current SelectorPart
	i := 0
	ok := true

	for i < len(s) {
		skipSpace := false
		for i < len(s) && s[i] == ' ' {
			skipSpace = true
			i++
		}

		if i >= len(s) {
			break
		}

		combinator := byte(0)
		if skipSpace && len(current.Conditions) > 0 {
			combinator = ' '
		}

		if i < len(s) {
			switch s[i] {
			case '>', '+', '~':
				if len(current.Conditions) == 0 {
					ok = false
				}
				combinator = s[i]
				i++
				for i < len(s) && s[i] == ' ' {
					i++
				}
			}
		}

		if i >= len(s) {
			ok = false
			break
		}

		if combinator != 0 && len(current.Conditions) > 0 {
			parts = append(parts, current)
			current = SelectorPart{Combinator: combinator}
		}

		condStart := i
		cond, newI := parseCondition(s, i)
		if newI == i {
			ok = false
			break
		}
		i = newI
		if !validCondition(cond, s[condStart:newI]) {
			ok = false
		}
		current.Conditions = append(current.Conditions, cond)
	}

	if len(current.Conditions) > 0 {
		parts = append(parts, current)
	}

	return Selector{Parts: parts}, ok
}

func validCondition(c Condition, raw string) bool {
	switch c.Type {
	case CondUniversal:
		return true
	case CondType:
		return validIdent(raw)
	case CondID, CondClass:
		if len(raw) < 2 {
			return false
		}
		return validIdent(raw[1:])
	case CondAttr:
		return validAttrCondition(c, raw)
	case CondPseudoClass:
		return validPseudoClass(c)
	case CondPseudoElement:
		return validPseudoElement(c)
	}
	return true
}

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' {
		if len(s) < 2 || (s[1] != '-' && !isIdentStart(s[1]) && !isValidCSSEscapeAt(s, 1)) {
			return false
		}
	} else if !isIdentStart(s[0]) && !isValidCSSEscapeAt(s, 0) {
		return false
	}
	_, end, ok := consumeCSSIdent(s, 0)
	return ok && end == len(s)
}

func isValidCSSEscapeAt(s string, i int) bool {
	return i+1 < len(s) && s[i] == '\\' && s[i+1] != '\n' && s[i+1] != '\r' && s[i+1] != '\f'
}

func consumeCSSIdent(s string, start int) (string, int, bool) {
	var out strings.Builder
	i := start
	for i < len(s) {
		if isIdentChar(s[i]) {
			out.WriteByte(s[i])
			i++
			continue
		}
		if s[i] != '\\' {
			break
		}
		if !isValidCSSEscapeAt(s, i) {
			return out.String(), i, false
		}
		i++
		if isCSSHex(s[i]) {
			begin := i
			for i < len(s) && i-begin < 6 && isCSSHex(s[i]) {
				i++
			}
			value, _ := strconv.ParseUint(s[begin:i], 16, 32)
			r := rune(value)
			if r == 0 || r > utf8.MaxRune || (r >= 0xD800 && r <= 0xDFFF) {
				r = utf8.RuneError
			}
			out.WriteRune(r)
			if i < len(s) && isCSSWhitespace(s[i]) {
				if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
					i++
				}
				i++
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		out.WriteRune(r)
		i += size
	}
	return out.String(), i, true
}

func isCSSHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func isCSSWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func validAttrCondition(c Condition, raw string) bool {
	if !validIdent(c.Attr) {
		return false
	}
	if c.Op == "" {
		return true
	}
	inner := strings.TrimSpace(raw)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	eq := strings.IndexByte(inner, '=')
	if eq < 0 || eq+1 >= len(inner) {
		return false
	}
	val := strings.TrimSpace(inner[eq+1:])
	if n := len(val); n >= 2 && (val[n-2] == ' ' || val[n-2] == '\t') &&
		(val[n-1] == 'i' || val[n-1] == 'I' || val[n-1] == 's' || val[n-1] == 'S') {
		if val[n-1] == 's' || val[n-1] == 'S' {
			return false
		}
		val = strings.TrimSpace(val[:n-1])
	}
	if val == "" {
		return false
	}
	if val[0] == '"' || val[0] == '\'' {
		return strings.TrimSpace(val[quotedRunEnd(val, 0):]) == ""
	}
	return validIdent(val)
}

func validPseudoClass(c Condition) bool {
	switch strings.ToLower(c.Value) {
	case "first-child", "last-child", "only-child", "first-of-type",
		"last-of-type", "only-of-type", "root", "empty",
		"link", "any-link", "visited", "active", "focus-visible",
		"hover", "focus", "focus-within", "scope", "popover-open",
		"checked", "disabled", "enabled", "placeholder-shown",
		"modal", "defined", "target", "valid", "invalid", "required",
		"optional", "read-only", "read-write", "default", "indeterminate",
		"in-range", "out-of-range", "open", "picture-in-picture",
		"fullscreen", "autofill", "user-invalid", "user-valid", "current",
		"past", "future", "active-view-transition", "host":
		return true
	case "lang", "dir", "host-context", "active-view-transition-type":
		return strings.TrimSpace(c.Pseudo) != ""
	case "state":
		return validIdent(strings.TrimSpace(c.Pseudo))
	case "not":
		return validSelectorList(c.Pseudo, false)
	case "is", "where":
		return validForgivingSelectorList(c.Pseudo)
	case "has":
		return validSelectorList(c.Pseudo, true)
	case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
		expr, of := splitNthOf(c.Pseudo)
		if _, _, ok := parseNth(strings.TrimSpace(expr)); !ok {
			return false
		}
		if len(of) == 0 {
			return !nthHasOfClause(c.Pseudo)
		}
		return validSelectorList(strings.Join(of, ","), false)
	}
	return false
}

func validPseudoElement(c Condition) bool {
	switch strings.ToLower(c.Value) {
	case "before", "after", "first-line", "first-letter", "marker",
		"placeholder", "backdrop", "file-selector-button", "cue",
		"grammar-error", "spelling-error", "target-text", "view-transition",
		"selection":
		return c.Pseudo == ""
	case "part", "slotted", "highlight":
		return strings.TrimSpace(c.Pseudo) != ""
	}
	return false
}

func validSelectorList(s string, relative bool) bool {
	alts, ok := splitSelectorListExact(s)
	if !ok || len(alts) == 0 {
		return false
	}
	for _, alt := range alts {
		if alt == "" {
			return false
		}
		if relative {
			alt = strings.TrimSpace(strings.TrimLeft(alt, ">+~"))
			if alt == "" {
				return false
			}
		}
		if sel, ok := parseSelector(alt); !ok || hasPseudoElement(sel) {
			return false
		}
	}
	return true
}

func validForgivingSelectorList(s string) bool {
	_, splitOK := splitSelectorListExact(s)
	return splitOK
}

func hasPseudoElement(sel Selector) bool {
	for _, part := range sel.Parts {
		for _, c := range part.Conditions {
			if c.Type == CondPseudoElement {
				return true
			}
		}
	}
	return false
}

func nthHasOfClause(arg string) bool {
	for i := 0; i+4 <= len(arg); i++ {
		if arg[i] != ' ' && arg[i] != '\t' && arg[i] != '\n' {
			continue
		}
		j := i + 1
		for j < len(arg) && (arg[j] == ' ' || arg[j] == '\t' || arg[j] == '\n') {
			j++
		}
		if j+2 <= len(arg) && strings.EqualFold(arg[j:j+2], "of") {
			if j+2 == len(arg) || arg[j+2] == ' ' || arg[j+2] == '\t' || arg[j+2] == '\n' {
				return true
			}
		}
	}
	return false
}

func unquoteCSSValue(v string) string {
	if n := len(v); n >= 2 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] && quotedRunEnd(v, 0) == n {
		return DecodeEscapes(v[1 : n-1])
	}
	return DecodeEscapes(v)
}

func quotedRunEnd(s string, i int) int {
	quote := s[i]
	i++
	for i < len(s) && s[i] != quote {
		if s[i] == '\\' {
			i++
		}
		i++
	}
	if i < len(s) {
		i++
	}
	return i
}

func attrConditionEnd(s string, i int) int {
	for i < len(s) {
		switch c := s[i]; c {
		case '\\':
			i += 2
		case '"', '\'':
			i = quotedRunEnd(s, i)
		case ']':
			return i
		default:
			i++
		}
	}
	return len(s)
}

func parseCondition(s string, i int) (Condition, int) {
	if i >= len(s) {
		return Condition{}, i
	}

	switch s[i] {
	case '#':
		i++
		value, end, _ := consumeCSSIdent(s, i)
		return Condition{Type: CondID, Value: value}, end

	case '.':
		i++
		value, end, _ := consumeCSSIdent(s, i)
		return Condition{Type: CondClass, Value: value}, end

	case '[':
		i++
		start := i
		i = attrConditionEnd(s, i)
		attr := s[start:i]
		if i < len(s) {
			i++
		}
		if eqIdx := strings.IndexAny(attr, "=~|^$*"); eqIdx >= 0 {
			name := strings.TrimSpace(attr[:eqIdx])
			rest := attr[eqIdx:]
			op := "="
			val := rest[1:]
			if len(rest) > 1 && rest[1] == '=' {
				op = rest[:2]
				val = rest[2:]
			}
			valStr := strings.TrimSpace(val)
			caseInsensitive := false
			if strings.HasSuffix(valStr, " i") || strings.HasSuffix(valStr, " I") {
				caseInsensitive = true
				valStr = strings.TrimSpace(valStr[:len(valStr)-2])
			} else if strings.HasSuffix(valStr, " s") || strings.HasSuffix(valStr, " S") {
				valStr = strings.TrimSpace(valStr[:len(valStr)-2])
			}
			valStr = unquoteCSSValue(valStr)
			return Condition{Type: CondAttr, Attr: name, Op: op, Value: valStr, CaseInsensitive: caseInsensitive}, i
		}
		return Condition{Type: CondAttr, Attr: strings.TrimSpace(attr)}, i

	case ':':
		i++
		if i < len(s) && s[i] == ':' {
			i++
			start := i
			for i < len(s) && isIdentChar(s[i]) {
				i++
			}
			name := s[start:i]
			arg := ""
			if i < len(s) && s[i] == '(' {
				i++
				depth := 1
				argStart := i
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
				arg = s[argStart:i]
				if i < len(s) {
					i++
				}
			}
			return Condition{Type: CondPseudoElement, Value: name, Pseudo: arg}, i
		}
		start := i
		for i < len(s) && isIdentChar(s[i]) {
			i++
		}
		name := s[start:i]
		if i < len(s) && s[i] == '(' {
			i++
			depth := 1
			argStart := i
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
			arg := strings.TrimSpace(s[argStart:i])
			if i < len(s) {
				i++
			}
			cond := Condition{Type: CondPseudoClass, Value: name, Pseudo: arg}
			cond.precompile()
			return cond, i
		}
		switch strings.ToLower(name) {
		case "before", "after", "first-line", "first-letter":
			return Condition{Type: CondPseudoElement, Value: strings.ToLower(name)}, i
		}
		return Condition{Type: CondPseudoClass, Value: name}, i

	case '*':
		i++
		return Condition{Type: CondUniversal}, i

	default:
		if isIdentStart(s[i]) || s[i] == '-' || s[i] == '\\' {
			value, end, _ := consumeCSSIdent(s, i)
			return Condition{Type: CondType, Value: strings.ToLower(value)}, end
		}
	}

	return Condition{}, i
}

func (s Selector) Matches(n *dom.Node) bool {
	return s.MatchesIndexed(n, nil)
}

func (s Selector) MatchesPart(host, target *dom.Node) bool {
	if host == nil || target == nil || len(s.Parts) == 0 {
		return false
	}
	partName := ""
	partPart, partCond := -1, -1
	for i, part := range s.Parts {
		for j, cond := range part.Conditions {
			if cond.Type == CondPseudoElement && cond.Value == "part" {
				partPart, partCond, partName = i, j, cond.Pseudo
				break
			}
		}
	}
	if partPart < 0 || partName == "" {
		return false
	}
	found := false
	for _, name := range strings.Fields(target.GetAttribute("part")) {
		if name == partName {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	before := Selector{Parts: make([]SelectorPart, partPart+1)}
	copy(before.Parts, s.Parts[:partPart+1])
	before.Parts[partPart].Conditions = append([]Condition(nil), s.Parts[partPart].Conditions[:partCond]...)
	if len(before.Parts[partPart].Conditions) == 0 {
		before.Parts[partPart].Conditions = []Condition{{Type: CondUniversal}}
	}
	if !before.MatchesIndexed(host, nil) {
		return false
	}
	return matchConditions(s.Parts[partPart].Conditions[partCond+1:], target, nil)
}

func (s Selector) WithScope(root *dom.Node) Selector {
	out := Selector{Parts: make([]SelectorPart, len(s.Parts))}
	for i, part := range s.Parts {
		p := SelectorPart{Combinator: part.Combinator, Conditions: make([]Condition, len(part.Conditions))}
		for j, c := range part.Conditions {
			c.scope = root
			c.compiled = scopeCompiled(c.compiled, root)
			p.Conditions[j] = c
		}
		out.Parts[i] = p
	}
	return out
}

func scopeCompiled(pc *pseudoCompiled, root *dom.Node) *pseudoCompiled {
	if pc == nil {
		return nil
	}
	out := &pseudoCompiled{nthExpr: pc.nthExpr, hasNth: pc.hasNth}
	for _, sel := range pc.sels {
		out.sels = append(out.sels, sel.WithScope(root))
	}
	for _, arg := range pc.has {
		out.has = append(out.has, hasArg{combinator: arg.combinator, sel: arg.sel.WithScope(root)})
	}
	for _, sel := range pc.of {
		out.of = append(out.of, sel.WithScope(root))
	}
	return out
}

func (s Selector) UsesScope() bool {
	for _, part := range s.Parts {
		for _, c := range part.Conditions {
			if c.Type != CondPseudoClass {
				continue
			}
			if c.Value == "scope" {
				return true
			}
			if c.compiled == nil {
				continue
			}
			for _, sub := range c.compiled.sels {
				if sub.UsesScope() {
					return true
				}
			}
			for _, arg := range c.compiled.has {
				if arg.sel.UsesScope() {
					return true
				}
			}
			for _, sub := range c.compiled.of {
				if sub.UsesScope() {
					return true
				}
			}
		}
	}
	return false
}

func SelectorsUseScope(sels []Selector) bool {
	for _, sel := range sels {
		if sel.UsesScope() {
			return true
		}
	}
	return false
}

func (s Selector) MatchesIndexed(n *dom.Node, ix *SiblingIndex) bool {
	if len(s.Parts) == 0 {
		return false
	}
	return matchParts(s.Parts, len(s.Parts)-1, n, ix)
}

func (s Selector) MatchesPseudoElement(n *dom.Node, pseudo string) bool {
	return s.MatchesPseudoElementIndexed(n, pseudo, nil)
}

func (s Selector) MatchesPseudoElementIndexed(n *dom.Node, pseudo string, ix *SiblingIndex) bool {
	if len(s.Parts) == 0 {
		return false
	}
	key := s.Parts[len(s.Parts)-1]
	hasPseudo := false
	for _, c := range key.Conditions {
		if c.Type == CondPseudoElement && c.Value == pseudo {
			hasPseudo = true
			break
		}
	}
	if !hasPseudo {
		return false
	}
	return matchPartsPseudo(s.Parts, len(s.Parts)-1, n, pseudo, ix)
}

func matchPartsPseudo(parts []SelectorPart, idx int, n *dom.Node, pseudo string, ix *SiblingIndex) bool {
	if idx < 0 || n == nil || !n.Element() {
		return false
	}

	part := parts[idx]
	if !matchConditionsPseudo(part.Conditions, n, pseudo, idx == len(parts)-1, ix) {
		return false
	}

	if idx == 0 {
		return true
	}

	switch part.Combinator {
	case ' ':
		for p := n.Parent; p != nil; p = p.Parent {
			if matchPartsPseudo(parts, idx-1, p, pseudo, ix) {
				return true
			}
		}
		return false
	case '>':
		return matchPartsPseudo(parts, idx-1, n.Parent, pseudo, ix)
	case '+':
		sib := prevElement(n)
		return sib != nil && matchPartsPseudo(parts, idx-1, sib, pseudo, ix)
	case '~':
		for sib := prevElement(n); sib != nil; sib = prevElement(sib) {
			if matchPartsPseudo(parts, idx-1, sib, pseudo, ix) {
				return true
			}
		}
		return false
	}

	return false
}

func matchConditionsPseudo(conds []Condition, n *dom.Node, pseudo string, isKey bool, ix *SiblingIndex) bool {
	for _, c := range conds {
		if isKey && c.Type == CondPseudoElement && c.Value == pseudo {
			continue
		}
		if !matchCondition(c, n, ix) {
			return false
		}
	}
	return true
}

func matchParts(parts []SelectorPart, idx int, n *dom.Node, ix *SiblingIndex) bool {
	if idx < 0 || n == nil || !n.Element() {
		return false
	}

	part := parts[idx]
	if !matchConditions(part.Conditions, n, ix) {
		return false
	}

	if idx == 0 {
		return true
	}

	switch part.Combinator {
	case ' ':
		for p := n.Parent; p != nil; p = p.Parent {
			if matchParts(parts, idx-1, p, ix) {
				return true
			}
		}
		return false
	case '>':
		return matchParts(parts, idx-1, n.Parent, ix)
	case '+':
		sib := prevElement(n)
		return sib != nil && matchParts(parts, idx-1, sib, ix)
	case '~':
		for sib := prevElement(n); sib != nil; sib = prevElement(sib) {
			if matchParts(parts, idx-1, sib, ix) {
				return true
			}
		}
		return false
	}

	return false
}

func prevElement(n *dom.Node) *dom.Node {
	for s := n.PrevSibling; s != nil; s = s.PrevSibling {
		if s.Element() {
			return s
		}
	}
	return nil
}

func matchConditions(conds []Condition, n *dom.Node, ix *SiblingIndex) bool {
	for _, c := range conds {
		if !matchCondition(c, n, ix) {
			return false
		}
	}
	return true
}

func matchCondition(c Condition, n *dom.Node, ix *SiblingIndex) bool {
	switch c.Type {
	case CondUniversal:
		return true
	case CondType:
		return n.Data == c.Value
	case CondClass:
		for _, cls := range n.ClassList() {
			if cls == c.Value {
				return true
			}
		}
		return false
	case CondID:
		return n.GetAttribute("id") == c.Value
	case CondAttr:
		return matchAttr(c, n)
	case CondPseudoClass:
		return matchPseudoClass(c, n, ix)
	case CondPseudoElement:
		return false
	}
	return false
}

func matchAttr(c Condition, n *dom.Node) bool {
	if c.Op == "" {
		return n.HasAttribute(c.Attr)
	}
	val := n.GetAttribute(c.Attr)
	targetVal := c.Value
	if c.CaseInsensitive {
		val = strings.ToLower(val)
		targetVal = strings.ToLower(targetVal)
	}
	switch c.Op {
	case "=":
		return val == targetVal
	case "~=":
		for _, v := range strings.Fields(val) {
			if v == targetVal {
				return true
			}
		}
		return false
	case "|=":
		return val == targetVal || strings.HasPrefix(val, targetVal+"-")
	case "^=":
		return strings.HasPrefix(val, targetVal)
	case "$=":
		return strings.HasSuffix(val, targetVal)
	case "*=":
		return strings.Contains(val, targetVal)
	}
	return false
}

func matchPseudoClass(c Condition, n *dom.Node, ix *SiblingIndex) bool {
	switch c.Value {
	case "first-child":
		return isFirstElementChild(n)
	case "last-child":
		return isLastElementChild(n)
	case "only-child":
		return isFirstElementChild(n) && isLastElementChild(n)
	case "root":
		return n.Parent != nil && n.Parent.Type == dom.NodeDocument
	case "empty":
		return n.FirstChild == nil
	case "defined":
		return n.Namespace != dom.NSHTML || !n.CustomElementUndefined
	case "not":
		if c.Pseudo == "" {
			return true
		}
		if c.compiled != nil && len(c.compiled.sels) > 0 {
			for _, sel := range c.compiled.sels {
				if sel.MatchesIndexed(n, ix) {
					return false
				}
			}
			return true
		}
		selectors := splitSelectorList(c.Pseudo)
		for _, selStr := range selectors {
			sel := ParseSelector(selStr)
			if sel.MatchesIndexed(n, ix) {
				return false
			}
		}
		return true
	case "is", "where":
		if c.Pseudo == "" {
			return false
		}
		if c.compiled != nil {
			for _, sel := range c.compiled.sels {
				if sel.MatchesIndexed(n, ix) {
					return true
				}
			}
			return false
		}
		selectors := splitSelectorList(c.Pseudo)
		for _, selStr := range selectors {
			sel := ParseSelector(selStr)
			if sel.MatchesIndexed(n, ix) {
				return true
			}
		}
		return false
	case "link", "any-link":
		tag := strings.ToLower(n.Data)
		return (tag == "a" || tag == "area" || tag == "link") && n.HasAttribute("href")
	case "lang":
		return matchLang(c.Pseudo, n)
	case "dir":
		return matchesDirectionality(n, c.Pseudo)
	case "state":
		return n.CustomStates != nil && n.CustomStates[c.Pseudo]
	case "visited":
		return false
	case "active":
		return IsActive(n)
	case "focus-visible":
		return IsFocusVisible(n)
	case "host":
		if n.ShadowRoot == nil {
			return false
		}
		if c.Pseudo == "" {
			return true
		}
		if c.compiled != nil && len(c.compiled.sels) > 0 {
			for _, sel := range c.compiled.sels {
				if sel.MatchesIndexed(n, ix) {
					return true
				}
			}
			return false
		}
		selectors := splitSelectorList(c.Pseudo)
		for _, selStr := range selectors {
			sel := ParseSelector(selStr)
			if sel.MatchesIndexed(n, ix) {
				return true
			}
		}
		return false
	case "hover":
		return IsHovered(n)
	case "focus":
		return IsFocused(n)
	case "focus-within":
		return matchesFocusWithin(n)
	case "scope":
		if c.scope != nil {
			return n == c.scope
		}
		return n.IsScopeRoot
	case "popover-open":
		return n.IsPopoverOpen
	case "modal":
		return n.Data == "dialog" && n.IsModal && n.HasAttribute("open")
	case "checked":
		return matchesChecked(n)
	case "required":
		return isConstraintControl(n) && n.HasAttribute("required")
	case "optional":
		return isConstraintControl(n) && !n.HasAttribute("required")
	case "invalid":
		if n.Data == "form" || n.Data == "fieldset" {
			return formHasInvalidControl(n)
		}
		return isConstraintControl(n) && constraintControlInvalid(n)
	case "valid":
		if n.Data == "form" || n.Data == "fieldset" {
			return !formHasInvalidControl(n)
		}
		return isConstraintControl(n) && !constraintControlInvalid(n)
	case "user-invalid":
		if n.Data == "form" || n.Data == "fieldset" {
			return n.UserInteracted && formHasInvalidControl(n)
		}
		return isConstraintControl(n) && n.UserInteracted && constraintControlInvalid(n)
	case "user-valid":
		if n.Data == "form" || n.Data == "fieldset" {
			return n.UserInteracted && !formHasInvalidControl(n)
		}
		return isConstraintControl(n) && n.UserInteracted && !constraintControlInvalid(n)
	case "in-range":
		return constraintControlInRange(n, false)
	case "out-of-range":
		return constraintControlInRange(n, true)
	case "disabled":
		return isDisableable(n) && isDisabled(n)
	case "enabled":
		return isDisableable(n) && !isDisabled(n)
	case "read-write":
		return matchesReadWrite(n)
	case "read-only":
		return n != nil && n.Element() && !matchesReadWrite(n)
	case "placeholder-shown":
		return placeholderIsShown(n)
	case "has":
		if c.Pseudo == "" {
			return false
		}
		if c.compiled != nil {
			return matchHasCompiled(c.compiled.has, n, ix)
		}
		return matchHas(c.Pseudo, n, ix)
	case "nth-child":
		return matchNthC(c, n, false, false, ix)
	case "nth-last-child":
		return matchNthC(c, n, false, true, ix)
	case "nth-of-type":
		return matchNthC(c, n, true, false, ix)
	case "nth-last-of-type":
		return matchNthC(c, n, true, true, ix)
	case "first-of-type":
		return matchNth("1", n, true, false, ix)
	case "last-of-type":
		return matchNth("1", n, true, true, ix)
	case "only-of-type":
		return matchNth("1", n, true, false, ix) && matchNth("1", n, true, true, ix)
	}
	return false
}

func matchesReadWrite(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	if n.Namespace == dom.NSHTML && n.Data == "input" {
		if isDisabled(n) || n.HasAttribute("readonly") {
			return false
		}
		switch strings.ToLower(n.GetAttribute("type")) {
		case "", "text", "search", "url", "tel", "email", "password", "date", "month", "week", "time", "datetime-local", "number":
			return true
		default:
			return false
		}
	}
	if n.Namespace == dom.NSHTML && n.Data == "textarea" {
		return !isDisabled(n) && !n.HasAttribute("readonly")
	}
	return dom.IsContentEditable(n)
}

func isConstraintControl(n *dom.Node) bool {
	if n == nil || n.Namespace != dom.NSHTML {
		return false
	}
	if n.FormAssociatedCustomElement {
		return true
	}
	switch n.Data {
	case "textarea", "select":
		return true
	case "input":
		switch strings.ToLower(n.GetAttribute("type")) {
		case "hidden", "button", "reset", "submit", "image":
			return false
		default:
			return true
		}
	default:
		return false
	}
}

func constraintControlInvalid(n *dom.Node) bool {
	if n.FormAssociatedCustomElement {
		return !n.CustomElementValid
	}
	if n.HasAttribute("disabled") || n.HasAttribute("readonly") {
		return false
	}
	if n.CustomValidityMessage != "" || n.PatternMismatch || n.TypeMismatch || n.RangeUnderflow || n.RangeOverflow || n.StepMismatch || dom.IsTooLong(n) || dom.IsTooShort(n) {
		return true
	}
	if !n.HasAttribute("required") {
		return false
	}
	if n.Data == "input" {
		switch strings.ToLower(n.GetAttribute("type")) {
		case "checkbox":
			return !n.InputChecked()
		case "radio":
			name := n.GetAttribute("name")
			root := n
			for root.Parent != nil {
				root = root.Parent
			}
			checked := false
			_ = root.Walk(func(candidate *dom.Node) bool {
				if candidate.Data == "input" && strings.EqualFold(candidate.GetAttribute("type"), "radio") && candidate.GetAttribute("name") == name && candidate.InputChecked() {
					checked = true
					return false
				}
				return true
			})
			return !checked
		case "file":
			return n.InputFileCount == 0
		}
	}
	value := n.InputControlValue()
	if n.Data == "select" {
		value = ""
		for _, option := range dom.SelectOptions(n) {
			if option.OptionSelected() {
				value = option.GetAttribute("value")
				if value == "" {
					value = strings.TrimSpace(option.TextContent())
				}
				break
			}
		}
	}
	return value == ""
}

func constraintControlInRange(n *dom.Node, outOfRange bool) bool {
	if n == nil {
		return false
	}
	typ := strings.ToLower(n.GetAttribute("type"))
	if !isConstraintControl(n) || isDisabled(n) || n.HasAttribute("readonly") || n.Data != "input" || !isRangeLimitedInputType(typ) || !n.RangeLimited {
		return false
	}
	out := n.RangeUnderflow || n.RangeOverflow
	if outOfRange {
		return out
	}
	return !out
}

func isRangeLimitedInputType(typ string) bool {
	switch typ {
	case "number", "date", "month", "week", "time", "datetime-local":
		return true
	default:
		return false
	}
}

func formHasInvalidControl(form *dom.Node) bool {
	if form == nil {
		return false
	}
	invalid := false
	var walk func(*dom.Node)
	walk = func(parent *dom.Node) {
		for child := parent.FirstChild; child != nil && !invalid; child = child.NextSibling {
			if isConstraintControl(child) && constraintControlInvalid(child) {
				if form.Data == "fieldset" {
					invalid = true
					return
				}
				owner := child
				if id, hasForm := child.GetAttributeNS("", "form"); hasForm && id != "" && child.Doc != nil {
					owner = child.Doc.ElementByID(id)
				} else {
					for ancestor := child.Parent; ancestor != nil; ancestor = ancestor.Parent {
						if ancestor.Data == "form" {
							owner = ancestor
							break
						}
					}
				}
				if owner == form {
					invalid = true
					return
				}
			}
			walk(child)
		}
	}
	walk(form)
	return invalid
}

func matchesDirectionality(n *dom.Node, argument string) bool {
	want := strings.ToLower(strings.TrimSpace(argument))
	if want != "ltr" && want != "rtl" {
		return false
	}
	return n != nil && n.Directionality() == want
}

func matchesChecked(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		return (typ == "checkbox" || typ == "radio") && n.InputChecked()
	case "option":
		if selectNode := dom.NearestSelect(n); selectNode != nil {
			dom.NormalizeSelectSelectedness(selectNode)
		}
		return n.OptionSelected()
	}
	return false
}

func matchesFocusWithin(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	if IsFocused(n) {
		return true
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if matchesFocusWithin(child) {
			return true
		}
	}
	return false
}

func isDisableable(n *dom.Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	switch n.Data {
	case "button", "fieldset", "input", "optgroup", "option", "select", "textarea":
		return true
	default:
		return n.FormAssociatedCustomElement
	}
}

func isDisabled(n *dom.Node) bool {
	if n == nil || !isDisableable(n) {
		return false
	}
	if n.HasAttribute("disabled") {
		return true
	}
	if dom.IsDisabledByFieldset(n) {
		return true
	}
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if n.Data == "option" && (parent.Data == "optgroup" || parent.Data == "select") && parent.HasAttribute("disabled") {
			return true
		}
	}
	return false
}

func placeholderIsShown(n *dom.Node) bool {
	if n == nil || !n.Element() || n.GetAttribute("placeholder") == "" {
		return false
	}
	switch n.Data {
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "", "text", "password", "email", "tel", "url", "search", "number":
			return n.InputControlValue() == ""
		default:
			return false
		}
	case "textarea":
		return n.InputControlValue() == ""
	default:
		return false
	}
}

func IsPlaceholderShown(n *dom.Node) bool { return placeholderIsShown(n) }

func matchHas(arg string, n *dom.Node, ix *SiblingIndex) bool {
	selectors := splitSelectorList(arg)
	for _, selStr := range selectors {
		selStr = strings.TrimSpace(selStr)
		if selStr == "" {
			continue
		}
		combinator := byte(' ')
		rest := selStr
		if len(selStr) > 0 {
			switch selStr[0] {
			case '>':
				combinator = '>'
				rest = strings.TrimSpace(selStr[1:])
			case '+':
				combinator = '+'
				rest = strings.TrimSpace(selStr[1:])
			case '~':
				combinator = '~'
				rest = strings.TrimSpace(selStr[1:])
			}
		}
		if rest == "" {
			continue
		}
		sel := ParseSelector(rest)
		switch combinator {
		case '>':
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == dom.NodeElement && sel.MatchesIndexed(c, ix) {
					return true
				}
			}
		case '+':
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement {
					if sel.MatchesIndexed(sib, ix) {
						return true
					}
					break
				}
			}
		case '~':
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement && sel.MatchesIndexed(sib, ix) {
					return true
				}
			}
		default:
			if hasDescendantMatching(n, sel, ix) {
				return true
			}
		}
	}
	return false
}

func matchHasCompiled(args []hasArg, n *dom.Node, ix *SiblingIndex) bool {
	for _, a := range args {
		switch a.combinator {
		case '>':
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == dom.NodeElement && a.sel.MatchesIndexed(c, ix) {
					return true
				}
			}
		case '+':
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement {
					if a.sel.MatchesIndexed(sib, ix) {
						return true
					}
					break
				}
			}
		case '~':
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement && a.sel.MatchesIndexed(sib, ix) {
					return true
				}
			}
		default:
			if hasDescendantMatching(n, a.sel, ix) {
				return true
			}
		}
	}
	return false
}

func hasDescendantMatching(n *dom.Node, sel Selector, ix *SiblingIndex) bool {
	if ix != nil {
		selKey := formatSel(sel)
		key := descCacheKey{parent: n, sel: selKey}
		if cached := ix.hasDesc[key]; cached {
			return true
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == dom.NodeElement {
			if sel.MatchesIndexed(c, ix) {
				if ix != nil {
					ix.hasDesc[descCacheKey{parent: n, sel: formatSel(sel)}] = true
				}
				return true
			}
			if hasDescendantMatching(c, sel, ix) {
				if ix != nil {
					ix.hasDesc[descCacheKey{parent: n, sel: formatSel(sel)}] = true
				}
				return true
			}
		}
	}

	if ix != nil {
		ix.hasDesc[descCacheKey{parent: n, sel: formatSel(sel)}] = false
	}
	return false
}

func matchNth(arg string, n *dom.Node, ofType, fromEnd bool, ix *SiblingIndex) bool {
	expr, ofStrs := splitNthOf(arg)
	var of []Selector
	for _, selStr := range ofStrs {
		of = append(of, ParseSelector(selStr))
	}
	return matchNthParsed(expr, of, n, ofType, fromEnd, ix, nil)
}

func matchNthC(c Condition, n *dom.Node, ofType, fromEnd bool, ix *SiblingIndex) bool {
	if c.compiled != nil && c.compiled.hasNth {
		return matchNthParsed(c.compiled.nthExpr, c.compiled.of, n, ofType, fromEnd, ix, any(c.compiled))
	}
	return matchNth(c.Pseudo, n, ofType, fromEnd, ix)
}

func matchNthParsed(expr string, of []Selector, n *dom.Node, ofType, fromEnd bool, ix *SiblingIndex, fkey any) bool {
	a, b, ok := parseNth(expr)
	if !ok {
		return false
	}
	if len(of) > 0 {
		matched := false
		for _, sel := range of {
			if sel.MatchesIndexed(n, ix) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	i, total := nthIndexFilteredSel(n, ofType, of, ix, fkey)
	if i == 0 {
		return false
	}
	if fromEnd {
		i = total - i + 1
	}
	if a == 0 {
		return i == b
	}
	d := i - b
	return d%a == 0 && d/a >= 0
}

func matchLang(arg string, n *dom.Node) bool {
	matched := false
	for _, part := range splitSelectorList(arg) {
		lang := strings.ToLower(strings.TrimSpace(part))
		if lang == "" {
			continue
		}
		if lang == "*" {
			return true
		}
		if matchOneLang(lang, n) {
			matched = true
			break
		}
	}
	return matched
}

func matchOneLang(lang string, n *dom.Node) bool {
	for el := n; el != nil; el = el.Parent {
		if !el.Element() {
			continue
		}
		if v, present := xmlLangOf(el); present {
			return v != "" && langMatches(v, lang)
		}
		if el.HasAttribute("lang") {
			v := el.GetAttribute("lang")
			return v != "" && langMatches(v, lang)
		}
	}
	if n == nil || n.Doc == nil {
		return false
	}
	if value, present := documentContentLanguage(n.Doc); present {
		return value != "" && langMatches(value, lang)
	}
	return n.Doc.ContentLanguage != "" && langMatches(firstLanguageTag(n.Doc.ContentLanguage), lang)
}

func xmlLangOf(el *dom.Node) (string, bool) {
	for _, a := range el.Attr {
		if a.Namespace == dom.NSXML && strings.EqualFold(a.Name, "xml:lang") {
			return a.Value, true
		}
	}
	return "", false
}

func documentContentLanguage(doc *dom.Document) (string, bool) {
	if doc == nil || doc.Head == nil {
		return "", false
	}
	var language string
	doc.Head.Walk(func(n *dom.Node) bool {
		if !n.Element() || n.Data != "meta" || !strings.EqualFold(strings.TrimSpace(n.GetAttribute("http-equiv")), "content-language") {
			return true
		}
		language = firstLanguageTag(n.GetAttribute("content"))
		return false
	})
	return language, language != ""
}

func firstLanguageTag(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func langMatches(have, want string) bool {
	have = strings.ToLower(strings.TrimSpace(have))
	return have == want || strings.HasPrefix(have, want+"-")
}

func splitNthOf(arg string) (expr string, of []string) {
	lower := strings.ToLower(arg)
	for i := 0; i+4 <= len(arg); i++ {
		if lower[i] != ' ' && lower[i] != '\t' && lower[i] != '\n' {
			continue
		}
		j := i + 1
		for j < len(arg) && (arg[j] == ' ' || arg[j] == '\t' || arg[j] == '\n') {
			j++
		}
		if j+2 < len(arg) && lower[j] == 'o' && lower[j+1] == 'f' {
			k := j + 2
			if k < len(arg) && arg[k] != ' ' && arg[k] != '\t' && arg[k] != '\n' {
				continue
			}
			return strings.TrimSpace(arg[:i]), splitSelectorList(strings.TrimSpace(arg[k:]))
		}
	}
	return arg, nil
}

type SiblingIndex struct {
	lists   map[*dom.Node]*siblingList
	filt    map[filterKey]*filteredSibs
	hasDesc map[descCacheKey]bool
}

type siblingList struct {
	elems    []*dom.Node
	pos      map[*dom.Node]int
	tagRank  map[*dom.Node]int
	tagTotal map[string]int
}

type descCacheKey struct {
	parent *dom.Node
	sel    string
}

type filterKey struct {
	parent *dom.Node
	cond   any
}

type filteredSibs struct {
	rank  []int
	total int
}

func NewSiblingIndex() *SiblingIndex {
	return &SiblingIndex{
		lists:   make(map[*dom.Node]*siblingList),
		filt:    make(map[filterKey]*filteredSibs),
		hasDesc: make(map[descCacheKey]bool),
	}
}

func (ix *SiblingIndex) listOf(parent *dom.Node) *siblingList {
	if l, ok := ix.lists[parent]; ok {
		return l
	}
	l := &siblingList{}
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if !c.Element() {
			continue
		}
		if l.pos == nil {
			l.pos = make(map[*dom.Node]int)
			l.tagRank = make(map[*dom.Node]int)
			l.tagTotal = make(map[string]int)
		}
		l.elems = append(l.elems, c)
		l.pos[c] = len(l.elems)
		l.tagTotal[c.Data]++
		l.tagRank[c] = l.tagTotal[c.Data]
	}
	ix.lists[parent] = l
	return l
}

func (ix *SiblingIndex) filteredOf(parent *dom.Node, key any, filters []Selector) *filteredSibs {
	if key != nil {
		k := filterKey{parent: parent, cond: key}
		if f, ok := ix.filt[k]; ok {
			return f
		}
		f := buildFiltered(ix.listOf(parent), filters, ix)
		ix.filt[k] = f
		return f
	}
	return buildFiltered(ix.listOf(parent), filters, ix)
}

func buildFiltered(l *siblingList, filters []Selector, ix *SiblingIndex) *filteredSibs {
	f := &filteredSibs{rank: make([]int, len(l.elems))}
	for i, c := range l.elems {
		for _, s := range filters {
			if s.MatchesIndexed(c, ix) {
				f.total++
				f.rank[i] = f.total
				break
			}
		}
	}
	return f
}

func nthIndex(n *dom.Node, ofType bool) (index, total int) {
	return nthIndexFiltered(n, ofType, nil)
}

func nthIndexFiltered(n *dom.Node, ofType bool, of []string) (index, total int) {
	var filters []Selector
	for _, selStr := range of {
		filters = append(filters, ParseSelector(selStr))
	}
	return nthIndexFilteredSel(n, ofType, filters, nil, nil)
}

func nthIndexFilteredSel(n *dom.Node, ofType bool, filters []Selector, ix *SiblingIndex, fkey any) (index, total int) {
	if n.Parent == nil {
		return 0, 0
	}
	if ix == nil {
		return nthIndexLive(n, ofType, filters)
	}
	l := ix.listOf(n.Parent)
	if len(filters) == 0 {
		if ofType {
			return l.tagRank[n], l.tagTotal[n.Data]
		}
		return l.pos[n], len(l.elems)
	}
	f := ix.filteredOf(n.Parent, fkey, filters)
	p := l.pos[n]
	if p < 1 || p > len(f.rank) {
		return 0, f.total
	}
	return f.rank[p-1], f.total
}

func nthIndexLive(n *dom.Node, ofType bool, filters []Selector) (index, total int) {
	counts := func(c *dom.Node) bool {
		if !c.Element() || (ofType && c.Data != n.Data) {
			return false
		}
		if len(filters) == 0 {
			return true
		}
		for _, f := range filters {
			if f.Matches(c) {
				return true
			}
		}
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if !counts(c) {
			continue
		}
		total++
		if c == n {
			index = total
		}
	}
	return index, total
}

func parseNth(arg string) (a, b int, ok bool) {
	s := strings.ToLower(strings.Join(strings.Fields(arg), ""))
	switch s {
	case "":
		return 0, 0, false
	case "odd":
		return 2, 1, true
	case "even":
		return 2, 0, true
	}
	i := strings.IndexByte(s, 'n')
	if i < 0 {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0, 0, false
		}
		return 0, v, true
	}
	switch coef := s[:i]; coef {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		v, err := strconv.Atoi(coef)
		if err != nil {
			return 0, 0, false
		}
		a = v
	}
	rest := s[i+1:]
	if rest == "" {
		return a, 0, true
	}
	sign := 1
	switch rest[0] {
	case '+':
		rest = rest[1:]
	case '-':
		sign = -1
		rest = rest[1:]
	default:
		return 0, 0, false
	}
	v, err := strconv.Atoi(rest)
	if err != nil {
		return 0, 0, false
	}
	return a, sign * v, true
}

func isFirstElementChild(n *dom.Node) bool {
	if n.Parent == nil {
		return false
	}
	for c := n.Parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() {
			return c == n
		}
	}
	return false
}

func isLastElementChild(n *dom.Node) bool {
	if n.Parent == nil {
		return false
	}
	for c := n.Parent.LastChild; c != nil; c = c.PrevSibling {
		if c.Element() {
			return c == n
		}
	}
	return false
}

func splitSelectorList(s string) []string {
	all, _ := splitSelectorListExact(s)
	result := make([]string, 0, len(all))
	for _, sel := range all {
		if sel != "" {
			result = append(result, sel)
		}
	}
	return result
}

func splitSelectorListExact(s string) ([]string, bool) {
	var result []string
	var current strings.Builder
	depth := 0
	ok := true

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) {
			current.WriteByte(ch)
			i++
			current.WriteByte(s[i])
			continue
		}
		switch ch {
		case '"', '\'':
			end := quotedRunEnd(s, i)
			current.WriteString(s[i:end])
			i = end - 1
		case '(':
			depth++
			current.WriteByte(ch)
		case ')':
			depth--
			if depth < 0 {
				ok = false
			}
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	result = append(result, strings.TrimSpace(current.String()))
	return result, ok
}

func SplitSelectorList(s string) []string {
	return splitSelectorList(s)
}

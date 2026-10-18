package css

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// formatSel returns a compact string representation of sel for caching purposes.
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

// Selector is a compiled CSS selector.
type Selector struct {
	Parts []SelectorPart
}

// SelectorPart is one segment of a compound selector, connected by a combinator.
type SelectorPart struct {
	Combinator byte
	Conditions []Condition
}

// Condition is one test on a node.
type Condition struct {
	Type            ConditionType
	Value           string
	Attr            string
	Op              string
	Pseudo          string
	CaseInsensitive bool
	// compiled holds the Pseudo selector arguments parsed once at stylesheet
	// parse time. Functional pseudo-classes otherwise re-parse their argument
	// on every element they test (:not, :is, :has, nth-of), which is O(argument
	// length) of tokenizer work per element per candidate. The pointer is
	// shared by every copy of the Condition; nil means precompilation did not
	// apply (a hand-built Condition, or a pseudo without selector args), and
	// match falls back to parsing live.
	compiled *pseudoCompiled
}

// pseudoCompiled holds selector arguments parsed once.
type pseudoCompiled struct {
	// sels serves :not (exactly one) and :is/:where (the comma-separated list).
	sels []Selector
	// has serves :has alternatives with their leading combinators preserved.
	has []hasArg
	// of serves :nth-child(An+B of S) filters.
	of []Selector
	// nthExpr is the An+B microsyntax before `of`, so match skips the split.
	nthExpr string
	hasNth  bool
}

// hasArg is one :has() alternative: the relationship plus the selector it applies to.
type hasArg struct {
	combinator byte
	sel        Selector
}

// precompile parses the selector arguments of functional pseudo-classes once.
// It runs at stylesheet parse time; match code uses the result when present
// and parses live otherwise, so behavior is identical either way.
func (c *Condition) precompile() {
	if c.Type != CondPseudoClass || c.Pseudo == "" {
		return
	}
	switch c.Value {
	case "not", "is", "where":
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

// compileHasArgs splits a :has() argument the way matchHas evaluates it: comma
// alternatives, each with an optional leading combinator.
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

// ConditionType identifies the kind of selector condition.
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

// Specificity returns the (a, b, c) specificity of the selector.
func (s Selector) Specificity() (a, b, c int) {
	for _, part := range s.Parts {
		for _, cond := range part.Conditions {
			switch cond.Type {
			case CondID:
				a++
			case CondClass, CondAttr:
				b++
			case CondPseudoClass:
				// :where() contributes zero specificity.
				if cond.Value == "where" {
					continue
				}
				// :is() and :not() take the highest specificity of their arguments.
				if cond.Value == "is" || cond.Value == "not" {
					var sels []Selector
					if cond.compiled != nil && len(cond.compiled.sels) > 0 {
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
				// :has() takes the highest specificity of its arguments.
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
				// Other pseudo-classes contribute (0, 1, 0), and an `of`
				// selector list in a functional one adds its most specific
				// argument on top, the way `:nth-child(2 of #id)` outranks a
				// lone class.
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

// CompareSpecificity returns -1, 0, or 1 comparing two specificity tuples.
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

// ParseSelector parses a CSS selector string.
func ParseSelector(s string) Selector {
	s = strings.TrimSpace(s)
	if s == "" {
		return Selector{}
	}

	var parts []SelectorPart
	var current SelectorPart
	i := 0

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
			case '>':
				combinator = '>'
				i++
				for i < len(s) && s[i] == ' ' {
					i++
				}
			case '+':
				combinator = '+'
				i++
				for i < len(s) && s[i] == ' ' {
					i++
				}
			case '~':
				combinator = '~'
				i++
				for i < len(s) && s[i] == ' ' {
					i++
				}
			}
		}

		if combinator != 0 && len(current.Conditions) > 0 {
			// A combinator relates its left-hand part to the part that follows it,
			// and matchParts reads it off the right-hand side, so the flushed part
			// keeps the combinator that introduced it and the new one takes this.
			parts = append(parts, current)
			current = SelectorPart{Combinator: combinator}
		}

		cond, newI := parseCondition(s, i)
		if newI == i {
			break
		}
		i = newI
		current.Conditions = append(current.Conditions, cond)
	}

	if len(current.Conditions) > 0 {
		parts = append(parts, current)
	}

	return Selector{Parts: parts}
}

func parseCondition(s string, i int) (Condition, int) {
	if i >= len(s) {
		return Condition{}, i
	}

	switch s[i] {
	case '#':
		i++
		start := i
		for i < len(s) && isIdentChar(s[i]) {
			i++
		}
		return Condition{Type: CondID, Value: s[start:i]}, i

	case '.':
		i++
		start := i
		for i < len(s) && isIdentChar(s[i]) {
			i++
		}
		return Condition{Type: CondClass, Value: s[start:i]}, i

	case '[':
		i++
		start := i
		for i < len(s) && s[i] != ']' {
			i++
		}
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
			valStr = strings.Trim(valStr, "\"' ")
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
			return Condition{Type: CondPseudoElement, Value: s[start:i]}, i
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
			arg := s[argStart:i]
			if i < len(s) {
				i++
			}
			cond := Condition{Type: CondPseudoClass, Value: name, Pseudo: arg}
			cond.precompile()
			return cond, i
		}
		// The four original pseudo-elements keep their single-colon spellings
		// for compatibility (CSS 2.1 §5.12.3): :before means ::before, and
		// matching them as pseudo-classes would never find an element. Every
		// float clearfix in the wild (.row:after) depends on this.
		switch strings.ToLower(name) {
		case "before", "after", "first-line", "first-letter":
			return Condition{Type: CondPseudoElement, Value: strings.ToLower(name)}, i
		}
		return Condition{Type: CondPseudoClass, Value: name}, i

	case '*':
		i++
		return Condition{Type: CondUniversal}, i

	default:
		if isIdentStart(s[i]) || s[i] == '-' {
			start := i
			for i < len(s) && isIdentChar(s[i]) {
				i++
			}
			return Condition{Type: CondType, Value: strings.ToLower(s[start:i])}, i
		}
	}

	return Condition{}, i
}

// Matches reports whether the selector matches the given node.
func (s Selector) Matches(n *dom.Node) bool {
	return s.MatchesIndexed(n, nil)
}

// MatchesIndexed is Matches with a per-pass sibling cache. A style
// resolution pass tests thousands of elements against selectors containing
// :nth-child/:nth-of-type/:has, and the naive evaluation counts element
// siblings per tested node: O(siblings) each, O(siblings²) per parent. A
// 4000-row list with `li:nth-child(2n)` spends ~50ms in counting alone.
// The index builds each touched parent's element order once per pass, so
// counting drops to O(1) amortized. DOM is frozen during a pass, hence no
// invalidation. A nil index keeps the old live-walk behavior for callers
// outside a resolution pass.
func (s Selector) MatchesIndexed(n *dom.Node, ix *SiblingIndex) bool {
	if len(s.Parts) == 0 {
		return false
	}
	return matchParts(s.Parts, len(s.Parts)-1, n, ix)
}

// MatchesPseudoElement reports whether the selector matches the given
// pseudo-element (::before, ::after, etc.) of the given node. The key compound
// must have a CondPseudoElement condition for the given name; the rest of the
// selector is matched against the node normally.
func (s Selector) MatchesPseudoElement(n *dom.Node, pseudo string) bool {
	return s.MatchesPseudoElementIndexed(n, pseudo, nil)
}

// MatchesPseudoElementIndexed is MatchesPseudoElement with a sibling cache.
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

// matchPartsPseudo is like matchParts but skips CondPseudoElement conditions
// for the given pseudo name in the key compound.
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

// matchConditionsPseudo is like matchConditions but skips CondPseudoElement
// conditions for the given pseudo name when isKey is true.
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
		// :is() and :where() take a comma-separated selector list and match if
		// any selector in the list matches. The difference is specificity:
		// :is() takes the highest specificity of its arguments, :where() is zero.
		// Specificity is handled in the Specificity() method; here we just match.
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
		// Match any hyperlink element (<a href>, <area href>, <link href>).
		// We have no navigation history, so all links are treated as unvisited.
		tag := strings.ToLower(n.Data)
		return (tag == "a" || tag == "area" || tag == "link") && n.HasAttribute("href")
	case "lang":
		return matchLang(c.Pseudo, n)
	case "visited", "active", "focus-within", "focus-visible":
		// These interaction pseudo-classes require runtime state that the engine
		// does not track yet. Returning false matches what a browser shows on
		// a first load, which is the state every reference render is captured
		// in - treating `a:visited` as `a:link` painted Wikipedia's links in
		// MediaWiki's visited purple because that rule comes after the link one.
		return false
	case "hover":
		return IsHovered(n)
	case "focus":
		return IsFocused(n)
	case "checked", "disabled", "enabled", "placeholder-shown":
		return false
	case "has":
		// :has() is a relational pseudo-class: it matches if the selector
		// argument matches at least one element relative to this element.
		// The argument may start with a combinator:
		//   :has(.child)     - descendant
		//   :has(> .child)   - direct child
		//   :has(+ .sibling) - adjacent next sibling
		//   :has(~ .sibling) - general next sibling
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

// matchHas implements the :has() relational pseudo-class. The argument is a
// selector list that may start with a combinator to specify the relationship:
//
//	:has(.child)     - any descendant
//	:has(> .child)   - direct child
//	:has(+ .sibling) - adjacent next sibling
//	:has(~ .sibling) - general next sibling
//
// Returns true if at least one relative element matches any selector in the list.
func matchHas(arg string, n *dom.Node, ix *SiblingIndex) bool {
	selectors := splitSelectorList(arg)
	for _, selStr := range selectors {
		selStr = strings.TrimSpace(selStr)
		if selStr == "" {
			continue
		}
		// Check for leading combinator
		combinator := byte(' ') // default: descendant
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
			// Direct child
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == dom.NodeElement && sel.MatchesIndexed(c, ix) {
					return true
				}
			}
		case '+':
			// Adjacent next sibling
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement {
					if sel.MatchesIndexed(sib, ix) {
						return true
					}
					break // only check the first element sibling
				}
			}
		case '~':
			// General next siblings
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement && sel.MatchesIndexed(sib, ix) {
					return true
				}
			}
		default:
			// Descendant (any depth)
			if hasDescendantMatching(n, sel, ix) {
				return true
			}
		}
	}
	return false
}

// matchHasCompiled evaluates :has() against pre-parsed alternatives: same
// relationships as matchHas, without re-splitting or re-parsing per element.
func matchHasCompiled(args []hasArg, n *dom.Node, ix *SiblingIndex) bool {
	for _, a := range args {
		switch a.combinator {
		case '>':
			// Direct child
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == dom.NodeElement && a.sel.MatchesIndexed(c, ix) {
					return true
				}
			}
		case '+':
			// Adjacent next sibling
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement {
					if a.sel.MatchesIndexed(sib, ix) {
						return true
					}
					break // only check the first element sibling
				}
			}
		case '~':
			// General next siblings
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement && a.sel.MatchesIndexed(sib, ix) {
					return true
				}
			}
		default:
			// Descendant (any depth)
			if hasDescendantMatching(n, a.sel, ix) {
				return true
			}
		}
	}
	return false
}

// hasDescendantMatching checks if any descendant of n matches the selector.
func hasDescendantMatching(n *dom.Node, sel Selector, ix *SiblingIndex) bool {
	// Use cached result if available (from :has() matching on same parent+sel)
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
				// Cache intermediate results when found in subtree
				if ix != nil {
					ix.hasDesc[descCacheKey{parent: n, sel: formatSel(sel)}] = true
				}
				return true
			}
		}
	}

	// Cache negative result too to avoid re-computation
	if ix != nil {
		ix.hasDesc[descCacheKey{parent: n, sel: formatSel(sel)}] = false
	}
	return false
}

// matchNth resolves an An+B expression against the node's position among its
// element siblings.
func matchNth(arg string, n *dom.Node, ofType, fromEnd bool, ix *SiblingIndex) bool {
	expr, ofStrs := splitNthOf(arg)
	var of []Selector
	for _, selStr := range ofStrs {
		of = append(of, ParseSelector(selStr))
	}
	return matchNthParsed(expr, of, n, ofType, fromEnd, ix, nil)
}

// matchNthC is matchNth for a parsed Condition: precompiled `of` filters and
// the An+B microsyntax come from the sidecar, so matching parses nothing.
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
	// `:nth-child(An+B of S)` counts only siblings matching S, and the
	// element itself must match one of them. Without the filter every `of`
	// form silently matched nothing.
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

// matchLang implements :lang(): any comma-separated range matching wins. A
// range matches when the nearest ancestor-or-self language (lang, else
// xml:lang) equals it or starts with it plus a hyphen (BCP47 prefix rule).
// An empty range list never matches; `*` matches every element.
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
	// Each level decides in order: an XML-namespaced xml:lang wins ties on
	// its own level, plain lang answers otherwise, and an empty level defers
	// upward. Only setAttributeNS creates namespaced attributes - the HTML
	// parser never does, so static xml:lang stays invisible here exactly
	// like the reference engine.
	for el := n; el != nil; el = el.Parent {
		if !el.Element() {
			continue
		}
		if v := xmlLangOf(el); v != "" {
			return langMatches(v, lang)
		}
		if v := el.GetAttribute("lang"); v != "" {
			return langMatches(v, lang)
		}
	}
	return false
}

// xmlLangOf reads the XML-namespaced xml:lang only; a parser-produced
// literal of the same spelling does not count.
func xmlLangOf(el *dom.Node) string {
	for _, a := range el.Attr {
		if a.Namespace == dom.NSXML && strings.EqualFold(a.Name, "xml:lang") {
			return a.Value
		}
	}
	return ""
}

// langMatches reports BCP47 prefix equality: en covers en-US but not
// english or fr.
func langMatches(have, want string) bool {
	have = strings.ToLower(strings.TrimSpace(have))
	return have == want || strings.HasPrefix(have, want+"-")
}

// splitNthOf splits "An+B of S1, S2" into the microsyntax and the selector
// list. `of` needs whitespace around it; without it there is no filter.
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

// SiblingIndex caches per-parent element-child order for one style resolution
// pass, turning :nth-child/:nth-of-type counting from O(siblings) per tested
// node into O(1) amortized. The DOM is frozen while a pass runs, so entries
// need no invalidation; the whole index is dropped with the pass.
type SiblingIndex struct {
	lists   map[*dom.Node]*siblingList
	filt    map[filterKey]*filteredSibs
	hasDesc map[descCacheKey]bool // caches has() descendant matches by (parent, selString)
}

// siblingList is one parent's element children in document order plus the
// ranks derived from it in the same single walk.
type siblingList struct {
	elems    []*dom.Node
	pos      map[*dom.Node]int
	tagRank  map[*dom.Node]int
	tagTotal map[string]int
}

// descCacheKey identifies one :has() descendant check on a parent.
// Reserved for future caching; currently unused but kept to document the
// intended key shape.
type descCacheKey struct {
	parent *dom.Node
	sel    string
}

// filterKey identifies one `:nth-child(An+B of S)` filter set on one parent.
// cond is the Condition's *pseudoCompiled: shared by every copy of the
// Condition, distinct per parsed rule, hence a correct cache identity. A nil
// cond (hand-built Condition) disables caching for that call.
type filterKey struct {
	parent *dom.Node
	cond   any
}

// filteredSibs is the per-child rank among filter-matching siblings.
type filteredSibs struct {
	rank  []int
	total int
}

// NewSiblingIndex returns an empty per-pass cache.
func NewSiblingIndex() *SiblingIndex {
	return &SiblingIndex{
		lists:   make(map[*dom.Node]*siblingList),
		filt:    make(map[filterKey]*filteredSibs),
		hasDesc: make(map[descCacheKey]bool),
	}
}

// listOf returns parent's element children, building the entry once.
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

// filteredOf returns the matching-rank table for one filter set on a parent,
// building it once per (parent, cond). A nil cond still uses the cached child
// list but rebuilds ranks every call, exactly like the old live walk.
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

// nthIndex returns the node's 1-based position among its element siblings and
// how many such siblings exist. ofType restricts both counts to the same tag
// name, which is what separates nth-child from nth-of-type.
func nthIndex(n *dom.Node, ofType bool) (index, total int) {
	return nthIndexFiltered(n, ofType, nil)
}

// nthIndexFiltered is nthIndex with an additional selector filter for the
// `:nth-child(An+B of S)` form: only siblings matching one of the selectors
// count. Filters arrive pre-parsed: parsing them here would redo the work for
// every sibling of every tested element.
func nthIndexFiltered(n *dom.Node, ofType bool, of []string) (index, total int) {
	var filters []Selector
	for _, selStr := range of {
		filters = append(filters, ParseSelector(selStr))
	}
	return nthIndexFilteredSel(n, ofType, filters, nil, nil)
}

// nthIndexFilteredSel is nthIndexFiltered over pre-parsed filters with an
// optional per-pass sibling cache. With a cache, unfiltered counting is two
// map lookups and a filtered one is a table read; a nil cache walks siblings
// exactly like the old code. fkey identifies the filter set for caching and
// must be the Condition's *pseudoCompiled (or nil to disable filter caching).
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

// nthIndexLive is the original sibling walk, kept for callers outside a
// cached resolution pass.
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

// parseNth reads the An+B microsyntax along with the odd and even keywords. The
// argument arrives with its whitespace intact, so the whole expression is
// squeezed first and only then split around the `n`.
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

// splitSelectorList splits a comma-separated selector list into individual selectors.
// Handles nested parentheses (e.g., :is(.a, .b:not(.c))).
func splitSelectorList(s string) []string {
	var result []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '(':
			depth++
			current.WriteByte(ch)
		case ')':
			depth--
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				// End of one selector.
				sel := strings.TrimSpace(current.String())
				if sel != "" {
					result = append(result, sel)
				}
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	// Last selector.
	sel := strings.TrimSpace(current.String())
	if sel != "" {
		result = append(result, sel)
	}

	return result
}

// SplitSelectorList is the exported version of splitSelectorList for testing.
func SplitSelectorList(s string) []string {
	return splitSelectorList(s)
}

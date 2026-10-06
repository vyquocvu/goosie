package css

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

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
	Type   ConditionType
	Value  string
	Attr   string
	Op     string
	Pseudo string
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
				// :is() and :where() have special specificity rules.
				if cond.Value == "where" {
					// :where() contributes zero specificity.
					continue
				}
				if cond.Value == "is" {
					// :is() takes the highest specificity of its arguments.
					if cond.Pseudo != "" {
						selectors := splitSelectorList(cond.Pseudo)
						maxA, maxB, maxC := 0, 0, 0
						for _, selStr := range selectors {
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
			val = strings.Trim(val, "\"' ")
			return Condition{Type: CondAttr, Attr: name, Op: op, Value: val}, i
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
			return Condition{Type: CondPseudoClass, Value: name, Pseudo: arg}, i
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
	if len(s.Parts) == 0 {
		return false
	}
	return matchParts(s.Parts, len(s.Parts)-1, n)
}

// MatchesPseudoElement reports whether the selector matches the given
// pseudo-element (::before, ::after, etc.) of the given node. The key compound
// must have a CondPseudoElement condition for the given name; the rest of the
// selector is matched against the node normally.
func (s Selector) MatchesPseudoElement(n *dom.Node, pseudo string) bool {
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
	return matchPartsPseudo(s.Parts, len(s.Parts)-1, n, pseudo)
}

// matchPartsPseudo is like matchParts but skips CondPseudoElement conditions
// for the given pseudo name in the key compound.
func matchPartsPseudo(parts []SelectorPart, idx int, n *dom.Node, pseudo string) bool {
	if idx < 0 || n == nil || !n.Element() {
		return false
	}

	part := parts[idx]
	if !matchConditionsPseudo(part.Conditions, n, pseudo, idx == len(parts)-1) {
		return false
	}

	if idx == 0 {
		return true
	}

	switch part.Combinator {
	case ' ':
		for p := n.Parent; p != nil; p = p.Parent {
			if matchPartsPseudo(parts, idx-1, p, pseudo) {
				return true
			}
		}
		return false
	case '>':
		return matchPartsPseudo(parts, idx-1, n.Parent, pseudo)
	case '+':
		sib := prevElement(n)
		return sib != nil && matchPartsPseudo(parts, idx-1, sib, pseudo)
	case '~':
		for sib := prevElement(n); sib != nil; sib = prevElement(sib) {
			if matchPartsPseudo(parts, idx-1, sib, pseudo) {
				return true
			}
		}
		return false
	}

	return false
}

// matchConditionsPseudo is like matchConditions but skips CondPseudoElement
// conditions for the given pseudo name when isKey is true.
func matchConditionsPseudo(conds []Condition, n *dom.Node, pseudo string, isKey bool) bool {
	for _, c := range conds {
		if isKey && c.Type == CondPseudoElement && c.Value == pseudo {
			continue
		}
		if !matchCondition(c, n) {
			return false
		}
	}
	return true
}

func matchParts(parts []SelectorPart, idx int, n *dom.Node) bool {
	if idx < 0 || n == nil || !n.Element() {
		return false
	}

	part := parts[idx]
	if !matchConditions(part.Conditions, n) {
		return false
	}

	if idx == 0 {
		return true
	}

	switch part.Combinator {
	case ' ':
		for p := n.Parent; p != nil; p = p.Parent {
			if matchParts(parts, idx-1, p) {
				return true
			}
		}
		return false
	case '>':
		return matchParts(parts, idx-1, n.Parent)
	case '+':
		sib := prevElement(n)
		return sib != nil && matchParts(parts, idx-1, sib)
	case '~':
		for sib := prevElement(n); sib != nil; sib = prevElement(sib) {
			if matchParts(parts, idx-1, sib) {
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

func matchConditions(conds []Condition, n *dom.Node) bool {
	for _, c := range conds {
		if !matchCondition(c, n) {
			return false
		}
	}
	return true
}

func matchCondition(c Condition, n *dom.Node) bool {
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
		return matchPseudoClass(c, n)
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
	switch c.Op {
	case "=":
		return val == c.Value
	case "~=":
		for _, v := range strings.Fields(val) {
			if v == c.Value {
				return true
			}
		}
		return false
	case "|=":
		return val == c.Value || strings.HasPrefix(val, c.Value+"-")
	case "^=":
		return strings.HasPrefix(val, c.Value)
	case "$=":
		return strings.HasSuffix(val, c.Value)
	case "*=":
		return strings.Contains(val, c.Value)
	}
	return false
}

func matchPseudoClass(c Condition, n *dom.Node) bool {
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
		sel := ParseSelector(c.Pseudo)
		return !sel.Matches(n)
	case "is", "where":
		// :is() and :where() take a comma-separated selector list and match if
		// any selector in the list matches. The difference is specificity:
		// :is() takes the highest specificity of its arguments, :where() is zero.
		// Specificity is handled in the Specificity() method; here we just match.
		if c.Pseudo == "" {
			return false
		}
		selectors := splitSelectorList(c.Pseudo)
		for _, selStr := range selectors {
			sel := ParseSelector(selStr)
			if sel.Matches(n) {
				return true
			}
		}
		return false
	case "link", "any-link":
		// Match any hyperlink element (<a href>, <area href>, <link href>).
		// We have no navigation history, so all links are treated as unvisited.
		tag := strings.ToLower(n.Data)
		return (tag == "a" || tag == "area" || tag == "link") && n.HasAttribute("href")
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
		return matchHas(c.Pseudo, n)
	case "nth-child":
		return matchNth(c.Pseudo, n, false, false)
	case "nth-last-child":
		return matchNth(c.Pseudo, n, false, true)
	case "nth-of-type":
		return matchNth(c.Pseudo, n, true, false)
	case "nth-last-of-type":
		return matchNth(c.Pseudo, n, true, true)
	case "first-of-type":
		return matchNth("1", n, true, false)
	case "last-of-type":
		return matchNth("1", n, true, true)
	case "only-of-type":
		return matchNth("1", n, true, false) && matchNth("1", n, true, true)
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
func matchHas(arg string, n *dom.Node) bool {
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
				if c.Type == dom.NodeElement && sel.Matches(c) {
					return true
				}
			}
		case '+':
			// Adjacent next sibling
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement {
					if sel.Matches(sib) {
						return true
					}
					break // only check the first element sibling
				}
			}
		case '~':
			// General next siblings
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == dom.NodeElement && sel.Matches(sib) {
					return true
				}
			}
		default:
			// Descendant (any depth)
			if hasDescendantMatching(n, sel) {
				return true
			}
		}
	}
	return false
}

// hasDescendantMatching checks if any descendant of n matches the selector.
func hasDescendantMatching(n *dom.Node, sel Selector) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == dom.NodeElement {
			if sel.Matches(c) {
				return true
			}
			if hasDescendantMatching(c, sel) {
				return true
			}
		}
	}
	return false
}

// matchNth resolves an An+B expression against the node's position among its
// element siblings.
func matchNth(arg string, n *dom.Node, ofType, fromEnd bool) bool {
	expr, ofSels := splitNthOf(arg)
	a, b, ok := parseNth(expr)
	if !ok {
		return false
	}
	// `:nth-child(An+B of S)` counts only siblings matching S, and the
	// element itself must match one of them. Without the filter every `of`
	// form silently matched nothing.
	if len(ofSels) > 0 {
		matched := false
		for _, selStr := range ofSels {
			if ParseSelector(selStr).Matches(n) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	i, total := nthIndexFiltered(n, ofType, ofSels)
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

// nthIndex returns the node's 1-based position among its element siblings and
// how many such siblings exist. ofType restricts both counts to the same tag
// name, which is what separates nth-child from nth-of-type.
func nthIndex(n *dom.Node, ofType bool) (index, total int) {
	return nthIndexFiltered(n, ofType, nil)
}

// nthIndexFiltered is nthIndex with an additional selector filter for the
// `:nth-child(An+B of S)` form: only siblings matching one of the selectors
// count.
func nthIndexFiltered(n *dom.Node, ofType bool, of []string) (index, total int) {
	if n.Parent == nil {
		return 0, 0
	}
	var filters []Selector
	for _, selStr := range of {
		filters = append(filters, ParseSelector(selStr))
	}
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

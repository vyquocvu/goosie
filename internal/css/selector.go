package css

import (
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
	Type  ConditionType
	Value string
	Attr  string
	Op    string
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
			case CondClass, CondAttr, CondPseudoClass:
				b++
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
			current.Combinator = combinator
			parts = append(parts, current)
			current = SelectorPart{}
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
	return matchParts(s.Parts, len(s.Parts)- 1, n)
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
	}
	return false
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

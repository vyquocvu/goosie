package css

import (
	"strings"
)

// CompiledSelector is a flat, compact representation of a selector chain.
// It replaces the linked SelectorSequence with a slice for efficient matching.
type CompiledSelector struct {
	Parts       []selectorPart // Flattened chain, rightmost last
	Specificity [3]uint16      // Precomputed [id, class, tag] specificity
	Key         bucketKey      // Key for bucket lookup (from rightmost part)
}

// selectorPart is one step in a compiled selector chain.
type selectorPart struct {
	Selector   SimpleSelector
	Combinator byte // 0=none, ' '=descendant, '>'=child, '+'=adjacent, '~'=general sibling
}

// bucketKey identifies which bucket a selector belongs to based on its rightmost simple selector.
type bucketKey struct {
	kind bucketKind
	key  string // ID name, class name, tag name (lowercased), or attr name
}

// bucketKind identifies the type of bucket for rule organization.
type bucketKind uint8

const (
	bucketID        bucketKind = iota // #id
	bucketClass                       // .class
	bucketTag                         // element tag
	bucketAttr                        // [attr]
	bucketUniversal                   // * or empty
)

// CompiledRule is a rule with pre-compiled selectors and specificity.
type CompiledRule struct {
	Selectors    []CompiledSelector
	Declarations []Declaration
	SourceOrder  uint32
	Origin       Origin
	Specificity  [3]uint16 // Maximum specificity among selectors
}

// CompiledStyleSheet is a pre-processed stylesheet with bucketed rules.
// Rules are organized by their rightmost selector key for O(1) candidate lookup.
type CompiledStyleSheet struct {
	rules           []CompiledRule
	idBucket        map[string][]int // ID -> rule indices
	classBucket     map[string][]int // class -> rule indices
	tagBucket       map[string][]int // tag (lowercased) -> rule indices
	attrBucket      map[string][]int // attr name -> rule indices
	universalBucket []int            // rule indices for universal selectors
}

// Element is the interface for DOM elements during selector matching.
// This keeps the CSS package independent from the renderer.
type Element interface {
	// TagName returns the element's tag name (e.g., "div", "p")
	TagName() string
	// ID returns the element's ID attribute value
	ID() string
	// Classes returns the element's class list
	Classes() []string
	// GetAttribute returns the attribute value and whether it exists
	GetAttribute(name string) (string, bool)
	// ParentElement returns the parent element, or nil if root
	ParentElement() Element
	// PreviousSiblingElement returns the previous sibling element, or nil
	PreviousSiblingElement() Element
	// ForEachChild iterates children until fn returns false
	ForEachChild(fn func(Element) bool)
	// ForEachAncestor iterates ancestors until fn returns false
	ForEachAncestor(fn func(Element) bool)
	// ForEachPrecedingSibling iterates preceding siblings until fn returns false
	ForEachPrecedingSibling(fn func(Element) bool)
}

// ComputeSpecificity calculates the CSS specificity of a selector sequence.
// Returns [id, class, tag] counts per CSS spec.
func ComputeSpecificity(seq *SelectorSequence) [3]uint16 {
	var spec [3]uint16
	for s := seq; s != nil; s = s.Next {
		addSimpleSpecificity(&spec, &s.Simple)
	}
	return spec
}

// addSimpleSpecificity adds the specificity contribution of a simple selector.
func addSimpleSpecificity(spec *[3]uint16, sel *SimpleSelector) {
	// ID contributes to spec[0]
	if sel.ID != "" {
		spec[0]++
	}

	// Classes, attributes, and pseudo-classes contribute to spec[1]
	spec[1] += uint16(len(sel.Classes))
	spec[1] += uint16(len(sel.Attributes))
	spec[1] += uint16(len(sel.PseudoClasses))

	// Tag names and pseudo-elements contribute to spec[2]
	if sel.TagName != "" && !sel.Universal {
		spec[2]++
	}
	spec[2] += uint16(len(sel.PseudoElements))

	// Universal selector (*) contributes nothing
}

// CompareSpecificity compares two specificity values.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func CompareSpecificity(a, b [3]uint16) int {
	for i := 0; i < 3; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// CompileStyleSheet converts a StyleSheet into a CompiledStyleSheet with
// precomputed specificity and bucketed rules for efficient matching.
func CompileStyleSheet(sheet *StyleSheet) *CompiledStyleSheet {
	if sheet == nil {
		return &CompiledStyleSheet{
			idBucket:    make(map[string][]int),
			classBucket: make(map[string][]int),
			tagBucket:   make(map[string][]int),
			attrBucket:  make(map[string][]int),
		}
	}

	cs := &CompiledStyleSheet{
		rules:       make([]CompiledRule, 0, len(sheet.Rules)),
		idBucket:    make(map[string][]int),
		classBucket: make(map[string][]int),
		tagBucket:   make(map[string][]int),
		attrBucket:  make(map[string][]int),
	}

	for _, rule := range sheet.Rules {
		compiled := compileRule(rule)
		ruleIdx := len(cs.rules)
		cs.rules = append(cs.rules, compiled)

		// Bucket by each selector's key
		for _, sel := range compiled.Selectors {
			switch sel.Key.kind {
			case bucketID:
				cs.idBucket[sel.Key.key] = append(cs.idBucket[sel.Key.key], ruleIdx)
			case bucketClass:
				cs.classBucket[sel.Key.key] = append(cs.classBucket[sel.Key.key], ruleIdx)
			case bucketTag:
				cs.tagBucket[sel.Key.key] = append(cs.tagBucket[sel.Key.key], ruleIdx)
			case bucketAttr:
				cs.attrBucket[sel.Key.key] = append(cs.attrBucket[sel.Key.key], ruleIdx)
			case bucketUniversal:
				cs.universalBucket = append(cs.universalBucket, ruleIdx)
			}
		}
	}

	return cs
}

// compileRule compiles a single rule.
func compileRule(rule Rule) CompiledRule {
	cr := CompiledRule{
		Selectors:    make([]CompiledSelector, 0, len(rule.Selectors)),
		Declarations: rule.Declarations,
		SourceOrder:  rule.SourceOrder,
		Origin:       rule.Origin,
	}

	for _, seq := range rule.Selectors {
		compiled := compileSelectorSequence(&seq)
		cr.Specificity = maxSpecificity(cr.Specificity, compiled.Specificity)
		cr.Selectors = append(cr.Selectors, compiled)
	}

	return cr
}

// compileSelectorSequence compiles a SelectorSequence linked list into a flat slice.
func compileSelectorSequence(seq *SelectorSequence) CompiledSelector {
	cs := CompiledSelector{}

	// Walk the linked list and collect parts
	for s := seq; s != nil; s = s.Next {
		part := selectorPart{
			Selector: s.Simple,
		}
		if s.Combinator != "" {
			part.Combinator = combinatorByte(s.Combinator)
		}
		cs.Parts = append(cs.Parts, part)
	}

	// Compute specificity
	cs.Specificity = ComputeSpecificity(seq)

	// Determine bucket key from rightmost part
	if len(cs.Parts) > 0 {
		cs.Key = computeBucketKey(&cs.Parts[len(cs.Parts)-1].Selector)
	}

	return cs
}

// combinatorByte converts a combinator string to a byte.
func combinatorByte(s string) byte {
	switch s {
	case " ":
		return ' '
	case ">":
		return '>'
	case "+":
		return '+'
	case "~":
		return '~'
	default:
		return 0
	}
}

// computeBucketKey determines the bucket key for a simple selector.
// Priority: ID > class > tag > attr > universal
func computeBucketKey(sel *SimpleSelector) bucketKey {
	if sel.ID != "" {
		return bucketKey{kind: bucketID, key: sel.ID}
	}
	if len(sel.Classes) > 0 {
		return bucketKey{kind: bucketClass, key: sel.Classes[0]}
	}
	if sel.TagName != "" && !sel.Universal {
		return bucketKey{kind: bucketTag, key: strings.ToLower(sel.TagName)}
	}
	if len(sel.Attributes) > 0 {
		return bucketKey{kind: bucketAttr, key: sel.Attributes[0].Name}
	}
	return bucketKey{kind: bucketUniversal}
}

// maxSpecificity returns the higher of two specificity values.
func maxSpecificity(a, b [3]uint16) [3]uint16 {
	if CompareSpecificity(a, b) >= 0 {
		return a
	}
	return b
}

// MatchElement returns all compiled rules that match the given element.
// It uses bucket lookup to avoid scanning all rules.
func (cs *CompiledStyleSheet) MatchElement(elem Element) []CompiledRule {
	if elem == nil {
		return nil
	}

	// Collect candidate rule indices from buckets
	candidates := cs.collectCandidates(elem)
	if len(candidates) == 0 {
		return nil
	}

	var matched []CompiledRule
	if len(candidates) <= 32 {
		for i, idx := range candidates {
			duplicate := false
			for j := 0; j < i; j++ {
				if candidates[j] == idx {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}

			rule := &cs.rules[idx]
			for _, sel := range rule.Selectors {
				if matchCompiledSelector(&sel, elem) {
					matched = append(matched, *rule)
					break
				}
			}
		}
		return matched
	}

	// Deduplicate and match for large candidate lists
	seen := make(map[int]bool, len(candidates))
	for _, idx := range candidates {
		if seen[idx] {
			continue
		}
		seen[idx] = true

		rule := &cs.rules[idx]
		for _, sel := range rule.Selectors {
			if matchCompiledSelector(&sel, elem) {
				matched = append(matched, *rule)
				break // Only add rule once even if multiple selectors match
			}
		}
	}

	return matched
}

func toLowerFast(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			return strings.ToLower(s)
		}
	}
	return s
}

// collectCandidates gathers rule indices from relevant buckets.
func (cs *CompiledStyleSheet) collectCandidates(elem Element) []int {
	var candidates []int

	// ID bucket
	if id := elem.ID(); id != "" {
		candidates = append(candidates, cs.idBucket[id]...)
	}

	// Class buckets
	for _, class := range elem.Classes() {
		candidates = append(candidates, cs.classBucket[class]...)
	}

	// Tag bucket
	tag := toLowerFast(elem.TagName())
	candidates = append(candidates, cs.tagBucket[tag]...)

	// Attribute buckets - check all element attributes matching indexed attributes
	for attrName, ruleIndices := range cs.attrBucket {
		if _, ok := elem.GetAttribute(attrName); ok {
			candidates = append(candidates, ruleIndices...)
		}
	}

	// Universal bucket always applies
	candidates = append(candidates, cs.universalBucket...)

	return candidates
}

// matchCompiledSelector checks if a compiled selector matches an element.
func matchCompiledSelector(sel *CompiledSelector, elem Element) bool {
	if len(sel.Parts) == 0 {
		return false
	}

	// Match from right to left
	return matchParts(sel.Parts, len(sel.Parts)-1, elem)
}

// matchParts recursively matches selector parts from right to left.
func matchParts(parts []selectorPart, idx int, elem Element) bool {
	if idx < 0 || elem == nil {
		return false
	}

	part := &parts[idx]

	// Check if current element matches this part's simple selector
	if !matchSimpleSelector(&part.Selector, elem) {
		return false
	}

	// If this is the leftmost part, we're done
	if idx == 0 {
		return true
	}

	// Check combinator with the part to the left
	leftPart := &parts[idx-1]
	switch leftPart.Combinator {
	case ' ': // Descendant: leftpart elem
		// Check if any ancestor matches leftPart
		var found bool
		elem.ForEachAncestor(func(ancestor Element) bool {
			if matchParts(parts, idx-1, ancestor) {
				found = true
				return false // stop iteration
			}
			return true // continue
		})
		return found

	case '>': // Child: leftpart > elem
		parent := elem.ParentElement()
		if parent == nil {
			return false
		}
		return matchParts(parts, idx-1, parent)

	case '+': // Adjacent sibling: leftpart + elem
		sibling := elem.PreviousSiblingElement()
		if sibling == nil {
			return false
		}
		return matchParts(parts, idx-1, sibling)

	case '~': // General sibling: leftpart ~ elem
		var found bool
		elem.ForEachPrecedingSibling(func(sibling Element) bool {
			if matchParts(parts, idx-1, sibling) {
				found = true
				return false // stop iteration
			}
			return true // continue
		})
		return found

	default:
		// No combinator means this is part of a compound selector
		// which should have been merged into a single SimpleSelector
		return matchParts(parts, idx-1, elem)
	}
}

// matchSimpleSelector checks if a simple selector matches an element.
func matchSimpleSelector(sel *SimpleSelector, elem Element) bool {
	// Universal selector with no other constraints matches everything
	if sel.Universal && sel.TagName == "" && sel.ID == "" &&
		len(sel.Classes) == 0 && len(sel.PseudoClasses) == 0 &&
		len(sel.Attributes) == 0 && len(sel.PseudoElements) == 0 {
		return true
	}

	// Check tag name (case-insensitive)
	if sel.TagName != "" {
		if !strings.EqualFold(sel.TagName, elem.TagName()) {
			return false
		}
	}

	// Check ID
	if sel.ID != "" {
		if elem.ID() != sel.ID {
			return false
		}
	}

	// Check classes
	if len(sel.Classes) > 0 {
		elemClasses := elem.Classes()
		for _, reqClass := range sel.Classes {
			found := false
			for _, c := range elemClasses {
				if c == reqClass {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Check attributes
	for _, attr := range sel.Attributes {
		if !matchAttributeSelector(&attr, elem) {
			return false
		}
	}

	// Pseudo-classes and pseudo-elements require runtime state
	// For now, we skip them in the compiled matcher
	// They will be handled by the renderer's StyleManager

	return true
}

// matchAttributeSelector checks if an attribute selector matches an element.
func matchAttributeSelector(attr *AttributeSelector, elem Element) bool {
	value, exists := elem.GetAttribute(attr.Name)
	if !exists {
		return false
	}

	// Presence check only
	if attr.Operator == "" {
		return true
	}

	switch attr.Operator {
	case "=":
		return value == attr.Value
	case "~=":
		// Word match
		for _, word := range strings.Fields(value) {
			if word == attr.Value {
				return true
			}
		}
		return false
	case "|=":
		// Exact or prefix with hyphen
		return value == attr.Value || strings.HasPrefix(value, attr.Value+"-")
	case "^=":
		// Starts with
		return strings.HasPrefix(value, attr.Value)
	case "$=":
		// Ends with
		return strings.HasSuffix(value, attr.Value)
	case "*=":
		// Contains
		return strings.Contains(value, attr.Value)
	}

	return false
}

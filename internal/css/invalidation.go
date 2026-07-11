// Package css provides style invalidation for incremental style recalculation.
//
// M3.4: Style Invalidation
//
// The StyleInvalidator determines which nodes need style recalculation after
// a DOM mutation. It uses the CompiledStyleSheet's bucket structure to find
// affected rules efficiently, avoiding full-document recalculation.
//
// Key features:
//   - Mutation classification (class, ID, attribute, inline style, text, insertion, removal)
//   - Bucket-based affected rule lookup via CompiledStyleSheet
//   - Descendant invalidation when inherited CSS properties change
//   - Sibling invalidation for adjacent (+) and general sibling (~) combinators
//   - Mutation batching to coalesce multiple DOM changes before recalculation
//   - Zero-allocation invalidation result for common single-element cases
//
// Design:
//
//	The invalidator is conservative — it may over-invalidate rather than miss
//	nodes that need recalculation. This is safe: extra invalidation is a
//	performance cost, not a correctness issue.
//
// This is additive infrastructure. The existing renderer InvalidationTracker
// continues to work. The StyleInvalidator provides the CSS-aware analysis
// that determines *which* nodes the tracker should mark dirty.
package css

// MutationType classifies the kind of DOM mutation that occurred.
type MutationType uint8

const (
	// MutationClassChange is a class attribute addition, removal, or change.
	MutationClassChange MutationType = iota + 1
	// MutationIDChange is an ID attribute change.
	MutationIDChange
	// MutationAttributeChange is a generic attribute change.
	MutationAttributeChange
	// MutationInlineStyleChange is a style attribute change.
	MutationInlineStyleChange
	// MutationTextChange is a text content change.
	MutationTextChange
	// MutationInsertion is a node insertion into the tree.
	MutationInsertion
	// MutationRemoval is a node removal from the tree.
	MutationRemoval
)

// String returns a human-readable name for the mutation type.
func (mt MutationType) String() string {
	switch mt {
	case MutationClassChange:
		return "ClassChange"
	case MutationIDChange:
		return "IDChange"
	case MutationAttributeChange:
		return "AttributeChange"
	case MutationInlineStyleChange:
		return "InlineStyleChange"
	case MutationTextChange:
		return "TextChange"
	case MutationInsertion:
		return "Insertion"
	case MutationRemoval:
		return "Removal"
	default:
		return "Unknown"
	}
}

// Mutation records a single DOM mutation for style invalidation analysis.
type Mutation struct {
	// Type classifies the mutation.
	Type MutationType
	// Target is the element affected by the mutation.
	// Must implement the Element interface for selector matching.
	Target Element
	// OldAttr is the old attribute/class/ID value (for class/ID/attr changes).
	OldAttr string
	// NewAttr is the new attribute/class/ID value (for class/ID/attr changes).
	NewAttr string
	// Parent is the parent element (for insertions).
	Parent Element
}

// InvalidationResult describes what needs style recalculation after a mutation.
type InvalidationResult struct {
	// Self is true if the mutation target needs style recalculation.
	Self bool
	// InvalidateDescendants is true if all descendants need recalculation
	// (set when inherited CSS properties change).
	InvalidateDescendants bool
	// InvalidateNextSibling is true if the immediately following sibling
	// needs recalculation (set for adjacent sibling combinator '+').
	InvalidateNextSibling bool
	// InvalidateFollowingSiblings is true if all following siblings need
	// recalculation (set for general sibling combinator '~').
	InvalidateFollowingSiblings bool
	// LayoutDirty is true if layout needs recalculation (e.g., text change).
	LayoutDirty bool
	// Targets lists the specific elements that need invalidation.
	// For simple cases this contains just the mutation target.
	Targets []Element
}

// IsEmpty reports whether no invalidation is needed.
func (r *InvalidationResult) IsEmpty() bool {
	return !r.Self && !r.InvalidateDescendants &&
		!r.InvalidateNextSibling && !r.InvalidateFollowingSiblings &&
		!r.LayoutDirty && len(r.Targets) == 0
}

// StyleInvalidator analyzes DOM mutations against a compiled stylesheet
// to determine which elements need style recalculation.
type StyleInvalidator struct {
	sheet *CompiledStyleSheet

	// Batch state
	batching  bool
	batchMuts []Mutation
	batchSeen map[Element]bool
}

// NewStyleInvalidator creates a StyleInvalidator for the given compiled stylesheet.
func NewStyleInvalidator(sheet *CompiledStyleSheet) *StyleInvalidator {
	return &StyleInvalidator{
		sheet: sheet,
	}
}

// ComputeInvalidation analyzes a single mutation and returns what needs
// style recalculation.
func (inv *StyleInvalidator) ComputeInvalidation(m Mutation) InvalidationResult {
	if m.Target == nil {
		return InvalidationResult{}
	}

	switch m.Type {
	case MutationTextChange:
		return inv.invalidateTextChange(m)
	case MutationInlineStyleChange:
		return inv.invalidateInlineStyle(m)
	case MutationClassChange:
		return inv.invalidateClassChange(m)
	case MutationIDChange:
		return inv.invalidateIDChange(m)
	case MutationAttributeChange:
		return inv.invalidateAttributeChange(m)
	case MutationInsertion:
		return inv.invalidateInsertion(m)
	case MutationRemoval:
		return inv.invalidateRemoval(m)
	default:
		return InvalidationResult{}
	}
}

// BeginBatch starts collecting mutations for batched invalidation.
func (inv *StyleInvalidator) BeginBatch() {
	inv.batching = true
	inv.batchMuts = inv.batchMuts[:0]
	if inv.batchSeen == nil {
		inv.batchSeen = make(map[Element]bool)
	}
	for k := range inv.batchSeen {
		delete(inv.batchSeen, k)
	}
}

// RecordMutation adds a mutation to the current batch.
func (inv *StyleInvalidator) RecordMutation(m Mutation) {
	if !inv.batching {
		return
	}
	inv.batchMuts = append(inv.batchMuts, m)
}

// FlushBatch computes the combined invalidation result for all recorded
// mutations and resets the batch state.
func (inv *StyleInvalidator) FlushBatch() InvalidationResult {
	inv.batching = false
	if len(inv.batchMuts) == 0 {
		return InvalidationResult{}
	}

	var combined InvalidationResult
	targetSet := make(map[Element]bool)

	for _, m := range inv.batchMuts {
		r := inv.ComputeInvalidation(m)
		combined.Self = combined.Self || r.Self
		combined.InvalidateDescendants = combined.InvalidateDescendants || r.InvalidateDescendants
		combined.InvalidateNextSibling = combined.InvalidateNextSibling || r.InvalidateNextSibling
		combined.InvalidateFollowingSiblings = combined.InvalidateFollowingSiblings || r.InvalidateFollowingSiblings
		combined.LayoutDirty = combined.LayoutDirty || r.LayoutDirty

		// Deduplicate targets
		for _, t := range r.Targets {
			if t != nil && !targetSet[t] {
				targetSet[t] = true
				combined.Targets = append(combined.Targets, t)
			}
		}
	}

	inv.batchMuts = inv.batchMuts[:0]
	for k := range inv.batchSeen {
		delete(inv.batchSeen, k)
	}

	return combined
}

// --- Private invalidation methods ---

// invalidateTextChange handles text content changes.
// Text changes affect layout but not style.
func (inv *StyleInvalidator) invalidateTextChange(m Mutation) InvalidationResult {
	return InvalidationResult{
		LayoutDirty: true,
		Targets:     []Element{m.Target},
	}
}

// invalidateInlineStyle handles inline style attribute changes.
// Always invalidates self since inline styles override stylesheet rules.
func (inv *StyleInvalidator) invalidateInlineStyle(m Mutation) InvalidationResult {
	return InvalidationResult{
		Self:    true,
		Targets: []Element{m.Target},
	}
}

// invalidateClassChange handles class additions, removals, and changes.
func (inv *StyleInvalidator) invalidateClassChange(m Mutation) InvalidationResult {
	result := InvalidationResult{Targets: []Element{m.Target}}

	affected := inv.affectedRuleIndices(m)
	if len(affected) == 0 && !inv.hasUniversalRules() {
		// No rules reference this class — check if old/new class has rules
		if !inv.classHasRules(m.OldAttr) && !inv.classHasRules(m.NewAttr) {
			return InvalidationResult{}
		}
	}

	result.Self = true

	// Check if affected rules contain inherited properties
	if inv.rulesContainInherited(affected) {
		result.InvalidateDescendants = true
	}

	// Check for sibling combinators
	inv.addSiblingInvalidation(&result)

	return result
}

// invalidateIDChange handles ID attribute changes.
func (inv *StyleInvalidator) invalidateIDChange(m Mutation) InvalidationResult {
	result := InvalidationResult{
		Self:    true,
		Targets: []Element{m.Target},
	}

	affected := inv.affectedRuleIndices(m)
	if inv.rulesContainInherited(affected) {
		result.InvalidateDescendants = true
	}

	inv.addSiblingInvalidation(&result)
	return result
}

// invalidateAttributeChange handles generic attribute changes.
func (inv *StyleInvalidator) invalidateAttributeChange(m Mutation) InvalidationResult {
	result := InvalidationResult{
		Self:    true,
		Targets: []Element{m.Target},
	}

	affected := inv.affectedRuleIndices(m)
	if inv.rulesContainInherited(affected) {
		result.InvalidateDescendants = true
	}

	inv.addSiblingInvalidation(&result)
	return result
}

// invalidateInsertion handles node insertion.
func (inv *StyleInvalidator) invalidateInsertion(m Mutation) InvalidationResult {
	result := InvalidationResult{
		Self:    true,
		Targets: []Element{m.Target},
	}

	// Check for sibling combinators in the stylesheet
	inv.addSiblingInvalidation(&result)

	return result
}

// invalidateRemoval handles node removal.
func (inv *StyleInvalidator) invalidateRemoval(m Mutation) InvalidationResult {
	result := InvalidationResult{
		Self:    true,
		Targets: []Element{m.Target},
	}

	// Removal may affect sibling selectors
	inv.addSiblingInvalidation(&result)

	return result
}

// --- Analysis helpers ---

// affectedRuleIndices returns the indices of rules in the compiled stylesheet
// that could be affected by the given mutation.
func (inv *StyleInvalidator) affectedRuleIndices(m Mutation) []int {
	if inv.sheet == nil {
		return nil
	}

	var indices []int

	switch m.Type {
	case MutationClassChange:
		// Check both old and new class buckets
		if m.OldAttr != "" {
			indices = append(indices, inv.sheet.classBucket[m.OldAttr]...)
		}
		if m.NewAttr != "" && m.NewAttr != m.OldAttr {
			indices = append(indices, inv.sheet.classBucket[m.NewAttr]...)
		}
		// Also check tag bucket since the element's tag may match rules
		if m.Target != nil {
			tag := toLower(m.Target.TagName())
			indices = append(indices, inv.sheet.tagBucket[tag]...)
		}

	case MutationIDChange:
		if m.OldAttr != "" {
			indices = append(indices, inv.sheet.idBucket[m.OldAttr]...)
		}
		if m.NewAttr != "" && m.NewAttr != m.OldAttr {
			indices = append(indices, inv.sheet.idBucket[m.NewAttr]...)
		}

	case MutationAttributeChange:
		if m.OldAttr != "" {
			indices = append(indices, inv.sheet.attrBucket[m.OldAttr]...)
		}
		if m.NewAttr != "" && m.NewAttr != m.OldAttr {
			indices = append(indices, inv.sheet.attrBucket[m.NewAttr]...)
		}
		// Also check tag bucket
		if m.Target != nil {
			tag := toLower(m.Target.TagName())
			indices = append(indices, inv.sheet.tagBucket[tag]...)
		}

	case MutationInlineStyleChange, MutationInsertion, MutationRemoval:
		// These always affect the target; include tag-based rules
		if m.Target != nil {
			tag := toLower(m.Target.TagName())
			indices = append(indices, inv.sheet.tagBucket[tag]...)
		}
	}

	// Universal bucket always applies
	indices = append(indices, inv.sheet.universalBucket...)

	return dedupInts(indices)
}

// rulesContainInherited checks if any of the given rules contain inherited
// CSS properties.
func (inv *StyleInvalidator) rulesContainInherited(indices []int) bool {
	if inv.sheet == nil {
		return false
	}
	for _, idx := range indices {
		if idx >= 0 && idx < len(inv.sheet.rules) {
			if declarationsContainInherited(inv.sheet.rules[idx].Declarations) {
				return true
			}
		}
	}
	return false
}

// declarationsContainInherited checks if any declaration is an inherited property.
func declarationsContainInherited(decls []Declaration) bool {
	for i := range decls {
		if IsInheritedProperty(decls[i].Property) {
			return true
		}
	}
	return false
}

// hasSiblingCombinator checks if any rule in the stylesheet uses sibling
// combinators (+ or ~).
func (inv *StyleInvalidator) hasSiblingCombinator() bool {
	if inv.sheet == nil {
		return false
	}
	for i := range inv.sheet.rules {
		for j := range inv.sheet.rules[i].Selectors {
			for k := range inv.sheet.rules[i].Selectors[j].Parts {
				c := inv.sheet.rules[i].Selectors[j].Parts[k].Combinator
				if c == '+' || c == '~' {
					return true
				}
			}
		}
	}
	return false
}

// addSiblingInvalidation checks for sibling combinators and sets the
// appropriate invalidation flags.
func (inv *StyleInvalidator) addSiblingInvalidation(result *InvalidationResult) {
	if inv.sheet == nil {
		return
	}
	// Scan rules for sibling combinators
	for i := range inv.sheet.rules {
		for j := range inv.sheet.rules[i].Selectors {
			sel := &inv.sheet.rules[i].Selectors[j]
			for k := range sel.Parts {
				switch sel.Parts[k].Combinator {
				case '+':
					result.InvalidateNextSibling = true
				case '~':
					result.InvalidateFollowingSiblings = true
				}
			}
		}
	}
}

// classHasRules checks if any rule in the stylesheet has a class bucket
// entry for the given class name.
func (inv *StyleInvalidator) classHasRules(class string) bool {
	if inv.sheet == nil || class == "" {
		return false
	}
	_, ok := inv.sheet.classBucket[class]
	return ok
}

// hasUniversalRules checks if the stylesheet has universal bucket rules.
func (inv *StyleInvalidator) hasUniversalRules() bool {
	if inv.sheet == nil {
		return false
	}
	return len(inv.sheet.universalBucket) > 0
}

// --- Utility functions ---

// toLower converts ASCII string to lowercase without allocation for common cases.
func toLower(s string) string {
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// dedupInts removes duplicate integers from a sorted-ish slice.
// Uses a map for O(n) deduplication.
func dedupInts(s []int) []int {
	if len(s) <= 1 {
		return s
	}
	seen := make(map[int]bool, len(s))
	result := s[:0]
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

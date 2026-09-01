package renderer

import (
	"fmt"
	"hash"
	"hash/fnv"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/css"
)

// StyleManager applies styles from a stylesheet to a render tree.
type StyleManager struct {
	defaultStylesheet *css.StyleSheet
	stylesheet        *css.StyleSheet
	viewportWidth     float32
	viewportHeight    float32
	mediaEvaluator    *MediaQueryEvaluator

	// prepared caches the flattened selector parts for each stylesheet so the
	// per-node matching loop does not re-flatten (and re-allocate) every
	// selector on every rule×node pair. Built once per stylesheet on first
	// use within a render pass.
	prepared map[*css.StyleSheet][]preparedRule

	// mediaMatch memoizes @media prelude evaluations for the current
	// viewport; cleared by SetViewport.
	mediaMatch map[string]bool
}

// preparedSelector holds a flattened selector part sequence and its precomputed specificity.
type preparedSelector struct {
	parts       []styleSelectorPart
	specificity [3]uint16
}

// preparedRule is precomputed selector-part data for one CSS rule. The
// selectors field holds the preparedSelector for each of the
// rule's selector sequences; declarations is the rule's declarations. It is
// immutable for the duration of a style pass, so it can be shared across all
// nodes without re-computation.
type preparedRule struct {
	selectors    []preparedSelector
	declarations []css.Declaration
	sourceOrder  int
}

// NewStyleManager creates a new StyleManager.
func NewStyleManager(stylesheet *css.StyleSheet) *StyleManager {
	return &StyleManager{
		stylesheet:        stylesheet,
		defaultStylesheet: GetDefaultStyleSheet(),
	}
}

// NewStyleManagerWithViewport creates a StyleManager with viewport dimensions for media query support.
func NewStyleManagerWithViewport(stylesheet *css.StyleSheet, viewportWidth, viewportHeight float32) *StyleManager {
	return &StyleManager{
		defaultStylesheet: GetDefaultStyleSheet(),
		stylesheet:        stylesheet,
		viewportWidth:     viewportWidth,
		viewportHeight:    viewportHeight,
		mediaEvaluator:    NewMediaQueryEvaluator(viewportWidth, viewportHeight),
	}
}

// SetViewport updates the viewport dimensions for media query evaluation.
func (sm *StyleManager) SetViewport(width, height float32) {
	sm.viewportWidth = width
	sm.viewportHeight = height
	sm.mediaMatch = nil // prelude results are viewport-dependent
	if sm.mediaEvaluator != nil {
		sm.mediaEvaluator.UpdateViewport(width, height)
	} else {
		sm.mediaEvaluator = NewMediaQueryEvaluator(width, height)
	}
}

// ApplyStyles applies the styles to the given render tree.
func (sm *StyleManager) ApplyStyles(node *RenderNode) {
	if node == nil {
		return
	}

	// Reset the style pool at the start of each render pass (root node).
	if node.Parent == nil {
		globalStylePool.Reset()
	}

	if node.ComputedStyle == nil {
		node.ComputedStyle = &Style{
			Opacity: 1.0,
			Display: css.DisplayAtomBlock, // Default to block for now, ideally depends on tag
		}
	}

	// Inherit styles from parent
	if node.Parent != nil && node.Parent.ComputedStyle != nil {
		node.ComputedStyle.Color = node.Parent.ComputedStyle.Color
		node.ComputedStyle.FontSize = node.Parent.ComputedStyle.FontSize
		node.ComputedStyle.FontWeight = node.Parent.ComputedStyle.FontWeight
		node.ComputedStyle.FontFamily = node.Parent.ComputedStyle.FontFamily
		node.ComputedStyle.LetterSpacing = node.Parent.ComputedStyle.LetterSpacing
		node.ComputedStyle.LineHeight = node.Parent.ComputedStyle.LineHeight
		node.ComputedStyle.FontStyle = node.Parent.ComputedStyle.FontStyle
		node.ComputedStyle.TextDecoration = node.Parent.ComputedStyle.TextDecoration
		node.ComputedStyle.TextTransform = node.Parent.ComputedStyle.TextTransform
		node.ComputedStyle.TextAlign = node.Parent.ComputedStyle.TextAlign
		node.ComputedStyle.WhiteSpace = node.Parent.ComputedStyle.WhiteSpace
		node.ComputedStyle.ListStyleType = node.Parent.ComputedStyle.ListStyleType
		node.ComputedStyle.ListStylePosition = node.Parent.ComputedStyle.ListStylePosition
		node.ComputedStyle.Visibility = node.Parent.ComputedStyle.Visibility

		// Inherit custom properties from parent via pointer sharing (copy-on-write).
		// The child shares the parent's map; cloning happens only when the child
		// sets its own custom property in applyDeclaration.
		if node.Parent.ComputedStyle.CustomProperties != nil {
			node.ComputedStyle.CustomProperties = node.Parent.ComputedStyle.CustomProperties
		}
	}

	sm.applyMatchingRules(sm.defaultStylesheet, node)
	sm.applyMatchingRules(sm.stylesheet, node)

	// Apply @media rules if viewport is set
	if sm.mediaEvaluator != nil {
		sm.applyMediaRules(node)
	}

	// Apply inline styles from style attribute
	if styleAttr, ok := node.GetAttribute("style"); ok && styleAttr != "" {
		sm.applyInlineStyles(node, styleAttr)
	}

	// CSS 2.1 Section 9.7: If float is not 'none' or position is absolute/fixed, display is converted to block
	if node.ComputedStyle != nil {
		if node.ComputedStyle.Position == css.PositionAtomAbsolute || node.ComputedStyle.Position == css.PositionAtomFixed ||
			node.ComputedStyle.Float == css.FloatAtomLeft || node.ComputedStyle.Float == css.FloatAtomRight {
			if node.ComputedStyle.Display == css.DisplayAtomInline || node.ComputedStyle.Display == css.DisplayAtomInlineBlock {
				node.ComputedStyle.Display = css.DisplayAtomBlock
			}
		}
	}

	// Before interning, clone CustomProperties if non-nil so the interned
	// style owns its own map and is truly immutable. This avoids mutation
	// of shared maps (e.g., pointer-shared from parent during inheritance).
	if node.ComputedStyle != nil && node.ComputedStyle.CustomProperties != nil {
		cloned := make(map[string]string, len(node.ComputedStyle.CustomProperties))
		for k, v := range node.ComputedStyle.CustomProperties {
			cloned[k] = v
		}
		node.ComputedStyle.CustomProperties = cloned
	}

	// Intern the computed style for deduplication.
	if node.ComputedStyle != nil {
		node.ComputedStyle = globalStylePool.Intern(node.ComputedStyle)
	}

	for _, child := range node.Children {
		sm.ApplyStyles(child)
	}
}

// applyInlineStyles parses and applies inline styles from the style attribute
func (sm *StyleManager) applyInlineStyles(node *RenderNode, styleAttr string) {
	declarations, err := css.ParseStyleAttribute(styleAttr)
	if err != nil {
		// Just ignore parsing errors for now
		return
	}

	for _, decl := range declarations {
		sm.applyDeclaration(node, decl)
	}
}

// applyMediaRules applies styles from @media rules that match current viewport.
func (sm *StyleManager) applyMediaRules(node *RenderNode) {
	sm.applyMediaRulesForStylesheet(sm.defaultStylesheet, node)
	sm.applyMediaRulesForStylesheet(sm.stylesheet, node)
}

func (sm *StyleManager) applyMediaRulesForStylesheet(stylesheet *css.StyleSheet, node *RenderNode) {
	if stylesheet == nil || sm.mediaEvaluator == nil {
		return
	}

	sm.applyConditionalAtRules(stylesheet.AtRules, node, true)
}

// mediaMatchFor evaluates an @media prelude with memoization. Style
// application walks every node against every conditional rule; without the
// cache each prelude would be re-parsed and re-evaluated per node. The cache
// is dropped by SetViewport, on which all results depend.
func (sm *StyleManager) mediaMatchFor(prelude string) bool {
	if sm.mediaMatch == nil {
		sm.mediaMatch = make(map[string]bool)
	}
	if m, ok := sm.mediaMatch[prelude]; ok {
		return m
	}
	m := sm.mediaEvaluator.Evaluate(prelude)
	sm.mediaMatch[prelude] = m
	return m
}

func (sm *StyleManager) applyConditionalAtRules(atRules []css.AtRule, node *RenderNode, parentMatches bool) {
	for _, atRule := range atRules {
		matches := parentMatches
		switch atRule.Name {
		case "media":
			matches = matches && sm.mediaMatchFor(atRule.Prelude)
		case "supports":
			// Unsupported feature tests are treated permissively until capability
			// detection is modeled explicitly.
		default:
			matches = false
		}
		if !matches {
			continue
		}

		type matchedAtRuleItem struct {
			declarations []css.Declaration
			specificity  [3]uint16
			sourceOrder  int
		}
		var matchedAt []matchedAtRuleItem

		for i, rule := range atRule.Rules {
			matchedSel := false
			var bestSpec [3]uint16
			for _, selectorSeq := range rule.Selectors {
				if sm.matchesSequence(selectorSeq, node) {
					spec := css.ComputeSpecificity(&selectorSeq)
					if !matchedSel || css.CompareSpecificity(spec, bestSpec) > 0 {
						bestSpec = spec
					}
					matchedSel = true
				}
			}
			if matchedSel {
				matchedAt = append(matchedAt, matchedAtRuleItem{
					declarations: rule.Declarations,
					specificity:  bestSpec,
					sourceOrder:  i,
				})
			}
		}

		sort.SliceStable(matchedAt, func(i, j int) bool {
			cmp := css.CompareSpecificity(matchedAt[i].specificity, matchedAt[j].specificity)
			if cmp != 0 {
				return cmp < 0
			}
			return matchedAt[i].sourceOrder < matchedAt[j].sourceOrder
		})

		for _, m := range matchedAt {
			for _, decl := range m.declarations {
				if !decl.Important {
					sm.applyDeclaration(node, decl)
				}
			}
		}
		for _, m := range matchedAt {
			for _, decl := range m.declarations {
				if decl.Important {
					sm.applyDeclaration(node, decl)
				}
			}
		}

		sm.applyConditionalAtRules(atRule.AtRules, node, matches)
	}
}

func (sm *StyleManager) applyMatchingRules(stylesheet *css.StyleSheet, node *RenderNode) {
	if stylesheet == nil {
		return
	}
	type matchedRuleItem struct {
		declarations []css.Declaration
		specificity  [3]uint16
		sourceOrder  int
	}
	var matched []matchedRuleItem

	for _, rule := range sm.preparedFor(stylesheet) {
		matchedSel := false
		var bestSpec [3]uint16
		for _, sel := range rule.selectors {
			if sm.matchStyleParts(sel.parts, len(sel.parts)-1, node) {
				if !matchedSel || css.CompareSpecificity(sel.specificity, bestSpec) > 0 {
					bestSpec = sel.specificity
				}
				matchedSel = true
			}
		}
		if matchedSel {
			matched = append(matched, matchedRuleItem{
				declarations: rule.declarations,
				specificity:  bestSpec,
				sourceOrder:  rule.sourceOrder,
			})
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		cmp := css.CompareSpecificity(matched[i].specificity, matched[j].specificity)
		if cmp != 0 {
			return cmp < 0
		}
		return matched[i].sourceOrder < matched[j].sourceOrder
	})

	// Pass 1: Apply normal declarations in sorted specificity / source order
	for _, m := range matched {
		for _, decl := range m.declarations {
			if !decl.Important {
				sm.applyDeclaration(node, decl)
			}
		}
	}

	// Pass 2: Apply !important declarations in sorted specificity / source order
	for _, m := range matched {
		for _, decl := range m.declarations {
			if decl.Important {
				sm.applyDeclaration(node, decl)
			}
		}
	}
}

// preparedFor returns the flattened-selector representation of a stylesheet,
// computing it once and caching it so the node-matching loops in
// applyMatchingRules do not re-flatten selectors (and re-allocate) for every
// node in the tree. Only the main and default stylesheets are cached, keyed
// by pointer; the map is bounded per render pass.
func (sm *StyleManager) preparedFor(stylesheet *css.StyleSheet) []preparedRule {
	if sm.prepared == nil {
		sm.prepared = make(map[*css.StyleSheet][]preparedRule)
	}
	if rules, ok := sm.prepared[stylesheet]; ok {
		return rules
	}
	rules := make([]preparedRule, 0, len(stylesheet.Rules))
	for i, rule := range stylesheet.Rules {
		pr := preparedRule{
			declarations: rule.Declarations,
			sourceOrder:  i,
		}
		for _, ss := range rule.Selectors {
			spec := css.ComputeSpecificity(&ss)
			pr.selectors = append(pr.selectors, preparedSelector{
				parts:       flattenSelectorSequence(&ss),
				specificity: spec,
			})
		}
		rules = append(rules, pr)
	}
	sm.prepared[stylesheet] = rules
	return rules
}

type styleSelectorPart struct {
	Selector   css.SimpleSelector
	Combinator string
}

func flattenSelectorSequence(seq *css.SelectorSequence) []styleSelectorPart {
	var parts []styleSelectorPart
	for s := seq; s != nil; s = s.Next {
		parts = append(parts, styleSelectorPart{
			Selector:   s.Simple,
			Combinator: s.Combinator,
		})
	}
	return parts
}

// matchesSequence checks if a selector sequence matches a node
func (sm *StyleManager) matchesSequence(seq css.SelectorSequence, node *RenderNode) bool {
	parts := flattenSelectorSequence(&seq)
	return sm.matchStyleParts(parts, len(parts)-1, node)
}

// matchStyleParts recursively matches selector parts from right to left.
func (sm *StyleManager) matchStyleParts(parts []styleSelectorPart, idx int, node *RenderNode) bool {
	if idx < 0 || node == nil {
		return false
	}

	part := &parts[idx]

	// Check if current node matches this part's simple selector
	if !sm.matchesSimple(part.Selector, node) {
		return false
	}

	// If this is the leftmost part, we're done
	if idx == 0 {
		return true
	}

	// Check combinator with the part to the left
	leftPart := &parts[idx-1]
	switch leftPart.Combinator {
	case " ": // Descendant combinator
		current := node.Parent
		for current != nil {
			if sm.matchStyleParts(parts, idx-1, current) {
				return true
			}
			current = current.Parent
		}
		return false

	case ">": // Child combinator
		if node.Parent == nil {
			return false
		}
		return sm.matchStyleParts(parts, idx-1, node.Parent)

	case "+": // Adjacent sibling combinator
		sibling := sm.getPreviousSibling(node)
		if sibling == nil {
			return false
		}
		return sm.matchStyleParts(parts, idx-1, sibling)

	case "~": // General sibling combinator
		sibling := sm.getPreviousSibling(node)
		for sibling != nil {
			if sm.matchStyleParts(parts, idx-1, sibling) {
				return true
			}
			sibling = sm.getPreviousSibling(sibling)
		}
		return false

	default:
		return sm.matchStyleParts(parts, idx-1, node)
	}
}

// getPreviousSibling returns the previous sibling of a node
func (sm *StyleManager) getPreviousSibling(node *RenderNode) *RenderNode {
	if node.Parent == nil {
		return nil
	}

	for i, child := range node.Parent.Children {
		if child == node && i > 0 {
			return node.Parent.Children[i-1]
		}
	}

	return nil
}

// matchesSimple checks if a simple selector matches a node
func (sm *StyleManager) matchesSimple(selector css.SimpleSelector, node *RenderNode) bool {
	// If the selector targets a pseudo-element, it should never match a normal RenderNode
	if len(selector.PseudoElements) > 0 {
		return false
	}

	// Universal selector matches everything only when it has no other constraints
	if selector.Universal && selector.TagName == "" && selector.ID == "" &&
		len(selector.Classes) == 0 && len(selector.PseudoClasses) == 0 &&
		len(selector.Attributes) == 0 && len(selector.PseudoElements) == 0 {
		return true
	}

	// Check tag name
	if selector.TagName != "" && selector.TagName != node.TagName {
		return false
	}

	// Check ID
	if selector.ID != "" {
		id, ok := node.GetAttribute("id")
		if !ok || id != selector.ID {
			return false
		}
	}

	// Check classes
	if len(selector.Classes) > 0 {
		classAttr, ok := node.GetAttribute("class")
		if !ok {
			return false
		}
		classes := strings.Fields(classAttr)
		for _, selClass := range selector.Classes {
			found := false
			for _, nodeClass := range classes {
				if selClass == nodeClass {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Check pseudo-classes
	for _, pseudoClass := range selector.PseudoClasses {
		if !sm.matchesPseudoClass(pseudoClass, node) {
			return false
		}
	}

	// Check attributes
	for _, attr := range selector.Attributes {
		if !sm.matchesAttribute(attr, node) {
			return false
		}
	}

	return true
}

// matchesPseudoClass checks if a pseudo-class matches a node
func (sm *StyleManager) matchesPseudoClass(pseudoClass string, node *RenderNode) bool {
	// Handle functional pseudo-classes
	if strings.HasPrefix(pseudoClass, "nth-child(") && strings.HasSuffix(pseudoClass, ")") {
		arg := pseudoClass[len("nth-child(") : len(pseudoClass)-1]
		if node.Parent == nil {
			return false
		}

		// Find node's index (1-based)
		index := -1
		for i, child := range node.Parent.Children {
			if child == node {
				index = i + 1
				break
			}
		}

		if index == -1 {
			return false
		}

		if arg == "even" {
			return index%2 == 0
		} else if arg == "odd" {
			return index%2 != 0
		}

		// Handle numeric values
		if n, err := strconv.Atoi(arg); err == nil {
			return index == n
		}

		return false
	}

	switch pseudoClass {
	case "root":
		return node.Parent == nil
	case "link", "visited":
		return node.TagName == "a"
	case "hover", "focus", "active":
		// These require state tracking, not implemented yet
		return false
	case "first-child":
		if node.Parent == nil || len(node.Parent.Children) == 0 {
			return false
		}
		return node.Parent.Children[0] == node
	case "last-child":
		if node.Parent == nil || len(node.Parent.Children) == 0 {
			return false
		}
		return node.Parent.Children[len(node.Parent.Children)-1] == node
	default:
		return false
	}
}

// matchesAttribute checks if an attribute selector matches a node
func (sm *StyleManager) matchesAttribute(attr css.AttributeSelector, node *RenderNode) bool {
	value, ok := node.GetAttribute(attr.Name)
	if !ok {
		return false
	}

	if attr.Operator == "" {
		// Just checking for attribute presence
		return true
	}

	switch attr.Operator {
	case "=":
		return value == attr.Value
	case "~=":
		// Word match
		words := strings.Fields(value)
		for _, word := range words {
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

// Legacy function for backward compatibility with old Selector type
func (sm *StyleManager) matches(selector css.SimpleSelector, node *RenderNode) bool {
	return sm.matchesSimple(selector, node)
}

// colorNameToHex maps CSS named colors (CSS Level 4) to their hex values.
var colorNameToHex = map[string]string{
	// CSS Level 1
	"black":   "#000000",
	"silver":  "#c0c0c0",
	"gray":    "#808080",
	"white":   "#ffffff",
	"maroon":  "#800000",
	"red":     "#ff0000",
	"purple":  "#800080",
	"fuchsia": "#ff00ff",
	"green":   "#008000",
	"lime":    "#00ff00",
	"olive":   "#808000",
	"yellow":  "#ffff00",
	"navy":    "#000080",
	"blue":    "#0000ff",
	"teal":    "#008080",
	"aqua":    "#00ffff",
	// CSS Level 2
	"orange": "#ffa500",
	// CSS Level 3 / SVG
	"aliceblue":            "#f0f8ff",
	"antiquewhite":         "#faebd7",
	"aquamarine":           "#7fffd4",
	"azure":                "#f0ffff",
	"beige":                "#f5f5dc",
	"bisque":               "#ffe4c4",
	"blanchedalmond":       "#ffebcd",
	"blueviolet":           "#8a2be2",
	"brown":                "#a52a2a",
	"burlywood":            "#deb887",
	"cadetblue":            "#5f9ea0",
	"chartreuse":           "#7fff00",
	"chocolate":            "#d2691e",
	"coral":                "#ff7f50",
	"cornflowerblue":       "#6495ed",
	"cornsilk":             "#fff8dc",
	"crimson":              "#dc143c",
	"cyan":                 "#00ffff",
	"darkblue":             "#00008b",
	"darkcyan":             "#008b8b",
	"darkgoldenrod":        "#b8860b",
	"darkgray":             "#a9a9a9",
	"darkgreen":            "#006400",
	"darkgrey":             "#a9a9a9",
	"darkkhaki":            "#bdb76b",
	"darkmagenta":          "#8b008b",
	"darkolivegreen":       "#556b2f",
	"darkorange":           "#ff8c00",
	"darkorchid":           "#9932cc",
	"darkred":              "#8b0000",
	"darksalmon":           "#e9967a",
	"darkseagreen":         "#8fbc8f",
	"darkslateblue":        "#483d8b",
	"darkslategray":        "#2f4f4f",
	"darkslategrey":        "#2f4f4f",
	"darkturquoise":        "#00ced1",
	"darkviolet":           "#9400d3",
	"deeppink":             "#ff1493",
	"deepskyblue":          "#00bfff",
	"dimgray":              "#696969",
	"dimgrey":              "#696969",
	"dodgerblue":           "#1e90ff",
	"firebrick":            "#b22222",
	"floralwhite":          "#fffaf0",
	"forestgreen":          "#228b22",
	"gainsboro":            "#dcdcdc",
	"ghostwhite":           "#f8f8ff",
	"gold":                 "#ffd700",
	"goldenrod":            "#daa520",
	"greenyellow":          "#adff2f",
	"grey":                 "#808080",
	"honeydew":             "#f0fff0",
	"hotpink":              "#ff69b4",
	"indianred":            "#cd5c5c",
	"indigo":               "#4b0082",
	"ivory":                "#fffff0",
	"khaki":                "#f0e68c",
	"lavender":             "#e6e6fa",
	"lavenderblush":        "#fff0f5",
	"lawngreen":            "#7cfc00",
	"lemonchiffon":         "#fffacd",
	"lightblue":            "#add8e6",
	"lightcoral":           "#f08080",
	"lightcyan":            "#e0ffff",
	"lightgoldenrodyellow": "#fafad2",
	"lightgray":            "#d3d3d3",
	"lightgreen":           "#90ee90",
	"lightgrey":            "#d3d3d3",
	"lightpink":            "#ffb6c1",
	"lightsalmon":          "#ffa07a",
	"lightseagreen":        "#20b2aa",
	"lightskyblue":         "#87cefa",
	"lightslategray":       "#778899",
	"lightslategrey":       "#778899",
	"lightsteelblue":       "#b0c4de",
	"lightyellow":          "#ffffe0",
	"limegreen":            "#32cd32",
	"linen":                "#faf0e6",
	"magenta":              "#ff00ff",
	"mediumaquamarine":     "#66cdaa",
	"mediumblue":           "#0000cd",
	"mediumorchid":         "#ba55d3",
	"mediumpurple":         "#9370db",
	"mediumseagreen":       "#3cb371",
	"mediumslateblue":      "#7b68ee",
	"mediumspringgreen":    "#00fa9a",
	"mediumturquoise":      "#48d1cc",
	"mediumvioletred":      "#c71585",
	"midnightblue":         "#191970",
	"mintcream":            "#f5fffa",
	"mistyrose":            "#ffe4e1",
	"moccasin":             "#ffe4b5",
	"navajowhite":          "#ffdead",
	"oldlace":              "#fdf5e6",
	"olivedrab":            "#6b8e23",
	"orangered":            "#ff4500",
	"orchid":               "#da70d6",
	"palegoldenrod":        "#eee8aa",
	"palegreen":            "#98fb98",
	"paleturquoise":        "#afeeee",
	"palevioletred":        "#db7093",
	"papayawhip":           "#ffefd5",
	"peachpuff":            "#ffdab9",
	"peru":                 "#cd853f",
	"pink":                 "#ffc0cb",
	"plum":                 "#dda0dd",
	"powderblue":           "#b0e0e6",
	"rosybrown":            "#bc8f8f",
	"royalblue":            "#4169e1",
	"saddlebrown":          "#8b4513",
	"salmon":               "#fa8072",
	"sandybrown":           "#f4a460",
	"seagreen":             "#2e8b57",
	"seashell":             "#fff5ee",
	"sienna":               "#a0522d",
	"skyblue":              "#87ceeb",
	"slateblue":            "#6a5acd",
	"slategray":            "#708090",
	"slategrey":            "#708090",
	"snow":                 "#fffafa",
	"springgreen":          "#00ff7f",
	"steelblue":            "#4682b4",
	"tan":                  "#d2b48c",
	"thistle":              "#d8bfd8",
	"tomato":               "#ff6347",
	"turquoise":            "#40e0d0",
	"violet":               "#ee82ee",
	"wheat":                "#f5deb3",
	"whitesmoke":           "#f5f5f5",
	"yellowgreen":          "#9acd32",
	// CSS Level 4
	"rebeccapurple": "#663399",
}

func (sm *StyleManager) applyDeclaration(node *RenderNode, decl css.Declaration) {
	// Resolve var() tokens using this element's custom properties
	if strings.Contains(decl.Value, "var(") {
		resolved := resolveVarTokens(decl.Value, node.ComputedStyle)
		if resolved == "" {
			return // unresolved variable with no fallback, skip
		}
		decl.Value = resolved
	}

	if node.Styles == nil {
		node.Styles = make(map[string]string)
	}
	node.Styles[decl.Property] = decl.Value

	style := node.ComputedStyle

	// CSS custom property declaration (e.g. --color-base: #ff0000)
	if strings.HasPrefix(decl.Property, "--") {
		if node.ComputedStyle.CustomProperties == nil {
			node.ComputedStyle.CustomProperties = make(map[string]string)
		} else {
			// Copy-on-write: clone the map before mutating so we don't
			// modify a map shared with the parent or sibling nodes.
			cloned := make(map[string]string, len(node.ComputedStyle.CustomProperties))
			for k, v := range node.ComputedStyle.CustomProperties {
				cloned[k] = v
			}
			node.ComputedStyle.CustomProperties = cloned
		}
		node.ComputedStyle.CustomProperties[decl.Property] = strings.TrimSpace(decl.Value)
		return
	}

	switch decl.Property {
	case "display":
		style.Display = css.DisplayAtomFromString(decl.Value)
	case "visibility":
		style.Visibility = css.VisibilityAtomFromString(decl.Value)
	case "font-size":
		parentFontSize := float32(16.0) // Default font size
		if node.Parent != nil && node.Parent.ComputedStyle != nil && node.Parent.ComputedStyle.FontSize > 0 {
			parentFontSize = node.Parent.ComputedStyle.FontSize
		}
		if val, err := parseFontSize(decl.Value, parentFontSize); err == nil {
			style.FontSize = val
		}
	case "font-weight":
		style.FontWeight = decl.Value
	case "color":
		if val, err := parseColor(decl.Value); err == nil {
			style.Color = val
		}
	case "background-color":
		if val, err := parseColor(decl.Value); err == nil {
			style.BackgroundColor = val
		}
	case "background-image":
		style.BackgroundImage = decl.Value
	case "background-repeat":
		style.BackgroundRepeat = css.BackgroundRepeatAtomFromString(decl.Value)
	case "background-position":
		style.BackgroundPosition = css.BackgroundPositionAtomFromString(decl.Value)
	case "background-size":
		style.BackgroundSize = css.BackgroundSizeAtomFromString(decl.Value)
	case "background-attachment":
		style.BackgroundAttachment = css.BackgroundAttachmentAtomFromString(decl.Value)
	case "background":
		applyBackgroundShorthand(style, decl.Value)
	case "width":
		style.Width = decl.Value
	case "height":
		style.Height = decl.Value
	case "font-family":
		style.FontFamily = decl.Value
	case "opacity":
		if val, err := strconv.ParseFloat(decl.Value, 32); err == nil {
			style.Opacity = float32(val)
		}
	case "text-align":
		style.TextAlign = css.TextAlignAtomFromString(decl.Value)
	case "letter-spacing":
		if decl.Value == "normal" {
			style.LetterSpacing = 0
		} else {
			style.LetterSpacing = parseLength(decl.Value, style.FontSize)
		}
	case "line-height":
		style.LineHeight = parseLineHeight(decl.Value, style.FontSize)
	case "font-style":
		style.FontStyle = css.FontStyleAtomFromString(decl.Value)
	case "text-decoration":
		style.TextDecoration = css.TextDecorationAtomFromString(decl.Value)
	case "text-transform":
		style.TextTransform = css.TextTransformAtomFromString(decl.Value)

	// Margin properties
	case "margin":
		// Shorthand: apply to all sides
		values := parseBoxShorthand(decl.Value)
		style.MarginTop = values[0]
		style.MarginRight = values[1]
		style.MarginBottom = values[2]
		style.MarginLeft = values[3]
	case "margin-top":
		style.MarginTop = decl.Value
	case "margin-right":
		style.MarginRight = decl.Value
	case "margin-bottom":
		style.MarginBottom = decl.Value
	case "margin-left":
		style.MarginLeft = decl.Value

	// Padding properties
	case "padding":
		// Shorthand: apply to all sides
		values := parseBoxShorthand(decl.Value)
		style.PaddingTop = values[0]
		style.PaddingRight = values[1]
		style.PaddingBottom = values[2]
		style.PaddingLeft = values[3]
	case "padding-top":
		style.PaddingTop = decl.Value
	case "padding-right":
		style.PaddingRight = decl.Value
	case "padding-bottom":
		style.PaddingBottom = decl.Value
	case "padding-left":
		style.PaddingLeft = decl.Value

	// Border width properties
	case "border-width":
		// Shorthand: apply to all sides
		values := parseBoxShorthand(decl.Value)
		style.BorderTopWidth = values[0]
		style.BorderRightWidth = values[1]
		style.BorderBottomWidth = values[2]
		style.BorderLeftWidth = values[3]
	case "border-top-width":
		style.BorderTopWidth = decl.Value
	case "border-right-width":
		style.BorderRightWidth = decl.Value
	case "border-bottom-width":
		style.BorderBottomWidth = decl.Value
	case "border-left-width":
		style.BorderLeftWidth = decl.Value

	// Border style properties
	case "border-style":
		// Shorthand: apply to all sides
		values := parseBoxShorthand(decl.Value)
		style.BorderTopStyle = values[0]
		style.BorderRightStyle = values[1]
		style.BorderBottomStyle = values[2]
		style.BorderLeftStyle = values[3]
	case "border-top-style":
		style.BorderTopStyle = decl.Value
	case "border-right-style":
		style.BorderRightStyle = decl.Value
	case "border-bottom-style":
		style.BorderBottomStyle = decl.Value
	case "border-left-style":
		style.BorderLeftStyle = decl.Value

	// Border color properties
	case "border-color":
		// Shorthand: apply to all sides
		values := strings.Fields(decl.Value)
		colors := parseBoxShorthandColors(values)
		style.BorderTopColor = colors[0]
		style.BorderRightColor = colors[1]
		style.BorderBottomColor = colors[2]
		style.BorderLeftColor = colors[3]
	case "border-top-color":
		if val, err := parseColor(decl.Value); err == nil {
			style.BorderTopColor = val
		}
	case "border-right-color":
		if val, err := parseColor(decl.Value); err == nil {
			style.BorderRightColor = val
		}
	case "border-bottom-color":
		if val, err := parseColor(decl.Value); err == nil {
			style.BorderBottomColor = val
		}
	case "border-left-color":
		if val, err := parseColor(decl.Value); err == nil {
			style.BorderLeftColor = val
		}

	// Border shorthand properties
	case "border":
		// Parse "border: 1px solid black" format
		parseBorderShorthand(decl.Value, style, "all")
	case "border-top":
		parseBorderShorthand(decl.Value, style, "top")
	case "border-right":
		parseBorderShorthand(decl.Value, style, "right")
	case "border-bottom":
		parseBorderShorthand(decl.Value, style, "bottom")
	case "border-left":
		parseBorderShorthand(decl.Value, style, "left")

	// Flexbox container properties
	case "flex-direction":
		style.FlexDirection = decl.Value
	case "flex-wrap":
		style.FlexWrap = decl.Value
	case "flex-flow":
		// Shorthand for flex-direction and flex-wrap
		parts := strings.Fields(decl.Value)
		for _, part := range parts {
			switch part {
			case "row", "row-reverse", "column", "column-reverse":
				style.FlexDirection = part
			case "nowrap", "wrap", "wrap-reverse":
				style.FlexWrap = part
			}
		}
	case "justify-content":
		style.JustifyContent = decl.Value
	case "align-items":
		style.AlignItems = decl.Value
	case "align-content":
		style.AlignContent = decl.Value
	case "gap":
		style.Gap = decl.Value
	case "row-gap":
		style.RowGap = decl.Value
	case "column-gap":
		style.ColumnGap = decl.Value

	// Grid Container properties
	case "grid-template-columns":
		style.GridTemplateColumns = decl.Value
	case "grid-template-rows":
		style.GridTemplateRows = decl.Value

	// Grid Item properties
	case "grid-column-start":
		style.GridColumnStart = decl.Value
	case "grid-column-end":
		style.GridColumnEnd = decl.Value
	case "grid-row-start":
		style.GridRowStart = decl.Value
	case "grid-row-end":
		style.GridRowEnd = decl.Value
	case "grid-column":
		// Shorthand: start / end
		parts := strings.Split(decl.Value, "/")
		if len(parts) > 0 {
			style.GridColumnStart = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			style.GridColumnEnd = strings.TrimSpace(parts[1])
		}
	case "grid-row":
		// Shorthand: start / end
		parts := strings.Split(decl.Value, "/")
		if len(parts) > 0 {
			style.GridRowStart = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			style.GridRowEnd = strings.TrimSpace(parts[1])
		}

	// Positioning
	case "position":
		style.Position = css.PositionAtomFromString(decl.Value)
	case "z-index":
		if val, err := strconv.Atoi(decl.Value); err == nil {
			style.ZIndex = val
		} else if decl.Value == "auto" {
			style.ZIndex = 0 // auto usually means 0 (stacking context wise) or inherit
		}
	case "top":
		style.Top = decl.Value
	case "right":
		style.Right = decl.Value
	case "bottom":
		style.Bottom = decl.Value
	case "left":
		style.Left = decl.Value

	// Overflow
	case "overflow":
		style.Overflow = css.OverflowAtomFromString(decl.Value)
		if style.OverflowX == "" {
			style.OverflowX = decl.Value
		}
		if style.OverflowY == "" {
			style.OverflowY = decl.Value
		}
	case "overflow-x":
		style.OverflowX = decl.Value
	case "overflow-y":
		style.OverflowY = decl.Value
	case "text-overflow":
		style.TextOverflow = decl.Value

	// Float and Clear
	case "float":
		style.Float = css.FloatAtomFromString(decl.Value)
	case "clear":
		style.Clear = decl.Value

	// Box Sizing
	case "box-sizing":
		style.BoxSizing = decl.Value

	// Min/Max constraints
	case "min-width":
		style.MinWidth = decl.Value
	case "max-width":
		style.MaxWidth = decl.Value
	case "min-height":
		style.MinHeight = decl.Value
	case "max-height":
		style.MaxHeight = decl.Value

	// Visual and other properties
	case "border-radius":
		style.BorderRadius = decl.Value
	case "box-shadow":
		style.BoxShadow = decl.Value
	case "text-shadow":
		style.TextShadow = decl.Value
	case "transform":
		style.Transform = decl.Value
	case "transform-origin":
		style.TransformOrigin = decl.Value
	case "transition":
		style.Transition = decl.Value
	case "cursor":
		style.Cursor = decl.Value
	case "vertical-align":
		style.VerticalAlign = decl.Value
	case "white-space":
		style.WhiteSpace = css.WhiteSpaceAtomFromString(decl.Value)
	case "word-break":
		style.WordBreak = decl.Value
	case "font":
		parentFontSize := float32(16.0)
		if node.Parent != nil && node.Parent.ComputedStyle != nil && node.Parent.ComputedStyle.FontSize > 0 {
			parentFontSize = node.Parent.ComputedStyle.FontSize
		}
		parseFontShorthand(decl.Value, style, parentFontSize)
	case "list-style":
		parseListStyleShorthand(decl.Value, style)
	case "list-style-type":
		style.ListStyleType = css.ListStyleTypeAtomFromString(decl.Value)
	case "list-style-position":
		style.ListStylePosition = css.ListStylePositionAtomFromString(decl.Value)
	case "table-layout":
		style.TableLayout = decl.Value
	case "border-collapse":
		style.BorderCollapse = decl.Value
	case "border-spacing":
		style.BorderSpacing = decl.Value

	// Flexbox item properties
	case "flex-grow":
		if val, err := strconv.ParseFloat(decl.Value, 32); err == nil {
			style.FlexGrow = float32(val)
		}
	case "flex-shrink":
		if val, err := strconv.ParseFloat(decl.Value, 32); err == nil {
			style.FlexShrink = float32(val)
		}
	case "flex-basis":
		style.FlexBasis = decl.Value
	case "flex":
		// Shorthand: flex-grow [flex-shrink] [flex-basis]
		parseFlexShorthand(decl.Value, style)
	case "align-self":
		style.AlignSelf = decl.Value
	case "order":
		if val, err := strconv.Atoi(decl.Value); err == nil {
			style.Order = val
		}
	}
}

// parseFontShorthand parses CSS font shorthand:
// [ [ <'font-style'> || <'font-variant'> || <'font-weight'> ]? <'font-size'> [ / <'line-height'> ]? <'font-family'> ]
func parseFontShorthand(value string, style *Style, parentFontSize float32) {
	val := strings.TrimSpace(value)
	if val == "" || val == "inherit" || val == "initial" || val == "unset" {
		return
	}

	tokens := strings.Fields(val)
	if len(tokens) == 0 {
		return
	}

	sizeIdx := -1
	for i, tok := range tokens {
		if strings.Contains(tok, "/") {
			parts := strings.SplitN(tok, "/", 2)
			if _, err := parseFontSize(parts[0], parentFontSize); err == nil {
				sizeIdx = i
				break
			}
		} else {
			if _, err := parseFontSize(tok, parentFontSize); err == nil {
				isWeightKeyword := (tok == "100" || tok == "200" || tok == "300" || tok == "400" ||
					tok == "500" || tok == "600" || tok == "700" || tok == "800" || tok == "900")
				if isWeightKeyword && i+1 < len(tokens) {
					nextTok := tokens[i+1]
					nextSizePart := nextTok
					if strings.Contains(nextTok, "/") {
						nextSizePart = strings.SplitN(nextTok, "/", 2)[0]
					}
					if _, errNext := parseFontSize(nextSizePart, parentFontSize); errNext == nil {
						continue
					}
				}
				sizeIdx = i
				break
			}
		}
	}

	if sizeIdx == -1 {
		return
	}

	for i := 0; i < sizeIdx; i++ {
		tok := strings.ToLower(tokens[i])
		switch tok {
		case "italic", "oblique":
			style.FontStyle = css.FontStyleAtomItalic
		case "bold", "bolder", "lighter", "100", "200", "300", "400", "500", "600", "700", "800", "900":
			style.FontWeight = tok
		}
	}

	sizeToken := tokens[sizeIdx]
	if strings.Contains(sizeToken, "/") {
		parts := strings.SplitN(sizeToken, "/", 2)
		if sz, err := parseFontSize(parts[0], parentFontSize); err == nil {
			style.FontSize = sz
			style.LineHeight = parseLineHeight(parts[1], style.FontSize)
		}
	} else {
		if sz, err := parseFontSize(sizeToken, parentFontSize); err == nil {
			style.FontSize = sz
		}
	}

	if sizeIdx+1 < len(tokens) {
		pos := strings.Index(val, sizeToken)
		if pos != -1 {
			fam := strings.TrimSpace(val[pos+len(sizeToken):])
			if fam != "" {
				style.FontFamily = fam
			}
		}
	}
}

// parseListStyleShorthand parses CSS list-style shorthand:
// [ <'list-style-type'> || <'list-style-position'> || <'list-style-image'> ]
func parseListStyleShorthand(value string, style *Style) {
	val := strings.TrimSpace(value)
	if val == "" {
		return
	}
	tokens := strings.Fields(val)
	for _, tok := range tokens {
		tokLower := strings.ToLower(tok)
		switch tokLower {
		case "inside":
			style.ListStylePosition = css.ListStylePositionAtomInside
		case "outside":
			style.ListStylePosition = css.ListStylePositionAtomOutside
		case "none", "disc", "circle", "square", "decimal",
			"lower-roman", "upper-roman", "lower-alpha", "upper-alpha",
			"lower-latin", "upper-latin", "cjk-decimal", "armenian", "georgian":
			style.ListStyleType = css.ListStyleTypeAtomFromString(tokLower)
		}
	}
}

func parseFontSize(value string, parentFontSize float32) (float32, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return parentFontSize, nil
	}

	if isCalcExpr(value) {
		return evalCalcExpr(value, parentFontSize, 1280, 800, parentFontSize), nil
	}

	// Keywords
	switch value {
	case "xx-small":
		return 9, nil
	case "x-small":
		return 10, nil
	case "small":
		return 13, nil
	case "medium":
		return 16, nil
	case "large":
		return 18, nil
	case "x-large":
		return 24, nil
	case "xx-large":
		return 32, nil
	case "smaller":
		return parentFontSize / 1.2, nil
	case "larger":
		return parentFontSize * 1.2, nil
	case "inherit":
		return parentFontSize, nil
	}

	// IMPORTANT: check rem before em
	if strings.HasSuffix(value, "rem") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(value, "rem"), 32)
		if err != nil {
			return 0, err
		}
		return float32(val) * 16.0, nil
	}
	if strings.HasSuffix(value, "px") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 32)
		if err != nil {
			return 0, err
		}
		return float32(val), nil
	}
	if strings.HasSuffix(value, "em") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(value, "em"), 32)
		if err != nil {
			return 0, err
		}
		return float32(val) * parentFontSize, nil
	}
	if strings.HasSuffix(value, "%") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 32)
		if err != nil {
			return 0, err
		}
		return (float32(val) / 100.0) * parentFontSize, nil
	}
	if strings.HasSuffix(value, "pt") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(value, "pt"), 32)
		if err != nil {
			return 0, err
		}
		return float32(val) * 4.0 / 3.0, nil
	}

	// Try to parse as plain number
	if val, err := strconv.ParseFloat(value, 32); err == nil {
		return float32(val), nil
	}

	return 0, fmt.Errorf("unsupported font size unit: %s", value)
}

// parseLength parses a CSS length value and returns its numeric value in pixels
// Supports: px, em, rem, plain numbers (treated as px), and keyword values (thin, medium, thick)
func parseLineHeight(value string, fontSize float32) float32 {
	if value == "normal" || value == "" {
		return 0
	}

	// Try parsing as a unitless number (e.g., "1.5")
	if f, err := strconv.ParseFloat(value, 32); err == nil {
		return float32(f) * fontSize
	}

	// Try parsing as a length (px, em, etc.)
	return parseLength(value, fontSize)
}

// parseLengthWithViewport parses a CSS length value with support for vw/vh/% units.
// Returns -1 if the value is empty, "auto", or uses an unsupported unit (so callers
// can detect "unset" vs 0).
func parseLengthWithViewport(value string, fontSize, viewportWidth, viewportHeight, percentBase float32) float32 {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		return -1
	}
	if isCalcExpr(value) {
		return evalCalcExpr(value, fontSize, viewportWidth, viewportHeight, percentBase)
	}
	if value == "0" {
		return 0
	}
	if strings.HasSuffix(value, "vw") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "vw"), 32); err == nil {
			return float32(val) / 100.0 * viewportWidth
		}
	} else if strings.HasSuffix(value, "vh") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "vh"), 32); err == nil {
			return float32(val) / 100.0 * viewportHeight
		}
	} else if strings.HasSuffix(value, "%") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 32); err == nil {
			return float32(val) / 100.0 * percentBase
		}
	}
	v := parseLength(value, fontSize)
	if v == 0 && value != "0" {
		return -1 // unsupported unit, treat as unset
	}
	return v
}

func parseLength(value string, fontSize float32) float32 {
	value = strings.TrimSpace(value)

	// Handle empty or "0" values
	if value == "" || value == "0" {
		return 0
	}

	if isCalcExpr(value) {
		// For parseLength without viewport context, use defaults for vw/vh
		return evalCalcExpr(value, fontSize, 1280, 800, fontSize)
	}

	// Handle keyword values for border widths
	switch value {
	case "thin":
		return 1.0
	case "medium":
		return 3.0
	case "thick":
		return 5.0
	}

	// Parse numeric values with units
	// IMPORTANT: Check rem before em since "rem" ends with "em"
	// Otherwise "1.5rem" would be incorrectly parsed as "1.5r" + "em"
	if strings.HasSuffix(value, "rem") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "rem"), 32); err == nil {
			// rem is relative to root font size (typically 16px)
			return float32(val) * 16.0
		}
	} else if strings.HasSuffix(value, "px") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 32); err == nil {
			return float32(val)
		}
	} else if strings.HasSuffix(value, "em") {
		if val, err := strconv.ParseFloat(strings.TrimSuffix(value, "em"), 32); err == nil {
			return float32(val) * fontSize
		}
	} else {
		// Try to parse as plain number (treated as px)
		if val, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(val)
		}
	}

	return 0
}

func parseColor(value string) (color.Color, error) {
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	if lowerValue == "transparent" {
		return color.Transparent, nil
	}
	if lowerValue == "currentcolor" || lowerValue == "inherit" {
		// Cannot resolve these without context; treat as parse failure so caller inherits
		return nil, fmt.Errorf("contextual color: %s", value)
	}
	if hex, ok := colorNameToHex[lowerValue]; ok {
		return parseHexColor(hex)
	}
	if strings.HasPrefix(lowerValue, "#") {
		return parseHexColor(lowerValue)
	}
	if strings.HasPrefix(lowerValue, "rgb(") && strings.HasSuffix(lowerValue, ")") {
		return parseRgbColor(lowerValue[4 : len(lowerValue)-1])
	}
	if strings.HasPrefix(lowerValue, "rgba(") && strings.HasSuffix(lowerValue, ")") {
		return parseRgbColor(lowerValue[5 : len(lowerValue)-1])
	}
	if strings.HasPrefix(lowerValue, "hsl(") && strings.HasSuffix(lowerValue, ")") {
		return parseHslColor(lowerValue[4 : len(lowerValue)-1])
	}
	if strings.HasPrefix(lowerValue, "hsla(") && strings.HasSuffix(lowerValue, ")") {
		return parseHslColor(lowerValue[5 : len(lowerValue)-1])
	}
	if strings.HasPrefix(lowerValue, "oklch(") && strings.HasSuffix(lowerValue, ")") {
		return parseOklchColor(lowerValue[6 : len(lowerValue)-1])
	}
	if strings.HasPrefix(lowerValue, "oklab(") && strings.HasSuffix(lowerValue, ")") {
		return parseOklabColor(lowerValue[6 : len(lowerValue)-1])
	}
	return nil, fmt.Errorf("unsupported color format: %s", value)
}

func parseRgbColor(content string) (color.Color, error) {
	normalized := strings.ReplaceAll(content, ",", " ")
	normalized = strings.ReplaceAll(normalized, "/", " ")
	parts := strings.Fields(normalized)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid rgb format")
	}

	parseVal := func(s string, max float32) (float32, error) {
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "%") {
			val, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32)
			if err != nil {
				return 0, err
			}
			return float32(val) / 100.0 * max, nil
		}
		val, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return 0, err
		}
		return float32(val), nil
	}

	r, err1 := parseVal(parts[0], 255)
	g, err2 := parseVal(parts[1], 255)
	b, err3 := parseVal(parts[2], 255)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, fmt.Errorf("error parsing rgb values")
	}

	a := float32(1.0)
	if len(parts) >= 4 {
		val, err := parseVal(parts[3], 1.0)
		if err == nil {
			a = val
		}
	}

	return color.RGBA{
		R: uint8(clamp(r, 0, 255)),
		G: uint8(clamp(g, 0, 255)),
		B: uint8(clamp(b, 0, 255)),
		A: uint8(clamp(a*255, 0, 255)),
	}, nil
}

func parseHslColor(content string) (color.Color, error) {
	normalized := strings.ReplaceAll(content, ",", " ")
	normalized = strings.ReplaceAll(normalized, "/", " ")
	parts := strings.Fields(normalized)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid hsl format")
	}

	parseVal := func(s string, max float32) (float32, error) {
		s = strings.TrimSpace(s)
		if strings.HasSuffix(s, "%") {
			val, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 32)
			if err != nil {
				return 0, err
			}
			return float32(val) / 100.0 * max, nil
		}
		if strings.HasSuffix(s, "deg") {
			s = strings.TrimSuffix(s, "deg")
		}
		val, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return 0, err
		}
		return float32(val), nil
	}

	h, err1 := parseVal(parts[0], 360)
	s, err2 := parseVal(parts[1], 1.0)
	l, err3 := parseVal(parts[2], 1.0)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, fmt.Errorf("error parsing hsl values")
	}

	// Normalize Hue to [0, 360)
	h = float32(int(h)%360 + 360)
	h = float32(int(h) % 360)

	a := float32(1.0)
	if len(parts) >= 4 {
		val, err := parseVal(parts[3], 1.0)
		if err == nil {
			a = val
		}
	}

	r, g, b := hslToRgb(h/360, s, l)

	return color.RGBA{
		R: uint8(clamp(r*255, 0, 255)),
		G: uint8(clamp(g*255, 0, 255)),
		B: uint8(clamp(b*255, 0, 255)),
		A: uint8(clamp(a*255, 0, 255)),
	}, nil
}

func hslToRgb(h, s, l float32) (r, g, b float32) {
	if s == 0 {
		return l, l, l
	}

	var hue2rgb func(p, q, t float32) float32
	hue2rgb = func(p, q, t float32) float32 {
		if t < 0 {
			t += 1
		}
		if t > 1 {
			t -= 1
		}
		if t < 1.0/6.0 {
			return p + (q-p)*6*t
		}
		if t < 1.0/2.0 {
			return q
		}
		if t < 2.0/3.0 {
			return p + (q-p)*(2.0/3.0-t)*6
		}
		return p
	}

	var q float32
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	r = hue2rgb(p, q, h+1.0/3.0)
	g = hue2rgb(p, q, h)
	b = hue2rgb(p, q, h-1.0/3.0)

	return r, g, b
}

// parseOklchColor parses oklch(L C H [/ alpha]) where L∈[0,1], C≥0, H in degrees.
// It converts via: oklch → oklab → linear-sRGB → gamma-corrected sRGB.
func parseOklchColor(content string) (color.Color, error) {
	// Normalize: replace "/" with space for alpha separator
	normalized := strings.ReplaceAll(content, "/", " ")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	parts := strings.Fields(normalized)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid oklch format")
	}

	parseF := func(s string) (float32, error) {
		s = strings.TrimSuffix(strings.TrimSpace(s), "deg")
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err
	}

	l, err := parseF(parts[0])
	if err != nil {
		return nil, err
	}
	c, err := parseF(parts[1])
	if err != nil {
		return nil, err
	}
	h, err := parseF(parts[2])
	if err != nil {
		return nil, err
	}

	alpha := float32(1.0)
	if len(parts) >= 4 {
		if v, err := parseF(parts[3]); err == nil {
			alpha = v
		}
	}

	// oklch → oklab
	hRad := h * 3.14159265358979 / 180.0
	a := c * float32(math.Cos(float64(hRad)))
	b := c * float32(math.Sin(float64(hRad)))

	r, g, bv := oklabToSrgb(l, a, b)

	return color.RGBA{
		R: f32ToByte(r),
		G: f32ToByte(g),
		B: f32ToByte(bv),
		A: f32ToByte(alpha),
	}, nil
}

// parseOklabColor parses oklab(L a b [/ alpha]).
func parseOklabColor(content string) (color.Color, error) {
	normalized := strings.ReplaceAll(content, "/", " ")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	parts := strings.Fields(normalized)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid oklab format")
	}

	parseF := func(s string) (float32, error) {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 32)
		return float32(v), err
	}

	l, err := parseF(parts[0])
	if err != nil {
		return nil, err
	}
	a, err := parseF(parts[1])
	if err != nil {
		return nil, err
	}
	b, err := parseF(parts[2])
	if err != nil {
		return nil, err
	}

	alpha := float32(1.0)
	if len(parts) >= 4 {
		if v, err := parseF(parts[3]); err == nil {
			alpha = v
		}
	}

	r, g, bv := oklabToSrgb(l, a, b)

	return color.RGBA{
		R: f32ToByte(r),
		G: f32ToByte(g),
		B: f32ToByte(bv),
		A: f32ToByte(alpha),
	}, nil
}

// oklabToSrgb converts OKLab (L, a, b) to gamma-corrected sRGB [0, 1].
func oklabToSrgb(l, a, b float32) (r, g, bOut float32) {
	// OKLab → LMS (cube roots of LMS cone responses)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b

	// Cube to get LMS
	lv := l_ * l_ * l_
	mv := m_ * m_ * m_
	sv := s_ * s_ * s_

	// LMS → linear sRGB (M^-1)
	lr := +4.0767416621*lv - 3.3077115913*mv + 0.2309699292*sv
	lg := -1.2684380046*lv + 2.6097574011*mv - 0.3413193965*sv
	lb := -0.0041960863*lv - 0.7034186147*mv + 1.7076147010*sv

	// Linear sRGB → gamma-corrected sRGB
	gammaCorrect := func(c float32) float32 {
		if c <= 0.0031308 {
			return 12.92 * c
		}
		return float32(1.055*math.Pow(float64(c), 1.0/2.4) - 0.055)
	}

	r = gammaCorrect(lr)
	g = gammaCorrect(lg)
	bOut = gammaCorrect(lb)
	return
}

func clamp(val, minVal, maxVal float32) float32 {
	return min(max(val, minVal), maxVal)
}

// f32ToByte converts a normalized float in [0,1] to a byte channel, rounding
// to the nearest integer like browsers do (rather than truncating).
func f32ToByte(val float32) uint8 {
	return uint8(clamp(float32(math.Round(float64(val*255))), 0, 255))
}

func parseHexColor(hex string) (color.Color, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return nil, fmt.Errorf("invalid hex color length")
	}
	rgb, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return nil, err
	}
	return color.RGBA{
		R: uint8(rgb >> 16),
		G: uint8(rgb >> 8),
		B: uint8(rgb),
		A: 255,
	}, nil
}

// parseBoxShorthand parses CSS box model shorthand values
// Returns [top, right, bottom, left] values
// Supports: 1 value (all), 2 values (vertical horizontal), 3 values (top horizontal bottom), 4 values (top right bottom left)
func parseBoxShorthand(value string) [4]string {
	values := strings.Fields(value)
	var result [4]string

	switch len(values) {
	case 1:
		// All sides same
		result[0] = values[0]
		result[1] = values[0]
		result[2] = values[0]
		result[3] = values[0]
	case 2:
		// Vertical horizontal
		result[0] = values[0] // top
		result[1] = values[1] // right
		result[2] = values[0] // bottom
		result[3] = values[1] // left
	case 3:
		// Top horizontal bottom
		result[0] = values[0] // top
		result[1] = values[1] // right
		result[2] = values[2] // bottom
		result[3] = values[1] // left
	case 4:
		// Top right bottom left
		result[0] = values[0]
		result[1] = values[1]
		result[2] = values[2]
		result[3] = values[3]
	default:
		// Invalid, return zeros
		result[0] = "0"
		result[1] = "0"
		result[2] = "0"
		result[3] = "0"
	}

	return result
}

// parseBoxShorthandColors parses color values for box model shorthand
func parseBoxShorthandColors(values []string) [4]color.Color {
	var defaultColor color.Color // nil
	var result [4]color.Color

	switch len(values) {
	case 1:
		// All sides same
		if c, err := parseColor(values[0]); err == nil {
			result[0] = c
			result[1] = c
			result[2] = c
			result[3] = c
		} else {
			result[0] = defaultColor
			result[1] = defaultColor
			result[2] = defaultColor
			result[3] = defaultColor
		}
	case 2:
		// Vertical horizontal
		c0, err0 := parseColor(values[0])
		c1, err1 := parseColor(values[1])
		if err0 == nil {
			result[0] = c0
			result[2] = c0
		} else {
			result[0] = defaultColor
			result[2] = defaultColor
		}
		if err1 == nil {
			result[1] = c1
			result[3] = c1
		} else {
			result[1] = defaultColor
			result[3] = defaultColor
		}
	case 3:
		// Top horizontal bottom
		for i := 0; i < 3; i++ {
			if c, err := parseColor(values[i]); err == nil {
				result[i] = c
			} else {
				result[i] = defaultColor
			}
		}
		// left = horizontal
		if c, err := parseColor(values[1]); err == nil {
			result[3] = c
		} else {
			result[3] = defaultColor
		}
	case 4:
		// Top right bottom left
		for i := 0; i < 4; i++ {
			if c, err := parseColor(values[i]); err == nil {
				result[i] = c
			} else {
				result[i] = defaultColor
			}
		}
	default:
		// Invalid, return default
		result[0] = defaultColor
		result[1] = defaultColor
		result[2] = defaultColor
		result[3] = defaultColor
	}

	return result
}

// parseBorderShorthand parses the border shorthand property
// Format: "width style color" in any order
func parseBorderShorthand(value string, style *Style, side string) {
	parts := strings.Fields(value)

	var width, borderStyle, borderColor string

	// Parse each part
	for _, part := range parts {
		// Check if it's a width (has px, em, etc. or is a number)
		if strings.HasSuffix(part, "px") || strings.HasSuffix(part, "em") ||
			strings.HasSuffix(part, "rem") || part == "thin" || part == "medium" || part == "thick" {
			width = part
		} else if isBorderStyle(part) {
			borderStyle = part
		} else {
			// Assume it's a color
			borderColor = part
		}
	}

	// Apply to the specified side(s)
	switch side {
	case "all":
		if width != "" {
			style.BorderTopWidth = width
			style.BorderRightWidth = width
			style.BorderBottomWidth = width
			style.BorderLeftWidth = width
		}
		if borderStyle != "" {
			style.BorderTopStyle = borderStyle
			style.BorderRightStyle = borderStyle
			style.BorderBottomStyle = borderStyle
			style.BorderLeftStyle = borderStyle
		}
		if borderColor != "" {
			if c, err := parseColor(borderColor); err == nil {
				style.BorderTopColor = c
				style.BorderRightColor = c
				style.BorderBottomColor = c
				style.BorderLeftColor = c
			}
		}
	case "top":
		if width != "" {
			style.BorderTopWidth = width
		}
		if borderStyle != "" {
			style.BorderTopStyle = borderStyle
		}
		if borderColor != "" {
			if c, err := parseColor(borderColor); err == nil {
				style.BorderTopColor = c
			}
		}
	case "right":
		if width != "" {
			style.BorderRightWidth = width
		}
		if borderStyle != "" {
			style.BorderRightStyle = borderStyle
		}
		if borderColor != "" {
			if c, err := parseColor(borderColor); err == nil {
				style.BorderRightColor = c
			}
		}
	case "bottom":
		if width != "" {
			style.BorderBottomWidth = width
		}
		if borderStyle != "" {
			style.BorderBottomStyle = borderStyle
		}
		if borderColor != "" {
			if c, err := parseColor(borderColor); err == nil {
				style.BorderBottomColor = c
			}
		}
	case "left":
		if width != "" {
			style.BorderLeftWidth = width
		}
		if borderStyle != "" {
			style.BorderLeftStyle = borderStyle
		}
		if borderColor != "" {
			if c, err := parseColor(borderColor); err == nil {
				style.BorderLeftColor = c
			}
		}
	}
}

// isBorderStyle checks if a string is a valid border style
func isBorderStyle(s string) bool {
	styles := []string{"none", "hidden", "dotted", "dashed", "solid", "double", "groove", "ridge", "inset", "outset"}
	for _, style := range styles {
		if s == style {
			return true
		}
	}
	return false
}

// parseFlexShorthand parses the flex shorthand property
// flex: none | [ <flex-grow> <flex-shrink>? || <flex-basis> ]
// Examples: "1", "1 1", "1 1 auto", "0 0 auto", "none", "auto"
func parseFlexShorthand(value string, style *Style) {
	value = strings.TrimSpace(value)

	// Handle special values
	switch value {
	case "none":
		style.FlexGrow = 0
		style.FlexShrink = 0
		style.FlexBasis = "auto"
		return
	case "auto":
		style.FlexGrow = 1
		style.FlexShrink = 1
		style.FlexBasis = "auto"
		return
	case "initial":
		style.FlexGrow = 0
		style.FlexShrink = 1
		style.FlexBasis = "auto"
		return
	}

	parts := strings.Fields(value)
	if len(parts) == 0 {
		return
	}

	// First value is always flex-grow
	if val, err := strconv.ParseFloat(parts[0], 32); err == nil {
		style.FlexGrow = float32(val)
	}

	if len(parts) >= 2 {
		// Second value could be flex-shrink or flex-basis
		if val, err := strconv.ParseFloat(parts[1], 32); err == nil {
			style.FlexShrink = float32(val)
		} else {
			// It's a flex-basis
			style.FlexBasis = parts[1]
		}
	}

	if len(parts) >= 3 {
		// Third value is flex-basis
		style.FlexBasis = parts[2]
	}
}

func applyBackgroundShorthand(style *Style, value string) {
	var tokens []string
	var current strings.Builder
	inParens := 0
	for _, r := range value {
		if r == '(' {
			inParens++
			current.WriteRune(r)
		} else if r == ')' {
			if inParens > 0 {
				inParens--
			}
			current.WriteRune(r)
		} else if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if inParens > 0 {
				current.WriteRune(r)
			} else {
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	var positionTokens []string
	for _, tok := range tokens {
		tokLower := strings.ToLower(tok)
		if strings.HasPrefix(tokLower, "url(") || tokLower == "none" {
			style.BackgroundImage = tok
		} else if tokLower == "repeat" || tokLower == "no-repeat" || tokLower == "repeat-x" || tokLower == "repeat-y" || tokLower == "space" || tokLower == "round" {
			style.BackgroundRepeat = css.BackgroundRepeatAtomFromString(tokLower)
		} else if tokLower == "fixed" || tokLower == "scroll" || tokLower == "local" {
			style.BackgroundAttachment = css.BackgroundAttachmentAtomFromString(tokLower)
		} else if tokLower == "cover" || tokLower == "contain" {
			style.BackgroundSize = css.BackgroundSizeAtomFromString(tokLower)
		} else if tokLower == "top" || tokLower == "bottom" || tokLower == "left" || tokLower == "right" || tokLower == "center" {
			positionTokens = append(positionTokens, tokLower)
		} else if col, err := parseColor(tok); err == nil {
			style.BackgroundColor = col
		}
	}
	if len(positionTokens) > 0 {
		style.BackgroundPosition = css.BackgroundPositionAtomFromString(strings.Join(positionTokens, " "))
	}
}

func parseBackgroundShorthandColor(value string) (color.Color, bool) {
	var temp Style
	applyBackgroundShorthand(&temp, value)
	if temp.BackgroundColor != nil {
		return temp.BackgroundColor, true
	}
	return nil, false
}

// MatchRules returns all rules in the style manager's stylesheets that match the given node,
// sorted by specificity (highest specificity first).
func (sm *StyleManager) MatchRules(node *RenderNode) []css.Rule {
	var matched []css.Rule
	if sm.stylesheet != nil {
		for _, rule := range sm.stylesheet.Rules {
			matches := false
			for _, seq := range rule.Selectors {
				if sm.matchesSequence(seq, node) {
					matches = true
					break
				}
			}
			if matches {
				matched = append(matched, rule)
			}
		}
	}
	if sm.defaultStylesheet != nil {
		for _, rule := range sm.defaultStylesheet.Rules {
			matches := false
			for _, seq := range rule.Selectors {
				if sm.matchesSequence(seq, node) {
					matches = true
					break
				}
			}
			if matches {
				matched = append(matched, rule)
			}
		}
	}

	// Sort by specificity (descending)
	sort.Slice(matched, func(i, j int) bool {
		cmp := css.CompareSpecificity(matched[i].Specificity, matched[j].Specificity)
		if cmp != 0 {
			return cmp > 0
		}
		// If specificity is equal, preserve source order (higher source order first)
		return matched[i].SourceOrder > matched[j].SourceOrder
	})
	return matched
}

// --- Fingerprint and StylePool for computed style deduplication ---

// Fingerprint returns a uint64 hash of the Style for deduplication.
// Two Style values with identical fields produce the same fingerprint.
func (s *Style) Fingerprint() uint64 {
	h := fnv.New64a()

	// Write enum/atom fields packed into bytes
	h.Write([]byte{byte(s.Display), byte(s.Position), byte(s.Float), byte(s.TextAlign)})
	h.Write([]byte{byte(s.Visibility), byte(s.FontStyle), byte(s.TextDecoration), byte(s.TextTransform)})
	h.Write([]byte{byte(s.WhiteSpace), byte(s.BackgroundRepeat), byte(s.BackgroundPosition)})
	h.Write([]byte{byte(s.BackgroundSize), byte(s.BackgroundAttachment)})
	h.Write([]byte{byte(s.ListStyleType), byte(s.ListStylePosition)})

	// Write float fields as their bit representation
	var fbuf [8]byte
	rendererPutFloat32(fbuf[:4], s.FontSize)
	rendererPutFloat32(fbuf[4:], s.LineHeight)
	h.Write(fbuf[:8])

	rendererPutFloat32(fbuf[:4], s.Opacity)
	rendererPutFloat32(fbuf[4:], s.LetterSpacing)
	h.Write(fbuf[:8])

	rendererPutFloat32(fbuf[:4], s.FlexGrow)
	rendererPutFloat32(fbuf[4:], s.FlexShrink)
	h.Write(fbuf[:8])

	// Write int fields
	var ibuf [8]byte
	ibuf[0] = byte(s.ZIndex)
	ibuf[1] = byte(s.ZIndex >> 8)
	ibuf[2] = byte(s.ZIndex >> 16)
	ibuf[3] = byte(s.ZIndex >> 24)
	ibuf[4] = byte(s.Order)
	ibuf[5] = byte(s.Order >> 8)
	ibuf[6] = byte(s.Order >> 16)
	ibuf[7] = byte(s.Order >> 24)
	h.Write(ibuf[:8])

	// Write color fields (Color, BackgroundColor, and 4 border colors)
	writeColorToHash(h, s.Color)
	writeColorToHash(h, s.BackgroundColor)
	writeColorToHash(h, s.BorderTopColor)
	writeColorToHash(h, s.BorderRightColor)
	writeColorToHash(h, s.BorderBottomColor)
	writeColorToHash(h, s.BorderLeftColor)

	// Write string fields with null separators
	h.Write([]byte(s.FontFamily))
	h.Write([]byte{0})
	h.Write([]byte(s.BackgroundImage))
	h.Write([]byte{0})
	h.Write([]byte(s.FontWeight))
	h.Write([]byte{0})
	h.Write([]byte(s.Width))
	h.Write([]byte{0})
	h.Write([]byte(s.Height))
	h.Write([]byte{0})

	// Positioning strings
	h.Write([]byte(s.Top))
	h.Write([]byte{0})
	h.Write([]byte(s.Right))
	h.Write([]byte{0})
	h.Write([]byte(s.Bottom))
	h.Write([]byte{0})
	h.Write([]byte(s.Left))
	h.Write([]byte{0})
	h.Write([]byte(s.Clear))
	h.Write([]byte{0})

	// Overflow strings
	h.Write([]byte(s.OverflowX))
	h.Write([]byte{0})
	h.Write([]byte(s.OverflowY))
	h.Write([]byte{0})
	h.Write([]byte(s.TextOverflow))
	h.Write([]byte{0})
	h.Write([]byte(s.BoxSizing))
	h.Write([]byte{0})

	// Min/Max constraints
	h.Write([]byte(s.MinWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.MaxWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.MinHeight))
	h.Write([]byte{0})
	h.Write([]byte(s.MaxHeight))
	h.Write([]byte{0})

	// Margin
	h.Write([]byte(s.MarginTop))
	h.Write([]byte{0})
	h.Write([]byte(s.MarginRight))
	h.Write([]byte{0})
	h.Write([]byte(s.MarginBottom))
	h.Write([]byte{0})
	h.Write([]byte(s.MarginLeft))
	h.Write([]byte{0})

	// Padding
	h.Write([]byte(s.PaddingTop))
	h.Write([]byte{0})
	h.Write([]byte(s.PaddingRight))
	h.Write([]byte{0})
	h.Write([]byte(s.PaddingBottom))
	h.Write([]byte{0})
	h.Write([]byte(s.PaddingLeft))
	h.Write([]byte{0})

	// Border widths and styles
	h.Write([]byte(s.BorderTopWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderRightWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderBottomWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderLeftWidth))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderTopStyle))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderRightStyle))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderBottomStyle))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderLeftStyle))
	h.Write([]byte{0})

	// Flexbox
	h.Write([]byte(s.FlexDirection))
	h.Write([]byte{0})
	h.Write([]byte(s.FlexWrap))
	h.Write([]byte{0})
	h.Write([]byte(s.JustifyContent))
	h.Write([]byte{0})
	h.Write([]byte(s.AlignItems))
	h.Write([]byte{0})
	h.Write([]byte(s.AlignContent))
	h.Write([]byte{0})
	h.Write([]byte(s.Gap))
	h.Write([]byte{0})
	h.Write([]byte(s.RowGap))
	h.Write([]byte{0})
	h.Write([]byte(s.ColumnGap))
	h.Write([]byte{0})
	h.Write([]byte(s.FlexBasis))
	h.Write([]byte{0})
	h.Write([]byte(s.AlignSelf))
	h.Write([]byte{0})

	// Grid
	h.Write([]byte(s.GridTemplateColumns))
	h.Write([]byte{0})
	h.Write([]byte(s.GridTemplateRows))
	h.Write([]byte{0})
	h.Write([]byte(s.GridColumnStart))
	h.Write([]byte{0})
	h.Write([]byte(s.GridColumnEnd))
	h.Write([]byte{0})
	h.Write([]byte(s.GridRowStart))
	h.Write([]byte{0})
	h.Write([]byte(s.GridRowEnd))
	h.Write([]byte{0})

	// Visual properties
	h.Write([]byte(s.BorderRadius))
	h.Write([]byte{0})
	h.Write([]byte(s.BoxShadow))
	h.Write([]byte{0})
	h.Write([]byte(s.TextShadow))
	h.Write([]byte{0})
	h.Write([]byte(s.Transform))
	h.Write([]byte{0})
	h.Write([]byte(s.TransformOrigin))
	h.Write([]byte{0})
	h.Write([]byte(s.Transition))
	h.Write([]byte{0})
	h.Write([]byte(s.Cursor))
	h.Write([]byte{0})
	h.Write([]byte(s.VerticalAlign))
	h.Write([]byte{0})
	h.Write([]byte(s.WordBreak))
	h.Write([]byte{0})
	h.Write([]byte(s.TableLayout))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderCollapse))
	h.Write([]byte{0})
	h.Write([]byte(s.BorderSpacing))
	h.Write([]byte{0})

	return h.Sum64()
}

// writeColorToHash writes a color.Color to a hash for fingerprinting.
func writeColorToHash(h hash.Hash64, c color.Color) {
	if c == nil {
		h.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
		return
	}
	r, g, b, a := c.RGBA()
	var buf [8]byte
	buf[0] = byte(r >> 8)
	buf[1] = byte(r)
	buf[2] = byte(g >> 8)
	buf[3] = byte(g)
	buf[4] = byte(b >> 8)
	buf[5] = byte(b)
	buf[6] = byte(a >> 8)
	buf[7] = byte(a)
	h.Write(buf[:8])
}

// rendererPutFloat32 writes float32 bits to a byte slice.
func rendererPutFloat32(b []byte, f float32) {
	bits := math.Float32bits(f)
	b[0] = byte(bits >> 24)
	b[1] = byte(bits >> 16)
	b[2] = byte(bits >> 8)
	b[3] = byte(bits)
}

// rendererStylePoolEntry is one entry in the renderer style pool.
type rendererStylePoolEntry struct {
	fp    uint64
	style Style
}

// rendererStylePool is a cache for deduplicating identical renderer.Style values.
// When an identical style is interned, the existing pointer is returned,
// reducing memory allocations for repeated computed styles.
//
// Safe for concurrent use.
type rendererStylePool struct {
	mu     sync.Mutex
	styles map[uint64]*rendererStylePoolEntry
}

// globalStylePool is the package-level style pool used by ApplyStyles.
var globalStylePool = &rendererStylePool{
	styles: make(map[uint64]*rendererStylePoolEntry),
}

// Intern returns a pointer to the deduplicated Style.
// If an identical style already exists in the pool (same fingerprint and fields),
// it returns the existing pointer. Otherwise, it inserts the new style.
func (p *rendererStylePool) Intern(s *Style) *Style {
	if s == nil {
		return nil
	}

	fp := s.Fingerprint()

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.styles[fp]; ok {
		// Verify actual equality to handle fingerprint collisions.
		if styleEqual(&entry.style, s) {
			return &entry.style
		}
		// Fingerprint collision with different content — overwrite.
		// Collisions are extremely rare with FNV-1a 64-bit.
	}

	// Insert new entry.
	entry := &rendererStylePoolEntry{
		fp:    fp,
		style: *s,
	}
	p.styles[fp] = entry
	return &entry.style
}

// Reset clears the pool. Called at the start of each render pass.
func (p *rendererStylePool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.styles = make(map[uint64]*rendererStylePoolEntry)
}

// styleEqual reports whether two Style values are field-by-field identical.
func styleEqual(a, b *Style) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare atom fields
	if a.Display != b.Display || a.Position != b.Position || a.Float != b.Float ||
		a.TextAlign != b.TextAlign || a.Visibility != b.Visibility ||
		a.FontStyle != b.FontStyle || a.TextDecoration != b.TextDecoration ||
		a.TextTransform != b.TextTransform || a.WhiteSpace != b.WhiteSpace ||
		a.BackgroundRepeat != b.BackgroundRepeat || a.BackgroundPosition != b.BackgroundPosition ||
		a.BackgroundSize != b.BackgroundSize || a.BackgroundAttachment != b.BackgroundAttachment ||
		a.ListStyleType != b.ListStyleType || a.ListStylePosition != b.ListStylePosition {
		return false
	}

	// Compare float fields
	if a.FontSize != b.FontSize || a.LineHeight != b.LineHeight ||
		a.Opacity != b.Opacity || a.LetterSpacing != b.LetterSpacing ||
		a.FlexGrow != b.FlexGrow || a.FlexShrink != b.FlexShrink {
		return false
	}

	// Compare int fields
	if a.ZIndex != b.ZIndex || a.Order != b.Order {
		return false
	}

	// Compare color fields
	if !rendererColorsEqual(a.Color, b.Color) ||
		!rendererColorsEqual(a.BackgroundColor, b.BackgroundColor) ||
		!rendererColorsEqual(a.BorderTopColor, b.BorderTopColor) ||
		!rendererColorsEqual(a.BorderRightColor, b.BorderRightColor) ||
		!rendererColorsEqual(a.BorderBottomColor, b.BorderBottomColor) ||
		!rendererColorsEqual(a.BorderLeftColor, b.BorderLeftColor) {
		return false
	}

	// Compare string fields
	return a.FontFamily == b.FontFamily &&
		a.BackgroundImage == b.BackgroundImage &&
		a.FontWeight == b.FontWeight &&
		a.Width == b.Width &&
		a.Height == b.Height &&
		a.Top == b.Top &&
		a.Right == b.Right &&
		a.Bottom == b.Bottom &&
		a.Left == b.Left &&
		a.Clear == b.Clear &&
		a.OverflowX == b.OverflowX &&
		a.OverflowY == b.OverflowY &&
		a.TextOverflow == b.TextOverflow &&
		a.BoxSizing == b.BoxSizing &&
		a.MinWidth == b.MinWidth &&
		a.MaxWidth == b.MaxWidth &&
		a.MinHeight == b.MinHeight &&
		a.MaxHeight == b.MaxHeight &&
		a.MarginTop == b.MarginTop &&
		a.MarginRight == b.MarginRight &&
		a.MarginBottom == b.MarginBottom &&
		a.MarginLeft == b.MarginLeft &&
		a.PaddingTop == b.PaddingTop &&
		a.PaddingRight == b.PaddingRight &&
		a.PaddingBottom == b.PaddingBottom &&
		a.PaddingLeft == b.PaddingLeft &&
		a.BorderTopWidth == b.BorderTopWidth &&
		a.BorderRightWidth == b.BorderRightWidth &&
		a.BorderBottomWidth == b.BorderBottomWidth &&
		a.BorderLeftWidth == b.BorderLeftWidth &&
		a.BorderTopStyle == b.BorderTopStyle &&
		a.BorderRightStyle == b.BorderRightStyle &&
		a.BorderBottomStyle == b.BorderBottomStyle &&
		a.BorderLeftStyle == b.BorderLeftStyle &&
		a.FlexDirection == b.FlexDirection &&
		a.FlexWrap == b.FlexWrap &&
		a.JustifyContent == b.JustifyContent &&
		a.AlignItems == b.AlignItems &&
		a.AlignContent == b.AlignContent &&
		a.Gap == b.Gap &&
		a.RowGap == b.RowGap &&
		a.ColumnGap == b.ColumnGap &&
		a.FlexBasis == b.FlexBasis &&
		a.AlignSelf == b.AlignSelf &&
		a.GridTemplateColumns == b.GridTemplateColumns &&
		a.GridTemplateRows == b.GridTemplateRows &&
		a.GridColumnStart == b.GridColumnStart &&
		a.GridColumnEnd == b.GridColumnEnd &&
		a.GridRowStart == b.GridRowStart &&
		a.GridRowEnd == b.GridRowEnd &&
		a.BorderRadius == b.BorderRadius &&
		a.BoxShadow == b.BoxShadow &&
		a.TextShadow == b.TextShadow &&
		a.Transform == b.Transform &&
		a.TransformOrigin == b.TransformOrigin &&
		a.Transition == b.Transition &&
		a.Cursor == b.Cursor &&
		a.VerticalAlign == b.VerticalAlign &&
		a.WordBreak == b.WordBreak &&
		a.TableLayout == b.TableLayout &&
		a.BorderCollapse == b.BorderCollapse &&
		a.BorderSpacing == b.BorderSpacing
}

// rendererColorsEqual compares two color.Color values for equality.
func rendererColorsEqual(a, b color.Color) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

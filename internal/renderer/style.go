package renderer

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

// StyleManager applies styles from a stylesheet to a render tree.
type StyleManager struct {
	stylesheet     *css.StyleSheet
	viewportWidth  float32
	viewportHeight float32
	mediaEvaluator *MediaQueryEvaluator
}

// NewStyleManager creates a new StyleManager.
func NewStyleManager(stylesheet *css.StyleSheet) *StyleManager {
	return &StyleManager{stylesheet: stylesheet}
}

// NewStyleManagerWithViewport creates a StyleManager with viewport dimensions for media query support.
func NewStyleManagerWithViewport(stylesheet *css.StyleSheet, viewportWidth, viewportHeight float32) *StyleManager {
	return &StyleManager{
		stylesheet:     stylesheet,
		viewportWidth:  viewportWidth,
		viewportHeight: viewportHeight,
		mediaEvaluator: NewMediaQueryEvaluator(viewportWidth, viewportHeight),
	}
}

// SetViewport updates the viewport dimensions for media query evaluation.
func (sm *StyleManager) SetViewport(width, height float32) {
	sm.viewportWidth = width
	sm.viewportHeight = height
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

	if node.ComputedStyle == nil {
		node.ComputedStyle = &Style{}
	}

	// Inherit styles from parent
	if node.Parent != nil && node.Parent.ComputedStyle != nil {
		node.ComputedStyle.Color = node.Parent.ComputedStyle.Color
		node.ComputedStyle.FontSize = node.Parent.ComputedStyle.FontSize
	}

	sm.applyMatchingRules(node)
	
	// Apply @media rules if viewport is set
	if sm.mediaEvaluator != nil {
		sm.applyMediaRules(node)
	}

	for _, child := range node.Children {
		sm.ApplyStyles(child)
	}
}

// applyMediaRules applies styles from @media rules that match current viewport.
func (sm *StyleManager) applyMediaRules(node *RenderNode) {
	if sm.stylesheet == nil || sm.mediaEvaluator == nil {
		return
	}
	
	for _, atRule := range sm.stylesheet.AtRules {
		if atRule.Name == "media" {
			if sm.mediaEvaluator.Evaluate(atRule.Prelude) {
				// Apply rules from this @media block
				for _, rule := range atRule.Rules {
					for _, selectorSeq := range rule.Selectors {
						if sm.matchesSequence(selectorSeq, node) {
							for _, decl := range rule.Declarations {
								sm.applyDeclaration(node, decl)
							}
						}
					}
				}
			}
		}
	}
}

func (sm *StyleManager) applyMatchingRules(node *RenderNode) {
	for _, rule := range sm.stylesheet.Rules {
		for _, selectorSeq := range rule.Selectors {
			if sm.matchesSequence(selectorSeq, node) {
				for _, decl := range rule.Declarations {
					sm.applyDeclaration(node, decl)
				}
			}
		}
	}
}

// matchesSequence checks if a selector sequence matches a node
// Note: Selectors are stored left-to-right (e.g., "div > p" stored as div->p)
// but we match right-to-left for efficiency (first check if node matches p, then check parent matches div)
func (sm *StyleManager) matchesSequence(seq css.SelectorSequence, node *RenderNode) bool {
	return sm.matchesFromRight(&seq, node)
}

// matchesFromRight recursively matches selectors from right to left
func (sm *StyleManager) matchesFromRight(seq *css.SelectorSequence, node *RenderNode) bool {
	// Find the rightmost selector in the chain
	if seq.Next == nil {
		// This is the rightmost selector, match it against the node
		return sm.matchesSimple(seq.Simple, node)
	}
	
	// This is not the rightmost, so we need to match the rightmost first
	// and then check if this one matches the appropriate ancestor/sibling
	return sm.matchesWithCombinatorLeftToRight(seq, node)
}

// matchesWithCombinatorLeftToRight handles combinator matching for left-to-right sequences
func (sm *StyleManager) matchesWithCombinatorLeftToRight(seq *css.SelectorSequence, node *RenderNode) bool {
	// seq = A (combinator) B
	// We need to check if B matches the node, then verify A matches the related element
	
	// First, recursively match the right side
	if !sm.matchesFromRight(seq.Next, node) {
		return false
	}
	
	// Now check if the left side (seq.Simple) matches according to the combinator
	switch seq.Combinator {
	case " ": // Descendant combinator: A B means B is descendant of A
		return sm.hasMatchingAncestor(seq.Simple, node)
	case ">": // Child combinator: A > B means B is direct child of A
		if node.Parent != nil {
			return sm.matchesSimple(seq.Simple, node.Parent)
		}
		return false
	case "+": // Adjacent sibling: A + B means B immediately follows A
		sibling := sm.getPreviousSibling(node)
		if sibling != nil {
			return sm.matchesSimple(seq.Simple, sibling)
		}
		return false
	case "~": // General sibling: A ~ B means B is preceded by A
		return sm.hasMatchingPreviousSibling(seq.Simple, node)
	}
	
	return false
}

// hasMatchingAncestor checks if any ancestor matches the selector
func (sm *StyleManager) hasMatchingAncestor(selector css.SimpleSelector, node *RenderNode) bool {
	current := node.Parent
	for current != nil {
		if sm.matchesSimple(selector, current) {
			return true
		}
		current = current.Parent
	}
	return false
}

// hasMatchingPreviousSibling checks if any previous sibling matches the selector
func (sm *StyleManager) hasMatchingPreviousSibling(selector css.SimpleSelector, node *RenderNode) bool {
	if node.Parent == nil {
		return false
	}
	
	// Find node's index in parent's children
	nodeIndex := -1
	for i, child := range node.Parent.Children {
		if child == node {
			nodeIndex = i
			break
		}
	}
	
	if nodeIndex == -1 {
		return false
	}
	
	// Check all previous siblings
	for i := nodeIndex - 1; i >= 0; i-- {
		if sm.matchesSimple(selector, node.Parent.Children[i]) {
			return true
		}
	}
	
	return false
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
	if strings.HasPrefix(pseudoClass, "nth-child(") {
		// For now, just return true for basic support
		return true
	}
	
	switch pseudoClass {
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

// colorNameToHex maps common color names to their hex values.
var colorNameToHex = map[string]string{
	"black":   "#000000",
	"white":   "#ffffff",
	"red":     "#ff0000",
	"green":   "#008000",
	"blue":    "#0000ff",
	"yellow":  "#ffff00",
	"cyan":    "#00ffff",
	"magenta": "#ff00ff",
	"silver":  "#c0c0c0",
	"gray":    "#808080",
	"maroon":  "#800000",
	"olive":   "#808000",
	"purple":  "#800080",
	"teal":    "#008080",
	"navy":    "#000080",
}

func (sm *StyleManager) applyDeclaration(node *RenderNode, decl css.Declaration) {
	style := node.ComputedStyle
	switch decl.Property {
	case "display":
		style.Display = decl.Value
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

func parseFontSize(value string, parentFontSize float32) (float32, error) {
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
	return 0, fmt.Errorf("unsupported font size unit")
}

// parseLength parses a CSS length value and returns its numeric value in pixels
// Supports: px, em, rem, plain numbers (treated as px), and keyword values (thin, medium, thick)
func parseLength(value string, fontSize float32) float32 {
	value = strings.TrimSpace(value)
	
	// Handle empty or "0" values
	if value == "" || value == "0" {
		return 0
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
	lowerValue := strings.ToLower(value)
	if hex, ok := colorNameToHex[lowerValue]; ok {
		return parseHexColor(hex)
	}
	if strings.HasPrefix(lowerValue, "#") {
		return parseHexColor(lowerValue)
	}
	// Add support for other color formats like rgb() later
	return color.Black, fmt.Errorf("unsupported color format: %s", value)
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
	defaultColor := color.Black
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

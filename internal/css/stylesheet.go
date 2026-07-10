package css

import (
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// Origin represents the origin of a CSS rule (M3.1)
type Origin uint8

const (
	OriginUserAgent Origin = iota // Browser default styles
	OriginUser                    // User stylesheets
	OriginAuthor                  // Document stylesheets (default)
)

// StyleSheet represents a CSS stylesheet.
type StyleSheet struct {
	Rules   []Rule
	AtRules []AtRule
}

// Rule represents a single CSS rule.
type Rule struct {
	Selectors    []SelectorSequence
	Declarations []Declaration
	SourceOrder  uint32    // Declaration order in stylesheet (M3.1)
	Origin       Origin    // Rule origin (M3.1)
	Specificity  [3]uint16 // Computed specificity (M3.1)
}

// AtRule represents an at-rule like @media, @import, @keyframes
type AtRule struct {
	Name         string
	Prelude      string
	Rules        []Rule
	AtRules      []AtRule
	Declarations []Declaration
}

// SelectorSequence represents a complete selector with combinators
// e.g., "div > p.class" or "h1 + p"
type SelectorSequence struct {
	Simple     SimpleSelector
	Combinator string // "", " " (descendant), ">" (child), "+" (adjacent), "~" (general sibling)
	Next       *SelectorSequence
}

// SimpleSelector represents a simple selector (e.g., "div.class#id:hover")
type SimpleSelector struct {
	TagName        string
	ID             string
	Classes        []string
	PseudoClasses  []string
	PseudoElements []string
	Attributes     []AttributeSelector
	Universal      bool // true for "*"
}

// AttributeSelector represents an attribute selector like [type="text"]
type AttributeSelector struct {
	Name     string
	Operator string // "=", "~=", "|=", "^=", "$=", "*="
	Value    string
}

// Selector is an alias for backward compatibility
type Selector = SimpleSelector

// Declaration represents a CSS property-value pair.
type Declaration struct {
	Property     string    // Property name (kept for backward compatibility)
	Value        string    // Property value
	Important    bool      // !important flag
	PropertyAtom atom.Atom // Interned property name (M3.1)
	IsHot        bool      // true if property is in hot set (M3.1)
}

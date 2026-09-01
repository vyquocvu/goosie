package css

import (
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// Property classification for M3.1
// Hot properties are common layout/color/font properties stored in typed fields
// Cold properties are rare and stored in secondary structures

// Hot property set - common CSS properties that benefit from fast access
var hotProperties = map[string]bool{
	// Layout
	"display":    true,
	"position":   true,
	"top":        true,
	"right":      true,
	"bottom":     true,
	"left":       true,
	"float":      true,
	"clear":      true,
	"z-index":    true,
	"overflow":   true,
	"overflow-x": true,
	"overflow-y": true,
	"visibility": true,
	"opacity":    true,

	// Box model
	"width":          true,
	"height":         true,
	"min-width":      true,
	"min-height":     true,
	"max-width":      true,
	"max-height":     true,
	"margin":         true,
	"margin-top":     true,
	"margin-right":   true,
	"margin-bottom":  true,
	"margin-left":    true,
	"padding":        true,
	"padding-top":    true,
	"padding-right":  true,
	"padding-bottom": true,
	"padding-left":   true,

	// Border
	"border":          true,
	"border-top":      true,
	"border-right":    true,
	"border-bottom":   true,
	"border-left":     true,
	"border-width":    true,
	"border-style":    true,
	"border-color":    true,
	"border-radius":   true,
	"border-collapse": true,
	"border-spacing":  true,

	// Typography
	"font":            true,
	"font-family":     true,
	"font-size":       true,
	"font-weight":     true,
	"font-style":      true,
	"font-variant":    true,
	"line-height":     true,
	"text-align":      true,
	"text-decoration": true,
	"text-transform":  true,
	"text-indent":     true,
	"text-overflow":   true,
	"white-space":     true,
	"word-spacing":    true,
	"letter-spacing":  true,
	"vertical-align":  true,

	// Color and background
	"color":               true,
	"background":          true,
	"background-color":    true,
	"background-image":    true,
	"background-repeat":   true,
	"background-position": true,
	"background-size":     true,

	// Flexbox
	"flex":            true,
	"flex-direction":  true,
	"flex-wrap":       true,
	"flex-flow":       true,
	"flex-grow":       true,
	"flex-shrink":     true,
	"flex-basis":      true,
	"justify-content": true,
	"align-items":     true,
	"align-content":   true,
	"align-self":      true,
	"order":           true,

	// List
	"list-style":          true,
	"list-style-type":     true,
	"list-style-position": true,
	"list-style-image":    true,

	// Table
	"table-layout": true,
	"caption-side": true,
	"empty-cells":  true,
}

// IsHotProperty returns true if the property is in the hot set
func IsHotProperty(property string) bool {
	return hotProperties[property]
}

// PropertyTable is a bounded table for interning CSS property names (M3.1)
// Uses the atom package infrastructure with LRU eviction
var PropertyTable *atom.Table

func init() {
	// Initialize property table with reasonable bounds
	// ~200 properties should cover most CSS with minimal memory
	PropertyTable = atom.NewTable(256, 16384) // 256 entries, 16KB string data

	// Pre-intern all hot properties
	for prop := range hotProperties {
		PropertyTable.Intern(prop)
	}
}

// InternPropertyName interns a CSS property name and returns its atom
// Returns AtomNone (0) if the property cannot be interned
func InternPropertyName(property string) atom.Atom {
	if property == "" {
		return atom.AtomNone
	}
	return PropertyTable.Intern(property)
}

// LookupPropertyAtom looks up an interned property name
// Returns the atom (AtomNone if not found)
func LookupPropertyAtom(property string) atom.Atom {
	if property == "" {
		return atom.AtomNone
	}
	return PropertyTable.Lookup(property)
}

// GetPropertyName returns the string for a property atom
// Returns empty string if not found
func GetPropertyName(a atom.Atom) string {
	if a == atom.AtomNone {
		return ""
	}
	return PropertyTable.LookupByAtom(a)
}

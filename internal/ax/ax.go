// Package ax defines the accessibility tree the engine derives from a laid-out
// document: one tree of labelled, bounded nodes per document that a platform
// accessibility API can walk. It has no dependencies so the engine, the
// surface, and the platform shims can all share the same node type.
package ax

// Role is the accessibility role of one node in the application tree.
type Role uint8

const (
	RoleDocument Role = iota
	RoleGroup
	RoleStaticText
	RoleLink
	RoleButton
	RoleImage
	RoleTextField
	RoleSearch
	RoleProgressBar
	RoleMeter
	RoleTabGroup
	RoleTab
	RoleComboBox
	RoleOption
	RoleHeading
	// RoleListBox is an exposed list of rows: the role Chromium answers for a
	// <select> whose used display size is above 1, and for an authored
	// role="listbox". Appended last so no existing role changes number.
	RoleListBox
)

func (r Role) String() string {
	switch r {
	case RoleDocument:
		return "document"
	case RoleGroup:
		return "group"
	case RoleStaticText:
		return "text"
	case RoleLink:
		return "link"
	case RoleButton:
		return "button"
	case RoleImage:
		return "image"
	case RoleTextField:
		return "textbox"
	case RoleSearch:
		return "search"
	case RoleProgressBar:
		return "progressbar"
	case RoleMeter:
		return "meter"
	case RoleTabGroup:
		return "tablist"
	case RoleTab:
		return "tab"
	case RoleComboBox:
		return "combobox"
	case RoleOption:
		return "option"
	case RoleHeading:
		return "heading"
	case RoleListBox:
		return "listbox"
	default:
		return "generic"
	}
}

// Prop is one entry of the property list a platform accessibility API answers for
// a node: the state and value keys Chromium reports over CDP
// (Accessibility.getFullAXTree) - disabled, expanded, focusable, focused,
// hasPopup, invalid, multiselectable, orientation, required, selected and
// activedescendant.
//
// Value spells the answer the way the reference spells it, a boolean as "true" or
// "false" and a token verbatim, because the reference distinguishes a boolean from
// a string that reads like one: `invalid` is the STRING "false", not a false
// (scripts/verify-optionax-props.js). A property that names another node - today
// only activedescendant - carries its target's DOMNodeID in Ref and leaves Value
// empty, since the reference's own value is a node id that changes per run.
//
// A pruned key is ABSENT rather than answered false: the reference drops
// `focusable` from a disabled control and `selected` from an option whose control
// is disabled, so emitting the key would answer something Chromium does not.
type Prop struct {
	Name  string
	Value string
	Ref   uint64
}

// BoolProp spells a boolean property the way the reference does.
func BoolProp(name string, v bool) Prop {
	if v {
		return Prop{Name: name, Value: "true"}
	}
	return Prop{Name: name, Value: "false"}
}

// StringProp answers a token or a string-valued property verbatim.
func StringProp(name, value string) Prop {
	return Prop{Name: name, Value: value}
}

// RefProp answers a property that names another node, by that node's DOMNodeID.
func RefProp(name string, target uint64) Prop {
	return Prop{Name: name, Ref: target}
}

// Node is one accessible object. Document bounds are document coordinates in
// CSS pixels; native chrome bounds are logical points from the window's top left.
//
// ID is a stable identifier for automation. Label is the spoken name: element
// text for links and buttons, alt text for images, or a field label. Value is
// an editable value or selected state. Href is a link target. Children nests
// in accessibility order.
//
// Props is the node's property list. Selected and Disabled stay alongside it
// because they are what the platform shims read today, and they answer a
// different question: Selected is the DOM's selectedness, which survives the
// pruning the reference applies to the `selected` property of an option in a
// disabled control.
type Node struct {
	Role       Role
	RoleName   string
	DOMNodeID  uint64
	ID         string
	Label      string
	Value      string
	Href       string
	Selected   bool
	Disabled   bool
	AddressBar bool
	Focused    bool
	Props      []Prop

	X0, Y0, X1, Y1 float32

	Children []Node
}

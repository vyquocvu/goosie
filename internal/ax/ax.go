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
)

// Node is one accessible object. Document bounds are document coordinates in
// CSS pixels; native chrome bounds are logical points from the window's top left.
//
// ID is a stable identifier for automation. Label is the spoken name: element
// text for links and buttons, alt text for images, or a field label. Value is
// an editable value or selected state. Href is a link target. Children nests
// in accessibility order.
type Node struct {
	Role       Role
	ID         string
	Label      string
	Value      string
	Href       string
	Selected   bool
	Disabled   bool
	AddressBar bool
	Focused    bool

	X0, Y0, X1, Y1 float32

	Children []Node
}

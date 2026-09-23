// Package ax defines the accessibility tree the engine derives from a laid-out
// document: one tree of labelled, bounded nodes per document that a platform
// accessibility API can walk. It has no dependencies so the engine, the
// surface, and the platform shims can all share the same node type.
package ax

// Role is the accessibility role of one node in the tree.
type Role uint8

const (
	RoleDocument Role = iota
	RoleGroup
	RoleStaticText
	RoleLink
	RoleButton
	RoleImage
	RoleTextField
)

// Node is one accessible object. Bounds are document coordinates in CSS pixels,
// the same space as every other engine coordinate.
//
// Label is the spoken name: element text for links and buttons, the alt text
// for images, the placeholder for text fields. Value is a field's editable
// content. Href is a link's target. Children nests in document order.
type Node struct {
	Role  Role
	Label string
	Value string
	Href  string

	X0, Y0, X1, Y1 float32

	Children []Node
}

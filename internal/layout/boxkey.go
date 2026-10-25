package layout

import "github.com/vyquocvu/goosie/internal/style"

// BoxKey is the document-lifetime identity of a layout box. An ObjectID
// indexes the current arena and changes when the tree is rebuilt; a BoxKey is
// derived only from stable inputs — the originating DOM node, the box kind and
// an ordinal — so the same box keeps the same key across reflows. The zero
// value means no key has been assigned.
type BoxKey uint64

// BoxKind classifies the box a key names. The zero value is reserved so a key
// built without a kind stays zero and reads as unassigned.
type BoxKind uint16

const (
	BoxKindUnknown BoxKind = iota
	BoxKindBlock
	BoxKindInline
	BoxKindAnonymous
	BoxKindPseudoBefore
	BoxKindPseudoAfter
	BoxKindTable
	BoxKindTableRow
	BoxKindTableCell
	BoxKindTableRowGroup
	BoxKindTableColumn
	BoxKindTableColumnGroup
	BoxKindTableCaption
	BoxKindListItem
)

const (
	boxKeyNodeBits    = 32
	boxKeyKindBits    = 16
	boxKeyOrdinalBits = 16

	boxKeyKindShift    = boxKeyNodeBits
	boxKeyOrdinalShift = boxKeyNodeBits + boxKeyKindBits
)

// NewBoxKey builds a key from the originating DOM node, the box kind and an
// ordinal that distinguishes several boxes of the same kind generated for one
// node. Each field occupies a disjoint bit range, so distinct triples always
// produce distinct keys.
func NewBoxKey(nodeID uint32, kind BoxKind, ordinal uint16) BoxKey {
	return BoxKey(nodeID) |
		BoxKey(kind)<<boxKeyKindShift |
		BoxKey(ordinal)<<boxKeyOrdinalShift
}

// NodeID returns the originating DOM node ID encoded in the key.
func (k BoxKey) NodeID() uint32 { return uint32(k) }

// Kind returns the box kind encoded in the key.
func (k BoxKey) Kind() BoxKind { return BoxKind(uint16(k >> boxKeyKindShift)) }

// Ordinal returns the ordinal encoded in the key.
func (k BoxKey) Ordinal() uint16 { return uint16(k >> boxKeyOrdinalShift) }

// boxKindForStyle maps a computed display value to the key's box kind. A
// missing style leaves the kind unknown; the caller still keys by node ID.
func boxKindForStyle(s *style.ComputedStyle) BoxKind {
	if s == nil {
		return BoxKindUnknown
	}
	switch s.Display {
	case style.DisplayInline:
		return BoxKindInline
	case style.DisplayListItem:
		return BoxKindListItem
	case style.DisplayTable:
		return BoxKindTable
	case style.DisplayTableRow:
		return BoxKindTableRow
	case style.DisplayTableCell:
		return BoxKindTableCell
	case style.DisplayTableRowGroup, style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup:
		return BoxKindTableRowGroup
	case style.DisplayTableColumn:
		return BoxKindTableColumn
	case style.DisplayTableColumnGroup:
		return BoxKindTableColumnGroup
	case style.DisplayTableCaption:
		return BoxKindTableCaption
	default:
		return BoxKindBlock
	}
}

package layout

import "github.com/vyquocvu/goosie/internal/style"

type BoxKey uint64

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
	BoxKindPseudoFirstLetter
	BoxKindWord
)

const (
	boxKeyNodeBits    = 32
	boxKeyKindBits    = 16
	boxKeyOrdinalBits = 16

	boxKeyKindShift    = boxKeyNodeBits
	boxKeyOrdinalShift = boxKeyNodeBits + boxKeyKindBits
)

func NewBoxKey(nodeID uint32, kind BoxKind, ordinal uint16) BoxKey {
	return BoxKey(nodeID) |
		BoxKey(kind)<<boxKeyKindShift |
		BoxKey(ordinal)<<boxKeyOrdinalShift
}

func (k BoxKey) NodeID() uint32 { return uint32(k) }

func (k BoxKey) Kind() BoxKind { return BoxKind(uint16(k >> boxKeyKindShift)) }

func (k BoxKey) Ordinal() uint16 { return uint16(k >> boxKeyOrdinalShift) }

func boxKindForStyle(s *style.ComputedStyle) BoxKind {
	if s == nil {
		return BoxKindUnknown
	}
	switch s.Display {
	case style.DisplayInline:
		return BoxKindInline
	case style.DisplayListItem:
		return BoxKindListItem
	case style.DisplayTable, style.DisplayInlineTable:
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

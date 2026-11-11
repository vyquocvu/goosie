package layout

import "github.com/vyquocvu/goosie/internal/style"

// FCKind names a formatting-context pass. The block-flow pass is the fallback;
// the others are selected by used display value.
type FCKind int

const (
	FCBlockFlow FCKind = iota
	FCTable
	FCGrid
	FCFlexRow
	FCFlexColumn
)

// layoutPassFunc is the shared container-pass signature: it lays out the box id
// inside containingW and returns content height. layoutTable also writes W/H.
type layoutPassFunc func(a *Arena, id ObjectID, containingW float32) float32

// fcRegistry maps a used display value to its pass. Flex dispatches on direction
// inside layoutFlex, exactly as the former else-if chain did.
//
// It is populated in init rather than by a composite-literal initializer: every
// pass transitively calls blockIntoInner, which calls lookupPass, which reads
// this map, so a var initializer referencing a pass would be a static
// initialization cycle. Filled once by init and only read afterwards, it is
// configuration rather than mutable global state.
var fcRegistry map[style.Display]layoutPassFunc

func init() {
	fcRegistry = map[style.Display]layoutPassFunc{
		style.DisplayTable:       layoutTable,
		style.DisplayInlineTable: layoutTable,
		style.DisplayGrid:        layoutGrid,
		style.DisplayInlineGrid:  layoutGrid,
		style.DisplayFlex:        layoutFlex,
		style.DisplayInlineFlex:  layoutFlex,
	}
}

func layoutFlex(a *Arena, id ObjectID, containingW float32) float32 {
	s := a.Get(id).Style
	if s != nil && (s.FlexDirection == style.FlexColumn || s.FlexDirection == style.FlexColumnReverse) {
		return layoutFlexColumn(a, id, containingW)
	}
	return layoutFlexRow(a, id, containingW)
}

// lookupPass returns the pass for a style, or false when the box is block flow
// (including a nil style, which falls through exactly as the else-if did).
func lookupPass(s *style.ComputedStyle) (layoutPassFunc, FCKind, bool) {
	if s == nil {
		return nil, FCBlockFlow, false
	}
	switch s.Display {
	case style.DisplayTable, style.DisplayInlineTable:
		return fcRegistry[s.Display], FCTable, true
	case style.DisplayGrid, style.DisplayInlineGrid:
		return fcRegistry[s.Display], FCGrid, true
	case style.DisplayFlex, style.DisplayInlineFlex:
		if s.FlexDirection == style.FlexColumn || s.FlexDirection == style.FlexColumnReverse {
			return fcRegistry[s.Display], FCFlexColumn, true
		}
		return fcRegistry[s.Display], FCFlexRow, true
	}
	return nil, FCBlockFlow, false
}

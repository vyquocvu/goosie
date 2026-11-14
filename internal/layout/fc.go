package layout

import "github.com/vyquocvu/goosie/internal/style"

type FCKind int

const (
	FCBlockFlow FCKind = iota
	FCTable
	FCGrid
	FCFlexRow
	FCFlexColumn
)

type layoutPassFunc func(a *Arena, id ObjectID, containingW float32) float32

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

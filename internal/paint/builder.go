package paint

import (
	"github.com/vyquocvu/goosie/internal/layout"
)

type Builder struct {
	list           *List
	arena          *layout.Arena
	scale          float32
	metrics        layout.Metrics
	focus          focusState
	selection      map[layout.BoxKey]SelSpan
	find           map[layout.BoxKey][]FindMark
	validation     *ValidationBubble
	listbox        *ListBox
	inspect        *InspectHighlight
	scrollOffset   func(obj *layout.Object, prop string) float64
	groupHeadStack []int32
}

type focusState struct {
	object            *layout.Object
	caret             int
	marked            string
	markedLen         int
	compositionNode   uint32
	compositionOffset int
}

func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

func (b *Builder) SetFocus(obj *layout.Object, caret int, marked string, markedLen int) {
	b.focus = focusState{object: obj, caret: caret, marked: marked, markedLen: markedLen}
}

func (b *Builder) SetCompositionCaret(nodeID uint32, offset int) {
	b.focus.compositionNode = nodeID
	b.focus.compositionOffset = offset
}

func (b *Builder) SetSelection(spans []SelSpan) {
	b.selection = make(map[layout.BoxKey]SelSpan, len(spans))
	for _, sp := range spans {
		b.selection[sp.Key] = sp
	}
}

func (b *Builder) SetFind(marks []FindMark) {
	b.find = make(map[layout.BoxKey][]FindMark, len(marks))
	for _, m := range marks {
		b.find[m.Key] = append(b.find[m.Key], m)
	}
}

func (b *Builder) SetScrollOffsetProvider(fn func(obj *layout.Object, prop string) float64) {
	b.scrollOffset = fn
}

func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root, nil, 0, 1)
	b.paintValidationBubble()
	b.paintSelectListBox()
	b.paintInspectHighlight()
}

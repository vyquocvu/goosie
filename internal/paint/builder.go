package paint

import "github.com/vyquocvu/goosie/internal/layout"

// Builder walks a layout arena and emits display commands into a List.
//
// The builder is the seam between layout and paint: layout produces positioned
// boxes with styles, and the builder translates each box into the drawing
// operations that paint understands. The builder runs after layout is complete;
// it does not mutate the arena.
type Builder struct {
	list      *List
	arena     *layout.Arena
	scale     float32
	metrics   layout.Metrics
	focus     focusState
	selection map[*layout.Object]SelSpan
	find      map[*layout.Object][]FindMark
}

// focusState groups the input state that influences text and control painting.
type focusState struct {
	object *layout.Object
	caret  int
	marked string
}

// NewBuilder returns a builder that will emit commands into list. metrics is
// the same glyph-advance source layout measured with; nil falls back to the
// half-em estimate layout used, which spaces glyphs approximately.
func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

// SetFocus marks the control to paint with a caret and focus ring. caret is a
// rune index into the control's value; marked is the IME composition preview
// shown spliced into the value at the caret.
func (b *Builder) SetFocus(obj *layout.Object, caret int, marked string) {
	b.focus = focusState{object: obj, caret: caret, marked: marked}
}

// SetSelection marks word boxes to paint with the selection highlight. The
// spans reference the same layout objects the arena walk visits, so identity
// in this map is what paintText matches on.
func (b *Builder) SetSelection(spans []SelSpan) {
	b.selection = make(map[*layout.Object]SelSpan, len(spans))
	for _, sp := range spans {
		b.selection[sp.Obj] = sp
	}
}

// SetFind marks find-in-page highlight rects to paint under their text boxes.
// A mark is emitted when the walk reaches its Obj, so the fill lands beneath
// that run's glyphs rather than on top of the whole layer.
func (b *Builder) SetFind(marks []FindMark) {
	b.find = make(map[*layout.Object][]FindMark, len(marks))
	for _, m := range marks {
		b.find[m.Obj] = append(b.find[m.Obj], m)
	}
}

// Build walks the arena starting at root and appends display commands to the
// list. The root object's position is taken as the origin; a page with a
// scrolled viewport passes the scroll offset as root.X/Y before calling Build.
func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root, nil, 0, 1)
}

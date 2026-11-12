package paint

import (
	"github.com/vyquocvu/goosie/internal/layout"
)

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
	selection map[layout.BoxKey]SelSpan
	find      map[layout.BoxKey][]FindMark
	// validation is the constraint message a blocked submission is showing.
	validation *ValidationBubble
	// listbox is the <select> option list a pointer has opened, in document CSS
	// px. The engine computes it once and hit-tests the same rows.
	listbox *ListBox
	// inspect is the node the inspector is outlining, in document CSS px. It is
	// a paint-only overlay and never mutates the DOM.
	inspect *InspectHighlight
	// scrollOffset queries the JS runtime for an element's current scroll
	// offset along one axis. nil means no element scrolling is active.
	scrollOffset func(obj *layout.Object, prop string) float64
	// groupHeadStack holds the commandStart ordinal of each currently-open clean
	// faded group, outermost first. A nested group that would share its head
	// command with an enclosing recorded group is not recorded separately (see
	// paintBox): its fade threads per-command inside the outer buffer instead.
	// Safe (never loses a fade); the only case it touches is a decoration-less
	// faded container whose first command is an already-grouped faded child.
	groupHeadStack []int32
}

// focusState groups the input state that influences text and control painting.
type focusState struct {
	object            *layout.Object
	caret             int
	marked            string
	markedLen         int // rune length of marked range for selection highlight
	compositionNode   uint32
	compositionOffset int
}

// NewBuilder returns a builder that will emit commands into list. metrics is
// the same glyph-advance source layout measured with; nil falls back to the
// half-em estimate layout used, which spaces glyphs approximately.
func NewBuilder(list *List, arena *layout.Arena, scale float32, metrics layout.Metrics) *Builder {
	return &Builder{list: list, arena: arena, scale: scale, metrics: metrics}
}

// SetFocus marks the control to paint with a caret and focus ring. caret is a
// rune index into the control's value; marked is the IME composition preview
// shown spliced into the value at the caret; markedLen is the rune length of
// the marked range for selection highlight (may differ from len(marked) when
// the value already includes the marked text).
func (b *Builder) SetFocus(obj *layout.Object, caret int, marked string, markedLen int) {
	b.focus = focusState{object: obj, caret: caret, marked: marked, markedLen: markedLen}
}

// SetCompositionCaret locates a contenteditable IME preview in its source Text
// node using the Selection's UTF-16 offset.
func (b *Builder) SetCompositionCaret(nodeID uint32, offset int) {
	b.focus.compositionNode = nodeID
	b.focus.compositionOffset = offset
}

// SetSelection marks word boxes to paint with the selection highlight. Spans
// are keyed by BoxKey, the document-lifetime identity of the box the arena walk
// visits, so paintText matches on identity rather than an arena pointer.
func (b *Builder) SetSelection(spans []SelSpan) {
	b.selection = make(map[layout.BoxKey]SelSpan, len(spans))
	for _, sp := range spans {
		b.selection[sp.Key] = sp
	}
}

// SetFind marks find-in-page highlight rects to paint under their text boxes.
// Marks are keyed by BoxKey: a mark is emitted when the walk reaches the box
// with that identity, so the fill lands beneath that run's glyphs rather than
// on top of the whole layer.
func (b *Builder) SetFind(marks []FindMark) {
	b.find = make(map[layout.BoxKey][]FindMark, len(marks))
	for _, m := range marks {
		b.find[m.Key] = append(b.find[m.Key], m)
	}
}

// SetScrollOffsetProvider supplies a function that returns an element's current
// scroll offset along a given axis ("scrollTop" or "scrollLeft"). The builder
// uses this to translate children of scroll containers during paint.
func (b *Builder) SetScrollOffsetProvider(fn func(obj *layout.Object, prop string) float64) {
	b.scrollOffset = fn
}

// Build walks the arena starting at root and appends display commands to the
// list. The root object's position is taken as the origin; a page with a
// scrolled viewport passes the scroll offset as root.X/Y before calling Build.
func (b *Builder) Build(root layout.ObjectID) {
	b.paintBox(root, nil, 0, 1)
	b.paintValidationBubble()
	b.paintSelectListBox()
	b.paintInspectHighlight()
}

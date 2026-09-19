// Package layout builds a flat arena of positioned boxes from a styled DOM tree.
//
// The arena is the single data structure that connects style resolution to paint:
// every DOM element with a box gets one Object, indexed by ObjectID, mutated in
// place as each layout pass runs. The paint builder walks the arena and emits
// display commands; nothing in the frame path ever sees the DOM directly.
//
// Layout algorithms run in a fixed order: block, then inline, then float, then
// flex, then grid, then table. Each pass reads the style and the box geometry
// produced by earlier passes and writes positions and sizes. The order is the
// dependency order: floats position inside their containing block before inline
// content flows around them, for example.
package layout

import (
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// ObjectID identifies one box in the arena. It is a uint32 index, not a pointer,
// so the arena can be a flat slice and a pass can mutate any object without
// aliasing concerns. The zero value is reserved; valid IDs start at 1.
type ObjectID uint32

const rootID ObjectID = 1

// Object is one box in the layout arena.
//
// A box has four concentric rects: content, padding, border, margin. Layout
// writes Content and Margin; paint reads all four to position borders,
// backgrounds, and content. The rects are stored as origin plus four thicknesses
// rather than as four rects so a style change to one side of a border does not
// rewrite the other three.
type Object struct {
	Node   *dom.Node
	Style  *style.ComputedStyle
	Parent ObjectID

	FirstKid, LastKid        ObjectID
	NextSibling, PrevSibling ObjectID

	X, Y, W, H                                           float32
	MarginTop, MarginRight, MarginBottom, MarginLeft     float32
	PaddingTop, PaddingRight, PaddingBottom, PaddingLeft float32
	BorderTop, BorderRight, BorderBottom, BorderLeft     float32

	// Image carries the decoded pixels of a replaced element so paint can draw
	// it. It is `any` rather than an image type because layout may not depend
	// on image decoding; the engine sets it to a concrete decoded image after
	// layout, and paint reads it back out with a type assertion.
	Image any

	// BgImage carries the decoded pixels of this box's CSS background-image, if
	// any. Like Image it is `any` so layout stays free of the image packages;
	// paint type-asserts it and uses the object's Style for repeat, size and
	// position.
	BgImage any

	// StaticX/StaticY record where an out-of-flow box would have started had it
	// stayed in the flow. CSS uses that position when an absolutely positioned
	// box specifies neither the relevant inset nor a static-friendly pair.
	StaticX, StaticY float32

	Baseline float32
	flags    uint8
}

const (
	flagInlineLaidOut uint8 = 1 << iota
	// flagOutOfFlow marks a box block layout deliberately left unpositioned -
	// an absolutely positioned box, which the positioning pass places later. The
	// inline pass honours the mark so it does not lay the box's text out at the
	// zero position the box still holds when the main passes run.
	flagOutOfFlow
	// flagRowPlaced marks an inline-block that block layout already gave a slot
	// in a row, so the inline pass must not also drop it onto a text line.
	flagRowPlaced
	// flagAnonymous marks a box layout invented to hold inline content that
	// shares a container with block children. It has no DOM node, and its own
	// children are the run it wraps, so the wrapping pass must not recurse into
	// it.
	flagAnonymous
)

// ContentRect returns the content box as four edges.
func (o *Object) ContentRect() (x0, y0, x1, y1 float32) {
	x0 = o.X + o.BorderLeft + o.PaddingLeft
	y0 = o.Y + o.BorderTop + o.PaddingTop
	x1 = x0 + o.W
	y1 = y0 + o.H
	return
}

// BorderRect returns the border box as four edges.
func (o *Object) BorderRect() (x0, y0, x1, y1 float32) {
	x0 = o.X
	y0 = o.Y
	x1 = x0 + o.W + o.PaddingLeft + o.PaddingRight + o.BorderLeft + o.BorderRight
	y1 = y0 + o.H + o.PaddingTop + o.PaddingBottom + o.BorderTop + o.BorderBottom
	return
}

// BorderH returns the height of the border box. W and H are both content-box
// dimensions, which is what ContentRect, BorderRect and the explicit-height
// branch of resolveBoxSizes all assume; this is the one place that spelling is
// needed as an expression rather than as edges.
func (o *Object) BorderH() float32 {
	return o.H + o.PaddingTop + o.PaddingBottom + o.BorderTop + o.BorderBottom
}

// Metrics supplies per-glyph advance widths so layout measures text with the
// same numbers paint draws with. The size is the pixel size the glyph is drawn
// at: layout passes CSS pixels, paint passes device pixels. The slot is the
// resolved face - family, weight, slant - because measuring Times and drawing
// Arial is how text overflows its box. It lives as an interface rather than a
// concrete font type because layout may not depend on the raster package; the
// concrete Fonts type satisfies it structurally, and the binaries wire it in.
//
// A nil Metrics is valid: measurement falls back to a half-em-per-character
// estimate. Text with a nil Metrics renders, but spacing drifts from the real
// font, which is why every binary passes one.
type Metrics interface {
	GlyphAdvance(sizePx int32, r rune, slot frame.FontSlot) int32
	// GlyphAdvanceFixed reports the same advance in 26.6 fixed-point units
	// (1/64 px) as an int32. A browser keeps the pen position fractional across
	// a run and rounds only where a glyph lands; summing per-glyph integer
	// advances loses a fraction of a pixel per character, which over a text line
	// is tens of pixels of drift and every word wrapping in the wrong place.
	// Callers that measure a whole word or place a run use this; GlyphAdvance
	// remains for one-off widths where the integer is the answer.
	GlyphAdvanceFixed(sizePx int32, r rune, slot frame.FontSlot) int32
	// LineMetrics reports the face's ascent, descent and the height a line box
	// takes when line-height is normal, all in device pixels for sizePx. Layout
	// needs the ascent to place the baseline and the descent to size the content
	// area; the normal height is the font's own leading, which differs by family
	// by more than a pixel.
	LineMetrics(sizePx int32, slot frame.FontSlot) (ascent, descent, normalLineHeight int32)
}

// Arena is the flat box tree.
//
// Objects is indexed by ObjectID; the zero slot is unused so the zero ObjectID
// is a sentinel. Walk methods use the sibling/child links rather than a
// depth-first scan, so a pass that only needs the kids of one box touches one
// cache line per kid rather than the whole arena.
type Arena struct {
	Objects []Object
	byNode  map[dom.NodeID]ObjectID

	// Metrics is optional; see the Metrics interface. Set it before the
	// inline pass runs, which is where text is measured.
	Metrics Metrics

	// NaturalSizes reports the intrinsic size, in CSS px, of each decoded
	// replaced element keyed by its DOM node. It is plain floats because
	// layout may not depend on image decoding; the engine fills it in from the
	// images it fetched and decoded. A missing entry means the image is not
	// (yet) available, and sizing falls back to specified attributes.
	NaturalSizes map[dom.NodeID]NaturalSize

	// ViewportH is the height of the initial containing block, which is the box
	// a percentage height on the root element resolves against. Block sets it.
	// Zero means no viewport height is modelled, so every percentage height
	// behaves as auto.
	ViewportH float32
}

// NaturalSize is the intrinsic width and height of a replaced element.
type NaturalSize struct{ W, H float32 }

// NewArena returns an arena with the root object pre-allocated at index 1.
func NewArena() *Arena {
	a := &Arena{
		Objects: make([]Object, 1, 64),
		byNode:  make(map[dom.NodeID]ObjectID),
	}
	// Pre-allocate the root slot so rootID (1) is the first valid ID.
	a.Objects = append(a.Objects, Object{})
	return a
}

// Alloc reserves a new ObjectID and returns a pointer to the object. The pointer
// is stable across further Alloc calls because the arena grows by doubling; a
// pass that needs to read a sibling while writing a kid can hold both pointers.
func (a *Arena) Alloc() (ObjectID, *Object) {
	id := ObjectID(len(a.Objects))
	a.Objects = append(a.Objects, Object{})
	return id, &a.Objects[id]
}

// Get returns the object for an ID. The ID is assumed valid; a bad ID is a bug
// in the layout pass that produced it, and a panic is more useful than a nil
// check that hides the bug until paint.
func (a *Arena) Get(id ObjectID) *Object {
	return &a.Objects[id]
}

// ForNode returns the object already allocated for n, if any.
func (a *Arena) ForNode(id dom.NodeID) (ObjectID, bool) {
	oid, ok := a.byNode[id]
	return oid, ok
}

// Link adds kid as the last child of parent. The arena owns the link structure;
// a pass that reorders children calls Unlink first and then Link in the new
// order, rather than rewriting the links by hand.
func (a *Arena) Link(parent, kid ObjectID) {
	p := &a.Objects[parent]
	k := &a.Objects[kid]
	k.Parent = parent
	if p.FirstKid == 0 {
		p.FirstKid = kid
		p.LastKid = kid
		return
	}
	last := &a.Objects[p.LastKid]
	last.NextSibling = kid
	k.PrevSibling = p.LastKid
	p.LastKid = kid
}

// Build constructs the layout tree from a styled document. Every element node
// gets one object; text nodes are not objects but contribute runs to their
// parent's inline content during the inline pass. Pseudo-elements (::before,
// ::after) with non-normal content get synthetic objects injected as first/last
// children of their originating element.
func Build(doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, pseudoStyles map[style.PseudoKey]*style.ComputedStyle) *Arena {
	a := NewArena()
	// Use the pre-allocated root at index 1 for the document node.
	a.Objects[rootID].Node = &doc.Node
	if s, ok := styles[doc.Node.ID]; ok {
		a.Objects[rootID].Style = s
	}
	a.byNode[doc.Node.ID] = rootID
	// Build the subtree under the document.
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() || c.Type == dom.NodeText {
			kid := buildSubtree(a, c, doc, styles, pseudoStyles)
			a.Link(rootID, kid)
		}
	}
	a.Objects[rootID].X = 0
	a.Objects[rootID].Y = 0
	return a
}

func buildSubtree(a *Arena, n *dom.Node, doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, pseudoStyles map[style.PseudoKey]*style.ComputedStyle) ObjectID {
	id, ok := a.ForNode(n.ID)
	if ok {
		return id
	}
	id, obj := a.Alloc()
	a.byNode[n.ID] = id
	obj.Node = n
	if s, ok := styles[n.ID]; ok {
		obj.Style = s
	}

	// Inject ::before pseudo-element as first child if it has content.
	if n.Element() && pseudoStyles != nil {
		if beforeStyle, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "before"}]; ok {
			beforeID := buildPseudoElement(a, doc, beforeStyle)
			a.Link(id, beforeID)
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() || c.Type == dom.NodeText {
			kid := buildSubtree(a, c, doc, styles, pseudoStyles)
			a.Link(id, kid)
		}
	}

	// Inject ::after pseudo-element as last child if it has content.
	if n.Element() && pseudoStyles != nil {
		if afterStyle, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "after"}]; ok {
			afterID := buildPseudoElement(a, doc, afterStyle)
			a.Link(id, afterID)
		}
	}

	return id
}

// buildPseudoElement creates a layout object for a pseudo-element with synthetic
// text content. The object is an anonymous text node.
func buildPseudoElement(a *Arena, doc *dom.Document, pseudoStyle *style.ComputedStyle) ObjectID {
	id, obj := a.Alloc()
	obj.flags |= flagAnonymous
	obj.Style = pseudoStyle

	// Create a synthetic text node with the generated content.
	textNode := doc.NewText(pseudoStyle.Content)
	obj.Node = textNode

	return id
}

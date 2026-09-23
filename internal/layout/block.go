package layout

import (
	"sort"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/style"
)

// Block runs the block layout pass over the arena.
//
// Block layout is the first pass and the one every other pass depends on: it
// sets the width and height of every block box, positions block kids vertically
// with margin collapsing, and establishes the containing block for positioned
// descendants. The pass is a single recursive walk; the recursion is the
// containing-block stack.
func Block(a *Arena, root ObjectID, viewportW, viewportH float32) {
	a.ViewportH = viewportH
	flattenDisplayContents(a, root)
	blockInto(a, root, viewportW)
}

// flattenDisplayContents removes display:contents elements from the tree before
// layout runs. Each such element's children are spliced into its parent's child
// list in its place, so every layout pass sees them as direct children. The
// element itself is unlinked and zeroed; it generates no box and no paint.
func flattenDisplayContents(a *Arena, id ObjectID) {
	obj := a.Get(id)
	kid := obj.FirstKid
	for kid != 0 {
		next := a.Get(kid).NextSibling
		flattenDisplayContents(a, kid)
		k := a.Get(kid)
		if k.Style != nil && k.Style.Display == style.DisplayContents {
			prev := k.PrevSibling
			nextSib := k.NextSibling
			first := k.FirstKid
			last := k.LastKid
			if first != 0 {
				for c := first; c != 0; c = a.Get(c).NextSibling {
					a.Get(c).Parent = id
				}
				if prev != 0 {
					a.Get(prev).NextSibling = first
				} else {
					obj.FirstKid = first
				}
				a.Get(first).PrevSibling = prev
				a.Get(last).NextSibling = nextSib
				if nextSib != 0 {
					a.Get(nextSib).PrevSibling = last
				} else {
					obj.LastKid = last
				}
			} else {
				if prev != 0 {
					a.Get(prev).NextSibling = nextSib
				} else {
					obj.FirstKid = nextSib
				}
				if nextSib != 0 {
					a.Get(nextSib).PrevSibling = prev
				} else {
					obj.LastKid = prev
				}
			}
			k.FirstKid = 0
			k.LastKid = 0
			k.NextSibling = 0
			k.PrevSibling = 0
			k.W = 0
			k.H = 0
		}
		kid = next
	}
}

// definiteH reports the content height the children of box id may resolve a
// percentage height against, or -1 when that height is indefinite. CSS 2.1
// §10.5 makes a percentage height behave as auto unless the containing block's
// height is given, so the walk up the parent chain stops at the first auto box
// and the initial containing block - the viewport - ends it.
//
// Only the stylesheet answers this, never obj.H: the block pass lays children
// out before it knows the height they produce, so an auto box's H is whatever
// the last pass left in it.
func definiteH(a *Arena, id ObjectID) float32 {
	if id == 0 || id == rootID {
		if a.ViewportH > 0 {
			return a.ViewportH
		}
		return -1
	}
	obj := a.Get(id)
	s := obj.Style
	if s == nil {
		return -1
	}
	h := s.Height
	switch {
	case h >= 0:
	case isPctLength(h):
		ph := definiteH(a, obj.Parent)
		if ph < 0 {
			return -1
		}
		h = (-2 - h) * ph / 100
	default:
		return -1
	}
	if s.BoxSizing == style.BoxSizingBorderBox {
		h -= obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom
	}
	if h < 0 {
		h = 0
	}
	return h
}

func blockInto(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	// Two heights travel down this pass: parentH, which this box resolves its own
	// percentage height against, and childH, which it hands to its children. Both
	// are -1 when indefinite, and §10.5 turns an indefinite percentage height
	// into auto.
	parentH := definiteH(a, obj.Parent)
	childH := parentH
	var replacedH float32
	if obj.Style != nil {
		resolveBoxSizes(obj, containingW, parentH)
		childH = definiteH(a, id)
		if obj.Style.Display == style.DisplayNone {
			obj.W = 0
			obj.H = 0
			return 0
		}
		if w, h, ok := replacedSize(a, obj, containingW); ok {
			// An `auto` size on a replaced box is the control's own size. Nothing
			// downstream can measure it out of content the box does not have.
			if resolvePctLength(obj.Style.Width, containingW) < 0 {
				obj.W = w
			}
			replacedH = h
		}
	}
	// The content box is inside the padding. Children are positioned relative to
	// the content box origin, and the containing width for percentage children
	// is the content width.
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	// Inline content that shares a container with block children needs a block
	// box of its own to occupy a slot in the flow. A flex or grid container
	// blockifies its children instead, so only its text runs need a wrapper.
	if blockifiesChildren(obj.Style) {
		wrapTextRuns(a, id)
	} else {
		wrapInlineRuns(a, id)
	}
	// Re-fetch: the anonymous boxes just allocated may have grown the arena
	// slice, invalidating the pointer captured above.
	obj = a.Get(id)
	y := float32(0)
	prevBottomMargin := float32(0)
	firstChild := true
	// Inline-block kids flow as rows: they sit side by side from the current
	// vertical cursor, and the tallest of them reserves the row's height.
	var rowTop, rowH, rowCursor float32
	rowActive := false
	// Once the container has inline text of its own, though, its inline-blocks
	// belong to that text's line list: the inline pass flows them as one
	// unbreakable word each, which is what lets a title, its tag pills and its
	// domain share a line. The row cursor starts every line at the content edge,
	// so using it here would drop the pills on top of the title.
	hasInlineRuns := flowsInlineContent(a, obj)
	// Floats hang on the side they were thrown to, one after another along the
	// current flow height, and the deepest one reserves the container's height
	// only when that container establishes a formatting context.
	var floatLead, floatTrail, floatBottom float32
	closeRow := func() {
		if !rowActive {
			return
		}
		if bottom := rowTop + rowH; bottom > y {
			y = bottom
		}
		rowActive = false
		rowTop = 0
		rowH = 0
		rowCursor = 0
	}
	if obj.Style != nil && obj.Style.Display == style.DisplayTable {
		y = layoutTable(a, id, containingW)
	} else if obj.Style != nil && (obj.Style.Display == style.DisplayGrid || obj.Style.Display == style.DisplayInlineGrid) {
		y = layoutGrid(a, id, containingW)
	} else if obj.Style != nil && (obj.Style.Display == style.DisplayFlex || obj.Style.Display == style.DisplayInlineFlex) {
		if obj.Style.FlexDirection == style.FlexColumn || obj.Style.FlexDirection == style.FlexColumnReverse {
			y = layoutFlexColumn(a, id, containingW)
		} else {
			y = layoutFlexRow(a, id, containingW)
		}
	} else {
		for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			// An inline box that holds block content takes a vertical slot of its
			// own: CSS splits such a box around the block child, and the fragment
			// carrying that content spans the full line.
			if isBlock(k) || holdsBlockContent(a, kid) {
				if isOutOfFlow(k.Style) {
					// Out of the flow: record where the box would have started and
					// leave the placement to the positioning pass, which is the
					// first point a final containing block exists. The box takes
					// no part in this container's height or margin collapsing, and
					// does not consume the first-child slot.
					resolveBoxSizes(k, contentW, childH)
					k = a.Get(kid)
					k.StaticX = contentX + k.MarginLeft
					k.StaticY = contentY + y
					k.flags |= flagOutOfFlow
					continue
				}
				if k.Style.Float != style.FloatNone {
					// A float takes no vertical slot: the content after it starts at
					// the same height and rides past its side, which is what puts a
					// 30px vote column beside a story's text instead of above it. Its
					// auto width shrink-wraps like an inline-block's, and it hangs at
					// the edge of the content box, after any float already placed on
					// that side.
					closeRow()
					resolveBoxSizes(k, contentW, childH)
					k = a.Get(kid)
					if k.Style.MarginLeft == style.MarginAuto {
						k.MarginLeft = 0
					}
					if k.Style.MarginRight == style.MarginAuto {
						k.MarginRight = 0
					}
					extra := k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
					if resolvePctLength(k.Style.Width, contentW) < 0 {
						blockInto(a, kid, contentW)
						if !blockifiesChildren(a.Get(kid).Style) {
							inlineInto(a, kid)
						}
						k = a.Get(kid)
						var w float32
						if sx, right, ok := inlineExtent(a, kid); ok && right > sx {
							w = right - sx
						} else if mw := itemMaxContentW(a, kid); mw > 0 {
							w = mw
						}
						clearInlineLaidOut(a, kid)
						if w > 0 {
							k.W = w
							k = a.Get(kid)
							clampWidth(k, contentW)
							k = a.Get(kid)
						}
					}
					boxW := k.W + extra
					if boxW > contentW {
						boxW = contentW
					}
					outer := k.MarginLeft + boxW + k.MarginRight
					if k.Style.Float == style.FloatLeft {
						k.X = contentX + floatLead + k.MarginLeft
						floatLead += outer
					} else {
						k.X = contentX + contentW - floatTrail - k.MarginRight - boxW
						floatTrail += outer
					}
					k.Y = contentY + y + k.MarginTop
					blockInto(a, kid, boxW)
					if !blockifiesChildren(a.Get(kid).Style) {
						inlineInto(a, kid)
					}
					// Re-read the box: laying its content out may have grown the
					// arena and left the captured pointer pointing at the old slice.
					placed := a.Get(kid)
					if bottom := y + placed.MarginTop + placed.BorderH() + placed.MarginBottom; bottom > floatBottom {
						floatBottom = bottom
					}
					continue
				}
				if k.Style.Display == style.DisplayInlineBlock {
					if hasInlineRuns {
						// The inline pass places this box against the container's
						// text, so it gets no slot and no row here. It still has to
						// close a row any earlier inline-block opened one.
						closeRow()
						continue
					}
					// Inline-level, so it takes no vertical slot of its own: it
					// joins the row in progress, or starts a new one after the
					// content width runs out.
					resolveBoxSizes(k, contentW, childH)
					k = a.Get(kid)
					extra := k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
					// An auto width shrinks to the content instead of filling the
					// container, which is what puts a row of buttons side by side.
					// The measure is the extent the words take at the container's
					// own width, read back off the placed words rather than off the
					// box, because a text-align would otherwise be baked into it.
					shrink := resolvePctLength(k.Style.Width, contentW) < 0
					var srcX, measureX, measureY, contentExtent float32
					measured, shrank := false, false
					if shrink {
						measureX, measureY = k.X, k.Y
						blockInto(a, kid, contentW)
						inlineInto(a, kid)
						k = a.Get(kid)
						if sx, right, ok := inlineExtent(a, kid); ok && right > sx {
							srcX, contentExtent, measured = sx, right-sx, true
							k.W = contentExtent
							k = a.Get(kid)
							clampWidth(k, contentW)
							k = a.Get(kid)
						} else if w := blockMaxContentW(a, kid); w > 0 {
							// The measure allocated word objects, so the pointer
							// captured above may address the slice the arena grew
							// out of. Write through a fresh one.
							k = a.Get(kid)
							k.W = w
							clampWidth(k, contentW)
							k = a.Get(kid)
							clearInlineLaidOut(a, kid)
							shrank = true
						}
					}
					boxW := k.W + extra
					if rowActive && rowCursor+k.MarginLeft+boxW+k.MarginRight > contentW {
						closeRow()
					}
					if !rowActive {
						rowActive = true
						rowTop = y
					}
					k.X = contentX + rowCursor + k.MarginLeft
					k.Y = contentY + rowTop + k.MarginTop
					k.flags |= flagRowPlaced
					if measured {
						// The content is still where the measure left it, so it
						// travels to the box rather than being laid out a second
						// time, which would allocate a second set of word objects.
						shiftInlineContent(a, kid, k.X+k.BorderLeft+k.PaddingLeft-srcX, k.Y-measureY)
						// Block descendants were positioned against the measured
						// origin, so they move with the box.
						for c := a.Get(kid).FirstKid; c != 0; c = a.Get(c).NextSibling {
							if isBlock(a.Get(c)) {
								shiftSubtree(a, c, k.X-measureX, k.Y-measureY)
							}
						}
						k = a.Get(kid)
					} else {
						// A shrunk box has a width of its own now, so the re-layout
						// has to be handed that rather than the parent's line:
						// blockInto resolves an auto width against the containing
						// width it is given, which would widen the box straight back
						// out to the full row.
						own := contentW
						if shrank {
							own = boxW
						}
						blockInto(a, kid, own)
						inlineInto(a, kid)
						k = a.Get(kid)
					}
					if mh := k.MarginTop + k.BorderH() + k.MarginBottom; mh > rowH {
						rowH = mh
					}
					rowCursor += k.MarginLeft + boxW + k.MarginRight
					continue
				}
				closeRow()
				resolveBoxSizes(k, contentW, childH)
				if !isFlowBlock(k) {
					// resolveBoxSizes leaves an inline box at width 0. The fragment
					// CSS splits out around the block content is a block box, so it
					// fills the line instead.
					k.W = contentW - k.MarginLeft - k.MarginRight - k.PaddingLeft - k.PaddingRight - k.BorderLeft - k.BorderRight
					if k.W < 0 {
						k.W = 0
					}
				}

				var topMargin float32
				if firstChild {
					firstChild = false
					// A parent with no top border or padding has its top margin
					// collapse with its first in-flow child's, and the child's own
					// first-child chain rides along with it. carryTop already paid
					// for the whole run when this box was placed, so the child
					// starts flush with the content edge rather than pushing the
					// box down a second time. Only reached for non-flex containers;
					// flex parents never collapse margins with their items.
					if collapsesThroughTop(a, obj) && k.Style.Position == style.PositionStatic {
						topMargin = 0
					} else {
						topMargin = carryTop(a, kid)
					}
				} else {
					topMargin = collapseMargin(prevBottomMargin, carryTop(a, kid))
				}
				k.X = contentX + k.MarginLeft + centerBlockChildLead(obj, k, contentW)
				k.Y = contentY + y + topMargin
				blockInto(a, kid, contentW)
				// Run inline layout immediately so the element's height is known
				// before positioning the next sibling. A flex or grid container
				// already laid its content out itself; running the inline pass
				// over it would re-split the word objects its items made.
				if !blockifiesChildren(a.Get(kid).Style) {
					inlineInto(a, kid)
				}
				// Re-fetch k: inlineInto may have allocated word objects, growing
				// the arena slice and invalidating the pointer captured above.
				k = a.Get(kid)
				y = (k.Y - contentY) + k.BorderH()
				prevBottomMargin = carryBottom(a, kid)
			}
		}
		closeRow()
		// A container that establishes a formatting context grows around the
		// floats it holds. A plain block box does not: it collapses past them,
		// which is the clearance problem a clearfix exists for rather than a
		// height problem.
		if floatBottom > y && startsNewFC(a, a.Get(id)) {
			y = floatBottom
		}
	}
	// Re-fetch obj: the block kids loop or layoutFlexRow may have called
	// Alloc (via collectInline), growing the arena slice and invalidating
	// the pointer captured at function entry or after the flex path.
	obj = a.Get(id)
	// A box with no bottom border or padding hands its last child's bottom margin
	// to the next sibling instead of growing around it, so the margin only takes
	// up room inside this box when it has nowhere to carry out to.
	if !collapsesThroughBottom(a, obj) {
		y += prevBottomMargin
	}
	// H is the content height, like W; the padding and borders a box carries
	// are added back by ContentRect/BorderRect at the edges. A percentage height
	// only counts here when it resolved, which is why the test reads the height
	// through the containing block rather than the raw declaration.
	if obj.Style != nil && resolvePctLength(obj.Style.Height, parentH) < 0 {
		obj.H = y
		if replacedH > obj.H {
			obj.H = replacedH
		}
	}
	if obj.Style == nil {
		return obj.BorderH()
	}
	// The height children produced is the box's height only until min-height and
	// max-height have a say, which is why this runs after the stacking above.
	clampHeight(obj, parentH)
	return obj.BorderH() + obj.MarginTop + obj.MarginBottom
}

// layoutFlexRow lays out a single-line row flex container.
//
// Supports flex-grow, flex-basis, gap, and auto-width items. Items without
// flex-grow and without explicit sizing share the remaining space equally.
// align-items:stretch is the default for auto-height items in a definite-height
// container. Raw text nodes inside a flex container are skipped.
func layoutFlexRow(a *Arena, id ObjectID, containingW float32) float32 {
	// Re-fetch obj: the caller's pointer may be stale if a prior sibling's
	// layout caused arena reallocation. Recompute all content-box values
	// from the fresh pointer so coordinates are never derived from stale data.
	obj := a.Get(id)
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	type item struct {
		id       ObjectID
		obj      *Object
		basis    float32
		grow     float32
		measured bool
		// pct is the percentage a style width was written as, or 0 when the width
		// was not a percentage. Block layout has to re-resolve such a width, and
		// this says what it was a share of.
		pct float32
		// srcX is where the measured item's inline content actually landed, and
		// contentW the width that content needs. Both are read back from the
		// placed words so the measure survives a text-align that shifts them.
		srcX     float32
		contentW float32
		// minW is the item's min-content width plus its own box extras: the floor
		// flex-shrink may not take it below, which is CSS's min-width:auto for a
		// flex item. Zero means the item has no text floor we could compute.
		minW float32
		// declared marks an item whose basis came from the stylesheet rather than
		// from measuring its content. Only such an item has to hand that basis
		// back to the block pass as its containing width.
		declared bool
		// atMax marks an item whose base size exceeded the width its max-width
		// allows. Such an item is frozen at that clamped size before any
		// shrinking, so it takes nothing off the line's shortfall.
		atMax bool
		// blockKids marks an item whose content is block level rather than inline
		// text. inlineExtent cannot see it, so its max-content comes from
		// itemMaxContentW, and it must be re-laid out at its final width instead
		// of having measured words shifted into place.
		blockKids bool
		// order is the CSS `order` value; items sort by it (stable) before
		// measurement so a reordered line paints in visual order.
		order int
	}
	var items []item
	// A row container's main axis is horizontal, so the space between its items
	// comes from column-gap.
	gap := obj.Style.ColumnGap
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type != 1 {
			// Skip raw text inside flex containers, but clear the content so
			// the paint builder doesn't render it at the object's zero position.
			if k.Node.Type == 2 {
				k.Node.DataContent = ""
			}
			continue
		}
		if isOutOfFlow(k.Style) {
			// An out-of-flow child is not a flex item; the positioning pass
			// places it. Give it a static position from the container's content
			// origin so an unset inset has something to fall back to.
			k.StaticX = contentX
			k.StaticY = contentY
			k.flags |= flagOutOfFlow
			continue
		}
		items = append(items, item{id: kid, obj: k, grow: k.Style.FlexGrow, order: k.Style.Order})
	}
	// CSS `order`: reorder the line's items by their order value before any
	// measurement or placement, keeping DOM order among equal values.
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })
	n := len(items)
	if n == 0 {
		return 0
	}
	for i := range items {
		s := items[i].obj.Style
		basis := float32(-1)
		switch {
		case s.FlexBasis >= 0:
			basis = s.FlexBasis
		case isPctLength(s.FlexBasis):
			// A percentage basis is written as a sentinel too, and it speaks the
			// same box a percentage width would: the container's content box.
			// Missing it leaves the item on auto, so Bootstrap's grid columns -
			// `flex: 0 0 75%` - collapse to the width of their text. Unlike a
			// percentage width, the basis needs no `pct`: the item's own width
			// stays auto, so nothing has to be re-resolved against the container.
			basis = resolvePctLength(s.FlexBasis, contentW)
		case s.Width >= 0:
			basis = s.Width
		case resolvePctLength(s.Width, contentW) >= 0:
			// A percentage width arrives as a sentinel, so the checks above miss
			// it. It speaks the container's content box, which is exactly the
			// basis `width: 100%` gives a flex item.
			basis = resolvePctLength(s.Width, contentW)
			items[i].pct = -2 - s.Width
		}
		if basis < 0 {
			items[i].basis = -1
			continue
		}
		if s.BoxSizing != style.BoxSizingBorderBox {
			basis += s.PaddingLeft + s.PaddingRight + s.BorderLeftWidth + s.BorderRightWidth
		}
		clamped := clampFlexBasis(a.Get(items[i].id), basis, contentW)
		items[i].basis = clamped
		items[i].atMax = clamped < basis
		items[i].declared = true
	}

	// Re-fetch obj: the item collection loop may have triggered Alloc calls
	// (e.g. from prior block layout passes on sibling flex containers), growing
	// the arena and invalidating the pointer fetched at function entry.
	obj = a.Get(id)
	contentH := obj.H
	alignItems := obj.Style.AlignItems

	// First pass: measure the auto-basis items. Each is laid out at the
	// container's content origin with an effectively infinite available width,
	// which is max-content sizing: the text cannot wrap, so the extent it needs
	// is the extent it gets. The extent is read back off the placed words rather
	// than off the box, because resolveBoxSizes has already filled the item's own
	// width with the measurement width and a text-align would otherwise be baked
	// into the measure.
	for i := range items {
		if items[i].basis >= 0 {
			continue
		}
		it := a.Get(items[i].id)
		it.X, it.Y = contentX, contentY
		blockInto(a, items[i].id, maxFlexMeasureWidth)
		it = a.Get(items[i].id)
		if it.W <= 0 {
			// blockInto hands its children a resolved content width locally but
			// only commits W to the box for auto block sizes; an inline-display
			// flex item keeps its auto sentinel. The inline pass reads W, so an
			// uncommitted width would wrap every word onto its own line.
			it.W = maxFlexMeasureWidth - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
			it = a.Get(items[i].id)
		}
		inlineInto(a, items[i].id)
		it = a.Get(items[i].id)
		srcX, right, ok := inlineExtent(a, items[i].id)
		contentWidth := float32(0)
		if ok {
			contentWidth = right - srcX
		}
		// An item whose content is a block child - a nav cell wrapping a
		// block-level link, a card holding a header and a figure - has no inline
		// extent of its own, so inlineExtent reports nothing and the item would
		// collapse to zero width and pile onto its siblings. Its max-content is
		// the width the laid-out block subtree actually needs. Only take this
		// path when the inline pass found nothing: an item that does have inline
		// content already measured correctly above, and its block walk can
		// over-count against a not-yet-final obj.X during measurement.
		if !ok {
			if blockW := itemMaxContentW(a, items[i].id); blockW > 0 {
				contentWidth = blockW
				items[i].blockKids = true
			}
		}
		// A replaced element (input, img) has its width set by replacedSize() in
		// blockInto. If contentWidth is 0 but the element already has a width,
		// use that instead of overwriting it with 0.
		if contentWidth == 0 && it.W > 0 {
			contentWidth = it.W
		}
		it.W = contentWidth
		// Basis is a border-box width.
		items[i].basis = contentWidth + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		items[i].measured = !items[i].blockKids
		items[i].srcX = srcX
		items[i].contentW = contentWidth
		items[i].minW = inlineMinWidth(a, items[i].id) + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
	}

	// A row-reverse container reads its items from the container's right edge, so
	// the visual order is settled before the items are grouped into lines.
	reverse := a.Get(id).Style.FlexDirection == style.FlexRowReverse
	if reverse {
		for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
			items[l], items[r] = items[r], items[l]
		}
	}

	// Lines: a nowrap container keeps every item on one line and shrinks them
	// into it, while a wrapping container starts a new line at the first item that
	// no longer fits. `wrap-reverse` stacks the lines from the other end.
	wrap := a.Get(id).Style.FlexWrap
	lineOf := make([]int, n)
	lineCount := 0
	used := float32(0)
	lineOpen := false
	for i := range items {
		m := a.Get(items[i].id).Style
		outer := items[i].basis + flexMargin(m.MarginLeft) + flexMargin(m.MarginRight)
		if lineOpen {
			if used+gap+outer > contentW && wrap != style.FlexNowrap {
				lineCount++
				lineOpen = false
				used = 0
			} else {
				used += gap
			}
		}
		lineOf[i] = lineCount
		used += outer
		lineOpen = true
	}
	lineCount++

	// Distribute each line's free space once every basis is known, so an auto
	// item's share is computed against the real content widths rather than zero.
	// Space left over goes to the items that grow; space that does not fit comes
	// back off the items that shrink, weighted by how much of it they carry.
	for l := 0; l < lineCount; l++ {
		var lineBasis, lineMargins, growSum, shrinkWeight float32
		count := 0
		for i := range items {
			if lineOf[i] != l {
				continue
			}
			m := a.Get(items[i].id).Style
			lineBasis += items[i].basis
			lineMargins += flexMargin(m.MarginLeft) + flexMargin(m.MarginRight)
			growSum += items[i].grow
			shrinkWeight += items[i].basis * m.FlexShrink
			count++
		}
		if count == 0 {
			continue
		}
		avail := contentW - lineMargins - gap*float32(count-1)
		if avail < 0 {
			avail = 0
		}
		free := avail - lineBasis
		if free > 0 && growSum > 0 {
			// Growth is clamped (§9.7): an item that reaches its max-width freezes
			// there and the space it would have taken goes round again. Adding the
			// whole share in one pass left `basis` longer than the box the item
			// paints, and the cursor that advances by `basis` shifted every later
			// sibling by the difference.
			frozen := make([]bool, len(items))
			for spare, weight := free, growSum; spare > 0 && weight > 0; {
				var excess float32
				for i := range items {
					if lineOf[i] != l || items[i].grow <= 0 || frozen[i] {
						continue
					}
					want := items[i].basis + spare*items[i].grow/weight
					if clamped := clampFlexBasis(a.Get(items[i].id), want, contentW); clamped < want {
						excess += want - clamped
						items[i].basis = clamped
						frozen[i] = true
						continue
					}
					items[i].basis = want
				}
				spare = excess
				weight = 0
				for i := range items {
					if lineOf[i] == l && items[i].grow > 0 && !frozen[i] {
						weight += items[i].grow
					}
				}
			}
		} else if free < 0 && shrinkWeight > 0 {
			// Shrink proportionally to the weighted basis, but never below an
			// item's min-content floor - CSS's min-width:auto for a flex item.
			// An item that hits its floor freezes out of the pool and the rest
			// of the shortfall redistributes; two passes cover the common case
			// of one or two items freezing.
			frozen := make([]bool, len(items))
			// An item clamped down by its max-width is frozen there before the
			// shortfall is shared out. A `width: 370px; max-width: 300px` sidebar
			// pays for none of a sibling's overflow, so it keeps its 300px and the
			// main column gives up the difference instead of sliding under it.
			for i := range items {
				if lineOf[i] == l && items[i].atMax {
					frozen[i] = true
				}
			}
			for pass := 0; pass < 2; pass++ {
				var weight, basisSum float32
				for i := range items {
					if lineOf[i] != l {
						continue
					}
					if !frozen[i] {
						weight += items[i].basis * a.Get(items[i].id).Style.FlexShrink
					}
					basisSum += items[i].basis
				}
				free = avail - basisSum
				if free >= 0 || weight <= 0 {
					break
				}
				for i := range items {
					if lineOf[i] != l || frozen[i] {
						continue
					}
					share := free * (items[i].basis * a.Get(items[i].id).Style.FlexShrink) / weight
					w := items[i].basis + share
					if items[i].minW > 0 && w < items[i].minW {
						w = items[i].minW
						frozen[i] = true
					}
					if w < 0 {
						w = 0
					}
					items[i].basis = w
				}
			}
		}
	}

	// Whatever the main axis did not consume is spare space, and justify-content
	// decides where it goes.
	justify := a.Get(id).Style.JustifyContent

	// Second pass: place every line, and inside it every item. An item measured
	// above still holds its inline content at the measurement origin, so that
	// content travels with it; re-running the inline pass instead would allocate a
	// second set of word objects, which the laid-out flag exists to prevent.
	//
	// A line's cross size is only known once its items have been laid out, so each
	// line places its items first and aligns them afterwards.
	lineTop := float32(0)
	maxBottom := float32(0)
	for order := 0; order < lineCount; order++ {
		l := order
		if wrap == style.FlexWrapReverse {
			l = lineCount - 1 - order
		}
		var line []int
		for i := range items {
			if lineOf[i] == l {
				line = append(line, i)
			}
		}
		if len(line) == 0 {
			continue
		}
		remaining := contentW - gap*float32(len(line)-1)
		for _, i := range line {
			m := a.Get(items[i].id).Style
			remaining -= items[i].basis + flexMargin(m.MarginLeft) + flexMargin(m.MarginRight)
		}
		if remaining < 0 {
			remaining = 0
		}
		lead, trackGap := distributeFlex(justify, reverse, remaining, gap, len(line))

		cursor := contentX + lead
		for _, i := range line {
			// Re-fetch: the previous iteration may have grown the arena slice.
			it := a.Get(items[i].id)
			mTop := flexMargin(it.Style.MarginTop)
			finalX := cursor + flexMargin(it.Style.MarginLeft)
			if items[i].basis >= 0 && it.Style.Width < 0 {
				w := items[i].basis - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
				if w < 0 {
					w = 0
				}
				it.W = w
			}
			it.X = finalX
			it.Y = contentY + lineTop + mTop
			if items[i].measured {
				// Only an item shrunk below its max-content width needs its
				// words re-wrapped; one that fits keeps the shift path so
				// text-align still applies.
				shrunk := it.W+0.5 < items[i].contentW
				var h float32
				var wrapped bool
				if shrunk {
					h, wrapped = reflowInlineWords(a, items[i].id, it.W, finalX+it.BorderLeft+it.PaddingLeft, it.Y+it.BorderTop+it.PaddingTop)
				}
				if wrapped {
					// The item was shrunk below its max-content width, so its
					// single measured line re-wrapped into the width it ended up
					// with. That changes the item's content height.
					if it.Style.Height < 0 && h > 0 {
						it.H = h
						it = a.Get(items[i].id)
					}
				} else {
					// The words are still where the measurement pass left them.
					// Move them to the item's own content origin, re-applying the
					// item's alignment against the width it ended up with.
					extra := float32(0)
					if spare := it.W - items[i].contentW; spare > 0 {
						switch it.Style.TextAlign {
						case style.TextAlignCenter:
							extra = spare / 2
						case style.TextAlignRight:
							extra = spare
						}
					}
					shiftInlineContent(a, items[i].id, finalX+it.BorderLeft+it.PaddingLeft+extra-items[i].srcX, lineTop+mTop)
				}
			} else {
				containW := it.W
				if items[i].pct > 0 {
					// The item's width is a share of the container, so re-resolving
					// it inside block layout needs that share scaled back: the
					// basis is the width the percentage already produced.
					containW = items[i].basis * 100 / items[i].pct
				} else if items[i].declared && it.Style.Width == -1 {
					// The item's main size came from a declared basis while its own
					// width is auto. blockInto re-resolves an auto width as "fill the
					// containing block", so it has to be handed the basis back as
					// that container - otherwise the item stretches to the whole
					// flex line and every sibling after it overflows.
					containW = items[i].basis + it.MarginLeft + it.MarginRight
				} else if items[i].blockKids && it.Style.Width == -1 {
					// An item with block children was measured at max-content, but
					// the flex distribution may have grown or shrunk it. Update the
					// width to reflect the final basis before re-laying out the
					// block subtree.
					w := items[i].basis - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
					if w < 0 {
						w = 0
					}
					it.W = w
					containW = w
				}
				if items[i].blockKids {
					// The measure pass laid the block subtree out at an effectively
					// infinite width and left the inline flag set, so re-running the
					// inline pass would be a no-op. Clear it down to the nested
					// containers so the subtree re-wraps into the width it earned.
					clearInlineLaidOut(a, items[i].id)
				}
				blockInto(a, items[i].id, containW)
				it = a.Get(items[i].id)
				inlineInto(a, items[i].id)
				it = a.Get(items[i].id)
			}
			cursor += flexMargin(it.Style.MarginLeft) + items[i].basis + flexMargin(it.Style.MarginRight) + trackGap
		}

		// The line is as tall as its tallest margin box, unless the container has
		// a height of its own to hold a single line to.
		cross := float32(0)
		if contentH > 0 && lineCount == 1 {
			cross = contentH
		} else {
			for _, i := range line {
				it := a.Get(items[i].id)
				h := flexMargin(it.Style.MarginTop) + flexMargin(it.Style.MarginBottom) + it.BorderH()
				if h > cross {
					cross = h
				}
			}
		}
		for _, i := range line {
			it := a.Get(items[i].id)
			mTop := flexMargin(it.Style.MarginTop)
			mBottom := flexMargin(it.Style.MarginBottom)
			align := it.Style.AlignSelf
			if align == "" {
				align = alignItems
			}
			if align == "" {
				align = "stretch"
			}
			if align == "stretch" && it.Style.Height < 0 {
				// Stretch gives the item the line's cross size, keeping only the
				// space its own margins and box decorations need.
				pb := it.PaddingTop + it.PaddingBottom + it.BorderTop + it.BorderBottom + mTop + mBottom
				if cross > pb {
					it.H = cross - pb
				} else {
					it.H = 0
				}
				it = a.Get(items[i].id)
			}
			if spare := cross - it.BorderH() - mTop - mBottom; spare > 0 {
				switch align {
				case "flex-end":
					it.Y += spare
					shiftInlineContent(a, items[i].id, 0, spare)
				case "center":
					it.Y += spare / 2
					shiftInlineContent(a, items[i].id, 0, spare/2)
				}
			}
		}
		if bottom := lineTop + cross; bottom > maxBottom {
			maxBottom = bottom
		}
		lineTop += cross
	}
	return maxBottom
}

// clampFlexBasis holds a declared basis to the item's min- and max-width, which
// is what keeps `width: 100%; max-width: 400px` a 400px item rather than one that
// fills the line. Both constraints speak the same box language as the basis, so
// no border-sizing adjustment is needed here.
func clampFlexBasis(obj *Object, basis, containingW float32) float32 {
	s := obj.Style
	if s == nil {
		return basis
	}
	if maxW := resolvePctLength(s.MaxWidth, containingW); maxW >= 0 && basis > maxW {
		basis = maxW
	}
	if minW := resolvePctLength(s.MinWidth, containingW); minW >= 0 && basis < minW {
		basis = minW
	}
	if basis < 0 {
		basis = 0
	}
	return basis
}

// flexMargin resolves an item's margin against the flex rules: `auto` is a
// keyword carried as the -1 sentinel, and outside the free-space distribution it
// participates in as 0.
func flexMargin(v float32) float32 {
	if v == style.MarginAuto || v < 0 {
		return 0
	}
	return v
}

// distributeFlex turns justify-content into the leading offset and the per-track
// gap that place the line's items. remaining is the main-axis space flex-grow did
// not consume. A reverse direction has its main axis starting at the far edge,
// which flips a flex-start pack into a flex-end pack; the centred and evenly
// spread values mirror onto themselves.
func distributeFlex(justify string, reverse bool, remaining, gap float32, n int) (lead, trackGap float32) {
	if reverse {
		switch justify {
		case "", "flex-start":
			justify = "flex-end"
		case "flex-end":
			justify = "flex-start"
		}
	}
	trackGap = gap
	switch justify {
	case "flex-end":
		lead = remaining
	case "center":
		lead = remaining / 2
	case "space-between":
		if n > 1 {
			trackGap += remaining / float32(n-1)
		}
	case "space-around":
		per := remaining / float32(n)
		lead = per / 2
		trackGap += per
	case "space-evenly":
		per := remaining / float32(n+1)
		lead = per
		trackGap += per
	}
	return
}

// layoutFlexColumn lays out a single-line column flex container: the main axis
// runs vertically, so the items stack and flex-grow, justify-content and the
// free space all apply top-to-bottom, while the cross axis sizes them
// horizontally.
func layoutFlexColumn(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	contentH := obj.H
	alignItems := obj.Style.AlignItems
	// A column container stacks along the vertical axis, so the space between its
	// items comes from row-gap.
	gap := obj.Style.RowGap

	type item struct {
		id            ObjectID
		basis         float32
		grow          float32
		mTop, mBottom float32
		mLeft, mRight float32
		declared      bool
		crossW        float32
		order         int
	}
	var items []item
	// containerH is the column's definite content height, which is what a
	// percentage flex-basis or height on an item sizes against. It is -1 when the
	// container is content-sized, and then those percentages behave as auto.
	containerH := definiteH(a, id)
	vMargins := float32(0)
	growSum := float32(0)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type != 1 {
			if k.Node.Type == 2 {
				k.Node.DataContent = ""
			}
			continue
		}
		if isOutOfFlow(k.Style) {
			k.StaticX = contentX
			k.StaticY = contentY
			k.flags |= flagOutOfFlow
			continue
		}
		s := k.Style
		it := item{
			id:      kid,
			grow:    s.FlexGrow,
			mTop:    flexMargin(s.MarginTop),
			mBottom: flexMargin(s.MarginBottom),
			mLeft:   flexMargin(s.MarginLeft),
			mRight:  flexMargin(s.MarginRight),
			order:   s.Order,
		}
		// flex-basis and height both size the main axis here, as border-box
		// extents; a content-box declaration needs the padding and borders added.
		var declared float32 = -1
		switch {
		case s.FlexBasis >= 0:
			declared = s.FlexBasis
		case isPctLength(s.FlexBasis):
			declared = resolvePctLength(s.FlexBasis, containerH)
		case s.Height >= 0:
			declared = s.Height
		case isPctLength(s.Height):
			declared = resolvePctLength(s.Height, containerH)
		}
		if declared >= 0 {
			it.basis = declared
			if s.BoxSizing != style.BoxSizingBorderBox {
				it.basis += s.PaddingTop + s.PaddingBottom + s.BorderTopWidth + s.BorderBottomWidth
			}
			it.declared = true
		}
		items = append(items, it)
		vMargins += it.mTop + it.mBottom
		growSum += it.grow
	}
	// CSS `order`: stack the column's items by their order value, DOM order for
	// ties, before main-axis sizing distributes free space.
	sort.SliceStable(items, func(i, j int) bool { return items[i].order < items[j].order })
	n := len(items)
	if n == 0 {
		return 0
	}
	if reverse := obj.Style.FlexDirection == style.FlexColumnReverse; reverse {
		for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
			items[l], items[r] = items[r], items[l]
		}
	}

	// Measure pass: lay each item out across the cross size it ends up with -
	// the container's content box for a stretching item, its own fit-content for
	// one aligned away from the line. The natural main extent is then the height
	// that content produced.
	availForItems := contentH - vMargins - gap*float32(n-1)
	if availForItems < 0 {
		availForItems = 0
	}
	if contentH <= 0 {
		// An auto-height column has no main space to distribute: grow and
		// justify-content have nothing to work with.
		availForItems = 0
	}
	for i := range items {
		avail := contentW
		if w, ok := fitContentCrossW(a, items[i].id, contentX, contentY, contentW, alignItems); ok {
			avail = w
		}
		// A probe runs the item's own passes at the container's origin, so put the
		// box back on the line before laying it out at its final width.
		it := a.Get(items[i].id)
		it.X = contentX + items[i].mLeft
		it.Y = contentY
		blockInto(a, items[i].id, avail)
		it = a.Get(items[i].id)
		if !blockifiesChildren(a.Get(items[i].id).Style) {
			inlineInto(a, items[i].id)
			it = a.Get(items[i].id)
		}
		items[i].crossW = it.W + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		if items[i].declared {
			continue
		}
		items[i].basis = it.BorderH()
	}

	totalBasis := float32(0)
	for i := range items {
		totalBasis += items[i].basis
	}
	free := availForItems - totalBasis
	if free < 0 {
		free = 0
	}
	for i := range items {
		if items[i].grow > 0 && growSum > 0 {
			items[i].basis += free * items[i].grow / growSum
		}
	}
	remaining := availForItems
	for i := range items {
		remaining -= items[i].basis
	}
	if remaining < 0 {
		remaining = 0
	}
	lead, trackGap := distributeFlex(obj.Style.JustifyContent, obj.Style.FlexDirection == style.FlexColumnReverse, remaining, gap, n)

	// Place pass: the cross positions are already final, so only the vertical
	// origin moves, and it takes the item's whole subtree with it because block
	// descendants were positioned from that origin.
	cursor := contentY + lead
	maxBottom := float32(0)
	for i := range items {
		it := a.Get(items[i].id)
		if !items[i].declared && it.Style.Height < 0 && it.Style.FlexGrow > 0 {
			// Only a grown item outgrows the height its content measured at; an
			// auto item's basis is that measurement, so it keeps it.
			h := items[i].basis - it.PaddingTop - it.PaddingBottom - it.BorderTop - it.BorderBottom
			if h < 0 {
				h = 0
			}
			it.H = h
			it = a.Get(items[i].id)
		}
		// The measure pass left the whole subtree positioned from contentY, so a
		// single shift moves the item box and its descendants together. Setting
		// it.Y first and then shifting by the same delta would move only the box.
		if dy := (cursor + items[i].mTop) - it.Y; dy != 0 {
			shiftSubtree(a, items[i].id, 0, dy)
			it = a.Get(items[i].id)
		}
		align := it.Style.AlignSelf
		if align == "" {
			align = alignItems
		}
		if contentW > items[i].crossW {
			// The measure pass left the box at the start of the line, so the item
			// moves by the difference: assigning X and then shifting by the same
			// amount would move the box twice while its content moved once.
			targetX := contentX + items[i].mLeft
			switch align {
			case "center":
				targetX = contentX + (contentW-items[i].crossW)/2 + it.MarginLeft
			case "flex-end":
				targetX = contentX + contentW - items[i].crossW - it.MarginRight
			}
			if dx := targetX - it.X; dx != 0 {
				shiftSubtree(a, items[i].id, dx, 0)
				it = a.Get(items[i].id)
			}
		}
		if bottom := (it.Y - contentY) + items[i].mBottom + it.BorderH(); bottom > maxBottom {
			maxBottom = bottom
		}
		cursor = it.Y + it.BorderH() + items[i].mBottom + trackGap
	}
	return maxBottom
}

// fitContentCrossW reports the width a column flex item takes when its
// alignment does not stretch it: its max-content width, clamped to the
// container's content box. Only `align-items: stretch` fills the line, so a
// centred item is as wide as its own content and the box has to be moved to the
// middle - leaving it full width pins its text to the start edge and paints its
// background across the whole line. It reports false when the item keeps the
// container's width: a declared or percentage width, a stretching alignment, a
// replaced box measured through its own sizing, or content that overflows the
// line. Measuring leaves the subtree placed at the probe's origin, so the caller
// re-runs the block and inline passes at the width returned here.
func fitContentCrossW(a *Arena, id ObjectID, contentX, contentY, avail float32, alignItems string) (float32, bool) {
	s := a.Get(id).Style
	if s == nil || s.Width >= 0 || isPctLength(s.Width) {
		return 0, false
	}
	align := s.AlignSelf
	if align == "" {
		align = alignItems
	}
	switch align {
	case "center", "flex-start", "flex-end":
	default:
		return 0, false
	}

	it := a.Get(id)
	it.X, it.Y = contentX, contentY
	blockInto(a, id, maxFlexMeasureWidth)
	it = a.Get(id)
	if it.W <= 0 {
		it.W = maxFlexMeasureWidth - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
		it = a.Get(id)
	}
	if !blockifiesChildren(it.Style) {
		clearInlineLaidOut(a, id)
		inlineInto(a, id)
		it = a.Get(id)
	}
	boxW := it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
	contentWidth := float32(-1)
	if srcX, right, ok := inlineExtent(a, id); ok {
		contentWidth = right - srcX
	} else if w := itemMaxContentW(a, id); w > 0 {
		contentWidth = w
	}
	// The probe left the subtree laid out at max-content width and marked it
	// done, so the caller's inline pass would skip it and leave the words hung
	// where the wide measure put them - whether or not the width it settles on
	// turns out to be narrower.
	clearInlineLaidOut(a, id)
	if contentWidth < 0 || contentWidth+boxW >= avail {
		return 0, false
	}
	return contentWidth + boxW, true
}

// blockMaxContentW is the max-content width of a box whose content is block
// level, which inlineExtent cannot see because it only walks placed words. The
// subtree is probed at a width nothing wraps at; the caller's own pass then lays
// it out again at the width the box settles on.
func blockMaxContentW(a *Arena, id ObjectID) float32 {
	blockInto(a, id, maxFlexMeasureWidth)
	if !blockifiesChildren(a.Get(id).Style) {
		inlineInto(a, id)
	}
	w := itemMaxContentW(a, id)
	clearInlineLaidOut(a, id)
	if w < 0 {
		return 0
	}
	return w
}

// maxFlexMeasureWidth is the available width handed to a flex item being
// measured for max-content sizing. It is large enough that no text wraps and
// small enough that the geometry stays inside the engine's own limits.
const maxFlexMeasureWidth = 10000

// inlineExtent reports the horizontal span of an object's inline descendants in
// absolute coordinates, and false when it has none. Flex measurement reads the
// width its content actually took instead of reconstructing it from advances.
func inlineExtent(a *Arena, id ObjectID) (minX, maxX float32, ok bool) {
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isBlock(k) {
			continue
		}
		_, _, x1, _ := k.BorderRect()
		// An inline element container carries no box of its own; only the words
		// under it are positioned and sized, so an empty span must not drag the
		// extent back to the origin.
		if x1 > k.X {
			if !ok || k.X < minX {
				minX = k.X
			}
			if !ok || x1 > maxX {
				maxX = x1
			}
			ok = true
		}
		if cmin, cmax, cok := inlineExtent(a, kid); cok {
			if !ok || cmin < minX {
				minX = cmin
			}
			if !ok || cmax > maxX {
				maxX = cmax
			}
			// Content that only exists below this child is still the item's own
			// width: an inline element carries no box, so the words under it are
			// the extent.
			ok = true
		}
	}
	return
}

// inlineMinWidth reports the widest word in a measured subtree: the item's
// min-content width, which is the floor flex-shrink cannot pass. Space runs are
// excluded because they collapse at a break point.
func inlineMinWidth(a *Arena, id ObjectID) float32 {
	m := float32(0)
	var walk func(ObjectID)
	walk = func(cid ObjectID) {
		for kid := a.Get(cid).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if k.Node != nil && k.Node.Type == 2 {
				if k.W > m && !isSpaceWord(k.Node.DataContent) {
					m = k.W
				}
				continue
			}
			if !isBlock(k) {
				walk(kid)
			}
		}
	}
	walk(id)
	return m
}

// itemMaxContentW reports the max-content width of a flex or grid item's content
// box. inlineExtent only sees inline descendants, so an item whose content is a
// block child measures zero through it. This walks the laid-out subtree - the
// caller has already run blockInto at an effectively infinite width, so nothing
// wrapped - and takes the furthest right any descendant word reaches, adding each
// block child's own box on both flanks so its padding, border and margin count.
// It stops at nested flex and grid containers, which size their own content.
func itemMaxContentW(a *Arena, id ObjectID) float32 {
	return maxContentW(a, id, true)
}

// maxContentW is itemMaxContentW. `probed` says the caller laid the subtree out
// at maxFlexMeasureWidth first, which is what makes reading a block container's
// placed words safe; without it a box that blockifies its children is still
// wrapped at whatever width an earlier pass gave it and its words would measure
// as its min-content.
func maxContentW(a *Arena, id ObjectID, probed bool) float32 {
	obj := a.Get(id)
	if !probed && blockifiesChildren(obj.Style) {
		return 0
	}
	origin := obj.X + obj.BorderLeft + obj.PaddingLeft
	right := origin
	var walk func(ObjectID, float32) float32
	walk = func(cid ObjectID, trail float32) float32 {
		run := float32(0)
		for kid := a.Get(cid).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
				continue
			}
			lead := flexMargin(k.Style.MarginLeft) + k.BorderLeft + k.PaddingLeft
			trailBox := k.PaddingRight + k.BorderRight + flexMargin(k.Style.MarginRight)
			if k.Node != nil && k.Node.Type == 2 {
				// Max-content is the words themselves on one line, so the running
				// sum of their own widths is the measure. Reading a placed run's
				// right edge instead takes in where an earlier pass put it, and a
				// centred label sits far right of the box's origin: the button that
				// way measures its own offset as its content.
				run += k.W
				if w := origin + trail + run; w > right {
					right = w
				}
				continue
			}
			if isBlock(k) && !atomicInlineLevel(k.Style) {
				// A block sibling ends the inline line the run so far describes.
				run = 0
				// A nested flex or grid container sizes its own content, so use
				// its width rather than walking into it.
				if blockifiesChildren(k.Style) {
					if w := origin + trail + lead + k.W + trailBox; w > right {
						right = w
					}
					continue
				}
				// A child that brings a width of its own to the line - a fixed-size
				// figure, a min-width box - is a line of its own as far as
				// max-content goes, even when the text inside it is narrower.
				if own := declaredOuterW(k.Style); own >= 0 {
					if w := origin + trail + lead + own + trailBox; w > right {
						right = w
					}
				}
				// The words inside a block child sit after its leading box, and the
				// box closes after its trailing one, so both flanks belong to the
				// measure. Counting only the trailing side left a nav cell holding a
				// padded link 32px short of its own padding and wrapped its label.
				before := right
				walk(kid, trail+lead)
				if right > before {
					if w := right + trailBox; w > right {
						right = w
					}
				}
				continue
			}
			// An inline-level child - a plain inline element or an inline-block -
			// goes on the same line as its siblings, and its own box flanks count
			// as much as a word's. Measuring only the glyphs inside left a row of
			// tag pills narrower than their own padding and the text after them
			// painted straight through the labels.
			run += lead + walk(kid, trail+run+lead) + trailBox
			if w := origin + trail + run; w > right {
				right = w
			}
		}
		return run
	}
	walk(id, 0)
	w := right - origin
	if w < 0 {
		w = 0
	}
	return w
}

// clearInlineLaidOut drops the inline-laid-out flag down an item's subtree so a
// re-run of the inline pass re-wraps it. It does not descend into a nested flex
// or grid container: that container re-lays its own items, and clearing the
// words it placed would make the outer pass allocate a second set.
func clearInlineLaidOut(a *Arena, id ObjectID) {
	obj := a.Get(id)
	obj.flags &^= flagInlineLaidOut
	if blockifiesChildren(obj.Style) {
		return
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		clearInlineLaidOut(a, kid)
	}
}

// reflowInlineWords re-wraps the word objects that flex max-content measurement
// left on a single line into the item's final, shrunk width - the way a browser
// re-lays out a shrunk flex item's text instead of overflowing it. It reports
// false, leaving the caller to shift the content as before, for anything it must
// not touch: mixed block content or unbreakable white-space. The return on
// success is the content height of the wrapped result.
func reflowInlineWords(a *Arena, id ObjectID, finalW, baseX, baseY float32) (float32, bool) {
	if finalW <= 0 {
		return 0, false
	}
	var words []*Object
	supported := true
	var walk func(ObjectID)
	walk = func(cid ObjectID) {
		for kid := a.Get(cid).FirstKid; kid != 0 && supported; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if k.Node != nil && k.Node.Type == 2 {
				switch k.Style.WhiteSpace {
				case style.WhiteSpaceNowrap, style.WhiteSpacePre, style.WhiteSpacePrewrite, style.WhiteSpacePreline:
					supported = false
					return
				}
				words = append(words, k)
				continue
			}
			if isBlock(k) {
				supported = false
				return
			}
			walk(kid)
		}
	}
	walk(id)
	if !supported || len(words) == 0 {
		return 0, false
	}
	type reLine struct {
		from, to int
		h        float32
	}
	var lines []reLine
	cur := reLine{from: 0}
	x := float32(0)
	for i, wd := range words {
		if wd.W <= 0 {
			continue
		}
		leadingSpace := x == 0 && isSpaceWord(wd.Node.DataContent)
		if x > 0 && x+wd.W > finalW {
			cur.to = i
			lines = append(lines, cur)
			cur = reLine{from: i}
			x = 0
			leadingSpace = isSpaceWord(wd.Node.DataContent)
		}
		lh, _ := runHeights(a.Metrics, wd.Style.FontSize, wd.Style.FontSlot(), wd.Style.LineHeight)
		if lh > cur.h {
			cur.h = lh
		}
		if !leadingSpace {
			x += wd.W
		}
	}
	cur.to = len(words)
	lines = append(lines, cur)

	y := baseY
	total := float32(0)
	for _, ln := range lines {
		xx := baseX
		for _, wd := range words[ln.from:ln.to] {
			wd.X = xx
			wd.Y = y + (ln.h-wd.H)/2
			if !(xx == baseX && isSpaceWord(wd.Node.DataContent)) {
				xx += wd.W
			}
		}
		y += ln.h
		total += ln.h
	}
	return total, true
}

// shiftInlineContent moves the inline content of a subtree by a delta. Block
// descendants are skipped rather than shifted: block layout positions them from
// their parent's content box, so they already sit where they belong.
func shiftInlineContent(a *Arena, id ObjectID, dx, dy float32) {
	if dx == 0 && dy == 0 {
		return
	}
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if isBlock(k) {
			shiftInlineContent(a, kid, dx, dy)
			continue
		}
		k.X += dx
		k.Y += dy
		shiftInlineContent(a, kid, dx, dy)
	}
}

// replacedSize reports the content size a replaced element brings with it: an
// input control, or an image. These are drawn by the control or the decoded
// pixels rather than laid out from text content, so their `auto` size has to
// come from outside the box model.
func replacedSize(a *Arena, obj *Object, containingW float32) (w, h float32, ok bool) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() {
		return 0, 0, false
	}
	if obj.Node.Data == "img" {
		return imgContentSize(a, obj, containingW)
	}
	if obj.Node.Data != "input" && obj.Node.Data != "textarea" {
		return 0, 0, false
	}
	if obj.Node.Data == "textarea" {
		w, h = 154, 16
		if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
			h = lh
		}
		if rows, err := strconv.Atoi(obj.Node.GetAttribute("rows")); err == nil && rows > 1 {
			h *= float32(rows)
		}
		return w, h, true
	}
	switch strings.ToLower(obj.Node.GetAttribute("type")) {
	case "checkbox", "radio":
		return 13, 13, true
	case "", "text", "password", "email", "tel", "url", "search", "number":
	default:
		return 0, 0, false
	}
	w, h = 154, 16
	if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
		h = lh
	}
	return w, h, true
}

// imgContentSize resolves an image box's content size the way CSS replaced
// elements do: an explicit CSS length wins, then the width/height attributes,
// then the decoded image's intrinsic size. When only one axis is pinned from an
// attribute or the intrinsic size, the other follows the intrinsic aspect ratio
// so an image constrained to one dimension keeps its shape.
func imgContentSize(a *Arena, obj *Object, containingW float32) (w, h float32, ok bool) {
	s := obj.Style
	natW, natH := float32(0), float32(0)
	if a.NaturalSizes != nil && obj.Node != nil {
		if ns, found := a.NaturalSizes[obj.Node.ID]; found {
			natW, natH = ns.W, ns.H
		}
	}
	attrW := attrPx(obj.Node.GetAttribute("width"))
	attrH := attrPx(obj.Node.GetAttribute("height"))

	ratio := float32(0)
	if natW > 0 && natH > 0 {
		ratio = natH / natW
	} else if attrW > 0 && attrH > 0 {
		ratio = attrH / attrW
	}

	w, haveW := float32(0), false
	if v := resolvePctLength(s.Width, containingW); v >= 0 {
		w, haveW = v, true
	} else if attrW > 0 {
		w, haveW = attrW, true
	} else if natW > 0 {
		w, haveW = natW, true
	}
	h, haveH := float32(0), false
	if s.Height >= 0 {
		h, haveH = resolvePctLength(s.Height, containingW), true
	} else if attrH > 0 {
		h, haveH = attrH, true
	} else if natH > 0 {
		h, haveH = natH, true
	}

	if haveW && !haveH && ratio > 0 {
		h, haveH = w*ratio, true
	} else if haveH && !haveW && ratio > 0 {
		w, haveW = h/ratio, true
	}
	if !haveW || !haveH {
		return 0, 0, false
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	// Responsive images. `img { max-width: 100% }` is the single most common way
	// a page keeps an image inside its column, and without clamping here the box
	// keeps its intrinsic width and overflows into its neighbours. When the width
	// is clamped and the height was derived from the image (no explicit CSS
	// height), the height follows the aspect ratio so the picture stays undistorted.
	if clamped := clampReplaced(w, resolvePctLength(s.MinWidth, containingW), resolvePctLength(s.MaxWidth, containingW)); clamped != w {
		w = clamped
		if s.Height < 0 && ratio > 0 {
			h = w * ratio
		}
	}
	h = clampReplaced(h, resolvePctLength(s.MinHeight, containingW), resolvePctLength(s.MaxHeight, containingW))
	return w, h, true
}

// clampReplaced bounds a replaced-element dimension to its min/max, where a max
// below 0 and a min at 0 mean "unconstrained" (the resolved-style sentinels).
func clampReplaced(v, minW, maxW float32) float32 {
	if minW > 0 && v < minW {
		v = minW
	}
	if maxW >= 0 && v > maxW {
		v = maxW
	}
	return v
}

// attrPx reads a presentational length attribute as CSS pixels. A missing,
// non-numeric, or non-positive value is 0, meaning "unspecified".
func attrPx(v string) float32 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if i := strings.IndexAny(v, " \t"); i >= 0 {
		v = v[:i]
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil || f <= 0 {
		return 0
	}
	return float32(f)
}

// resolveBoxSizes turns one box's declared sizes into the margins, paddings,
// borders and content width and height the passes downstream read. containingH is
// the height this box's percentage heights resolve against, or -1 when the
// containing block has no definite height and CSS 2.1 §10.5 makes them auto.
func resolveBoxSizes(obj *Object, containingW, containingH float32) {
	s := obj.Style
	if s == nil {
		return
	}
	obj.PaddingTop = resolvePctLength(s.PaddingTop, containingW)
	obj.PaddingRight = resolvePctLength(s.PaddingRight, containingW)
	obj.PaddingBottom = resolvePctLength(s.PaddingBottom, containingW)
	obj.PaddingLeft = resolvePctLength(s.PaddingLeft, containingW)
	obj.BorderTop = s.BorderTopWidth
	obj.BorderRight = s.BorderRightWidth
	obj.BorderBottom = s.BorderBottomWidth
	obj.BorderLeft = s.BorderLeftWidth

	innerExtra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight

	var explicitW float32 = -1
	widthVal := resolvePctLength(s.Width, containingW)
	if widthVal >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			explicitW = widthVal - innerExtra
		} else {
			explicitW = widthVal
		}
		if explicitW < 0 {
			explicitW = 0
		}
	}

	// Margin auto: MarginAuto (-1) from the style means the CSS value was
	// "auto". It is a share of the space the box's used width leaves on the
	// line, which is not known until the width has been resolved and clamped,
	// so the first pass treats it as 0 and distributeAutoMargins revisits it
	// below. Vertical auto margins are always 0; the -1 sentinel in the style is
	// the keyword, not a length.
	leftAuto := s.MarginLeft == style.MarginAuto
	rightAuto := s.MarginRight == style.MarginAuto

	if leftAuto {
		obj.MarginLeft = 0
	} else {
		obj.MarginLeft = s.MarginLeft
	}
	if rightAuto {
		obj.MarginRight = 0
	} else {
		obj.MarginRight = s.MarginRight
	}
	obj.MarginTop = s.MarginTop
	obj.MarginBottom = s.MarginBottom
	if obj.MarginTop == style.MarginAuto {
		obj.MarginTop = 0
	}
	if obj.MarginBottom == style.MarginAuto {
		obj.MarginBottom = 0
	}

	// Width: -1 means auto. For block boxes, auto fills the containing block.
	if explicitW >= 0 {
		obj.W = explicitW
	} else if s.Display != style.DisplayInline {
		obj.W = containingW - obj.MarginLeft - obj.MarginRight - innerExtra
	}
	if obj.W < 0 {
		obj.W = 0
	}
	clampWidth(obj, containingW)
	if s.Display != style.DisplayInline {
		distributeAutoMargins(obj, leftAuto, rightAuto, containingW, innerExtra)
	}

	// Height: -1 means auto, resolved later by blockInto from children. A
	// percentage of an indefinite parent arrives back negative for the same
	// reason, so it falls into that same auto path.
	heightVal := resolvePctLength(s.Height, containingH)
	if heightVal >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			obj.H = heightVal - obj.PaddingTop - obj.PaddingBottom - obj.BorderTop - obj.BorderBottom
		} else {
			obj.H = heightVal
		}
	}
	if obj.H < 0 {
		obj.H = 0
	}
	clampHeight(obj, containingH)
}

// distributeAutoMargins gives the line an auto-width or clamped block leaves
// over to its auto horizontal margins (CSS 2.1 §10.3.3 rule 3).
//
// It runs after min-width and max-width have settled the used width, because
// §10.4 re-solves the whole equation with that clamped width: `max-width: 60rem`
// on an auto-width body is what leaves 320px for `margin: auto` to split, which
// is how a centered page column works.
func distributeAutoMargins(obj *Object, leftAuto, rightAuto bool, containingW, innerExtra float32) {
	if !leftAuto && !rightAuto {
		return
	}
	remaining := containingW - obj.W - innerExtra
	if remaining < 0 {
		remaining = 0
	}
	if leftAuto && rightAuto {
		obj.MarginLeft = remaining / 2
		obj.MarginRight = remaining / 2
	} else if leftAuto {
		obj.MarginLeft = remaining
	} else {
		obj.MarginRight = remaining
	}
}

// centerBlockChildLead is the left offset a block child of <center> takes to sit
// in the middle of its parent's content box, the same way `margin: 0 auto`
// centres a fixed-width box. It is gated on the tag rather than on the
// inherited text-align because that is what the two mean: a plain
// `text-align: center` container centres its text, not its full-width children.
func centerBlockChildLead(parent, kid *Object, contentW float32) float32 {
	if parent.Node == nil || parent.Node.Data != "center" {
		return 0
	}
	// A child that already carries a declared horizontal margin, or whose auto
	// margins resolved to a share of the leftover, has been placed once.
	if kid.MarginLeft != 0 || kid.MarginRight != 0 {
		return 0
	}
	extra := kid.PaddingLeft + kid.PaddingRight + kid.BorderLeft + kid.BorderRight
	remaining := contentW - kid.W - extra
	if remaining <= 0 {
		return 0
	}
	return remaining / 2
}

// clampWidth applies min-width and max-width to a box whose width is otherwise
// settled. Both constraints speak the same box language as width itself, so
// under border-sizing they are measured from the border box inward.
func clampWidth(obj *Object, containingW float32) {
	s := obj.Style
	if s == nil {
		return
	}
	minW := resolvePctLength(s.MinWidth, containingW)
	maxW := resolvePctLength(s.MaxWidth, containingW)
	if s.BoxSizing == style.BoxSizingBorderBox {
		extra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight
		if minW >= 0 {
			minW -= extra
		}
		if maxW >= 0 {
			maxW -= extra
		}
	}
	if minW > 0 && obj.W < minW {
		obj.W = minW
	}
	// A negative max is either the unset sentinel or a value the clamps above
	// drove below zero; both mean the content width is zero at most.
	if maxW >= 0 && obj.W > maxW {
		obj.W = maxW
	}
	if obj.W < 0 {
		obj.W = 0
	}
}

// clampHeight applies min-height and max-height. min-height raises a box whose
// content fell short of it; max-height shortens the box and leaves the content
// over flowing, which is what the box's own overflow then clips.
func clampHeight(obj *Object, containingH float32) {
	s := obj.Style
	if s == nil {
		return
	}
	minH := resolvePctLength(s.MinHeight, containingH)
	maxH := resolvePctLength(s.MaxHeight, containingH)
	if s.BoxSizing == style.BoxSizingBorderBox {
		extra := obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom
		if minH >= 0 {
			minH -= extra
		}
		if maxH >= 0 {
			maxH -= extra
		}
	}
	if minH > 0 && obj.H < minH {
		obj.H = minH
	}
	if maxH >= 0 && obj.H > maxH {
		obj.H = maxH
	}
	if obj.H < 0 {
		obj.H = 0
	}
}

// blockifiesChildren reports whether a container turns every child into a box of
// its own, so raw inline content never needs an anonymous block to hold it.
func blockifiesChildren(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.Display {
	case style.DisplayFlex, style.DisplayInlineFlex, style.DisplayGrid, style.DisplayInlineGrid:
		return true
	}
	return false
}

// atomicInlineLevel reports a box that is a block formatting root but still
// sits on a line of text: it breaks its own content into lines but not the
// parent's, so max-content keeps measuring it in the parent's inline run.
func atomicInlineLevel(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.Display {
	case style.DisplayInlineBlock, style.DisplayInlineFlex, style.DisplayInlineGrid:
		return true
	}
	return false
}

func isBlock(obj *Object) bool {
	if obj.Style == nil {
		return false
	}
	switch obj.Style.Display {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayTable,
		style.DisplayFlex, style.DisplayInlineFlex, style.DisplayInlineBlock,
		style.DisplayGrid, style.DisplayInlineGrid,
		style.DisplayTableRow, style.DisplayTableCell, style.DisplayTableCaption,
		style.DisplayTableRowGroup, style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup:
		return true
	}
	return false
}

// flowsInlineContent reports whether a container has inline content of its own
// to flow: text that renders, an inline-level element, or a forced break.
// Whitespace-only text does not count, because between two inline-blocks it is
// only a space and the row cursor lays that pair out correctly on its own.
func flowsInlineContent(a *Arena, obj *Object) bool {
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
			continue
		}
		if k.Node != nil && k.Node.Type == 2 {
			if strings.TrimSpace(k.Node.DataContent) != "" {
				return true
			}
			continue
		}
		if !isBlock(k) {
			return true
		}
	}
	return false
}

// holdsBlockContent reports whether an inline-level box has block-level content
// below it. CSS splits such a box around that content, leaving a block-level
// fragment where the inline box used to be, so block flow has to give it a slot.
func holdsBlockContent(a *Arena, id ObjectID) bool {
	obj := a.Get(id)
	if obj.Node == nil || obj.Node.Type != 1 || isBlock(obj) {
		return false
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
			continue
		}
		if isFlowBlock(k) || holdsBlockContent(a, kid) {
			return true
		}
	}
	return false
}

// isOutOfFlow reports the positions that take a box out of its parent's flow.
// The positioning pass places such a box against its containing block instead,
// so it contributes neither to the container's height nor to its collapsing.
func isOutOfFlow(s *style.ComputedStyle) bool {
	return s.Position == style.PositionAbsolute || s.Position == style.PositionFixed
}

// isFlowFloat reports a box thrown to one side of its container. A float is out
// of the block flow - it takes no vertical slot - but unlike an absolutely
// positioned box it stays in the document, so it still paints where the block
// pass puts it.
func isFlowFloat(s *style.ComputedStyle) bool {
	return s != nil && (s.Float == style.FloatLeft || s.Float == style.FloatRight)
}

// startsNewFC reports whether obj is a box a descendant's margin cannot collapse
// through. Only a block-level box in the normal flow that does not clip absorbs a
// child's margin into its own edge, so flex and grid items, inline-blocks, table
// parts, and anything with a visible-elsewhere overflow all stop the chain.
func startsNewFC(a *Arena, obj *Object) bool {
	s := obj.Style
	if s == nil {
		return true
	}
	switch s.Display {
	case style.DisplayBlock, style.DisplayListItem:
	default:
		return true
	}
	if s.Overflow != style.OverflowVisible || s.OverflowX != style.OverflowVisible || s.OverflowY != style.OverflowVisible {
		return true
	}
	if p := a.Get(obj.Parent); p.Style != nil {
		switch p.Style.Display {
		case style.DisplayFlex, style.DisplayInlineFlex, style.DisplayGrid, style.DisplayInlineGrid:
			return true
		}
	}
	return false
}

// edgeDeclared reports whether a box edge is there at all. A carry walk reaches
// descendants block layout has not resolved yet, and a percentage padding is not
// the zero its box still holds: it has to count as an edge that stops a margin,
// because the width it resolves against is not known at this point either.
func edgeDeclared(v float32) bool { return v > 0 || v <= -2 }

// collapsesThroughTop reports whether obj's first child's top margin reaches all
// the way to obj's own top edge, which needs a new formatting context to be
// absent and the top edge to carry no border or padding.
func collapsesThroughTop(a *Arena, obj *Object) bool {
	if obj.Style == nil || startsNewFC(a, obj) {
		return false
	}
	return !edgeDeclared(obj.Style.BorderTopWidth) && !edgeDeclared(obj.Style.PaddingTop)
}

// collapsesThroughBottom is the mirror image, with one extra condition: a box of
// a definite height keeps its last child's margin inside as content that
// overflows, so nothing carries out of the bottom edge.
func collapsesThroughBottom(a *Arena, obj *Object) bool {
	if obj.Style == nil || startsNewFC(a, obj) {
		return false
	}
	return obj.Style.Height < 0 &&
		!edgeDeclared(obj.Style.BorderBottomWidth) && !edgeDeclared(obj.Style.PaddingBottom)
}

// firstCollapsingChild is obj's first child that takes a vertical slot of its
// own, when that child is a block whose top margin can collapse with the
// container, and 0 otherwise. Whitespace between blocks generates no box and is
// stepped over; real inline content ends the search without a match, because it
// gets an anonymous block of its own and an anonymous block has no margin to
// carry.
func firstCollapsingChild(a *Arena, obj *Object) ObjectID {
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if outOfFlowForCollapse(k) || isWhitespaceText(k) {
			continue
		}
		if !isFlowBlock(k) {
			return 0
		}
		return kid
	}
	return 0
}

// lastCollapsingChild is firstCollapsingChild read from the other end.
func lastCollapsingChild(a *Arena, obj *Object) ObjectID {
	var last ObjectID
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if outOfFlowForCollapse(k) || isWhitespaceText(k) {
			continue
		}
		if !isFlowBlock(k) {
			return 0
		}
		last = kid
	}
	return last
}

// outOfFlowForCollapse reports the children that take no part in their
// container's flow, and so cannot collapse with it. display:none generates no box
// at all, which the caller treats the same way.
func outOfFlowForCollapse(k *Object) bool {
	if k.Style == nil || k.Style.Display == style.DisplayNone {
		return true
	}
	return isOutOfFlow(k.Style)
}

// carryMargin reads a vertical margin straight off the style rather than off the
// box, because a carry walk looks at descendants that resolveBoxSizes has not
// visited yet. `auto` and the percentage sentinel are not lengths: an auto
// vertical margin is zero, and a percentage one is not supported yet. Negative
// margins share the sentinel's encoding, so they read as zero here too and only
// take effect where the box itself is placed.
func carryMargin(v float32) float32 {
	if v <= style.MarginAuto {
		return 0
	}
	return v
}

// carryTop is the margin that ends up above id's border box once its first
// child's has collapsed up through a transparent top edge. Containers place a box
// with this instead of with the box's own margin-top, which is what stops a
// heading's margin from being added to its grandparent's previous sibling rather
// than collapsing with it.
func carryTop(a *Arena, id ObjectID) float32 {
	obj := a.Get(id)
	if obj.Style == nil {
		return 0
	}
	m := carryMargin(obj.Style.MarginTop)
	if !collapsesThroughTop(a, obj) {
		return m
	}
	if kid := firstCollapsingChild(a, obj); kid != 0 {
		m = collapseMargin(m, carryTop(a, kid))
	}
	return m
}

// carryBottom is carryTop for the bottom edge: the margin a box presents to its
// next sibling, which is its own once the last child's has collapsed down
// through it.
func carryBottom(a *Arena, id ObjectID) float32 {
	obj := a.Get(id)
	if obj.Style == nil {
		return 0
	}
	m := carryMargin(obj.Style.MarginBottom)
	if !collapsesThroughBottom(a, obj) {
		return m
	}
	if kid := lastCollapsingChild(a, obj); kid != 0 {
		m = collapseMargin(m, carryBottom(a, kid))
	}
	return m
}

func collapseMargin(a, b float32) float32 {
	if a >= 0 && b >= 0 {
		return maxF(a, b)
	}
	if a < 0 && b < 0 {
		return minF(a, b)
	}
	return a + b
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// resolvePctLength resolves a style length value that may be a percentage
// sentinel (encoded as -2-pct) against the containing width. Non-percentage
// values pass through unchanged.
func resolvePctLength(val, containingW float32) float32 {
	if isPctLength(val) {
		pct := -2 - val
		return pct * containingW / 100
	}
	return val
}

// isPctLength reports whether a style length carries the percentage sentinel
// rather than a length or the -1 auto keyword.
func isPctLength(val float32) bool {
	return val <= -2 && val >= -102
}

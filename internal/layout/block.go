package layout

import (
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
func Block(a *Arena, root ObjectID, viewportW float32) {
	blockInto(a, root, viewportW)
}

func blockInto(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	var replacedH float32
	if obj.Style != nil {
		resolveBoxSizes(obj, containingW)
		if obj.Style.Display == style.DisplayNone {
			obj.W = 0
			obj.H = 0
			return 0
		}
		if w, h, ok := replacedSize(a, obj); ok {
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
					resolveBoxSizes(k, contentW)
					k = a.Get(kid)
					k.StaticX = contentX + k.MarginLeft
					k.StaticY = contentY + y
					k.flags |= flagOutOfFlow
					continue
				}
				if k.Style.Display == style.DisplayInlineBlock {
					// Inline-level, so it takes no vertical slot of its own: it
					// joins the row in progress, or starts a new one after the
					// content width runs out.
					resolveBoxSizes(k, contentW)
					k = a.Get(kid)
					extra := k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
					// An auto width shrinks to the content instead of filling the
					// container, which is what puts a row of buttons side by side.
					// The measure is the extent the words take at the container's
					// own width, read back off the placed words rather than off the
					// box, because a text-align would otherwise be baked into it.
					shrink := resolvePctLength(k.Style.Width, contentW) < 0
					var srcX, measureX, measureY, contentExtent float32
					measured := false
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
						blockInto(a, kid, contentW)
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
				resolveBoxSizes(k, contentW)
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
				k.X = contentX + k.MarginLeft
				k.Y = contentY + y + topMargin
				blockInto(a, kid, contentW)
				// Run inline layout immediately so the element's height is known
				// before positioning the next sibling.
				inlineInto(a, kid)
				// Re-fetch k: inlineInto may have allocated word objects, growing
				// the arena slice and invalidating the pointer captured above.
				k = a.Get(kid)
				y = (k.Y - contentY) + k.BorderH()
				prevBottomMargin = carryBottom(a, kid)
			}
		}
		closeRow()
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
	// are added back by ContentRect/BorderRect at the edges.
	if obj.Style != nil && obj.Style.Height < 0 {
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
	clampHeight(obj, containingW)
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
	}
	var items []item
	gap := obj.Style.Gap
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
		items = append(items, item{id: kid, obj: k, grow: k.Style.FlexGrow})
	}
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
		items[i].basis = clampFlexBasis(a.Get(items[i].id), basis, contentW)
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
		inlineInto(a, items[i].id)
		it = a.Get(items[i].id)
		srcX, right, ok := inlineExtent(a, items[i].id)
		contentWidth := float32(0)
		if ok {
			contentWidth = right - srcX
		}
		it.W = contentWidth
		// Basis is a border-box width.
		items[i].basis = contentWidth + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		items[i].measured = true
		items[i].srcX = srcX
		items[i].contentW = contentWidth
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
			for i := range items {
				if lineOf[i] == l && items[i].grow > 0 {
					items[i].basis += free * items[i].grow / growSum
				}
			}
		} else if free < 0 && shrinkWeight > 0 {
			for i := range items {
				if lineOf[i] != l {
					continue
				}
				share := free * (items[i].basis * a.Get(items[i].id).Style.FlexShrink) / shrinkWeight
				if w := items[i].basis + share; w > 0 {
					items[i].basis = w
				} else {
					items[i].basis = 0
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
				// The words are still where the measurement pass left them. Move them
				// to the item's own content origin, re-applying the item's alignment
				// against the width it ended up with.
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
			} else {
				containW := it.W
				if items[i].pct > 0 {
					// The item's width is a share of the container, so re-resolving
					// it inside block layout needs that share scaled back: the
					// basis is the width the percentage already produced.
					containW = items[i].basis * 100 / items[i].pct
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
	gap := obj.Style.Gap

	type item struct {
		id            ObjectID
		basis         float32
		grow          float32
		mTop, mBottom float32
		mLeft, mRight float32
		declared      bool
		crossW        float32
	}
	var items []item
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
		}
		// flex-basis and height both size the main axis here, as border-box
		// extents; a content-box declaration needs the padding and borders added.
		var declared float32 = -1
		if s.FlexBasis >= 0 {
			declared = s.FlexBasis
		} else if s.Height >= 0 {
			declared = resolvePctLength(s.Height, contentW)
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
	n := len(items)
	if n == 0 {
		return 0
	}
	if reverse := obj.Style.FlexDirection == style.FlexColumnReverse; reverse {
		for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
			items[l], items[r] = items[r], items[l]
		}
	}

	// Measure pass: lay each item out across the full cross size, which is what
	// the final layout gives it once its own horizontal margins are gone. The
	// natural main extent is then the height that content produced.
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
		it := a.Get(items[i].id)
		it.X = contentX + items[i].mLeft
		it.Y = contentY
		blockInto(a, items[i].id, contentW)
		it = a.Get(items[i].id)
		inlineInto(a, items[i].id)
		it = a.Get(items[i].id)
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
		it.Y = cursor + items[i].mTop
		if !items[i].declared && it.Style.Height < 0 && it.Style.FlexGrow > 0 {
			// Only a grown item outgrows the height its content measured at; an
			// auto item's basis is that measurement, so it keeps it.
			h := items[i].basis - it.PaddingTop - it.PaddingBottom - it.BorderTop - it.BorderBottom
			if h < 0 {
				h = 0
			}
			it.H = h
		}
		if dy := it.Y - contentY; dy != 0 {
			shiftSubtree(a, items[i].id, 0, dy)
			it = a.Get(items[i].id)
		}
		align := it.Style.AlignSelf
		if align == "" {
			align = alignItems
		}
		if it.Style.Width >= 0 && contentW > items[i].crossW {
			switch align {
			case "center":
				it.X = contentX + (contentW-items[i].crossW)/2 + it.MarginLeft
			case "flex-end":
				it.X = contentX + contentW - items[i].crossW - it.MarginRight
			}
			shiftSubtree(a, items[i].id, it.X-(contentX+items[i].mLeft), 0)
			it = a.Get(items[i].id)
		}
		if bottom := (it.Y - contentY) + items[i].mBottom + it.BorderH(); bottom > maxBottom {
			maxBottom = bottom
		}
		cursor = it.Y + it.BorderH() + items[i].mBottom + trackGap
	}
	return maxBottom
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

// replacedSize reports the content size an input control brings with it. A text
// field and a tick box are drawn by the control rather than laid out from
// content, so their `auto` size has to come from outside the box model.
func replacedSize(a *Arena, obj *Object) (w, h float32, ok bool) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() || obj.Node.Data != "input" {
		return 0, 0, false
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

func resolveBoxSizes(obj *Object, containingW float32) {
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
	// "auto". For horizontal margins on a block with an explicit width, auto
	// distributes the remaining space (CSS 2.1 §10.3.3). For auto width or
	// vertical margins, auto margins collapse to 0.
	leftAuto := s.MarginLeft == style.MarginAuto
	rightAuto := s.MarginRight == style.MarginAuto

	if explicitW >= 0 && (leftAuto || rightAuto) {
		// Remaining space the two horizontal margins share.
		remaining := containingW - explicitW - innerExtra
		if remaining < 0 {
			remaining = 0
		}
		if leftAuto && rightAuto {
			obj.MarginLeft = remaining / 2
			obj.MarginRight = remaining / 2
		} else if leftAuto {
			obj.MarginLeft = remaining
			obj.MarginRight = s.MarginRight
		} else {
			obj.MarginRight = remaining
			obj.MarginLeft = s.MarginLeft
		}
	} else {
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
	}
	obj.MarginTop = s.MarginTop
	obj.MarginBottom = s.MarginBottom
	// A vertical `auto` margin is zero; the sentinel -1 in the style is the
	// keyword, not a length.
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

	// Height: -1 means auto, resolved later by blockInto from children.
	heightVal := resolvePctLength(s.Height, containingW)
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
	clampHeight(obj, containingW)
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
func clampHeight(obj *Object, containingW float32) {
	s := obj.Style
	if s == nil {
		return
	}
	minH := resolvePctLength(s.MinHeight, containingW)
	maxH := resolvePctLength(s.MaxHeight, containingW)
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
	if val <= -2 && val >= -102 {
		pct := -2 - val
		return pct * containingW / 100
	}
	return val
}

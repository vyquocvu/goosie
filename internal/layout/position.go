package layout

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/style"
)

// containingBlock is the rect an absolutely positioned box resolves its insets,
// width and height against: the padding box of the nearest positioned ancestor.
type containingBlock struct {
	x0, y0, w, h float32
}

// Positioning runs the positioning pass over an arena whose block and inline
// passes have finished.
//
// A relatively positioned box keeps its layout position and is shifted out of
// flow, taking its subtree with it and leaving a hole behind. An absolutely or
// fixed positioned box was already skipped by block layout, so this pass lays it
// out against its containing block. The walk is top-down because an absolute
// box's position depends on its containing block being final, and a relative
// shift has to move the containing block of the absolute descendants inside it.
//
// A fixed box's containing block is the initial one, which is why the pass takes
// the viewport rect: `bottom` and a percentage height cannot resolve without a
// viewport height. With viewportH of 0 the initial containing block degenerates
// to the viewport's top edge, so a fixed box pinned by `bottom` lands above it.
func Positioning(a *Arena, root ObjectID, viewportW, viewportH float32) {
	icb := containingBlock{w: viewportW, h: viewportH}
	anchors := map[string]ObjectID{}
	for i := range a.Objects {
		obj := &a.Objects[i]
		if obj.Node != nil && obj.Node.Element() && obj.Style != nil && obj.Style.AnchorName != "" {
			anchors[obj.Style.AnchorName] = ObjectID(i)
		}
	}
	positionSubtree(a, root, icb, icb, anchors)
}

func positionSubtree(a *Arena, id ObjectID, cb, icb containingBlock, anchors map[string]ObjectID) {
	obj := a.Get(id)
	s := obj.Style
	if s != nil && s.Display == style.DisplayNone {
		return
	}
	// Text nodes inherit their parent's style wholesale, including position, so
	// only elements can position themselves. The document root has no style at
	// all and only carries the walk.
	element := obj.Node == nil || obj.Node.Element()
	if s != nil && element {
		switch s.Position {
		case style.PositionRelative:
			rel := containingBlock{cb.x0, cb.y0, cb.w, cb.h}
			if parent := a.Get(obj.Parent); parent != nil && parent.Style != nil {
				x0, y0, x1, y1 := parent.ContentRect()
				rel = containingBlock{x0, y0, x1 - x0, y1 - y0}
			}
			dx := -resolvePctLength(s.Right, rel.w)
			if s.HasLeft {
				dx = resolvePctLength(s.Left, rel.w)
			}
			dy := -resolvePctLength(s.Bottom, rel.h)
			if s.HasTop {
				dy = resolvePctLength(s.Top, rel.h)
			}
			shiftSubtree(a, id, dx, dy)
			obj = a.Get(id)
		case style.PositionAbsolute:
			placeOutOfFlow(a, id, cb, anchors)
			obj = a.Get(id)
		case style.PositionFixed:
			placeOutOfFlow(a, id, icb, anchors)
			obj = a.Get(id)
		}
	}

	childCB := cb
	if element && s != nil && s.Position != style.PositionStatic {
		childCB = containingBlock{
			x0: obj.X + obj.BorderLeft,
			y0: obj.Y + obj.BorderTop,
			w:  obj.W + obj.PaddingLeft + obj.PaddingRight,
			h:  obj.H + obj.PaddingTop + obj.PaddingBottom,
		}
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		positionSubtree(a, kid, childCB, icb, anchors)
	}
}

// placeOutOfFlow sizes and positions one out-of-flow box, then runs block and
// inline layout over its subtree. The box arrives with no geometry: block layout
// leaves out-of-flow children to this pass so they take no part in the flow. The
// caller hands over the containing block, which is the parent's for an absolute
// box and the viewport's for a fixed one.
func placeOutOfFlow(a *Arena, id ObjectID, cb containingBlock, anchors map[string]ObjectID) {
	obj := a.Get(id)
	s := obj.Style
	// The box's own layout is this pass's job: clear the mark block layout left
	// so the inline pass below runs over it.
	obj.flags &^= flagOutOfFlow
	// Margins, borders and padding percentages resolve against the containing
	// block's width, exactly as they do for an in-flow block. Heights resolve
	// against its height, and a containing block with no height of its own - an
	// unset viewport - leaves those percentages auto.
	sizeH := cb.h
	if sizeH <= 0 {
		sizeH = -1
	}
	resolveBoxSizes(a, obj, cb.w, sizeH)
	obj = a.Get(id)
	extra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight
	vExtra := obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom
	insetL := resolvePctLength(s.Left, cb.w)
	insetR := resolvePctLength(s.Right, cb.w)
	insetT := resolvePctLength(s.Top, cb.h)
	insetB := resolvePctLength(s.Bottom, cb.h)

	// The CSS rule for over-constrained boxes: in a left-to-right writing mode
	// `right` is the value that gets ignored.
	x := obj.StaticX
	w := obj.W
	// `auto` is the only width that takes shrink-to-fit sizing. A percentage
	// width is encoded as a negative sentinel, and resolveBoxSizes has already
	// turned it into a concrete content width on obj.W (box-sizing included), so
	// testing `s.Width < 0` here threw `width:50%` away as if it were auto.
	shrinkW := resolvePctLength(s.Width, cb.w) < 0
	// stretched marks the one auto width that is not content-driven: both insets
	// pinned, so the box takes whatever the two leave between them.
	stretched := false
	if !shrinkW {
		switch {
		case s.HasLeft:
			x = cb.x0 + obj.MarginLeft + insetL
		case s.HasRight:
			x = cb.x0 + cb.w - obj.MarginRight - insetR - (w + extra)
		}
	} else if s.HasLeft && s.HasRight {
		w = cb.w - insetL - insetR - extra - obj.MarginLeft - obj.MarginRight
		if w < 0 {
			w = 0
		}
		shrinkW = false
		stretched = true
		x = cb.x0 + obj.MarginLeft + insetL
	} else if s.HasLeft {
		x = cb.x0 + obj.MarginLeft + insetL
	} else if s.HasRight {
		// Right is pinned and the width is content-driven, so the left edge is
		// only known once the content has been measured; place at the static
		// position and let the measure below correct it.
		x = obj.StaticX
	}

	y := obj.StaticY
	h := obj.H
	explicitH := resolvePctLength(s.Height, sizeH) >= 0
	if explicitH {
		switch {
		case s.HasTop:
			y = cb.y0 + obj.MarginTop + insetT
		case s.HasBottom:
			y = cb.y0 + cb.h - obj.MarginBottom - insetB - (h + vExtra)
		}
	} else if s.HasTop && s.HasBottom {
		h = cb.h - insetT - insetB - vExtra - obj.MarginTop - obj.MarginBottom
		if h < 0 {
			h = 0
		}
		explicitH = true
		y = cb.y0 + obj.MarginTop + insetT
	} else if s.HasTop {
		y = cb.y0 + obj.MarginTop + insetT
	}

	if s.WidthAnchor != "" {
		if asz, ok := resolveAnchorSize(a, s.WidthAnchor, s.PositionAnchor, anchors, false); ok {
			w = asz - extra
			if w < 0 {
				w = 0
			}
			shrinkW = false
		}
	}
	if s.HeightAnchor != "" {
		if asz, ok := resolveAnchorSize(a, s.HeightAnchor, s.PositionAnchor, anchors, true); ok {
			h = asz - vExtra
			if h < 0 {
				h = 0
			}
			explicitH = true
		}
	}

	if s.TopAnchor != "" {
		if ay, ok := resolveAnchor(a, s.TopAnchor, s.PositionAnchor, anchors, true); ok {
			y = ay + obj.MarginTop
		}
	}
	if s.BottomAnchor != "" {
		if ay, ok := resolveAnchor(a, s.BottomAnchor, s.PositionAnchor, anchors, true); ok {
			if explicitH {
				y = ay - (h + vExtra) - obj.MarginBottom
			} else if s.TopAnchor != "" {
				h = ay - y - vExtra - obj.MarginBottom
				if h < 0 {
					h = 0
				}
				explicitH = true
			}
		}
	}
	if s.LeftAnchor != "" {
		if ax, ok := resolveAnchor(a, s.LeftAnchor, s.PositionAnchor, anchors, false); ok {
			x = ax + obj.MarginLeft
		}
	}
	if s.RightAnchor != "" {
		if ax, ok := resolveAnchor(a, s.RightAnchor, s.PositionAnchor, anchors, false); ok {
			if !shrinkW {
				x = ax - (w + extra) - obj.MarginRight
			} else if s.LeftAnchor != "" {
				w = ax - x - extra - obj.MarginRight
				if w < 0 {
					w = 0
				}
				shrinkW = false
			}
		}
	}

	obj.X, obj.Y = x, y
	obj.W = w
	measureW := w + obj.MarginLeft + obj.MarginRight + extra
	switch {
	case shrinkW:
		// Max-content sizing: lay the box out at a width no text can fill, read
		// back the extent its content took, and give the box that width.
		measureW = maxFlexMeasureWidth
	case !stretched:
		// The width is already resolved, so the measure width only has to be a
		// containing block the same resolution lands back on: handing it the
		// border-box width instead made a percentage grow by its own padding on
		// every pass, and carry its children out with it.
		if s.WidthAnchor != "" {
			measureW = w
		} else {
			measureW = cb.w
		}
	}
	blockInto(a, id, measureW)
	obj = a.Get(id)
	inlineInto(a, id)
	obj = a.Get(id)
	if s.WidthAnchor != "" {
		obj.W = w
	}
	if s.HeightAnchor != "" {
		obj.H = h
	}

	if shrinkW {
		srcX, right, ok := inlineExtent(a, id)
		if ok {
			contentW := right - srcX
			if contentW < 0 {
				contentW = 0
			}
			obj.W = contentW
			if s.HasRight {
				obj.X = cb.x0 + cb.w - obj.MarginRight - insetR - (contentW + extra)
			}
			// The measurement laid the words out inside a box as wide as the
			// measure width, so they have to be brought back to the content
			// origin the box now has. The width is the content's own extent, so
			// no alignment has anywhere to spread.
			shiftInlineContent(a, id, obj.X+obj.BorderLeft+obj.PaddingLeft-srcX, 0)
			obj = a.Get(id)
		} else {
			// No inline extent: the content is block-level (or there is no
			// content at all), so there are no words to read back. Fall back
			// to the block preferred width rather than keeping the probe
			// width, which stranded 10000px abspos boxes across pages (and
			// blew every overflow clip and document extent around them). An
			// empty box measures zero, which is its correct shrink-to-fit,
			// and min/max-width still constrain the result. The subtree keeps
			// its probe layout and spills visibly past the shrunk box instead
			// of rewrapping: a rewrap needs a second full layout pass, while
			// the spill matches overflow:visible, the common case.
			bw, _ := absposContentW(a, id)
			avail := cb.w - obj.MarginLeft - obj.MarginRight
			if s.HasLeft {
				avail -= insetL
			}
			if s.HasRight {
				avail -= insetR
			}
			if avail < 0 {
				avail = 0
			}
			if bw > avail {
				bw = avail
			}
			obj.W = clampFlexBasis(obj, bw, cb.w)
			if s.HasRight && !s.HasLeft {
				obj.X = cb.x0 + cb.w - obj.MarginRight - insetR - (obj.W + extra)
			}
			obj = a.Get(id)
		}
	}
	if explicitH {
		obj.H = h
	} else if s.HasBottom {
		shiftSubtree(a, id, 0, cb.y0+cb.h-obj.MarginBottom-insetB-obj.BorderH()-obj.Y)
		obj = a.Get(id)
	}
	// The children blockInto positioned sit relative to this box's origin, which
	// was final before the layout calls above, so nothing further is needed.
}

// absposContentW measures the max-content width of an abspos box's subtree,
// including words under block children. inlineExtent stops at blocks (by
// design: flex items with block content take the itemMaxContentW path), and
// itemMaxContentW only reads placed text runs and declared widths, so words
// wrapped inside a plain block child are invisible to both. This walks every
// in-flow descendant leaf with a measured width instead. Out-of-flow
// descendants contribute nothing, matching maxContentW.
func absposContentW(a *Arena, id ObjectID) (float32, bool) {
	var minX, maxX float32
	ok := false
	var walk func(ObjectID)
	walk = func(cid ObjectID) {
		for kid := a.Get(cid).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
				continue
			}
			if k.FirstKid == 0 {
				if k.W <= 0 {
					continue
				}
				x1 := k.X + k.W
				if !ok || k.X < minX {
					minX = k.X
				}
				if !ok || x1 > maxX {
					maxX = x1
				}
				ok = true
				continue
			}
			walk(kid)
		}
	}
	walk(id)
	if !ok || maxX <= minX {
		return 0, false
	}
	return maxX - minX, true
}

// shiftSubtree moves a box and everything inside it. Unlike shiftInlineContent
// this includes block descendants: their positions were computed from this box's
// origin, which has just moved.
func shiftSubtree(a *Arena, id ObjectID, dx, dy float32) {
	if dx == 0 && dy == 0 {
		return
	}
	obj := a.Get(id)
	obj.X += dx
	obj.Y += dy
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style != nil && k.Style.Position == style.PositionFixed {
			// A fixed box is anchored to the viewport rather than to this subtree,
			// so the shift that moves the subtree leaves it where it is.
			continue
		}
		shiftSubtree(a, kid, dx, dy)
	}
}

func resolveAnchor(a *Arena, expr, defaultAnchor string, anchors map[string]ObjectID, vertical bool) (float32, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	parts := strings.Split(expr, ",")
	first := strings.TrimSpace(parts[0])
	fallback := ""
	if len(parts) > 1 {
		fallback = strings.TrimSpace(parts[1])
	}
	tokens := strings.Fields(first)
	anchorName := defaultAnchor
	side := ""
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "--") {
			anchorName = tok
		} else {
			side = strings.ToLower(tok)
		}
	}
	ancID, ok := anchors[anchorName]
	if !ok {
		if fallback != "" {
			return parseFallbackLength(fallback), true
		}
		return 0, false
	}
	anc := a.Get(ancID)
	switch side {
	case "top":
		return anc.Y, true
	case "bottom":
		return anc.Y + anc.BorderH(), true
	case "left":
		return anc.X, true
	case "right":
		return anc.X + anc.BorderW(), true
	case "center":
		if vertical {
			return anc.Y + anc.BorderH()/2, true
		}
		return anc.X + anc.BorderW()/2, true
	case "start":
		if vertical {
			return anc.Y, true
		}
		return anc.X, true
	case "end":
		if vertical {
			return anc.Y + anc.BorderH(), true
		}
		return anc.X + anc.BorderW(), true
	}
	if fallback != "" {
		return parseFallbackLength(fallback), true
	}
	return 0, false
}

func resolveAnchorSize(a *Arena, expr, defaultAnchor string, anchors map[string]ObjectID, vertical bool) (float32, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return 0, false
	}
	parts := strings.Split(expr, ",")
	first := strings.TrimSpace(parts[0])
	fallback := ""
	if len(parts) > 1 {
		fallback = strings.TrimSpace(parts[1])
	}
	tokens := strings.Fields(first)
	anchorName := defaultAnchor
	axis := ""
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "--") {
			anchorName = tok
		} else {
			axis = strings.ToLower(tok)
		}
	}
	ancID, ok := anchors[anchorName]
	if !ok {
		if fallback != "" {
			return parseFallbackLength(fallback), true
		}
		return 0, false
	}
	anc := a.Get(ancID)
	switch axis {
	case "width":
		return anc.BorderW(), true
	case "height":
		return anc.BorderH(), true
	default:
		if vertical {
			return anc.BorderH(), true
		}
		return anc.BorderW(), true
	}
}

func parseFallbackLength(s string) float32 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "px")
	f, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0
	}
	return float32(f)
}

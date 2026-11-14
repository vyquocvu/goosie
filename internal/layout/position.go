package layout

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

type containingBlock struct {
	x0, y0, w, h float32
}

func Positioning(a *Arena, root ObjectID, viewportW, viewportH float32) {
	icb := containingBlock{w: viewportW, h: viewportH}
	anchors := map[string]ObjectID{}
	anchorNodes := map[dom.NodeID]ObjectID{}
	for i := range a.Objects {
		obj := &a.Objects[i]
		if obj.Node != nil && obj.Style != nil {
			anchorNodes[obj.Node.ID] = ObjectID(i)
		}
		if obj.Node != nil && obj.Node.Element() && obj.Style != nil && obj.Style.AnchorName != "" {
			anchors[obj.Style.AnchorName] = ObjectID(i)
		}
	}
	positionSubtree(a, root, icb, icb, anchors, anchorNodes)
}

func positionSubtree(a *Arena, id ObjectID, cb, icb containingBlock, anchors map[string]ObjectID, anchorNodes map[dom.NodeID]ObjectID) {
	obj := a.Get(id)
	s := obj.Style
	if s != nil && s.Display == style.DisplayNone {
		return
	}
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
		case style.PositionSticky:
			rel := containingBlock{cb.x0, cb.y0, cb.w, cb.h}
			parentObj := a.Get(obj.Parent)
			if parentObj != nil && parentObj.Style != nil {
				x0, y0, x1, y1 := parentObj.ContentRect()
				rel = containingBlock{x0, y0, x1 - x0, y1 - y0}
			}
			dx := float32(0)
			if s.HasLeft {
				dx = resolvePctLength(s.Left, rel.w)
			} else if s.HasRight {
				dx = -resolvePctLength(s.Right, rel.w)
			}
			dy := float32(0)
			if s.HasTop {
				dy = resolvePctLength(s.Top, rel.h)
			} else if s.HasBottom {
				dy = -resolvePctLength(s.Bottom, rel.h)
			}
			if parentObj != nil && parentObj.Style != nil {
				_, py0, _, py1 := parentObj.ContentRect()
				if dy > 0 && obj.Y+obj.H+dy > py1 {
					dy = py1 - (obj.Y + obj.H)
					if dy < 0 {
						dy = 0
					}
				} else if dy < 0 && obj.Y+dy < py0 {
					dy = py0 - obj.Y
					if dy > 0 {
						dy = 0
					}
				}
			}
			if dx != 0 || dy != 0 {
				shiftSubtree(a, id, dx, dy)
				obj = a.Get(id)
			}
		case style.PositionAbsolute:
			placeOutOfFlow(a, id, cb, anchors, anchorNodes)
			obj = a.Get(id)
		case style.PositionFixed:
			placeOutOfFlow(a, id, icb, anchors, anchorNodes)
			obj = a.Get(id)
		}
		if s.OffsetPath != "" && s.OffsetPath != "none" && s.OffsetDistance != 0 {
			dx, dy := computeMotionOffset(s.OffsetPath, s.OffsetDistance)
			if dx != 0 || dy != 0 {
				shiftSubtree(a, id, dx, dy)
				obj = a.Get(id)
			}
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
		positionSubtree(a, kid, childCB, icb, anchors, anchorNodes)
	}
}

func placeOutOfFlow(a *Arena, id ObjectID, cb containingBlock, anchors map[string]ObjectID, anchorNodes map[dom.NodeID]ObjectID) {
	obj := a.Get(id)
	s := obj.Style
	obj.flags &^= flagOutOfFlow
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

	x := obj.StaticX
	w := obj.W
	shrinkW := resolvePctLength(s.Width, cb.w) < 0
	centerX := normalizeAxis(s.JustifySelf) == "center" && s.HasLeft && s.HasRight
	stretched := false
	if !shrinkW {
		switch {
		case s.HasLeft:
			x = cb.x0 + obj.MarginLeft + insetL
		case s.HasRight:
			x = cb.x0 + cb.w - obj.MarginRight - insetR - (w + extra)
		}
	} else if s.HasLeft && s.HasRight && !centerX {
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
		x = obj.StaticX
	}

	y := obj.StaticY
	h := obj.H
	explicitH := resolvePctLength(s.Height, sizeH) >= 0
	centerY := normalizeAxis(s.AlignSelf) == "center" && s.HasTop && s.HasBottom
	if explicitH {
		switch {
		case s.HasTop:
			y = cb.y0 + obj.MarginTop + insetT
		case s.HasBottom:
			y = cb.y0 + cb.h - obj.MarginBottom - insetB - (h + vExtra)
		}
	} else if s.HasTop && s.HasBottom && !centerY {
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
		measureW = maxFlexMeasureWidth
	case !stretched:
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
			shiftInlineContent(a, id, obj.X+obj.BorderLeft+obj.PaddingLeft-srcX, 0)
			obj = a.Get(id)
		} else {
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
	if centerX {
		left := cb.x0 + insetL + obj.MarginLeft
		right := cb.x0 + cb.w - insetR - obj.MarginRight
		x := left + (right-left-obj.BorderW())/2
		shiftSubtree(a, id, x-obj.X, 0)
		obj = a.Get(id)
	}
	if centerY {
		top := cb.y0 + insetT + obj.MarginTop
		bottom := cb.y0 + cb.h - insetB - obj.MarginBottom
		y := top + (bottom-top-obj.BorderH())/2
		shiftSubtree(a, id, 0, y-obj.Y)
		obj = a.Get(id)
	}
	if obj.Node != nil && obj.Node.Element() && obj.Node.IsPopoverOpen && obj.Node.HasAttribute("popover") &&
		s.Position == style.PositionFixed && !s.HasTop && !s.HasRight && !s.HasBottom && !s.HasLeft {
		x := cb.x0 + (cb.w-obj.BorderW())/2
		y := cb.y0 + (cb.h-obj.BorderH())/2
		shiftSubtree(a, id, x-obj.X, y-obj.Y)
		obj = a.Get(id)
	}
	if s.PositionArea != "" {
		var anchorID ObjectID
		if s.PositionAnchorNodeID != 0 {
			anchorID = anchorNodes[s.PositionAnchorNodeID]
		} else if s.PositionAnchor != "" && s.PositionAnchor != "auto" {
			anchorID = anchors[s.PositionAnchor]
		}
		if anchor := a.Get(anchorID); anchor != nil {
			area := strings.Fields(s.PositionArea)
			if dx, dy, ok := positionAreaOffset(area, obj, anchor); ok {
				shiftSubtree(a, id, dx, dy)
			}
		}
	}
}

func positionAreaOffset(tokens []string, obj, anchor *Object) (float32, float32, bool) {
	if len(tokens) == 0 || obj == nil || anchor == nil {
		return 0, 0, false
	}
	side := ""
	spanAll := false
	for _, token := range tokens {
		switch strings.ToLower(token) {
		case "left", "right", "top", "bottom", "center":
			side = strings.ToLower(token)
		case "span-all":
			spanAll = true
		}
	}
	if !spanAll && len(tokens) > 1 {
		return 0, 0, false
	}
	switch side {
	case "left":
		return anchor.X - obj.BorderW() - obj.X,
			anchor.Y + (anchor.BorderH()-obj.BorderH())/2 - obj.Y, true
	case "right":
		return anchor.X + anchor.BorderW() - obj.X,
			anchor.Y + (anchor.BorderH()-obj.BorderH())/2 - obj.Y, true
	case "top":
		return anchor.X + (anchor.BorderW()-obj.BorderW())/2 - obj.X,
			anchor.Y - obj.BorderH() - obj.Y, true
	case "bottom":
		return anchor.X + (anchor.BorderW()-obj.BorderW())/2 - obj.X,
			anchor.Y + anchor.BorderH() - obj.Y, true
	case "center":
		return anchor.X + (anchor.BorderW()-obj.BorderW())/2 - obj.X,
			anchor.Y + (anchor.BorderH()-obj.BorderH())/2 - obj.Y, true
	default:
		return 0, 0, false
	}
}

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

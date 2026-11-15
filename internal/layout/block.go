package layout

import (
	"sort"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

func Block(a *Arena, root ObjectID, viewportW, viewportH float32) {
	a.ViewportW = viewportW
	a.ViewportH = viewportH
	flattenDisplayContents(a, root)
	a.inlineFragLines = make(map[ObjectID]map[int]frame.RectF)
	resolveInlineEdges(a, root, viewportW)
	blockInto(a, root, viewportW)
}

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
	return blockIntoInner(a, id, containingW, true)
}

func blockIntoSized(a *Arena, id ObjectID, containingW float32) float32 {
	return blockIntoInner(a, id, containingW, false)
}

func blockIntoInner(a *Arena, id ObjectID, containingW float32, resolve bool) float32 {
	obj := a.Get(id)
	parentH := definiteH(a, obj.Parent)
	childH := parentH
	var replacedH float32
	if obj.Style != nil {
		if resolve {
			resolveBoxSizes(a, obj, containingW, parentH)
		}
		childH = definiteH(a, id)
		if obj.Style.Display == style.DisplayNone {
			obj.W = 0
			obj.H = 0
			return 0
		}
		if w, h, ok := replacedSize(a, obj, containingW); ok {
			if resolveBoxWidth(a, obj.Style, containingW) < 0 {
				obj.W = w
			}
			replacedH = h
		}
	}
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	if blockifiesChildren(obj.Style) {
		wrapTextRuns(a, id)
	} else if !isTableStructure(obj.Style) {
		wrapInlineRuns(a, id)
	}
	obj = a.Get(id)
	y := float32(0)
	prevBottomMargin := float32(0)
	firstChild := true
	var rowTop, rowCursor float32
	rowActive := false
	var sm strutMetrics
	hasInlineRuns := flowsInlineContent(a, obj)
	var floatLead, floatTrail, floatBottom float32
	floatTop := float32(-1)
	if obj.floatSeedHas {
		floatTop = obj.floatSeedTop
	}
	obj.floatSeedHas = false
	floatBottomL, floatBottomR := obj.floatSeedL, obj.floatSeedR
	obj.floatSeedL, obj.floatSeedR = 0, 0
	var rowKids []ObjectID
	closeRow := func() {
		if !rowActive {
			return
		}
		st := a.Get(id).Style
		if st != nil && (st.TextAlign == style.TextAlignCenter || st.TextAlign == style.TextAlignRight) {
			spare := contentW - rowCursor
			if spare > 0 {
				extra := float32(0)
				if st.TextAlign == style.TextAlignCenter {
					extra = spare / 2
				} else {
					extra = spare
				}
				for _, rk := range rowKids {
					shiftSubtree(a, rk, extra, 0)
				}
			}
		}
		boxes := make([]runDemand, len(rowKids))
		for i, rk := range rowKids {
			boxes[i] = atomicDemand(boxOfAtomic(a, rk, sm), sm)
		}
		fr := lineExtent(sm, boxes)
		for i, rk := range rowKids {
			if d := fr.atomicTop(boxes[i].box); d != 0 {
				shiftSubtree(a, rk, 0, d)
			}
		}
		rowKids = rowKids[:0]
		if bottom := rowTop + fr.height; bottom > y {
			y = bottom
		}
		rowActive = false
		rowTop = 0
		rowCursor = 0
	}
	if obj.Style != nil && obj.Style.ContentVisibility == "hidden" {
		if obj.Style.Width < 0 && obj.Style.ContainIntrinsicWidth > 0 {
			obj.W = obj.Style.ContainIntrinsicWidth
		}
		if obj.Style.Height < 0 && obj.Style.ContainIntrinsicHeight > 0 {
			obj.H = obj.Style.ContainIntrinsicHeight
			y = obj.H
		}
	} else if pass, kind, ok := lookupPass(obj.Style); ok {
		a.PassCounts[kind]++
		y = pass(a, id, containingW)
	} else {
		a.PassCounts[FCBlockFlow]++
		for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if isBlock(k) || holdsBlockContent(a, kid) {
				if isOutOfFlow(k.Style) {
					resolveBoxSizes(a, k, contentW, childH)
					k = a.Get(kid)
					k.StaticX = contentX + k.MarginLeft
					k.StaticY = contentY + y
					k.flags |= flagOutOfFlow
					continue
				}
				if k.Style.Float != style.FloatNone {
					closeRow()
					resolveBoxSizes(a, k, contentW, childH)
					k = a.Get(kid)
					if k.Style.MarginLeft == style.MarginAuto {
						k.MarginLeft = 0
					}
					if k.Style.MarginRight == style.MarginAuto {
						k.MarginRight = 0
					}
					extra := k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
					if resolveBoxWidth(a, k.Style, contentW) < 0 {
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
							clampWidth(a, k, contentW)
							k = a.Get(kid)
						}
					}
					boxW := k.W + extra
					if boxW > contentW {
						boxW = contentW
					}
					outer := k.MarginLeft + boxW + k.MarginRight
					floatY := y
					if f := clearanceFloor(k.Style, floatBottomL, floatBottomR); f > floatY {
						floatY = f
						if f >= floatBottomL {
							floatLead = 0
						}
						if f >= floatBottomR {
							floatTrail = 0
						}
					}
					if floatLead+outer+floatTrail > contentW {
						if floatBottom > floatY {
							floatY = floatBottom
						}
						floatLead, floatTrail = 0, 0
					}
					if floatTop > floatY {
						floatY = floatTop
					}
					if floatY > floatTop {
						floatTop = floatY
					}
					if k.Style.Float == style.FloatLeft {
						k.X = contentX + floatLead + k.MarginLeft
						floatLead += outer
					} else {
						k.X = contentX + contentW - floatTrail - k.MarginRight - boxW
						floatTrail += outer
					}
					k.Y = contentY + floatY + k.MarginTop
					blockIntoSized(a, kid, boxW)
					if !blockifiesChildren(a.Get(kid).Style) {
						inlineInto(a, kid)
					}
					placed := a.Get(kid)
					bottom := floatY + placed.MarginTop + placed.BorderH() + placed.MarginBottom
					if bottom > floatBottom {
						floatBottom = bottom
					}
					if placed.Style != nil {
						switch placed.Style.Float {
						case style.FloatLeft:
							if bottom > floatBottomL {
								floatBottomL = bottom
							}
						case style.FloatRight:
							if bottom > floatBottomR {
								floatBottomR = bottom
							}
						}
					}
					continue
				}
				if k.Style.Display == style.DisplayInlineBlock {
					if hasInlineRuns {
						closeRow()
						continue
					}
					resolveBoxSizes(a, k, contentW, childH)
					k = a.Get(kid)
					extra := k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
					intrinsicTextarea := k.Node != nil && k.Node.Data == "textarea"
					shrink := resolveBoxWidth(a, k.Style, contentW) < 0 && !intrinsicTextarea
					var srcX, measureX, measureY, contentExtent float32
					measured, shrank := false, false
					if shrink {
						measureX, measureY = k.X, k.Y
						blockInto(a, kid, contentW)
						inlineInto(a, kid)
						k = a.Get(kid)
						if k.Style != nil && k.Style.AspectRatio > 0 && k.H > 0 {
							k.W = k.H * k.Style.AspectRatio
							clampWidth(a, k, contentW)
							k = a.Get(kid)
						} else if sx, right, ok := inlineExtent(a, kid); ok && right > sx {
							srcX, contentExtent, measured = sx, right-sx, true
							k.W = contentExtent
							k = a.Get(kid)
							clampWidth(a, k, contentW)
							k = a.Get(kid)
						} else if w := blockMaxContentW(a, kid); w > 0 {
							k = a.Get(kid)
							k.W = w
							clampWidth(a, k, contentW)
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
						if st := a.Get(id).Style; st != nil {
							asc, desc, strutLine := fontHeights(a.Metrics, st.FontSize, st.FontSlot(), st.LineHeight)
							sm = ofStrut(asc, desc, strutLine,
								faceXHeight(a.Metrics, st.FontSize, st.FontSlot()), st.FontSize)
						}
					}
					k.X = contentX + rowCursor + k.MarginLeft
					k.Y = contentY + rowTop + k.MarginTop
					k.flags |= flagRowPlaced
					rowKids = append(rowKids, kid)
					if measured {
						shiftInlineContent(a, kid, k.X+k.BorderLeft+k.PaddingLeft-srcX, k.Y-measureY)
						for c := a.Get(kid).FirstKid; c != 0; c = a.Get(c).NextSibling {
							if isBlock(a.Get(c)) {
								shiftSubtree(a, c, k.X-measureX, k.Y-measureY)
							}
						}
						k = a.Get(kid)
					} else {
						own := contentW
						if shrank {
							own = boxW
						}
						blockInto(a, kid, own)
						inlineInto(a, kid)
						k = a.Get(kid)
					}
					rowCursor += k.MarginLeft + boxW + k.MarginRight
					continue
				}
				closeRow()
				resolveBoxSizes(a, k, contentW, childH)
				if !isFlowBlock(k) {
					k.W = contentW - k.MarginLeft - k.MarginRight - k.PaddingLeft - k.PaddingRight - k.BorderLeft - k.BorderRight
					if k.W < 0 {
						k.W = 0
					}
				}

				var topMargin float32
				if firstChild {
					firstChild = false
					if collapsesThroughTop(a, obj) && !isOutOfFlow(k.Style) {
						topMargin = 0
					} else {
						topMargin = carryTop(a, kid)
					}
				} else {
					topMargin = collapseMargin(prevBottomMargin, carryTop(a, kid))
				}
				if f := clearanceFloor(k.Style, floatBottomL, floatBottomR); y+topMargin < f {
					y = f - topMargin
				}
				k.X = contentX + k.MarginLeft + centerBlockChildLead(obj, k, contentW)
				k.Y = contentY + y + topMargin
				blockifiedInline := k.Style.Display == style.DisplayInline && holdsBlockContent(a, kid)
				if !startsNewFC(a, k) || blockifiedInline {
					childTop := k.Y + k.BorderTop + k.PaddingTop - contentY
					k.floatSeedL = floatBottomL - childTop
					if k.floatSeedL < 0 {
						k.floatSeedL = 0
					}
					k.floatSeedR = floatBottomR - childTop
					if k.floatSeedR < 0 {
						k.floatSeedR = 0
					}
					if floatTop >= 0 {
						k.floatSeedHas = true
						k.floatSeedTop = floatTop - childTop
						if k.floatSeedTop < 0 {
							k.floatSeedTop = 0
						}
					}
				}
				blockInto(a, kid, contentW)
				if !blockifiesChildren(a.Get(kid).Style) {
					inlineInto(a, kid)
				}
				k = a.Get(kid)
				if !startsNewFC(a, k) || blockifiedInline {
					childTop := k.Y + k.BorderTop + k.PaddingTop - contentY
					if b := childTop + k.floatBottomL; b > floatBottomL {
						floatBottomL = b
					}
					if b := childTop + k.floatBottomR; b > floatBottomR {
						floatBottomR = b
					}
					if k.floatTop >= 0 {
						if b := childTop + k.floatTop; floatTop < 0 || b > floatTop {
							floatTop = b
						}
					}
				}
				y = (k.Y - contentY) + k.BorderH()
				prevBottomMargin = carryBottom(a, kid)
			}
		}
		closeRow()
		if floatBottom > y && startsNewFC(a, a.Get(id)) {
			y = floatBottom
		}
	}
	obj = a.Get(id)
	obj.floatBottomL = floatBottomL
	obj.floatBottomR = floatBottomR
	obj.floatTop = floatTop
	if !collapsesThroughBottom(a, obj) {
		y += prevBottomMargin
	}
	if obj.Style != nil && resolvePctLength(obj.Style.Height, parentH) < 0 {
		if obj.Style.AspectRatio > 0 && obj.W > 0 {
			obj.H = obj.W / obj.Style.AspectRatio
		} else {
			obj.H = y
		}
		if replacedH > obj.H {
			obj.H = replacedH
		}
		if obj.H == 0 && obj.Node != nil && obj.Node.Text() {
			lineH, _ := runHeights(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot(), obj.Style.LineHeight)
			obj.H = lineH
		}
	}
	if obj.Style == nil {
		return obj.BorderH()
	}
	clampHeight(a, obj, parentH)
	return obj.BorderH() + obj.MarginTop + obj.MarginBottom
}

func layoutFlexRow(a *Arena, id ObjectID, containingW float32) float32 {
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
		id        ObjectID
		obj       *Object
		basis     float32
		grow      float32
		measured  bool
		pct       float32
		srcX      float32
		contentW  float32
		minW      float32
		declared  bool
		atMax     bool
		blockKids bool
		order     int
	}
	var items []item
	gap := obj.Style.ColumnGap
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && !k.Node.Element() {
			if k.Node.Text() {
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
		items = append(items, item{id: kid, obj: k, grow: k.Style.FlexGrow, order: k.Style.Order})
	}
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
			basis = resolvePctLength(s.FlexBasis, contentW)
		case s.Width >= 0:
			basis = s.Width
		case resolveBoxWidth(a, s, contentW) >= 0:
			basis = resolveBoxWidth(a, s, contentW)
			items[i].pct = -2 - s.Width
		}
		if basis < 0 {
			items[i].basis = -1
			continue
		}
		if s.BoxSizing != style.BoxSizingBorderBox {
			ubw := s.UsedBorderWidths()
			basis += s.PaddingLeft + s.PaddingRight + ubw[1] + ubw[3]
		}
		clamped := clampFlexBasis(a.Get(items[i].id), basis, contentW)
		items[i].basis = clamped
		items[i].atMax = clamped < basis
		items[i].declared = true
		it := a.Get(items[i].id)
		it.X, it.Y = contentX, contentY
		blockInto(a, items[i].id, maxFlexMeasureWidth)
		it = a.Get(items[i].id)
		inlineInto(a, items[i].id)
		it = a.Get(items[i].id)
		extra := it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		floor := inlineMinWidth(a, items[i].id) + extra
		if items[i].basis < floor {
			floor = items[i].basis
		}
		if explicitMin := resolveBoxMinWidth(a, it.Style, contentW); explicitMin > 0 {
			if it.Style.BoxSizing == style.BoxSizingBorderBox {
				floor = explicitMin
			} else {
				floor = explicitMin + extra
			}
		}
		if floor < 0 {
			floor = 0
		}
		items[i].minW = floor
	}

	obj = a.Get(id)
	contentH := obj.H
	alignItems := obj.Style.AlignItems

	for i := range items {
		if items[i].basis >= 0 {
			continue
		}
		it := a.Get(items[i].id)
		it.X, it.Y = contentX, contentY
		blockInto(a, items[i].id, maxFlexMeasureWidth)
		it = a.Get(items[i].id)
		if it.W <= 0 {
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
		if !ok {
			if blockW := itemMaxContentW(a, items[i].id); blockW > 0 {
				contentWidth = blockW
				items[i].blockKids = true
			}
		}
		if contentWidth == 0 && it.W > 0 && isReplacedElement(it) {
			contentWidth = it.W
		}
		lo, hi := flexContentBounds(a, it, contentW)
		if lo > 0 && contentWidth < lo {
			contentWidth = lo
		}
		if hi >= 0 && contentWidth > hi {
			contentWidth = hi
		}
		it.W = contentWidth
		items[i].basis = contentWidth + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		items[i].measured = !items[i].blockKids
		items[i].srcX = srcX
		items[i].contentW = contentWidth
		items[i].minW = inlineMinWidth(a, items[i].id) + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight
		if floor := lo + it.PaddingLeft + it.PaddingRight + it.BorderLeft + it.BorderRight; floor > items[i].minW {
			items[i].minW = floor
		}
	}

	reverse := a.Get(id).Style.FlexDirection == style.FlexRowReverse
	if reverse {
		for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
			items[l], items[r] = items[r], items[l]
		}
	}

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
			frozen := make([]bool, len(items))
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

	justify := a.Get(id).Style.JustifyContent

	for i := range items {
		it := a.Get(items[i].id)
		if it.BorderH() > 0 {
			continue
		}
		it.X, it.Y = contentX, contentY
		blockInto(a, items[i].id, contentW)
		it = a.Get(items[i].id)
		if !blockifiesChildren(it.Style) {
			inlineInto(a, items[i].id)
		}
	}

	lineCross := make([]float32, lineCount)
	for l := 0; l < lineCount; l++ {
		if lineCount == 1 && contentH > 0 {
			lineCross[l] = contentH
			continue
		}
		for i := range items {
			if lineOf[i] != l {
				continue
			}
			it := a.Get(items[i].id)
			h := flexMargin(it.Style.MarginTop) + flexMargin(it.Style.MarginBottom) + it.BorderH()
			if h > lineCross[l] {
				lineCross[l] = h
			}
		}
	}
	if lineCount > 1 && contentH > 0 {
		if ac := normalizeAxis(obj.Style.AlignContent); ac == "" {
			for l := range lineCross {
				lineCross[l] = contentH / float32(lineCount)
			}
		}
	}
	crossUsed := float32(0)
	for _, h := range lineCross {
		crossUsed += h
	}
	if lineCount > 1 {
		crossUsed += gap * float32(lineCount-1)
	}
	crossRemaining := contentH - crossUsed
	if crossRemaining < 0 {
		crossRemaining = 0
	}
	wrapReverse := wrap == style.FlexWrapReverse
	crossLead, crossTrackGap := distributeFlex(obj.Style.AlignContent, wrapReverse, crossRemaining, gap, lineCount)
	lineY := make([]float32, lineCount)
	{
		y := contentY + crossLead
		for order := 0; order < lineCount; order++ {
			l := order
			if wrapReverse {
				l = lineCount - 1 - order
			}
			lineY[l] = y
			y += lineCross[l] + crossTrackGap
		}
	}

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
			it := a.Get(items[i].id)
			mTop := flexMargin(it.Style.MarginTop)
			finalX := cursor + flexMargin(it.Style.MarginLeft)
			prevX, prevY := it.X, it.Y
			if items[i].basis >= 0 && it.Style.Width < 0 {
				w := items[i].basis - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
				if w < 0 {
					w = 0
				}
				it.W = w
			}
			it.X = finalX
			it.Y = lineY[l] + mTop
			if items[i].measured {
				shrunk := it.W+0.5 < items[i].contentW
				var h float32
				var wrapped bool
				if shrunk {
					h, wrapped = reflowInlineWords(a, items[i].id, it.W, finalX+it.BorderLeft+it.PaddingLeft, it.Y+it.BorderTop+it.PaddingTop)
				}
				if wrapped {
					if it.Style.Height < 0 && h > 0 {
						it.H = h
						it = a.Get(items[i].id)
					}
				} else {
					extra := float32(0)
					if spare := it.W - items[i].contentW; spare > 0 {
						switch it.Style.TextAlign {
						case style.TextAlignCenter:
							extra = spare / 2
						case style.TextAlignRight:
							extra = spare
						}
					}
					shiftInlineContent(a, items[i].id, finalX+it.BorderLeft+it.PaddingLeft+extra-items[i].srcX, lineY[l]+mTop)
				}
			} else {
				containW := it.W
				if items[i].pct > 0 {
					containW = items[i].basis * 100 / items[i].pct
				} else if items[i].declared && it.Style.Width == -1 {
					containW = items[i].basis + it.MarginLeft + it.MarginRight
				} else if items[i].blockKids && it.Style.Width == -1 {
					w := items[i].basis - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
					if w < 0 {
						w = 0
					}
					it.W = w
					containW = items[i].basis + it.MarginLeft + it.MarginRight
				}
				if items[i].blockKids {
					clearInlineLaidOut(a, items[i].id)
					blockInto(a, items[i].id, containW)
					it = a.Get(items[i].id)
				} else {
					prevW := it.W
					if hasBlockChild(a, items[i].id) {
						clearInlineLaidOut(a, items[i].id)
						blockInto(a, items[i].id, containW)
						it = a.Get(items[i].id)
					} else {
						blockInto(a, items[i].id, containW)
						it = a.Get(items[i].id)
						if it.W != prevW {
							clearInlineLaidOut(a, items[i].id)
							blockInto(a, items[i].id, containW)
							it = a.Get(items[i].id)
						} else {
							shiftInlineContent(a, items[i].id, finalX-prevX, lineY[l]+mTop-prevY)
						}
					}
				}
				inlineInto(a, items[i].id)
				it = a.Get(items[i].id)
			}
			if items[i].basis >= 0 && it.Style.Width >= 0 {
				want := items[i].basis - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
				if want < 0 {
					want = 0
				}
				if it.W != want {
					it.W = want
					it = a.Get(items[i].id)
					clearInlineLaidOut(a, items[i].id)
					blockIntoSized(a, items[i].id, want)
					inlineInto(a, items[i].id)
					it = a.Get(items[i].id)
				}
			}
			cursor += flexMargin(it.Style.MarginLeft) + items[i].basis + flexMargin(it.Style.MarginRight) + trackGap
		}

		cross := lineCross[l]
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
				pb := it.PaddingTop + it.PaddingBottom + it.BorderTop + it.BorderBottom + mTop + mBottom
				if cross > pb {
					it.H = cross - pb
				} else {
					it.H = 0
				}
				it = a.Get(items[i].id)
			}
			if spare := cross - it.BorderH() - mTop - mBottom; spare > 0 {
				switch normalizeAxis(align) {
				case "flex-end":
					shiftSubtree(a, items[i].id, 0, spare)
				case "center":
					shiftSubtree(a, items[i].id, 0, spare/2)
				}
			}
		}
		if bottom := lineY[l] - contentY + cross; bottom > maxBottom {
			maxBottom = bottom
		}
	}
	return maxBottom
}

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

func flexContentBounds(a *Arena, obj *Object, containingW float32) (float32, float32) {
	s := obj.Style
	if s == nil {
		return 0, -1
	}
	lo := resolveBoxMinWidth(a, s, containingW)
	hi := resolveBoxMaxWidth(a, s, containingW)
	if s.BoxSizing == style.BoxSizingBorderBox {
		extra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight
		if lo >= 0 {
			lo -= extra
		}
		if hi >= 0 {
			hi -= extra
		}
	}
	return lo, hi
}

func flexMargin(v float32) float32 {
	if v == style.MarginAuto || v < 0 {
		return 0
	}
	return v
}

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
	containerH := definiteH(a, id)
	vMargins := float32(0)
	growSum := float32(0)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && !k.Node.Element() {
			if k.Node.Text() {
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
				ubw := s.UsedBorderWidths()
				it.basis += s.PaddingTop + s.PaddingBottom + ubw[0] + ubw[2]
			}
			it.declared = true
		}
		items = append(items, it)
		vMargins += it.mTop + it.mBottom
		growSum += it.grow
	}
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

	availForItems := contentH - vMargins - gap*float32(n-1)
	if availForItems < 0 {
		availForItems = 0
	}
	if contentH <= 0 {
		availForItems = 0
	}
	for i := range items {
		avail := contentW
		if w, ok := fitContentCrossW(a, items[i].id, contentX, contentY, contentW, alignItems); ok {
			avail = w
		}
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
	_ = totalBasis

	wrap := obj.Style.FlexWrap
	lineOf := make([]int, n)
	lineCount := 1
	if wrap != style.FlexNowrap && contentH > 0 {
		lineCount = 0
		used := float32(0)
		lineOpen := false
		for i := range items {
			outer := items[i].basis + items[i].mTop + items[i].mBottom
			if lineOpen {
				if used+gap+outer > contentH {
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
	}

	lineCross := make([]float32, lineCount)
	if lineCount == 1 {
		lineCross[0] = contentW
	} else if ac := normalizeAxis(obj.Style.AlignContent); ac == "" {
		for l := range lineCross {
			lineCross[l] = contentW / float32(lineCount)
		}
	} else {
		for l := 0; l < lineCount; l++ {
			for i := range items {
				if lineOf[i] != l {
					continue
				}
				if w, ok := probeCrossW(a, items[i].id, contentX, contentY, contentW); ok && w > lineCross[l] {
					lineCross[l] = w
				}
			}
			if lineCross[l] <= 0 {
				lineCross[l] = contentW
			}
		}
	}
	crossUsed := float32(0)
	for l := 0; l < lineCount; l++ {
		crossUsed += lineCross[l]
	}
	if lineCount > 1 {
		crossUsed += obj.Style.ColumnGap * float32(lineCount-1)
	}
	crossRemaining := contentW - crossUsed
	if crossRemaining < 0 {
		crossRemaining = 0
	}
	wrapReverse := wrap == style.FlexWrapReverse
	crossLead, crossTrackGap := distributeFlex(obj.Style.AlignContent, wrapReverse, crossRemaining, obj.Style.ColumnGap, lineCount)
	lineX := make([]float32, lineCount)
	x := contentX + crossLead
	for order := 0; order < lineCount; order++ {
		l := order
		if wrapReverse {
			l = lineCount - 1 - order
		}
		lineX[l] = x
		x += lineCross[l] + crossTrackGap
	}

	mainReverse := obj.Style.FlexDirection == style.FlexColumnReverse
	maxBottom := float32(0)
	for order := 0; order < lineCount; order++ {
		l := order
		if wrapReverse {
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
		var lineBasis, lineMargins, lineGrow float32
		for _, i := range line {
			lineBasis += items[i].basis
			lineMargins += items[i].mTop + items[i].mBottom
			lineGrow += items[i].grow
		}
		avail := contentH - lineMargins - gap*float32(len(line)-1)
		if avail < 0 {
			avail = 0
		}
		free := avail - lineBasis
		if free < 0 {
			free = 0
		}
		for _, i := range line {
			if items[i].grow > 0 && lineGrow > 0 {
				items[i].basis += free * items[i].grow / lineGrow
			}
		}
		remaining := avail
		for _, i := range line {
			remaining -= items[i].basis
		}
		if remaining < 0 {
			remaining = 0
		}
		lead, trackGap := distributeFlex(obj.Style.JustifyContent, mainReverse, remaining, gap, len(line))

		cursor := contentY + lead
		lineOriginX := lineX[l]
		for _, i := range line {
			it := a.Get(items[i].id)
			if lineCross[l] < contentW {
				alignN := it.Style.AlignSelf
				if alignN == "" {
					alignN = alignItems
				}
				if normalizeAxis(alignN) == "" && it.Style.Width < 0 && !isPctLength(it.Style.Width) {
					w := lineCross[l] - it.PaddingLeft - it.PaddingRight - it.BorderLeft - it.BorderRight
					if w < 0 {
						w = 0
					}
					it.X = lineOriginX + items[i].mLeft
					blockInto(a, items[i].id, w)
					it = a.Get(items[i].id)
					if !blockifiesChildren(a.Get(items[i].id).Style) {
						inlineInto(a, items[i].id)
						it = a.Get(items[i].id)
					}
				}
			}
			if items[i].declared {
				h := items[i].basis - it.PaddingTop - it.PaddingBottom - it.BorderTop - it.BorderBottom
				if h < 0 {
					h = 0
				}
				it.H = h
				it = a.Get(items[i].id)
			} else if it.Style.Height < 0 && it.Style.FlexGrow > 0 {
				h := items[i].basis - it.PaddingTop - it.PaddingBottom - it.BorderTop - it.BorderBottom
				if h < 0 {
					h = 0
				}
				it.H = h
				it = a.Get(items[i].id)
			}
			if dy := (cursor + items[i].mTop) - it.Y; dy != 0 {
				shiftSubtree(a, items[i].id, 0, dy)
				it = a.Get(items[i].id)
			}
			align := it.Style.AlignSelf
			if align == "" {
				align = alignItems
			}
			if lineCross[l] > items[i].crossW {
				targetX := lineOriginX + items[i].mLeft
				switch normalizeAxis(align) {
				case "center":
					targetX = lineOriginX + (lineCross[l]-items[i].crossW)/2 + it.MarginLeft
				case "flex-end":
					targetX = lineOriginX + lineCross[l] - items[i].crossW - it.MarginRight
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
	}
	return maxBottom
}

func fitContentCrossW(a *Arena, id ObjectID, contentX, contentY, avail float32, alignItems string) (float32, bool) {
	s := a.Get(id).Style
	if s == nil || s.Width >= 0 || isPctLength(s.Width) {
		return 0, false
	}
	align := s.AlignSelf
	if align == "" {
		align = alignItems
	}
	switch normalizeAxis(align) {
	case "center", "flex-start", "flex-end":
	default:
		return 0, false
	}
	return probeCrossW(a, id, contentX, contentY, avail)
}

func probeCrossW(a *Arena, id ObjectID, contentX, contentY, avail float32) (float32, bool) {
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
	clearInlineLaidOut(a, id)
	if contentWidth < 0 || contentWidth+boxW >= avail {
		return 0, false
	}
	return contentWidth + boxW, true
}

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

const maxFlexMeasureWidth = 10000

func inlineExtent(a *Arena, id ObjectID) (minX, maxX float32, ok bool) {
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isBlock(k) {
			continue
		}
		_, _, x1, _ := k.BorderRect()
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
			ok = true
		}
	}
	return
}

func inlineMinWidth(a *Arena, id ObjectID) float32 {
	m := float32(0)
	var walk func(ObjectID)
	walk = func(cid ObjectID) {
		for kid := a.Get(cid).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if k.Node != nil && k.Node.Text() {
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

func itemMaxContentW(a *Arena, id ObjectID) float32 {
	return maxContentW(a, id, true)
}

func isReplacedElement(obj *Object) bool {
	if obj == nil || obj.Node == nil || !obj.Node.Element() {
		return false
	}
	switch obj.Node.Data {
	case "img", "input", "select", "textarea", "video", "audio", "canvas", "svg":
		return true
	}
	return false
}

func maxContentW(a *Arena, id ObjectID, probed bool) float32 {
	obj := a.Get(id)
	if obj.Node != nil && obj.Node.Text() {
		return measureWord(obj.Node.DataContent, obj.Style.FontSize, a.Metrics, obj.Style.FontSlot(), obj.Style.LetterSpacing)
	}
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
			if k.Node != nil && k.Node.Text() {
				run += k.W
				if w := origin + trail + run; w > right {
					right = w
				}
				continue
			}
			if isBlock(k) && !atomicInlineLevel(k.Style) {
				run = 0
				if blockifiesChildren(k.Style) {
					if definiteWidth(k.Style) {
						if w := origin + trail + lead + k.W + trailBox; w > right {
							right = w
						}
						continue
					}
					if dir := k.Style.FlexDirection; k.Style.Display == style.DisplayFlex ||
						k.Style.Display == style.DisplayInlineFlex {
						if dir == style.FlexRow || dir == style.FlexRowReverse {
							if w := origin + trail + lead + rowFlexMaxContent(a, kid) + trailBox; w > right {
								right = w
							}
							continue
						}
					} else {
						if w := origin + trail + lead + k.W + trailBox; w > right {
							right = w
						}
						continue
					}
				}
				if own := declaredOuterW(k.Style); own >= 0 {
					if w := origin + trail + lead + own + trailBox; w > right {
						right = w
					}
				}
				before := right
				walk(kid, trail+lead)
				if right > before {
					if w := right + trailBox; w > right {
						right = w
					}
				}
				continue
			}
			inner := walk(kid, trail+run+lead)
			if inner <= 0 {
				switch {
				case atomicInlineLevel(k.Style) && k.Style.Width == -1:
					inner = maxContentW(a, kid, true)
				case declaredOuterW(k.Style) >= 0:
					inner = declaredOuterW(k.Style)
				case k.Node != nil && k.Node.Element():
					switch k.Node.Data {
					case "img", "input", "select", "textarea", "video", "audio", "canvas", "svg":
						inner = k.W
					}
				}
			}
			run += lead + inner + trailBox
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

func definiteWidth(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	if s.Width < 0 || s.WidthCalc.Set {
		return false
	}
	if isPctLength(s.MinWidth) || isPctLength(s.MaxWidth) {
		return false
	}
	if s.MinWidthCalc.Set || s.MaxWidthCalc.Set {
		return false
	}
	return true
}

func rowFlexMaxContent(a *Arena, id ObjectID) float32 {
	obj := a.Get(id)
	run := float32(0)
	n := 0
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
			continue
		}
		var inner float32
		if k.Node != nil && k.Node.Text() {
			if isSpaceWord(k.Node.DataContent) {
				continue
			}
			inner = k.W
		} else {
			inner = maxContentW(a, kid, true)
			if own := declaredOuterW(k.Style); own > inner {
				inner = own
			}
			if inner <= 0 && isReplacedElement(k) {
				inner = k.W
			}
		}
		if n > 0 {
			run += obj.Style.ColumnGap
		}
		run += flexMargin(k.Style.MarginLeft) + k.BorderLeft + k.PaddingLeft + inner + k.PaddingRight + k.BorderRight + flexMargin(k.Style.MarginRight)
		n++
	}
	return run
}

func hasBlockChild(a *Arena, id ObjectID) bool {
	for kid := a.Get(id).FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		if k := a.Get(kid); isBlock(k) {
			return true
		}
	}
	return false
}

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
			if k.Node != nil && k.Node.Text() {
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

func replacedSize(a *Arena, obj *Object, containingW float32) (w, h float32, ok bool) {
	s := obj.Style
	if s == nil || obj.Node == nil || !obj.Node.Element() {
		return 0, 0, false
	}
	if obj.Node.Data == "img" {
		return imgContentSize(a, obj, containingW)
	}
	if obj.Node.Data == "select" {
		w, h = 152, 21
		if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
			h = lh + 2
		}
		return w, h, true
	}
	if obj.Node.Data == "svg" {
		return svgContentSize(obj, containingW)
	}
	if obj.Node.Data == "video" || obj.Node.Data == "canvas" {
		w, h = 300, 150
		if wAttr := obj.Node.GetAttribute("width"); wAttr != "" {
			if parsedW, err := strconv.ParseFloat(strings.TrimSuffix(wAttr, "px"), 32); err == nil && parsedW > 0 {
				w = float32(parsedW)
			}
		}
		if hAttr := obj.Node.GetAttribute("height"); hAttr != "" {
			if parsedH, err := strconv.ParseFloat(strings.TrimSuffix(hAttr, "px"), 32); err == nil && parsedH > 0 {
				h = float32(parsedH)
			}
		}
		return w, h, true
	}
	if obj.Node.Data == "audio" {
		if !obj.Node.HasAttribute("controls") && !obj.Node.HasAttribute("width") && !obj.Node.HasAttribute("height") {
			return 0, 0, true
		}
		w, h = 300, 32
		if wAttr := obj.Node.GetAttribute("width"); wAttr != "" {
			if parsedW, err := strconv.ParseFloat(strings.TrimSuffix(wAttr, "px"), 32); err == nil && parsedW > 0 {
				w = float32(parsedW)
			}
		}
		if hAttr := obj.Node.GetAttribute("height"); hAttr != "" {
			if parsedH, err := strconv.ParseFloat(strings.TrimSuffix(hAttr, "px"), 32); err == nil && parsedH > 0 {
				h = float32(parsedH)
			}
		}
		return w, h, true
	}
	if obj.Node.Data != "input" && obj.Node.Data != "textarea" {
		return 0, 0, false
	}
	if obj.Node.Data == "textarea" {
		fontSize := int32(s.FontSize)
		if fontSize <= 0 {
			fontSize = 16
		}
		advance := fontSize / 2 * 64
		if a.Metrics != nil {
			advance = a.Metrics.GlyphAdvanceFixed(fontSize, '0', s.FontSlot())
		}
		if advance <= 0 {
			advance = fontSize / 2 * 64
		}
		w = float32(obj.Node.TextareaCols()) * float32(advance) / 64
		if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
			h = lh
		} else {
			h = float32(fontSize)
		}
		h *= float32(obj.Node.TextareaRows())
		if w > 16384 {
			w = 16384
		}
		if h > 16384 {
			h = 16384
		}
		return w, h, true
	}
	switch strings.ToLower(obj.Node.GetAttribute("type")) {
	case "image":
		return imgContentSize(a, obj, containingW)
	case "checkbox", "radio":
		return 13, 13, true
	case "range":
		return 129, 16, true
	case "submit", "reset", "button":
		var text string
		if obj.Node.HasAttribute("value") {
			text = obj.Node.GetAttribute("value")
		} else {
			switch strings.ToLower(obj.Node.GetAttribute("type")) {
			case "submit":
				text = "Submit"
			case "reset":
				text = "Reset"
			}
		}
		w = estimateTextWidth(a, s, text)
		if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
			h = lh
		} else {
			h = 15
		}
		return w, h, true
	case "", "text", "password", "email", "tel", "url", "search", "number":
	case "date", "time", "datetime-local", "datetime", "month", "week", "color", "file":
		switch strings.ToLower(obj.Node.GetAttribute("type")) {
		case "date":
			w, h = 119.328125, 17.328125
		case "time":
			w, h = 72, 20
		case "datetime-local", "datetime":
			w, h = 179.328125, 17.328125
		case "month":
			w, h = 149.328125, 17.328125
		case "week":
			w, h = 141.328125, 17.328125
		case "color":
			w, h = 44, 23
		case "file":
			w = 253
			h = 21
			if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
				h = lh + 6
			}
		}
		return w, h, true
	default:
		return 0, 0, false
	}
	w, h = 145, 15
	if lh, _ := runHeights(a.Metrics, s.FontSize, s.FontSlot(), s.LineHeight); lh > 0 {
		h = lh
	}
	return w, h, true
}

func estimateTextWidth(a *Arena, s *style.ComputedStyle, text string) float32 {
	if text == "" {
		return 0
	}
	fontSize := int32(s.FontSize)
	if fontSize <= 0 {
		return 0
	}
	slot := s.FontSlot()
	runes := []rune(text)
	if a.Metrics != nil {
		var pen int32
		for i, r := range runes {
			pen += a.Metrics.GlyphAdvanceFixed(fontSize, r, slot)
			if i < len(runes)-1 {
				pen += int32(s.LetterSpacing) * 64
			}
			if r == ' ' {
				pen += int32(s.WordSpacing) * 64
			}
		}
		return float32((pen + 32) >> 6)
	}
	return float32(len(runes)) * s.FontSize / 2
}

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
	if s.AspectRatio > 0 {
		ratio = 1.0 / s.AspectRatio
	} else if natW > 0 && natH > 0 {
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
	} else if natH > 0 && !haveW {
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
	if clamped := clampReplaced(w, resolveBoxMinWidth(a, s, containingW), resolveBoxMaxWidth(a, s, containingW)); clamped != w {
		w = clamped
		if s.Height < 0 && ratio > 0 {
			h = w * ratio
		}
	}
	h = clampReplaced(h, resolveBoxMinHeight(a, s, containingW), resolveBoxMaxHeight(a, s, containingW))
	return w, h, true
}

func clampReplaced(v, minW, maxW float32) float32 {
	if minW > 0 && v < minW {
		v = minW
	}
	if maxW >= 0 && v > maxW {
		v = maxW
	}
	return v
}

func svgContentSize(obj *Object, containingW float32) (w, h float32, ok bool) {
	w, h = 300, 150
	attrW := attrPx(obj.Node.GetAttribute("width"))
	attrH := attrPx(obj.Node.GetAttribute("height"))
	vbW, vbH := float32(0), float32(0)
	if f := strings.Fields(obj.Node.GetAttribute("viewBox")); len(f) == 4 {
		if v, err := strconv.ParseFloat(f[2], 32); err == nil && v > 0 {
			vbW = float32(v)
		}
		if v, err := strconv.ParseFloat(f[3], 32); err == nil && v > 0 {
			vbH = float32(v)
		}
	}
	ratio := float32(0)
	if vbW > 0 && vbH > 0 {
		ratio = vbH / vbW
	}
	switch {
	case attrW > 0 && attrH > 0:
		w, h = attrW, attrH
	case attrW > 0:
		w = attrW
		if ratio > 0 {
			h = w * ratio
		}
	case attrH > 0:
		h = attrH
		if ratio > 0 {
			w = h / ratio
		}
	case ratio > 0 && containingW > 0:
		w = containingW
		h = w * ratio
	}
	return w, h, true
}

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

func resolveBoxSizes(a *Arena, obj *Object, containingW, containingH float32) {
	s := obj.Style
	if s == nil {
		return
	}
	obj.PaddingTop = resolvePctLength(s.PaddingTop, containingW)
	obj.PaddingRight = resolvePctLength(s.PaddingRight, containingW)
	obj.PaddingBottom = resolvePctLength(s.PaddingBottom, containingW)
	obj.PaddingLeft = resolvePctLength(s.PaddingLeft, containingW)
	ubw := s.UsedBorderWidths()
	obj.BorderTop, obj.BorderRight = ubw[0], ubw[1]
	obj.BorderBottom, obj.BorderLeft = ubw[2], ubw[3]

	innerExtra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight

	var explicitW float32 = -1
	widthVal := resolveBoxWidth(a, s, containingW)
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

	if explicitW >= 0 {
		obj.W = explicitW
	} else if s.Display != style.DisplayInline {
		obj.W = containingW - obj.MarginLeft - obj.MarginRight - innerExtra
	}
	if obj.W < 0 {
		obj.W = 0
	}
	clampWidth(a, obj, containingW)
	if s.Display != style.DisplayInline {
		distributeAutoMargins(obj, leftAuto, rightAuto, containingW, innerExtra)
	}

	heightVal := resolveBoxHeight(a, s, containingH)
	if heightVal >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			obj.H = heightVal - obj.PaddingTop - obj.PaddingBottom - obj.BorderTop - obj.BorderBottom
		} else {
			obj.H = heightVal
		}
	} else if s.ContentVisibility == "hidden" && s.ContainIntrinsicHeight > 0 {
		obj.H = s.ContainIntrinsicHeight
	} else if s.AspectRatio > 0 && obj.W > 0 {
		obj.H = obj.W / s.AspectRatio
	}
	if obj.H < 0 {
		obj.H = 0
	}
	clampHeight(a, obj, containingH)

	if explicitW < 0 && s.AspectRatio > 0 && obj.H > 0 && s.Display != style.DisplayBlock {
		obj.W = obj.H * s.AspectRatio
		clampWidth(a, obj, containingW)
	}
}

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

func centerBlockChildLead(parent, kid *Object, contentW float32) float32 {
	if parent.Node == nil || parent.Node.Data != "center" {
		return 0
	}
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

func clampWidth(a *Arena, obj *Object, containingW float32) {
	s := obj.Style
	if s == nil {
		return
	}
	minW := resolveBoxMinWidth(a, s, containingW)
	maxW := resolveBoxMaxWidth(a, s, containingW)
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
	if maxW >= 0 && obj.W > maxW {
		obj.W = maxW
	}
	if obj.W < 0 {
		obj.W = 0
	}
}

func clampHeight(a *Arena, obj *Object, containingH float32) {
	s := obj.Style
	if s == nil {
		return
	}
	minH := resolveBoxMinHeight(a, s, containingH)
	maxH := resolveBoxMaxHeight(a, s, containingH)
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

func atomicInlineLevel(s *style.ComputedStyle) bool {
	if s == nil {
		return false
	}
	switch s.Display {
	case style.DisplayInlineBlock, style.DisplayInlineFlex, style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

func isBlock(obj *Object) bool {
	if obj.Style == nil {
		return false
	}
	switch obj.Style.Display {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayTable, style.DisplayInlineTable,
		style.DisplayFlex, style.DisplayInlineFlex, style.DisplayInlineBlock,
		style.DisplayGrid, style.DisplayInlineGrid,
		style.DisplayTableRow, style.DisplayTableCell, style.DisplayTableCaption,
		style.DisplayTableRowGroup, style.DisplayTableHeaderGroup, style.DisplayTableFooterGroup:
		return true
	}
	return false
}

func flowsInlineContent(a *Arena, obj *Object) bool {
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isOutOfFlow(k.Style) {
			continue
		}
		if k.Node != nil && k.Node.Text() {
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

func holdsBlockContent(a *Arena, id ObjectID) bool {
	obj := a.Get(id)
	if obj.Node == nil || !obj.Node.Element() || isBlock(obj) {
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

func isOutOfFlow(s *style.ComputedStyle) bool {
	return s.Position == style.PositionAbsolute || s.Position == style.PositionFixed
}

func isFlowFloat(s *style.ComputedStyle) bool {
	return s != nil && (s.Float == style.FloatLeft || s.Float == style.FloatRight)
}

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
	if s.FlowRoot {
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

func edgeDeclared(v float32) bool { return v > 0 || v <= -2 }

func collapsesThroughTop(a *Arena, obj *Object) bool {
	if obj.Style == nil || startsNewFC(a, obj) {
		return false
	}
	return !edgeDeclared(obj.Style.BorderTopWidth) && !edgeDeclared(obj.Style.PaddingTop)
}

func collapsesThroughBottom(a *Arena, obj *Object) bool {
	if obj.Style == nil || startsNewFC(a, obj) {
		return false
	}
	return obj.Style.Height < 0 &&
		!edgeDeclared(obj.Style.BorderBottomWidth) && !edgeDeclared(obj.Style.PaddingBottom)
}

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

func outOfFlowForCollapse(k *Object) bool {
	if k.Style == nil || k.Style.Display == style.DisplayNone {
		return true
	}
	return isOutOfFlow(k.Style)
}

func carryMargin(v float32) float32 {
	if v <= style.MarginAuto {
		return 0
	}
	return v
}

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

func resolvePctLength(val, containingW float32) float32 {
	if isPctLength(val) {
		pct := -2 - val
		return pct * containingW / 100
	}
	return val
}

func resolveBoxWidth(a *Arena, s *style.ComputedStyle, containingW float32) float32 {
	if s == nil {
		return -1
	}
	if s.WidthCalc.Set {
		if v, ok := s.WidthCalc.Resolve(containingW, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.Width, containingW)
}

func resolveBoxHeight(a *Arena, s *style.ComputedStyle, containingH float32) float32 {
	if s == nil {
		return -1
	}
	if s.HeightCalc.Set {
		if v, ok := s.HeightCalc.Resolve(containingH, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.Height, containingH)
}

func resolveBoxMinWidth(a *Arena, s *style.ComputedStyle, containingW float32) float32 {
	if s == nil {
		return -1
	}
	if s.MinWidthCalc.Set {
		if v, ok := s.MinWidthCalc.Resolve(containingW, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.MinWidth, containingW)
}

func resolveBoxMaxWidth(a *Arena, s *style.ComputedStyle, containingW float32) float32 {
	if s == nil {
		return -1
	}
	if s.MaxWidthCalc.Set {
		if v, ok := s.MaxWidthCalc.Resolve(containingW, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.MaxWidth, containingW)
}

func resolveBoxMinHeight(a *Arena, s *style.ComputedStyle, containingH float32) float32 {
	if s == nil {
		return -1
	}
	if s.MinHeightCalc.Set {
		if v, ok := s.MinHeightCalc.Resolve(containingH, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.MinHeight, containingH)
}

func resolveBoxMaxHeight(a *Arena, s *style.ComputedStyle, containingH float32) float32 {
	if s == nil {
		return -1
	}
	if s.MaxHeightCalc.Set {
		if v, ok := s.MaxHeightCalc.Resolve(containingH, a.ViewportW, a.ViewportH); ok {
			return v
		}
		return -1
	}
	return resolvePctLength(s.MaxHeight, containingH)
}

func isPctLength(val float32) bool {
	return val <= -2 && val >= -102
}

func clearanceFloor(s *style.ComputedStyle, floatBottomL, floatBottomR float32) float32 {
	if s == nil {
		return 0
	}
	switch s.Clear {
	case style.ClearLeft:
		return floatBottomL
	case style.ClearRight:
		return floatBottomR
	case style.ClearBoth:
		if floatBottomL > floatBottomR {
			return floatBottomL
		}
		return floatBottomR
	default:
		return 0
	}
}

package layout

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

type floatIntrusion struct {
	x0, x1, y0, y1 float32
	isLeft         bool
	shapeOutside   string
	shapeMargin    float32
}

type breakCtx struct {
	contentW float32
	x0       float32
	strut    float32
	in       []floatIntrusion
	yTop     float32
	xoff     float32
	avail    float32
}

func collectFloatIntrusions(a *Arena, id ObjectID) []floatIntrusion {
	var out []floatIntrusion
	prev := ObjectID(0)
	for c := id; c != 0; {
		co := a.Get(c)
		for kid := co.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			if kid == prev {
				continue
			}
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if k.Style.Float != style.FloatLeft && k.Style.Float != style.FloatRight {
				continue
			}
			if k.flags&flagOutOfFlow != 0 {
				continue
			}
			x0, y0, x1, y1 := k.BorderRect()
			x0 -= k.MarginLeft
			y0 -= k.MarginTop
			x1 += k.MarginRight
			y1 += k.MarginBottom
			if x1 <= x0 || y1 <= y0 {
				continue
			}
			fi := floatIntrusion{
				x0:     x0,
				x1:     x1,
				y0:     y0,
				y1:     y1,
				isLeft: k.Style.Float == style.FloatLeft,
			}
			if k.Style.ShapeOutside != "" && k.Style.ShapeOutside != "none" {
				fi.shapeOutside = k.Style.ShapeOutside
				fi.shapeMargin = k.Style.ShapeMargin
			}
			out = append(out, fi)
		}
		if startsNewFC(a, co) {
			break
		}
		prev = c
		c = co.Parent
	}
	return out
}

func (bc *breakCtx) spanAt(yTop, h float32) (float32, float32) {
	if len(bc.in) == 0 {
		return 0, bc.contentW
	}
	l, r := float32(0), float32(0)
	for _, f := range bc.in {
		if f.y0 >= yTop+h || f.y1 <= yTop {
			continue
		}
		fx0, fx1 := f.x0, f.x1
		if f.shapeOutside != "" {
			if sx0, sx1, ok := computeShapeBounds(f, yTop, h); ok {
				fx0, fx1 = sx0, sx1
			}
		}

		if fx0 <= bc.x0+1 && fx1 > bc.x0 {
			if d := fx1 - bc.x0; d > l {
				l = d
			}
		}
		if fx1 >= bc.x0+bc.contentW-1 && fx0 < bc.x0+bc.contentW {
			if d := bc.x0 + bc.contentW - fx0; d > r {
				r = d
			}
		}
	}
	if avail := bc.contentW - l - r; avail > 0 {
		return l, avail
	}
	return l, 0
}

func computeShapeBounds(f floatIntrusion, yTop, h float32) (float32, float32, bool) {
	shape := strings.ToLower(strings.TrimSpace(f.shapeOutside))
	if strings.HasPrefix(shape, "circle") {
		bw := f.x1 - f.x0
		bh := f.y1 - f.y0
		r := bw
		if bh < r {
			r = bh
		}
		r /= 2
		cx := (f.x0 + f.x1) / 2
		cy := (f.y0 + f.y1) / 2

		if open := strings.Index(shape, "("); open >= 0 {
			if close := strings.LastIndex(shape, ")"); close > open {
				inner := strings.TrimSpace(shape[open+1 : close])
				if inner != "" {
					parts := strings.Split(inner, "at")
					rPart := strings.TrimSpace(parts[0])
					if rPart != "" {
						if strings.HasSuffix(rPart, "%") {
							if val, err := strconv.ParseFloat(strings.TrimSuffix(rPart, "%"), 64); err == nil {
								r = float32(val/100) * (bw + bh) / 4
							}
						} else if val, err := strconv.ParseFloat(strings.TrimSuffix(rPart, "px"), 64); err == nil {
							r = float32(val)
						}
					}
					if len(parts) > 1 {
						posFields := strings.Fields(parts[1])
						if len(posFields) >= 1 {
							if strings.HasSuffix(posFields[0], "%") {
								if val, err := strconv.ParseFloat(strings.TrimSuffix(posFields[0], "%"), 64); err == nil {
									cx = f.x0 + float32(val/100)*bw
								}
							} else if val, err := strconv.ParseFloat(strings.TrimSuffix(posFields[0], "px"), 64); err == nil {
								cx = f.x0 + float32(val)
							}
						}
						if len(posFields) >= 2 {
							if strings.HasSuffix(posFields[1], "%") {
								if val, err := strconv.ParseFloat(strings.TrimSuffix(posFields[1], "%"), 64); err == nil {
									cy = f.y0 + float32(val/100)*bh
								}
							} else if val, err := strconv.ParseFloat(strings.TrimSuffix(posFields[1], "px"), 64); err == nil {
								cy = f.y0 + float32(val)
							}
						}
					}
				}
			}
		}

		effR := r + f.shapeMargin
		y0 := yTop
		if f.y0 > y0 {
			y0 = f.y0
		}
		y1 := yTop + h
		if f.y1 < y1 {
			y1 = f.y1
		}

		var dy float32
		if cy >= y0 && cy <= y1 {
			dy = 0
		} else if cy < y0 {
			dy = y0 - cy
		} else {
			dy = cy - y1
		}

		if dy >= effR {
			if f.isLeft {
				return f.x0, f.x0, true
			}
			return f.x1, f.x1, true
		}

		dx := float32(math.Sqrt(float64(effR*effR - dy*dy)))
		if f.isLeft {
			return f.x0, cx + dx, true
		}
		return cx - dx, f.x1, true
	}
	return f.x0, f.x1, false
}

func (bc *breakCtx) startLine(yTop, h float32) {
	if len(bc.in) == 0 {
		bc.yTop, bc.xoff, bc.avail = yTop, 0, bc.contentW
		return
	}
	for iter := 0; iter < 8; iter++ {
		l, avail := bc.spanAt(yTop, h)
		if avail > 0 {
			bc.yTop, bc.xoff, bc.avail = yTop, l, avail
			return
		}
		low := float32(-1)
		for _, f := range bc.in {
			if f.y0 < yTop+h && f.y1 > yTop && (low < 0 || f.y1 < low) {
				low = f.y1
			}
		}
		if low < 0 {
			bc.yTop, bc.xoff, bc.avail = yTop, l, 0
			return
		}
		yTop = low
	}
	bc.yTop, bc.xoff, bc.avail = yTop, 0, bc.contentW
}

func (bc *breakCtx) nextLine(h float32) {
	if h < bc.strut {
		h = bc.strut
	}
	bc.startLine(bc.yTop+h, bc.strut)
}

func Inline(a *Arena, root ObjectID) {
	resolveInlineEdges(a, root, a.Get(root).W)
	inlineInto(a, root)
	inlineSelfRects(a, root)
}

func inlineSelfRects(a *Arena, id ObjectID) {
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		inlineSelfRects(a, kid)
	}
	if obj.Style == nil || obj.Node == nil || !obj.Node.Element() {
		return
	}
	if obj.Style.Display != style.DisplayInline {
		return
	}
	a.finalizeInlineFragments(id)
	x0, y0, x1, y1, ok := inlineFragmentBounds(a, id)
	if !ok {
		return
	}
	obj.X = x0
	obj.Y = y0
	obj.W = x1 - x0 - obj.PaddingLeft - obj.PaddingRight - obj.BorderLeft - obj.BorderRight
	obj.H = y1 - y0 - obj.PaddingTop - obj.PaddingBottom - obj.BorderTop - obj.BorderBottom
	if obj.W < 0 {
		obj.W = 0
	}
	if obj.H < 0 {
		obj.H = 0
	}
}

func inlineFragmentBounds(a *Arena, id ObjectID) (minX, minY, maxX, maxY float32, ok bool) {
	obj := a.Get(id)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isBlock(k) {
			continue
		}
		x0, y0, x1, y1 := k.BorderRect()
		if x1 > x0 && y1 > y0 {
			if !ok || x0 < minX {
				minX = x0
			}
			if !ok || y0 < minY {
				minY = y0
			}
			if !ok || x1 > maxX {
				maxX = x1
			}
			if !ok || y1 > maxY {
				maxY = y1
			}
			ok = true
		}
		cx0, cy0, cx1, cy1, cok := inlineFragmentBounds(a, kid)
		if cok {
			if !ok || cx0 < minX {
				minX = cx0
			}
			if !ok || cy0 < minY {
				minY = cy0
			}
			if !ok || cx1 > maxX {
				maxX = cx1
			}
			if !ok || cy1 > maxY {
				maxY = cy1
			}
			ok = true
		}
	}
	return
}

func resolveInlineEdges(a *Arena, id ObjectID, containingW float32) {
	obj := a.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}
	cw := containingW
	if isBlock(obj) {
		cw = obj.W
	}
	if obj.Style != nil && obj.Style.Display == style.DisplayInline && obj.Node != nil && obj.Node.Element() {
		ubw := obj.Style.UsedBorderWidths()
		obj.BorderTop, obj.BorderRight, obj.BorderBottom, obj.BorderLeft = ubw[0], ubw[1], ubw[2], ubw[3]
		obj.PaddingTop = resolvePctLength(obj.Style.PaddingTop, cw)
		obj.PaddingRight = resolvePctLength(obj.Style.PaddingRight, cw)
		obj.PaddingBottom = resolvePctLength(obj.Style.PaddingBottom, cw)
		obj.PaddingLeft = resolvePctLength(obj.Style.PaddingLeft, cw)
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		resolveInlineEdges(a, kid, cw)
	}
}

func inlineFragmentGenerator(a *Arena, frag *Object) (ObjectID, bool) {
	if frag.Node == nil || !frag.Node.Text() || frag.Node.Parent == nil {
		return 0, false
	}
	gen, ok := a.ForNode(frag.Node.Parent.ID)
	if !ok {
		return 0, false
	}
	g := a.Get(gen)
	if g.Style == nil || g.Style.Display != style.DisplayInline {
		return 0, false
	}
	return gen, true
}

func (a *Arena) recordInlineFrag(gen ObjectID, line int, r frame.RectF) {
	if a.inlineFragLines == nil {
		a.inlineFragLines = make(map[ObjectID]map[int]frame.RectF)
	}
	byLine := a.inlineFragLines[gen]
	if byLine == nil {
		byLine = make(map[int]frame.RectF)
		a.inlineFragLines[gen] = byLine
	}
	if cur, ok := byLine[line]; ok {
		byLine[line] = unionRectF(cur, r)
	} else {
		byLine[line] = r
	}
}

func unionRectF(a, b frame.RectF) frame.RectF {
	return frame.RectF4(minF(a.X0, b.X0), minF(a.Y0, b.Y0), maxF(a.X1, b.X1), maxF(a.Y1, b.Y1))
}

func (a *Arena) finalizeInlineFragments(id ObjectID) {
	if len(a.inlineFragLines) == 0 {
		return
	}
	obj := a.Get(id)
	merged := make(map[int]frame.RectF)
	for line, r := range a.inlineFragLines[id] {
		merged[line] = r
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display != style.DisplayInline {
			continue
		}
		for line, r := range a.inlineFragLines[kid] {
			if cur, ok := merged[line]; ok {
				merged[line] = unionRectF(cur, r)
			} else {
				merged[line] = r
			}
		}
	}
	if len(merged) == 0 {
		return
	}
	lines := make([]int, 0, len(merged))
	for line := range merged {
		lines = append(lines, line)
	}
	sort.Ints(lines)

	rtl := obj.Style.Direction == "rtl"
	frags := make([]frame.RectF, 0, len(lines))
	expanded := make(map[int]frame.RectF, len(lines))
	for i, line := range lines {
		r := merged[line]
		first := i == 0
		last := i == len(lines)-1
		x0, y0, x1, y1 := r.X0, r.Y0, r.X1, r.Y1
		y0 -= obj.PaddingTop + obj.BorderTop
		y1 += obj.PaddingBottom + obj.BorderBottom
		expandLeft := (first && !rtl) || (last && rtl)
		expandRight := (last && !rtl) || (first && rtl)
		if expandLeft {
			x0 -= obj.PaddingLeft + obj.BorderLeft
		}
		if expandRight {
			x1 += obj.PaddingRight + obj.BorderRight
		}
		f := frame.RectF4(x0, y0, x1, y1)
		frags = append(frags, f)
		expanded[line] = f
	}
	obj.InlineFragments = frags
	a.inlineFragLines[id] = expanded
}

func inlineInto(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}
	if obj.flags&flagInlineLaidOut != 0 {
		return
	}
	if obj.flags&flagOutOfFlow != 0 {
		return
	}
	obj.flags |= flagInlineLaidOut
	contentW := obj.W
	strut := float32(0)
	var sm strutMetrics
	if obj.Style != nil {
		asc, desc, strutLine := fontHeights(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot(), obj.Style.LineHeight)
		strut = strutLine
		sm = ofStrut(asc, desc, strutLine,
			faceXHeight(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot()), obj.Style.FontSize)
	}
	bc := &breakCtx{
		contentW: contentW,
		x0:       obj.X + obj.PaddingLeft + obj.BorderLeft,
		strut:    strut,
		in:       collectFloatIntrusions(a, id),
	}
	bc.startLine(obj.Y+obj.PaddingTop+obj.BorderTop, strut)
	var lines []lineBox
	var current lineBox
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Node == nil || k.Style == nil || k.Style.Display == style.DisplayNone || k.Style.Float != style.FloatNone {
			continue
		}
		if k.Node.Text() {
			collectInline(a, kid, &lines, &current, bc)
			continue
		}
		if isBlock(k) {
			if atomicInlineLevel(k.Style) && k.Node != nil && k.Node.Element() &&
				k.flags&flagRowPlaced == 0 {
				atomicInlineBlock(a, kid, &lines, &current, bc)
				continue
			}
			if len(current.runs) > 0 {
				trimTrailingSpaces(a, &current)
				if len(current.runs) > 0 {
					lines = append(lines, current)
					bc.nextLine(current.h)
				}
				current = lineBox{}
				bc.startLine(bc.yTop, strut)
			}
			if blockifiesChildren(a.Get(kid).Style) {
				continue
			}
			inlineInto(a, kid)
			continue
		}
		if k.Node != nil && k.Node.Data == "br" {
			if len(current.runs) > 0 {
				trimTrailingSpaces(a, &current)
				if len(current.runs) > 0 {
					lines = append(lines, current)
					bc.nextLine(current.h)
				}
			} else {
				s := a.Get(id).Style
				size, lineHeight := float32(16), float32(0)
				var slot frame.FontSlot
				if s != nil {
					size, lineHeight, slot = s.FontSize, s.LineHeight, s.FontSlot()
				}
				h, _ := runHeights(a.Metrics, size, slot, lineHeight)
				lines = append(lines, lineBox{h: h})
				bc.nextLine(h)
			}
			current = lineBox{}
			bc.startLine(bc.yTop, strut)
			continue
		}
		collectInline(a, kid, &lines, &current, bc)
	}
	if len(current.runs) > 0 {
		trimTrailingSpaces(a, &current)
		if len(current.runs) > 0 {
			lines = append(lines, current)
		}
	}
	if obj.Style != nil && obj.Style.TextWrap == "balance" && len(lines) >= 2 && len(lines) <= 10 {
		lines = balanceLines(a, lines, contentW)
	}
	if obj.Style != nil && obj.Style.LineClamp > 0 && len(lines) > obj.Style.LineClamp {
		clamp := obj.Style.LineClamp
		for _, line := range lines[clamp:] {
			for _, run := range line.runs {
				rObj := a.Get(run.obj)
				rObj.W, rObj.H = 0, 0
			}
		}
		lines = lines[:clamp]
	}
	if obj.Node != nil && obj.Node.Element() && len(lines) > 0 {
		if flStyle, ok := a.FirstLineStyles[obj.Node.ID]; ok && flStyle != nil {
			applyFirstLineRuns(a, &lines[0], flStyle, obj.Style)
		}
	}
	applyTakenSoftHyphens(a, lines)
	obj = a.Get(id)
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	contentW = obj.W
	justifyExtra := float32(0)
	for li, line := range lines {
		demands := runDemands(a, line.runs, sm)
		fr := lineExtent(sm, demands)
		line.h = fr.height
		justifyExtra = 0
		lxoff, lavail := bc.spanAt(y, line.h)
		if lavail <= 0 {
			lxoff, lavail = 0, contentW
		}
		baseX := obj.X + obj.PaddingLeft + obj.BorderLeft + lxoff
		alignW := line.w
		if obj.Style != nil && alignW < lavail {
			switch obj.Style.TextAlign {
			case style.TextAlignCenter:
				baseX += (lavail - alignW) / 2
			case style.TextAlignRight:
				baseX += lavail - alignW
			case style.TextAlignJustify:
				if li < len(lines)-1 {
					justifyExtra = computeJustifyExtra(line, lavail, a)
				}
			}
		}
		lineRuns := line.runs
		rtlStart := obj.Style != nil && obj.Style.Direction == "rtl"
		if obj.Style != nil && (rtlStart || needsBidi(obj.Style, line.runs)) {
			lineRuns, rtlStart = bidiReorder(obj.Style, a, line.runs)
		}
		if rtlStart && obj.Style.TextAlign == style.TextAlignStart && alignW < lavail {
			baseX += lavail - alignW
		}
		x := baseX
		for _, run := range lineRuns {
			runObj := a.Get(run.obj)
			if run.atomic {
				k := a.Get(run.obj)
				dx := (x + k.MarginLeft) - run.srcX
				d := demandForRun(demands, line.runs, run.obj)
				dy := (y + fr.atomicTop(d.box) + k.MarginTop) - run.srcY
				if isRubyAnnotation(k, a) {
					dy -= line.h * 0.4
				}
				shiftSubtree(a, run.obj, dx, dy)
				x += run.w
				continue
			}
			letterSpacing := float32(0)
			if runObj.Style != nil {
				letterSpacing = runObj.Style.LetterSpacing
			}
			wordW := measureWord(run.text, run.size, a.Metrics, run.slot, letterSpacing)
			if run.text == " " && runObj.Style != nil {
				wordW += runObj.Style.WordSpacing
			}
			if runObj.X == 0 && runObj.Y == 0 {
				runObj.X = x
				d := demandForRun(demands, line.runs, run.obj)
				baseY := y + fr.atomicTop(d.box) + d.half
				if isRubyAnnotation(runObj, a) {
					baseY -= run.contentH * 0.5
				}
				runObj.Y = baseY
			}
			runObj.W = wordW
			runObj.H = run.contentH
			if gen, ok := inlineFragmentGenerator(a, runObj); ok {
				g := a.Get(gen)
				if g.BorderTop > 0 || g.BorderRight > 0 || g.BorderBottom > 0 || g.BorderLeft > 0 {
					fx0, fy0, fx1, fy1 := runObj.BorderRect()
					a.recordInlineFrag(gen, li, frame.RectF4(fx0, fy0, fx1, fy1))
				}
			}
			x += wordW
			if justifyExtra > 0 && isSpaceWord(run.text) {
				x += justifyExtra
			}
		}
		y += line.h
		lineH += line.h
	}
	if obj.Style != nil && obj.Style.Height < 0 && lineH > 0 {
		if obj.H == 0 {
			obj.H = lineH
		}
	}
}

func computeJustifyExtra(line lineBox, contentW float32, a *Arena) float32 {
	spaceCount := 0
	for _, run := range line.runs {
		if isSpaceWord(run.text) {
			spaceCount++
		}
	}
	if spaceCount == 0 {
		return 0
	}
	return (contentW - line.w) / float32(spaceCount)
}

func collectInline(a *Arena, nodeID ObjectID, lines *[]lineBox, current *lineBox, bc *breakCtx) {
	k := a.Get(nodeID)
	if k.Style == nil || k.Style.Display == style.DisplayNone {
		return
	}
	if isBlock(k) {
		if atomicInlineLevel(k.Style) && k.Node != nil &&
			(k.Node.Element() || k.flags&flagAnonymous != 0) &&
			k.flags&flagRowPlaced == 0 {
			atomicInlineBlock(a, nodeID, lines, current, bc)
		}
		return
	}
	if k.Node != nil && k.Node.Text() {
		text := k.Node.DataContent
		if k.Style.TextTransform != "" {
			text = applyTextTransform(text, k.Style.TextTransform)
		}
		useMerged := false
		if k.Style.WhiteSpace == style.WhiteSpacePreline {
			text = collapseWhitespacePreserveNewlines(text)
		} else if shouldCollapseWhitespace(k) {
			useMerged = true
		}
		if text == "" {
			k.Node = nil
			return
		}
		fontSize := k.Style.FontSize
		slot := k.Style.FontSlot()
		lineH, contentH := runHeights(a.Metrics, fontSize, slot, k.Style.LineHeight)
		letterSpacing := k.Style.LetterSpacing
		wordSpacing := k.Style.WordSpacing
		nowrap := k.Style.WhiteSpace == style.WhiteSpaceNowrap ||
			k.Style.WhiteSpace == style.WhiteSpacePre ||
			k.Style.WhiteSpace == style.WhiteSpacePrewrite ||
			k.Style.TextWrap == "nowrap"
		var words []string
		if useMerged {
			words = splitCollapsedWords(text)
		} else {
			words = splitWords(text)
		}
		if !nowrap {
			if k.Style.WordBreak == "break-all" {
				var charWords []string
				for _, w := range words {
					if w == " " || w == "\n" {
						charWords = append(charWords, w)
					} else {
						for _, r := range w {
							charWords = append(charWords, string(r))
						}
					}
				}
				words = charWords
			} else {
				if k.Style.WordBreak != "keep-all" {
					words = splitCJKWords(words, bc.avail, fontSize, a.Metrics, slot, letterSpacing)
				}
				words = splitSoftBreaks(words, bc.avail, fontSize, a.Metrics, slot, letterSpacing)
				words = splitOverlongWords(words, fontSize, a.Metrics, slot, letterSpacing)
			}
		}
		var firstWordID ObjectID
		var lastWordID ObjectID
		tabSpaces := int(k.Style.TabSize)
		if tabSpaces <= 0 {
			tabSpaces = 8
		}
		for _, word := range words {
			if word == "\n" {
				if current.w > 0 || len(current.runs) > 0 {
					trimTrailingSpaces(a, current)
					if len(current.runs) > 0 {
						*lines = append(*lines, *current)
						bc.nextLine(current.h)
					}
					*current = lineBox{}
				}
				continue
			}
			wordW := measureWord(word, fontSize, a.Metrics, slot, letterSpacing)
			if word == " " {
				wordW += wordSpacing
			} else if word == "\t" {
				singleSpaceW := measureWord(" ", fontSize, a.Metrics, slot, letterSpacing) + wordSpacing
				wordW = float32(tabSpaces) * singleSpaceW
			}
			if word == " " && len(current.runs) == 0 {
				continue
			}
			if !nowrap && current.w+wordW > bc.avail && current.w > 0 {
				trimTrailingSpaces(a, current)
				if len(current.runs) > 0 {
					*lines = append(*lines, *current)
					bc.nextLine(current.h)
				}
				*current = lineBox{}
			}
			wordID, wordObj := a.Alloc()
			nodeCopy := *k.Node
			nodeCopy.DataContent = word
			wordObj.Node = &nodeCopy
			if a.wordSeq < ^uint16(0) {
				a.setKey(wordID, NewBoxKey(uint32(nodeCopy.ID), BoxKindWord, a.wordSeq))
				a.wordSeq++
			}
			wordObj.Style = k.Style
			wordObj.Parent = a.Get(nodeID).Parent
			if lastWordID == 0 {
				firstWordID = wordID
			} else {
				a.Get(lastWordID).NextSibling = wordID
				wordObj.PrevSibling = lastWordID
			}
			lastWordID = wordID
			current.runs = append(current.runs, textRun{
				text:     word,
				size:     fontSize,
				lineH:    lineH,
				contentH: contentH,
				obj:      wordID,
				slot:     slot,
				w:        wordW,
			})
			current.w += wordW
			current.h = maxF(current.h, lineH)
		}
		if lastWordID != 0 {
			k = a.Get(nodeID)
			parentID := k.Parent
			prev := k.PrevSibling
			next := k.NextSibling
			if prev != 0 {
				a.Get(prev).NextSibling = firstWordID
				a.Get(firstWordID).PrevSibling = prev
			} else {
				a.Get(parentID).FirstKid = firstWordID
			}
			if next != 0 {
				a.Get(next).PrevSibling = lastWordID
				a.Get(lastWordID).NextSibling = next
			} else {
				a.Get(parentID).LastKid = lastWordID
			}
			k.Node = nil
		}
		return
	}
	k = a.Get(nodeID)
	if k.Node != nil && k.Node.Element() && k.Node.Data == "img" {
		atomicInlineReplaced(a, nodeID, lines, current, bc)
		return
	}
	k = a.Get(nodeID)
	for child := k.FirstKid; child != 0; child = a.Get(child).NextSibling {
		collectInline(a, child, lines, current, bc)
	}
}

type lineBox struct {
	runs     []textRun
	trailing []textRun
	w, h     float32
}

func trimTrailingSpaces(a *Arena, lb *lineBox) {
	for len(lb.runs) > 0 {
		last := lb.runs[len(lb.runs)-1]
		if !isSpaceWord(last.text) {
			return
		}
		if s := a.Get(last.obj).Style; s != nil {
			switch s.WhiteSpace {
			case style.WhiteSpacePre, style.WhiteSpacePrewrite, style.WhiteSpacePreline:
				return
			}
		}
		lb.w -= last.w
		lb.trailing = append(lb.trailing, last)
		lb.runs = lb.runs[:len(lb.runs)-1]
	}
}

func balanceLines(a *Arena, lines []lineBox, contentW float32) []lineBox {
	if len(lines) <= 1 || len(lines) > 10 || contentW <= 0 {
		return lines
	}
	var allRuns []textRun
	totalW := float32(0)
	for _, l := range lines {
		allRuns = append(allRuns, l.runs...)
		for i := len(l.trailing) - 1; i >= 0; i-- {
			allRuns = append(allRuns, l.trailing[i])
			totalW += l.trailing[i].w
		}
		totalW += l.w
	}
	targetW := totalW / float32(len(lines))
	var newLines []lineBox
	var cur lineBox
	curH := lines[0].h
	for _, r := range allRuns {
		if cur.w > 0 && len(cur.runs) > 0 && cur.w+r.w > targetW*1.15 && len(newLines) < len(lines)-1 {
			trimTrailingSpaces(a, &cur)
			newLines = append(newLines, cur)
			cur = lineBox{h: curH}
			if isSpaceWord(r.text) {
				continue
			}
		}
		cur.runs = append(cur.runs, r)
		cur.w += r.w
		if r.lineH > cur.h {
			cur.h = r.lineH
		}
	}
	if len(cur.runs) > 0 {
		trimTrailingSpaces(a, &cur)
		newLines = append(newLines, cur)
	}
	if len(newLines) == len(lines) {
		return newLines
	}
	return lines
}

type textRun struct {
	text     string
	size     float32
	lineH    float32
	contentH float32
	obj      ObjectID
	slot     frame.FontSlot
	w        float32
	atomic   bool
	srcX     float32
	srcY     float32
}

const (
	atomicOnBaseline = iota
	atomicToTop
	atomicToBottom
	atomicToTextTop
	atomicToTextBottom
	atomicToLength
	atomicToScript
	atomicToMiddle
)

type atomicBox struct {
	extent float32
	align  int
	below  float32
	length float32
}

func atomicAlign(va string) (int, float32) {
	switch va {
	case "", "baseline":
		return atomicOnBaseline, 0
	case "super":
		return atomicToScript, 1
	case "sub":
		return atomicToScript, -1
	case "top":
		return atomicToTop, 0
	case "bottom":
		return atomicToBottom, 0
	case "text-top":
		return atomicToTextTop, 0
	case "text-bottom":
		return atomicToTextBottom, 0
	case "middle":
		return atomicToMiddle, 0
	}
	if n, ok := alignLength(va); ok {
		return atomicToLength, n
	}
	return atomicToBottom, 0
}

func alignLength(va string) (float32, bool) {
	s := strings.TrimSpace(va)
	if !strings.HasSuffix(s, "px") {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "px")), 32)
	if err != nil {
		return 0, false
	}
	return float32(n), true
}

func resolveScript(a *Arena, k *Object, b *atomicBox, sm strutMetrics) {
	if b.align != atomicToScript {
		return
	}
	if isRubyAnnotation(k, a) {
		b.align, b.length = atomicOnBaseline, 0
		return
	}
	if b.length > 0 {
		b.length = sm.sup
	} else {
		b.length = -sm.sub
	}
	b.align = atomicToLength
}

func boxOfAtomic(a *Arena, id ObjectID, sm strutMetrics) atomicBox {
	k := a.Get(id)
	va := ""
	if k.Style != nil {
		va = k.Style.VerticalAlign
	}
	b := atomicBox{extent: k.MarginTop + k.BorderH() + k.MarginBottom}
	b.align, b.length = atomicAlign(va)
	resolveScript(a, k, &b, sm)
	if (b.align != atomicOnBaseline && b.align != atomicToLength) || k.Style == nil ||
		(!isFormControl(k) && (isReplacedElement(k) || !hasInFlowLineBoxes(a, id))) {
		return b
	}
	if isAppearanceWidget(k) {
		b.below = k.MarginBottom
		return b
	}
	asc, desc, lineH := fontHeights(a.Metrics, k.Style.FontSize, k.Style.FontSlot(), k.Style.LineHeight)
	_, below := strutSplit(asc, desc, lineH)
	b.below = below + k.PaddingBottom + k.BorderBottom + k.MarginBottom
	if b.below > b.extent {
		b.below = b.extent
	}
	return b
}

func isFormControl(k *Object) bool {
	if k.Node == nil || !k.Node.Element() {
		return false
	}
	switch k.Node.Data {
	case "select":
		return true
	case "input":
		return strings.ToLower(k.Node.GetAttribute("type")) != "image"
	}
	return false
}

func isAppearanceWidget(k *Object) bool {
	if k.Node == nil || k.Node.Data != "input" {
		return false
	}
	switch strings.ToLower(k.Node.GetAttribute("type")) {
	case "checkbox", "radio", "range":
		return true
	}
	return false
}

func hasInFlowLineBoxes(a *Arena, id ObjectID) bool {
	left := 256
	var found bool
	var walk func(ObjectID)
	walk = func(cur ObjectID) {
		if found || left <= 0 {
			return
		}
		left--
		k := a.Get(cur)
		if k.Style != nil && k.Style.Display == style.DisplayNone {
			return
		}
		if k.Node != nil && k.Node.Text() {
			if strings.TrimSpace(k.Node.DataContent) != "" {
				found = true
				return
			}
		}
		for c := k.FirstKid; c != 0; c = a.Get(c).NextSibling {
			walk(c)
		}
	}
	walk(id)
	return found
}

type strutMetrics struct {
	above, below float32
	asc, desc    float32
	xh           float32
	sup, sub     float32
}

func ofStrut(asc, desc, lineH, xh, sizePx float32) strutMetrics {
	above, below := strutSplit(asc, desc, lineH)
	trunc := func(v float32) float32 { return float32(int64(v*64)) / 64 }
	return strutMetrics{
		above: above, below: below, asc: asc, desc: desc, xh: xh,
		sup: trunc(sizePx/3) + 1, sub: trunc(sizePx/5) + 1,
	}
}

func faceXHeight(m Metrics, size float32, slot frame.FontSlot) float32 {
	if m == nil || size <= 0 {
		return 0
	}
	if v := m.XHeightFixed(int32(size), slot); v > 0 {
		return float32(v) / 64
	}
	return size * 0.5
}

type lineFrame struct {
	base   float32
	height float32
	sm     strutMetrics
}

type runDemand struct {
	above  float32
	below  float32
	asc    float32
	half   float32
	box    atomicBox
	atomic bool
	edge   bool
	bottom bool
}

func atomicDemand(b atomicBox, sm strutMetrics) runDemand {
	d := runDemand{box: b, atomic: true}
	switch b.align {
	case atomicOnBaseline:
		d.above, d.below = b.extent-b.below, b.below
	case atomicToLength:
		d.above, d.below = b.extent-b.below+b.length, b.below-b.length
	case atomicToTextTop:
		d.above, d.below = sm.asc, b.extent-sm.asc
	case atomicToTextBottom:
		d.above, d.below = b.extent-sm.desc, sm.desc
	case atomicToMiddle:
		d.above, d.below = sm.xh/2+b.extent/2, b.extent/2-sm.xh/2
	default:
		d.edge = true
		d.bottom = b.align != atomicToTop
	}
	return d
}

func demandOfRun(a *Arena, r textRun, sm strutMetrics) runDemand {
	if r.atomic {
		return atomicDemand(boxOfAtomic(a, r.obj, sm), sm)
	}
	asc, desc, lineH := fontHeights(a.Metrics, r.size, r.slot, r.lineH)
	k := a.Get(r.obj)
	va := ""
	if k.Style != nil {
		va = k.Style.VerticalAlign
	}
	b := atomicBox{extent: lineH}
	b.align, b.length = atomicAlign(va)
	resolveScript(a, k, &b, sm)
	b.below = lineH - (asc + (lineH-(asc+desc))/2)
	d := atomicDemand(b, sm)
	d.asc, d.half, d.box = asc, (lineH-(asc+desc))/2, b
	return d
}

func runDemands(a *Arena, runs []textRun, sm strutMetrics) []runDemand {
	if len(runs) == 0 {
		return nil
	}
	ds := make([]runDemand, len(runs))
	for i, r := range runs {
		ds[i] = demandOfRun(a, r, sm)
	}
	return ds
}

func demandForRun(ds []runDemand, runs []textRun, id ObjectID) runDemand {
	for i, r := range runs {
		if r.obj == id && i < len(ds) {
			return ds[i]
		}
	}
	return runDemand{}
}

func lineExtent(sm strutMetrics, ds []runDemand) lineFrame {
	base, below := sm.above, sm.below
	for _, d := range ds {
		if d.edge {
			continue
		}
		if d.above > base {
			base = d.above
		}
		if d.below > below {
			below = d.below
		}
	}
	height := base + below
	topStretch, bottomStretch := height, height
	for _, d := range ds {
		if !d.edge {
			continue
		}
		if d.bottom {
			if d.box.extent > bottomStretch {
				bottomStretch = d.box.extent
			}
			continue
		}
		if d.box.extent > topStretch {
			topStretch = d.box.extent
		}
	}
	if bottomStretch > height {
		height = bottomStretch
		if b := height - below; b > base {
			base = b
		}
	}
	if topStretch > height {
		height = topStretch
	}
	return lineFrame{base: base, height: height, sm: sm}
}

func (f lineFrame) atomicTop(b atomicBox) float32 {
	switch b.align {
	case atomicOnBaseline:
		return f.base - (b.extent - b.below)
	case atomicToLength:
		return f.base - (b.extent - b.below) - b.length
	case atomicToTextTop:
		return f.base - f.sm.asc
	case atomicToTextBottom:
		return f.base + f.sm.desc - b.extent
	case atomicToMiddle:
		return f.base - f.sm.xh/2 - b.extent/2
	case atomicToTop:
		return 0
	default:
		return f.height - b.extent
	}
}

func atomicInlineBlock(a *Arena, id ObjectID, lines *[]lineBox, current *lineBox, bc *breakCtx) {
	blockInto(a, id, bc.contentW)
	inlineInto(a, id)
	k := a.Get(id)
	intrinsicTextarea := k.Node != nil && k.Node.Data == "textarea"
	if k.Style != nil && !intrinsicTextarea && resolveBoxWidth(a, k.Style, bc.contentW) < 0 {
		if w := itemMaxContentW(a, id); w > 0 && w < k.W {
			k.W = w
			k = a.Get(id)
			clampWidth(a, k, bc.contentW)
			k = a.Get(id)
			clearInlineLaidOut(a, id)
			blockIntoSized(a, id, k.W)
			inlineInto(a, id)
			k = a.Get(id)
		}
	}
	boxW := k.W + k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
	lineW := boxW + k.MarginLeft + k.MarginRight
	lineH := k.BorderH() + k.MarginTop + k.MarginBottom
	if len(current.runs) > 0 && current.w+lineW > bc.avail {
		trimTrailingSpaces(a, current)
		if len(current.runs) > 0 {
			*lines = append(*lines, *current)
			bc.nextLine(current.h)
		}
		*current = lineBox{}
	}
	current.runs = append(current.runs, textRun{
		obj:      id,
		atomic:   true,
		w:        lineW,
		lineH:    lineH,
		contentH: lineH,
		srcX:     k.X,
		srcY:     k.Y,
	})
	current.w += lineW
	current.h = maxF(current.h, lineH)
}

func atomicInlineReplaced(a *Arena, id ObjectID, lines *[]lineBox, current *lineBox, bc *breakCtx) {
	k := a.Get(id)
	if s := k.Style; s != nil {
		k.PaddingTop = resolvePctLength(s.PaddingTop, bc.contentW)
		k.PaddingRight = resolvePctLength(s.PaddingRight, bc.contentW)
		k.PaddingBottom = resolvePctLength(s.PaddingBottom, bc.contentW)
		k.PaddingLeft = resolvePctLength(s.PaddingLeft, bc.contentW)
		ubw := s.UsedBorderWidths()
		k.BorderTop, k.BorderRight = ubw[0], ubw[1]
		k.BorderBottom, k.BorderLeft = ubw[2], ubw[3]
		margins := [4]float32{s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft}
		dst := [4]*float32{&k.MarginTop, &k.MarginRight, &k.MarginBottom, &k.MarginLeft}
		auto := [4]bool{false, s.MarginRightAuto, false, s.MarginLeftAuto}
		for i, m := range margins {
			if auto[i] {
				*dst[i] = 0
			} else {
				*dst[i] = resolvePctLength(m, bc.contentW)
			}
		}
		k = a.Get(id)
	}
	w, h, ok := replacedSize(a, k, bc.contentW)
	if !ok {
		return
	}
	k.W = w
	k.H = h
	boxW := k.W + k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
	lineW := boxW + k.MarginLeft + k.MarginRight
	lineH := k.BorderH() + k.MarginTop + k.MarginBottom
	if len(current.runs) > 0 && current.w+lineW > bc.avail {
		trimTrailingSpaces(a, current)
		if len(current.runs) > 0 {
			*lines = append(*lines, *current)
			bc.nextLine(current.h)
		}
		*current = lineBox{}
	}
	current.runs = append(current.runs, textRun{
		obj:      id,
		atomic:   true,
		w:        lineW,
		lineH:    lineH,
		contentH: lineH,
		srcX:     k.X,
		srcY:     k.Y,
	})
	current.w += lineW
	current.h = maxF(current.h, lineH)
}

func runHeights(m Metrics, size float32, slot frame.FontSlot, styleLineHeight float32) (lineH, contentH float32) {
	asc, desc, lineH := fontHeights(m, size, slot, styleLineHeight)
	return lineH, asc + desc
}

func fontHeights(m Metrics, size float32, slot frame.FontSlot, styleLineHeight float32) (asc, desc, lineH float32) {
	normal := float32(0)
	if m != nil {
		a, d, h := m.LineMetrics(int32(size), slot)
		asc, desc, normal = float32(a), float32(d), float32(h)
	}
	if asc+desc == 0 {
		asc, desc, normal = size*0.8, size*0.2, size*1.2
	}
	lineH = normal
	if styleLineHeight > 0 {
		lineH = styleLineHeight
	}
	return asc, desc, lineH
}

func strutSplit(asc, desc, lineH float32) (above, below float32) {
	half := lineH - (asc + desc)
	above = asc + half/2
	return above, lineH - above
}

const maxWordFragment = float32(1 << 16)

func splitOverlongWords(words []string, fontSize float32, m Metrics, slot frame.FontSlot, letterSpacing float32) []string {
	advance := func(r rune) float32 {
		if m == nil {
			return fontSize * 0.5
		}
		return float32(int64(m.GlyphAdvanceFixed(int32(fontSize), r, slot))) / 64
	}
	bounded := func(word string) bool {
		return word == " " || word == "\n" || measureWord(word, fontSize, m, slot, letterSpacing) <= maxWordFragment
	}
	for _, word := range words {
		if !bounded(word) {
			return breakOverlongWords(words, bounded, advance, letterSpacing)
		}
	}
	return words
}

func breakOverlongWords(words []string, bounded func(string) bool, advance func(rune) float32, letterSpacing float32) []string {
	out := make([]string, 0, len(words)+8)
	for _, word := range words {
		if bounded(word) {
			out = append(out, word)
			continue
		}
		runes := []rune(word)
		start, sum, count := 0, float32(0), 0
		for i := range runes {
			next := sum + advance(runes[i])
			if count > 0 {
				next += letterSpacing
			}
			if count > 0 && next > maxWordFragment {
				out = append(out, string(runes[start:i]))
				start, sum, count = i, advance(runes[i]), 1
				continue
			}
			sum, count = next, count+1
		}
		out = append(out, string(runes[start:]))
	}
	return out
}

func cjkBreakRune(r rune) bool {
	return (r >= 0x3040 && r <= 0x30FF) ||
		(r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF)
}

func kinsokuNoLineStart(r rune) bool {
	switch r {
	case 0x3001, 0x3002, 0x30FC:
		return true
	case 0xFF01, 0xFF09, 0xFF0C, 0xFF0E, 0xFF1A, 0xFF1B, 0xFF1F, 0xFF3D, 0xFF5D:
		return true
	case 0x3009, 0x300B, 0x300D, 0x300F, 0x3011, 0x3015:
		return true
	case 0x3041, 0x3043, 0x3045, 0x3047, 0x3048, 0x3063, 0x3083, 0x3085, 0x3087, 0x308E, 0x3095, 0x3096:
		return true
	case 0x30A1, 0x30A3, 0x30A5, 0x30A7, 0x30A9, 0x30C3, 0x30E3, 0x30E5, 0x30E7, 0x30EE, 0x30F5, 0x30F6:
		return true
	case 0x3005, 0x309D, 0x309E, 0x30FD, 0x30FE:
		return true
	case 0x2026, 0x2025, 0x3003:
		return true
	}
	return false
}

func splitCJKWords(words []string, avail, fontSize float32, m Metrics, slot frame.FontSlot, letterSpacing float32) []string {
	overlong := false
	for _, w := range words {
		if w != " " && w != "\n" && strings.ContainsFunc(w, cjkBreakRune) &&
			measureWord(w, fontSize, m, slot, letterSpacing) > avail {
			overlong = true
			break
		}
	}
	if !overlong {
		return words
	}
	out := make([]string, 0, len(words)+8)
	for _, word := range words {
		if word == " " || word == "\n" || !strings.ContainsFunc(word, cjkBreakRune) ||
			measureWord(word, fontSize, m, slot, letterSpacing) <= avail {
			out = append(out, word)
			continue
		}
		runes := []rune(word)
		start := 0
		for i := 0; i < len(runes); i++ {
			if !cjkBreakRune(runes[i]) {
				continue
			}
			end := i + 1
			for end < len(runes) && unicode.IsMark(runes[end]) {
				end++
			}
			for end < len(runes) && kinsokuNoLineStart(runes[end]) {
				end++
				for end < len(runes) && unicode.IsMark(runes[end]) {
					end++
				}
			}
			if end == len(runes) {
				break
			}
			out = append(out, string(runes[start:end]))
			start = end
			i = end - 1
		}
		out = append(out, string(runes[start:]))
	}
	return out
}

func softBreakRune(r rune) bool {
	return r == '\u00AD' || r == '\u200B'
}

func splitSoftBreaks(words []string, avail, fontSize float32, m Metrics, slot frame.FontSlot, letterSpacing float32) []string {
	overlong := false
	for _, w := range words {
		if w != " " && w != "\n" && strings.ContainsFunc(w, softBreakRune) &&
			measureWord(w, fontSize, m, slot, letterSpacing) > avail {
			overlong = true
			break
		}
	}
	if !overlong {
		return words
	}
	out := make([]string, 0, len(words)+8)
	for _, word := range words {
		if word == " " || word == "\n" || !strings.ContainsFunc(word, softBreakRune) ||
			measureWord(word, fontSize, m, slot, letterSpacing) <= avail {
			out = append(out, word)
			continue
		}
		runes := []rune(word)
		start := 0
		for i := 0; i < len(runes); i++ {
			if !softBreakRune(runes[i]) {
				continue
			}
			end := i + 1
			for end < len(runes) && unicode.IsMark(runes[end]) {
				end++
			}
			if end == len(runes) {
				break
			}
			out = append(out, string(runes[start:end]))
			start = end
			i = end - 1
		}
		out = append(out, string(runes[start:]))
	}
	return out
}

func applyTakenSoftHyphens(a *Arena, lines []lineBox) {
	if len(lines) < 2 {
		return
	}
	for i := 0; i < len(lines)-1; i++ {
		line := &lines[i]
		if len(line.runs) == 0 {
			continue
		}
		last := &line.runs[len(line.runs)-1]
		if last.atomic || !strings.HasSuffix(last.text, "\u00AD") {
			continue
		}
		replaced := strings.TrimSuffix(last.text, "\u00AD") + "-"
		letterSpacing := float32(0)
		obj := a.Get(last.obj)
		if obj.Style != nil {
			letterSpacing = obj.Style.LetterSpacing
		}
		newW := measureWord(replaced, last.size, a.Metrics, last.slot, letterSpacing)
		line.w += newW - last.w
		last.w = newW
		last.text = replaced
		if obj.Node != nil {
			obj.Node.DataContent = replaced
		}
	}
}

func measureWord(word string, fontSize float32, m Metrics, slot frame.FontSlot, letterSpacing float32) float32 {
	n := utf8.RuneCountInString(word)
	if n == 0 {
		return 0
	}
	extra := float32(n-1) * letterSpacing
	if m == nil {
		return float32(n)*fontSize*0.5 + extra
	}
	sum := int64(0)
	for _, r := range word {
		sum += int64(m.GlyphAdvanceFixed(int32(fontSize), r, slot))
	}
	return float32(sum)/64 + extra
}

func splitCollapsedWords(text string) []string {
	nword, inWord := 0, false
	for i := 0; i < len(text); i++ {
		if b := text[i]; b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == 0x0b || b == 0x0c {
			inWord = false
		} else if !inWord {
			inWord = true
			nword++
		}
	}
	out := make([]string, 0, 2*nword+1)
	start := -1
	spaced := false
	for i, r := range text {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, text[start:i])
				start = -1
			}
			if !spaced {
				out = append(out, " ")
				spaced = true
			}
		} else {
			if start < 0 {
				start = i
			}
			spaced = false
		}
	}
	if start >= 0 {
		out = append(out, text[start:])
	}
	return out
}

func splitWords(text string) []string {
	var words []string
	var current strings.Builder
	for _, r := range text {
		if r == '\n' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, "\n")
		} else if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func shouldCollapseWhitespace(obj *Object) bool {
	if obj.Style == nil {
		return true
	}
	switch obj.Style.WhiteSpace {
	case style.WhiteSpacePre, style.WhiteSpacePrewrite:
		return false
	}
	return true
}

func isSpaceWord(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return len(s) > 0
}

func collapseWhitespace(s string) string {
	var out strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				out.WriteRune(' ')
				prevSpace = true
			}
		} else {
			out.WriteRune(r)
			prevSpace = false
		}
	}
	return out.String()
}

func collapseWhitespacePreserveNewlines(s string) string {
	var out strings.Builder
	prevSpace := true
	for _, r := range s {
		if r == '\n' {
			out.WriteRune('\n')
			prevSpace = true
		} else if unicode.IsSpace(r) {
			if !prevSpace {
				out.WriteRune(' ')
				prevSpace = true
			}
		} else {
			out.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimRight(out.String(), " \t")
}

func isRubyAnnotation(obj *Object, a *Arena) bool {
	for curr := obj; curr != nil; {
		if curr.Node != nil {
			if curr.Node.DataAtom == dom.AtomRt {
				return true
			}
			if curr.Node.Parent != nil && curr.Node.Parent.DataAtom == dom.AtomRt {
				return true
			}
		}
		if curr.Parent == 0 {
			break
		}
		curr = a.Get(curr.Parent)
	}
	return false
}

func applyTextTransform(text string, transform string) string {
	if text == "" || transform == "" || transform == "none" {
		return text
	}
	switch transform {
	case "uppercase":
		return strings.ToUpper(text)
	case "lowercase":
		return strings.ToLower(text)
	case "capitalize":
		return strings.Title(text)
	case "full-width":
		var b strings.Builder
		for _, r := range text {
			if r >= 0x21 && r <= 0x7E {
				b.WriteRune(r + 0xFEE0)
			} else if r == 0x20 {
				b.WriteRune(0x3000)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	case "full-size-kana":
		var b strings.Builder
		for _, r := range text {
			switch r {
			case 'ぁ':
				b.WriteRune('あ')
			case 'ぃ':
				b.WriteRune('い')
			case 'ぅ':
				b.WriteRune('う')
			case 'ぇ':
				b.WriteRune('え')
			case 'ぉ':
				b.WriteRune('お')
			case 'っ':
				b.WriteRune('つ')
			case 'ゃ':
				b.WriteRune('や')
			case 'ゅ':
				b.WriteRune('ゆ')
			case 'ょ':
				b.WriteRune('よ')
			case 'ァ':
				b.WriteRune('ア')
			case 'ィ':
				b.WriteRune('イ')
			case 'ゥ':
				b.WriteRune('ウ')
			case 'ェ':
				b.WriteRune('エ')
			case 'ォ':
				b.WriteRune('オ')
			case 'ッ':
				b.WriteRune('ツ')
			case 'ャ':
				b.WriteRune('ヤ')
			case 'ュ':
				b.WriteRune('ユ')
			case 'ョ':
				b.WriteRune('ヨ')
			default:
				b.WriteRune(r)
			}
		}
		return b.String()
	default:
		return text
	}
}

func applyFirstLineRuns(a *Arena, line *lineBox, flStyle, containerStyle *style.ComputedStyle) {
	if line == nil || flStyle == nil {
		return
	}
	newW := float32(0)
	newH := line.h
	for i := range line.runs {
		run := &line.runs[i]
		runObj := a.Get(run.obj)
		if run.atomic {
			newW += run.w
			continue
		}
		base := runObj.Style
		if base == nil {
			base = containerStyle
		}
		if base == nil {
			continue
		}
		overridden := *base
		overridden.FontFamily = flStyle.FontFamily
		overridden.CustomFamily = flStyle.CustomFamily
		overridden.CustomFontIdx = flStyle.CustomFontIdx
		overridden.FontSize = flStyle.FontSize
		overridden.FontWeight = flStyle.FontWeight
		overridden.FontStyle = flStyle.FontStyle
		overridden.Color = flStyle.Color
		overridden.BackgroundColor = flStyle.BackgroundColor
		overridden.BackgroundGradient = flStyle.BackgroundGradient
		overridden.BackgroundRadialGradient = flStyle.BackgroundRadialGradient
		overridden.BackgroundImage = flStyle.BackgroundImage
		overridden.BackgroundRepeat = flStyle.BackgroundRepeat
		overridden.BackgroundRepeatY = flStyle.BackgroundRepeatY
		overridden.BackgroundSize = flStyle.BackgroundSize
		overridden.BackgroundOrigin = flStyle.BackgroundOrigin
		overridden.BackgroundClip = flStyle.BackgroundClip
		overridden.BgSizeW = flStyle.BgSizeW
		overridden.BgSizeH = flStyle.BgSizeH
		overridden.BgSizeWPct = flStyle.BgSizeWPct
		overridden.BgSizeHPct = flStyle.BgSizeHPct
		overridden.BackgroundPosXMode = flStyle.BackgroundPosXMode
		overridden.BackgroundPosYMode = flStyle.BackgroundPosYMode
		overridden.BackgroundPosX = flStyle.BackgroundPosX
		overridden.BackgroundPosY = flStyle.BackgroundPosY
		overridden.BgPosXPct = flStyle.BgPosXPct
		overridden.BgPosYPct = flStyle.BgPosYPct
		overridden.TextDecoration = flStyle.TextDecoration
		overridden.TextTransform = flStyle.TextTransform
		overridden.LetterSpacing = flStyle.LetterSpacing
		overridden.WordSpacing = flStyle.WordSpacing
		if flStyle.LineHeight > 0 {
			overridden.LineHeight = flStyle.LineHeight
		}
		runObj.Style = &overridden

		newSize := overridden.FontSize
		newSlot := overridden.FontSlot()
		newLetterSpacing := overridden.LetterSpacing
		newWordSpacing := overridden.WordSpacing
		lineH, contentH := runHeights(a.Metrics, newSize, newSlot, overridden.LineHeight)
		wordW := measureWord(run.text, newSize, a.Metrics, newSlot, newLetterSpacing)
		if run.text == " " {
			wordW += newWordSpacing
		}
		run.size = newSize
		run.slot = newSlot
		run.lineH = lineH
		run.contentH = contentH
		run.w = wordW
		runObj.W = wordW
		runObj.H = contentH
		newW += wordW
		if lineH > newH {
			newH = lineH
		}
	}
	line.w = newW
	line.h = newH
}

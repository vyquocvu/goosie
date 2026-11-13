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

// floatIntrusion is one float box narrowing the lines it overlaps, in
// absolute coordinates over the float's margin box: line boxes avoid floats
// by their margins, the way block boxes do.
type floatIntrusion struct {
	x0, x1, y0, y1 float32
	isLeft         bool
	shapeOutside   string
	shapeMargin    float32
}

// breakCtx carries float intrusions and the Y cursor for one inline
// container's line breaking. When no float intrudes every query answers the
// full width at zero offset, which is exactly the old behavior.
type breakCtx struct {
	contentW float32 // full content width, the fallback span
	x0       float32 // absolute content-box left
	strut    float32 // container line-height floor for overlap tests
	in       []floatIntrusion
	yTop     float32 // absolute top of the line being built
	xoff     float32 // left inset of the line being built
	avail    float32 // available width of the line being built
}

// collectFloatIntrusions gathers the floats whose margin boxes can overlap
// the lines of id: the container's own float kids (text flowing directly in
// a float container) and float kids of its ancestors up to the nearest
// formatting-context boundary (a block kid's lines beside its uncles). The
// subtree holding id's own lines is skipped, so a float never intrudes into
// itself. Inline layout runs after block layout, so every float box is final.
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

// spanAt returns the left inset and available width for a line at yTop with
// estimated height h, without moving any cursor: the placement pass uses it
// per line. An empty intrusion list answers the full span.
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

		// A float starting at or left of the content edge pushes the line
		// right; one ending at or past the right edge pulls its end left. A
		// float wider than the content does both, leaving nothing.
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

// startLine settles the line beginning at yTop: when floats leave it no
// width the line slides below the shallowest overlapping float, the way
// content flows under a float it cannot fit beside. Bounded: each slide ends
// a float overlap.
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

// nextLine advances past a completed line of height h and settles the next.
func (bc *breakCtx) nextLine(h float32) {
	if h < bc.strut {
		h = bc.strut
	}
	bc.startLine(bc.yTop+h, bc.strut)
}

// Inline runs the inline layout pass over the arena.
//
// Inline layout builds line boxes from inline-level content: text nodes and
// inline elements. Each line box is a horizontal strip of glyphs; when content
// exceeds the containing block's width, it wraps to a new line. The pass runs
// after block layout so every inline box knows its containing block's width.
func Inline(a *Arena, root ObjectID) {
	// The capture map was reset and inline borders resolved at Block entry,
	// where inlineInto first runs. Re-resolve edges here so percentage padding
	// picks up the now-known block widths before fragments are finalized; do not
	// reset the map, which already holds the captured per-line rects.
	resolveInlineEdges(a, root, a.Get(root).W)
	inlineInto(a, root)
	// Give display:inline element boxes a box of their own. The line pass above
	// positions only the word/replaced fragments under an inline element, never
	// the element itself, so an inline <a> would otherwise stay at the zero rect
	// and be unhittable, invisible to hover, and reported with no geometry.
	inlineSelfRects(a, root)
}

// inlineSelfRects walks the tree and sets every display:inline element box to
// the union of the laid-out fragments beneath it, so the inline element owns a
// real border rect the way a browser does. Post-order, so a nested inline
// element's fragments (and its own box) are resolved before its parent unions
// them.
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
	// Fold per-line border fragments before sizing the union box: children are
	// already finalized (post-order), so a nested inline's borders union in.
	a.finalizeInlineFragments(id)
	x0, y0, x1, y1, ok := inlineFragmentBounds(a, id)
	if !ok {
		return
	}
	// Set the content box so BorderRect() equals the union, subtracting this
	// inline box's own padding/border thicknesses (usually zero for <a>).
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

// inlineFragmentBounds unions the border rects of the positioned non-block
// descendants of id: word fragments, replaced images, and nested inline boxes.
// Block/inline-block descendants are atomic slots handled by block layout, so
// they are skipped here just as inlineExtent skips them.
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

// resolveInlineEdges populates the border and padding widths of display:inline
// element boxes from their style. The block pass resolves only block-level
// children, so a pure inline element's arena Border*/Padding* are otherwise zero
// and its border could never paint. containingW is the nearest block container's
// content width, which inline percentage padding resolves against. Borderless,
// paddingless inline (the whole existing corpus) resolves to zero, so nothing
// changes for it; BorderRect() stays the content union either way.
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

// inlineFragmentGenerator resolves the display:inline element that owns a word
// fragment, by DOM parent rather than arena parent: layout reparents runs into
// anonymous boxes, so the box a run hangs off is frequently anonymous while the
// element that owns the copied style is the text node's DOM parent. Mirrors
// paint's fragmentOwnsDecorations. Returns false when the generator is not an
// inline element (e.g. text sitting directly in a block).
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

// recordInlineFrag unions a word fragment's content rect into the generating
// element's per-line accumulation.
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

// unionRectF returns the smallest rect covering both inputs (frame.RectF has no
// Union method; the int32 frame.Rect one does not apply to CSS-px floats).
func unionRectF(a, b frame.RectF) frame.RectF {
	return frame.RectF4(minF(a.X0, b.X0), minF(a.Y0, b.Y0), maxF(a.X1, b.X1), maxF(a.Y1, b.Y1))
}

// finalizeInlineFragments folds the per-line accumulation into obj.InlineFragments.
// Post-order: each inline child's fragments are already expanded border boxes, so
// unioning them per line gives this box's content area including nested inline
// borders. Each line's union is then expanded outward by this box's border and
// padding: top/bottom on every fragment, the inline-start edge on the first and
// the inline-end edge on the last, mapped to physical edges by direction.
func (a *Arena) finalizeInlineFragments(id ObjectID) {
	// Nothing captured anywhere (the whole borderless-inline corpus): return
	// before allocating, so layout alloc counters stay identical for the gates.
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
	// The block's own font and line-height form the strut; it is needed before
	// the first line breaks (float overlap tests) as well as at placement.
	strut := float32(0)
	var sm strutMetrics
	if obj.Style != nil {
		asc, desc, strutLine := fontHeights(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot(), obj.Style.LineHeight)
		strut = strutLine
		sm = ofStrut(asc, desc, strutLine,
			faceXHeight(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot()), obj.Style.FontSize)
	}
	// Floats intruding into this container shorten the lines overlapping
	// them. Siblings of the container's own text and uncles of nested blocks
	// both count, up to the nearest formatting-context boundary.
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
				// A flex or grid container lays its own content out and sizes
				// it; descending here would re-split the word objects it made.
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
				// A break with nothing before it still occupies a line: the empty
				// line box is a lone <br>'s whole contribution to the flow, so it
				// is sized from the container's own font rather than from a run.
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
		// Inline-level child: either a text node or an inline element (<a>, <span>, <b>).
		// Collect its text runs into the enclosing block's current line box so they are
		// positioned sequentially inside this block container.
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
	// Apply ::first-line style overrides to the first formatted line. The
	// pseudo-element is matched on the block container's originating element;
	// if present, every run on lines[0] gets its font, color, background,
	// text-decoration, text-transform, letter-spacing, word-spacing, and
	// line-height replaced by the first-line values.
	if obj.Node != nil && obj.Node.Element() && len(lines) > 0 {
		if flStyle, ok := a.FirstLineStyles[obj.Node.ID]; ok && flStyle != nil {
			applyFirstLineRuns(a, &lines[0], flStyle, obj.Style)
		}
	}
	// A U+00AD that closed a line is a taken hyphenation opportunity: the
	// glyph Chromium draws is U+002D, charged its real advance, and only
	// then. An unused soft hyphen stays default-ignorable (zero advance,
	// no ink). Applied after wrap, balance, clamp and ::first-line so the
	// hyphen lands on the line that actually broke and is measured with the
	// font that line will paint.
	applyTakenSoftHyphens(a, lines)
	// Re-fetch obj: the Alloc calls in the text path may have reallocated the
	// arena slice, so the pointer captured at function entry is stale.
	obj = a.Get(id)
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	contentW = obj.W
	// Strut was computed before breaking; every line box is at least as tall.
	justifyExtra := float32(0)
	for li, line := range lines {
		// Both sides of the line's baseline resolve from every run on it - glyph
		// runs and atomic boxes in the same currency (lineExtent, CSS 2.1 10.6.1
		// and 10.8) - and the strut is one of the demands, so the line box can
		// neither forget the block's own descent nor grow past the runs sitting on
		// it. The height it answers is already at least the strut, so there is no
		// clamp left to write. It is asked before the float span so the shortened
		// line is measured against the height the line actually gets.
		demands := runDemands(a, line.runs, sm)
		fr := lineExtent(sm, demands)
		line.h = fr.height
		justifyExtra = 0
		// A line overlapping floats lays out inside the shortened span, so
		// alignment centers within what the floats leave, not the full box.
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
		// Bidi resolution answers two separate questions with one level list:
		// the order the runs and the glyphs inside each word land in, and
		// whether the line's start edge is its right edge. A line of only
		// Latin under direction:rtl keeps its logical order and only moves to
		// the right edge, so both come from the same call.
		lineRuns := line.runs
		rtlStart := obj.Style != nil && obj.Style.Direction == "rtl"
		if obj.Style != nil && (rtlStart || needsBidi(obj.Style, line.runs)) {
			lineRuns, rtlStart = bidiReorder(obj.Style, a, line.runs)
		}
		// `start` alignment follows the line's direction. Measured: an RTL line
		// with no text-align ends its ink on the content box's right edge
		// (rightmost edge x800.001 in a box ending at x800, and x770.001 with
		// 30px inline padding), text-align:left moves it back to x600 and
		// center leaves equal gaps (scripts/verify-br087-bidi.js
		// y_cs_default_rtl, u_padding, ab_cs_left_rtl, s_align_center).
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
				// The run's box goes wherever its `vertical-align` class puts it -
				// its own baseline on the line's for the default, the line's top or
				// bottom edge for `top`/`bottom`, the parent font's content edges for
				// `text-top`/`text-bottom`, a shift of its own baseline for a length -
				// and its content area starts one half-leading inside that box. On a
				// line of one size the default is exactly the centring this path used
				// to do, and on a line of mixed sizes it is the placement the
				// reference measures: a 64px run beside 16px text sits 12px above the
				// block that owns it, overflowing, because the line grew by the run's
				// reach above the baseline rather than by its whole box.
				d := demandForRun(demands, line.runs, run.obj)
				baseY := y + fr.atomicTop(d.box) + d.half
				if isRubyAnnotation(runObj, a) {
					baseY -= run.contentH * 0.5
				}
				runObj.Y = baseY
			}
			runObj.W = wordW
			runObj.H = run.contentH
			// Record this word's content rect under its generating inline
			// element, keyed by line, so a wrapping span fragments its border.
			// Only border-bearing generators are tracked; borderless inline
			// content (the whole existing corpus) captures nothing.
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
		// Only a block with no block-level children takes its height from the
		// line boxes: H is the content height, so line boxes fill it directly,
		// and anything blockInto already stacked into it must survive.
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
		// Pseudo-element boxes are anonymous text nodes, not elements: they
		// still form an atomic inline-level box of their own, which is how an
		// inline-block ::before gets a background and a size.
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
		// The whitespace-collapse path (normal, nowrap) fuses collapsing and
		// splitting into one pass below, so the raw text is kept: collapsing
		// first would allocate a normalized copy just to re-scan it. The
		// pre-line path keeps its own collapse (it preserves newlines and
		// trims edges, which the fusion does not do) and the pre paths split
		// the raw text so exact space runes survive.
		useMerged := false
		if k.Style.WhiteSpace == style.WhiteSpacePreline {
			// pre-line: preserve line breaks, collapse other whitespace
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
				// Soft hyphen and ZWSP stay break opportunities under keep-all:
				// that keyword only suppresses CJK breaks, not these.
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
			// Newline: force line break, don't add as a run
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
			// A space at the start of a line is collapsible whitespace that
			// has nowhere to separate anything: drop it rather than indenting
			// the line.
			if word == " " && len(current.runs) == 0 {
				continue
			}
			// Wrap against the float-shortened width, not the full content
			// box: beside a float the line holds fewer words.
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
			// Re-fetch k: the Alloc calls above may have grown the arena slice,
			// invalidating the pointer captured at function entry.
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
	// An inline replaced image is an unbreakable atomic box on the line, sized
	// from its own content rather than from text. It has no children to recurse
	// into, so it is handled before the generic inline-element descent.
	k = a.Get(nodeID)
	if k.Node != nil && k.Node.Element() && k.Node.Data == "img" {
		atomicInlineReplaced(a, nodeID, lines, current, bc)
		return
	}
	// Inline element (e.g. <a>, <span>, <b>): recursively collect from its children.
	// Re-fetch k in case Alloc calls during text processing reallocated the arena.
	k = a.Get(nodeID)
	for child := k.FirstKid; child != 0; child = a.Get(child).NextSibling {
		collectInline(a, child, lines, current, bc)
	}
}

type lineBox struct {
	runs []textRun
	w, h float32
}

// trimTrailingSpaces drops the space runs closing a line and takes their width
// back out of the line box. Under white-space:normal a space ending a line is
// collapsible and renders as nothing, so leaving it in inflated line.w and pushed
// text-align:center and :right content away from where it belongs. In the pre*
// modes a trailing space is significant, so those runs stay.
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
		totalW += l.w
	}
	targetW := totalW / float32(len(lines))
	var newLines []lineBox
	var cur lineBox
	curH := lines[0].h
	for i, r := range allRuns {
		if cur.w > 0 && cur.w+r.w > targetW*1.15 && len(newLines) < len(lines)-1 {
			trimTrailingSpaces(a, &cur)
			newLines = append(newLines, cur)
			cur = lineBox{h: curH}
		}
		cur.runs = append(cur.runs, r)
		cur.w += r.w
		if r.lineH > cur.h {
			cur.h = r.lineH
		}
		if i == len(allRuns)-1 {
			trimTrailingSpaces(a, &cur)
			newLines = append(newLines, cur)
		}
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
	// An atomic run is a whole inline-level box rather than a word: it carries
	// no text of its own, and srcX/srcY record where its own layout left it so
	// the subtree can travel to the line position decided later.
	atomic bool
	srcX   float32
	srcY   float32
}

// The three ways this model can place an atomic inline-level box vertically,
// sorted from its vertical-align. Measured on Chromium 2026-11-12 over rows of
// 26px inline-blocks with 10px bottom margins in a 16px sans-serif block: the
// default baseline alignment gives a 40px line - 36 above the baseline plus the
// strut's 4px descent below it - while `top`, `bottom` and `middle` each give
// exactly 36, the boxes' own margin height and no descent.
const (
	atomicOnBaseline   = iota // its own baseline on the line's (the default)
	atomicToTop               // vertical-align: top
	atomicToBottom            // vertical-align: bottom
	atomicToTextTop           // the box's top edge on the parent font's content top
	atomicToTextBottom        // the box's bottom edge on the parent font's content bottom
	atomicToLength            // vertical-align: <length>, the box's baseline shifted
	atomicToScript            // vertical-align: super/sub, the same shift at the parent's script offset
	atomicToMiddle            // the box's centre on the parent's x-height over the baseline
)

// atomicBox is one atomic inline-level box's demand on its line: the height of
// its margin box, how it is aligned, and how much of that height hangs below
// the baseline it aligns on.
type atomicBox struct {
	extent float32
	align  int
	below  float32
	// length is the shift `vertical-align: <length>` asks for, positive raising
	// the box's own baseline above the line's.
	length float32
}

// atomicAlign sorts a vertical-align value into the classes above and takes the
// length out of it when there is one. `super` and `sub` carry their sign here
// and their amount in resolveScript, because the shift belongs to the parent's
// font rather than to the declaration.
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

// alignLength reads the px form of a vertical-align length. em, ex and
// percentages are not resolved here rather than guessed at: they fall back to
// the bottom edge, which is the behaviour the residual defect measures.
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

// resolveScript gives `super` and `sub` the shift of their own baseline that the
// reference measures, then lets them ride the length path: both move the box's
// baseline, so both sides of the box move with it and the line grows instead of
// the box being lifted out of one that stayed put.
//
// Measured on Chromium 2026-11-12 over fourteen font sizes from 8 to 80px: the
// shift is the *parent's* size over 3 raised, over 5 lowered, plus one pixel,
// each truncated to 1/64 px - 6.328125 and 4.1875 at 16px, 5 and 3.390625 at
// 12px, 22.328125 and 13.796875 at 64px. It does not belong to the box's own
// size: a 24px run of text in a 64px block is lowered by 13.796875px, the 64px
// number, and a 64px run in a 16px block raised by 6.328125px, the 16px one.
// The same pair answers for an atomic box - a 4px chip supered on a 16px parent
// sits at 3.671875 where its baseline puts it at 10.
func resolveScript(a *Arena, k *Object, b *atomicBox, sm strutMetrics) {
	if b.align != atomicToScript {
		return
	}
	if isRubyAnnotation(k, a) {
		// The UA sheet asks for `vertical-align: super` on an <rt> because that
		// is the closest declaration to what ruby does, but the reference lays
		// ruby out with its own algorithm and the shift above is much smaller
		// than that raise: on a 16px line with an 8px annotation the script
		// answer is 6.328125px, which leaves the annotation inside the base
		// text it belongs above. Ruby keeps its proportional raise at placement
		// and takes no shift here.
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

// boxOfAtomic records what one atomic box demands of its line. It takes the
// arena and id rather than an object pointer because it walks the box's subtree,
// and the arena grows during layout: a pointer captured earlier can address the
// slice it grew out of. The strut comes along because two of the values this
// reads resolve against the parent's font rather than the box's own.
func boxOfAtomic(a *Arena, id ObjectID, sm strutMetrics) atomicBox {
	k := a.Get(id)
	va := ""
	if k.Style != nil {
		va = k.Style.VerticalAlign
	}
	b := atomicBox{extent: k.MarginTop + k.BorderH() + k.MarginBottom}
	b.align, b.length = atomicAlign(va)
	resolveScript(a, k, &b, sm)
	// A control's text lives in a shadow tree the arena never sees, so the
	// subtree walk cannot answer "does it have line boxes" for one: it does.
	// A length-aligned box needs the same number: the shift is applied to its
	// own baseline, so what sits below that baseline is what moves.
	if (b.align != atomicOnBaseline && b.align != atomicToLength) || k.Style == nil ||
		(!isFormControl(k) && (isReplacedElement(k) || !hasInFlowLineBoxes(a, id))) {
		return b
	}
	// Appearance-only widgets put their baseline on their bottom *border* edge:
	// not on the bottom margin edge the way an image or an empty inline-block
	// does, and not on an interior text line the way the controls that render a
	// value do. Measured on Chromium 2026-11-12 with the UA margins in place: a
	// 13px checkbox with 3px above and below answers a 20px line at offset 3 on
	// the 16px strut (above 16 = margin-top + height, below 3 = margin-bottom),
	// a 16px range with 2px of margin answers 22 at offset 2, the same checkbox
	// with `margin: 10px` answers 33 at offset 10, and with `margin: 5px 0 15px`
	// 33 at offset 5 - while a 13px *image* on the same strut takes 18 at offset
	// 1, and so does `input[type=image]` given the same 13px box: those align on
	// their bottom margin edge. So the widget's own box is what rides the
	// baseline and only its bottom margin hangs under it, which is exactly
	// `below = margin-bottom` here.
	if isAppearanceWidget(k) {
		b.below = k.MarginBottom
		return b
	}
	// CSS 2.1 10.8: an inline-block with in-flow line boxes and visible overflow
	// aligns its LAST LINE's baseline, not its bottom margin edge, so the descent
	// of that line plus everything under it (padding, border, margin) sits below
	// the parent's baseline. Goosie does not record the interior baseline - text
	// runs are centred, not baselined (GOOSIE-INLINEBASELINE-001) - so the line's
	// own strut stands in for it. Measured on Chromium 2026-11-12: a 202x62
	// inline-block holding a float and text, in a 16px/20px block, gives a 62px
	// line - its 5.5px of descent under the baseline covers the strut's, so the
	// line neither grows nor moves.
	asc, desc, lineH := fontHeights(a.Metrics, k.Style.FontSize, k.Style.FontSlot(), k.Style.LineHeight)
	_, below := strutSplit(asc, desc, lineH)
	b.below = below + k.PaddingBottom + k.BorderBottom + k.MarginBottom
	if b.below > b.extent {
		b.below = b.extent
	}
	return b
}

// isFormControl reports the boxes whose visible text lives in a shadow tree the
// arena never sees, so nothing but a shortcut can answer "does it have line
// boxes" for them: they align on that text's baseline like any inline-block with
// line boxes, which is what keeps a lone control from gaining the strut's
// descent on top of its own. Measured on Chromium 2026-11-12, each control alone
// on a line in a 16px block (strut line 18), as line box / box offset / box:
// text input 21 / 0 / 21, select 19 / 0 / 19, file 21 / 0 / 21, colour swatch
// 27 / 0 / 27, button 21 / 0 / 21 - while a 20px image on the same strut takes
// 24, the whole descent added.
//
// Two names that used to be here are not. A textarea answers 40 / 0 / 36 for a
// rows=2 box, the strut's descent under it, and the same whether or not it holds
// text: it aligns on its bottom margin edge like the other replaced boxes. An
// input[type=image] answers 18 / 1 / 13 and 24 / 0 / 40x20, byte for byte what a
// plain <img> of the same box gives. A <button> needs no shortcut either: its
// label is a DOM child, and measuring it is what makes an empty button answer
// 18 for a 6px box, which the shortcut turned into 20.
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

// isAppearanceWidget names the inputs whose whole box *is* the widget: there is
// no interior text line for a baseline to come from, so they sort as form
// controls for every other purpose and take their baseline from their own bottom
// border edge instead (see boxOfAtomic). Measured 2026-11-12: checkbox, radio and
// range answer here, while the widgets that render a value - text input, number,
// button, select, textarea, colour picker - do not.
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

// hasInFlowLineBoxes reports whether the subtree holds text that survives
// whitespace collapse, which is what gives an atomic box a baseline of its own
// instead of its bottom margin edge. The walk stops at the first such text and
// gives up past a bound, so a row of many deep subtrees cannot turn layout into
// a quadratic scan; giving up answers "no line boxes", which is the shape every
// icon, sprite and control on a strip of atoms already has.
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

// strutMetrics is the containing block's own font geometry in the two
// currencies a line needs: split around the baseline, which is the demand the
// line box can never go under (CSS 2.1 10.6.1), and raw, which is what
// `vertical-align: text-top`/`text-bottom` and a parent's content edges align
// on. Keeping both in one value is what stops the four floats from being
// threaded through every demand and placement call separately.
type strutMetrics struct {
	above, below float32
	asc, desc    float32
	// xh is the parent face's x-height: `vertical-align: middle` puts a box's
	// centre half of it above the baseline, and a run of text measures against
	// the same number however tall its own box is (measured 2026-11-12 - a 64px
	// run centred on a 16px parent's 8.375px x-height, not its own 33.5px one).
	xh float32
	// sup and sub are the baseline shifts `super` and `sub` ask for, read from
	// resolveScript. They are sizes, not edges: the reference truncates each to
	// 1/64 px, which is why the arithmetic keeps the fraction.
	sup, sub float32
}

// ofStrut builds the pair from a font's heights and its line height, plus the
// two numbers the edge-aligned values measure against: the x-height the face
// reports, and the parent's script offsets.
func ofStrut(asc, desc, lineH, xh, sizePx float32) strutMetrics {
	above, below := strutSplit(asc, desc, lineH)
	// size/3 raised, size/5 lowered, one pixel each, truncated to the 1/64 px
	// the layout currency works in - see resolveScript.
	trunc := func(v float32) float32 { return float32(int64(v*64)) / 64 }
	return strutMetrics{
		above: above, below: below, asc: asc, desc: desc, xh: xh,
		sup: trunc(sizePx/3) + 1, sub: trunc(sizePx/5) + 1,
	}
}

// faceXHeight reports the x-height in px. A face that gives none - a host with
// no font backend at all - answers half the size, the same shape as the
// proportional fallback in fontHeights; no reference number exists for that
// state, and it is a fallback rather than a measurement.
func faceXHeight(m Metrics, size float32, slot frame.FontSlot) float32 {
	if m == nil || size <= 0 {
		return 0
	}
	if v := m.XHeightFixed(int32(size), slot); v > 0 {
		return float32(v) / 64
	}
	return size * 0.5
}

// lineFrame is a resolved line: where its baseline sits, how tall it is, and the
// parent font the edge-aligned values measure against.
type lineFrame struct {
	base   float32
	height float32
	sm     strutMetrics
}

// runDemand is what one run on a line - a glyph run or a whole atomic box -
// asks of the line's baseline. Text and atoms answer the same two numbers, so a
// line mixing them resolves both sides of its baseline in one pass
// (GOOSIE-INLINEBASELINE-001 was exactly the seam between the two answers).
type runDemand struct {
	above  float32   // reach from the baseline up to the run's box top
	below  float32   // reach from the baseline down to the run's box bottom
	asc    float32   // text only: its content area starts this far above the baseline
	half   float32   // text only: the leading half that sits above the content area
	box    atomicBox // what lineFrame.atomicTop needs to place the box
	atomic bool
	// edge marks a box measured against the line's own top or bottom edge -
	// `vertical-align: top`/`bottom`, the only two values of the property that
	// work that way: they stretch the line box rather than moving the baseline
	// under them.
	edge bool
	// bottom narrows that to the boxes pinned to the line's *bottom* edge. When
	// one of them stretches the line, the extra height lands above the baseline
	// instead of below it, carrying every other box on the line down with it.
	bottom bool
}

// atomicDemand turns one box's margin geometry into the same currency, given the
// block's own ascent and descent. Measured on Chromium 2026-11-12 for a 4px chip
// on a 16px strut (baseline 14px down an 18px line, so the parent's content box
// spans 0..18): `baseline` puts the top at 10, `top` and `text-top` at 0,
// `bottom` and `text-bottom` at 14, `5px` at 5, `-5px` at 15 - and the `-5px`
// line is 19 tall, because the lowered box hangs below the strut's descent and
// the line has to contain it. `middle` answers 7.8125 on that same row: half
// the parent's 8.375px x-height plus half the box is what rises above the
// baseline, which is the one number this used to have to leave out.
func atomicDemand(b atomicBox, sm strutMetrics) runDemand {
	d := runDemand{box: b, atomic: true}
	switch b.align {
	case atomicOnBaseline:
		d.above, d.below = b.extent-b.below, b.below
	case atomicToLength:
		// The shift moves the box's own baseline, so both of its sides move with
		// it: raising it by 5px takes 5px off the bottom and adds 5 to the top.
		d.above, d.below = b.extent-b.below+b.length, b.below-b.length
	case atomicToTextTop:
		d.above, d.below = sm.asc, b.extent-sm.asc
	case atomicToTextBottom:
		d.above, d.below = b.extent-sm.desc, sm.desc
	case atomicToMiddle:
		// The box's centre sits half an x-height above the baseline, so each of
		// its sides is that point moved by half the extent - and either side can
		// be the one that stretches the line. Measured on Chromium 2026-11-12
		// for chips centred on the 16px strut (x-height 8.375, baseline 14): a
		// 4px box answers an unchanged 18px line at offset 7.8125, a 20px box a
		// 20px line with the block's own text 0.1875px down it, a 40px box a
		// 40px line with that text at 10.1875.
		d.above, d.below = sm.xh/2+b.extent/2, b.extent/2-sm.xh/2
	default:
		// `top` and `bottom` measure against the line's own edges: they stretch
		// the box rather than move the baseline under it.
		d.edge = true
		d.bottom = b.align != atomicToTop
	}
	return d
}

// demandOfRun measures one run against the line's baseline. A text run's box is
// its line height, split around the baseline by its own ascent and descent, so a
// 64px run under a 50px declared height reaches 47px above the baseline and only
// 3px below it: the line box grows by the run without the run's content area
// fitting inside it.
//
// The same box is what its `vertical-align` moves. Measured on Chromium
// 2026-11-12 for a 16px run with `line-height: 40px` (half-leading 11px) on a
// 50px strut (baseline 30px down), the content area starts at 16 for `baseline`,
// 11 for `top` and for `5px`, 21 for `bottom` and for `-5px`, and 27 on a 56px
// line for `text-top`; `text-bottom` answers 11 on that same 56px line with the
// block's own text 6px lower down it, because that box hangs above the line's
// top edge and the line has to grow to contain it. Those are the atomic answers
// for a box 40px tall whose content area starts 11px inside its top edge, so a
// run is measured as one box and placed by box top plus half-leading - the class
// it carries comes from the element the run belongs to, not from the block. Two
// values of the property then answer the block's font rather than the run's:
// `middle` centres on the parent's x-height and `super`/`sub` shift by the
// parent's script offset, whichever size the run turns out to have, and both are
// settled before this function places anything.
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
	// The box an edge value pins is the run's line-height box, and its baseline
	// sits `asc` inside that box with the leading split around it. The half is
	// left unclamped because a 64px face under `line-height: 10px` takes 32px out
	// of each side, and the box that overflows the line is what the reference
	// then measures against the line's edges.
	b.below = lineH - (asc + (lineH-(asc+desc))/2)
	d := atomicDemand(b, sm)
	d.asc, d.half, d.box = asc, (lineH-(asc+desc))/2, b
	return d
}

// runDemands measures every run of a line, in the logical order the runs were
// collected in. Placement walks the bidi-resolved order and looks each run up by
// object, so the two slices stay in step without assuming the order.
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

// demandForRun finds the demand one run carries.
func demandForRun(ds []runDemand, runs []textRun, id ObjectID) runDemand {
	for i, r := range runs {
		if r.obj == id && i < len(ds) {
			return ds[i]
		}
	}
	return runDemand{}
}

// lineExtent is CSS 2.1 10.6.1 and 10.8 for any line: the line box must contain
// the strut built from the block's own font, and the strut reaches below the
// baseline. Both sides of the baseline resolve on their own - base is the offset
// of the baseline from the line's top edge, height the line box's height -
// because a box's own baseline can sit anywhere inside it: the empty 36px chip
// puts its bottom margin edge on the baseline, so the strut's 4px descent still
// hangs under it (a 40px line), while a 62px inline-block whose last line's
// baseline sits 5.5px above its own bottom edge covers that descent itself and
// the line stays 62px.
//
// The scalar strut clamp this replaced cannot say either of these: for a 36px row
// the strut's whole height (18px) is smaller than the box, so nothing moves there,
// yet 3.5px of the strut still belongs below the baseline - which is every row of
// icon strips, sprite rows and chip lists shifting up as it stacks.
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
	// Edge boxes resolve in two steps rather than one pass, because only the one
	// that actually stretched the line gets to move the baseline: a `top` box
	// growing the line must not push it down for an unrelated `bottom` box.
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
	// A box measured against the line's own edges stretches it instead of
	// moving the baseline down under it: a 58px `top` box beside a 36px
	// baseline box gives a 58px line, not 62 (measured).
	if bottomStretch > height {
		// A box pinned to the *bottom* edge stretches the line upward, so the
		// baseline moves down with the stretch and carries everything else on the
		// line with it. Measured 2026-11-12: a 74px `bottom` inline-block on a
		// 50px strut gives a 74px line whose baseline sits 54px down, so the
		// block's own text starts 40px into it where `top` on the same box leaves
		// the baseline at 30 and the text at 16; in an 18px strut the same
		// `bottom` box answers a 40px line and a 36px baseline.
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

// atomicTop is where one box's margin box starts inside the line. Baseline and
// length-aligned boxes put a baseline of their own on the line's - `super` and
// `sub` being lengths taken from the parent's size - `top` and `bottom` ride the
// line's own edges, `text-top`/`text-bottom` ride the parent font's, and
// `middle` centres the box on half the parent's x-height above the baseline.
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

// atomicInlineBlock lays out an inline-block and adds it to the line being
// built. The box behaves as a single unbreakable word whose glyph is a nested
// layout: it takes part in the line's horizontal flow and grows the line box,
// but its own interior is laid out as a block.
func atomicInlineBlock(a *Arena, id ObjectID, lines *[]lineBox, current *lineBox, bc *breakCtx) {
	blockInto(a, id, bc.contentW)
	inlineInto(a, id)
	k := a.Get(id)
	// An auto width shrinks to the content, the way the block pass shrinks a row
	// of inline-blocks. Filling the line would give the box the whole row and push
	// every sibling onto a line of its own, which is what turned a title followed
	// by its tag pills into two lines.
	intrinsicTextarea := k.Node != nil && k.Node.Data == "textarea"
	if k.Style != nil && !intrinsicTextarea && resolveBoxWidth(a, k.Style, bc.contentW) < 0 {
		if w := itemMaxContentW(a, id); w > 0 && w < k.W {
			k.W = w
			k = a.Get(id)
			clampWidth(a, k, bc.contentW)
			k = a.Get(id)
			// The interior was laid out for the wide box it has just lost, so its
			// words are re-wrapped and re-aligned at the width the box settles on.
			// Without this a centred label keeps the position the wide measure gave
			// it and paints outside its own button.
			clearInlineLaidOut(a, id)
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

// atomicInlineReplaced adds an inline replaced image to the line being built.
// Unlike an inline-block, its size is not measured from content: it comes from
// the CSS/attribute/intrinsic resolution in the block pass helper. The box still
// behaves as one unbreakable unit on the line.
func atomicInlineReplaced(a *Arena, id ObjectID, lines *[]lineBox, current *lineBox, bc *breakCtx) {
	k := a.Get(id)
	// Replaced boxes never passed through resolveBoxSizes, so their style
	// padding/border/margin stayed zero and padding on an <img> silently
	// vanished. Copy them here the same way so the content box (where
	// paint draws the pixels) sits inside them.
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
		for i, m := range margins {
			if m == style.MarginAuto {
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

// runHeights returns the height a run contributes to its line box and the
// height of its content area, both in CSS px.
//
// With line-height:normal the line box is the font's own leading-inclusive
// height, which is what makes Times lines a pixel taller than Arial's at the
// same size. An explicit line-height replaces it, and the content area stays
// ascent+descent so the half-leading can be split around the baseline.
func runHeights(m Metrics, size float32, slot frame.FontSlot, styleLineHeight float32) (lineH, contentH float32) {
	asc, desc, lineH := fontHeights(m, size, slot, styleLineHeight)
	return lineH, asc + desc
}

// fontHeights reads the three numbers a line box is built from: the face's own
// ascent and descent, and the line box height the style asks for. It is the
// same reading runHeights takes, broken out because the strut needs the
// ascent and descent separately — a line of atomic boxes has to know what the
// block owes below the baseline, not just how tall its own line box is.
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

// strutSplit divides a box's line height around its baseline: half of the
// leading goes above the ascent and half below the descent (CSS 2.1 10.6.1).
// With line-height: normal the two parts sum to the font's own line box height,
// and an explicit line-height widens both sides equally, which is what makes a
// 36px inline-block row 51px tall at line-height:40px rather than 40.
//
// The leading may be negative and is not clamped: a line-height shorter than the
// content area pulls both edges inward, and the content area then overflows.
// `line-height: 1` on a 16px face is the case: the declared height is one em, so
// with this engine's 16px ascent 14 / descent 4 face the split is 13 above the
// baseline and 3 below, and clamping the leading at zero would keep demanding 18
// - making every tight paragraph one line taller than the reference, which stays
// 16px and lets the glyphs overflow it (measured on Chromium 2026-11-12).
func strutSplit(asc, desc, lineH float32) (above, below float32) {
	half := lineH - (asc + desc)
	above = asc + half/2
	// The underside is taken from the declared height, not from a second
	// rounded half: exact arithmetic makes the two the same number, but summing
	// independently rounded halves turned a 21.6px line into 21.600002px, and
	// that string is what getComputedStyle reports.
	return above, lineH - above
}

// measureWord returns the width of word at fontSize. With metrics it is the
// exact sum of the font's glyph advances; without, a half-em-per-character
// estimate. The two must stay the only sources of width so line breaking and
// painted spacing can't disagree. letterSpacing adds extra space between each
// pair of characters.
// maxWordFragment is the widest piece a single unbreakable word may be laid out
// as. A word beyond it can carry a box's geometry past the engine's finite-width
// guard all by itself, which used to refuse the whole document for one base64
// blob. No viewport is anywhere near this wide, so an ordinary word that merely
// overflows its line is left to overflow exactly as before.
const maxWordFragment = float32(1 << 16)

// splitOverlongWords breaks a word too wide to bound into rune chunks that fit
// under maxWordFragment. Advantages accumulate the way measureWord does, so a
// chunk the splitter calls narrow is still narrow where the line box places it.
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

// breakOverlongWords rewrites the whole token list, splitting every unbounded
// word. Tokens already measured as bounded are copied through untouched, so an
// ordinary overflowing word keeps overflowing on one line.
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

// cjkBreakRune reports whether a line may end after r. Chromium 1.63 put a
// break opportunity after every Han ideograph, kana and Hangul syllable
// (scripts/verify-br087-text-parity.js: 25 ideographs in a 200px box -> lines
// of 12, 12, 1; the kana sentence -> 12, 12, 3), while Latin runs keep
// breaking only at spaces.
func cjkBreakRune(r rune) bool {
	return (r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
		(r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul syllables
		(r >= 0xF900 && r <= 0xFAFF) // CJK Compatibility Ideographs
}

// kinsokuNoLineStart reports whether r must not begin a line (行頭禁則).
// Chromium 1.63 pulled the previous character down rather than leave one of
// these at line start (scripts/verify-br087-text-parity.js: the full-stop
// fixture `文本文本文本文本文本文本。文本` broke as 11 ideographs + `本。文本`,
// stop leads a line=false). The set covers the JIS X 4051 "no line start"
// class: closing brackets, CJK punctuation, small kana, the prolonged-sound
// mark, and iteration marks.
func kinsokuNoLineStart(r rune) bool {
	switch r {
	// CJK punctuation: full stop, comma, prolonged sound
	case 0x3001, 0x3002, 0x30FC: // 、 。 ー
		return true
	// Fullwidth forms of ASCII punctuation that close
	case 0xFF01, 0xFF09, 0xFF0C, 0xFF0E, 0xFF1A, 0xFF1B, 0xFF1F, 0xFF3D, 0xFF5D: // ！ ） ， ． ： ； ？ ］ ｝
		return true
	// CJK closing brackets
	case 0x3009, 0x300B, 0x300D, 0x300F, 0x3011, 0x3015: // 〉 》 」 』 】 〙
		return true
	// Small Hiragana
	case 0x3041, 0x3043, 0x3045, 0x3047, 0x3048, 0x3063, 0x3083, 0x3085, 0x3087, 0x308E, 0x3095, 0x3096: // ぁ ぃ ぅ ぇ ぉ っ ゃ ゅ ょ ゎ ゕ ゖ
		return true
	// Small Katakana
	case 0x30A1, 0x30A3, 0x30A5, 0x30A7, 0x30A9, 0x30C3, 0x30E3, 0x30E5, 0x30E7, 0x30EE, 0x30F5, 0x30F6: // ァ ィ ゥ ェ ォ ッ ャ ュ ョ ヮ ヵ ヶ
		return true
	// Iteration marks
	case 0x3005, 0x309D, 0x309E, 0x30FD, 0x30FE: // 々 ゝ ゞ ヽ ヾ
		return true
	// Horizontal ellipsis and related
	case 0x2026, 0x2025, 0x3003: // … ‥ 〃
		return true
	}
	return false
}

// splitCJKWords turns a word too long for its line into the units the line
// breaker may place: each CJK rune closes a token, so the text after it -
// including a Latin run that had no space before it - starts a new one, and
// "中文aaa" breaks as 中 | 文aaa. A combining mark never opens a fragment, so
// the decomposed accents stay with the base they decorate. A kinsoku no-line-
// start character (。、ー and the rest of kinsokuNoLineStart) that would open
// the next fragment is pulled into the current one instead, so the break sits
// before the ideograph that precedes it and the forbidden character never
// leads a line (GOOSIE-KINSOKU-001). A word that still fits keeps its single
// box: a break opportunity that is never taken must not fragment the run.
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
			// Kinsoku: extend past any no-line-start rune so the fragment
			// that follows the break does not open with one.
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

// softBreakRune reports a CSS wrap opportunity that is not whitespace and
// not CJK: U+00AD (soft hyphen) and U+200B (zero-width space). Chromium 1.63
// broke `42 a's + opportunity + 48 b's` in a 200px box at that opportunity
// (scripts/verify-br087-text-parity.js BREAK-shy / BREAK-zws) while a word
// with neither overflowed as one line.
func softBreakRune(r rune) bool {
	return r == '\u00AD' || r == '\u200B'
}

// splitSoftBreaks is the SHY/ZWSP counterpart of splitCJKWords: a word too
// long for its line becomes the units the line breaker may place, each
// opportunity closing a token so the text after it starts a new one. A
// combining mark never opens a fragment. A word that still fits keeps its
// single box, so an unused soft hyphen does not split the run it sits in
// and does not draw a hyphen.
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

// applyTakenSoftHyphens substitutes U+002D for a trailing U+00AD on every
// line that actually wrapped. The last line of the block did not take the
// opportunity, so its unused SHY stays invisible. Width is remeasured
// because the ignorable SHY cost nothing and the hyphen does (Chromium
// inked a 5.328px hyphen after the overflowing a-run).
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

// splitCollapsedWords fuses collapseWhitespace and splitWords for the
// whitespace-collapse path (normal, nowrap): each maximal whitespace run
// becomes one shared " " token and each maximal non-space run becomes a
// substring of text, so neither a normalized copy nor a per-word Builder is
// built. The output is identical to splitWords(collapseWhitespace(text)):
// the collapse step normalizes every whitespace run to a single ASCII space
// (so the shared " " token matches exactly, including "\n" and "\t" runs),
// and non-space runs pass through byte-for-byte (so the substrings match).
// Only the collapse path may use it: the pre modes need exact space runes
// and pre-line preserves newlines and trims edges, neither of which this does.
func splitCollapsedWords(text string) []string {
	// Advisory capacity from a cheap byte pre-scan: ASCII whitespace covers
	// every separator except exotic Unicode spaces, prose holds about one
	// space token per word, and a miscount only costs a slice regrow, never
	// correctness.
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
	start := -1 // byte offset of the open word run, -1 when none
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
			// Newline: flush current word, add newline as separate token
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

// collapseWhitespace runs each whitespace sequence together into one space but
// keeps that space at the edges of the text node. Whether an edge space survives
// is a property of the line it lands on, not of the node: " and " between two
// inline elements is a real word separator, while the same space at a line start
// or line end collapses away. Trimming here instead glued
// "<b>a</b> and <b>b</b>" into "aandb".
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

// collapseWhitespacePreserveNewlines collapses whitespace but preserves line breaks.
// Used for white-space: pre-line.
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

// isRubyAnnotation checks if an object is an <rt> ruby text annotation or descendant thereof.
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

// applyTextTransform applies CSS text-transform to an input string.
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
				// ASCII 0x21 (!) to 0x7E (~) map to full-width 0xFF01 to 0xFF5E
				b.WriteRune(r + 0xFEE0)
			} else if r == 0x20 {
				// Space maps to ideographic space 0x3000
				b.WriteRune(0x3000)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	case "full-size-kana":
		var b strings.Builder
		for _, r := range text {
			// Convert small hiragana/katakana to full-size equivalents
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

// applyFirstLineRuns overrides the style of every run on the first formatted
// line with the ::first-line pseudo-element style. Font properties, color,
// background, text-decoration, text-transform, letter-spacing, word-spacing,
// and line-height are replaced; the run is then remeasured so placement uses
// the new metrics.
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
			// Atomic inline boxes (replaced content, inline-block) keep their
			// own geometry; only non-atomic text runs participate in first-line.
			newW += run.w
			continue
		}
		// Build an overridden style: start from the run's own style and apply
		// the first-line font/color properties on top.
		base := runObj.Style
		if base == nil {
			base = containerStyle
		}
		if base == nil {
			continue
		}
		overridden := *base
		// Font properties.
		overridden.FontFamily = flStyle.FontFamily
		overridden.CustomFamily = flStyle.CustomFamily
		overridden.CustomFontIdx = flStyle.CustomFontIdx
		overridden.FontSize = flStyle.FontSize
		overridden.FontWeight = flStyle.FontWeight
		overridden.FontStyle = flStyle.FontStyle
		// Color and background.
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
		// Text properties.
		overridden.TextDecoration = flStyle.TextDecoration
		overridden.TextTransform = flStyle.TextTransform
		overridden.LetterSpacing = flStyle.LetterSpacing
		overridden.WordSpacing = flStyle.WordSpacing
		// Line-height: use the first-line value when it was explicitly set.
		if flStyle.LineHeight > 0 {
			overridden.LineHeight = flStyle.LineHeight
		}
		runObj.Style = &overridden

		// Remeasure the run with the new font.
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

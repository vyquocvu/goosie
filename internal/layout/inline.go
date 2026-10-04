package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// Inline runs the inline layout pass over the arena.
//
// Inline layout builds line boxes from inline-level content: text nodes and
// inline elements. Each line box is a horizontal strip of glyphs; when content
// exceeds the containing block's width, it wraps to a new line. The pass runs
// after block layout so every inline box knows its containing block's width.
func Inline(a *Arena, root ObjectID) {
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
	var lines []lineBox
	var current lineBox
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Node == nil || k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node.Text() {
			collectInline(a, kid, contentW, &lines, &current)
			continue
		}
		if isBlock(k) {
			if k.Style.Display == style.DisplayInlineBlock && k.Node != nil && k.Node.Element() &&
				k.flags&flagRowPlaced == 0 {
				atomicInlineBlock(a, kid, contentW, &lines, &current)
				continue
			}
			if len(current.runs) > 0 {
				trimTrailingSpaces(a, &current)
				if len(current.runs) > 0 {
					lines = append(lines, current)
				}
				current = lineBox{}
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
			}
			current = lineBox{}
			continue
		}
		// Inline-level child: either a text node or an inline element (<a>, <span>, <b>).
		// Collect its text runs into the enclosing block's current line box so they are
		// positioned sequentially inside this block container.
		collectInline(a, kid, contentW, &lines, &current)
	}
	if len(current.runs) > 0 {
		trimTrailingSpaces(a, &current)
		if len(current.runs) > 0 {
			lines = append(lines, current)
		}
	}
	// Re-fetch obj: the Alloc calls in the text path may have reallocated the
	// arena slice, so the pointer captured at function entry is stale.
	obj = a.Get(id)
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	contentW = obj.W
	// The block's own font and line-height form the strut, and every line box is
	// at least as tall as it even when all the runs inside are smaller. Without
	// it a `* { line-height: 26px }` reset reaches a 40px heading through the
	// link wrapping its text and collapses the heading's own line.
	strut := float32(0)
	if obj.Style != nil {
		strut, _ = runHeights(a.Metrics, obj.Style.FontSize, obj.Style.FontSlot(), obj.Style.LineHeight)
	}
	justifyExtra := float32(0)
	for li, line := range lines {
		if line.h < strut {
			line.h = strut
		}
		justifyExtra = 0
		baseX := obj.X + obj.PaddingLeft + obj.BorderLeft
		alignW := line.w
		if obj.Style != nil && alignW < contentW {
			switch obj.Style.TextAlign {
			case style.TextAlignCenter:
				baseX += (contentW - alignW) / 2
			case style.TextAlignRight:
				baseX += contentW - alignW
			case style.TextAlignJustify:
				if li < len(lines)-1 {
					justifyExtra = computeJustifyExtra(line, contentW, a)
				}
			}
		}
		x := baseX
		for _, run := range line.runs {
			runObj := a.Get(run.obj)
			if run.atomic {
				// The box's bottom margin edge sits on the line's baseline. This
				// model centres text runs rather than sitting them on a baseline,
				// and the line's own descent is a few pixels, so bottom-aligning
				// against the line box puts it where Chromium puts it.
				k := a.Get(run.obj)
				dx := (x + k.MarginLeft) - run.srcX
				dy := (y + line.h - k.BorderH() - k.MarginBottom) - run.srcY
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
				// The content area sits in the middle of the line box: the
				// surplus leading splits evenly above and below it, so the
				// baseline lands at half-leading + ascent. Paint reads the
				// ascent back out of the same metrics.
				runObj.Y = y + (line.h-run.contentH)/2
			}
			runObj.W = wordW
			runObj.H = run.contentH
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

func collectInline(a *Arena, nodeID ObjectID, contentW float32, lines *[]lineBox, current *lineBox) {
	k := a.Get(nodeID)
	if k.Style == nil || k.Style.Display == style.DisplayNone {
		return
	}
	if isBlock(k) {
		if k.Style.Display == style.DisplayInlineBlock && k.Node != nil && k.Node.Element() &&
			k.flags&flagRowPlaced == 0 {
			atomicInlineBlock(a, nodeID, contentW, lines, current)
		}
		return
	}
	if k.Node != nil && k.Node.Text() {
		text := k.Node.DataContent
		if k.Style.WhiteSpace == style.WhiteSpacePreline {
			// pre-line: preserve line breaks, collapse other whitespace
			text = collapseWhitespacePreserveNewlines(text)
		} else if shouldCollapseWhitespace(k) {
			text = collapseWhitespace(text)
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
			k.Style.WhiteSpace == style.WhiteSpacePrewrite
		words := splitWords(text)
		if !nowrap {
			words = splitOverlongWords(words, fontSize, a.Metrics, slot, letterSpacing)
		}
		var firstWordID ObjectID
		var lastWordID ObjectID
		for _, word := range words {
			// Newline: force line break, don't add as a run
			if word == "\n" {
				if current.w > 0 || len(current.runs) > 0 {
					trimTrailingSpaces(a, current)
					if len(current.runs) > 0 {
						*lines = append(*lines, *current)
					}
					*current = lineBox{}
				}
				continue
			}
			wordW := measureWord(word, fontSize, a.Metrics, slot, letterSpacing)
			if word == " " {
				wordW += wordSpacing
			}
			// A space at the start of a line is collapsible whitespace that
			// has nowhere to separate anything: drop it rather than indenting
			// the line.
			if word == " " && len(current.runs) == 0 {
				continue
			}
			if !nowrap && current.w+wordW > contentW && current.w > 0 {
				trimTrailingSpaces(a, current)
				if len(current.runs) > 0 {
					*lines = append(*lines, *current)
				}
				*current = lineBox{}
			}
			wordID, wordObj := a.Alloc()
			nodeCopy := *k.Node
			nodeCopy.DataContent = word
			wordObj.Node = &nodeCopy
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
		atomicInlineReplaced(a, nodeID, contentW, lines, current)
		return
	}
	// Inline element (e.g. <a>, <span>, <b>): recursively collect from its children.
	// Re-fetch k in case Alloc calls during text processing reallocated the arena.
	k = a.Get(nodeID)
	for child := k.FirstKid; child != 0; child = a.Get(child).NextSibling {
		collectInline(a, child, contentW, lines, current)
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

// atomicInlineBlock lays out an inline-block and adds it to the line being
// built. The box behaves as a single unbreakable word whose glyph is a nested
// layout: it takes part in the line's horizontal flow and grows the line box,
// but its own interior is laid out as a block.
func atomicInlineBlock(a *Arena, id ObjectID, contentW float32, lines *[]lineBox, current *lineBox) {
	blockInto(a, id, contentW)
	inlineInto(a, id)
	k := a.Get(id)
	// An auto width shrinks to the content, the way the block pass shrinks a row
	// of inline-blocks. Filling the line would give the box the whole row and push
	// every sibling onto a line of its own, which is what turned a title followed
	// by its tag pills into two lines.
	if k.Style != nil && resolveBoxWidth(a, k.Style, contentW) < 0 {
		if w := itemMaxContentW(a, id); w > 0 && w < k.W {
			k.W = w
			k = a.Get(id)
			clampWidth(a, k, contentW)
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
	if len(current.runs) > 0 && current.w+lineW > contentW {
		trimTrailingSpaces(a, current)
		if len(current.runs) > 0 {
			*lines = append(*lines, *current)
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
func atomicInlineReplaced(a *Arena, id ObjectID, contentW float32, lines *[]lineBox, current *lineBox) {
	k := a.Get(id)
	w, h, ok := replacedSize(a, k, contentW)
	if !ok {
		return
	}
	k.W = w
	k.H = h
	boxW := k.W + k.PaddingLeft + k.PaddingRight + k.BorderLeft + k.BorderRight
	lineW := boxW + k.MarginLeft + k.MarginRight
	lineH := k.BorderH() + k.MarginTop + k.MarginBottom
	if len(current.runs) > 0 && current.w+lineW > contentW {
		trimTrailingSpaces(a, current)
		if len(current.runs) > 0 {
			*lines = append(*lines, *current)
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
	asc, desc, normal := float32(0), float32(0), float32(0)
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
	return lineH, asc + desc
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
	for _, r := range []rune(word) {
		sum += int64(m.GlyphAdvanceFixed(int32(fontSize), r, slot))
	}
	return float32(sum)/64 + extra
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

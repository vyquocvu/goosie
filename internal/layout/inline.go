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
}

func inlineInto(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.Style != nil && obj.Style.Display == style.DisplayNone {
		return
	}
	// Skip if already laid out (prevents duplicate word objects when called
	// from both block layout and the main Inline pass).
	if obj.flags&flagInlineLaidOut != 0 {
		return
	}
	// An out-of-flow box is placed by the positioning pass, which then runs the
	// inline pass over it. Laying it out here would position its text against a
	// box that has no position yet.
	if obj.flags&flagOutOfFlow != 0 {
		return
	}
	obj.flags |= flagInlineLaidOut
	contentW := obj.W
	var lines []lineBox
	var current lineBox
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type == 2 {
			collectInline(a, kid, contentW, &lines, &current)
			continue
		}
		if isBlock(k) {
			if k.Style.Display == style.DisplayInlineBlock && k.Node != nil && k.Node.Type == 1 &&
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
	justifyExtra := float32(0)
	for li, line := range lines {
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
		if k.Style.Display == style.DisplayInlineBlock && k.Node != nil && k.Node.Type == 1 &&
			k.flags&flagRowPlaced == 0 {
			atomicInlineBlock(a, nodeID, contentW, lines, current)
		}
		return
	}
	if k.Node != nil && k.Node.Type == 2 {
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
func measureWord(word string, fontSize float32, m Metrics, slot frame.FontSlot, letterSpacing float32) float32 {
	n := utf8.RuneCountInString(word)
	if n == 0 {
		return 0
	}
	extra := float32(n-1) * letterSpacing
	if m == nil {
		return float32(n)*fontSize*0.5 + extra
	}
	sum := float32(0)
	for _, r := range []rune(word) {
		sum += float32(m.GlyphAdvance(int32(fontSize), r, slot))
	}
	return sum + extra
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

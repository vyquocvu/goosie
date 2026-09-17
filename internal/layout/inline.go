package layout

import (
	"strings"
	"unicode"

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
	contentW := obj.W
	var lines []lineBox
	var current lineBox
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if isBlock(k) {
			if len(current.runs) > 0 {
				lines = append(lines, current)
				current = lineBox{}
			}
			inlineInto(a, kid)
			continue
		}
		// Inline-level child: either a text node or an inline element (<a>, <span>, <b>).
		// Collect its text runs into the enclosing block's current line box so they are
		// positioned sequentially inside this block container.
		collectInline(a, kid, contentW, &lines, &current)
	}
	if len(current.runs) > 0 {
		lines = append(lines, current)
	}
	// Re-fetch obj: the Alloc calls in the text path may have reallocated the
	// arena slice, so the pointer captured at function entry is stale.
	obj = a.Get(id)
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	contentW = obj.W
	for _, line := range lines {
		baseX := obj.X + obj.PaddingLeft + obj.BorderLeft
		// Apply text-align offset.
		if obj.Style != nil && line.w < contentW {
			switch obj.Style.TextAlign {
			case style.TextAlignCenter:
				baseX += (contentW - line.w) / 2
			case style.TextAlignRight:
				baseX += contentW - line.w
			}
		}
		x := baseX
		for _, run := range line.runs {
			runObj := a.Get(run.obj)
			wordW := measureWord(run.text, run.size, a.Metrics)
			if runObj.X == 0 && runObj.Y == 0 {
				runObj.X = x
				// Half-leading: the glyph box centers in the line box, which
				// is what makes a tall line-height center its text.
				runObj.Y = y + (line.h-run.size*1.2)/2
			}
			runObj.W = wordW
			runObj.H = run.size * 1.2
			x += wordW
		}
		y += line.h
		lineH += line.h
	}
	if obj.H == 0 && lineH > 0 {
		obj.H = lineH + obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom
	}
}

// collectInline recursively traverses inline-level nodes (text nodes and inline
// elements like <a>, <span>, <b>) and collects their text runs into the
// enclosing block container's line boxes.
func collectInline(a *Arena, nodeID ObjectID, contentW float32, lines *[]lineBox, current *lineBox) {
	k := a.Get(nodeID)
	if k.Style == nil || k.Style.Display == style.DisplayNone {
		return
	}
	if isBlock(k) {
		return
	}
	if k.Node != nil && k.Node.Type == 2 {
		text := k.Node.DataContent
		if shouldCollapseWhitespace(k) {
			text = collapseWhitespace(text)
		}
		if text == "" {
			return
		}
		fontSize := k.Style.FontSize
		lineH := k.Style.LineHeight
		if lineH <= 0 {
			lineH = fontSize * 1.2
		}
		words := splitWords(text)
		var firstWordID ObjectID
		var lastWordID ObjectID
		for _, word := range words {
			wordW := measureWord(word, fontSize, a.Metrics)
			if current.w+wordW > contentW && current.w > 0 {
				*lines = append(*lines, *current)
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
				text:  word,
				size:  fontSize,
				lineH: lineH,
				obj:   wordID,
			})
			current.w += wordW
			current.h = maxF(current.h, lineH)
		}
		if lastWordID != 0 {
			parentID := a.Get(nodeID).Parent
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
			a.Get(nodeID).Node = nil
		}
		return
	}
	// Inline element (e.g. <a>, <span>, <b>): recursively collect from its children.
	for child := k.FirstKid; child != 0; child = a.Get(child).NextSibling {
		collectInline(a, child, contentW, lines, current)
	}
}

type lineBox struct {

	runs []textRun
	w, h float32
}

type textRun struct {
	text  string
	size  float32
	lineH float32
	obj   ObjectID
}

// measureWord returns the width of word at fontSize. With metrics it is the
// exact sum of the font's glyph advances; without, a half-em-per-character
// estimate. The two must stay the only sources of width so line breaking and
// painted spacing can't disagree.
func measureWord(word string, fontSize float32, m Metrics) float32 {
	if m == nil {
		return float32(len(word)) * fontSize * 0.5
	}
	sum := float32(0)
	for _, r := range word {
		sum += float32(m.GlyphAdvance(int32(fontSize), r))
	}
	return sum
}

func splitWords(text string) []string {
	var words []string
	var current strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) {
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
	case style.WhiteSpacePre, style.WhiteSpacePreline:
		return false
	}
	return true
}

func collapseWhitespace(s string) string {
	var out strings.Builder
	prevSpace := true
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
	return strings.TrimSpace(out.String())
}

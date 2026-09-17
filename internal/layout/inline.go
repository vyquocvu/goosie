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
		if k.Node != nil && k.Node.Type == 2 {
			text := k.Node.DataContent
			if shouldCollapseWhitespace(k) {
				text = collapseWhitespace(text)
			}
			if text == "" {
				continue
			}
			fontSize := k.Style.FontSize
			// A declared line-height is already px; an unset one falls back to
			// the 1.2 multiplier the CSS initial value computes to.
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
					lines = append(lines, current)
					current = lineBox{}
				}
				wordID, wordObj := a.Alloc()
				nodeCopy := *k.Node
				nodeCopy.DataContent = word
				wordObj.Node = &nodeCopy
				wordObj.Style = k.Style
				wordObj.Parent = id
				// Word-to-word links only; splicing into the parent chain
				// happens once after the loop.
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
				// Splice [firstWordID..lastWordID] into the kid chain in place
				// of the text node. Every write goes through a fresh a.Get:
				// the Alloc calls above may have reallocated the arena slice,
				// so pointers captured earlier must not be written through.
				prev := k.PrevSibling
				next := k.NextSibling
				if prev != 0 {
					a.Get(prev).NextSibling = firstWordID
					a.Get(firstWordID).PrevSibling = prev
				} else {
					a.Get(id).FirstKid = firstWordID
				}
				if next != 0 {
					a.Get(next).PrevSibling = lastWordID
					a.Get(lastWordID).NextSibling = next
				} else {
					a.Get(id).LastKid = lastWordID
				}
				// Clear the original text node so the paint builder doesn't
				// paint it.
				a.Get(kid).Node = nil
			}
			continue
		}
		inlineInto(a, kid)
	}
	if len(current.runs) > 0 {
		lines = append(lines, current)
	}
	// Re-fetch obj: the Alloc calls in the text path may have reallocated the
	// arena slice, so the pointer captured at function entry is stale.
	obj = a.Get(id)
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	for _, line := range lines {
		x := obj.X + obj.PaddingLeft + obj.BorderLeft
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

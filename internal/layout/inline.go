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
			words := splitWords(text)
			var firstWordID ObjectID
			var prevWordID ObjectID
			for _, word := range words {
				wordW := measureWord(word, fontSize)
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
				// Link word object into arena's child list
				if firstWordID == 0 {
					firstWordID = wordID
					obj.FirstKid = wordID
				} else {
					a.Get(prevWordID).NextSibling = wordID
					wordObj.PrevSibling = prevWordID
				}
				prevWordID = wordID
				current.runs = append(current.runs, textRun{
					text: word,
					size: fontSize,
					obj:  wordID,
				})
				current.w += wordW
				current.h = maxF(current.h, fontSize*1.2)
			}
			if prevWordID != 0 {
				obj.LastKid = prevWordID
			}
			// Clear the original text node so the paint builder doesn't paint it
			k.Node = nil
			continue
		}
		inlineInto(a, kid)
	}
	if len(current.runs) > 0 {
		lines = append(lines, current)
	}
	y := obj.Y + obj.PaddingTop + obj.BorderTop
	lineH := float32(0)
	for _, line := range lines {
		x := obj.X + obj.PaddingLeft + obj.BorderLeft
		for _, run := range line.runs {
			runObj := a.Get(run.obj)
			wordW := measureWord(run.text, run.size)
			if runObj.X == 0 && runObj.Y == 0 {
				runObj.X = x
				runObj.Y = y
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
	text string
	size float32
	obj  ObjectID
}

func measureWord(word string, fontSize float32) float32 {
	return float32(len(word)) * fontSize * 0.5
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

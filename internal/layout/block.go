package layout

import "github.com/vyquocvu/goosie/internal/style"

// Block runs the block layout pass over the arena.
//
// Block layout is the first pass and the one every other pass depends on: it
// sets the width and height of every block box, positions block kids vertically
// with margin collapsing, and establishes the containing block for positioned
// descendants. The pass is a single recursive walk; the recursion is the
// containing-block stack.
func Block(a *Arena, root ObjectID, viewportW float32) {
	blockInto(a, root, viewportW)
}

func blockInto(a *Arena, id ObjectID, containingW float32) float32 {
	obj := a.Get(id)
	if obj.Style != nil {
		resolveBoxSizes(obj, containingW)
		if obj.Style.Display == style.DisplayNone {
			obj.W = 0
			obj.H = 0
			return 0
		}
	}
	// The content box is inside the padding. Children are positioned relative to
	// the content box origin, and the containing width for percentage children
	// is the content width.
	contentX := obj.X + obj.BorderLeft + obj.PaddingLeft
	contentY := obj.Y + obj.BorderTop + obj.PaddingTop
	contentW := obj.W
	if contentW <= 0 {
		contentW = containingW - obj.BorderLeft - obj.PaddingLeft - obj.PaddingRight - obj.BorderRight
	}
	if contentW < 0 {
		contentW = 0
	}
	y := float32(0)
	prevBottomMargin := float32(0)
	firstChild := true
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if isBlock(k) {
			blockInto(a, kid, contentW)
			// Margin collapsing: between adjacent siblings, the collapsed margin
			// is max(prevBottom, thisTop) for positive margins.
			var topMargin float32
			if firstChild {
				topMargin = k.MarginTop
				firstChild = false
			} else {
				topMargin = collapseMargin(prevBottomMargin, k.MarginTop)
			}
			k.X = contentX + k.MarginLeft
			k.Y = contentY + y + topMargin
			// y advances to the bottom border edge of the child
			y = (k.Y - contentY) + k.H
			prevBottomMargin = k.MarginBottom
		}
	}
	// Add the last child's bottom margin to the total height
	y += prevBottomMargin
	// Auto height is the content height plus padding and borders.
	if obj.Style != nil && obj.Style.Height < 0 {
		obj.H = y + obj.PaddingTop + obj.PaddingBottom + obj.BorderTop + obj.BorderBottom
	}
	if obj.Style == nil {
		return obj.H
	}
	return obj.H + obj.MarginTop + obj.MarginBottom
}

func resolveBoxSizes(obj *Object, containingW float32) {
	s := obj.Style
	if s == nil {
		return
	}
	obj.MarginTop = s.MarginTop
	obj.MarginRight = s.MarginRight
	obj.MarginBottom = s.MarginBottom
	obj.MarginLeft = s.MarginLeft
	obj.PaddingTop = s.PaddingTop
	obj.PaddingRight = s.PaddingRight
	obj.PaddingBottom = s.PaddingBottom
	obj.PaddingLeft = s.PaddingLeft
	obj.BorderTop = s.BorderTopWidth
	obj.BorderRight = s.BorderRightWidth
	obj.BorderBottom = s.BorderBottomWidth
	obj.BorderLeft = s.BorderLeftWidth

	// Width: -1 means auto. For block boxes, auto fills the containing block.
	if s.Width >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			obj.W = s.Width - obj.PaddingLeft - obj.PaddingRight - obj.BorderLeft - obj.BorderRight
		} else {
			obj.W = s.Width
		}
	} else if s.Display != style.DisplayInline {
		obj.W = containingW - obj.MarginLeft - obj.MarginRight -
			obj.PaddingLeft - obj.PaddingRight - obj.BorderLeft - obj.BorderRight
	}
	if obj.W < 0 {
		obj.W = 0
	}

	// Height: -1 means auto, resolved later by blockInto from children.
	if s.Height >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			obj.H = s.Height - obj.PaddingTop - obj.PaddingBottom - obj.BorderTop - obj.BorderBottom
		} else {
			obj.H = s.Height
		}
	}
	if obj.H < 0 {
		obj.H = 0
	}
}

func isBlock(obj *Object) bool {
	if obj.Style == nil {
		return false
	}
	switch obj.Style.Display {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayTable,
		style.DisplayFlex:
		return true
	}
	return false
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

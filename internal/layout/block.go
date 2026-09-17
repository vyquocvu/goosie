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
	if obj.Style != nil && obj.Style.Display == style.DisplayFlex {
		// Flex containers lay their items out on a row, not in the block flow,
		// so the whole kid walk below is replaced. No margin collapsing
		// happens in or through a flex container.
		y = layoutFlexRow(a, obj, contentX, contentY, contentW)
	} else {
		for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
			k := a.Get(kid)
			if k.Style == nil || k.Style.Display == style.DisplayNone {
				continue
			}
			if isBlock(k) {
				// Pre-resolve box sizes for the child before positioning: this
				// computes k.W and resolves auto horizontal margins (centering)
				// so k.MarginLeft is correct when we set k.X below.
				// resolveBoxSizes is idempotent and will run again at the top of
				// the recursive blockInto call; calling it here is not redundant
				// because k.X/k.Y are not yet known (they depend on the margins
				// we are about to compute).
				resolveBoxSizes(k, contentW)

				var topMargin float32
				if firstChild {
					topMargin = k.MarginTop
					firstChild = false
				} else {
					topMargin = collapseMargin(prevBottomMargin, k.MarginTop)
				}
				k.X = contentX + k.MarginLeft
				k.Y = contentY + y + topMargin
				blockInto(a, kid, contentW)
				// y advances to the bottom border edge of the child
				y = (k.Y - contentY) + k.H
				prevBottomMargin = k.MarginBottom
			}
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


// layoutFlexRow lays out a single-line row flex container.
//
// This is the minimal flex pass the gate fixture needs: items on one row,
// free space distributed by flex-grow, horizontal margins respected, and
// auto-height items stretched to the container's content height (the CSS
// align-items:stretch default). Items with flex-grow 0 and an explicit width
// take that width; auto-width non-growing items are not yet supported and
// collapse to zero. Raw text inside a flex container is skipped rather than
// wrapped in an anonymous item.
func layoutFlexRow(a *Arena, obj *Object, contentX, contentY, contentW float32) float32 {
	type item struct {
		id    ObjectID
		obj   *Object
		share float32
	}
	var items []item
	hMargins := float32(0)
	growSum := float32(0)
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone {
			continue
		}
		if k.Node != nil && k.Node.Type != 1 {
			continue
		}
		items = append(items, item{id: kid, obj: k})
		hMargins += k.Style.MarginLeft + k.Style.MarginRight
		growSum += k.Style.FlexGrow
	}
	free := contentW - hMargins
	if free < 0 {
		free = 0
	}
	for i := range items {
		it := &items[i]
		s := it.obj.Style
		if s.FlexGrow > 0 && growSum > 0 {
			it.share = free * s.FlexGrow / growSum
		} else if s.Width >= 0 {
			it.share = s.Width
			if s.BoxSizing != style.BoxSizingBorderBox {
				it.share += s.PaddingLeft + s.PaddingRight + s.BorderLeftWidth + s.BorderRightWidth
			}
			it.share += s.MarginLeft + s.MarginRight
		}
	}
	contentH := obj.H - obj.PaddingTop - obj.PaddingBottom - obj.BorderTop - obj.BorderBottom
	cursor := contentX
	maxBottom := float32(0)
	for _, it := range items {
		// X and Y before recursing, same rule as the block branch: the
		// recursion derives every descendant's origin from them. The item
		// recursion receives the share as its containing width so an auto
		// width resolves to share minus margins, padding, and borders.
		it.obj.X = cursor + it.obj.Style.MarginLeft
		it.obj.Y = contentY
		blockInto(a, it.id, it.share)
		// align-items: stretch. The item's own height was just resolved;
		// only an auto-height item stretches, and only into a definite
		// container height.
		if contentH > 0 && it.obj.Style.Height < 0 {
			it.obj.H = contentH
		}
		if bottom := (it.obj.Y - contentY) + it.obj.H; bottom > maxBottom {
			maxBottom = bottom
		}
		cursor += it.share
	}
	return maxBottom
}

func resolveBoxSizes(obj *Object, containingW float32) {
	s := obj.Style
	if s == nil {
		return
	}
	obj.PaddingTop = s.PaddingTop
	obj.PaddingRight = s.PaddingRight
	obj.PaddingBottom = s.PaddingBottom
	obj.PaddingLeft = s.PaddingLeft
	obj.BorderTop = s.BorderTopWidth
	obj.BorderRight = s.BorderRightWidth
	obj.BorderBottom = s.BorderBottomWidth
	obj.BorderLeft = s.BorderLeftWidth

	innerExtra := obj.PaddingLeft + obj.PaddingRight + obj.BorderLeft + obj.BorderRight

	// Width: -1 means auto. Compute explicit width first so auto-margin
	// centering can use it.
	var explicitW float32 = -1
	if s.Width >= 0 {
		if s.BoxSizing == style.BoxSizingBorderBox {
			explicitW = s.Width - innerExtra
		} else {
			explicitW = s.Width
		}
		if explicitW < 0 {
			explicitW = 0
		}
	}

	// Margin auto: MarginAuto (-1) from the style means the CSS value was
	// "auto". For horizontal margins on a block with an explicit width, auto
	// distributes the remaining space (CSS 2.1 §10.3.3). For auto width or
	// vertical margins, auto margins collapse to 0.
	leftAuto := s.MarginLeft == style.MarginAuto
	rightAuto := s.MarginRight == style.MarginAuto

	if explicitW >= 0 && (leftAuto || rightAuto) {
		// Remaining space the two horizontal margins share.
		remaining := containingW - explicitW - innerExtra
		if remaining < 0 {
			remaining = 0
		}
		if leftAuto && rightAuto {
			obj.MarginLeft = remaining / 2
			obj.MarginRight = remaining / 2
		} else if leftAuto {
			obj.MarginLeft = remaining
			obj.MarginRight = s.MarginRight
		} else {
			obj.MarginRight = remaining
			obj.MarginLeft = s.MarginLeft
		}
	} else {
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
	}
	obj.MarginTop = s.MarginTop
	obj.MarginBottom = s.MarginBottom

	// Width: -1 means auto. For block boxes, auto fills the containing block.
	if explicitW >= 0 {
		obj.W = explicitW
	} else if s.Display != style.DisplayInline {
		obj.W = containingW - obj.MarginLeft - obj.MarginRight - innerExtra
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

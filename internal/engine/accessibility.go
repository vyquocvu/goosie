package engine

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// AccessibilityTree derives the document's accessibility tree from the layout
// arena: one document node whose children mirror the laid-out elements. Links,
// buttons, images, and text fields become their own nodes; a block whose
// content is pure text becomes static text; everything else becomes a group.
// Bounds are document coordinates in CSS pixels, matching the last Reflow. A
// nil arena yields nil.
func (s *Session) AccessibilityTree() []ax.Node {
	if s.Arena == nil || len(s.Arena.Objects) < 2 {
		return nil
	}
	b := &axBuilder{session: s, objs: s.Arena.Objects}
	root, bubble := b.visit(1)
	if root == nil {
		if bubble.text == "" {
			return nil
		}
		root = &ax.Node{Role: ax.RoleStaticText, Label: bubble.text,
			X0: bubble.x0, Y0: bubble.y0, X1: bubble.x1, Y1: bubble.y1}
	}
	doc := ax.Node{
		Role: ax.RoleDocument,
		X0:   root.X0, Y0: root.Y0, X1: root.X1, Y1: root.Y1,
	}
	if root.Role == ax.RoleStaticText {
		doc.Children = append(doc.Children, *root)
	} else {
		doc.Children = append(doc.Children, root.Children...)
	}
	return []ax.Node{doc}
}

type axBuilder struct {
	session *Session
	objs    []layout.Object
}

// axBubble is the text a box contributes to its nearest emitted ancestor, with
// the document-space rect that text occupies.
type axBubble struct {
	text           string
	x0, y0, x1, y1 float32
}

// visit returns the accessible node the box at id contributes, or nil when the
// box only carries text - word boxes and inline text bearers - plus that text
// so it can join the nearest emitted ancestor.
func (b *axBuilder) visit(id layout.ObjectID) (*ax.Node, axBubble) {
	if id == 0 || int(id) >= len(b.objs) {
		return nil, axBubble{}
	}
	obj := &b.objs[id]
	if obj.Node == nil {
		// Anonymous box layout invented to hold inline content: a single text
		// run passes through as text, anything more structured stays a group.
		kids, _ := b.collect(obj)
		if len(kids) == 1 && kids[0].Role == ax.RoleStaticText {
			k := kids[0]
			return nil, axBubble{text: k.Label, x0: k.X0, y0: k.Y0, x1: k.X1, y1: k.Y1}
		}
		if len(kids) == 0 {
			return nil, axBubble{}
		}
		x0, y0, x1, y1 := b.bounds(obj, kids)
		return &ax.Node{Role: ax.RoleGroup, X0: x0, Y0: y0, X1: x1, Y1: y1, Children: kids}, axBubble{}
	}
	if obj.Node.Type == dom.NodeText {
		t := strings.TrimSpace(obj.Node.DataContent)
		x0, y0, x1, y1 := obj.BorderRect()
		if t == "" || !(x0 < x1 && y0 < y1) {
			return nil, axBubble{}
		}
		return nil, axBubble{text: t, x0: x0, y0: y0, x1: x1, y1: y1}
	}

	kids, label := b.collect(obj)
	x0, y0, x1, y1 := b.bounds(obj, kids)
	visible := x0 < x1 && y0 < y1
	emit := func(role ax.Role, lab, val, href string) *ax.Node {
		return &ax.Node{Role: role, Label: lab, Value: val, Href: href,
			X0: x0, Y0: y0, X1: x1, Y1: y1, Children: kids}
	}

	n := obj.Node
	switch n.DataAtom {
	case dom.AtomA:
		if href := n.GetAttribute("href"); href != "" && visible {
			return emit(ax.RoleLink, label, "", href), axBubble{}
		}
		return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
	case dom.AtomImg:
		if !visible {
			return nil, axBubble{}
		}
		return emit(ax.RoleImage, n.GetAttribute("alt"), "", ""), axBubble{}
	case dom.AtomButton:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		return emit(ax.RoleButton, label, "", ""), axBubble{}
	case dom.AtomInput:
		typ := strings.ToLower(n.GetAttribute("type"))
		if !visible || typ == "hidden" {
			return nil, axBubble{}
		}
		switch typ {
		case "button", "submit", "reset":
			return emit(ax.RoleButton, n.GetAttribute("value"), "", ""), axBubble{}
		case "checkbox", "radio", "range", "file", "color",
			"date", "time", "datetime-local", "month", "week":
			return nil, axBubble{}
		default:
			return emit(ax.RoleTextField, n.GetAttribute("placeholder"), n.GetAttribute("value"), ""), axBubble{}
		}
	case dom.AtomTextarea:
		if !visible {
			return nil, axBubble{}
		}
		node := emit(ax.RoleTextField, n.GetAttribute("placeholder"), b.session.controlValue(n), "")
		node.Children = nil
		return node, axBubble{}
	}

	// Generic element: an inline text bearer's words join the nearest block
	// ancestor; a block keeps its own group even when it holds only text, so
	// containers like body and paragraphs survive as structure.
	hasKids := false
	for _, k := range kids {
		if k.Role != ax.RoleStaticText {
			hasKids = true
			break
		}
	}
	if !hasKids && b.inlineBox(obj) {
		return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
	}
	if len(kids) == 0 {
		return nil, axBubble{}
	}
	return emit(ax.RoleGroup, "", "", ""), axBubble{}
}

// collect walks obj's children in document order and returns the accessible
// nodes they contribute with runs of bare text merged into static-text nodes
// where they occurred, plus the bare text alone for labelling widgets.
func (b *axBuilder) collect(obj *layout.Object) ([]ax.Node, string) {
	var kids []ax.Node
	var words []string
	lastWasText := false
	appendText := func(t string, bx0, by0, bx1, by1 float32) {
		if n := len(kids); lastWasText && n > 0 && kids[n-1].Role == ax.RoleStaticText {
			k := &kids[n-1]
			k.Label += " " + t
			if bx0 < k.X0 {
				k.X0 = bx0
			}
			if by0 < k.Y0 {
				k.Y0 = by0
			}
			if bx1 > k.X1 {
				k.X1 = bx1
			}
			if by1 > k.Y1 {
				k.Y1 = by1
			}
			return
		}
		kids = append(kids, ax.Node{Role: ax.RoleStaticText, Label: t,
			X0: bx0, Y0: by0, X1: bx1, Y1: by1})
		lastWasText = true
	}
	for k := obj.FirstKid; k != 0 && int(k) < len(b.objs); k = b.objs[k].NextSibling {
		ck := &b.objs[k]
		if ck.Node != nil && ck.Node.Type == dom.NodeText {
			if t := strings.TrimSpace(ck.Node.DataContent); t != "" {
				if rx0, ry0, rx1, ry1 := ck.BorderRect(); rx0 < rx1 && ry0 < ry1 {
					words = append(words, t)
					appendText(t, rx0, ry0, rx1, ry1)
				}
			}
			continue
		}
		node, bubble := b.visit(k)
		if node != nil {
			kids = append(kids, *node)
			lastWasText = false
			continue
		}
		if bubble.text != "" {
			words = append(words, bubble.text)
			appendText(bubble.text, bubble.x0, bubble.y0, bubble.x1, bubble.y1)
		}
	}
	return kids, strings.Join(words, " ")
}

// bounds returns the box's border rect, falling back to the union of the
// children's rects: inline boxes carry no geometry of their own - their words
// do - and so do unsized roots.
func (b *axBuilder) bounds(obj *layout.Object, kids []ax.Node) (x0, y0, x1, y1 float32) {
	x0, y0, x1, y1 = obj.BorderRect()
	if x0 < x1 && y0 < y1 {
		return
	}
	for _, k := range kids {
		if k.X1 <= k.X0 && k.Y1 <= k.Y0 {
			continue
		}
		if x1 <= x0 && y1 <= y0 {
			x0, y0, x1, y1 = k.X0, k.Y0, k.X1, k.Y1
			continue
		}
		if k.X0 < x0 {
			x0 = k.X0
		}
		if k.Y0 < y0 {
			y0 = k.Y0
		}
		if k.X1 > x1 {
			x1 = k.X1
		}
		if k.Y1 > y1 {
			y1 = k.Y1
		}
	}
	return
}

// inlineBox reports whether the box is display:inline, whose text bubbles to
// the nearest block-level ancestor instead of becoming its own static text.
func (b *axBuilder) inlineBox(obj *layout.Object) bool {
	return obj.Style != nil && obj.Style.Display == style.DisplayInline
}

package layout

import (
	"strings"
	"unicode"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

type ObjectID uint32

const rootID ObjectID = 1

type Object struct {
	Node  *dom.Node
	Style *style.ComputedStyle

	Key BoxKey

	Parent ObjectID

	FirstKid, LastKid        ObjectID
	NextSibling, PrevSibling ObjectID

	X, Y, W, H                                           float32
	DefiniteH                                            float32
	MarginTop, MarginRight, MarginBottom, MarginLeft     float32
	PaddingTop, PaddingRight, PaddingBottom, PaddingLeft float32
	BorderTop, BorderRight, BorderBottom, BorderLeft     float32

	CollapsedBorderRight  *style.ComputedStyle
	CollapsedBorderBottom *style.ComputedStyle

	GlyphOrder []int

	BidiLevels []byte

	InlineFragments []frame.RectF

	GridCols    []float32
	GridRows    []float32
	GridCol     int
	GridRow     int
	GridColSpan int
	GridRowSpan int

	Image any

	BgImage any

	MaskImage any

	BorderImage any

	Media any

	BgVector bool

	BgNoRatio bool

	StaticX, StaticY float32

	floatBottomL, floatBottomR float32
	floatSeedL, floatSeedR     float32
	floatTop                   float32
	floatSeedTop               float32
	floatSeedHas               bool

	Baseline float32
	flags    uint8
}

const (
	flagInlineLaidOut uint8 = 1 << iota
	flagOutOfFlow
	flagRowPlaced
	flagAnonymous
	flagMinHeightClamped
)

func (o *Object) IsImageInput() bool {
	return o != nil && dom.IsImageInput(o.Node)
}

func (o *Object) ContentRect() (x0, y0, x1, y1 float32) {
	x0 = o.X + o.BorderLeft + o.PaddingLeft
	y0 = o.Y + o.BorderTop + o.PaddingTop
	x1 = x0 + o.W
	y1 = y0 + o.H
	return
}

func (o *Object) BorderRect() (x0, y0, x1, y1 float32) {
	x0 = o.X
	y0 = o.Y
	x1 = x0 + o.W + o.PaddingLeft + o.PaddingRight + o.BorderLeft + o.BorderRight
	y1 = y0 + o.H + o.PaddingTop + o.PaddingBottom + o.BorderTop + o.BorderBottom
	return
}

func (o *Object) BgAreaRect(box style.BgBox) (x0, y0, x1, y1 float32) {
	x0, y0, x1, y1 = o.BorderRect()
	switch box {
	case style.BgBoxPadding, style.BgBoxContent:
		x0 += o.BorderLeft
		y0 += o.BorderTop
		x1 -= o.BorderRight
		y1 -= o.BorderBottom
		if box == style.BgBoxContent {
			x0 += o.PaddingLeft
			y0 += o.PaddingTop
			x1 -= o.PaddingRight
			y1 -= o.PaddingBottom
		}
	}
	return x0, y0, x1, y1
}

func (o *Object) BorderW() float32 {
	return o.W + o.PaddingLeft + o.PaddingRight + o.BorderLeft + o.BorderRight
}

func (o *Object) BorderH() float32 {
	return o.H + o.PaddingTop + o.PaddingBottom + o.BorderTop + o.BorderBottom
}

type Metrics interface {
	GlyphAdvance(sizePx int32, r rune, slot frame.FontSlot) int32
	GlyphAdvanceFixed(sizePx int32, r rune, slot frame.FontSlot) int32
	LineMetrics(sizePx int32, slot frame.FontSlot) (ascent, descent, normalLineHeight int32)
	XHeightFixed(sizePx int32, slot frame.FontSlot) int32
}

type Arena struct {
	Objects []Object
	byNode  map[dom.NodeID]ObjectID

	byKey   map[BoxKey]ObjectID
	wordSeq uint16
	anonSeq uint16

	PassCounts [5]uint64

	Metrics Metrics

	QuirksMode bool

	NaturalSizes map[dom.NodeID]NaturalSize

	ViewportH float32

	ViewportW float32

	MeasurePctSet bool
	MeasurePctW   float32
	MeasurePctH   float32

	QuoteDepth int

	FirstLineStyles map[dom.NodeID]*style.ComputedStyle

	PlaceholderStyles map[dom.NodeID]*style.ComputedStyle

	inlineFragLines map[ObjectID]map[int]frame.RectF
}

type NaturalSize struct {
	W, H       float32
	FromSource bool
}

func NewArena() *Arena {
	a := &Arena{
		Objects: make([]Object, 1, 64),
		byNode:  make(map[dom.NodeID]ObjectID),
		byKey:   make(map[BoxKey]ObjectID),
	}
	a.Objects = append(a.Objects, Object{DefiniteH: -1})
	return a
}

func (a *Arena) Alloc() (ObjectID, *Object) {
	id := ObjectID(len(a.Objects))
	a.Objects = append(a.Objects, Object{DefiniteH: -1})
	return id, &a.Objects[id]
}

func (a *Arena) Get(id ObjectID) *Object {
	return &a.Objects[id]
}

func (a *Arena) ForNode(id dom.NodeID) (ObjectID, bool) {
	oid, ok := a.byNode[id]
	return oid, ok
}

func (a *Arena) setKey(id ObjectID, k BoxKey) {
	a.Objects[id].Key = k
	if a.byKey == nil {
		a.byKey = make(map[BoxKey]ObjectID)
	}
	a.byKey[k] = id
}

func (a *Arena) ForKey(k BoxKey) (ObjectID, bool) {
	id, ok := a.byKey[k]
	return id, ok
}

func (a *Arena) anonNodeID(id ObjectID) uint32 {
	for cur := id; cur != 0; cur = a.Objects[cur].Parent {
		if n := a.Objects[cur].Node; n != nil && n.ID != 0 {
			return uint32(n.ID)
		}
	}
	return 0
}

func (a *Arena) FCCount(k FCKind) uint64 { return a.PassCounts[k] }

func (a *Arena) RefreshComputedStyles(styles map[dom.NodeID]*style.ComputedStyle) bool {
	if a == nil {
		return false
	}
	for i := 1; i < len(a.Objects); i++ {
		obj := &a.Objects[i]
		if obj.flags&flagAnonymous != 0 {
			return false
		}
		if obj.Node != nil && obj.Node.ID != 0 {
			if _, ok := styles[obj.Node.ID]; !ok && obj.Style != nil {
				return false
			}
		}
	}
	for i := 1; i < len(a.Objects); i++ {
		obj := &a.Objects[i]
		if obj.Node != nil {
			obj.Style = styles[obj.Node.ID]
		}
	}
	return true
}

func (a *Arena) RefreshComputedStylesForNodes(styles map[dom.NodeID]*style.ComputedStyle, nodes []dom.NodeID) bool {
	if a == nil {
		return false
	}
	if len(nodes) == 0 {
		return true
	}
	for i := 1; i < len(a.Objects); i++ {
		if a.Objects[i].flags&flagAnonymous != 0 {
			return false
		}
	}
	wanted := make(map[dom.NodeID]struct{}, len(nodes))
	for _, id := range nodes {
		wanted[id] = struct{}{}
	}
	for i := 1; i < len(a.Objects); i++ {
		obj := &a.Objects[i]
		if obj.Node == nil {
			continue
		}
		if _, ok := wanted[obj.Node.ID]; ok {
			obj.Style = styles[obj.Node.ID]
		}
	}
	return true
}

func (a *Arena) Link(parent, kid ObjectID) {
	p := &a.Objects[parent]
	k := &a.Objects[kid]
	k.Parent = parent
	if p.FirstKid == 0 {
		p.FirstKid = kid
		p.LastKid = kid
		return
	}
	last := &a.Objects[p.LastKid]
	last.NextSibling = kid
	k.PrevSibling = p.LastKid
	p.LastKid = kid
}

func Build(doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, pseudoStyles map[style.PseudoKey]*style.ComputedStyle) *Arena {
	a := NewArena()
	if doc != nil {
		a.QuirksMode = doc.QuirksMode
		if n := countLayoutNodes(&doc.Node); n > 0 {
			need := n + 2*len(pseudoStyles) + 2
			if need > cap(a.Objects) {
				objs := make([]Object, len(a.Objects), need)
				copy(objs, a.Objects)
				a.Objects = objs
			}
			if need > 0 {
				byNode := make(map[dom.NodeID]ObjectID, need)
				for k, v := range a.byNode {
					byNode[k] = v
				}
				a.byNode = byNode
			}
		}
	}
	a.Objects[rootID].Node = &doc.Node
	a.setKey(rootID, NewBoxKey(uint32(doc.Node.ID), BoxKindBlock, 0))
	if s, ok := styles[doc.Node.ID]; ok {
		a.Objects[rootID].Style = s
	}
	a.byNode[doc.Node.ID] = rootID
	for key, cs := range pseudoStyles {
		if key.Pseudo == "first-line" {
			if a.FirstLineStyles == nil {
				a.FirstLineStyles = make(map[dom.NodeID]*style.ComputedStyle)
			}
			a.FirstLineStyles[key.NodeID] = cs
		} else if key.Pseudo == "placeholder" {
			if a.PlaceholderStyles == nil {
				a.PlaceholderStyles = make(map[dom.NodeID]*style.ComputedStyle)
			}
			a.PlaceholderStyles[key.NodeID] = cs
		}
	}
	rootScope := &style.CounterScope{}
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() || c.Type == dom.NodeText {
			kid := buildSubtree(a, c, doc, styles, pseudoStyles, rootScope)
			a.Link(rootID, kid)
		}
	}
	a.Objects[rootID].X = 0
	a.Objects[rootID].Y = 0
	return a
}

func countLayoutNodes(root *dom.Node) int {
	n := 0
	stack := []*dom.Node{root}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.ShadowRoot != nil {
			for c := top.ShadowRoot.FirstChild; c != nil; c = c.NextSibling {
				if c.Element() || c.Type == dom.NodeText {
					n++
				}
				if c.FirstChild != nil {
					stack = append(stack, c)
				}
			}
		}
		for c := top.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() || c.Type == dom.NodeText {
				n++
			}
			if c.FirstChild != nil {
				stack = append(stack, c)
			}
		}
	}
	return n
}

func buildSubtree(a *Arena, n *dom.Node, doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, pseudoStyles map[style.PseudoKey]*style.ComputedStyle, scope *style.CounterScope) ObjectID {
	return buildSubtreeWithHost(a, n, nil, doc, styles, pseudoStyles, scope)
}

func buildSubtreeWithHost(a *Arena, n *dom.Node, shadowHost *dom.Node, doc *dom.Document, styles map[dom.NodeID]*style.ComputedStyle, pseudoStyles map[style.PseudoKey]*style.ComputedStyle, scope *style.CounterScope) ObjectID {
	if n.ID != 0 {
		if id, ok := a.ForNode(n.ID); ok {
			return id
		}
	}
	id, obj := a.Alloc()
	if n.ID != 0 {
		a.byNode[n.ID] = id
	}
	obj.Node = n
	if s, ok := styles[n.ID]; ok {
		obj.Style = s
	}
	if n.ID != 0 {
		a.setKey(id, NewBoxKey(uint32(n.ID), boxKindForStyle(obj.Style), 0))
	}

	if n.Element() && obj.Style != nil {
		scope = scope.ApplyReset(obj.Style.CounterReset)
		scope = scope.ApplySet(obj.Style.CounterSet)
		scope = scope.ApplyIncrement(obj.Style.CounterIncrement)
	}

	if n.Element() && pseudoStyles != nil {
		if beforeStyle, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "before"}]; ok {
			beforeID := buildPseudoElement(a, doc, n.ID, BoxKindPseudoBefore, beforeStyle, scope)
			a.Link(id, beforeID)
		}
	}

	var childHead *dom.Node
	currentHost := shadowHost
	if n.ShadowRoot != nil {
		childHead = n.ShadowRoot.FirstChild
		currentHost = n
	} else {
		childHead = n.FirstChild
	}
	textareaText := (*dom.Node)(nil)
	if n.Data == "textarea" {
		for c := childHead; c != nil; c = c.NextSibling {
			if c.Text() {
				textareaText = c
				break
			}
		}
		if n.InputValueInitialized && textareaText == nil && n.InputControlValue() != "" {
			virtualText := &dom.Node{Type: dom.NodeText, DataContent: n.InputControlValue(), Parent: n, Doc: doc}
			kid := buildSubtreeWithHost(a, virtualText, nil, doc, styles, pseudoStyles, scope)
			if obj.Style != nil {
				textStyle := *obj.Style
				textStyle.Display = style.DisplayInline
				a.Get(kid).Style = &textStyle
			}
			a.Link(id, kid)
		}
	}

	var flTextNode *dom.Node
	var flRemNode *dom.Node
	if n.Element() && pseudoStyles != nil {
		if flStyle, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "first-letter"}]; ok && style.PseudoStyleApplies("first-letter", flStyle) {
			for c := childHead; c != nil; c = c.NextSibling {
				if !c.Element() && c.Type != dom.NodeText {
					continue
				}
				if c.Element() {
					break
				}
				if c.Type == dom.NodeText {
					if strings.TrimSpace(c.DataContent) != "" {
						flTextNode = c
						break
					}
				}
			}
			if flTextNode != nil {
				runes := []rune(flTextNode.DataContent)
				firstIdx := -1
				for i, r := range runes {
					if !unicode.IsSpace(r) {
						firstIdx = i
						break
					}
				}
				if firstIdx >= 0 {
					letter := string(runes[firstIdx])
					remCopy := *flTextNode
					remCopy.DataContent = string(runes[firstIdx+1:])
					flRemNode = &remCopy
					flID := buildFirstLetterPseudoElement(a, doc, n.ID, letter, flStyle)
					a.Link(id, flID)
				}
			}
		}
	}

	for c := childHead; c != nil; c = c.NextSibling {
		if !c.Element() && c.Type != dom.NodeText {
			continue
		}
		if c.Element() && c.Data == "slot" && currentHost != nil {
			slotName := c.GetAttribute("name")
			var assigned []*dom.Node
			for lc := currentHost.FirstChild; lc != nil; lc = lc.NextSibling {
				if !lc.Element() && lc.Type != dom.NodeText {
					continue
				}
				if lc.Type == dom.NodeText && strings.TrimSpace(lc.DataContent) == "" {
					continue
				}
				if slotName != "" {
					if lc.Element() && lc.GetAttribute("slot") == slotName {
						assigned = append(assigned, lc)
					}
				} else {
					if !lc.Element() || lc.GetAttribute("slot") == "" {
						assigned = append(assigned, lc)
					}
				}
			}
			if len(assigned) > 0 {
				for _, ak := range assigned {
					kid := buildSubtreeWithHost(a, ak, nil, doc, styles, pseudoStyles, scope)
					a.Link(id, kid)
				}
			} else {
				for sc := c.FirstChild; sc != nil; sc = sc.NextSibling {
					if sc.Element() || sc.Type == dom.NodeText {
						kid := buildSubtreeWithHost(a, sc, currentHost, doc, styles, pseudoStyles, scope)
						a.Link(id, kid)
					}
				}
			}
			continue
		}

		layoutChild := c
		if n.Data == "textarea" && n.InputValueInitialized && c.Text() {
			textCopy := *c
			if c == textareaText {
				textCopy.DataContent = n.InputControlValue()
			} else {
				textCopy.DataContent = ""
			}
			layoutChild = &textCopy
		}
		kid := buildSubtreeWithHost(a, layoutChild, currentHost, doc, styles, pseudoStyles, scope)
		if c == flTextNode && flRemNode != nil {
			a.Get(kid).Node = flRemNode
		}
		a.Link(id, kid)
	}

	if n.Element() && pseudoStyles != nil {
		if afterStyle, ok := pseudoStyles[style.PseudoKey{NodeID: n.ID, Pseudo: "after"}]; ok {
			afterID := buildPseudoElement(a, doc, n.ID, BoxKindPseudoAfter, afterStyle, scope)
			a.Link(id, afterID)
		}
	}

	return id
}

func buildPseudoElement(a *Arena, doc *dom.Document, nodeID dom.NodeID, kind BoxKind, pseudoStyle *style.ComputedStyle, scope *style.CounterScope) ObjectID {
	id, obj := a.Alloc()
	obj.flags |= flagAnonymous
	obj.Style = pseudoStyle
	a.setKey(id, NewBoxKey(uint32(nodeID), kind, 0))

	text := pseudoStyle.Content
	if len(pseudoStyle.ContentParts) > 0 {
		text = style.EvaluateContentWithQuotes(pseudoStyle.ContentParts, scope, pseudoStyle.Quotes, &a.QuoteDepth)
	}
	textNode := doc.NewText(text)
	obj.Node = textNode

	return id
}

func buildFirstLetterPseudoElement(a *Arena, doc *dom.Document, nodeID dom.NodeID, letter string, pseudoStyle *style.ComputedStyle) ObjectID {
	id, obj := a.Alloc()
	st := pseudoStyle
	if st.Float != style.FloatNone && st.Display == style.DisplayInline {
		stCopy := *st
		stCopy.Display = style.DisplayBlock
		st = &stCopy
	}
	obj.Style = st
	a.setKey(id, NewBoxKey(uint32(nodeID), BoxKindPseudoFirstLetter, 0))
	obj.Node = doc.NewText(letter)
	return id
}

type ViewTransitionRect struct {
	Name           string
	X0, Y0, X1, Y1 float32
}

func (a *Arena) FindViewTransitions() []ViewTransitionRect {
	var result []ViewTransitionRect
	for i := range a.Objects {
		obj := &a.Objects[i]
		if obj.Node != nil && obj.Node.Element() && obj.Style != nil && obj.Style.ViewTransitionName != "" && obj.Style.ViewTransitionName != "none" {
			x0, y0, x1, y1 := obj.BorderRect()
			result = append(result, ViewTransitionRect{
				Name: obj.Style.ViewTransitionName,
				X0:   x0, Y0: y0, X1: x1, Y1: y1,
			})
		}
	}
	return result
}

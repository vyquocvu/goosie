package layout

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/style"
)

func wrapInlineRuns(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.flags&flagAnonymous != 0 {
		return
	}
	stacked := false
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		if isFlowBlock(a.Get(kid)) {
			stacked = true
			break
		}
	}
	if !stacked {
		return
	}
	anonStyle := style.AnonymousBlockStyle(obj.Style)
	var run []ObjectID
	flush := func() {
		if len(run) > 0 && runGeneratesContent(a, run) {
			spliceAnonymousBlock(a, id, run, anonStyle)
		}
		run = nil
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isFlowBlock(k) {
			flush()
			continue
		}
		run = append(run, kid)
	}
	flush()
}

func wrapTextRuns(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.flags&flagAnonymous != 0 {
		return
	}
	anonStyle := style.AnonymousBlockStyle(obj.Style)
	var run []ObjectID
	flush := func() {
		if len(run) > 0 && runGeneratesContent(a, run) {
			spliceAnonymousBlock(a, id, run, anonStyle)
		}
		run = nil
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Node != nil && k.Node.Text() {
			run = append(run, kid)
			continue
		}
		flush()
	}
	flush()
}

func runGeneratesContent(a *Arena, kids []ObjectID) bool {
	for _, kid := range kids {
		k := a.Get(kid)
		if k.Node == nil {
			continue
		}
		if !k.Node.Text() {
			if k.Style != nil && isOutOfFlow(k.Style) {
				continue
			}
			return true
		}
		if strings.TrimSpace(k.Node.DataContent) != "" {
			return true
		}
	}
	return false
}

func isWhitespaceText(k *Object) bool {
	return k.Node != nil && !k.Node.Element() && strings.TrimSpace(k.Node.DataContent) == ""
}

func isFlowBlock(obj *Object) bool {
	if obj.Node != nil && !obj.Node.Element() {
		return false
	}
	if !isBlock(obj) {
		return false
	}
	if isFlowFloat(obj.Style) {
		return true
	}
	return obj.Style.Display != style.DisplayInlineBlock
}

func spliceAnonymousBlock(a *Arena, parent ObjectID, kids []ObjectID, s *style.ComputedStyle) {
	first, last := kids[0], kids[len(kids)-1]
	prev, next := a.Get(first).PrevSibling, a.Get(last).NextSibling

	id, _ := a.Alloc()
	*a.Get(id) = Object{Style: s, Parent: parent, FirstKid: first, LastKid: last, flags: flagAnonymous}
	if a.anonSeq < ^uint16(0) {
		a.setKey(id, NewBoxKey(a.anonNodeID(parent), BoxKindAnonymous, a.anonSeq))
		a.anonSeq++
	}
	for _, kid := range kids {
		a.Get(kid).Parent = id
	}
	a.Get(first).PrevSibling = 0
	a.Get(last).NextSibling = 0
	for i := 0; i+1 < len(kids); i++ {
		a.Get(kids[i]).NextSibling = kids[i+1]
		a.Get(kids[i+1]).PrevSibling = kids[i]
	}

	anon := a.Get(id)
	anon.PrevSibling = prev
	anon.NextSibling = next
	p := a.Get(parent)
	if prev != 0 {
		a.Get(prev).NextSibling = id
	} else {
		p.FirstKid = id
	}
	if next != 0 {
		a.Get(next).PrevSibling = id
	} else {
		p.LastKid = id
	}
}

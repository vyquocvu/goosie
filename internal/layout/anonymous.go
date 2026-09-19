package layout

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/style"
)

// wrapInlineRuns gives each maximal run of inline-level children a block box of
// its own so block flow can stack it against its block siblings.
//
// CSS wraps inline content in an anonymous block box whenever it shares a
// container with block-level children. Without that box the content has no
// vertical slot at all: a `<br>` between two `<div>`s contributes no height, and
// text beside a heading draws on top of it. A container whose children are all
// inline-level keeps its content directly, which is what CSS does and what the
// inline pass already assumes.
func wrapInlineRuns(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.flags&flagAnonymous != 0 {
		// Already the wrapper of a run: its children are that run.
		return
	}
	// A run only needs a box of its own once a block-level sibling is there to
	// stack against. An inline-block is inline-level, so it does not qualify.
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
		// Collapsible whitespace between two blocks generates no box at all, so
		// wrapping it would only get in the way of margin collapsing.
		if len(run) > 0 && runGeneratesContent(a, run) {
			spliceAnonymousBlock(a, id, run, anonStyle)
		}
		run = nil
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Style == nil || k.Style.Display == style.DisplayNone || isFlowBlock(k) {
			// A hidden box keeps its own slot in the parent, so it ends the run.
			flush()
			continue
		}
		run = append(run, kid)
	}
	flush()
}

// wrapTextRuns gives each contiguous run of text inside a flex or grid
// container a box of its own so it becomes an item.
//
// An element child of such a container is already an item, but text is not: the
// spec wraps each run of it in an anonymous flex or grid item. Wrapping only the
// text, rather than whole inline runs, is what keeps `Label <span>x</span>` two
// items side by side instead of one.
func wrapTextRuns(a *Arena, id ObjectID) {
	obj := a.Get(id)
	if obj.flags&flagAnonymous != 0 {
		return
	}
	anonStyle := style.AnonymousBlockStyle(obj.Style)
	var run []ObjectID
	flush := func() {
		// Whitespace between two items generates no box.
		if len(run) > 0 && runGeneratesContent(a, run) {
			spliceAnonymousBlock(a, id, run, anonStyle)
		}
		run = nil
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		k := a.Get(kid)
		if k.Node != nil && k.Node.Type == 2 {
			run = append(run, kid)
			continue
		}
		flush()
	}
	flush()
}

// runGeneratesContent reports whether a run of inline-level children has
// anything to draw. Only whitespace between two blocks does not.
func runGeneratesContent(a *Arena, kids []ObjectID) bool {
	for _, kid := range kids {
		k := a.Get(kid)
		if k.Node == nil {
			continue
		}
		if k.Node.Type != 2 {
			return true
		}
		if strings.TrimSpace(k.Node.DataContent) != "" {
			return true
		}
	}
	return false
}

// isWhitespaceText reports the one kind of child that generates no box at all.
// The text between block siblings stays in the arena as a text object, but it
// must not be mistaken for the first or last child a margin can collapse with.
func isWhitespaceText(k *Object) bool {
	return k.Node != nil && k.Node.Type != 1 && strings.TrimSpace(k.Node.DataContent) == ""
}

// isFlowBlock reports whether a box is block-level for the purposes of block
// flow. Text nodes inherit their parent's display, so they are inline content no
// matter what the flag says, and an inline-block takes part in a line rather
// than a stack.
func isFlowBlock(obj *Object) bool {
	if obj.Node != nil && obj.Node.Type != 1 {
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

// spliceAnonymousBlock wraps kids, which must be consecutive siblings, in a new
// block box at their position in the parent's child list.
func spliceAnonymousBlock(a *Arena, parent ObjectID, kids []ObjectID, s *style.ComputedStyle) {
	first, last := kids[0], kids[len(kids)-1]
	prev, next := a.Get(first).PrevSibling, a.Get(last).NextSibling

	id, _ := a.Alloc()
	*a.Get(id) = Object{Style: s, Parent: parent, FirstKid: first, LastKid: last, flags: flagAnonymous}
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

package engine

import (
	"github.com/vyquocvu/goosie/internal/frame"
)

type Reason uint8

const (
	Scroll Reason = iota

	Hover

	StyleChange

	ImageLoad

	DOMMutation

	Resize

	StylesheetChange

	AnimationTick
)

type Invalidation struct {
	reasons            Reason
	rects              []frame.RectF
	subtrees           []uint32
	fullDoc            bool
	domMutationPending bool
}

type Plan struct {
	FullDoc bool

	Rects []frame.RectF

	Subtree []uint32

	StyleObjects int

	LayoutObjects int

	Viewport frame.Viewport

	TilesInvalidated int

	Reasons Reason
}

func (inv *Invalidation) Add(reason Reason, rect frame.RectF, objectID uint32) {
	inv.reasons |= reason
	if reason == DOMMutation {
		inv.domMutationPending = true
	}

	if reason == Resize || reason == StylesheetChange {
		inv.fullDoc = true
		return
	}

	if reason == Scroll {
		return
	}

	if rect.X1 > rect.X0 && rect.Y1 > rect.Y0 {
		found := false
		for _, r := range inv.rects {
			if r == rect {
				found = true
				break
			}
		}
		if !found {
			inv.rects = append(inv.rects, rect)
		}
	}

	if reason != ImageLoad && objectID != 0 {
		found := false
		for _, id := range inv.subtrees {
			if id == objectID {
				found = true
				break
			}
		}
		if !found {
			inv.subtrees = append(inv.subtrees, objectID)
		}
	}
}

func (inv *Invalidation) Resolve(viewport frame.Viewport, grid *frame.Grid) Plan {
	p := Plan{
		FullDoc:  inv.fullDoc,
		Viewport: viewport,
		Reasons:  inv.reasons,
	}

	if inv.fullDoc {
		return p
	}

	if inv.reasons == Scroll {
		return p
	}

	if len(inv.rects) > 0 {
		p.Rects = append([]frame.RectF(nil), inv.rects...)
	}
	if len(inv.subtrees) > 0 {
		p.Subtree = append([]uint32(nil), inv.subtrees...)
		p.LayoutObjects = len(p.Subtree)
	}

	if inv.reasons&(Hover|StyleChange) != 0 {
		p.StyleObjects = len(inv.subtrees)
	}

	if grid != nil && len(inv.rects) > 0 {
		for _, r := range inv.rects {
			p.TilesInvalidated += grid.Invalidate(r.ToDevice(1))
		}
	}

	return p
}

func (inv *Invalidation) Reset() {
	inv.reasons = 0
	inv.rects = inv.rects[:0]
	inv.subtrees = inv.subtrees[:0]
	inv.fullDoc = false
	inv.domMutationPending = false
}

func (inv *Invalidation) Empty() bool {
	return inv == nil || inv.reasons == 0
}

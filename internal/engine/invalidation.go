package engine

import (
	"github.com/vyquocvu/goosie/internal/frame"
)

// Reason identifies why content changed. Each reason maps to a different
// invalidation strategy: scroll is viewport-only, mutations require subtree
// re-layout, style changes require re-style, etc.
type Reason uint8

const (
	// Scroll is a viewport offset change. It carries no rects and no subtree,
	// which is how invariant 1 holds: a scroll resolution produces a plan
	// with LayoutObjects == 0 and StyleObjects == 0.
	Scroll Reason = iota

	// Hover is a pointer state change on a specific element.
	Hover

	// StyleChange is an inline style mutation on a specific element.
	StyleChange

	// ImageLoad is an image resource becoming available.
	ImageLoad

	// DOMMutation is a structural change (insert, remove, move).
	DOMMutation

	// Resize is a viewport size change, requiring full re-layout.
	Resize

	// StylesheetChange is a <style> or <link> mutation, requiring full re-style.
	StylesheetChange
)

// Invalidation accumulates change reasons within a single batch. The engine
// collects invalidations between frames, then resolves them to a minimal plan.
type Invalidation struct {
	reasons  Reason
	rects    []frame.RectF
	subtrees []uint32
	fullDoc  bool
}

// Plan is the resolved output of an invalidation batch. It tells the engine
// exactly what work is needed: full document re-style/layout, or specific
// rects to repaint, or specific subtrees to re-layout.
type Plan struct {
	// FullDoc is true when the entire document must be re-styled and re-laid-out.
	FullDoc bool

	// Rects are damaged regions that need repaint but not re-layout.
	Rects []frame.RectF

	// Subtree are object IDs that need re-layout (and repaint).
	Subtree []uint32

	// StyleObjects is the count of objects that need re-styling.
	StyleObjects int

	// LayoutObjects is the count of objects that need re-layout.
	LayoutObjects int

	// Viewport is the current viewport state. Scroll invalidations update this
	// without scheduling any content work.
	Viewport frame.Viewport

	// TilesInvalidated is the count of tiles marked stale by this batch.
	TilesInvalidated int

	// Reasons is the bitmask of all reasons in this batch.
	Reasons Reason
}

// Add records one invalidation event. reason identifies the cause, rect is the
// damaged region in layout pixels (zero for full-doc reasons), and objectID is
// the affected layout object (0 for non-subtree reasons).
func (inv *Invalidation) Add(reason Reason, rect frame.RectF, objectID uint32) {
	inv.reasons |= reason

	// Full-document reasons short-circuit accumulation.
	if reason == Resize || reason == StylesheetChange {
		inv.fullDoc = true
		return
	}

	// Scroll carries viewport only - no rects, no subtrees. This is how
	// invariant 1 holds: scroll produces a plan with zero content work.
	if reason == Scroll {
		return
	}

	// Local damage: accumulate rect (deduplicated).
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

	// ImageLoad is paint-only: the image's rect needs repaint but no re-layout.
	// All other local reasons (Hover, StyleChange, DOMMutation) require re-layout.
	if reason != ImageLoad && objectID != 0 {
		// Deduplicate: only add if not already present.
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

// Resolve produces a minimal plan from the accumulated batch. viewport is the
// current viewport state; grid is the tile grid (may be nil for testing).
func (inv *Invalidation) Resolve(viewport frame.Viewport, grid *frame.Grid) Plan {
	p := Plan{
		FullDoc:  inv.fullDoc,
		Viewport: viewport,
		Reasons:  inv.reasons,
	}

	if inv.fullDoc {
		return p
	}

	// Scroll-only batch: no rects, no subtrees, just viewport update.
	if inv.reasons == Scroll {
		return p
	}

	// Local damage: copy rects and subtrees.
	if len(inv.rects) > 0 {
		p.Rects = append([]frame.RectF(nil), inv.rects...)
	}
	if len(inv.subtrees) > 0 {
		p.Subtree = append([]uint32(nil), inv.subtrees...)
		p.LayoutObjects = len(p.Subtree)
	}

	// Count style objects: Hover and StyleChange both require re-style.
	if inv.reasons&(Hover|StyleChange) != 0 {
		p.StyleObjects = len(inv.subtrees)
	}

	// Invalidate tiles intersecting damage rects. Convert layout rects to
	// device rects using scale=1 for now (the engine will pass the real scale
	// once the rendering pipeline is wired up).
	if grid != nil && len(inv.rects) > 0 {
		for _, r := range inv.rects {
			p.TilesInvalidated += grid.Invalidate(r.ToDevice(1))
		}
	}

	return p
}

// Reset clears the batch for the next frame.
func (inv *Invalidation) Reset() {
	inv.reasons = 0
	inv.rects = inv.rects[:0]
	inv.subtrees = inv.subtrees[:0]
	inv.fullDoc = false
}

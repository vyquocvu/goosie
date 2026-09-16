package paint

import (
	"slices"

	"github.com/vyquocvu/goosie/internal/frame"
)

// List is a grow-only slab of display commands owned by one producer. It exists
// because the frame path has two very different lifetimes: building a list, where
// appending and occasional sorting are fine, and reading one a thousand times per
// scroll, where neither an append nor an allocation may happen. Splitting those
// into List (mutable) and LayerDL (frozen) makes the split enforceable, which is
// the only way "workers read it without a lock" is a fact rather than a hope.
//
// A List is not safe for concurrent use, and is not meant to be: the engine
// thread owns it.
type List struct {
	cmds   []DisplayCmd
	bounds frame.Rect
}

// NewList returns a builder with room for n commands. The capacity is a hint
// rather than a limit; passing the box count from layout is what keeps a
// first paint from growing the slab repeatedly.
func NewList(n int) *List {
	if n < 0 {
		n = 0
	}
	return &List{cmds: make([]DisplayCmd, 0, n)}
}

// Reset truncates the builder for the next content version while keeping the
// backing array. This is the reason a rebuild after a DOM mutation costs one
// pass rather than a fresh allocation: the slab survives, only its length does
// not.
func (l *List) Reset() {
	// Zero before reuse so a stale Glyphs slice or image reference cannot be
	// retained by the backing array and keep decoded pixels alive.
	for i := range l.cmds {
		l.cmds[i] = DisplayCmd{}
	}
	l.cmds = l.cmds[:0]
	l.bounds = frame.Rect{}
}

// Append adds a command and folds it into the running bounds, so Bounds is O(1)
// and a frame never recomputes the union.
func (l *List) Append(c DisplayCmd) {
	l.cmds = append(l.cmds, c)
	if b, ok := c.Bounds(); ok {
		if len(l.cmds) == 1 || l.bounds.Empty() {
			l.bounds = b
		} else {
			l.bounds = l.bounds.Union(b)
		}
	}
}

// Len returns the number of commands.
func (l *List) Len() int { return len(l.cmds) }

// At returns the i'th command in paint order.
func (l *List) At(i int) DisplayCmd { return l.cmds[i] }

// All returns the commands in paint order. It is the same slice the builder
// owns, so callers must not mutate it; the frozen path is Publish on the
// LayerDL returned by Build.
func (l *List) All() []DisplayCmd { return l.cmds }

// Bounds returns the union of every command's bounds, which is the layer's
// content extent. Empty for a list with nothing to paint.
func (l *List) Bounds() frame.Rect { return l.bounds }

// SortStable orders the slab by Z, preserving append order within equal Z.
//
// Sorting is exposed as an explicit call rather than folded into Append because
// append order is paint order for normal flow, and a producer that resolves
// z-index into Z before calling this gets correct overlap for free. It runs once
// per content version: a sort in the frame path would be the regression this
// whole design exists to remove.
func (l *List) SortStable() {
	slices.SortStableFunc(l.cmds, func(a, b DisplayCmd) int {
		switch {
		case a.Z < b.Z:
			return -1
		case a.Z > b.Z:
			return 1
		}
		return 0
	})
}

// Build hands the slab to a LayerDL of the given content version, still
// unfrozen. The builder is left empty and must not be reused for this version:
// transferring ownership rather than copying or sharing the slice is what makes
// the published list safe for concurrent reads.
func (l *List) Build(version uint64) *LayerDL {
	dl := &LayerDL{cmds: l.cmds, version: version, bounds: l.bounds}
	l.cmds = nil
	l.bounds = frame.Rect{}
	return dl
}

// LayerDL is one layer's display list after publication: immutable, self-
// describing, and safe for any number of raster workers to read at once.
//
// It satisfies frame.Content, which is how the frame package tracks layer
// versions without importing paint and creating a cycle.
type LayerDL struct {
	cmds      []DisplayCmd
	version   uint64
	bounds    frame.Rect
	published bool
}

// Publish freezes the list and returns it for chaining. Calling it twice panics
// on purpose: a second mutation point means some producer still holds the
// builder, and the resulting torn tile would be diagnosed as a rasterizer bug
// three layers away from the cause.
func (d *LayerDL) Publish() *LayerDL {
	if d.published {
		panic("paint: LayerDL published twice; a display list is frozen after publication")
	}
	d.published = true
	return d
}

// Frozen reports whether the list has been published, and therefore whether any
// read of it is race-free.
func (d *LayerDL) Frozen() bool { return d.published }

// Version returns the content version the list describes.
func (d *LayerDL) Version() uint64 { return d.version }

// Extent returns the content-space box the list paints. It is frame.Content's
// second method and the box the tile grid is built over.
func (d *LayerDL) Extent() frame.Rect { return d.bounds }

// Bounds is an alias for Extent, kept because callers that already speak of
// display lists say bounds.
func (d *LayerDL) Bounds() frame.Rect { return d.bounds }

// Len returns the command count.
func (d *LayerDL) Len() int { return len(d.cmds) }

// At returns the i'th command. Reading a command is the concurrent access the
// freeze exists to permit.
func (d *LayerDL) At(i int) DisplayCmd { return d.cmds[i] }

// All returns every command in paint order. The slice is shared and read-only.
func (d *LayerDL) All() []DisplayCmd { return d.cmds }

// Intersecting returns the tightest contiguous span of the paint order that
// contains every command touching r.
//
// It is a span, not a filtered copy, and that is deliberate: a filtered result
// would need either an allocation per tile or a scratch buffer per worker, and
// the frame path may do neither. The caller still clips each command it draws,
// so the only cost of the over-approximation is a bounds test per command in the
// span. Returns nil when nothing intersects, which is the common case for tiles
// over whitespace and is why the empty result is worth distinguishing.
func (d *LayerDL) Intersecting(r frame.Rect) []DisplayCmd {
	lo, hi := -1, -1
	for i := range d.cmds {
		if !d.cmds[i].Intersects(r) {
			continue
		}
		if lo < 0 {
			lo = i
		}
		hi = i
	}
	if lo < 0 {
		return nil
	}
	return d.cmds[lo : hi+1 : hi+1]
}

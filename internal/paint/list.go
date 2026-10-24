package paint

import (
	"image"
	"reflect"
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
	// Freeze each command's bounds once: Intersecting runs per tile per frame,
	// and a text command's bounds walk its glyphs, so recomputing them there
	// costs O(tiles x commands x glyphs) per frame. The sidecar pays the same
	// walk exactly once per content version; values are identical to Bounds().
	if n := len(l.cmds); n > 0 {
		cb := make([]cmdBounds, n)
		for i := range l.cmds {
			r, ok := l.cmds[i].Bounds()
			cb[i] = cmdBounds{r: r, ok: ok}
		}
		dl.cb = cb
	}
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
	// cb caches Bounds() per command, filled by Build. Intersecting reads it
	// instead of re-walking glyphs per tile; nil (a LayerDL built outside
	// List.Build, only in tests) falls back to live Bounds calls.
	cb []cmdBounds
}

// cmdBounds is one frozen command bounds entry.
type cmdBounds struct {
	r  frame.Rect
	ok bool
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

// ContentDamage includes old and new ink, so reflow cannot leave displaced pixels cached.
func ContentDamage(before, after frame.Content) frame.Rect {
	old, oldOK := before.(*LayerDL)
	next, nextOK := after.(*LayerDL)
	if !oldOK || !nextOK || old.Version() != next.Version() {
		var dirty frame.Rect
		if before != nil {
			dirty = before.Extent()
		}
		if after != nil {
			dirty = dirty.Union(after.Extent())
		}
		return dirty
	}
	var dirty frame.Rect
	add := func(cmd DisplayCmd) {
		if bounds, ok := cmd.Bounds(); ok {
			dirty = dirty.Union(bounds)
		}
	}
	if len(old.cmds) == len(next.cmds) {
		for i, cmd := range old.cmds {
			if !sameCommand(cmd, next.cmds[i]) {
				add(cmd)
				add(next.cmds[i])
			}
		}
		return dirty
	}
	start, oldEnd, newEnd := 0, len(old.cmds), len(next.cmds)
	for start < oldEnd && start < newEnd && sameCommand(old.cmds[start], next.cmds[start]) {
		start++
	}
	for oldEnd > start && newEnd > start && sameCommand(old.cmds[oldEnd-1], next.cmds[newEnd-1]) {
		oldEnd--
		newEnd--
	}
	for _, cmd := range old.cmds[start:oldEnd] {
		add(cmd)
	}
	for _, cmd := range next.cmds[start:newEnd] {
		add(cmd)
	}
	return dirty
}

func sameCommand(a, b DisplayCmd) bool {
	return a.Kind == b.Kind && a.Rect == b.Rect && a.Color == b.Color &&
		a.Border == b.Border && a.Radius == b.Radius && a.Opacity == b.Opacity &&
		a.Clip == b.Clip && a.Z == b.Z && a.Filter == b.Filter && a.BackdropFilter == b.BackdropFilter &&
		a.Text.Color == b.Text.Color && slices.Equal(a.Text.Glyphs, b.Text.Glyphs) &&
		a.Image.SrcBox == b.Image.SrcBox && sameImage(a.Image.Src, b.Image.Src) && sameImage(a.Mask, b.Mask) &&
		a.Gradient.Angle == b.Gradient.Angle && slices.Equal(a.Gradient.Stops, b.Gradient.Stops) &&
		a.RadialGradient.Shape == b.RadialGradient.Shape && a.RadialGradient.Cx == b.RadialGradient.Cx &&
		a.RadialGradient.Cy == b.RadialGradient.Cy && slices.Equal(a.RadialGradient.Stops, b.RadialGradient.Stops)
}

// Images are immutable; compare identity without walking decoded pixels or assuming a comparable implementation.
func sameImage(a, b image.Image) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	typ := reflect.TypeOf(a)
	return typ == reflect.TypeOf(b) && typ.Comparable() && a == b
}

// Intersecting returns the tightest contiguous span of the paint order that
// contains every command touching r.
//
// It is a span, not a filtered copy, and that is deliberate: a filtered result
// would need either an allocation per tile or a scratch buffer per worker, and
// the frame path may do neither. The caller still clips each command it draws,
// so the only cost of the over-approximation is a bounds test per command in the
// span. Returns nil when nothing intersects, which is the common case for tiles
// over whitespace and is why the empty result is worth distinguishing.
// CachedIntersects reports whether command i touches r using the frozen bounds
// sidecar instead of re-walking glyphs. It answers exactly what At(i).Intersects
// would, so the rasterizer's per-tile filter stays identical while dropping
// from O(glyphs) to O(1) per command.
func (d *LayerDL) CachedIntersects(i int, r frame.Rect) bool {
	if len(d.cb) == len(d.cmds) && i >= 0 && i < len(d.cmds) {
		return d.cb[i].ok && d.cb[i].r.Intersects(r)
	}
	return d.cmds[i].Intersects(r)
}

func (d *LayerDL) Intersecting(r frame.Rect) []DisplayCmd {
	lo, hi := -1, -1
	if len(d.cb) == len(d.cmds) {
		for i := range d.cmds {
			if !d.cb[i].ok || !d.cb[i].r.Intersects(r) {
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	} else {
		for i := range d.cmds {
			if !d.cmds[i].Intersects(r) {
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return nil
	}
	return d.cmds[lo : hi+1 : hi+1]
}

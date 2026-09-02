package renderer

import (
	"image/color"
	"math"
	"slices"
)

// Area returns the area of the rectangle. Returns 0 for empty or negative-size rects.
func (r RectF) Area() float32 {
	if r.W <= 0 || r.H <= 0 {
		return 0
	}
	return r.W * r.H
}

// IsEmpty reports whether the rectangle has zero or negative area.
func (r RectF) IsEmpty() bool {
	return r.W <= 0 || r.H <= 0
}

// Equal reports whether r and other are exactly equal.
func (r RectF) Equal(other RectF) bool {
	return r.X == other.X && r.Y == other.Y && r.W == other.W && r.H == other.H
}

// NearlyEqual reports whether r and other are equal within the given tolerance.
func (r RectF) NearlyEqual(other RectF, eps float32) bool {
	return float32Abs(r.X-other.X) <= eps &&
		float32Abs(r.Y-other.Y) <= eps &&
		float32Abs(r.W-other.W) <= eps &&
		float32Abs(r.H-other.H) <= eps
}

// RectUnion returns the smallest rectangle containing both a and b.
func RectUnion(a, b RectF) RectF {
	if a.IsEmpty() {
		return b
	}
	if b.IsEmpty() {
		return a
	}
	x0 := min(a.X, b.X)
	y0 := min(a.Y, b.Y)
	x1 := max(a.X+a.W, b.X+b.W)
	y1 := max(a.Y+a.H, b.Y+b.H)
	return RectF{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// RectIntersection returns the intersection of a and b, or zero-size rect if no overlap.
func RectIntersection(a, b RectF) RectF {
	x0 := max(a.X, b.X)
	y0 := max(a.Y, b.Y)
	x1 := min(a.X+a.W, b.X+b.W)
	y1 := min(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return RectF{}
	}
	return RectF{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func float32Abs(x float32) float32 {
	return float32(math.Abs(float64(x)))
}

// --- DirtyRegion ---

// DirtyRegion tracks a bounded collection of dirty rectangles that need
// repainting. When the number of rects exceeds maxRects, overlapping rects
// are merged to keep the count bounded, ensuring O(n·k) merge complexity
// rather than O(n²).
type DirtyRegion struct {
	rects    []RectF
	maxRects int
}

// NewDirtyRegion creates a dirty region with the given maximum rect count.
// When the number of added rects exceeds this limit, overlapping rects are
// automatically merged. A reasonable default is 64.
func NewDirtyRegion(maxRects int) *DirtyRegion {
	if maxRects <= 0 {
		maxRects = 64
	}
	return &DirtyRegion{
		rects:    make([]RectF, 0, maxRects),
		maxRects: maxRects,
	}
}

// Add adds a rectangle to the dirty region. Zero-size and negative-size
// rectangles are ignored. If adding this rect would exceed the maximum
// count, overlapping rects are merged first.
func (dr *DirtyRegion) Add(r RectF) {
	if r.IsEmpty() {
		return
	}
	dr.rects = append(dr.rects, r)
	if len(dr.rects) > dr.maxRects {
		dr.MergeOverlapping()
	}
}

// Clear removes all dirty rectangles.
func (dr *DirtyRegion) Clear() {
	dr.rects = dr.rects[:0]
}

// Len returns the number of dirty rectangles.
func (dr *DirtyRegion) Len() int {
	return len(dr.rects)
}

// Rects returns a copy of the dirty rectangles.
func (dr *DirtyRegion) Rects() []RectF {
	out := make([]RectF, len(dr.rects))
	copy(out, dr.rects)
	return out
}

// TotalArea returns the total area covered by the dirty region, computed
// as the area of the bounding rect union. For non-overlapping rects this
// is the exact sum; for overlapping rects it computes the union area.
func (dr *DirtyRegion) TotalArea() float32 {
	if len(dr.rects) == 0 {
		return 0
	}
	// Compute union area by sweeping
	// For small rect counts, use pairwise union
	if len(dr.rects) == 1 {
		return dr.rects[0].Area()
	}
	// Use inclusion-exclusion for small counts, bounding rect for large
	if len(dr.rects) <= 16 {
		return dr.exactArea()
	}
	return dr.BoundingRect().Area()
}

// exactArea computes the exact area using coordinate compression.
func (dr *DirtyRegion) exactArea() float32 {
	// Collect all unique x and y coordinates
	n := len(dr.rects)
	xs := make([]float32, 0, 2*n)
	ys := make([]float32, 0, 2*n)
	for _, r := range dr.rects {
		xs = append(xs, r.X, r.X+r.W)
		ys = append(ys, r.Y, r.Y+r.H)
	}
	xs = uniqueSorted(xs)
	ys = uniqueSorted(ys)

	var total float32
	for i := 0; i < len(xs)-1; i++ {
		for j := 0; j < len(ys)-1; j++ {
			// Check if this cell is covered by any rect
			cx := (xs[i] + xs[i+1]) * 0.5
			cy := (ys[j] + ys[j+1]) * 0.5
			for _, r := range dr.rects {
				if r.Contains(cx, cy) {
					total += (xs[i+1] - xs[i]) * (ys[j+1] - ys[j])
					break
				}
			}
		}
	}
	return total
}

// uniqueSorted returns a sorted slice with duplicates removed.
func uniqueSorted(vals []float32) []float32 {
	if len(vals) == 0 {
		return vals
	}
	slices.Sort(vals)
	return slices.Compact(vals)
}

// Expand expands all dirty rectangles by the given amount on each side.
func (dr *DirtyRegion) Expand(amount float32) {
	for i := range dr.rects {
		dr.rects[i].X -= amount
		dr.rects[i].Y -= amount
		dr.rects[i].W += 2 * amount
		dr.rects[i].H += 2 * amount
	}
}

// ExpandClipped expands all dirty rectangles by the given amount, then
// clips each to the viewport bounds.
func (dr *DirtyRegion) ExpandClipped(amount float32, viewport RectF) {
	for i := range dr.rects {
		r := dr.rects[i]
		r.X -= amount
		r.Y -= amount
		r.W += 2 * amount
		r.H += 2 * amount
		dr.rects[i] = RectIntersection(r, viewport)
	}
	// Remove any rects that became empty after clipping
	n := 0
	for _, r := range dr.rects {
		if !r.IsEmpty() {
			dr.rects[n] = r
			n++
		}
	}
	dr.rects = dr.rects[:n]
}

// Merge adds all rects from other into this region.
func (dr *DirtyRegion) Merge(other *DirtyRegion) {
	if other == nil {
		return
	}
	for _, r := range other.rects {
		dr.Add(r)
	}
}

// MergeOverlapping merges overlapping or touching rectangles in-place.
// Uses a greedy O(n²) pairwise merge that is bounded by maxRects.
func (dr *DirtyRegion) MergeOverlapping() {
	if len(dr.rects) <= 1 {
		return
	}
	changed := true
	for changed {
		changed = false
		for i := 0; i < len(dr.rects); i++ {
			for j := i + 1; j < len(dr.rects); j++ {
				if dr.rects[i].Intersects(dr.rects[j]) || touching(dr.rects[i], dr.rects[j]) {
					dr.rects[i] = RectUnion(dr.rects[i], dr.rects[j])
					// Remove j by swapping with last
					dr.rects[j] = dr.rects[len(dr.rects)-1]
					dr.rects = dr.rects[:len(dr.rects)-1]
					changed = true
					j-- // recheck position j
				}
			}
		}
	}
}

// touching reports whether two rectangles share an edge (touching but not overlapping).
func touching(a, b RectF) bool {
	// Horizontally touching
	if a.X+a.W == b.X || b.X+b.W == a.X {
		// Check vertical overlap
		return a.Y < b.Y+b.H && b.Y < a.Y+a.H
	}
	// Vertically touching
	if a.Y+a.H == b.Y || b.Y+b.H == a.Y {
		// Check horizontal overlap
		return a.X < b.X+b.W && b.X < a.X+a.W
	}
	return false
}

// BoundingRect returns the smallest rectangle containing all dirty rects.
// Returns a zero rect if the region is empty.
func (dr *DirtyRegion) BoundingRect() RectF {
	if len(dr.rects) == 0 {
		return RectF{}
	}
	result := dr.rects[0]
	for i := 1; i < len(dr.rects); i++ {
		result = RectUnion(result, dr.rects[i])
	}
	return result
}

// Contains reports whether point (x, y) lies within any dirty rectangle.
func (dr *DirtyRegion) Contains(x, y float32) bool {
	for _, r := range dr.rects {
		if r.Contains(x, y) {
			return true
		}
	}
	return false
}

// Intersects reports whether any dirty rectangle overlaps with r.
func (dr *DirtyRegion) Intersects(r RectF) bool {
	for _, dr2 := range dr.rects {
		if dr2.Intersects(r) {
			return true
		}
	}
	return false
}

// --- EffectParams and ExpandForEffects ---

// EffectParams describes visual effects that extend the painted area beyond
// an element's content bounds. These are used to expand dirty regions so
// that shadow, border, and antialiasing artifacts are fully repainted.
type EffectParams struct {
	ShadowBlur    float32
	ShadowOffsetX float32
	ShadowOffsetY float32
	BorderWidth   float32
	AAMargin      float32 // antialiasing margin in pixels
}

// MaxExpansion returns the maximum expansion amount across all effect params.
func (ep EffectParams) MaxExpansion() float32 {
	m := ep.BorderWidth + ep.AAMargin
	shadowExpand := ep.ShadowBlur + max(float32Abs(ep.ShadowOffsetX), float32Abs(ep.ShadowOffsetY))
	return max(m, shadowExpand)
}

// ExpandForEffects expands a rectangle to account for visual effects
// (shadows, borders, antialiasing).
func ExpandForEffects(r RectF, params EffectParams) RectF {
	if params.ShadowBlur == 0 && params.BorderWidth == 0 && params.AAMargin == 0 {
		return r
	}

	symmetric := params.BorderWidth + params.AAMargin
	leftExpand := symmetric + params.ShadowBlur + max(0, -params.ShadowOffsetX)
	rightExpand := symmetric + params.ShadowBlur + max(0, params.ShadowOffsetX)
	topExpand := symmetric + params.ShadowBlur + max(0, -params.ShadowOffsetY)
	bottomExpand := symmetric + params.ShadowBlur + max(0, params.ShadowOffsetY)

	return RectF{
		X: r.X - leftExpand,
		Y: r.Y - topExpand,
		W: r.W + leftExpand + rightExpand,
		H: r.H + topExpand + bottomExpand,
	}
}

// --- DirtyRegionTracker ---

// layoutBounds stores the last-known visual bounds for a layout object.
type layoutBounds struct {
	bounds RectF
}

// DirtyRegionTracker tracks per-LayoutID visual bounds across frames and
// produces a DirtyRegion when objects are invalidated. When an object moves,
// both the old and new bounds are added to the dirty region.
type DirtyRegionTracker struct {
	bounds   map[LayoutID]layoutBounds
	dirty    *DirtyRegion
	maxRects int
}

// NewDirtyRegionTracker creates a tracker with the given dirty region capacity.
func NewDirtyRegionTracker(maxRects int) *DirtyRegionTracker {
	if maxRects <= 0 {
		maxRects = 64
	}
	return &DirtyRegionTracker{
		bounds:   make(map[LayoutID]layoutBounds),
		dirty:    NewDirtyRegion(maxRects),
		maxRects: maxRects,
	}
}

// UpdateBounds records the current visual bounds for a layout object.
// This does not mark the object as dirty; use InvalidateLayoutID for that.
func (tr *DirtyRegionTracker) UpdateBounds(id LayoutID, bounds RectF) {
	if id == LayoutNone {
		return
	}
	tr.bounds[id] = layoutBounds{bounds: bounds}
}

// Bounds returns the last-known visual bounds for a layout object.
func (tr *DirtyRegionTracker) Bounds(id LayoutID) (RectF, bool) {
	if id == LayoutNone {
		return RectF{}, false
	}
	lb, ok := tr.bounds[id]
	if !ok {
		return RectF{}, false
	}
	return lb.bounds, true
}

// RemoveLayout removes a layout object and invalidates its old bounds.
func (tr *DirtyRegionTracker) RemoveLayout(id LayoutID) {
	if id == LayoutNone {
		return
	}
	if lb, ok := tr.bounds[id]; ok {
		tr.dirty.Add(lb.bounds)
		delete(tr.bounds, id)
	}
}

// InvalidateLayoutID marks the stored bounds for a layout object as dirty.
// If no bounds are stored, this is a no-op.
func (tr *DirtyRegionTracker) InvalidateLayoutID(id LayoutID) {
	if id == LayoutNone {
		return
	}
	if lb, ok := tr.bounds[id]; ok {
		tr.dirty.Add(lb.bounds)
	}
}

// InvalidateRect adds an arbitrary rectangle to the dirty region.
func (tr *DirtyRegionTracker) InvalidateRect(r RectF) {
	tr.dirty.Add(r)
}

// InvalidateMove handles an object that moved from oldBounds to newBounds.
// Both the old and new visual regions are marked dirty. The stored bounds
// are updated to newBounds.
func (tr *DirtyRegionTracker) InvalidateMove(id LayoutID, oldBounds, newBounds RectF) {
	if id == LayoutNone {
		return
	}
	tr.dirty.Add(oldBounds)
	tr.dirty.Add(newBounds)
	tr.bounds[id] = layoutBounds{bounds: newBounds}
}

// Finalize returns the accumulated dirty region and resets the pending
// dirty state. The returned DirtyRegion is a new object; the tracker's
// internal state is cleared for the next frame.
func (tr *DirtyRegionTracker) Finalize() *DirtyRegion {
	result := tr.dirty
	tr.dirty = NewDirtyRegion(tr.maxRects)
	return result
}

// Reset clears all tracked bounds and pending dirty regions.
func (tr *DirtyRegionTracker) Reset() {
	clear(tr.bounds)
	tr.dirty.Clear()
}

// --- Debug visualization ---

// DebugDirtyRegionOverlay generates a DisplayCommandList with semi-transparent
// rectangles overlaying each dirty region. This is intended for developer
// tools to visualize which screen areas are being repainted.
//
// If clr is zero-valued (fully transparent), a default semi-transparent red
// is used.
func DebugDirtyRegionOverlay(dr *DirtyRegion, clr color.Color) *DisplayCommandList {
	dl := NewDisplayCommandList()
	if dr == nil {
		return dl
	}

	// Use default color if none provided
	if clr == nil {
		clr = color.RGBA{R: 255, A: 64}
	} else {
		r, g, b, a := clr.RGBA()
		if r == 0 && g == 0 && b == 0 && a == 0 {
			clr = color.RGBA{R: 255, A: 64}
		}
	}

	rects := dr.Rects()
	for _, r := range rects {
		dl.Add(DisplayCommand{
			Kind: CmdRect,
			Rect: RectCommand{
				Bounds: r,
				Color:  clr,
			},
		})
	}
	return dl
}

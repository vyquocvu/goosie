package js

import (
	"sync"

	"github.com/dop251/goja"
)

// IntersectionObserver implementation for lazy loading, infinite scroll, and analytics.
// The observer tracks target elements and fires callbacks when they enter/exit the viewport.

// IntersectionEntry represents one observed target element.
type IntersectionEntry struct {
	target     *goja.Object
	callback   goja.Value
	lastState  bool       // last known intersection state
	rootMargin [4]float32 // top, right, bottom, left
	threshold  []float32
}

// IntersectionObserver is the JS-exposed observer object.
type IntersectionObserver struct {
	r        *Runtime
	obj      *goja.Object
	mu       sync.Mutex
	entries  []*IntersectionEntry
	callback goja.Value
	root     *goja.Object // nil means viewport
	// rootMargin and threshold are stored per-entry in real spec, but we simplify
	// by storing them on the observer and applying to all entries.
	rootMargin [4]float32
	threshold  []float32
}

// intersectionState holds the current viewport state provided by the engine.
// Stored per-runtime to avoid mutable globals.
type intersectionState struct {
	mu           sync.Mutex
	viewportRect [4]float32 // x, y, width, height
	// elementRects maps element IDs to their bounding rects.
	// The engine updates this during layout.
	elementRects map[int][4]float32
	// observers tracks all active observers for CheckIntersections.
	observers []*IntersectionObserver
}

const intersectionStateKey = "__intersectionState__"

func setIntersectionState(r *Runtime, is *intersectionState) {
	_ = r.vm.Set(intersectionStateKey, is)
}

func intersectionOf(r *Runtime) *intersectionState {
	v := r.vm.Get(intersectionStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	is, ok := v.Export().(*intersectionState)
	if !ok {
		return nil
	}
	return is
}

// SetViewportRect updates the viewport rectangle for intersection calculations.
// Called by the engine when the viewport changes.
func (r *Runtime) SetViewportRect(x, y, width, height float32) {
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	is.viewportRect = [4]float32{x, y, width, height}
	is.mu.Unlock()
}

// SetElementRect updates the bounding rect for an element.
// Called by the engine after layout.
func (r *Runtime) SetElementRect(elementID int, x, y, width, height float32) {
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	is.elementRects[elementID] = [4]float32{x, y, width, height}
	is.mu.Unlock()
}

// setupIntersectionObserver installs the IntersectionObserver constructor.
func (r *Runtime) setupIntersectionObserver() {
	// Initialize per-runtime intersection state.
	is := &intersectionState{
		elementRects: make(map[int][4]float32),
	}
	setIntersectionState(r, is)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructIntersectionObserver(call)
	}

	_ = r.vm.Set("IntersectionObserver", constructor)
}

// constructIntersectionObserver implements new IntersectionObserver(callback, options).
func (r *Runtime) constructIntersectionObserver(call goja.ConstructorCall) *goja.Object {
	if len(call.Arguments) < 1 {
		return call.This
	}

	callback := call.Arguments[0]
	if _, ok := goja.AssertFunction(callback); !ok {
		return call.This
	}

	obs := &IntersectionObserver{
		r:        r,
		callback: callback,
	}

	obj := call.This
	obs.obj = obj

	// Parse options.
	if len(call.Arguments) >= 2 {
		if opts, ok := call.Arguments[1].(*goja.Object); ok {
			// Parse rootMargin.
			if v := opts.Get("rootMargin"); v != nil && !goja.IsUndefined(v) {
				r.parseRootMargin(v.String(), &obs.rootMargin)
			}
			// Parse threshold.
			if v := opts.Get("threshold"); v != nil && !goja.IsUndefined(v) {
				obs.threshold = r.parseThreshold(v)
			}
		}
	}

	// Install methods.
	_ = obj.Set("observe", func(call goja.FunctionCall) goja.Value {
		return r.intersectionObserve(call, obs)
	})

	_ = obj.Set("unobserve", func(call goja.FunctionCall) goja.Value {
		return r.intersectionUnobserve(call, obs)
	})

	_ = obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
		return r.intersectionDisconnect(call, obs)
	})

	// Register observer
	if is := intersectionOf(r); is != nil {
		is.mu.Lock()
		is.observers = append(is.observers, obs)
		is.mu.Unlock()
	}

	return obj
}

// parseRootMargin parses a CSS margin string like "10px 20px" into [top, right, bottom, left].
func (r *Runtime) parseRootMargin(s string, margin *[4]float32) {
	// Simplified parser: handles "Npx" or "N" format.
	// Real spec supports %, em, rem, etc.
	// For now, just parse pixel values.
	parts := splitSpaces(s)
	values := make([]float32, 0, 4)
	for _, p := range parts {
		v := parsePixelValue(p)
		values = append(values, v)
	}

	switch len(values) {
	case 1:
		margin[0] = values[0]
		margin[1] = values[0]
		margin[2] = values[0]
		margin[3] = values[0]
	case 2:
		margin[0] = values[0]
		margin[1] = values[1]
		margin[2] = values[0]
		margin[3] = values[1]
	case 3:
		margin[0] = values[0]
		margin[1] = values[1]
		margin[2] = values[2]
		margin[3] = values[1]
	case 4:
		margin[0] = values[0]
		margin[1] = values[1]
		margin[2] = values[2]
		margin[3] = values[3]
	}
}

// parseThreshold parses the threshold option (number or array of numbers).
func (r *Runtime) parseThreshold(v goja.Value) []float32 {
	if arr, ok := v.(*goja.Object); ok {
		// Array of thresholds.
		lengthVal := arr.Get("length")
		if lengthVal == nil || goja.IsUndefined(lengthVal) {
			return []float32{0}
		}
		length := int(lengthVal.ToInteger())
		thresholds := make([]float32, length)
		for i := 0; i < length; i++ {
			val := arr.Get(intToString(i))
			if val != nil && !goja.IsUndefined(val) {
				thresholds[i] = float32(val.ToFloat())
			}
		}
		return thresholds
	}
	// Single number.
	return []float32{float32(v.ToFloat())}
}

// intersectionObserve implements observer.observe(target).
func (r *Runtime) intersectionObserve(call goja.FunctionCall, obs *IntersectionObserver) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}

	target, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		return goja.Undefined()
	}

	entry := &IntersectionEntry{
		target:     target,
		rootMargin: obs.rootMargin,
		threshold:  obs.threshold,
	}

	obs.mu.Lock()
	obs.entries = append(obs.entries, entry)
	obs.mu.Unlock()

	return goja.Undefined()
}

// intersectionUnobserve implements observer.unobserve(target).
func (r *Runtime) intersectionUnobserve(call goja.FunctionCall, obs *IntersectionObserver) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}

	target, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		return goja.Undefined()
	}

	obs.mu.Lock()
	for i, entry := range obs.entries {
		if entry.target == target {
			obs.entries = append(obs.entries[:i], obs.entries[i+1:]...)
			break
		}
	}
	obs.mu.Unlock()

	return goja.Undefined()
}

// intersectionDisconnect implements observer.disconnect().
func (r *Runtime) intersectionDisconnect(call goja.FunctionCall, obs *IntersectionObserver) goja.Value {
	obs.mu.Lock()
	obs.entries = nil
	obs.mu.Unlock()

	return goja.Undefined()
}

// CheckIntersections checks all observed elements and fires callbacks for changes.
// Called by the engine after layout/scroll.
func (r *Runtime) CheckIntersections() {
	is := intersectionOf(r)
	if is == nil {
		return
	}

	is.mu.Lock()
	observers := make([]*IntersectionObserver, len(is.observers))
	copy(observers, is.observers)
	viewportRect := is.viewportRect
	elementRects := make(map[int][4]float32)
	for k, v := range is.elementRects {
		elementRects[k] = v
	}
	is.mu.Unlock()

	for _, obs := range observers {
		obs.mu.Lock()
		entries := make([]*IntersectionEntry, len(obs.entries))
		copy(entries, obs.entries)
		callback := obs.callback
		threshold := obs.threshold
		obs.mu.Unlock()

		if callback == nil {
			continue
		}

		var changed []*IntersectionEntry
		for _, entry := range entries {
			// Get element rect
			r.mu.Lock()
			var nodeID int
			for id := range r.nodeRegistry {
				// Find the node ID for this target
				if targetObj, ok := entry.target.Export().(*goja.Object); ok {
					if nidVal := targetObj.Get("__nid__"); nidVal != nil {
						if id == int(nidVal.ToInteger()) {
							nodeID = id
							break
						}
					}
				}
			}
			r.mu.Unlock()

			if nodeID == 0 {
				continue
			}

			elemRect, ok := elementRects[nodeID]
			if !ok {
				continue
			}

			// Calculate intersection
			_ = rectsIntersect(viewportRect, elemRect)
			ratio := intersectionRatio(viewportRect, elemRect)

			// Check threshold
			thresholdMet := false
			for _, t := range threshold {
				if ratio >= t {
					thresholdMet = true
					break
				}
			}
			if len(threshold) == 0 && ratio > 0 {
				thresholdMet = true
			}

			// Check if state changed
			if thresholdMet != entry.lastState {
				entry.lastState = thresholdMet
				changed = append(changed, entry)
			}
		}

		// Fire callback with changed entries
		if len(changed) > 0 {
			r.fireIntersectionCallback(callback, changed, viewportRect)
		}
	}
}

// rectsIntersect checks if two rects [x, y, w, h] intersect.
func rectsIntersect(a, b [4]float32) bool {
	aRight := a[0] + a[2]
	aBottom := a[1] + a[3]
	bRight := b[0] + b[2]
	bBottom := b[1] + b[3]
	return a[0] < bRight && aRight > b[0] && a[1] < bBottom && aBottom > b[1]
}

// intersectionRatio calculates the intersection ratio of b within a.
func intersectionRatio(a, b [4]float32) float32 {
	x0 := max(a[0], b[0])
	y0 := max(a[1], b[1])
	x1 := min(a[0]+a[2], b[0]+b[2])
	y1 := min(a[1]+a[3], b[1]+b[3])

	if x1 <= x0 || y1 <= y0 {
		return 0
	}

	intersectArea := (x1 - x0) * (y1 - y0)
	elemArea := b[2] * b[3]
	if elemArea <= 0 {
		return 0
	}
	return intersectArea / elemArea
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// fireIntersectionCallback calls the observer callback with intersection entries.
func (r *Runtime) fireIntersectionCallback(callback goja.Value, entries []*IntersectionEntry, viewportRect [4]float32) {
	fn, ok := goja.AssertFunction(callback)
	if !ok {
		return
	}

	// Build entries array
	arr := r.vm.NewArray()
	for i, entry := range entries {
		entryObj := r.vm.NewObject()
		_ = entryObj.Set("target", entry.target)
		_ = entryObj.Set("isIntersecting", entry.lastState)
		_ = entryObj.Set("intersectionRatio", intersectionRatio(viewportRect, [4]float32{}))
		_ = arr.Set(intToString(i), entryObj)
	}

	_, _ = fn(goja.Undefined(), arr)
}

// Helper functions.

func splitSpaces(s string) []string {
	var parts []string
	start := 0
	inSpace := true
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' {
			if !inSpace {
				parts = append(parts, s[start:i])
				inSpace = true
			}
		} else {
			if inSpace {
				start = i
				inSpace = false
			}
		}
	}
	if !inSpace {
		parts = append(parts, s[start:])
	}
	return parts
}

func parsePixelValue(s string) float32 {
	// Strip "px" suffix if present.
	if len(s) >= 2 && s[len(s)-2:] == "px" {
		s = s[:len(s)-2]
	}
	// Parse as float.
	var v float32
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' {
			continue
		}
		s = s[:i]
		break
	}
	// Simple float parse.
	negative := false
	if len(s) > 0 && s[0] == '-' {
		negative = true
		s = s[1:]
	}
	intPart := float32(0)
	fracPart := float32(0)
	inFrac := false
	fracDiv := float32(1)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			inFrac = true
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			digit := float32(s[i] - '0')
			if inFrac {
				fracDiv *= 10
				fracPart += digit / fracDiv
			} else {
				intPart = intPart*10 + digit
			}
		}
	}
	v = intPart + fracPart
	if negative {
		v = -v
	}
	return v
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

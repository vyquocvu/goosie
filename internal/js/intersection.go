package js

import (
	"sync"

	"github.com/dop251/goja"
)

// IntersectionObserver implementation for lazy loading, infinite scroll, and analytics.
// The observer tracks target elements and fires callbacks when they enter/exit the viewport.

// IntersectionEntry represents one observed target element.
type IntersectionEntry struct {
	// target is the wrapper the page passed to observe(). It is kept only so the
	// callback payload carries the same object the page is holding; reading any
	// property off it would re-enter the runtime (see targetID).
	target *goja.Object
	// targetID is the wrapper's __nid__, captured at observe() time. Every
	// geometry lookup goes through this number, because the alternative - reading
	// __nid__ back off `target` when the observer is checked - cannot be done
	// safely: Goosie's node wrappers carry enumerable parentNode/shadowRoot
	// getters, so any export or property walk over a wrapper calls back into the
	// runtime, and the check runs while the runtime lock is held. Go mutexes are
	// not reentrant, so that path deadlocked the whole document load
	// (GOOSIE-IOEXPORT-001). targetID is -1 when the observe() argument was not a
	// node wrapper, which can never produce geometry and so never fires.
	targetID   int
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

// ViewportSize reports the configured viewport dimensions shared with layout.
func (r *Runtime) ViewportSize() (width, height float32) {
	is := intersectionOf(r)
	if is == nil {
		return 0, 0
	}
	is.mu.Lock()
	width, height = is.viewportRect[2], is.viewportRect[3]
	is.mu.Unlock()
	return width, height
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
		targetID:   -1,
		rootMargin: obs.rootMargin,
		threshold:  obs.threshold,
	}

	// Record the wrapper's registry key now, while this is an ordinary script
	// call: reading a property off a node wrapper can run page getters, and the
	// check phase cannot afford that (see IntersectionEntry.targetID). Mutation
	// and Resize observers take the same reading at the same point in their
	// observe(); unlike them, this observer does not throw on a non-node
	// argument, because observe(plainObject) has always been a no-op here and
	// turning it into a TypeError would break pages that observe a stub object
	// before the element exists.
	if nidVal := target.Get("__nid__"); nidVal != nil && !goja.IsUndefined(nidVal) {
		entry.targetID = int(nidVal.ToInteger())
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
			// Resolve the observed wrapper to the node it stands for. The runtime
			// lock covers a map read and nothing else: no goja property access and
			// no Export() may happen here, because node wrappers expose enumerable
			// parentNode/shadowRoot getters that call back into the runtime, and
			// re-entering this mutex without the lock is what hung every document
			// that registered an observer (GOOSIE-IOEXPORT-001). The rect map is
			// already a private copy taken above, so it needs no lock, but its keys
			// are dom.Node IDs rather than wrapper IDs - the two allocators are
			// separate - so the node is what translates one into the other.
			elemRect, ok := r.intersectionRectOf(entry.targetID, elementRects)
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

// intersectionRectOf resolves an observed wrapper's __nid__ to the rect the
// engine recorded for the node behind it. rects is the caller's private snapshot
// of intersectionState.elementRects, keyed by dom.Node ID, so the wrapper ID has
// to be translated through nodeRegistry: the two allocators (goja wrapper keys and
// dom.Document node IDs) count separately, and a document's first wrapper ID is
// already off by one from the matching node's ID. The runtime lock guards the
// registry read only; nothing here touches the JS heap, which is the property
// that keeps CheckIntersections out of the deadlock GOOSIE-IOEXPORT-001 describes.
func (r *Runtime) intersectionRectOf(wrapperID int, rects map[int][4]float32) ([4]float32, bool) {
	// -1 is the "not a node wrapper" sentinel from intersectionObserve. Wrapper ID 0
	// is a real registration and must not be confused with it.
	if wrapperID < 0 {
		return [4]float32{}, false
	}

	r.mu.Lock()
	node := r.nodeRegistry[wrapperID]
	var nodeID int
	if node != nil {
		nodeID = int(node.ID)
	}
	r.mu.Unlock()

	if node == nil {
		return [4]float32{}, false
	}
	rect, ok := rects[nodeID]
	return rect, ok
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

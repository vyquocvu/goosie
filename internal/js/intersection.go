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
	// This would be called on all active observers. For now, we provide a
	// mechanism for the engine to trigger checks.
	// In a full implementation, we'd maintain a registry of all observers.
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

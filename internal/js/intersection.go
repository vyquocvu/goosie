package js

import (
	"sync"

	"github.com/dop251/goja"
)

type IntersectionEntry struct {
	target     *goja.Object
	targetID   int
	callback   goja.Value
	lastState  bool
	rootMargin [4]float32
	threshold  []float32
}

type IntersectionObserver struct {
	r          *Runtime
	obj        *goja.Object
	mu         sync.Mutex
	entries    []*IntersectionEntry
	callback   goja.Value
	root       *goja.Object
	rootMargin [4]float32
	threshold  []float32
}

type intersectionState struct {
	mu           sync.Mutex
	viewportRect [4]float32
	elementRects map[int][4]float32
	observers    []*IntersectionObserver
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

func (r *Runtime) SetViewportRect(x, y, width, height float32) {
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	is.viewportRect = [4]float32{x, y, width, height}
	is.mu.Unlock()
}

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

func (r *Runtime) SetElementRect(elementID int, x, y, width, height float32) {
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	is.elementRects[elementID] = [4]float32{x, y, width, height}
	is.mu.Unlock()
}

func (r *Runtime) setupIntersectionObserver() {
	is := &intersectionState{
		elementRects: make(map[int][4]float32),
	}
	setIntersectionState(r, is)

	constructor := func(call goja.ConstructorCall) *goja.Object {
		return r.constructIntersectionObserver(call)
	}

	_ = r.vm.Set("IntersectionObserver", constructor)
}

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

	if len(call.Arguments) >= 2 {
		if opts, ok := call.Arguments[1].(*goja.Object); ok {
			if v := opts.Get("rootMargin"); v != nil && !goja.IsUndefined(v) {
				r.parseRootMargin(v.String(), &obs.rootMargin)
			}
			if v := opts.Get("threshold"); v != nil && !goja.IsUndefined(v) {
				obs.threshold = r.parseThreshold(v)
			}
		}
	}

	_ = obj.Set("observe", func(call goja.FunctionCall) goja.Value {
		return r.intersectionObserve(call, obs)
	})

	_ = obj.Set("unobserve", func(call goja.FunctionCall) goja.Value {
		return r.intersectionUnobserve(call, obs)
	})

	_ = obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
		return r.intersectionDisconnect(call, obs)
	})

	if is := intersectionOf(r); is != nil {
		is.mu.Lock()
		is.observers = append(is.observers, obs)
		is.mu.Unlock()
	}

	return obj
}

func (r *Runtime) parseRootMargin(s string, margin *[4]float32) {
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

func (r *Runtime) parseThreshold(v goja.Value) []float32 {
	if arr, ok := v.(*goja.Object); ok {
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
	return []float32{float32(v.ToFloat())}
}

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

	if nidVal := target.Get("__nid__"); nidVal != nil && !goja.IsUndefined(nidVal) {
		entry.targetID = int(nidVal.ToInteger())
	}

	obs.mu.Lock()
	obs.entries = append(obs.entries, entry)
	obs.mu.Unlock()

	return goja.Undefined()
}

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

func (r *Runtime) intersectionDisconnect(call goja.FunctionCall, obs *IntersectionObserver) goja.Value {
	obs.mu.Lock()
	obs.entries = nil
	obs.mu.Unlock()

	return goja.Undefined()
}

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
			elemRect, ok := r.intersectionRectOf(entry.targetID, elementRects)
			if !ok {
				continue
			}

			_ = rectsIntersect(viewportRect, elemRect)
			ratio := intersectionRatio(viewportRect, elemRect)

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

			if thresholdMet != entry.lastState {
				entry.lastState = thresholdMet
				changed = append(changed, entry)
			}
		}

		if len(changed) > 0 {
			r.fireIntersectionCallback(callback, changed, viewportRect)
		}
	}
}

func (r *Runtime) intersectionRectOf(wrapperID int, rects map[int][4]float32) ([4]float32, bool) {
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

func rectsIntersect(a, b [4]float32) bool {
	aRight := a[0] + a[2]
	aBottom := a[1] + a[3]
	bRight := b[0] + b[2]
	bBottom := b[1] + b[3]
	return a[0] < bRight && aRight > b[0] && a[1] < bBottom && aBottom > b[1]
}

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

func (r *Runtime) fireIntersectionCallback(callback goja.Value, entries []*IntersectionEntry, viewportRect [4]float32) {
	fn, ok := goja.AssertFunction(callback)
	if !ok {
		return
	}

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
	if len(s) >= 2 && s[len(s)-2:] == "px" {
		s = s[:len(s)-2]
	}
	var v float32
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' {
			continue
		}
		s = s[:i]
		break
	}
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

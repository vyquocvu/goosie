package js

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type rootMarginValue struct {
	value   float32
	percent bool
}

type IntersectionEntry struct {
	target         *goja.Object
	targetNode     *dom.Node
	initialized    bool
	isIntersecting bool
	thresholdIndex int
	threshold      []float32
}

type IntersectionObserver struct {
	r          *Runtime
	obj        *goja.Object
	mu         sync.Mutex
	entries    []*IntersectionEntry
	callback   goja.Value
	root       *dom.Node
	rootMargin [4]rootMarginValue
	threshold  []float32
}

type intersectionState struct {
	mu           sync.Mutex
	elementRects map[int][4]float32
	observers    []*IntersectionObserver
	checkQueued  bool
}

type intersectionResult struct {
	entry              *IntersectionEntry
	ratio              float32
	isIntersecting     bool
	boundingClientRect [4]float64
	intersectionRect   [4]float64
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
	if r == nil {
		return
	}
	if width < 0 || width != width || height < 0 || height != height {
		return
	}
	r.viewportMu.Lock()
	r.viewportX, r.viewportY = x, y
	r.viewportWidth, r.viewportHeight = width, height
	r.refreshMediaQueriesLocked()
	r.viewportMu.Unlock()
	r.scheduleMediaQueryNotify()
	r.scheduleIntersectionCheck()
}

func (r *Runtime) ViewportSize() (width, height float32) {
	if r == nil {
		return 0, 0
	}
	r.viewportMu.Lock()
	width, height = r.viewportWidth, r.viewportHeight
	r.viewportMu.Unlock()
	return width, height
}

func (r *Runtime) currentViewportRect() [4]float64 {
	r.viewportMu.Lock()
	x, y := float64(r.viewportX), float64(r.viewportY)
	width, height := float64(r.viewportWidth), float64(r.viewportHeight)
	r.viewportMu.Unlock()
	return [4]float64{x, y, width, height}
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
		panic(r.vm.NewTypeError("IntersectionObserver requires a callback argument"))
	}
	if _, ok := goja.AssertFunction(call.Arguments[0]); !ok {
		panic(r.vm.NewTypeError("IntersectionObserver callback must be a function"))
	}
	obs := &IntersectionObserver{
		r:         r,
		callback:  call.Arguments[0],
		threshold: []float32{0},
	}
	obs.obj = call.This
	if len(call.Arguments) >= 2 {
		if opts, ok := call.Arguments[1].(*goja.Object); ok {
			if value := opts.Get("root"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
				root, ok := value.(*goja.Object)
				if !ok {
					panic(r.vm.NewTypeError("IntersectionObserver root must be an Element or Document"))
				}
				if !root.StrictEquals(r.docObj()) {
					nidValue := root.Get("__nid__")
					if nidValue == nil || goja.IsUndefined(nidValue) {
						panic(r.vm.NewTypeError("IntersectionObserver root must be an Element or Document"))
					}
					r.mu.Lock()
					obs.root = r.nodeRegistry[int(nidValue.ToInteger())]
					r.mu.Unlock()
					if obs.root == nil || obs.root.Type != dom.NodeElement {
						panic(r.vm.NewTypeError("IntersectionObserver root must be an Element or Document"))
					}
				}
			}
			if value := opts.Get("rootMargin"); value != nil && !goja.IsUndefined(value) {
				margin, ok := parseRootMargin(value.String())
				if !ok {
					panic(r.domError("SyntaxError", "Invalid IntersectionObserver rootMargin"))
				}
				obs.rootMargin = margin
			}
			if value := opts.Get("threshold"); value != nil && !goja.IsUndefined(value) {
				threshold, ok := r.parseThreshold(value)
				if !ok {
					panic(r.vm.NewTypeError("IntersectionObserver threshold is too large"))
				}
				obs.threshold = threshold
			}
		}
	}
	_ = obs.obj.Set("observe", func(call goja.FunctionCall) goja.Value {
		return r.intersectionObserve(call, obs)
	})
	_ = obs.obj.Set("unobserve", func(call goja.FunctionCall) goja.Value {
		return r.intersectionUnobserve(call, obs)
	})
	_ = obs.obj.Set("disconnect", func(call goja.FunctionCall) goja.Value {
		return r.intersectionDisconnect(call, obs)
	})
	if is := intersectionOf(r); is != nil {
		is.mu.Lock()
		is.observers = append(is.observers, obs)
		is.mu.Unlock()
	}
	return obs.obj
}

func parseRootMargin(value string) ([4]rootMarginValue, bool) {
	var margin [4]rootMarginValue
	parts := strings.Fields(value)
	if len(parts) < 1 || len(parts) > 4 {
		return margin, false
	}
	values := make([]rootMarginValue, len(parts))
	for i, part := range parts {
		part = strings.ToLower(part)
		percent := strings.HasSuffix(part, "%")
		if percent {
			part = strings.TrimSuffix(part, "%")
		} else if strings.HasSuffix(part, "px") {
			part = strings.TrimSuffix(part, "px")
		} else if part != "0" {
			return margin, false
		}
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil || number != number || number < -1e12 || number > 1e12 {
			return margin, false
		}
		values[i] = rootMarginValue{value: float32(number), percent: percent}
	}
	switch len(values) {
	case 1:
		margin = [4]rootMarginValue{values[0], values[0], values[0], values[0]}
	case 2:
		margin = [4]rootMarginValue{values[0], values[1], values[0], values[1]}
	case 3:
		margin = [4]rootMarginValue{values[0], values[1], values[2], values[1]}
	case 4:
		margin = [4]rootMarginValue{values[0], values[1], values[2], values[3]}
	}
	return margin, true
}

func (r *Runtime) parseThreshold(value goja.Value) ([]float32, bool) {
	raw := []float64{value.ToFloat()}
	if object, ok := value.(*goja.Object); ok {
		lengthValue := object.Get("length")
		if lengthValue == nil || goja.IsUndefined(lengthValue) {
			return nil, false
		}
		length := int(lengthValue.ToInteger())
		if length < 0 || length > 64 {
			return nil, false
		}
		if length == 0 {
			return []float32{0}, true
		}
		raw = make([]float64, length)
		for i := range raw {
			item := object.Get(intToString(i))
			if item == nil || goja.IsUndefined(item) {
				raw[i] = 0
			} else {
				raw[i] = item.ToFloat()
			}
		}
	}
	thresholds := make([]float32, 0, len(raw))
	for _, number := range raw {
		if number != number {
			number = 0
		}
		if number < 0 {
			number = 0
		}
		if number > 1 {
			number = 1
		}
		thresholds = append(thresholds, float32(number))
	}
	sort.Slice(thresholds, func(i, j int) bool { return thresholds[i] < thresholds[j] })
	unique := thresholds[:0]
	for _, threshold := range thresholds {
		if len(unique) == 0 || unique[len(unique)-1] != threshold {
			unique = append(unique, threshold)
		}
	}
	return unique, true
}

func (r *Runtime) intersectionObserve(call goja.FunctionCall, obs *IntersectionObserver) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	target, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		return goja.Undefined()
	}
	nidValue := target.Get("__nid__")
	if nidValue == nil || goja.IsUndefined(nidValue) {
		return goja.Undefined()
	}
	r.mu.Lock()
	node := r.nodeRegistry[int(nidValue.ToInteger())]
	r.mu.Unlock()
	if node == nil {
		return goja.Undefined()
	}
	obs.mu.Lock()
	for _, entry := range obs.entries {
		if entry.target == target {
			obs.mu.Unlock()
			return goja.Undefined()
		}
	}
	obs.entries = append(obs.entries, &IntersectionEntry{
		target:     target,
		targetNode: node,
		threshold:  append([]float32(nil), obs.threshold...),
	})
	obs.mu.Unlock()
	if r.scriptExecutionDepth == 0 {
		r.scheduleIntersectionCheck()
	}
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
	r.scheduleIntersectionCheck()
}

func (r *Runtime) scheduleIntersectionCheck() {
	if r == nil {
		return
	}
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	if is.checkQueued {
		is.mu.Unlock()
		return
	}
	is.checkQueued = true
	is.mu.Unlock()
	r.queueMicrotask(func() error {
		r.deliverIntersections()
		return nil
	})
}

func (r *Runtime) deliverIntersections() {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return
	}
	is := intersectionOf(r)
	if is == nil {
		return
	}
	is.mu.Lock()
	is.checkQueued = false
	observers := append([]*IntersectionObserver(nil), is.observers...)
	elementRects := make(map[int][4]float32, len(is.elementRects))
	for id, rect := range is.elementRects {
		elementRects[id] = rect
	}
	is.mu.Unlock()
	viewportRect := r.currentViewportRect()
	provider := r.layoutGeometryProvider()
	for _, obs := range observers {
		obs.mu.Lock()
		entries := append([]*IntersectionEntry(nil), obs.entries...)
		callback := obs.callback
		root := obs.root
		margin := obs.rootMargin
		obs.mu.Unlock()
		if callback == nil || len(entries) == 0 {
			continue
		}
		rootRect, rootOK := r.intersectionRootRect(root, viewportRect, margin, provider, elementRects)
		var changed []intersectionResult
		for _, entry := range entries {
			info, ok := r.intersectionInfo(entry.targetNode, provider, elementRects)
			if !ok {
				continue
			}
			targetRect := r.clientRectFromNodeInfo(entry.targetNode, info)
			intersectionRect, isIntersecting := intersectClientRects(rootRect, targetRect)
			ratio := float32(clientIntersectionRatio(rootRect, targetRect))
			thresholdIndex := 0
			for _, threshold := range entry.threshold {
				if ratio < threshold {
					break
				}
				thresholdIndex++
			}
			obs.mu.Lock()
			present := false
			for _, current := range obs.entries {
				if current == entry {
					present = true
					break
				}
			}
			if !present {
				obs.mu.Unlock()
				continue
			}
			shouldNotify := !entry.initialized || entry.isIntersecting != isIntersecting || entry.thresholdIndex != thresholdIndex
			entry.initialized = true
			entry.isIntersecting = isIntersecting
			entry.thresholdIndex = thresholdIndex
			obs.mu.Unlock()
			if shouldNotify {
				changed = append(changed, intersectionResult{
					entry:              entry,
					ratio:              ratio,
					isIntersecting:     isIntersecting,
					boundingClientRect: targetRect,
					intersectionRect:   intersectionRect,
				})
			}
		}
		if len(changed) > 0 {
			r.fireIntersectionCallback(obs, callback, changed, rootRect, rootOK)
		}
	}
}

func (r *Runtime) intersectionRootRect(root *dom.Node, viewport [4]float64, margin [4]rootMarginValue, provider LayoutGeometryProvider, rects map[int][4]float32) ([4]float64, bool) {
	rect := viewport
	if root != nil && root.Type != dom.NodeDocument {
		info, ok := r.intersectionInfo(root, provider, rects)
		if !ok {
			return [4]float64{}, false
		}
		rect = r.clientRectFromNodeInfo(root, info)
	}
	top := resolveRootMargin(margin[0], rect[2])
	right := resolveRootMargin(margin[1], rect[2])
	bottom := resolveRootMargin(margin[2], rect[2])
	left := resolveRootMargin(margin[3], rect[2])
	x := rect[0] - left
	y := rect[1] - top
	width := rect[2] + left + right
	height := rect[3] + top + bottom
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return [4]float64{x, y, width, height}, true
}

func resolveRootMargin(margin rootMarginValue, basis float64) float64 {
	if margin.percent {
		return float64(margin.value) * float64(basis) / 100
	}
	return float64(margin.value)
}

func (r *Runtime) intersectionInfo(node *dom.Node, provider LayoutGeometryProvider, rects map[int][4]float32) (LayoutBoxInfo, bool) {
	if node == nil {
		return LayoutBoxInfo{}, false
	}
	if provider != nil {
		if info, ok := provider(node); ok {
			return info, true
		}
	}
	if rect, ok := rects[int(node.ID)]; ok {
		return LayoutBoxInfo{X: float64(rect[0]), Y: float64(rect[1]), Width: float64(rect[2]), Height: float64(rect[3])}, true
	}
	return LayoutBoxInfo{}, true
}

func intersectClientRects(a, b [4]float64) ([4]float64, bool) {
	left := max64(a[0], b[0])
	top := max64(a[1], b[1])
	right := min64(a[0]+a[2], b[0]+b[2])
	bottom := min64(a[1]+a[3], b[1]+b[3])
	if left > right || top > bottom {
		return [4]float64{}, false
	}
	return [4]float64{left, top, right - left, bottom - top}, true
}

func clientIntersectionRatio(a, b [4]float64) float64 {
	rect, intersects := intersectClientRects(a, b)
	if !intersects || b[2] <= 0 || b[3] <= 0 {
		return 0
	}
	area := rect[2] * rect[3]
	if area <= 0 {
		return 0
	}
	return area / (b[2] * b[3])
}

func (r *Runtime) fireIntersectionCallback(obs *IntersectionObserver, callback goja.Value, entries []intersectionResult, rootRect [4]float64, rootOK bool) {
	fn, ok := goja.AssertFunction(callback)
	if !ok {
		return
	}
	array := r.vm.NewArray()
	for i, result := range entries {
		entry := r.vm.NewObject()
		_ = entry.Set("target", result.entry.target)
		_ = entry.Set("isIntersecting", result.isIntersecting)
		_ = entry.Set("intersectionRatio", result.ratio)
		_ = entry.Set("boundingClientRect", r.newDOMRect(result.boundingClientRect[0], result.boundingClientRect[1], result.boundingClientRect[2], result.boundingClientRect[3]))
		_ = entry.Set("intersectionRect", r.newDOMRect(result.intersectionRect[0], result.intersectionRect[1], result.intersectionRect[2], result.intersectionRect[3]))
		if rootOK {
			_ = entry.Set("rootBounds", r.newDOMRect(rootRect[0], rootRect[1], rootRect[2], rootRect[3]))
		} else {
			_ = entry.Set("rootBounds", goja.Null())
		}
		_ = array.Set(intToString(i), entry)
	}
	if _, err := fn(obs.obj, array, obs.obj); err != nil {
		r.reportWindowException(err)
	}
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

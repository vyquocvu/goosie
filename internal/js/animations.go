package js

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
)

// jsAnimation holds the Go-side state for one Web Animation.
type jsAnimation struct {
	r         *Runtime
	nodeID    int
	keyframes []css.KeyframeStop
	effect    *goja.Object // KeyframeEffect JS object
	obj       *goja.Object // Animation JS object

	// Timing
	duration  time.Duration
	delay     time.Duration
	endDelay  time.Duration
	iterCount int // 0 = Infinity
	direction css.AnimationDirection
	fillMode  css.AnimationFillMode
	easing    css.TimingFunc

	// Playback state
	playState    string // "idle", "running", "paused", "finished"
	startTime    time.Time
	pausedAt     time.Duration // elapsed time when paused
	playbackRate float64
	finished     *promiseObj
	onfinish     goja.Value
	cancelled    bool
}

// animationStateKey is the global key under which we store the animation registry.
const animationStateKey = "__animRegistry__"

// animRegistry holds all active JS animations for one runtime.
type animRegistry struct {
	r     *Runtime
	anims []*jsAnimation
}

func getAnimRegistry(r *Runtime) *animRegistry {
	v := r.vm.Get(animationStateKey)
	if v == nil || goja.IsUndefined(v) {
		return nil
	}
	reg, ok := v.Export().(*animRegistry)
	if !ok {
		return nil
	}
	return reg
}

func setAnimRegistry(r *Runtime, reg *animRegistry) {
	_ = r.vm.Set(animationStateKey, reg)
}

// setupAnimations installs the Web Animations API:
// - Element.prototype.animate(keyframes, options)
// - Animation constructor
// - KeyframeEffect constructor
func (r *Runtime) setupAnimations() {
	reg := &animRegistry{r: r}
	setAnimRegistry(r, reg)
	r.installAnimationElementAPI(reg)

	// Install Animation and KeyframeEffect constructors globally.
	r.installAnimationConstructor(reg)
	r.installKeyframeEffectConstructor()
}

// installAnimationElementAPI binds animate() whenever setupDOM has created the
// Element prototype. Runtimes that receive their Document later use this when
// SetDOM rebuilds the prototype chain.
func (r *Runtime) installAnimationElementAPI(reg *animRegistry) {
	// Install animate() on Element.prototype only when a DOM is present;
	// without one, setupDOM skips prototype creation and elementProto is nil.
	if reg != nil && r.elementProto != nil {
		_ = r.elementProto.Set("animate", func(call goja.FunctionCall) goja.Value {
			return r.jsElementAnimate(call, reg)
		})
	}
}

// jsElementAnimate implements element.animate(keyframes, options).
func (r *Runtime) jsElementAnimate(call goja.FunctionCall, reg *animRegistry) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	options := goja.Undefined()
	if len(call.Arguments) > 1 {
		options = call.Arguments[1]
	}

	// Capture the __nid__ (nodeRegistry key) from the JS wrapper, which may
	// differ from node.ID.
	nid := thisNID(call)
	if nid == 0 {
		nid = int(node.ID)
	}

	// Parse keyframes from the first argument.
	kf, err := r.parseKeyframes(call.Arguments[0])
	if err != nil || len(kf) == 0 {
		// Return a dummy animation if keyframes are invalid.
		return r.createAnimationObject(reg, nid, nil, options)
	}

	return r.createAnimationObject(reg, nid, kf, options)
}

// parseKeyframes handles both array and object keyframe formats:
// - Array: [{opacity: 0}, {opacity: 1}]
// - Object: {opacity: [0, 1], transform: ["translateX(0)", "translateX(100px)"]}
func (r *Runtime) parseKeyframes(v goja.Value) ([]css.KeyframeStop, error) {
	if goja.IsNull(v) || goja.IsUndefined(v) {
		return nil, fmt.Errorf("invalid keyframes")
	}

	obj := v.ToObject(r.vm)
	if obj == nil {
		return nil, fmt.Errorf("invalid keyframes")
	}

	// Check if it's an array.
	lengthVal := obj.Get("length")
	if lengthVal != nil && !goja.IsUndefined(lengthVal) {
		length := int(lengthVal.ToInteger())
		if length > 0 {
			// Check if first element is an object (array format).
			first := obj.Get("0")
			if first != nil {
				if _, isObj := first.(*goja.Object); isObj {
					return r.parseArrayKeyframes(obj, length)
				}
			}
		}
	}

	// Object format: {property: [val1, val2, ...]}
	return r.parseObjectKeyframes(obj)
}

// parseArrayKeyframes handles [{offset: 0, opacity: 0}, {offset: 1, opacity: 1}].
func (r *Runtime) parseArrayKeyframes(arr *goja.Object, length int) ([]css.KeyframeStop, error) {
	stops := make([]css.KeyframeStop, 0, length)
	for i := 0; i < length; i++ {
		elem := arr.Get(fmt.Sprintf("%d", i))
		if elem == nil || goja.IsNull(elem) || goja.IsUndefined(elem) {
			continue
		}
		eObj := elem.ToObject(r.vm)
		if eObj == nil {
			continue
		}

		stop := css.KeyframeStop{
			Offset:       float32(i) / float32(maxInt(length-1, 1)),
			Declarations: make(map[string]string),
		}

		// Check for explicit offset.
		if offVal := eObj.Get("offset"); offVal != nil && !goja.IsUndefined(offVal) {
			stop.Offset = float32(offVal.ToFloat())
		}
		// Check for easing.
		if easeVal := eObj.Get("easing"); easeVal != nil && !goja.IsUndefined(easeVal) {
			_ = easeVal.String() // stored but not per-keyframe in our simplified model
		}

		// Extract all properties.
		keys := eObj.Keys()
		for _, key := range keys {
			if key == "offset" || key == "easing" || key == "composite" {
				continue
			}
			val := eObj.Get(key)
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				stop.Declarations[toCSSProperty(key)] = val.String()
			}
		}

		if len(stop.Declarations) > 0 {
			stops = append(stops, stop)
		}
	}

	// Ensure first stop is at 0 and last is at 1.
	if len(stops) > 0 {
		if stops[0].Offset != 0 {
			first := css.KeyframeStop{Offset: 0, Declarations: make(map[string]string)}
			for k, v := range stops[0].Declarations {
				first.Declarations[k] = v
			}
			stops = append([]css.KeyframeStop{first}, stops...)
		}
		if stops[len(stops)-1].Offset != 1 {
			last := css.KeyframeStop{Offset: 1, Declarations: make(map[string]string)}
			for k, v := range stops[len(stops)-1].Declarations {
				last.Declarations[k] = v
			}
			stops = append(stops, last)
		}
	}

	return stops, nil
}

// parseObjectKeyframes handles {opacity: [0, 1], transform: ["translateX(0)", "translateX(100px)"]}.
func (r *Runtime) parseObjectKeyframes(obj *goja.Object) ([]css.KeyframeStop, error) {
	keys := obj.Keys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty keyframes")
	}

	// Find the maximum array length across all properties.
	maxLen := 0
	propValues := make(map[string][]string)
	for _, key := range keys {
		if key == "offset" || key == "easing" || key == "composite" {
			continue
		}
		val := obj.Get(key)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			continue
		}

		cssProp := toCSSProperty(key)

		// Check if it's an array.
		if vObj, ok := val.(*goja.Object); ok {
			arrLen := vObj.Get("length")
			if arrLen != nil && !goja.IsUndefined(arrLen) {
				n := int(arrLen.ToInteger())
				if n > maxLen {
					maxLen = n
				}
				vals := make([]string, n)
				for i := 0; i < n; i++ {
					elem := vObj.Get(fmt.Sprintf("%d", i))
					if elem != nil && !goja.IsUndefined(elem) {
						vals[i] = elem.String()
					}
				}
				propValues[cssProp] = vals
				continue
			}
		}

		// Single value: treat as array of length 1.
		propValues[cssProp] = []string{val.String()}
		if 1 > maxLen {
			maxLen = 1
		}
	}

	if maxLen == 0 {
		return nil, fmt.Errorf("no animatable properties in keyframes")
	}

	// Build keyframe stops.
	stops := make([]css.KeyframeStop, maxLen)
	for i := 0; i < maxLen; i++ {
		offset := float32(i) / float32(maxInt(maxLen-1, 1))
		stops[i] = css.KeyframeStop{
			Offset:       offset,
			Declarations: make(map[string]string),
		}
		for prop, vals := range propValues {
			idx := i
			if idx >= len(vals) {
				idx = len(vals) - 1
			}
			if vals[idx] != "" {
				stops[i].Declarations[prop] = vals[idx]
			}
		}
	}

	return stops, nil
}

// parseTimingOptions extracts timing from the options argument.
func parseTimingOptions(r *Runtime, v goja.Value) (duration, delay, endDelay time.Duration, iterCount int, direction css.AnimationDirection, fill css.AnimationFillMode, easing css.TimingFunc) {
	// Defaults
	duration = 0
	delay = 0
	endDelay = 0
	iterCount = 1
	direction = css.AnimNormal
	fill = css.FillNone
	easing = css.TimingEase

	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}

	// If it's a number, treat it as duration in ms.
	if n, ok := v.Export().(float64); ok {
		duration = time.Duration(n) * time.Millisecond
		return
	}
	if n, ok := v.Export().(int64); ok {
		duration = time.Duration(n) * time.Millisecond
		return
	}

	obj := v.ToObject(r.vm)
	if obj == nil {
		return
	}

	if d := obj.Get("duration"); d != nil && !goja.IsUndefined(d) {
		duration = time.Duration(d.ToFloat() * float64(time.Millisecond))
	}
	if d := obj.Get("delay"); d != nil && !goja.IsUndefined(d) {
		delay = time.Duration(d.ToFloat() * float64(time.Millisecond))
	}
	if d := obj.Get("endDelay"); d != nil && !goja.IsUndefined(d) {
		endDelay = time.Duration(d.ToFloat() * float64(time.Millisecond))
	}
	if d := obj.Get("iterations"); d != nil && !goja.IsUndefined(d) {
		n := d.ToFloat()
		if math.IsInf(n, 1) {
			iterCount = 0 // Infinity
		} else {
			iterCount = int(n)
			if iterCount < 0 {
				iterCount = 0
			}
		}
	}
	if d := obj.Get("direction"); d != nil && !goja.IsUndefined(d) {
		direction = css.ParseAnimationDirection(d.String())
	}
	if d := obj.Get("fill"); d != nil && !goja.IsUndefined(d) {
		fill = css.ParseAnimationFillMode(d.String())
	}
	if d := obj.Get("easing"); d != nil && !goja.IsUndefined(d) {
		easing = css.ParseTimingFunc(d.String())
	}

	return
}

// createAnimationObject builds the Animation JS object and registers it.
func (r *Runtime) createAnimationObject(reg *animRegistry, nid int, kf []css.KeyframeStop, optionsVal goja.Value) goja.Value {
	duration, delay, endDelay, iterCount, direction, fill, easing := parseTimingOptions(r, optionsVal)

	anim := &jsAnimation{
		r:            r,
		nodeID:       nid,
		keyframes:    kf,
		duration:     duration,
		delay:        delay,
		endDelay:     endDelay,
		iterCount:    iterCount,
		direction:    direction,
		fillMode:     fill,
		easing:       easing,
		playState:    "idle",
		playbackRate: 1.0,
		startTime:    time.Now(),
	}

	// Create the finished promise.
	anim.finished = r.newPromiseObj()

	// Build the KeyframeEffect object.
	effect := r.buildKeyframeEffect(anim)
	anim.effect = effect

	// Build the Animation object.
	animObj := r.buildAnimationObject(anim, effect)
	anim.obj = animObj

	// Register and auto-play.
	reg.anims = append(reg.anims, anim)
	r.playAnimation(anim)

	return animObj
}

// buildKeyframeEffect creates the KeyframeEffect JS object.
func (r *Runtime) buildKeyframeEffect(anim *jsAnimation) *goja.Object {
	obj := r.vm.NewObject()

	// Store a reference back to the Go animation.
	_ = obj.Set("__anim__", anim)

	// getTiming() returns the timing properties.
	_ = obj.Set("getTiming", func(call goja.FunctionCall) goja.Value {
		result := r.vm.NewObject()
		_ = result.Set("delay", anim.delay.Seconds()*1000)
		_ = result.Set("duration", anim.duration.Seconds()*1000)
		_ = result.Set("endDelay", anim.endDelay.Seconds()*1000)
		_ = result.Set("iterations", animIterJS(anim.iterCount))
		_ = result.Set("direction", animDirectionJS(anim.direction))
		_ = result.Set("fill", animFillJS(anim.fillMode))
		_ = result.Set("easing", "ease")
		return result
	})

	// getKeyframes() returns the keyframe array.
	_ = obj.Set("getKeyframes", func(call goja.FunctionCall) goja.Value {
		arr := r.vm.NewArray()
		for i, kf := range anim.keyframes {
			frame := r.vm.NewObject()
			_ = frame.Set("offset", float64(kf.Offset))
			for prop, val := range kf.Declarations {
				_ = frame.Set(toJSProperty(prop), val)
			}
			_ = arr.Set(fmt.Sprintf("%d", i), frame)
		}
		return arr
	})

	return obj
}

// buildAnimationObject creates the Animation JS object with all its methods and properties.
func (r *Runtime) buildAnimationObject(anim *jsAnimation, effect *goja.Object) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.Set("__anim__", anim)
	_ = obj.Set("effect", effect)

	// timeline - a simple DocumentTimeline stub.
	timeline := r.vm.NewObject()
	_ = timeline.Set("currentTime", r.vm.ToValue(float64(time.Now().UnixMilli())))
	_ = obj.Set("timeline", timeline)

	// play() method
	_ = obj.Set("play", func(call goja.FunctionCall) goja.Value {
		r.playAnimation(anim)
		return goja.Undefined()
	})

	// pause() method
	_ = obj.Set("pause", func(call goja.FunctionCall) goja.Value {
		r.pauseAnimation(anim)
		return goja.Undefined()
	})

	// cancel() method
	_ = obj.Set("cancel", func(call goja.FunctionCall) goja.Value {
		r.cancelAnimation(anim)
		return goja.Undefined()
	})

	// finish() method
	_ = obj.Set("finish", func(call goja.FunctionCall) goja.Value {
		r.finishAnimation(anim)
		return goja.Undefined()
	})
	// commitStyles() writes the effect's current values into the target's
	// inline style declaration, using the CSSOM mutation path so observed style
	// attributes receive their custom-element reaction.
	_ = obj.Set("commitStyles", func(call goja.FunctionCall) goja.Value {
		node := r.nodeRegistry[anim.nodeID]
		if node == nil || len(anim.keyframes) == 0 {
			return goja.Undefined()
		}
		offset := float32(0)
		if anim.duration > 0 {
			elapsed := time.Since(anim.startTime) - anim.delay
			offset = float32(elapsed.Seconds() / anim.duration.Seconds())
			if offset < 0 {
				offset = 0
			} else if offset > 1 {
				offset = 1
			}
		}
		style := node.GetAttribute("style")
		for _, prop := range animatedProperties(anim.keyframes) {
			if value := interpolateKeyframes(anim.keyframes, prop, offset); value != "" {
				style = mergeInlineStyle(style, toCSSProperty(prop), value)
			}
		}
		r.setInlineStyleText(node, style)
		return goja.Undefined()
	})

	// reverse() method
	_ = obj.Set("reverse", func(call goja.FunctionCall) goja.Value {
		r.reverseAnimation(anim)
		return goja.Undefined()
	})

	// addEventListener / removeEventListener stubs for animation events.
	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})

	// Set initial JS-visible state.
	_ = obj.Set("__playState__", "idle")
	_ = obj.Set("__playbackRate__", 1.0)
	_ = obj.Set("__startTimeMs__", float64(0))
	_ = obj.Set("__onfinishHandler__", goja.Null())

	// Use RunString with closures that capture the obj reference for accessor properties.
	_ = r.vm.Set("__animObj__", obj)
	_, _ = r.vm.RunString(`
	(function(obj) {
		Object.defineProperty(obj, 'playState', {
			get: function() { return obj.__playState__; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'playbackRate', {
			get: function() { return obj.__playbackRate__; },
			set: function(v) { obj.__playbackRate__ = Number(v) || 1; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'currentTime', {
			get: function() {
				if (obj.__playState__ === 'idle') return null;
				if (obj.__pausedAtMs__ !== undefined && obj.__pausedAtMs__ !== null) {
					return obj.__pausedAtMs__;
				}
				var elapsed = Date.now() - obj.__startTimeMs__;
				return elapsed * obj.__playbackRate__;
			},
			set: function(v) {
				if (v === null || v === undefined) return;
				obj.__pausedAtMs__ = Number(v);
			},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'finished', {
			get: function() { return obj.__finishedPromise__ || Promise.resolve(); },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'onfinish', {
			get: function() { return obj.__onfinishHandler__ || null; },
			set: function(v) { obj.__onfinishHandler__ = v; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'oncancel', {
			get: function() { return null; },
			set: function() {},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'onremove', {
			get: function() { return null; },
			set: function() {},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(obj, 'startTime', {
			get: function() { return obj.__startTimeMs__ || null; },
			configurable: true,
			enumerable: true
		});
	})(__animObj__)
	`)
	_ = r.vm.Set("__animObj__", nil)

	return obj
}

// playAnimation starts or resumes an animation.
func (r *Runtime) playAnimation(anim *jsAnimation) {
	if anim.playState == "paused" {
		// Resume: adjust start time so currentTime is preserved.
		anim.startTime = time.Now().Add(-anim.pausedAt)
		anim.pausedAt = -1
		_ = anim.obj.Set("__pausedAtMs__", goja.Undefined())
	} else {
		anim.startTime = time.Now()
		anim.pausedAt = -1
		anim.cancelled = false
		// Create a new finished promise for this play.
		anim.finished = r.newPromiseObj()
		_ = anim.obj.Set("__finishedPromise__", anim.finished.obj)
	}
	anim.playState = "running"
	_ = anim.obj.Set("__playState__", "running")
	_ = anim.obj.Set("__playbackRate__", anim.playbackRate)
	_ = anim.obj.Set("__startTimeMs__", float64(anim.startTime.UnixMilli()))
}

// pauseAnimation pauses an animation.
func (r *Runtime) pauseAnimation(anim *jsAnimation) {
	if anim.playState == "finished" || anim.playState == "idle" {
		return
	}
	if anim.playState == "running" {
		elapsed := time.Since(anim.startTime)
		anim.pausedAt = elapsed
		_ = anim.obj.Set("__pausedAtMs__", float64(elapsed.Milliseconds()))
	}
	anim.playState = "paused"
	_ = anim.obj.Set("__playState__", "paused")
}

// cancelAnimation cancels an animation.
func (r *Runtime) cancelAnimation(anim *jsAnimation) {
	anim.playState = "idle"
	anim.cancelled = true
	anim.pausedAt = -1
	_ = anim.obj.Set("__playState__", "idle")
	// Reset the element's animated style.
	r.clearAnimatedStyle(anim)
}

// finishAnimation forces an animation to its end state.
func (r *Runtime) finishAnimation(anim *jsAnimation) {
	if anim.playState == "idle" || anim.playState == "finished" {
		return
	}
	anim.playState = "finished"
	_ = anim.obj.Set("__playState__", "finished")
	// Apply the final keyframe values.
	r.applyAnimationAtOffset(anim, animFinalOffset(anim))
	// Resolve the finished promise.
	if anim.finished != nil {
		r.resolvePromise(anim.finished, anim.obj)
	}
	// Fire onfinish.
	r.fireOnFinish(anim)
}

// reverseAnimation reverses the playback direction.
func (r *Runtime) reverseAnimation(anim *jsAnimation) {
	anim.playbackRate = -anim.playbackRate
	_ = anim.obj.Set("__playbackRate__", anim.playbackRate)
	if anim.playState == "running" {
		// Recalculate start time to preserve current position.
		elapsed := time.Since(anim.startTime)
		remaining := anim.duration - elapsed
		anim.startTime = time.Now().Add(-remaining)
		_ = anim.obj.Set("__startTimeMs__", float64(anim.startTime.UnixMilli()))
	}
}

// TickAnimations advances all registered animations. Called once per frame.
func (r *Runtime) TickAnimations() {
	reg := getAnimRegistry(r)
	if reg == nil || len(reg.anims) == 0 {
		return
	}

	now := time.Now()
	kept := reg.anims[:0]
	for _, anim := range reg.anims {
		if anim.cancelled {
			continue
		}
		if anim.playState != "running" {
			// Keep finished animations with fill mode, or paused ones.
			if anim.playState == "paused" || (anim.playState == "finished" &&
				(anim.fillMode == css.FillForwards || anim.fillMode == css.FillBoth)) {
				kept = append(kept, anim)
			}
			continue
		}

		elapsed := now.Sub(anim.startTime)
		if anim.playbackRate < 0 {
			// For reversed animations, we handle differently.
			elapsed = time.Duration(float64(elapsed) * anim.playbackRate)
		}

		// Still in delay period.
		if elapsed < anim.delay {
			// Apply fill-backwards if needed.
			if anim.fillMode == css.FillBackwards || anim.fillMode == css.FillBoth {
				r.applyAnimationAtOffset(anim, 0)
			}
			kept = append(kept, anim)
			continue
		}

		activeDuration := elapsed - anim.delay
		if anim.duration <= 0 {
			anim.playState = "finished"
			_ = anim.obj.Set("__playState__", "finished")
			r.applyAnimationAtOffset(anim, animFinalOffset(anim))
			if anim.finished != nil {
				r.resolvePromise(anim.finished, anim.obj)
			}
			r.fireOnFinish(anim)
			if anim.fillMode == css.FillForwards || anim.fillMode == css.FillBoth {
				kept = append(kept, anim)
			}
			continue
		}

		// Calculate total progress in iterations.
		totalProgress := float64(activeDuration) / float64(anim.duration)

		if anim.iterCount > 0 && totalProgress >= float64(anim.iterCount) {
			// Animation complete.
			anim.playState = "finished"
			_ = anim.obj.Set("__playState__", "finished")
			offset := animFinalOffset(anim)
			r.applyAnimationAtOffset(anim, offset)
			if anim.finished != nil {
				r.resolvePromise(anim.finished, anim.obj)
			}
			r.fireOnFinish(anim)
			if anim.fillMode == css.FillForwards || anim.fillMode == css.FillBoth {
				kept = append(kept, anim)
			}
			continue
		}

		// Current iteration.
		iteration := int(totalProgress)
		iterProgress := totalProgress - float64(iteration)

		// Apply direction.
		offset := animApplyDirection(iterProgress, iteration, anim.direction)

		// Apply easing.
		eased := anim.easing.Eval(float64(offset))
		offset = float32(eased)

		r.applyAnimationAtOffset(anim, offset)
		kept = append(kept, anim)
	}
	reg.anims = kept
}

// applyAnimationAtOffset computes interpolated values at the given offset and
// applies them to the DOM node's inline style.
func (r *Runtime) applyAnimationAtOffset(anim *jsAnimation, offset float32) {
	node := r.nodeRegistry[anim.nodeID]
	if node == nil {
		return
	}

	for _, prop := range animatedProperties(anim.keyframes) {
		val := interpolateKeyframes(anim.keyframes, prop, offset)
		if val != "" {
			node.SetAttribute("style", mergeInlineStyle(node.GetAttribute("style"), toCSSProperty(prop), val))
		}
	}
}

// clearAnimatedStyle removes animated properties from the inline style.
func (r *Runtime) clearAnimatedStyle(anim *jsAnimation) {
	node := r.nodeRegistry[anim.nodeID]
	if node == nil {
		return
	}
	for _, prop := range animatedProperties(anim.keyframes) {
		cssProp := toCSSProperty(prop)
		style := node.GetAttribute("style")
		node.SetAttribute("style", removeInlineStyle(style, cssProp))
	}
}

// fireOnFinish invokes the onfinish handler.
func (r *Runtime) fireOnFinish(anim *jsAnimation) {
	if anim.onfinish != nil {
		if fn, ok := goja.AssertFunction(anim.onfinish); ok {
			event := r.vm.NewObject()
			_ = event.Set("type", "finish")
			_ = event.Set("target", anim.obj)
			_, _ = fn(anim.obj, event)
		}
	}
	// Also check the JS-side handler.
	if anim.obj != nil {
		handler := anim.obj.Get("__onfinishHandler__")
		if handler != nil && !goja.IsUndefined(handler) && !goja.IsNull(handler) {
			if fn, ok := goja.AssertFunction(handler); ok {
				event := r.vm.NewObject()
				_ = event.Set("type", "finish")
				_ = event.Set("target", anim.obj)
				_, _ = fn(anim.obj, event)
			}
		}
	}
}

// installAnimationConstructor publishes the Animation constructor globally.
func (r *Runtime) installAnimationConstructor(reg *animRegistry) {
	_ = r.vm.Set("__reg__", reg)
	v, err := r.vm.RunString(`
	(function(reg) {
		function Animation(effect, timeline) {
			// Simplified constructor: creates an idle animation.
			this.effect = effect || null;
			this.timeline = timeline || null;
			this.__playState = 'idle';
			this.__playbackRate = 1;
			this.__startTime = null;
			this.__currentTime = null;
			this.__finishedPromise = Promise.resolve(this);
			this.onfinish = null;
			this.oncancel = null;
			this.onremove = null;
		}
		Animation.prototype.play = function() { this.__playState = 'running'; };
		Animation.prototype.pause = function() {
			if (this.__playState === 'running') this.__playState = 'paused';
		};
		Animation.prototype.cancel = function() { this.__playState = 'idle'; };
		Animation.prototype.finish = function() {
			// Noop if idle or already finished.
			if (this.__playState === 'idle' || this.__playState === 'finished') return;
			this.__playState = 'finished';
		};
		Animation.prototype.reverse = function() { this.__playbackRate = -this.__playbackRate; };
		Object.defineProperty(Animation.prototype, 'playState', {
			get: function() { return this.__playState; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(Animation.prototype, 'playbackRate', {
			get: function() { return this.__playbackRate; },
			set: function(v) { this.__playbackRate = Number(v) || 1; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(Animation.prototype, 'currentTime', {
			get: function() { return this.__currentTime; },
			set: function(v) { this.__currentTime = v; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(Animation.prototype, 'startTime', {
			get: function() { return this.__startTime; },
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(Animation.prototype, 'finished', {
			get: function() { return this.__finishedPromise; },
			configurable: true,
			enumerable: true
		});
		return Animation;
	})(__reg__)
	`)
	_ = r.vm.Set("__reg__", nil)
	if err == nil && v != nil {
		_ = r.vm.Set("Animation", v)
	}
}

// installKeyframeEffectConstructor publishes KeyframeEffect globally.
func (r *Runtime) installKeyframeEffectConstructor() {
	v, err := r.vm.RunString(`
		function KeyframeEffect(target, keyframes, options) {
			this.target = target;
			this.__keyframes = keyframes;
			this.__options = options;
		}
		KeyframeEffect.prototype.getKeyframes = function() { return this.__keyframes || []; };
		KeyframeEffect.prototype.setKeyframes = function(kf) { this.__keyframes = kf; };
		KeyframeEffect.prototype.getTiming = function() {
			var opts = this.__options || {};
			return {
				delay: opts.delay || 0,
				duration: opts.duration || 0,
				endDelay: opts.endDelay || 0,
				iterations: opts.iterations !== undefined ? opts.iterations : 1,
				direction: opts.direction || 'normal',
				fill: opts.fill || 'none',
				easing: opts.easing || 'ease'
			};
		};
		KeyframeEffect
	`)
	if err == nil && v != nil {
		_ = r.vm.Set("KeyframeEffect", v)
	}
}

// ---------------------------------------------------------------------------
// Interpolation helpers (self-contained, mirroring engine/animation.go logic)
// ---------------------------------------------------------------------------

// animatedProperties returns the union of all property names across keyframes.
func animatedProperties(stops []css.KeyframeStop) []string {
	seen := make(map[string]bool)
	var props []string
	for _, stop := range stops {
		for prop := range stop.Declarations {
			if !seen[prop] {
				seen[prop] = true
				props = append(props, prop)
			}
		}
	}
	return props
}

// interpolateKeyframes computes the value of one property at the given offset.
func interpolateKeyframes(stops []css.KeyframeStop, property string, offset float32) string {
	if len(stops) == 0 {
		return ""
	}
	if len(stops) == 1 {
		return stops[0].Declarations[property]
	}

	// Before first stop.
	if offset <= stops[0].Offset {
		return stops[0].Declarations[property]
	}
	// After last stop.
	if offset >= stops[len(stops)-1].Offset {
		return stops[len(stops)-1].Declarations[property]
	}

	// Find surrounding stops.
	lo, hi := 0, len(stops)-1
	for i := 0; i < len(stops)-1; i++ {
		if offset >= stops[i].Offset && offset <= stops[i+1].Offset {
			lo = i
			hi = i + 1
			break
		}
	}

	fromVal, fromOK := stops[lo].Declarations[property]
	toVal, toOK := stops[hi].Declarations[property]

	if !fromOK && !toOK {
		return ""
	}
	if !fromOK {
		return toVal
	}
	if !toOK {
		return fromVal
	}

	// Segment progress.
	span := float64(stops[hi].Offset - stops[lo].Offset)
	if span <= 0 {
		return toVal
	}
	segProgress := (float64(offset) - float64(stops[lo].Offset)) / span

	// Try color interpolation.
	if fromColor, ok := css.ParseColor(fromVal); ok {
		if toColor, ok2 := css.ParseColor(toVal); ok2 {
			c := interpColor(fromColor, toColor, segProgress)
			return formatCSSColor(c)
		}
	}

	// Try numeric interpolation.
	if fromNum, fromUnit, ok := parseCSSNumeric(fromVal); ok {
		if toNum, toUnit, ok2 := parseCSSNumeric(toVal); ok2 && toUnit == fromUnit {
			n := fromNum*(1-segProgress) + toNum*segProgress
			return formatCSSNumeric(n, fromUnit)
		}
	}

	// Discrete: snap at midpoint.
	if segProgress < 0.5 {
		return fromVal
	}
	return toVal
}

// interpColor linearly interpolates between two CSS colors.
func interpColor(from, to css.Color, t float64) css.Color {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	lerp := func(a, b uint8) uint8 {
		return uint8(float64(a)*(1-t) + float64(b)*t + 0.5)
	}
	return css.Color{R: lerp(from.R, to.R), G: lerp(from.G, to.G), B: lerp(from.B, to.B), A: lerp(from.A, to.A)}
}

// formatCSSColor formats a css.Color as a CSS color string.
func formatCSSColor(c css.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
	}
	alpha := float64(c.A) / 255.0
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", c.R, c.G, c.B, strconv.FormatFloat(alpha, 'f', 2, 64))
}

// parseCSSNumeric tries to parse a CSS value as number + unit.
func parseCSSNumeric(s string) (float64, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, "", false
	}
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	hasDigit := false
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		hasDigit = true
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			hasDigit = true
			i++
		}
	}
	if !hasDigit {
		return 0, "", false
	}
	numStr := s[:i]
	unit := strings.TrimSpace(s[i:])
	v, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, "", false
	}
	return v, unit, true
}

// formatCSSNumeric formats a numeric value with its unit.
func formatCSSNumeric(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s + unit
}

// animApplyDirection computes the effective offset within an iteration.
func animApplyDirection(progress float64, iteration int, dir css.AnimationDirection) float32 {
	switch dir {
	case css.AnimNormal:
		return float32(progress)
	case css.AnimReverse:
		return float32(1 - progress)
	case css.AnimAlternate:
		if iteration%2 == 0 {
			return float32(progress)
		}
		return float32(1 - progress)
	case css.AnimAlternateReverse:
		if iteration%2 == 0 {
			return float32(1 - progress)
		}
		return float32(progress)
	}
	return float32(progress)
}

// animFinalOffset returns the offset when an animation finishes.
func animFinalOffset(anim *jsAnimation) float32 {
	switch anim.direction {
	case css.AnimNormal, css.AnimAlternate:
		if anim.iterCount > 0 && anim.iterCount%2 == 0 && anim.direction == css.AnimAlternate {
			return 0
		}
		return 1
	case css.AnimReverse, css.AnimAlternateReverse:
		if anim.direction == css.AnimAlternateReverse {
			if anim.iterCount > 0 && anim.iterCount%2 == 0 {
				return 1
			}
		}
		return 0
	}
	return 1
}

// animIterJS returns the JS-visible iteration count (Infinity for 0).
func animIterJS(n int) interface{} {
	if n == 0 {
		return math.Inf(1)
	}
	return float64(n)
}

// animDirectionJS returns the JS string for a direction.
func animDirectionJS(d css.AnimationDirection) string {
	switch d {
	case css.AnimNormal:
		return "normal"
	case css.AnimReverse:
		return "reverse"
	case css.AnimAlternate:
		return "alternate"
	case css.AnimAlternateReverse:
		return "alternate-reverse"
	}
	return "normal"
}

// animFillJS returns the JS string for a fill mode.
func animFillJS(f css.AnimationFillMode) string {
	switch f {
	case css.FillNone:
		return "none"
	case css.FillForwards:
		return "forwards"
	case css.FillBackwards:
		return "backwards"
	case css.FillBoth:
		return "both"
	}
	return "none"
}

// ---------------------------------------------------------------------------
// Inline style helpers
// ---------------------------------------------------------------------------

// mergeInlineStyle adds or replaces one property in a CSS inline style string.
func mergeInlineStyle(style, prop, value string) string {
	if style == "" {
		return prop + ": " + value
	}

	// Check if property already exists.
	lower := strings.ToLower(prop)
	parts := strings.Split(style, ";")
	found := false
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 0 {
			continue
		}
		existingProp := strings.TrimSpace(trimmed[:colonIdx])
		if strings.ToLower(existingProp) == lower {
			parts[i] = " " + prop + ": " + value + " "
			found = true
			break
		}
	}

	if found {
		return strings.Join(parts, ";")
	}

	// Append new property.
	result := strings.TrimRight(style, ";")
	return result + "; " + prop + ": " + value
}

// removeInlineStyle removes one property from a CSS inline style string.
func removeInlineStyle(style, prop string) string {
	if style == "" {
		return ""
	}
	lower := strings.ToLower(prop)
	parts := strings.Split(style, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx < 0 {
			kept = append(kept, part)
			continue
		}
		existingProp := strings.TrimSpace(trimmed[:colonIdx])
		if strings.ToLower(existingProp) == lower {
			continue // skip this property
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "; ")
}

// toCSSProperty converts a JS camelCase property name to CSS kebab-case.
func toCSSProperty(s string) string {
	var sb strings.Builder
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteRune(c + ('a' - 'A'))
		} else {
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// toJSProperty converts a CSS kebab-case property name to JS camelCase.
func toJSProperty(s string) string {
	var sb strings.Builder
	upper := false
	for _, c := range s {
		if c == '-' {
			upper = true
			continue
		}
		if upper {
			if c >= 'a' && c <= 'z' {
				sb.WriteRune(c - ('a' - 'A'))
			} else {
				sb.WriteRune(c)
			}
			upper = false
		} else {
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

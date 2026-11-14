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

type jsAnimation struct {
	r         *Runtime
	nodeID    int
	keyframes []css.KeyframeStop
	effect    *goja.Object
	obj       *goja.Object

	duration  time.Duration
	delay     time.Duration
	endDelay  time.Duration
	iterCount int
	direction css.AnimationDirection
	fillMode  css.AnimationFillMode
	easing    css.TimingFunc

	playState    string
	startTime    time.Time
	pausedAt     time.Duration
	playbackRate float64
	finished     *promiseObj
	onfinish     goja.Value
	cancelled    bool
}

const animationStateKey = "__animRegistry__"

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

func (r *Runtime) setupAnimations() {
	reg := &animRegistry{r: r}
	setAnimRegistry(r, reg)
	r.installAnimationElementAPI(reg)

	r.installAnimationConstructor(reg)
	r.installKeyframeEffectConstructor()
}

func (r *Runtime) installAnimationElementAPI(reg *animRegistry) {
	if reg != nil && r.elementProto != nil {
		_ = r.elementProto.Set("animate", func(call goja.FunctionCall) goja.Value {
			return r.jsElementAnimate(call, reg)
		})
	}
}

func (r *Runtime) jsElementAnimate(call goja.FunctionCall, reg *animRegistry) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	options := goja.Undefined()
	if len(call.Arguments) > 1 {
		options = call.Arguments[1]
	}

	nid := thisNID(call)
	if nid == 0 {
		nid = int(node.ID)
	}

	kf, err := r.parseKeyframes(call.Arguments[0])
	if err != nil || len(kf) == 0 {
		return r.createAnimationObject(reg, nid, nil, options)
	}

	return r.createAnimationObject(reg, nid, kf, options)
}

func (r *Runtime) parseKeyframes(v goja.Value) ([]css.KeyframeStop, error) {
	if goja.IsNull(v) || goja.IsUndefined(v) {
		return nil, fmt.Errorf("invalid keyframes")
	}

	obj := v.ToObject(r.vm)
	if obj == nil {
		return nil, fmt.Errorf("invalid keyframes")
	}

	lengthVal := obj.Get("length")
	if lengthVal != nil && !goja.IsUndefined(lengthVal) {
		length := int(lengthVal.ToInteger())
		if length > 0 {
			first := obj.Get("0")
			if first != nil {
				if _, isObj := first.(*goja.Object); isObj {
					return r.parseArrayKeyframes(obj, length)
				}
			}
		}
	}

	return r.parseObjectKeyframes(obj)
}

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

		if offVal := eObj.Get("offset"); offVal != nil && !goja.IsUndefined(offVal) {
			stop.Offset = float32(offVal.ToFloat())
		}
		if easeVal := eObj.Get("easing"); easeVal != nil && !goja.IsUndefined(easeVal) {
			_ = easeVal.String()
		}

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

func (r *Runtime) parseObjectKeyframes(obj *goja.Object) ([]css.KeyframeStop, error) {
	keys := obj.Keys()
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty keyframes")
	}

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

		propValues[cssProp] = []string{val.String()}
		if 1 > maxLen {
			maxLen = 1
		}
	}

	if maxLen == 0 {
		return nil, fmt.Errorf("no animatable properties in keyframes")
	}

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

func parseTimingOptions(r *Runtime, v goja.Value) (duration, delay, endDelay time.Duration, iterCount int, direction css.AnimationDirection, fill css.AnimationFillMode, easing css.TimingFunc) {
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
			iterCount = 0
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

	anim.finished = r.newPromiseObj()

	effect := r.buildKeyframeEffect(anim)
	anim.effect = effect

	animObj := r.buildAnimationObject(anim, effect)
	anim.obj = animObj

	reg.anims = append(reg.anims, anim)
	r.playAnimation(anim)

	return animObj
}

func (r *Runtime) buildKeyframeEffect(anim *jsAnimation) *goja.Object {
	obj := r.vm.NewObject()

	_ = obj.Set("__anim__", anim)

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

func (r *Runtime) buildAnimationObject(anim *jsAnimation, effect *goja.Object) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.Set("__anim__", anim)
	_ = obj.Set("effect", effect)

	timeline := r.vm.NewObject()
	_ = timeline.Set("currentTime", r.vm.ToValue(float64(time.Now().UnixMilli())))
	_ = obj.Set("timeline", timeline)

	_ = obj.Set("play", func(call goja.FunctionCall) goja.Value {
		r.playAnimation(anim)
		return goja.Undefined()
	})

	_ = obj.Set("pause", func(call goja.FunctionCall) goja.Value {
		r.pauseAnimation(anim)
		return goja.Undefined()
	})

	_ = obj.Set("cancel", func(call goja.FunctionCall) goja.Value {
		r.cancelAnimation(anim)
		return goja.Undefined()
	})

	_ = obj.Set("finish", func(call goja.FunctionCall) goja.Value {
		r.finishAnimation(anim)
		return goja.Undefined()
	})
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

	_ = obj.Set("reverse", func(call goja.FunctionCall) goja.Value {
		r.reverseAnimation(anim)
		return goja.Undefined()
	})

	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})

	_ = obj.Set("__playState__", "idle")
	_ = obj.Set("__playbackRate__", 1.0)
	_ = obj.Set("__startTimeMs__", float64(0))
	_ = obj.Set("__onfinishHandler__", goja.Null())

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

func (r *Runtime) playAnimation(anim *jsAnimation) {
	if anim.playState == "paused" {
		anim.startTime = time.Now().Add(-anim.pausedAt)
		anim.pausedAt = -1
		_ = anim.obj.Set("__pausedAtMs__", goja.Undefined())
	} else {
		anim.startTime = time.Now()
		anim.pausedAt = -1
		anim.cancelled = false
		anim.finished = r.newPromiseObj()
		_ = anim.obj.Set("__finishedPromise__", anim.finished.obj)
	}
	anim.playState = "running"
	_ = anim.obj.Set("__playState__", "running")
	_ = anim.obj.Set("__playbackRate__", anim.playbackRate)
	_ = anim.obj.Set("__startTimeMs__", float64(anim.startTime.UnixMilli()))
}

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

func (r *Runtime) cancelAnimation(anim *jsAnimation) {
	anim.playState = "idle"
	anim.cancelled = true
	anim.pausedAt = -1
	_ = anim.obj.Set("__playState__", "idle")
	r.clearAnimatedStyle(anim)
}

func (r *Runtime) finishAnimation(anim *jsAnimation) {
	if anim.playState == "idle" || anim.playState == "finished" {
		return
	}
	anim.playState = "finished"
	_ = anim.obj.Set("__playState__", "finished")
	r.applyAnimationAtOffset(anim, animFinalOffset(anim))
	if anim.finished != nil {
		r.resolvePromise(anim.finished, anim.obj)
	}
	r.fireOnFinish(anim)
}

func (r *Runtime) reverseAnimation(anim *jsAnimation) {
	anim.playbackRate = -anim.playbackRate
	_ = anim.obj.Set("__playbackRate__", anim.playbackRate)
	if anim.playState == "running" {
		elapsed := time.Since(anim.startTime)
		remaining := anim.duration - elapsed
		anim.startTime = time.Now().Add(-remaining)
		_ = anim.obj.Set("__startTimeMs__", float64(anim.startTime.UnixMilli()))
	}
}

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
			if anim.playState == "paused" || (anim.playState == "finished" &&
				(anim.fillMode == css.FillForwards || anim.fillMode == css.FillBoth)) {
				kept = append(kept, anim)
			}
			continue
		}

		elapsed := now.Sub(anim.startTime)
		if anim.playbackRate < 0 {
			elapsed = time.Duration(float64(elapsed) * anim.playbackRate)
		}

		if elapsed < anim.delay {
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

		totalProgress := float64(activeDuration) / float64(anim.duration)

		if anim.iterCount > 0 && totalProgress >= float64(anim.iterCount) {
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

		iteration := int(totalProgress)
		iterProgress := totalProgress - float64(iteration)

		offset := animApplyDirection(iterProgress, iteration, anim.direction)

		eased := anim.easing.Eval(float64(offset))
		offset = float32(eased)

		r.applyAnimationAtOffset(anim, offset)
		kept = append(kept, anim)
	}
	reg.anims = kept
}

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

func (r *Runtime) fireOnFinish(anim *jsAnimation) {
	if anim.onfinish != nil {
		if fn, ok := goja.AssertFunction(anim.onfinish); ok {
			event := r.vm.NewObject()
			_ = event.Set("type", "finish")
			_ = event.Set("target", anim.obj)
			_, _ = fn(anim.obj, event)
		}
	}
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

func (r *Runtime) installAnimationConstructor(reg *animRegistry) {
	_ = r.vm.Set("__reg__", reg)
	v, err := r.vm.RunString(`
	(function(reg) {
		function Animation(effect, timeline) {
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

func interpolateKeyframes(stops []css.KeyframeStop, property string, offset float32) string {
	if len(stops) == 0 {
		return ""
	}
	if len(stops) == 1 {
		return stops[0].Declarations[property]
	}

	if offset <= stops[0].Offset {
		return stops[0].Declarations[property]
	}
	if offset >= stops[len(stops)-1].Offset {
		return stops[len(stops)-1].Declarations[property]
	}

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

	span := float64(stops[hi].Offset - stops[lo].Offset)
	if span <= 0 {
		return toVal
	}
	segProgress := (float64(offset) - float64(stops[lo].Offset)) / span

	if fromColor, ok := css.ParseColor(fromVal); ok {
		if toColor, ok2 := css.ParseColor(toVal); ok2 {
			c := interpColor(fromColor, toColor, segProgress)
			return formatCSSColor(c)
		}
	}

	if fromNum, fromUnit, ok := parseCSSNumeric(fromVal); ok {
		if toNum, toUnit, ok2 := parseCSSNumeric(toVal); ok2 && toUnit == fromUnit {
			n := fromNum*(1-segProgress) + toNum*segProgress
			return formatCSSNumeric(n, fromUnit)
		}
	}

	if segProgress < 0.5 {
		return fromVal
	}
	return toVal
}

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

func formatCSSColor(c css.Color) string {
	if c.A == 255 {
		return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
	}
	alpha := float64(c.A) / 255.0
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", c.R, c.G, c.B, strconv.FormatFloat(alpha, 'f', 2, 64))
}

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

func formatCSSNumeric(v float64, unit string) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s + unit
}

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

func animIterJS(n int) interface{} {
	if n == 0 {
		return math.Inf(1)
	}
	return float64(n)
}

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

func mergeInlineStyle(style, prop, value string) string {
	if style == "" {
		return prop + ": " + value
	}

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

	result := strings.TrimRight(style, ";")
	return result + "; " + prop + ": " + value
}

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
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "; ")
}

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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

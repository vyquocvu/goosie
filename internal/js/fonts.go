package js

import (
	"strings"

	"github.com/dop251/goja"
)

// setupFonts installs document.fonts, the FontFaceSet subset pages use to
// probe and trigger webfont loads. The engine loads @font-face natively, so
// load() resolves once the (already handled) document faces are in: from a
// script's point of view every declared face is available.
//
// Only the settled shape is implemented, never a synchronous throw. Boot
// scripts chain .catch() off load() instead of try/catching it, so a sync
// TypeError/SyntaxError would kill the page where a rejection is handled.
// check() likewise answers false for an unparseable spec instead of throwing.
func (r *Runtime) setupFonts() {
	docVal := r.vm.Get("document")
	doc, ok := docVal.(*goja.Object)
	if !ok {
		return
	}
	fonts := r.vm.NewObject()
	var faces []goja.Value

	_ = fonts.Set("status", "loaded")
	_ = fonts.Set("onloading", goja.Null())
	_ = fonts.Set("onloadingdone", goja.Null())
	_ = fonts.Set("onloadingerror", goja.Null())

	_ = fonts.Set("load", func(call goja.FunctionCall) goja.Value {
		p := r.newPromiseObj()
		if len(call.Arguments) == 0 || !isStringValue(call.Arguments[0]) {
			r.rejectPromise(p, namedError(r, "SyntaxError", "FontFaceSet.load: font argument must be a CSS font shorthand string"))
			return p.obj
		}
		spec := strings.TrimSpace(call.Arguments[0].String())
		if !hasFontSizeToken(spec) {
			r.rejectPromise(p, namedError(r, "SyntaxError", "FontFaceSet.load: could not parse font shorthand"))
			return p.obj
		}
		// Faces load through the engine's @font-face path; nothing further
		// to wait on, so settle with the (empty) loaded-face list.
		r.resolvePromise(p, r.vm.NewArray())
		return p.obj
	})

	_ = fonts.Set("check", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || !isStringValue(call.Arguments[0]) {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(hasFontSizeToken(strings.TrimSpace(call.Arguments[0].String())))
	})

	ready := r.newPromiseObj()
	r.resolvePromise(ready, fonts)
	_ = fonts.Set("ready", ready.obj)

	_ = fonts.Set("add", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 && call.Arguments[0] != nil {
			v := call.Arguments[0]
			for _, f := range faces {
				if f.SameAs(v) {
					return fonts
				}
			}
			faces = append(faces, v)
		}
		return fonts
	})
	_ = fonts.Set("delete", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		v := call.Arguments[0]
		for i, f := range faces {
			if f.SameAs(v) {
				faces = append(faces[:i], faces[i+1:]...)
				return r.vm.ToValue(true)
			}
		}
		return r.vm.ToValue(false)
	})
	_ = fonts.Set("has", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		v := call.Arguments[0]
		for _, f := range faces {
			if f.SameAs(v) {
				return r.vm.ToValue(true)
			}
		}
		return r.vm.ToValue(false)
	})
	_ = fonts.Set("clear", func(call goja.FunctionCall) goja.Value {
		faces = nil
		return goja.Undefined()
	})

	_ = doc.Set("fonts", fonts)
}

// namedError builds a plain error object carrying the spec exception name,
// the same shape quotaExceededError uses: callers assert on .name and there
// is no DOMException hierarchy yet.
func namedError(r *Runtime, name, message string) goja.Value {
	e := r.vm.NewObject()
	_ = e.Set("name", name)
	_ = e.Set("message", message)
	return e
}

func isStringValue(v goja.Value) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	_, ok := v.Export().(string)
	return ok
}

// hasFontSizeToken reports whether spec contains a plausible CSS font-size
// token: a number with an optional length unit. This is intentionally
// lenient - "400 10pt X", "italic bold 16px/1.5 Y" and "small-caps 700 2em Z"
// all pass - and exists only to reject obvious garbage ("not-a-font") with a
// SyntaxError the way the CSS parser would.
func hasFontSizeToken(spec string) bool {
	for i := 0; i < len(spec); i++ {
		if spec[i] < '0' || spec[i] > '9' {
			continue
		}
		j := i
		for j < len(spec) && spec[j] >= '0' && spec[j] <= '9' {
			j++
		}
		if j < len(spec) && spec[j] == '.' {
			k := j + 1
			for k < len(spec) && spec[k] >= '0' && spec[k] <= '9' {
				k++
			}
			if k > j+1 {
				j = k
			}
		}
		// A bare number only counts as a size when a length unit follows
		// (otherwise it is a weight like 400, which alone is not a font).
		k := j
		for k < len(spec) && ((spec[k] >= 'a' && spec[k] <= 'z') || (spec[k] >= 'A' && spec[k] <= 'Z') || spec[k] == '%') {
			k++
		}
		if k > j {
			return true
		}
		i = j
	}
	return false
}

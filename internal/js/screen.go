package js

import "github.com/dop251/goja"

// setupScreen installs window.screen. Pages feature-detect it —
// `typeof screen === 'undefined'` guards are common enough that its absence
// aborts a script mid-file with a ReferenceError and everything after it never
// runs (GOOSIE-SCREEN-001, measured on a live site's boot script).
//
// Every value comes from the viewport the engine configured, which is the same
// single source innerWidth/innerHeight read, so the two interfaces cannot
// disagree and neither reads a host display. That is deliberate: a headless run
// has no display, and a number borrowed from the machine it happens to run on
// would make screenshots and parity scores irreproducible. availWidth and
// availHeight equal the configured size because there is no menu bar or dock to
// subtract — the honest form of "no chrome is modelled", not a guess at one.
//
// colorDepth/pixelDepth are the 24-bit desktop value rather than a measurement;
// nothing in the engine renders at a different depth, so nothing can contradict
// it. screen.orientation is deliberately absent: an angle and a type both
// derive from a device's *natural* orientation, which a windowed browser with no
// host display cannot know, and a pair invented from the window's aspect ratio
// would be wrong in the specific way that misleads a page — Chromium reports the
// display, not the window. Leaving it absent is the detectable failure; a value
// that looked authoritative would not be.
func (r *Runtime) setupScreen(win *goja.Object) {
	sc := r.vm.NewObject()
	// Two names per axis because avail* is the same number here; index selects
	// which half of ViewportSize the accessor reports.
	for _, p := range []struct {
		name  string
		index int
	}{
		{"width", 0}, {"availWidth", 0}, {"height", 1}, {"availHeight", 1},
	} {
		index := p.index
		_ = sc.DefineAccessorProperty(p.name, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			w, h := r.ViewportSize()
			if index == 0 {
				return r.vm.ToValue(w)
			}
			return r.vm.ToValue(h)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	_ = sc.Set("colorDepth", 24)
	_ = sc.Set("pixelDepth", 24)
	_ = win.Set("screen", sc)
}

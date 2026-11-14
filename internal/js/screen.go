package js

import "github.com/dop251/goja"

func (r *Runtime) setupScreen(win *goja.Object) {
	sc := r.vm.NewObject()
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

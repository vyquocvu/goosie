package js

import (
	"math"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) jsElementsFromPoint(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 2 {
		return newJSArray(r.vm, nil)
	}
	x, y := call.Arguments[0].ToFloat(), call.Arguments[1].ToFloat()
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) {
		return newJSArray(r.vm, nil)
	}
	provider := r.layoutGeometryProvider()
	if provider == nil {
		return newJSArray(r.vm, nil)
	}
	var hits []*dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() && n.HasAttribute("inert") {
			return
		}
		// Closed popovers and their contents are not rendered hit-test targets,
		// even if a stale or simplified layout provider still returns boxes.
		if n.Element() && n.HasAttribute("popover") && !n.IsPopoverOpen {
			return
		}
		if n.Element() {
			box, ok := provider(n)
			if ok && box.Width > 0 && box.Height > 0 && x >= box.X && y >= box.Y && x < box.X+box.Width && y < box.Y+box.Height {
				hits = append(hits, n)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.ShadowRoot != nil {
			for child := n.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	walk(&r.domDoc.Node)
	values := make([]goja.Value, len(hits))
	for i := range hits {
		values[i] = r.wrapNode(hits[len(hits)-1-i])
	}
	return newJSArray(r.vm, values)
}

func (r *Runtime) jsElementFromPoint(call goja.FunctionCall) goja.Value {
	hits := r.jsElementsFromPoint(call)
	array, ok := hits.(*goja.Object)
	if !ok || array.Get("length").ToInteger() == 0 {
		return goja.Null()
	}
	return array.Get("0")
}

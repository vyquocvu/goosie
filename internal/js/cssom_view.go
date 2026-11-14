package js

import (
	"fmt"
	"math"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type LayoutBoxInfo struct {
	X, Y                                                 float64
	Width, Height                                        float64
	BorderTop, BorderRight, BorderBottom, BorderLeft     float64
	PaddingTop, PaddingRight, PaddingBottom, PaddingLeft float64
	MarginTop, MarginRight, MarginBottom, MarginLeft     float64
	ScrollWidth, ScrollHeight                            float64
	ScrollContainer                                      bool
}

type LayoutGeometryProvider func(node *dom.Node) (info LayoutBoxInfo, ok bool)

type OffsetParentProvider func(node *dom.Node) *dom.Node

func (r *Runtime) SetLayoutGeometryProvider(fn LayoutGeometryProvider) {
	r.mu.Lock()
	r.layoutGeometry = fn
	r.mu.Unlock()
}

func (r *Runtime) SetOffsetParentProvider(fn OffsetParentProvider) {
	r.mu.Lock()
	r.offsetParent = fn
	r.mu.Unlock()
}

func (r *Runtime) WindowScrollY() float64 {
	if r == nil {
		return 0
	}
	r.scrollMu.Lock()
	y := r.windowScrollY
	r.scrollMu.Unlock()
	return y
}

func (r *Runtime) SetWindowScrollX(x float64) {
	if r == nil {
		return
	}
	if x < 0 {
		x = 0
	}
	r.scrollMu.Lock()
	oldX := r.windowScrollX
	r.windowScrollX = x
	moved := x != oldX
	r.scrollMu.Unlock()
	if moved {
		r.QueueViewportScroll()
		if r.opts.OnViewportScroll != nil {
			r.opts.OnViewportScroll(x, r.windowScrollY)
		}
	}
}

func (r *Runtime) SetWindowScrollY(y float64) {
	if r == nil {
		return
	}
	if y < 0 {
		y = 0
	}
	r.scrollMu.Lock()
	oldY := r.windowScrollY
	r.windowScrollY = y
	moved := y != oldY
	r.scrollMu.Unlock()
	if moved {
		r.QueueViewportScroll()
		if r.opts.OnViewportScroll != nil {
			r.opts.OnViewportScroll(r.windowScrollX, y)
		}
	}
}

func (r *Runtime) QueueViewportScroll() {
	if r == nil || r.domDoc == nil {
		return
	}
	r.scrollMu.Lock()
	defer r.scrollMu.Unlock()
	r.queueScrollTarget(&r.domDoc.Node)
}

func (r *Runtime) queueScrollTarget(node *dom.Node) {
	if node == nil {
		return
	}
	if r.scrollQueued == nil {
		r.scrollQueued = make(map[*dom.Node]bool)
	}
	if !r.scrollQueued[node] {
		r.scrollQueued[node] = true
		r.scrollQueue = append(r.scrollQueue, node)
	}
}

func (r *Runtime) setWindowScrollX(x float64) {
	if r == nil {
		return
	}
	if x < 0 {
		x = 0
	}
	r.scrollMu.Lock()
	old := r.windowScrollX
	r.windowScrollX = x
	moved := x != old
	r.scrollMu.Unlock()
	if moved {
		r.QueueViewportScroll()
	}
}

func (r *Runtime) isDocumentScrollingElement(node *dom.Node) bool {
	if r == nil || r.domDoc == nil || node == nil {
		return false
	}
	if r.domDoc.QuirksMode {
		return node == documentBodyOf(r.domDoc)
	}
	return node == documentElementOf(r.domDoc)
}

func (r *Runtime) ancestorScrollOffset(node *dom.Node) (float64, float64) {
	if r == nil || node == nil {
		return 0, 0
	}
	var dx, dy float64
	styleFn := r.computedStyleProvider()
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if !parent.Element() || styleFn == nil {
			continue
		}
		style := styleFn(parent)
		if style == nil {
			continue
		}
		isScrollContainer := style["overflow"] == "scroll" || style["overflow"] == "auto" ||
			style["overflow-x"] == "scroll" || style["overflow-x"] == "auto" ||
			style["overflow-y"] == "scroll" || style["overflow-y"] == "auto"
		if !isScrollContainer {
			continue
		}
		dx += r.ElementScrollOffset(parent, "scrollLeft")
		dy += r.ElementScrollOffset(parent, "scrollTop")
	}
	return dx, dy
}

func (r *Runtime) layoutGeometryProvider() LayoutGeometryProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.layoutGeometry
}

func (r *Runtime) offsetParentProvider() OffsetParentProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.offsetParent
}

func (r *Runtime) installCSSOMView() {
	r.installDOMRectConstructor()
	r.installElementGeometryMethods()
	r.installElementScrollMethods()
	r.installWindowScrollMethods()
}

func (r *Runtime) installDOMRectConstructor() {
	_ = r.vm.Set("__makeDOMRect__", func(x, y, w, h float64) *goja.Object {
		return r.newDOMRect(x, y, w, h)
	})
	_, _ = r.vm.RunString(`
		function DOMRect(x, y, width, height) {
			if (!(this instanceof DOMRect)) {
				return __makeDOMRect__(Number(x)||0, Number(y)||0, Number(width)||0, Number(height)||0);
			}
			x = Number(x) || 0;
			y = Number(y) || 0;
			width = Number(width) || 0;
			height = Number(height) || 0;
			Object.defineProperties(this, {
				x: { value: x, writable: true, enumerable: true, configurable: true },
				y: { value: y, writable: true, enumerable: true, configurable: true },
				width: { value: width, writable: true, enumerable: true, configurable: true },
				height: { value: height, writable: true, enumerable: true, configurable: true },
				top: { value: Math.min(y, y + height), enumerable: true, configurable: true },
				right: { value: Math.max(x, x + width), enumerable: true, configurable: true },
				bottom: { value: Math.max(y, y + height), enumerable: true, configurable: true },
				left: { value: Math.min(x, x + width), enumerable: true, configurable: true }
			});
		}
		DOMRect.prototype.toJSON = function() {
			return { x: this.x, y: this.y, width: this.width, height: this.height,
			         top: this.top, right: this.right, bottom: this.bottom, left: this.left };
		};
		window.DOMRect = DOMRect;
	`)
	_ = r.vm.Set("__makeDOMRect__", nil)
}

func (r *Runtime) newDOMRect(x, y, w, h float64) *goja.Object {
	rect := r.vm.NewObject()
	_ = rect.Set("x", x)
	_ = rect.Set("y", y)
	_ = rect.Set("width", w)
	_ = rect.Set("height", h)
	_ = rect.Set("top", min64(y, y+h))
	_ = rect.Set("right", max64(x, x+w))
	_ = rect.Set("bottom", max64(y, y+h))
	_ = rect.Set("left", min64(x, x+w))
	return rect
}

func (r *Runtime) installElementGeometryMethods() {
	p := r.elementProto

	_ = p.Set("getBoundingClientRect", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.newDOMRect(0, 0, 0, 0)
		}
		provider := r.layoutGeometryProvider()
		if provider == nil {
			return r.newDOMRect(0, 0, 0, 0)
		}
		info, ok := provider(node)
		if !ok {
			return r.newDOMRect(0, 0, 0, 0)
		}
		adjX, adjY := r.ancestorScrollOffset(node)
		return r.newDOMRect(info.X-adjX, info.Y-adjY, info.Width, info.Height)
	})

	_ = p.Set("getClientRects", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.newDOMRectList(nil)
		}
		provider := r.layoutGeometryProvider()
		if provider == nil {
			return r.newDOMRectList(nil)
		}
		info, ok := provider(node)
		if !ok {
			return r.newDOMRectList(nil)
		}
		adjX, adjY := r.ancestorScrollOffset(node)
		rect := r.newDOMRect(info.X-adjX, info.Y-adjY, info.Width, info.Height)
		return r.newDOMRectList([]*goja.Object{rect})
	})

	_ = r.htmlElementProto.DefineAccessorProperty("offsetParent", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		provider := r.offsetParentProvider()
		if provider == nil {
			return goja.Null()
		}
		parent := provider(node)
		if parent == nil {
			return goja.Null()
		}
		return r.wrapNode(parent)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	for _, prop := range []string{"offsetTop", "offsetLeft"} {
		prop := prop
		_ = r.htmlElementProto.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			provider := r.layoutGeometryProvider()
			if provider == nil {
				return r.vm.ToValue(0)
			}
			info, ok := provider(node)
			if !ok {
				return r.vm.ToValue(0)
			}
			opProvider := r.offsetParentProvider()
			if opProvider != nil {
				parent := opProvider(node)
				if parent != nil {
					if parentInfo, pok := provider(parent); pok {
						if prop == "offsetTop" {
							return r.vm.ToValue(info.Y - parentInfo.Y)
						}
						return r.vm.ToValue(info.X - parentInfo.X)
					}
				}
			}
			if prop == "offsetTop" {
				return r.vm.ToValue(info.Y)
			}
			return r.vm.ToValue(info.X)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	for _, prop := range []string{"offsetWidth", "offsetHeight"} {
		prop := prop
		_ = r.htmlElementProto.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			provider := r.layoutGeometryProvider()
			if provider == nil {
				return r.vm.ToValue(0)
			}
			info, ok := provider(node)
			if !ok {
				return r.vm.ToValue(0)
			}
			if prop == "offsetWidth" {
				return r.vm.ToValue(info.Width)
			}
			return r.vm.ToValue(info.Height)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
}

func (r *Runtime) setElementScrollOffset(node *dom.Node, prop string, value float64) {
	max, scrollable, bounded := r.elementScrollBounds(node, prop)
	if bounded && !scrollable {
		return
	}
	if value < 0 {
		value = 0
	}
	if bounded && value > max {
		value = max
	}
	r.scrollMu.Lock()
	if r.scrollPositions == nil {
		r.scrollPositions = make(map[scrollKey]float64)
	}
	key := scrollKey{node: node, prop: prop}
	changed := r.scrollPositions[key] != value
	r.scrollPositions[key] = value
	if changed {
		r.queueScrollTarget(node)
	}
	r.scrollMu.Unlock()
}

func (r *Runtime) ElementScrollOffset(node *dom.Node, prop string) float64 {
	if r == nil || node == nil {
		return 0
	}
	r.scrollMu.Lock()
	v := r.scrollPositions[scrollKey{node: node, prop: prop}]
	r.scrollMu.Unlock()
	return v
}

func (r *Runtime) elementScrollBounds(node *dom.Node, prop string) (max float64, scrollable, bounded bool) {
	provider := r.layoutGeometryProvider()
	if provider == nil {
		return 0, false, false
	}
	info, ok := provider(node)
	if !ok {
		return 0, false, false
	}
	extent, client := info.ScrollHeight, info.Height-info.BorderTop-info.BorderBottom
	if prop == "scrollLeft" {
		extent, client = info.ScrollWidth, info.Width-info.BorderLeft-info.BorderRight
	}
	if extent <= 0 {
		extent = info.Height
		if prop == "scrollLeft" {
			extent = info.Width
		}
	}
	if client < 0 {
		client = 0
	}
	max = extent - client
	if max < 0 {
		max = 0
	}
	return max, info.ScrollContainer, true
}

func (r *Runtime) FlushScrollEvents() {
	if r == nil {
		return
	}
	r.scrollMu.Lock()
	due := r.scrollQueue
	r.scrollQueue = nil
	r.scrollQueued = make(map[*dom.Node]bool)
	r.scrollMu.Unlock()
	for _, node := range due {
		if node == nil {
			continue
		}
		viewport := r.domDoc != nil && node == &r.domDoc.Node
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent("scroll", viewport, false))
		r.dispatchWithWindowStep(node, dom.NewTrustedEvent("scrollend", viewport, false))
	}
}

func (r *Runtime) installElementScrollMethods() {
	p := r.htmlElementProto

	for _, prop := range []string{"scrollTop", "scrollLeft"} {
		prop := prop
		_ = p.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			if r.isDocumentScrollingElement(node) {
				if prop == "scrollTop" {
					return r.vm.ToValue(r.WindowScrollY())
				}
				r.scrollMu.Lock()
				x := r.windowScrollX
				r.scrollMu.Unlock()
				return r.vm.ToValue(x)
			}
			r.scrollMu.Lock()
			v := r.scrollPositions[scrollKey{node: node, prop: prop}]
			r.scrollMu.Unlock()
			return r.vm.ToValue(v)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			v := call.Argument(0).ToFloat()
			if v < 0 {
				v = 0
			}
			if r.isDocumentScrollingElement(node) {
				if prop == "scrollTop" {
					r.SetWindowScrollY(v)
				} else {
					r.setWindowScrollX(v)
				}
				return goja.Undefined()
			}
			r.setElementScrollOffset(node, prop, v)
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	for _, prop := range []string{"scrollWidth", "scrollHeight"} {
		prop := prop
		_ = p.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			provider := r.layoutGeometryProvider()
			if provider == nil {
				return r.vm.ToValue(0)
			}
			info, ok := provider(node)
			if !ok {
				return r.vm.ToValue(0)
			}
			if prop == "scrollWidth" {
				v := info.ScrollWidth
				if v <= 0 {
					v = info.Width
				}
				return r.vm.ToValue(v)
			}
			v := info.ScrollHeight
			if v <= 0 {
				v = info.Height
			}
			return r.vm.ToValue(v)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	for _, prop := range []string{"clientWidth", "clientHeight"} {
		prop := prop
		_ = p.DefineAccessorProperty(prop, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			provider := r.layoutGeometryProvider()
			if provider == nil {
				return r.vm.ToValue(0)
			}
			info, ok := provider(node)
			if !ok {
				return r.vm.ToValue(0)
			}
			if prop == "clientWidth" {
				v := info.Width - info.BorderLeft - info.BorderRight
				if v < 0 {
					v = 0
				}
				return r.vm.ToValue(v)
			}
			v := info.Height - info.BorderTop - info.BorderBottom
			if v < 0 {
				v = 0
			}
			return r.vm.ToValue(v)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	_ = p.Set("scrollIntoView", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.scrollElementIntoView(node, call)
		return goja.Undefined()
	})

	for _, method := range []string{"scrollTo", "scrollBy", "scroll"} {
		method := method
		_ = p.Set(method, func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			r.handleElementScroll(node, method, call)
			return goja.Undefined()
		})
	}
}

func (r *Runtime) handleElementScroll(node *dom.Node, method string, call goja.FunctionCall) {
	var dx, dy float64
	if len(call.Arguments) > 0 {
		arg := call.Arguments[0]
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			obj, isObj := arg.(*goja.Object)
			if isObj {
				if v := obj.Get("top"); v != nil && !goja.IsUndefined(v) {
					dy = v.ToFloat()
				}
				if v := obj.Get("left"); v != nil && !goja.IsUndefined(v) {
					dx = v.ToFloat()
				}
			} else {
				dx = arg.ToFloat()
				if len(call.Arguments) > 1 {
					dy = call.Arguments[1].ToFloat()
				}
			}
		}
	}

	if r.isDocumentScrollingElement(node) {
		if method == "scrollBy" {
			dy += r.WindowScrollY()
			r.scrollMu.Lock()
			dx += r.windowScrollX
			r.scrollMu.Unlock()
		}
		r.SetWindowScrollY(dy)
		r.setWindowScrollX(dx)
		return
	}
	if method == "scrollBy" {
		dx += r.elementScrollOffset(node, "scrollLeft")
		dy += r.elementScrollOffset(node, "scrollTop")
	}
	r.setElementScrollOffset(node, "scrollLeft", dx)
	r.setElementScrollOffset(node, "scrollTop", dy)
}

func (r *Runtime) scrollElementIntoView(node *dom.Node, call goja.FunctionCall) {
	block, inline := "start", "nearest"
	if len(call.Arguments) > 0 {
		arg := call.Arguments[0]
		if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			if obj, ok := arg.(*goja.Object); ok {
				if v := obj.Get("block"); v != nil && !goja.IsUndefined(v) {
					block = v.String()
				}
				if v := obj.Get("inline"); v != nil && !goja.IsUndefined(v) {
					inline = v.String()
				}
			} else if arg.ToBoolean() {
				block = "start"
				inline = "nearest"
			} else {
				block = "end"
				inline = "nearest"
			}
		}
	}

	provider := r.layoutGeometryProvider()
	if provider == nil {
		return
	}

	for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
		info, ok := provider(ancestor)
		if !ok || !info.ScrollContainer {
			continue
		}
		r.scrollAncestorToReveal(ancestor, node, provider, block, inline)
	}

	r.scrollViewportToReveal(node, provider, block, inline)
}

func (r *Runtime) scrollAncestorToReveal(ancestor, target *dom.Node, provider LayoutGeometryProvider, block, inline string) {
	aInfo, ok := provider(ancestor)
	if !ok {
		return
	}
	tInfo, ok := provider(target)
	if !ok {
		return
	}

	curTop := r.elementScrollOffset(ancestor, "scrollTop")
	curLeft := r.elementScrollOffset(ancestor, "scrollLeft")

	relY := tInfo.Y - aInfo.Y + curTop
	relX := tInfo.X - aInfo.X + curLeft

	contentTop := aInfo.BorderTop
	contentLeft := aInfo.BorderLeft
	clientH := aInfo.Height - aInfo.BorderTop - aInfo.BorderBottom
	clientW := aInfo.Width - aInfo.BorderLeft - aInfo.BorderRight

	newTop := scrollAxisOffset(relY, tInfo.Height, contentTop, clientH, block)
	newLeft := scrollAxisOffset(relX, tInfo.Width, contentLeft, clientW, inline)

	if newTop != curTop {
		r.setElementScrollOffset(ancestor, "scrollTop", newTop)
	}
	if newLeft != curLeft {
		r.setElementScrollOffset(ancestor, "scrollLeft", newLeft)
	}
}

func (r *Runtime) scrollViewportToReveal(target *dom.Node, provider LayoutGeometryProvider, block, inline string) {
	tInfo, ok := provider(target)
	if !ok {
		return
	}
	vw, vh := r.ViewportSize()
	curY := r.WindowScrollY()
	r.scrollMu.Lock()
	curX := r.windowScrollX
	r.scrollMu.Unlock()

	absY := tInfo.Y + curY
	absX := tInfo.X + curX

	newY := scrollAxisOffset(absY, tInfo.Height, 0, float64(vh), block)
	newX := scrollAxisOffset(absX, tInfo.Width, 0, float64(vw), inline)

	if newY != curY {
		r.SetWindowScrollY(newY)
	}
	if newX != curX {
		r.setWindowScrollX(newX)
	}
}

func scrollAxisOffset(elPos, elSize, contentOffset, clientSize float64, alignment string) float64 {
	viewStart := contentOffset
	viewEnd := contentOffset + clientSize
	elStart := elPos
	elEnd := elPos + elSize

	switch alignment {
	case "start":
		return elStart - viewStart
	case "center":
		return elStart - viewStart - (clientSize-elSize)/2
	case "end":
		return elEnd - viewEnd
	default:
		if elStart >= viewStart && elEnd <= viewEnd {
			return 0
		}
		if elStart < viewStart {
			return elStart - viewStart
		}
		return elEnd - viewEnd
	}
}

func (r *Runtime) elementScrollOffset(node *dom.Node, prop string) float64 {
	r.scrollMu.Lock()
	defer r.scrollMu.Unlock()
	return r.scrollPositions[scrollKey{node: node, prop: prop}]
}

func (r *Runtime) installWindowScrollMethods() {
	win := r.vm.GlobalObject()
	for prop, index := range map[string]int{"innerWidth": 0, "innerHeight": 1} {
		index := index
		_ = win.DefineAccessorProperty(prop, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			width, height := r.ViewportSize()
			if index == 0 {
				return r.vm.ToValue(width)
			}
			return r.vm.ToValue(height)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	_ = win.DefineAccessorProperty("devicePixelRatio", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		r.dprMu.Lock()
		v := r.devicePixelRatio
		r.dprMu.Unlock()
		return r.vm.ToValue(v)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	for _, prop := range []string{"scrollX", "pageXOffset"} {
		prop := prop
		_ = win.DefineAccessorProperty(prop, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			r.scrollMu.Lock()
			v := r.windowScrollX
			r.scrollMu.Unlock()
			return r.vm.ToValue(v)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	for _, prop := range []string{"scrollY", "pageYOffset"} {
		prop := prop
		_ = win.DefineAccessorProperty(prop, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			r.scrollMu.Lock()
			v := r.windowScrollY
			r.scrollMu.Unlock()
			return r.vm.ToValue(v)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	for _, method := range []string{"scrollTo", "scroll", "scrollBy"} {
		method := method
		_ = win.Set(method, func(call goja.FunctionCall) goja.Value {
			r.handleWindowScroll(method, call)
			return goja.Undefined()
		})
	}
}

func (r *Runtime) SetDevicePixelRatio(dpr float64) {
	if !(dpr > 0) || math.IsInf(dpr, 0) {
		return
	}
	r.dprMu.Lock()
	r.devicePixelRatio = dpr
	r.dprMu.Unlock()
}

func (r *Runtime) handleWindowScroll(method string, call goja.FunctionCall) {
	var dx, dy float64
	if len(call.Arguments) > 0 {
		arg := call.Arguments[0]
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			obj, isObj := arg.(*goja.Object)
			if isObj {
				if v := obj.Get("top"); v != nil && !goja.IsUndefined(v) {
					dy = v.ToFloat()
				}
				if v := obj.Get("left"); v != nil && !goja.IsUndefined(v) {
					dx = v.ToFloat()
				}
			} else {
				dx = arg.ToFloat()
				if len(call.Arguments) > 1 {
					dy = call.Arguments[1].ToFloat()
				}
			}
		}
	}

	r.setWindowScroll(dx, dy, method == "scrollBy")
}

func (r *Runtime) setWindowScroll(x, y float64, relative bool) {
	if r == nil {
		return
	}
	r.scrollMu.Lock()
	oldX, oldY := r.windowScrollX, r.windowScrollY
	if relative {
		x += oldX
		y += oldY
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	r.windowScrollX, r.windowScrollY = x, y
	moved := x != oldX || y != oldY
	r.scrollMu.Unlock()
	if moved {
		r.QueueViewportScroll()
		if r.opts.OnViewportScroll != nil {
			r.opts.OnViewportScroll(x, y)
		}
	}
}

func (r *Runtime) ScrollWindowBy(dx, dy float64) {
	r.setWindowScroll(dx, dy, true)
}

func (r *Runtime) newDOMRectList(rects []*goja.Object) *goja.Object {
	list := r.vm.NewObject()
	if rects == nil {
		rects = []*goja.Object{}
	}
	for i, rect := range rects {
		_ = list.Set(fmt.Sprintf("%d", i), rect)
	}
	_ = list.Set("length", len(rects))
	_ = list.Set("item", func(call goja.FunctionCall) goja.Value {
		idx := int(call.Argument(0).ToInteger())
		if idx < 0 || idx >= len(rects) {
			return goja.Null()
		}
		return rects[idx]
	})
	return list
}

type scrollKey struct {
	node *dom.Node
	prop string
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

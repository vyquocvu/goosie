package js

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// LayoutBoxInfo carries the geometry the CSSOM View APIs need for one DOM
// node. The engine fills it from the layout arena; the JS package cannot
// import internal/layout (archtest rules), so the struct is plain numbers.
type LayoutBoxInfo struct {
	// Border-box position in viewport (CSS pixel) coordinates.
	X, Y float64
	// Border-box dimensions (CSS pixels).
	Width, Height float64
	// Border thicknesses (CSS pixels).
	BorderTop, BorderRight, BorderBottom, BorderLeft float64
	// Padding thicknesses (CSS pixels).
	PaddingTop, PaddingRight, PaddingBottom, PaddingLeft float64
	// Margin thicknesses (CSS pixels).
	MarginTop, MarginRight, MarginBottom, MarginLeft float64
	// Scrollable content size (CSS pixels). Zero means "same as border box".
	ScrollWidth, ScrollHeight float64
}

// LayoutGeometryProvider returns the layout box info for one DOM node. It
// returns ok=false when the node has no box (no layout run, display:none,
// text node, etc.). The engine installs a closure over its arena.
type LayoutGeometryProvider func(node *dom.Node) (info LayoutBoxInfo, ok bool)

// OffsetParentProvider returns the offset parent for one DOM node (the nearest
// positioned ancestor), or nil when none exists.
type OffsetParentProvider func(node *dom.Node) *dom.Node

// SetLayoutGeometryProvider installs the engine's layout reader. The engine
// calls it before any script runs, alongside SetComputedStyleProvider.
func (r *Runtime) SetLayoutGeometryProvider(fn LayoutGeometryProvider) {
	r.mu.Lock()
	r.layoutGeometry = fn
	r.mu.Unlock()
}

// SetOffsetParentProvider installs the engine's offset-parent reader.
func (r *Runtime) SetOffsetParentProvider(fn OffsetParentProvider) {
	r.mu.Lock()
	r.offsetParent = fn
	r.mu.Unlock()
}

// WindowScrollY reports the current document viewport's vertical scroll
// offset. The engine uses it when moving focus to an off-screen control.
func (r *Runtime) WindowScrollY() float64 {
	if r == nil {
		return 0
	}
	r.scrollMu.Lock()
	y := r.windowScrollY
	r.scrollMu.Unlock()
	return y
}

// SetWindowScrollY updates the document viewport's vertical scroll offset.
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
	r.scrollMu.Unlock()
	if y != oldY {
		r.dispatchWindowScrollEvent()
	}
}

func (r *Runtime) dispatchWindowScrollEvent() {
	if r == nil || r.domDoc == nil {
		return
	}
	r.dispatchWithWindowStep(&r.domDoc.Node, dom.NewTrustedEvent("scroll", false, false))
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

// installCSSOMView installs the CSSOM View layout measurement APIs on
// Element.prototype and HTMLElement.prototype: getBoundingClientRect,
// getClientRects, offset properties, scroll properties, and scrollIntoView.
func (r *Runtime) installCSSOMView() {
	r.installDOMRectConstructor()
	r.installElementGeometryMethods()
	r.installElementScrollMethods()
	r.installWindowScrollMethods()
}

// installDOMRectConstructor publishes window.DOMRect and window.DOMRectList.
func (r *Runtime) installDOMRectConstructor() {
	// DOMRect is a simple data class. Pages construct it via new DOMRect(x,y,w,h)
	// or receive one from getBoundingClientRect().
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

// newDOMRect creates a DOMRect JS object with all eight properties.
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

// installElementGeometryMethods installs getBoundingClientRect, getClientRects,
// and the offset properties on Element.prototype.
func (r *Runtime) installElementGeometryMethods() {
	p := r.elementProto

	// getBoundingClientRect()
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
		return r.newDOMRect(info.X, info.Y, info.Width, info.Height)
	})

	// getClientRects()
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
		rect := r.newDOMRect(info.X, info.Y, info.Width, info.Height)
		return r.newDOMRectList([]*goja.Object{rect})
	})

	// Offset properties: offsetParent, offsetTop, offsetLeft, offsetWidth, offsetHeight
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
			// offsetTop/offsetLeft are relative to the offsetParent.
			// Without an offsetParent provider that gives geometry, we
			// return the absolute position (which is correct when
			// offsetParent is the viewport / null).
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

// installElementScrollMethods installs scrollTop, scrollLeft, scrollWidth,
// scrollHeight, and scrollIntoView on HTMLElement.prototype.
func (r *Runtime) installElementScrollMethods() {
	p := r.htmlElementProto

	// scrollTop and scrollLeft are get/set. They store per-element scroll
	// positions. Without a real scroll implementation, they are stored in a
	// per-node map on the runtime.
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
					r.scrollMu.Lock()
					r.windowScrollX = v
					r.scrollMu.Unlock()
				}
				return goja.Undefined()
			}
			r.scrollMu.Lock()
			if r.scrollPositions == nil {
				r.scrollPositions = make(map[scrollKey]float64)
			}
			r.scrollPositions[scrollKey{node: node, prop: prop}] = v
			r.scrollMu.Unlock()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	// scrollWidth and scrollHeight are read-only.
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

	// clientWidth and clientHeight: content area size (border-box minus borders).
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

	// scrollIntoView() — accepted but no-op visually (no scroll container
	// implementation yet). Must not throw.
	_ = p.Set("scrollIntoView", func(call goja.FunctionCall) goja.Value {
		// Accept any arguments (alignToTop, scrollIntoViewOptions) and do
		// nothing. The method exists so pages that call it do not throw.
		return goja.Undefined()
	})

	// scrollTo / scrollBy / scroll on the element itself.
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

// handleElementScroll processes element.scroll(), element.scrollTo(), and
// element.scrollBy() calls.
func (r *Runtime) handleElementScroll(node *dom.Node, method string, call goja.FunctionCall) {
	var dx, dy float64
	if len(call.Arguments) > 0 {
		arg := call.Arguments[0]
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			obj, isObj := arg.(*goja.Object)
			if isObj {
				// Options object: { top, left, behavior }
				if v := obj.Get("top"); v != nil && !goja.IsUndefined(v) {
					dy = v.ToFloat()
				}
				if v := obj.Get("left"); v != nil && !goja.IsUndefined(v) {
					dx = v.ToFloat()
				}
			} else {
				// Numeric arguments: scroll(x, y)
				dx = arg.ToFloat()
				if len(call.Arguments) > 1 {
					dy = call.Arguments[1].ToFloat()
				}
			}
		}
	}

	r.scrollMu.Lock()
	if r.scrollPositions == nil {
		r.scrollPositions = make(map[scrollKey]float64)
	}
	if method == "scrollBy" {
		r.scrollPositions[scrollKey{node: node, prop: "scrollLeft"}] += dx
		r.scrollPositions[scrollKey{node: node, prop: "scrollTop"}] += dy
	} else {
		// scrollTo / scroll: set absolute position
		r.scrollPositions[scrollKey{node: node, prop: "scrollLeft"}] = dx
		r.scrollPositions[scrollKey{node: node, prop: "scrollTop"}] = dy
	}
	// Clamp to non-negative
	if v := r.scrollPositions[scrollKey{node: node, prop: "scrollLeft"}]; v < 0 {
		r.scrollPositions[scrollKey{node: node, prop: "scrollLeft"}] = 0
	}
	if v := r.scrollPositions[scrollKey{node: node, prop: "scrollTop"}]; v < 0 {
		r.scrollPositions[scrollKey{node: node, prop: "scrollTop"}] = 0
	}
	r.scrollMu.Unlock()
}

// installWindowScrollMethods installs window.scroll(), window.scrollBy(),
// window.scrollTo(), and window.scrollX/scrollY/pageXOffset/pageYOffset.
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

	// window.scrollX / window.scrollY (and aliases pageXOffset / pageYOffset)
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

	// window.scrollTo, window.scroll, window.scrollBy
	for _, method := range []string{"scrollTo", "scroll", "scrollBy"} {
		method := method
		_ = win.Set(method, func(call goja.FunctionCall) goja.Value {
			r.handleWindowScroll(method, call)
			return goja.Undefined()
		})
	}
}

// handleWindowScroll processes window.scroll(), window.scrollTo(), and
// window.scrollBy() calls.
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

// setWindowScroll commits a document viewport position, either absolute or
// relative to the current offset, and dispatches scroll only when it moved.
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
	r.scrollMu.Unlock()
	if x != oldX || y != oldY {
		r.dispatchWindowScrollEvent()
	}
}

// ScrollWindowBy moves the document viewport by a wheel gesture's deltas. This is
// the default action a wheel event carries: the host calls it only when the page
// did not cancel the gesture. Clamping and the scroll event match the script-facing
// window.scrollBy(), which shares this path.
func (r *Runtime) ScrollWindowBy(dx, dy float64) {
	r.setWindowScroll(dx, dy, true)
}

// newDOMRectList builds a DOMRectList-shaped array-like object from a slice of
// DOMRect objects.
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

// scrollKey identifies one per-element scroll position.
type scrollKey struct {
	node *dom.Node
	prop string // "scrollTop" or "scrollLeft"
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

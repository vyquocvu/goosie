package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// window.getComputedStyle, and the document.defaultView pointer that leads to
// it.
//
// The property exists because jQuery reads it in three places, one of them on
// every `.css()` call. The lookup chain is the reason defaultView is part of
// this file:
//
//	Re = function (e) {
//	  var t = e.ownerDocument.defaultView;
//	  return t && t.opener || (t = C), t.getComputedStyle(e)
//	}
//	...
//	(n = n || Re(e)) && ("" !== (a = n.getPropertyValue(t) || n[t]))
//
// So a working `.css()` needs, in order: element.ownerDocument (node_api.go,
// and the same object as document), document.defaultView (this file),
// window.getComputedStyle (this file), and an object that answers both
// getPropertyValue("font-size") and the camelCase read styles.fontSize. Missing
// any one of them is a silent wrong answer rather than a thrown error, which is
// worse: the page keeps its inline styles and never reveals the content.
//
// What the object answers is limited to what this engine can name. It reports
// the cascade result the session resolved for that node, plus the used width and
// height layout produced for its box. It never derives a value: an
// unresolvable property reads back as the empty string, and a property the
// engine does not model is absent. That is the difference between a computed
// style and a guess, and it is the rule the task set for this API.

// ComputedStyleFunc renders one node's cascade result as CSS declaration
// values keyed by property name (kebab-case, exactly as CSS spells them). nil
// values and nil map mean "this engine has no answer for this node", which the
// binding reports as empty strings rather than inventing initials.
//
// The contract lives in internal/js because the import graph forbids this
// package from reaching internal/style or internal/engine; the engine supplies
// the concrete conversion, and the same shape the author writes in a style
// sheet is what comes back out.
type ComputedStyleFunc func(node *dom.Node) map[string]string

// SetComputedStyleProvider installs the engine's cascade reader. The engine
// calls it right after SetMutationBudget, before any script runs, so a document
// that reads a computed style in its first inline script gets the load-time
// cascade instead of an empty object. A nil provider leaves the values to the
// element's own declarations.
func (r *Runtime) SetComputedStyleProvider(fn ComputedStyleFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.computedStyle = fn
}

func (r *Runtime) computedStyleProvider() ComputedStyleFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.computedStyle
}

// computedValues is the value set getComputedStyle reports for one node: the
// cascade result where the engine has one, and on top of it the node's own
// style attribute.
//
// The inline declarations are merged second, and that ordering is a decision
// with a known cost. The cascade map is a snapshot of the last resolve, so for a
// property the page rewrote through `el.style` in the current turn it is the
// attribute that is true and the map that is stale - and jQuery's show/hide reads
// exactly those properties, so answering from the map would keep an element the
// script just hid painted. The cost is spelling: an inline colour comes back as
// the author wrote it ("#ee0000") rather than normalised to "rgb(238, 0, 0)",
// because internal/js may not import internal/css (archtest rules) and so cannot
// re-render it. The winner is right and the string is the page's own; a value
// the cascade resolved without an inline override is normalised by the engine,
// which is what test/engine/computed_style_render_test.go pins on both shapes.
func (r *Runtime) computedValues(node *dom.Node) map[string]string {
	out := map[string]string{}
	if fn := r.computedStyleProvider(); fn != nil {
		if got := fn(node); got != nil {
			for k, v := range got {
				out[k] = v
			}
		}
	}
	if node.Element() {
		for _, d := range parseInlineStyle(node.GetAttribute("style")) {
			out[d.name] = d.value
		}
	}
	return out
}

// jsGetComputedStyle implements window.getComputedStyle(elt, pseudoElt).
func (r *Runtime) jsGetComputedStyle(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'getComputedStyle' on 'Window': parameter 1 is not of type 'Element'."))
	}
	node := r.getNode(call.Argument(0))
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'getComputedStyle' on 'Window': parameter 1 is not of type 'Element'."))
	}
	// The second argument is accepted and ignored: a pseudo-element box is not a
	// node the layout tree carries, so there is nothing honest to report for it.
	// The returned object therefore holds the originating element's values and
	// says nothing about "::before" - recorded as unimplemented in
	// docs/tasks/TASK-047-DOM-JQUERY-APIS.md rather than silently half-right.
	values := r.computedValues(node)
	return r.computedStyleObject(values)
}

// computedStyleObject builds the CSSStyleDeclaration-shaped result: every
// resolved name as both its CSS spelling and its camelCase one, plus
// getPropertyValue. Both spellings are not decoration - jQuery tries
// `styles[camelCase]` first and only then `getPropertyValue`, and a page that
// finds neither reads "" and concludes the property is unset.
func (r *Runtime) computedStyleObject(values map[string]string) *goja.Object {
	obj := r.vm.NewObject()
	for name, value := range values {
		_ = obj.Set(name, value)
		if camel := camelCSSName(name); camel != "" && camel != name {
			_ = obj.Set(camel, value)
		}
	}
	_ = obj.Set("getPropertyValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.ToValue("")
		}
		name := cssNameFromJS(call.Arguments[0].String())
		if v, ok := values[name]; ok {
			return r.vm.ToValue(v)
		}
		return r.vm.ToValue("")
	})
	_ = obj.Set("getPropertyPriority", func(call goja.FunctionCall) goja.Value {
		// The cascade keeps no !important flag on the resolved value, so no
		// property can be reported as important. Answering "" is the honest
		// answer; answering "important" for everything would not be.
		return r.vm.ToValue("")
	})
	return obj
}

// camelCSSName turns a kebab-case property name into the camelCase form JS uses
// on a style object: font-size becomes fontSize. A custom property (--x) has no
// camel form and returns "".
func camelCSSName(name string) string {
	if name == "" || name[0] == '-' {
		return ""
	}
	var b []byte
	up := false
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' {
			up = true
			continue
		}
		if up {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			up = false
		}
		b = append(b, c)
	}
	return string(b)
}

// jsDefaultView installs document.defaultView. It is the window object itself,
// which is the same object the global scope is, so `document.defaultView ===
// window` and the pointer also carries addEventListener and getComputedStyle.
//
// Sizzle's setDocument reads it in its support pass -
// `(n = C.defaultView) && n.top !== n && (n.addEventListener ? ... : ...)` - and
// in a top-level browsing context that chain ends at `n.top !== n`, exactly as
// it does in a browser, because window.top is window. Reading defaultView as
// undefined instead takes a different branch of the library.
func (r *Runtime) jsDefaultView() {
	doc := r.docObj()
	_ = doc.DefineAccessorProperty("defaultView", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.GlobalObject()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

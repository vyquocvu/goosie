package js

import (
	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// DOMImplementation.createHTMLDocument, and the detached document it returns.
//
// This is not a member the sweep found a *page* reading. It is the member the
// library every one of those pages loads reads first, and the measurement is
// the engine's own: with the rest of this cluster in place, the real
// jquery-3.6.0.min.js (89,501 bytes) booting inside a page still stops at
//
//	TypeError: Cannot read property 'createHTMLDocument' of undefined or null
//
// because jQuery's load-time support probe is an unconditional statement:
//
//	y.createHTMLDocument = (
//	  ( _t = E.implementation.createHTMLDocument( "" ).body ).innerHTML
//	      = "<form></form><form></form>",
//	  2 === _t.childNodes.length
//	)
//
// and `S.parseHTML` reaches for it again on every non-single-tag fragment:
//
//	t || ( y.createHTMLDocument
//	      ? ( ( r = ( t = E.implementation.createHTMLDocument( "" ) )
//	            .createElement( "base" ) ).href = E.location.href,
//	         t.head.appendChild( r ) )
//	      : t = E )
//
// So until this file, `jQuery` was never defined at all on a page that inlines
// the library: the throw happens while jQuery installs itself, and the page's
// own `jQuery(function($){...})` then dies as "jQuery is not defined". Fixing
// the Node APIs without this one moves the wall from `createDocumentFragment`
// to `createHTMLDocument` and the content script still runs nothing - measured,
// see docs/tasks/TASK-047-DOM-JQUERY-APIS.md.
//
// Semantics are the standard's, kept small: createHTMLDocument(title) returns a
// new HTML document whose content is exactly the four nodes the spec builds -
// doctype, html, head with a title holding the argument, and an empty body. The
// document is detached: nothing about it is styled, laid out or painted, and it
// is not the runtime's `document`, so `getElementById` on it searches only its
// own two elements.

// setupDocumentImplementation installs document.implementation. Called from
// setupDOM after the document object exists.
func (r *Runtime) setupDocumentImplementation() {
	impl := r.vm.NewObject()
	_ = impl.Set("createHTMLDocument", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateHTMLDocument(call)
	})
	// DocumentType-ish reads the standard gives an implementation object:
	// jQuery never calls them and no measured page does, so they are absent
	// rather than stubbed - see the omission list in
	// docs/tasks/TASK-047-DOM-JQUERY-APIS.md.
	_ = r.docObj().Set("implementation", impl)
}

// jsCreateHTMLDocument implements document.implementation.createHTMLDocument(title).
//
// The spec builds the document by appending a doctype and the html/head/title/
// body skeleton, then putting the argument in the title as a text node. The
// skeleton is parsed because that is the tree the engine already knows how to
// allocate under its document caps; the title is then set through the text-node
// path rather than by splicing the argument into the source, because this
// parser reads <title> as raw text (internal/dom/parser.go's rawtext element
// set), so an argument containing "&lt;" would come back as the four characters
// "&lt;" instead of "<" - and an argument containing "</title>" would close the
// element and change the tree the caller asked for.
//
// The parse limits are the ones this package already applies to one innerHTML
// write (see js.go's fragment paths: 1024 nodes, depth 32): a detached document
// is a scratch tree, and a page asking it to hold more than a screen's worth of
// markup gets a null return rather than an unbounded allocation.
func (r *Runtime) jsCreateHTMLDocument(call goja.FunctionCall) goja.Value {
	title := ""
	if len(call.Arguments) > 0 {
		if v := call.Arguments[0]; v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			title = v.String()
		}
	}
	source := "<!DOCTYPE html><html><head><title></title></head><body></body></html>"
	doc, err := dom.ParseBounded(source, dom.ParseLimits{
		Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096,
	})
	if err != nil {
		return goja.Null()
	}
	if titleNode := firstTitle(doc); titleNode != nil && title != "" {
		setNodeText(titleNode, title)
	}
	return r.wrapDetachedDocument(doc)
}

// wrapDetachedDocument builds the JS object for a document that is not the
// runtime's own.
//
// The Node half comes from wrapNode, which is what makes an element created
// inside this document answer `el.ownerDocument === thatDocumentObject` by
// identity: wrapNode registers the node and caches the wrapper, so a later
// ownerDocument read returns this same object rather than a look-alike - the
// same identity rule the main document needed pre-seeded for Sizzle's `r != C`
// check.
//
// The document half is installed here, bound to THIS document rather than
// r.domDoc: createElement/createTextNode/createDocumentFragment allocate from the
// detached tree, getElementById/getElementsByTagName/getElementsByClassName
// search it, and documentElement/head/body/title read its four nodes.
// querySelector/querySelectorAll are copied from the node prototype path
// instead: they already resolve their root from the receiver, so they search the
// detached subtree unchanged.
func (r *Runtime) wrapDetachedDocument(doc *dom.Document) goja.Value {
	val := r.wrapNode(&doc.Node)
	obj := val.ToObject(r.vm)
	if obj == nil {
		return goja.Null()
	}
	_ = obj.Set("createElement", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		return r.wrapNode(doc.NewElement(call.Arguments[0].String()))
	})
	_ = obj.Set("createTextNode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		return r.wrapNode(doc.NewText(call.Arguments[0].String()))
	})
	_ = obj.Set("createComment", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		return r.wrapNode(doc.NewComment(call.Arguments[0].String()))
	})
	_ = obj.Set("createDocumentFragment", func(call goja.FunctionCall) goja.Value {
		return r.wrapNode(doc.NewDocumentFragment())
	})
	_ = obj.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Null()
		}
		node := doc.ElementByID(call.Arguments[0].String())
		if node == nil {
			return goja.Null()
		}
		return r.wrapNode(node)
	})
	_ = obj.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return newJSArray(r.vm, nil)
		}
		nodes := doc.ElementsByTagName(call.Arguments[0].String())
		wrapped := make([]goja.Value, len(nodes))
		for i, n := range nodes {
			wrapped[i] = r.wrapNode(n)
		}
		return newJSArray(r.vm, wrapped)
	})
	_ = obj.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return newJSArray(r.vm, nil)
		}
		nodes := doc.ElementsByClassName(call.Arguments[0].String())
		wrapped := make([]goja.Value, len(nodes))
		for i, n := range nodes {
			wrapped[i] = r.wrapNode(n)
		}
		return newJSArray(r.vm, wrapped)
	})
	for _, name := range []string{"querySelector", "querySelectorAll"} {
		if v := r.nodeProto.Get(name); v != nil {
			_ = obj.Set(name, v)
		}
	}
	for _, name := range []string{"appendChild", "insertBefore", "replaceChild", "cloneNode", "contains", "hasChildNodes"} {
		if v := r.nodeProto.Get(name); v != nil {
			_ = obj.Set(name, v)
		}
	}
	_ = obj.Set("documentElement", r.wrapNode(doc.HTML))
	if doc.Head != nil {
		_ = obj.Set("head", r.wrapNode(doc.Head))
	} else {
		_ = obj.Set("head", goja.Null())
	}
	if doc.Body != nil {
		_ = obj.Set("body", r.wrapNode(doc.Body))
	} else {
		_ = obj.Set("body", goja.Null())
	}
	// title is the text of the head's title element, which is the one node the
	// spec's createHTMLDocument steps put the argument in.
	titleNode := firstTitle(doc)
	_ = obj.DefineAccessorProperty("title", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if titleNode == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(titleNode.TextContent())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if titleNode != nil && len(call.Arguments) > 0 {
			setNodeText(titleNode, call.Arguments[0].String())
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	return val
}

// setNodeText replaces an element's children with one text node, which is what
// the standard's text-content setter does. internal/dom exposes the read but not
// the write, so the binding walks the children itself; this is the same shape as
// the main document's textContent setter in js.go.
func setNodeText(n *dom.Node, text string) {
	var old []*dom.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		old = append(old, c)
	}
	for _, c := range old {
		n.RemoveChild(c)
	}
	if text != "" {
		n.AppendChild(n.Doc.NewText(text))
	}
}

// firstTitle returns the document's head title element, or nil.
func firstTitle(doc *dom.Document) *dom.Node {
	if doc.Head == nil {
		return nil
	}
	for c := doc.Head.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "title" {
			return c
		}
	}
	return nil
}

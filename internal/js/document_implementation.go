package js

import (
	"strings"

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
// new HTML document with the doctype, html, head, and empty body the spec builds.
// A non-null title adds the title element and its text to the head. The
// document is detached: nothing about it is styled, laid out or painted, and it
// is not the runtime's `document`, so `getElementById` on it searches only its
// own two elements.

// setupDocumentImplementation installs document.implementation. Called from
// setupDOM after the document object exists.
func (r *Runtime) setupDocumentImplementation() {
	impl := r.vm.NewObject()
	r.installDOMImplementation(impl, r.domDoc)
	// DocumentType-ish reads the standard gives an implementation object:
	// jQuery never calls them and no measured page does, so they are absent
	// rather than stubbed - see the omission list in
	// docs/tasks/TASK-047-DOM-JQUERY-APIS.md.
	_ = r.docObj().Set("implementation", impl)
}

func (r *Runtime) installDOMImplementation(impl *goja.Object, owner *dom.Document) {
	_ = impl.Set("createHTMLDocument", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateHTMLDocument(call)
	})
	_ = impl.Set("createDocumentType", func(call goja.FunctionCall) goja.Value {
		return r.createDocumentType(call, owner)
	})
	_ = impl.Set("createDocument", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateXMLDocument(call, owner)
	})
}

func (r *Runtime) jsCreateXMLDocument(call goja.FunctionCall, owner *dom.Document) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'createDocument': 2 arguments required"))
	}
	stringArg := func(index int) string {
		value := call.Argument(index)
		if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
			return ""
		}
		return value.String()
	}
	namespace, qualifiedName := stringArg(0), stringArg(1)
	// The namespace is nullable, so Web IDL converts null to the empty
	// namespace. qualifiedName is also nullable, but undefined is converted by
	// DOMString to the literal "undefined" and therefore creates a root element.
	if len(call.Arguments) > 1 && goja.IsUndefined(call.Argument(1)) {
		qualifiedName = "undefined"
	}
	localName := qualifiedName
	if qualifiedName != "" {
		var exception string
		_, localName, exception = validateDOMElementQName(namespace, qualifiedName)
		if exception != "" {
			panic(r.domError(exception, "The namespace and qualified name are not valid for an element"))
		}
	}
	var doctype *dom.Node
	if len(call.Arguments) > 2 && !goja.IsNull(call.Argument(2)) && !goja.IsUndefined(call.Argument(2)) {
		doctype = r.getNode(call.Argument(2))
		if doctype == nil || doctype.Type != dom.NodeDoctype {
			panic(r.vm.NewTypeError("Failed to execute 'createDocument': parameter 3 is not of type 'DocumentType'"))
		}
	}
	created := dom.NewDocument()
	created.IsXML = true
	r.setDetachedDocumentURL(created, "about:blank")
	switch namespace {
	case "http://www.w3.org/1999/xhtml":
		created.ContentType = "application/xhtml+xml"
	case "http://www.w3.org/2000/svg":
		created.ContentType = "image/svg+xml"
	default:
		created.ContentType = "application/xml"
	}
	if doctype != nil {
		if doctype.Parent != nil {
			doctype.Parent.RemoveChild(doctype)
		}
		doctype.AdoptInto(created)
		created.Node.AppendChild(doctype)
		created.DocType = doctype
	}
	if qualifiedName != "" {
		element := created.NewElement(qualifiedName)
		element.DataAtom = dom.Lookup(localName)
		element.NamespaceURI = namespace
		switch namespace {
		case "http://www.w3.org/1999/xhtml":
			element.Namespace = dom.NSHTML
		case "http://www.w3.org/2000/svg":
			element.Namespace = dom.NSSVG
		case "http://www.w3.org/1998/Math/MathML":
			element.Namespace = dom.NSMathML
		default:
			element.Namespace = dom.NSOther
		}
		created.Node.AppendChild(element)
		created.HTML = nil
		created.Head = nil
		created.Body = nil
	}
	if owner != nil {
		created.ContentLanguage = owner.ContentLanguage
	}
	return r.wrapDetachedDocument(created)
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
	hasTitle := false
	if len(call.Arguments) > 0 {
		if v := call.Arguments[0]; v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			hasTitle = true
			title = v.String()
		}
	}
	source := "<!DOCTYPE html><html><head></head><body></body></html>"
	if hasTitle {
		source = "<!DOCTYPE html><html><head><title></title></head><body></body></html>"
	}
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
// search it, and documentElement/head/body/title read its live tree.
// querySelector/querySelectorAll are copied from the node prototype path
// instead: they already resolve their root from the receiver, so they search the
// detached subtree unchanged.
func (r *Runtime) wrapDetachedDocument(doc *dom.Document) goja.Value {
	return r.wrapDetachedDocumentWithRegistry(doc, nil)
}

func (r *Runtime) wrapDetachedDocumentWithRegistry(doc *dom.Document, registry *CustomElementRegistry) goja.Value {
	val := r.wrapNode(&doc.Node)
	obj := val.ToObject(r.vm)
	if obj == nil {
		return goja.Null()
	}
	r.installDetachedStyleSheets(obj, doc)
	_ = obj.DefineAccessorProperty("defaultView", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if frame := r.iframeForDocument(doc); frame != nil {
			return r.iframeWindow(frame)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	impl := r.vm.NewObject()
	r.installDOMImplementation(impl, doc)
	_ = obj.Set("implementation", impl)
	_ = obj.Set("createElement", func(call goja.FunctionCall) goja.Value {
		if frame := r.iframeForDocument(doc); frame != nil {
			if child := r.iframeChildRuntimes[frame]; child != nil {
				arguments := make([]goja.Value, len(call.Arguments))
				for i, argument := range call.Arguments {
					arguments[i] = child.importParentValue(r, argument)
				}
				created := child.jsCreateElementInDocument(goja.FunctionCall{Arguments: arguments}, doc, child.customElementRegistryForDocument(doc))
				return r.importChildValue(child, created, make(map[*goja.Object]*goja.Object), 0)
			}
		}
		defaultRegistry := registry
		if defaultRegistry == nil && r.iframeForDocument(doc) != nil {
			// An iframe's Document wrapper can be created while its initial
			// about:blank document is still current, then reused after navigation
			// installs the parsed document and its registry. Resolve the fallback
			// at call time so the stable wrapper does not retain a stale nil.
			defaultRegistry = r.customElementRegistryForDocument(doc)
		}
		return r.jsCreateElementInDocument(call, doc, defaultRegistry)
	})
	_ = obj.Set("createElementNS", func(call goja.FunctionCall) goja.Value {
		if frame := r.iframeForDocument(doc); frame != nil {
			if child := r.iframeChildRuntimes[frame]; child != nil {
				arguments := make([]goja.Value, len(call.Arguments))
				for i, argument := range call.Arguments {
					arguments[i] = child.importParentValue(r, argument)
				}
				created := child.jsCreateElementNSInDocument(goja.FunctionCall{Arguments: arguments}, doc)
				return r.importChildValue(child, created, make(map[*goja.Object]*goja.Object), 0)
			}
		}
		return r.jsCreateElementNSInDocument(call, doc)
	})
	_ = obj.Set("createAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateAttribute(call, doc)
	})
	_ = obj.Set("createAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateAttributeNS(call, doc)
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
	r.installCreateCDATASection(obj, doc)
	r.installCreateProcessingInstruction(obj, doc)
	_ = obj.Set("createDocumentFragment", func(call goja.FunctionCall) goja.Value {
		return r.wrapNode(doc.NewDocumentFragment())
	})
	_ = obj.Set("createRange", func(goja.FunctionCall) goja.Value {
		return r.jsCreateRangeForDocument(doc)
	})
	_ = obj.Set("importNode", func(call goja.FunctionCall) goja.Value {
		return r.jsImportNode(call, doc)
	})
	_ = obj.Set("adoptNode", func(call goja.FunctionCall) goja.Value {
		return r.jsAdoptNode(call, doc)
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
			return r.newLiveHTMLCollection(nil, "", "")
		}
		return r.newLiveHTMLCollection(&doc.Node, call.Arguments[0].String(), "")
	})
	_ = obj.Set("getElementsByTagNameNS", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return r.newLiveHTMLCollectionNS(nil, "", "")
		}
		return r.newLiveHTMLCollectionNS(&doc.Node, call.Arguments[0].String(), call.Arguments[1].String())
	})
	_ = obj.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.newLiveHTMLCollection(nil, "", "")
		}
		return r.newLiveHTMLCollection(&doc.Node, "", call.Arguments[0].String())
	})
	for _, name := range []string{"querySelector", "querySelectorAll"} {
		// TASK-052 installs the ParentNode query roots on Element.prototype;
		// Node.prototype is the fallback for a runtime without them.
		if v := r.elementProto.Get(name); v != nil {
			_ = obj.Set(name, v)
		} else if v := r.nodeProto.Get(name); v != nil {
			_ = obj.Set(name, v)
		}
	}
	for _, name := range []string{"appendChild", "insertBefore", "replaceChild", "cloneNode", "contains", "hasChildNodes"} {
		if v := r.nodeProto.Get(name); v != nil {
			_ = obj.Set(name, v)
		}
	}
	_ = obj.Set("open", func(call goja.FunctionCall) goja.Value {
		return r.documentOpenCall(doc, obj, call)
	})
	_ = obj.Set("write", func(call goja.FunctionCall) goja.Value {
		return r.writeToDocument(doc, obj, call, false)
	})
	_ = obj.Set("writeln", func(call goja.FunctionCall) goja.Value {
		return r.writeToDocument(doc, obj, call, true)
	})
	_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
		return r.documentClose(doc)
	})
	_ = obj.DefineAccessorProperty("documentElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(documentElementOf(doc))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	// Older XML DOM implementations exposed the document element under this
	// spelling; DOMParser WPT uses it for parsed XML documents.
	_ = obj.DefineAccessorProperty("rootElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(documentElementOf(doc))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("doctype", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(documentDoctypeOf(doc))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("head", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(documentChildOf(doc, "head"))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("body", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(documentBodyOf(doc))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		body := r.getNode(call.Argument(0))
		if body == nil || body.Type != dom.NodeElement || body.Namespace != dom.NSHTML || body.Data != "body" && body.Data != "frameset" {
			panic(r.vm.NewTypeError("The new body must be an HTMLBodyElement or HTMLFrameSetElement"))
		}
		root := documentElementOf(doc)
		if root == nil {
			panic(r.domError("HierarchyRequestError", "The document has no document element"))
		}
		old := documentBodyOf(doc)
		if old != nil {
			r.jsReplaceChild(goja.FunctionCall{This: r.wrapNode(root), Arguments: []goja.Value{call.Argument(0), r.wrapNode(old)}})
		} else {
			r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(root), Arguments: []goja.Value{call.Argument(0)}})
		}
		doc.Body = body
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_FALSE)
	// title is the text of the head's title element, which is the one node the
	// spec's createHTMLDocument steps put the argument in.
	_ = obj.DefineAccessorProperty("title", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		titleNode := firstTitle(doc)
		if titleNode == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(titleNode.TextContent())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		titleNode := firstTitle(doc)
		if titleNode == nil {
			head := documentChildOf(doc, "head")
			if head == nil {
				root := documentElementOf(doc)
				if root == nil {
					root = doc.NewElement("html")
					doc.Node.AppendChild(root)
					doc.HTML = root
				}
				head = doc.NewElement("head")
				r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(root), Arguments: []goja.Value{r.wrapNode(head)}})
				doc.Head = head
			}
			titleNode = doc.NewElement("title")
			r.jsAppendChild(goja.FunctionCall{This: r.wrapNode(head), Arguments: []goja.Value{r.wrapNode(titleNode)}})
		}
		r.setNodeTextContent(titleNode, call.Argument(0).String())
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	return val
}

func documentElementOf(doc *dom.Document) *dom.Node {
	if doc == nil {
		return nil
	}
	for child := doc.Node.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() {
			return child
		}
	}
	return nil
}

func documentChildOf(doc *dom.Document, name string) *dom.Node {
	root := documentElementOf(doc)
	if root == nil {
		return nil
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Element() && child.Data == name {
			return child
		}
	}
	return nil
}

func documentBodyOf(doc *dom.Document) *dom.Node {
	if body := documentChildOf(doc, "body"); body != nil {
		return body
	}
	return documentChildOf(doc, "frameset")
}

func documentDoctypeOf(doc *dom.Document) *dom.Node {
	if doc == nil {
		return nil
	}
	for child := doc.Node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == dom.NodeDoctype {
			return child
		}
	}
	return nil
}

func (r *Runtime) createDocumentType(call goja.FunctionCall, doc *dom.Document) goja.Value {
	if doc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	name := call.Arguments[0].String()
	node := doc.NewDoctype(name)
	node.DoctypeName = name
	if len(call.Arguments) > 1 {
		node.PublicID = call.Arguments[1].String()
	}
	if len(call.Arguments) > 2 {
		node.SystemID = call.Arguments[2].String()
	}
	return r.wrapNode(node)
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
	if doc == nil {
		return nil
	}
	head := documentChildOf(doc, "head")
	if head == nil {
		return nil
	}
	for c := head.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "title" {
			return c
		}
	}
	return nil
}

func (r *Runtime) installCreateCDATASection(document *goja.Object, doc *dom.Document) {
	_ = document.Set("createCDATASection", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'createCDATASection': 1 argument required"))
		}
		if doc == nil || !doc.IsXML {
			panic(r.domError("NotSupportedError", "createCDATASection is not supported on HTML documents"))
		}
		data := call.Arguments[0].String()
		if strings.Contains(data, "]]>") {
			panic(r.domError("InvalidCharacterError", "CDATA section data contains the CDATA section terminator"))
		}
		return r.wrapNode(doc.NewCDATASection(data))
	})
}

func (r *Runtime) installCreateProcessingInstruction(document *goja.Object, doc *dom.Document) {
	_ = document.Set("createProcessingInstruction", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(r.vm.NewTypeError("Failed to execute 'createProcessingInstruction': 2 arguments required"))
		}
		if doc == nil {
			panic(r.vm.NewTypeError("Failed to execute 'createProcessingInstruction': illegal invocation"))
		}
		target, data := call.Argument(0).String(), call.Argument(1).String()
		if !validXMLQName(target) {
			panic(r.domError("InvalidCharacterError", "The processing instruction target is not a valid XML name"))
		}
		if strings.Contains(data, "?>") {
			panic(r.domError("InvalidCharacterError", "Processing instruction data must not contain ?>"))
		}
		return r.wrapNode(doc.NewProcessingInstruction(target, data))
	})
}

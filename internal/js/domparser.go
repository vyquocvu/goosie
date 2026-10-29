package js

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// DOMParser and XMLSerializer bindings.
//
// DOMParser.parseFromString(str, type) parses an HTML or XML string into a
// detached Document. Supported types: "text/html", "application/xhtml+xml",
// "text/xml", "application/xml", "image/svg+xml".
//
// XMLSerializer.serializeToString(node) serializes a DOM node subtree to an
// XML string.

// setupDOMParser installs the DOMParser constructor on the global scope.
func (r *Runtime) setupDOMParser() {
	r.installDOMParserConstructor(r.vm.GlobalObject(), func() (*dom.Document, string) {
		if documentURL := r.documentURL(); documentURL != nil {
			return r.domDoc, documentURL.String()
		}
		return r.domDoc, ""
	})
}

func (r *Runtime) installDOMParserConstructor(target *goja.Object, parserContext func() (*dom.Document, string)) {
	proto := r.vm.NewObject()
	_ = proto.Set("parseFromString", func(call goja.FunctionCall) goja.Value {
		return r.jsParseFromString(call, r.domParserContextURL(call.This))
	})
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		_ = obj.SetPrototype(proto)
		associatedDocument, associatedURL := parserContext()
		_ = obj.DefineDataProperty("__domParserContextURL", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			currentDocument, currentURL := parserContext()
			if currentDocument != associatedDocument {
				return r.vm.ToValue(associatedURL)
			}
			return r.vm.ToValue(currentURL)
		}), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return obj
	})
	if constructor, ok := ctor.(*goja.Object); ok {
		_ = constructor.Set("prototype", proto)
		_ = proto.Set("constructor", constructor)
		_ = target.Set("DOMParser", constructor)
	}
}

func (r *Runtime) domParserContextURL(receiver goja.Value) string {
	obj, ok := receiver.(*goja.Object)
	if !ok {
		return ""
	}
	value := obj.Get("__domParserContextURL")
	contextGetter, ok := goja.AssertFunction(value)
	if !ok {
		return ""
	}
	result, err := contextGetter(goja.Undefined(), goja.Undefined())
	if err != nil || result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return ""
	}
	return result.String()
}

// setupXMLSerializer installs the XMLSerializer constructor on the global scope.
func (r *Runtime) setupXMLSerializer() {
	win := r.vm.GlobalObject()
	proto := r.vm.NewObject()
	_ = proto.Set("serializeToString", func(call goja.FunctionCall) goja.Value {
		return r.jsSerializeToString(call)
	})
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		_ = obj.SetPrototype(proto)
		return obj
	})
	_ = ctor.ToObject(r.vm).Set("prototype", proto)
	_ = win.Set("XMLSerializer", ctor)
}

// jsParseFromString implements DOMParser.parseFromString(str, type).
func (r *Runtime) jsParseFromString(call goja.FunctionCall, contextURL string) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'parseFromString': 2 arguments required"))
	}
	str := call.Arguments[0].String()
	mime := call.Arguments[1].String()

	switch mime {
	case "text/html":
		return r.parseHTMLString(str, contextURL)
	case "application/xhtml+xml", "text/xml", "application/xml", "image/svg+xml":
		return r.parseXMLString(str, mime, contextURL)
	default:
		panic(r.vm.NewTypeError("Failed to execute 'parseFromString': unsupported type '%s'", mime))
	}
}

// parseHTMLString parses an HTML string into a detached Document using the
// existing HTML parser.
func (r *Runtime) parseHTMLString(src string, contextURL string) goja.Value {
	doc, err := dom.ParseBoundedWithHandlersAndScripting(src, dom.ParseLimits{
		Nodes: 4096, Depth: 64, Attributes: 256, AttributeBytes: 8192,
	}, false, nil, nil, nil)
	if err != nil {
		// On parse failure, return an error document with a parsererror element.
		doc = dom.NewDocument()
		doc.CharacterSet = "UTF-8"
		doc.ContentType = "text/html"
		root := doc.NewElement("parsererror")
		root.AppendChild(doc.NewText(err.Error()))
		doc.Node.AppendChild(root)
		doc.HTML = root
	}
	doc.CharacterSet = "UTF-8"
	doc.ContentType = "text/html"
	markParsedScriptsStarted(&doc.Node)
	r.queueInitialDetailsToggleEventsForDocument(doc)
	r.setDetachedDocumentURL(doc, contextURL)
	return r.wrapDetachedDocument(doc)
}

// parseXMLString parses an XML string into a detached Document using a basic
// XML parser.
func (r *Runtime) parseXMLString(src string, mime string, contextURL string) goja.Value {
	doc, err := ParseXMLDocumentBounded(src, mime, dom.ParseLimits{
		Nodes: 4096, Depth: 64, Attributes: 256, AttributeBytes: 8192,
	})
	if err != nil {
		doc = xmlParserErrorDocument(mime, err)
	}
	doc.IsDOMParserDocument = true
	markParsedScriptsStarted(&doc.Node)
	r.setDetachedDocumentURL(doc, contextURL)
	return r.wrapDetachedDocument(doc)
}

// ParseXMLDocumentBounded parses an XML document into the engine DOM. It is
// exported for document loaders that receive an XML MIME type; unlike
// DOMParser, the returned document is active and its scripts have not been
// marked inert. Limits are checked during a strict XML tokenization pass,
// before the DOM tree is allocated.
func ParseXMLDocumentBounded(src string, mime string, limits dom.ParseLimits) (*dom.Document, error) {
	return ParseXMLDocumentBoundedWithParserCallbacks(src, mime, limits, nil, nil, nil)
}

// ParseXMLDocumentBoundedWithParserCallbacks runs hooks while an active XML
// document is being parsed. DOMParser uses the callback-free entry point.
func ParseXMLDocumentBoundedWithParserCallbacks(src string, mime string, limits dom.ParseLimits,
	onElement func(*dom.Node, []dom.Attribute), onInserted func(*dom.Node), onScript func(*dom.Document, *dom.Node),
) (*dom.Document, error) {
	doc := dom.NewDocument()
	if err := ParseXMLDocumentIntoBoundedWithParserCallbacks(doc, src, mime, limits, onElement, onInserted, onScript); err != nil {
		return nil, err
	}
	return doc, nil
}

// ParseXMLDocumentIntoBoundedWithParserCallbacks fills an existing active XML
// document so a browsing context can bind its runtime before parsing begins.
func ParseXMLDocumentIntoBoundedWithParserCallbacks(doc *dom.Document, src string, mime string, limits dom.ParseLimits,
	onElement func(*dom.Node, []dom.Attribute), onInserted func(*dom.Node), onScript func(*dom.Document, *dom.Node),
) error {
	if doc == nil {
		return fmt.Errorf("XML document is nil")
	}
	if limits.Nodes <= 0 || limits.Depth <= 0 || limits.Attributes <= 0 || limits.AttributeBytes <= 0 {
		return fmt.Errorf("XML parse limits must all be positive")
	}
	doc.IsXML = true
	doc.CharacterSet = "UTF-8"
	doc.ContentType = mime
	entities := parseXMLInternalEntities(src)
	if err := validateXMLDocumentWithLimits(src, entities, limits); err != nil {
		return err
	}
	nodes, err := parseXMLNodesWithCallbacks(src, doc, entities, onElement, onInserted, onScript)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		if n.Parent != &doc.Node {
			doc.Node.AppendChild(n)
		}
	}
	setDocShortcuts(doc)
	return nil
}

// ParseXMLFragmentBounded parses a well-formed XML fragment in the namespace
// context of context. HTML document wrappers in the XHTML namespace are
// stripped as required by the DOM Parsing contextual-fragment algorithm.
func ParseXMLFragmentBounded(context *dom.Node, src string, limits dom.ParseLimits) (*dom.Node, error) {
	if context == nil || !context.Element() {
		return nil, fmt.Errorf("XML fragment parsing requires an element context")
	}
	if len(src) > maxXMLFragmentBytes {
		return nil, fmt.Errorf("XML fragment input byte limit exceeded")
	}
	bindings := map[string]string{}
	var ancestors []*dom.Node
	for node := context; node != nil; node = node.Parent {
		ancestors = append(ancestors, node)
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		for _, attr := range ancestors[i].Attr {
			switch {
			case attr.Name == "xmlns":
				bindings[""] = attr.Value
			case strings.HasPrefix(attr.Name, "xmlns:"):
				bindings[strings.TrimPrefix(attr.Name, "xmlns:")] = attr.Value
			}
		}
	}
	if _, declared := bindings[""]; !declared && context.NamespaceURI != "" {
		bindings[""] = context.NamespaceURI
	}
	var wrapper strings.Builder
	wrapper.WriteString("<goosie-fragment")
	prefixes := make([]string, 0, len(bindings))
	for prefix := range bindings {
		if prefix != "xml" && prefix != "xmlns" {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if prefix == "" {
			wrapper.WriteString(` xmlns="`)
		} else {
			wrapper.WriteString(" xmlns:")
			wrapper.WriteString(prefix)
			wrapper.WriteString(`="`)
		}
		wrapper.WriteString(escapeXMLAttr(bindings[prefix]))
		wrapper.WriteByte('"')
	}
	wrapper.WriteByte('>')
	wrapper.WriteString(src)
	wrapper.WriteString("</goosie-fragment>")
	doc, err := ParseXMLDocumentBounded(wrapper.String(), "application/xml", limits)
	if err != nil {
		return nil, err
	}
	parsedWrapper := doc.HTML
	if parsedWrapper == nil || parsedWrapper.Data != "goosie-fragment" {
		return nil, fmt.Errorf("XML fragment wrapper was not parsed")
	}
	fragment := doc.NewDocumentFragment()
	const htmlNamespace = "http://www.w3.org/1999/xhtml"
	var appendChildren func(*dom.Node)
	appendChildren = func(parent *dom.Node) {
		for child := parent.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Element() && child.NamespaceURI == htmlNamespace &&
				(child.Data == "html" || child.Data == "head" || child.Data == "body") {
				appendChildren(child)
			} else {
				fragment.AppendChild(child)
			}
			child = next
		}
	}
	appendChildren(parsedWrapper)
	return fragment, nil
}

const maxXMLFragmentBytes = 1 << 20

func xmlParserErrorDocument(mime string, err error) *dom.Document {
	doc := dom.NewDocument()
	doc.IsXML = true
	doc.CharacterSet = "UTF-8"
	doc.ContentType = mime
	root := doc.NewElement("parsererror")
	root.Namespace = dom.NSOther
	root.NamespaceURI = "http://www.mozilla.org/newlayout/xml/parsererror.xml"
	root.AppendChild(doc.NewText(err.Error()))
	doc.Node.AppendChild(root)
	doc.HTML = root
	return doc
}

func markParsedScriptsStarted(root *dom.Node) {
	if root == nil {
		return
	}
	if root.Type == dom.NodeElement && root.Data == "script" {
		root.ScriptAlreadyStarted = true
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		markParsedScriptsStarted(child)
	}
}

func (r *Runtime) setDetachedDocumentURL(doc *dom.Document, raw string) {
	if doc == nil {
		return
	}
	if r.detachedDocumentURLs == nil {
		r.detachedDocumentURLs = make(map[*dom.Document]*url.URL)
	}
	if raw == "" {
		if current := r.documentURL(); current != nil {
			copyURL := *current
			r.detachedDocumentURLs[doc] = &copyURL
		}
		return
	}
	parsed, err := url.Parse(raw)
	if err == nil {
		r.detachedDocumentURLs[doc] = parsed
	}
}

// setDocShortcuts walks the top level of doc to find html/head/body.
// For XML documents the root element may not be named "html", so we also
// record the first element found as doc.HTML (documentElement).
func setDocShortcuts(doc *dom.Document) {
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() {
			lower := strings.ToLower(n.Data)
			if doc.HTML == nil {
				// First element becomes documentElement regardless of name.
				doc.HTML = n
			}
			if doc.Head == nil && lower == "head" {
				doc.Head = n
			}
			if doc.Body == nil && lower == "body" {
				doc.Body = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
}

// jsSerializeToString implements XMLSerializer.serializeToString(node).
func (r *Runtime) jsSerializeToString(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'serializeToString': 1 argument required"))
	}
	if object, ok := call.Argument(0).(*goja.Object); ok {
		if marker := object.Get("__xmlAttributeNode"); marker != nil && marker.ToBoolean() {
			return r.vm.ToValue("")
		}
	}
	node := r.getNode(call.Arguments[0])
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'serializeToString': the node is not valid"))
	}
	if err := validateXMLSerializationTree(node); err != nil {
		panic(r.domError("InvalidStateError", err.Error()))
	}
	var sb strings.Builder
	nextPrefix := 1
	ctx := xmlNamespaceContext{
		prefixes:     map[string]string{"xml": "http://www.w3.org/XML/1998/namespace"},
		prefixForURI: map[string]string{"http://www.w3.org/XML/1998/namespace": "xml"},
		prefixOrder:  []string{"xml"},
		nextPrefix:   &nextPrefix,
	}
	serializeXMLNodeWithContext(&sb, node, ctx)
	return r.vm.ToValue(sb.String())
}

func (r *Runtime) jsCreateAttribute(call goja.FunctionCall, ownerDocuments ...*dom.Document) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'createAttribute': 1 argument required"))
	}
	name := strings.ToLower(call.Argument(0).String())
	return r.newJSAttribute(name, "", ownerDocuments...)
}

func (r *Runtime) jsCreateAttributeNS(call goja.FunctionCall, ownerDocuments ...*dom.Document) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'createAttributeNS': 2 arguments required"))
	}
	namespace := ""
	if value := call.Arguments[0]; value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
		namespace = value.String()
	}
	qualifiedName := call.Arguments[1].String()
	if _, _, exception := validateDOMElementQName(namespace, qualifiedName); exception != "" {
		panic(r.domError(exception, "The namespace and qualified name are not valid for an attribute"))
	}
	return r.newJSAttribute(qualifiedName, namespace, ownerDocuments...)
}

type xmlNamespaceContext struct {
	defaultURI   string
	prefixes     map[string]string
	prefixForURI map[string]string
	prefixOrder  []string
	nextPrefix   *int
}

func (c xmlNamespaceContext) clone() xmlNamespaceContext {
	copy := xmlNamespaceContext{
		defaultURI:   c.defaultURI,
		prefixes:     make(map[string]string, len(c.prefixes)),
		prefixForURI: make(map[string]string, len(c.prefixForURI)),
		prefixOrder:  append([]string(nil), c.prefixOrder...),
		nextPrefix:   c.nextPrefix,
	}
	for prefix, uri := range c.prefixes {
		copy.prefixes[prefix] = uri
	}
	for uri, prefix := range c.prefixForURI {
		copy.prefixForURI[uri] = prefix
	}
	return copy
}

// serializeXMLNode writes the XML serialization of node to sb.
func serializeXMLNode(sb *strings.Builder, n *dom.Node) {
	nextPrefix := 1
	serializeXMLNodeWithContext(sb, n, xmlNamespaceContext{
		prefixes:     map[string]string{"xml": "http://www.w3.org/XML/1998/namespace"},
		prefixForURI: map[string]string{"http://www.w3.org/XML/1998/namespace": "xml"},
		prefixOrder:  []string{"xml"},
		nextPrefix:   &nextPrefix,
	})
}

func serializeXMLChildren(node *dom.Node) (string, error) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := validateXMLSerializationTree(child); err != nil {
			return "", err
		}
	}
	var output strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		serializeXMLNode(&output, child)
	}
	return output.String(), nil
}

func validateXMLSerializationTree(root *dom.Node) error {
	var check func(*dom.Node) error
	check = func(current *dom.Node) error {
		if current == nil {
			return nil
		}
		if current.Type == dom.NodeElement {
			if !validXMLQName(current.Data) || current.Prefix == "" && strings.Contains(current.Data, ":") ||
				current.Prefix != "" && (!strings.HasPrefix(current.Data, current.Prefix+":") || current.NamespaceURI == "") {
				return fmt.Errorf("element local name %q is not serializable as XML", current.Data)
			}
			for _, attr := range current.Attr {
				if !validXMLQName(attr.Name) {
					return fmt.Errorf("attribute name %q is not serializable as XML", attr.Name)
				}
				for _, r := range attr.Value {
					if !isXMLCharacter(r) {
						return fmt.Errorf("attribute value contains a character outside the XML Char production")
					}
				}
			}
		}
		if current.Type == dom.NodeText || current.Type == dom.NodeCDATASection || current.Type == dom.NodeComment {
			for _, r := range current.DataContent {
				if !isXMLCharacter(r) {
					return fmt.Errorf("character data contains a character outside the XML Char production")
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			if err := check(child); err != nil {
				return err
			}
		}
		return nil
	}
	return check(root)
}

func serializeXMLNodeWithContext(sb *strings.Builder, n *dom.Node, ctx xmlNamespaceContext) {
	if n == nil {
		return
	}
	switch n.Type {
	case dom.NodeDocument:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			serializeXMLNodeWithContext(sb, c, ctx)
		}
	case dom.NodeElement:
		local := n.Data
		childCtx := ctx.clone()
		uri := xmlElementNamespace(n, ctx)
		// Apply explicit namespace declarations before resolving this element's
		// qualified name. XMLParser stores declarations as attributes, while
		// HTML's foreign-content parser records the namespace directly on nodes.
		for _, a := range n.Attr {
			if a.Name == "xmlns" {
				if a.Value == uri {
					childCtx.defaultURI = a.Value
				}
			} else if strings.HasPrefix(a.Name, "xmlns:") {
				prefix := strings.TrimPrefix(a.Name, "xmlns:")
				childCtx.prefixes[prefix] = a.Value
				childCtx.prefixForURI[a.Value] = prefix
				childCtx.prefixOrder = append(childCtx.prefixOrder, prefix)
			}
		}
		prefix := n.Prefix
		generated := make([][2]string, 0, 2)
		ensurePrefix := func(prefix, namespace string) string {
			if childCtx.prefixes[prefix] == namespace {
				return prefix
			}
			if xmlHasPrefixDeclaration(n, prefix, namespace) {
				childCtx.prefixes[prefix] = namespace
				return prefix
			}
			if xmlHasPrefixDeclaration(n, prefix, "") && !strings.HasPrefix(prefix, "ns") {
				for {
					candidate := nextXMLPrefix(childCtx)
					if _, exists := childCtx.prefixes[candidate]; !exists {
						prefix = candidate
						break
					}
				}
			}
			childCtx.prefixes[prefix] = namespace
			childCtx.prefixForURI[namespace] = prefix
			childCtx.prefixOrder = append(childCtx.prefixOrder, prefix)
			generated = append(generated, [2]string{prefix, namespace})
			return prefix
		}

		localName := local
		if prefix != "" && strings.HasPrefix(localName, prefix+":") {
			localName = strings.TrimPrefix(localName, prefix+":")
		}
		if uri == "http://www.w3.org/XML/1998/namespace" {
			prefix = "xml"
		} else if uri == ctx.defaultURI {
			prefix = ""
		} else if prefix != "" && childCtx.prefixes[prefix] == uri {
			// Keep the current prefix when its declaration agrees.
		} else if prefix != "" && xmlHasPrefixDeclaration(n, prefix, "") {
			prefix = ensurePrefix(prefix, uri)
		} else if closest := nearestXMLPrefix(childCtx, uri); closest != "" {
			prefix = closest
		} else if prefix != "" {
			prefix = ensurePrefix(prefix, uri)
		} else {
			if ctx.defaultURI != uri {
				generated = append(generated, [2]string{"", uri})
			}
			childCtx.defaultURI = uri
		}
		if prefix == "" {
			local = localName
		} else {
			local = prefix + ":" + localName
		}

		type serializedAttr struct {
			name, value  string
			declarations [][2]string
		}
		attrs := make([]serializedAttr, 0, len(n.Attr))
		hasEmptyPrefixBinding := false
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Name, "xmlns:") && a.Value == "" {
				hasEmptyPrefixBinding = true
				break
			}
		}
		for _, a := range n.Attr {
			generatedStart := len(generated)
			name := a.Name
			switch a.NamespaceURI {
			case "http://www.w3.org/2000/xmlns/":
				// Namespace declarations are emitted exactly as supplied.
			case "http://www.w3.org/XML/1998/namespace":
				if !strings.Contains(name, ":") {
					name = "xml:" + name
				}
			case "":
				if a.Namespace == dom.NSXML {
					name = "xml:" + strings.TrimPrefix(name, "xml:")
				}
			default:
				attrPrefix := ""
				attrLocal := name
				if colon := strings.IndexByte(name, ':'); colon >= 0 {
					attrPrefix, attrLocal = name[:colon], name[colon+1:]
				}
				if a.NamespaceURI == "http://www.w3.org/XML/1998/namespace" {
					name = "xml:" + attrLocal
				} else {
					inputPrefix := attrPrefix
					attrPrefix = nearestXMLPrefix(childCtx, a.NamespaceURI)
					if attrPrefix == "" && inputPrefix != "" && childCtx.prefixes[inputPrefix] == a.NamespaceURI {
						attrPrefix = inputPrefix
					}
					if attrPrefix == "" && inputPrefix != "" && a.NamespaceURI == "http://www.w3.org/1999/xlink" {
						attrPrefix = ensurePrefix(inputPrefix, a.NamespaceURI)
					}
					if attrPrefix == "" {
						candidate := nextXMLPrefix(childCtx)
						attrPrefix = ensurePrefix(candidate, a.NamespaceURI)
					}
					name = attrPrefix + ":" + attrLocal
				}
			}
			if a.Name == "xmlns" {
				// The element's namespace URI, rather than a contradictory
				// declaration, determines the serialization namespace.
				defaultGenerated := false
				for _, declaration := range generated {
					if declaration[0] == "" && declaration[1] == uri {
						defaultGenerated = true
						break
					}
				}
				if a.Value == uri && ctx.defaultURI != uri && !defaultGenerated {
					attrs = append(attrs, serializedAttr{name: "xmlns", value: a.Value})
				} else if a.Value == "" && uri == "" && hasEmptyPrefixBinding && ctx.defaultURI == "" {
					attrs = append(attrs, serializedAttr{name: "xmlns", value: ""})
				}
				continue
			}
			if a.NamespaceURI == "http://www.w3.org/2000/xmlns/" {
				declaredPrefix := strings.TrimPrefix(a.Name, "xmlns:")
				if boundURI, bound := ctx.prefixes[declaredPrefix]; bound && boundURI == a.Value {
					continue
				}
			}
			declarations := append([][2]string(nil), generated[generatedStart:]...)
			generated = generated[:generatedStart]
			attrs = append(attrs, serializedAttr{name: name, value: a.Value, declarations: declarations})
		}

		sb.WriteString("<")
		sb.WriteString(local)
		for _, declaration := range generated {
			if declaration[0] == "" {
				sb.WriteString(` xmlns="`)
			} else {
				sb.WriteString(" xmlns:")
				sb.WriteString(declaration[0])
				sb.WriteString(`="`)
			}
			sb.WriteString(escapeXMLAttr(declaration[1]))
			sb.WriteString(`"`)
		}
		for _, a := range attrs {
			writeXMLNamespaceDeclarations(sb, a.declarations)
			sb.WriteString(" ")
			sb.WriteString(a.name)
			sb.WriteString(`="`)
			sb.WriteString(escapeXMLAttr(a.value))
			sb.WriteString(`"`)
		}
		if n.FirstChild == nil {
			if uri == "http://www.w3.org/1999/xhtml" && isXMLSerializerHTMLVoid(localName) {
				sb.WriteString(" />")
			} else if uri != "http://www.w3.org/1999/xhtml" {
				sb.WriteString("/>")
			} else {
				sb.WriteString("></")
				sb.WriteString(local)
				sb.WriteString(">")
			}
			return
		}
		sb.WriteString(">")
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			serializeXMLNodeWithContext(sb, c, childCtx)
		}
		sb.WriteString("</")
		sb.WriteString(local)
		sb.WriteString(">")
	case dom.NodeText:
		sb.WriteString(escapeXMLText(n.DataContent))
	case dom.NodeCDATASection:
		sb.WriteString("<![CDATA[")
		sb.WriteString(n.DataContent)
		sb.WriteString("]]>")
	case dom.NodeProcessingInstruction:
		sb.WriteString("<?")
		sb.WriteString(n.Data)
		if n.DataContent == "" {
			sb.WriteByte(' ')
		} else {
			sb.WriteByte(' ')
			sb.WriteString(n.DataContent)
		}
		sb.WriteString("?>")
	case dom.NodeComment:
		sb.WriteString("<!--")
		sb.WriteString(n.DataContent)
		sb.WriteString("-->")
	case dom.NodeDoctype:
		sb.WriteString("<!DOCTYPE ")
		name := n.DoctypeName
		if name == "" {
			name = n.DataContent
		}
		sb.WriteString(name)
		if n.PublicID != "" {
			sb.WriteString(` PUBLIC "`)
			sb.WriteString(n.PublicID)
			sb.WriteString(`"`)
			if n.SystemID != "" {
				sb.WriteString(` "`)
				sb.WriteString(n.SystemID)
				sb.WriteString(`"`)
			}
		} else if n.SystemID != "" {
			sb.WriteString(` SYSTEM "`)
			sb.WriteString(n.SystemID)
			sb.WriteString(`"`)
		}
		sb.WriteString(">")
	case dom.NodeDocumentFragment:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			serializeXMLNodeWithContext(sb, c, ctx)
		}
	}
}

func writeXMLNamespaceDeclarations(sb *strings.Builder, declarations [][2]string) {
	for _, declaration := range declarations {
		if declaration[0] == "" {
			sb.WriteString(` xmlns="`)
		} else {
			sb.WriteString(" xmlns:")
			sb.WriteString(declaration[0])
			sb.WriteString(`="`)
		}
		sb.WriteString(escapeXMLAttr(declaration[1]))
		sb.WriteString(`"`)
	}
}

func xmlElementNamespace(n *dom.Node, ctx xmlNamespaceContext) string {
	if n.NamespaceURI != "" {
		return n.NamespaceURI
	}
	if n.Doc != nil && n.Doc.IsXML {
		if n.Namespace == dom.NSHTML && strings.EqualFold(n.Doc.ContentType, "application/xhtml+xml") {
			return "http://www.w3.org/1999/xhtml"
		}
		return ""
	}
	if n.Doc != nil && !n.Doc.IsXML {
		switch n.Namespace {
		case dom.NSHTML:
			return "http://www.w3.org/1999/xhtml"
		case dom.NSSVG:
			return "http://www.w3.org/2000/svg"
		case dom.NSMathML:
			return "http://www.w3.org/1998/Math/MathML"
		}
	}
	if colon := strings.IndexByte(n.Data, ':'); colon >= 0 {
		return ctx.prefixes[n.Data[:colon]]
	}
	return ctx.defaultURI
}

func nextXMLPrefix(ctx xmlNamespaceContext) string {
	if ctx.nextPrefix == nil {
		return "ns1"
	}
	value := *ctx.nextPrefix
	*ctx.nextPrefix = value + 1
	return "ns" + strconv.Itoa(value)
}

func nearestXMLPrefix(ctx xmlNamespaceContext, uri string) string {
	if prefix := ctx.prefixForURI[uri]; prefix != "" && prefix != "xml" {
		return prefix
	}
	for i := len(ctx.prefixOrder) - 1; i >= 0; i-- {
		prefix := ctx.prefixOrder[i]
		if prefix != "" && prefix != "xml" && ctx.prefixes[prefix] == uri {
			return prefix
		}
	}
	return ""
}

func xmlHasPrefixDeclaration(n *dom.Node, prefix, uri string) bool {
	name := "xmlns"
	if prefix != "" {
		name += ":" + prefix
	}
	for _, a := range n.Attr {
		if a.Name == name && (uri == "" || a.Value == uri) {
			return true
		}
	}
	return false
}

func isXMLSerializerHTMLVoid(name string) bool {
	switch strings.ToLower(name) {
	case "area", "base", "basefont", "bgsound", "br", "col", "embed", "frame", "hr", "img", "input", "keygen", "link", "menuitem", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

// escapeXMLText escapes text for XML content.
func escapeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// escapeXMLAttr escapes an attribute value for XML.
func escapeXMLAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `<`, "&lt;")
	s = strings.ReplaceAll(s, `>`, "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "\t", "&#9;")
	s = strings.ReplaceAll(s, "\n", "&#10;")
	s = strings.ReplaceAll(s, "\r", "&#13;")
	return s
}

// ---------------------------------------------------------------------------
// Basic XML parser
// ---------------------------------------------------------------------------

// xmlParser is a minimal XML parser that builds a DOM tree.
type xmlParser struct {
	input           string
	pos             int
	doc             *dom.Document
	namespaceScopes []map[string]string
	entities        map[string]string
	onElement       func(*dom.Node, []dom.Attribute)
	onInserted      func(*dom.Node)
	onScript        func(*dom.Document, *dom.Node)
}

// parseXMLNodes parses the XML string and returns a slice of top-level nodes.
func parseXMLNodes(src string, doc *dom.Document, entities map[string]string) ([]*dom.Node, error) {
	return parseXMLNodesWithCallbacks(src, doc, entities, nil, nil, nil)
}

func parseXMLNodesWithCallbacks(src string, doc *dom.Document, entities map[string]string,
	onElement func(*dom.Node, []dom.Attribute), onInserted func(*dom.Node), onScript func(*dom.Document, *dom.Node),
) ([]*dom.Node, error) {
	p := &xmlParser{
		input:      src,
		doc:        doc,
		entities:   entities,
		onElement:  onElement,
		onInserted: onInserted,
		onScript:   onScript,
		namespaceScopes: []map[string]string{{
			"xml":   "http://www.w3.org/XML/1998/namespace",
			"xmlns": "http://www.w3.org/2000/xmlns/",
		}},
	}
	var nodes []*dom.Node
	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}
		if p.input[p.pos] == '<' {
			if p.pos+1 < len(p.input) {
				switch p.input[p.pos+1] {
				case '?':
					node, err := p.parseProcessingInstruction()
					if err != nil {
						return nil, err
					}
					if node != nil {
						nodes = append(nodes, node)
					}
					continue
				case '!':
					if p.pos+3 < len(p.input) && p.input[p.pos+2] == '-' && p.input[p.pos+3] == '-' {
						// Comment.
						p.pos += 4
						end := strings.Index(p.input[p.pos:], "-->")
						if end >= 0 {
							data := p.input[p.pos : p.pos+end]
							nodes = append(nodes, doc.NewComment(data))
							p.pos += end + 3
						} else {
							p.pos = len(p.input)
						}
						continue
					}
					if p.pos+8 < len(p.input) && strings.HasPrefix(p.input[p.pos+2:], "DOCTYPE") {
						doctype, err := p.parseDoctype()
						if err != nil {
							return nil, err
						}
						nodes = append(nodes, doctype)
						continue
					}
					if strings.HasPrefix(p.input[p.pos+2:], "[CDATA[") {
						// CDATA section.
						p.pos += 9 // skip <![CDATA[
						end := strings.Index(p.input[p.pos:], "]]>")
						if end >= 0 {
							data := p.input[p.pos : p.pos+end]
							nodes = append(nodes, doc.NewCDATASection(data))
							p.pos += end + 3
						} else {
							p.pos = len(p.input)
						}
						continue
					}
					// Unknown <! construct, skip.
					p.pos += 2
					end := strings.Index(p.input[p.pos:], ">")
					if end >= 0 {
						p.pos += end + 1
					} else {
						p.pos = len(p.input)
					}
					continue
				case '/':
					return nil, fmt.Errorf("unexpected XML closing tag")
				default:
					// Preserve document order for comments, processing instructions,
					// and doctypes that precede a root parsed incrementally below.
					for _, pending := range nodes {
						if pending.Parent == nil {
							doc.Node.AppendChild(pending)
						}
					}
					n, err := p.parseElement(&doc.Node)
					if err != nil {
						return nil, err
					}
					if n != nil {
						nodes = append(nodes, n)
					}
				}
			} else {
				p.pos++
			}
		} else {
			// Text at top level.
			start := p.pos
			for p.pos < len(p.input) && p.input[p.pos] != '<' {
				p.pos++
			}
			text := p.input[start:p.pos]
			if strings.TrimSpace(text) != "" {
				nodes = append(nodes, doc.NewText(text))
			}
		}
	}
	for _, pending := range nodes {
		if pending.Parent == nil {
			doc.Node.AppendChild(pending)
		}
	}
	return nodes, nil
}

func (p *xmlParser) parseProcessingInstruction() (*dom.Node, error) {
	start := p.pos
	p.pos += 2
	contentStart := p.pos
	end := strings.Index(p.input[p.pos:], "?>")
	if end < 0 {
		return nil, fmt.Errorf("unterminated XML processing instruction")
	}
	content := p.input[contentStart : contentStart+end]
	p.pos = contentStart + end + 2
	targetEnd := strings.IndexAny(content, " \t\r\n")
	target, data := content, ""
	if targetEnd >= 0 {
		target, data = content[:targetEnd], strings.TrimLeft(content[targetEnd:], " \t\r\n")
	}
	if !validXMLQName(target) {
		return nil, fmt.Errorf("invalid XML processing instruction target %q", target)
	}
	// The XML declaration is a byte-stream declaration, not a DOM node.
	if start == 0 && strings.EqualFold(target, "xml") {
		return nil, nil
	}
	return p.doc.NewProcessingInstruction(target, data), nil
}

func (p *xmlParser) parseDoctype() (*dom.Node, error) {
	const opener = "<!DOCTYPE"
	p.pos += len(opener)
	start := p.pos
	quote := byte(0)
	subsetDepth := 0
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else {
			switch c {
			case '\'', '"':
				quote = c
			case '[':
				subsetDepth++
			case ']':
				if subsetDepth > 0 {
					subsetDepth--
				}
			case '>':
				if subsetDepth == 0 {
					body := strings.TrimSpace(p.input[start:p.pos])
					p.pos++
					return parseXMLDoctypeBody(body, p.doc)
				}
			}
		}
		p.pos++
	}
	return nil, fmt.Errorf("unterminated XML doctype")
}

func parseXMLDoctypeBody(body string, doc *dom.Document) (*dom.Node, error) {
	nameEnd := strings.IndexAny(body, " \t\r\n[")
	if nameEnd < 0 {
		nameEnd = len(body)
	}
	name := body[:nameEnd]
	if name == "" {
		return nil, fmt.Errorf("XML doctype is missing a name")
	}
	rest := strings.TrimSpace(body[nameEnd:])
	var publicID, systemID string
	if strings.HasPrefix(rest, "PUBLIC") {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "PUBLIC"))
		var ok bool
		publicID, rest, ok = takeXMLQuoted(rest)
		if !ok {
			return nil, fmt.Errorf("XML doctype PUBLIC identifier is malformed")
		}
		systemID, rest, ok = takeXMLQuoted(rest)
		if !ok {
			return nil, fmt.Errorf("XML doctype SYSTEM identifier is missing")
		}
	} else if strings.HasPrefix(rest, "SYSTEM") {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "SYSTEM"))
		var ok bool
		systemID, rest, ok = takeXMLQuoted(rest)
		if !ok {
			return nil, fmt.Errorf("XML doctype SYSTEM identifier is malformed")
		}
	}
	node := doc.NewDoctype(name)
	node.DoctypeName = name
	node.PublicID = publicID
	node.SystemID = systemID
	return node, nil
}

func takeXMLQuoted(src string) (value, rest string, ok bool) {
	src = strings.TrimSpace(src)
	if len(src) < 2 || src[0] != '\'' && src[0] != '"' {
		return "", src, false
	}
	quote := src[0]
	end := strings.IndexByte(src[1:], quote)
	if end < 0 {
		return "", src, false
	}
	end++
	return src[1:end], src[end+1:], true
}

// parseElement parses an element and its children.
func (p *xmlParser) parseElement(parent *dom.Node) (*dom.Node, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '<' {
		return nil, nil
	}
	p.pos++ // skip <

	// Read tag name.
	tagStart := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
			break
		}
		p.pos++
	}
	tag := p.input[tagStart:p.pos]
	if !validXMLQName(tag) {
		return nil, fmt.Errorf("invalid XML element name %q", tag)
	}

	n := p.doc.NewElement(tag)
	if colon := strings.IndexByte(tag, ':'); colon >= 0 {
		n.Prefix = tag[:colon]
	}
	selfClosing := false

	// Parse attributes.
	seenAttributes := make(map[string]bool)
	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}
		if p.input[p.pos] == '>' {
			p.pos++
			break
		}
		if p.input[p.pos] == '/' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '>' {
			p.pos += 2
			selfClosing = true
			break
		}
		// Parse attribute.
		name, value := p.parseAttr()
		if !validXMLQName(name) {
			return nil, fmt.Errorf("invalid XML attribute name %q", name)
		}
		if seenAttributes[name] {
			return nil, fmt.Errorf("duplicate XML attribute %q", name)
		}
		seenAttributes[name] = true
		n.Attr = append(n.Attr, dom.Attribute{Name: name, Value: value})
	}

	parentNamespaces := p.namespaceScopes[len(p.namespaceScopes)-1]
	namespaces := make(map[string]string, len(parentNamespaces)+len(n.Attr))
	for prefix, uri := range parentNamespaces {
		namespaces[prefix] = uri
	}
	for i := range n.Attr {
		a := &n.Attr[i]
		switch {
		case a.Name == "xmlns":
			if a.Value == "http://www.w3.org/2000/xmlns/" || a.Value == "http://www.w3.org/XML/1998/namespace" {
				return nil, fmt.Errorf("invalid default XML namespace declaration")
			}
			a.Namespace = dom.NSOther
			a.NamespaceURI = "http://www.w3.org/2000/xmlns/"
			namespaces[""] = a.Value
		case strings.HasPrefix(a.Name, "xmlns:"):
			prefix := strings.TrimPrefix(a.Name, "xmlns:")
			if prefix == "xmlns" || a.Value == "" || a.Value == "http://www.w3.org/2000/xmlns/" ||
				prefix == "xml" && a.Value != "http://www.w3.org/XML/1998/namespace" ||
				prefix != "xml" && a.Value == "http://www.w3.org/XML/1998/namespace" {
				return nil, fmt.Errorf("invalid XML namespace binding for prefix %q", prefix)
			}
			a.Namespace = dom.NSOther
			a.NamespaceURI = "http://www.w3.org/2000/xmlns/"
			namespaces[prefix] = a.Value
		}
	}
	if colon := strings.IndexByte(tag, ':'); colon >= 0 {
		prefix := tag[:colon]
		n.NamespaceURI = namespaces[prefix]
		if n.NamespaceURI == "" {
			return nil, fmt.Errorf("unbound XML namespace prefix %q", prefix)
		}
	} else {
		n.NamespaceURI = namespaces[""]
	}
	switch n.NamespaceURI {
	case "":
		n.Namespace = dom.NSOther
	case "http://www.w3.org/1999/xhtml":
		n.Namespace = dom.NSHTML
	case "http://www.w3.org/2000/svg":
		n.Namespace = dom.NSSVG
	case "http://www.w3.org/1998/Math/MathML":
		n.Namespace = dom.NSMathML
	case "http://www.w3.org/XML/1998/namespace":
		n.Namespace = dom.NSXML
	default:
		if n.NamespaceURI != "" {
			n.Namespace = dom.NSOther
		}
	}
	if n.Namespace == dom.NSHTML {
		if dom.IsValidCustomElementName(n.Data) {
			n.CustomElementUndefined = true
		} else {
			for _, attr := range n.Attr {
				if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" && attr.Name == "is" {
					n.CustomElementUndefined = true
					break
				}
			}
		}
	}
	for i := range n.Attr {
		a := &n.Attr[i]
		if a.NamespaceURI == "http://www.w3.org/2000/xmlns/" {
			continue
		}
		if colon := strings.IndexByte(a.Name, ':'); colon >= 0 {
			prefix := a.Name[:colon]
			a.NamespaceURI = namespaces[prefix]
			if a.NamespaceURI == "" {
				return nil, fmt.Errorf("unbound XML namespace prefix %q", prefix)
			}
			switch a.NamespaceURI {
			case "http://www.w3.org/XML/1998/namespace":
				a.Namespace = dom.NSXML
			case "":
				a.Namespace = dom.NSHTML
			default:
				a.Namespace = dom.NSOther
			}
		}
	}
	if p.onElement != nil {
		p.onElement(n, append([]dom.Attribute(nil), n.Attr...))
	}
	if parent != nil {
		parent.AppendChild(n)
		if n.Namespace == dom.NSHTML {
			switch n.Data {
			case "html":
				if parent == &p.doc.Node {
					p.doc.HTML = n
				}
			case "head":
				if p.doc.Head == nil {
					p.doc.Head = n
				}
			case "body":
				if p.doc.Body == nil {
					p.doc.Body = n
				}
			}
		}
		if p.onInserted != nil {
			p.onInserted(n)
		}
	}
	if selfClosing {
		if p.onScript != nil && n.Data == "script" {
			p.onScript(p.doc, n)
		}
		return n, nil
	}
	p.namespaceScopes = append(p.namespaceScopes, namespaces)
	defer func() { p.namespaceScopes = p.namespaceScopes[:len(p.namespaceScopes)-1] }()

	// Parse children until closing tag.
	for p.pos < len(p.input) {
		if p.input[p.pos] == '<' {
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '/' {
				// Closing tag.
				p.pos += 2
				endTag := p.readName()
				if endTag != tag {
					return nil, fmt.Errorf("mismatched XML closing tag %q for %q", endTag, tag)
				}
				// Skip to >.
				p.skipWhitespace()
				if p.pos >= len(p.input) || p.input[p.pos] != '>' {
					return nil, fmt.Errorf("malformed XML closing tag")
				}
				p.pos++
				if p.onScript != nil && n.Data == "script" {
					p.onScript(p.doc, n)
				}
				return n, nil
			}
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '?' {
				pi, err := p.parseProcessingInstruction()
				if err != nil {
					return nil, err
				}
				if pi != nil {
					n.AppendChild(pi)
				}
				continue
			}
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '!' {
				if p.pos+3 < len(p.input) && p.input[p.pos+2] == '-' && p.input[p.pos+3] == '-' {
					// Comment.
					p.pos += 4
					end := strings.Index(p.input[p.pos:], "-->")
					if end >= 0 {
						data := p.input[p.pos : p.pos+end]
						n.AppendChild(p.doc.NewComment(data))
						p.pos += end + 3
					} else {
						p.pos = len(p.input)
					}
					continue
				}
				if strings.HasPrefix(p.input[p.pos+2:], "[CDATA[") {
					// CDATA section.
					p.pos += 9
					end := strings.Index(p.input[p.pos:], "]]>")
					if end >= 0 {
						data := p.input[p.pos : p.pos+end]
						n.AppendChild(p.doc.NewCDATASection(data))
						p.pos += end + 3
					} else {
						p.pos = len(p.input)
					}
					continue
				}
				// Unknown <! construct.
				p.pos += 2
				end := strings.Index(p.input[p.pos:], ">")
				if end >= 0 {
					p.pos += end + 1
				} else {
					p.pos = len(p.input)
				}
				continue
			}
			// Child element.
			_, err := p.parseElement(n)
			if err != nil {
				return nil, err
			}
		} else {
			// Text content.
			start := p.pos
			for p.pos < len(p.input) && p.input[p.pos] != '<' {
				p.pos++
			}
			text := p.input[start:p.pos]
			decoded, err := decodeXMLEntities(text, p.entities)
			if err != nil {
				return nil, err
			}
			text = decoded
			if text != "" {
				n.AppendChild(p.doc.NewText(text))
			}
		}
	}
	return nil, fmt.Errorf("unterminated XML element %q", tag)
}

// readName reads a name (tag or attribute name) from the current position.
func (p *xmlParser) readName() string {
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' || c == '=' || c == '"' || c == '\'' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

// parseAttr parses a single attribute name="value" pair.
func (p *xmlParser) parseAttr() (name, value string) {
	name = p.readName()
	if name == "" {
		// Skip unknown character.
		if p.pos < len(p.input) {
			p.pos++
		}
		return "", ""
	}
	p.skipWhitespace()
	if p.pos >= len(p.input) || p.input[p.pos] != '=' {
		return name, ""
	}
	p.pos++ // skip =
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return name, ""
	}
	quote := p.input[p.pos]
	if quote == '"' || quote == '\'' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != quote {
			p.pos++
		}
		value = p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // skip closing quote
		}
	} else {
		start := p.pos
		for p.pos < len(p.input) {
			c := p.input[p.pos]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
				break
			}
			p.pos++
		}
		value = p.input[start:p.pos]
	}
	value, _ = decodeXMLEntities(value, p.entities)
	return name, value
}

// skipWhitespace advances past whitespace characters.
func (p *xmlParser) skipWhitespace() {
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		p.pos++
	}
}

// decodeXMLEntities expands XML predefined, numeric, and internal general
// entity references. validateXMLDocument rejects unknown or malformed
// references before this tree-building pass.
func decodeXMLEntities(source string, entities map[string]string) (string, error) {
	return decodeXMLEntitiesDepth(source, entities, 0)
}

func decodeXMLEntitiesDepth(source string, entities map[string]string, depth int) (string, error) {
	if depth > 8 {
		return "", fmt.Errorf("XML entity expansion depth exceeded")
	}
	if !strings.ContainsRune(source, '&') {
		return source, nil
	}
	var result strings.Builder
	for len(source) != 0 {
		amp := strings.IndexByte(source, '&')
		if amp < 0 {
			result.WriteString(source)
			break
		}
		result.WriteString(source[:amp])
		source = source[amp+1:]
		semi := strings.IndexByte(source, ';')
		if semi < 0 {
			return "", fmt.Errorf("unterminated XML entity reference")
		}
		name := source[:semi]
		replacement, err := decodeXMLEntity(name, entities, depth)
		if err != nil {
			return "", err
		}
		result.WriteString(replacement)
		source = source[semi+1:]
		if result.Len() > 1<<20 {
			return "", fmt.Errorf("XML entity expansion size exceeded")
		}
	}
	return result.String(), nil
}

func decodeXMLEntity(name string, entities map[string]string, depth int) (string, error) {
	switch name {
	case "amp":
		return "&", nil
	case "lt":
		return "<", nil
	case "gt":
		return ">", nil
	case "quot":
		return `"`, nil
	case "apos":
		return "'", nil
	}
	if strings.HasPrefix(name, "#") {
		base := 10
		digits := name[1:]
		if strings.HasPrefix(digits, "x") || strings.HasPrefix(digits, "X") {
			base = 16
			digits = digits[1:]
		}
		if digits == "" {
			return "", fmt.Errorf("malformed XML character reference")
		}
		codePoint, err := strconv.ParseInt(digits, base, 32)
		if err != nil || !isXMLCharacter(rune(codePoint)) {
			return "", fmt.Errorf("invalid XML character reference")
		}
		return string(rune(codePoint)), nil
	}
	value, ok := entities[name]
	if !ok {
		return "", fmt.Errorf("undefined XML entity %q", name)
	}
	return decodeXMLEntitiesDepth(value, entities, depth+1)
}

func isXMLCharacter(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD || r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF
}

func validateXMLDocument(source string, entities map[string]string) error {
	return validateXMLDocumentWithLimits(source, entities, dom.ParseLimits{
		Nodes: 4096, Depth: 64, Attributes: 256, AttributeBytes: 8192,
	})
}

func validateXMLDocumentWithLimits(source string, entities map[string]string, limits dom.ParseLimits) error {
	if !utf8.ValidString(source) {
		return fmt.Errorf("invalid UTF-8 in XML document")
	}
	for _, character := range source {
		if !isXMLCharacter(character) {
			return fmt.Errorf("invalid XML character U+%04X", character)
		}
	}
	decoder := xml.NewDecoder(strings.NewReader(source))
	decoder.Strict = true
	decoder.Entity = entities
	depth, roots, doctypes, nodes, attributeCount, attributeBytes := 0, 0, 0, 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			nodes++
			attributeCount += len(token.Attr)
			for _, attr := range token.Attr {
				attributeBytes += len(attr.Name.Local) + len(attr.Name.Space) + len(attr.Value)
			}
			if nodes > limits.Nodes {
				return fmt.Errorf("XML node limit exceeded (%d)", limits.Nodes)
			}
			if attributeCount > limits.Attributes {
				return fmt.Errorf("XML attribute limit exceeded (%d)", limits.Attributes)
			}
			if attributeBytes > limits.AttributeBytes {
				return fmt.Errorf("XML attribute byte limit exceeded (%d)", limits.AttributeBytes)
			}
			if depth == 0 {
				roots++
			}
			depth++
			if depth > limits.Depth {
				return fmt.Errorf("XML depth limit exceeded (%d)", limits.Depth)
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return fmt.Errorf("unexpected XML end element")
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(token)) != "" {
				return fmt.Errorf("character data outside XML document element")
			}
			if depth > 0 {
				nodes++
			}
			if nodes > limits.Nodes {
				return fmt.Errorf("XML node limit exceeded (%d)", limits.Nodes)
			}
		case xml.Directive:
			if strings.HasPrefix(strings.TrimSpace(string(token)), "DOCTYPE") {
				doctypes++
				if doctypes > 1 || roots > 0 {
					return fmt.Errorf("misplaced XML doctype")
				}
			}
		case xml.Comment, xml.ProcInst:
			nodes++
			if nodes > limits.Nodes {
				return fmt.Errorf("XML node limit exceeded (%d)", limits.Nodes)
			}
		}
	}
	if roots != 1 || depth != 0 {
		return fmt.Errorf("XML document must contain exactly one complete document element")
	}
	return nil
}

func parseXMLInternalEntities(source string) map[string]string {
	doctype := strings.Index(source, "<!DOCTYPE")
	if doctype < 0 {
		return nil
	}
	start := doctype + len("<!DOCTYPE")
	quote := byte(0)
	open := -1
	for i := start; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if c == '[' {
			open = i
			break
		} else if c == '>' {
			return nil
		}
	}
	if open < 0 {
		return nil
	}
	quote = 0
	depth := 1
	close := -1
	for i := open + 1; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
		} else if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				close = i
				break
			}
		}
	}
	if close < 0 {
		return nil
	}
	subset := source[open+1 : close]
	entities := make(map[string]string)
	for offset := 0; offset < len(subset); {
		if strings.HasPrefix(subset[offset:], "<!--") {
			if end := strings.Index(subset[offset+4:], "-->"); end >= 0 {
				offset += 4 + end + len("-->")
				continue
			}
			break
		}
		rel := strings.Index(subset[offset:], "<!ENTITY")
		if rel < 0 {
			break
		}
		offset += rel + len("<!ENTITY")
		for offset < len(subset) && isXMLSpace(subset[offset]) {
			offset++
		}
		if offset < len(subset) && subset[offset] == '%' {
			if end := strings.IndexByte(subset[offset:], '>'); end >= 0 {
				offset += end + 1
				continue
			}
			break
		}
		nameStart := offset
		for offset < len(subset) && isXMLNameByte(subset[offset]) {
			offset++
		}
		name := subset[nameStart:offset]
		for offset < len(subset) && isXMLSpace(subset[offset]) {
			offset++
		}
		if name == "" || offset >= len(subset) || subset[offset] != '\'' && subset[offset] != '"' {
			if end := strings.IndexByte(subset[offset:], '>'); end >= 0 {
				offset += end + 1
				continue
			}
			break
		}
		q := subset[offset]
		offset++
		valueStart := offset
		for offset < len(subset) && subset[offset] != q {
			offset++
		}
		if offset >= len(subset) {
			break
		}
		entities[name] = subset[valueStart:offset]
		offset++
	}
	return entities
}

func isXMLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isXMLNameByte(c byte) bool {
	return c == ':' || c == '_' || c == '-' || c == '.' || c >= '0' && c <= '9' ||
		c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80
}

func validXMLQName(name string) bool {
	parts := strings.Split(name, ":")
	if len(parts) > 2 || len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !validXMLNCName(part) {
			return false
		}
	}
	return true
}

// validateDOMElementQName implements DOM's current "validate and extract"
// name rules for element names. DOM intentionally accepts a wider set than
// XML Names so DOM-created names are compatible with names the HTML parser can
// produce. Keep this separate from validXMLQName, which remains appropriate
// for XML parser constructs such as processing instructions.
func validateDOMElementQName(namespace, qualifiedName string) (prefix, localName, exception string) {
	localName = qualifiedName
	if colon := strings.IndexByte(qualifiedName, ':'); colon >= 0 {
		prefix = qualifiedName[:colon]
		localName = qualifiedName[colon+1:]
		if !validDOMNamespacePrefix(prefix) {
			return prefix, localName, "InvalidCharacterError"
		}
	}
	if !validDOMElementLocalName(localName) {
		return prefix, localName, "InvalidCharacterError"
	}

	if prefix != "" && namespace == "" {
		return prefix, localName, "NamespaceError"
	}
	if prefix == "xml" && namespace != "http://www.w3.org/XML/1998/namespace" {
		return prefix, localName, "NamespaceError"
	}
	const xmlnsNamespace = "http://www.w3.org/2000/xmlns/"
	if (qualifiedName == "xmlns" || prefix == "xmlns") && namespace != xmlnsNamespace {
		return prefix, localName, "NamespaceError"
	}
	if namespace == xmlnsNamespace && qualifiedName != "xmlns" && prefix != "xmlns" {
		return prefix, localName, "NamespaceError"
	}
	return prefix, localName, ""
}

func validDOMNamespacePrefix(prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, r := range prefix {
		if r == 0 || r == '/' || r == '>' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
			return false
		}
	}
	return true
}

func validDOMElementLocalName(name string) bool {
	if name == "" {
		return false
	}
	runes := []rune(name)
	first := runes[0]
	if first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z' {
		for _, r := range runes {
			if r == 0 || r == '/' || r == '>' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' {
				return false
			}
		}
		return true
	}
	if first != ':' && first != '_' && first < 0x80 {
		return false
	}
	for _, r := range runes[1:] {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' ||
			r == '-' || r == '.' || r == ':' || r == '_' || r >= 0x80 {
			continue
		}
		return false
	}
	return true
}

func validXMLNCName(name string) bool {
	if name == "" {
		return false
	}
	first := true
	for _, character := range name {
		if first {
			if !isXMLNameStart(character) {
				return false
			}
			first = false
			continue
		}
		if !isXMLNameStart(character) && !unicode.IsDigit(character) && character != '-' && character != '.' &&
			character != 0xB7 && !(character >= 0x0300 && character <= 0x036F) && !(character >= 0x203F && character <= 0x2040) {
			return false
		}
	}
	return true
}

func isXMLNameStart(character rune) bool {
	return character == '_' || unicode.IsLetter(character) ||
		character >= 0xC0 && character <= 0xD6 || character >= 0xD8 && character <= 0xF6 ||
		character >= 0xF8 && character <= 0x2FF || character >= 0x370 && character <= 0x37D ||
		character >= 0x37F && character <= 0x1FFF || character >= 0x200C && character <= 0x200D ||
		character >= 0x2070 && character <= 0x218F || character >= 0x2C00 && character <= 0x2FEF ||
		character >= 0x3001 && character <= 0xD7FF || character >= 0xF900 && character <= 0xFDCF ||
		character >= 0xFDF0 && character <= 0xFFFD || character >= 0x10000 && character <= 0xEFFFF
}

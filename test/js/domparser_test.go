package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// DOMParser
// ---------------------------------------------------------------------------

func TestDOMParserConstructorExists(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`typeof DOMParser`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("typeof DOMParser = %q, want %q", val, "function")
	}
}

func TestCreateProcessingInstructionExposesNodeAndCharacterDataInterfaces(t *testing.T) {
	doc := testDOM(t)
	r := newDOMRuntime(t, doc)
	value, err := r.Evaluate(`(() => {
		const pi = document.createProcessingInstruction("a:b", "value");
		return [pi.nodeType, pi.nodeName, pi.target, pi.nodeValue, pi.data, pi.length].join("|");
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if value != "7|a:b|a:b|value|value|5" {
		t.Fatalf("ProcessingInstruction fields = %q", value)
	}
}

func TestCharacterDataMethodsUseUTF16Offsets(t *testing.T) {
	doc := testDOM(t)
	r := newDOMRuntime(t, doc)
	value, err := r.Evaluate(`(() => {
		const text = document.createTextNode("A😀B");
		const initial = [text.length, text.substringData(1, 2)];
		text.insertData(3, "x");
		const inserted = text.data;
		text.deleteData(1, 2);
		text.replaceData(1, 1, "zz");
		text.appendData("!");
		let errorName = "";
		try { text.substringData(99, 1); } catch (error) { errorName = error.name; }
		const comment = document.createComment("x");
		comment.data = null;
		return [initial.join(":"), inserted, text.data, errorName, comment.data].join("|");
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if want := "4:😀|A😀xB|AzzB!|IndexSizeError|"; value != want {
		t.Fatalf("CharacterData methods = %q, want %q", value, want)
	}
}

func TestCharacterDataPreservesIsolatedUTF16Surrogates(t *testing.T) {
	doc := testDOM(t)
	r := newDOMRuntime(t, doc)
	value, err := r.Evaluate(`(() => {
		const text = document.createTextNode("😀x");
		const high = text.substringData(0, 1);
		text.textContent = high;
		const clone = text.cloneNode();
		return [text.length, text.data.charCodeAt(0), text.nodeValue.charCodeAt(0), text.textContent.charCodeAt(0),
			text.substringData(0, 1).charCodeAt(0), clone.data.charCodeAt(0)].join("|");
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if want := "1|55357|55357|55357|55357|55357"; value != want {
		t.Fatalf("isolated surrogate round trip = %q, want %q", value, want)
	}
}

func TestXMLSerializerProcessingInstructionsAndDoctypeQuotes(t *testing.T) {
	doc := testDOM(t)
	r := newDOMRuntime(t, doc)
	value, err := r.Evaluate(`(() => {
		const serializer = new XMLSerializer();
		const piEmpty = document.createProcessingInstruction("a", "");
		const piData = document.createProcessingInstruction("a:b", "value");
		const doctype = document.implementation.createDocumentType("html", "\"", "\"");
		return [serializer.serializeToString(piEmpty), serializer.serializeToString(piData), serializer.serializeToString(doctype)].join("|");
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<?a ?>|<?a:b value?>|<!DOCTYPE html PUBLIC """ """>`
	if value != want {
		t.Fatalf("XML serialization = %q, want %q", value, want)
	}
}

func TestParseXMLDocumentBoundedBuildsActiveXHTMLTree(t *testing.T) {
	doc, err := js.ParseXMLDocumentBounded(`<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml"><body><script><![CDATA[window.ran = true;]]></script></body></html>`, "application/xhtml+xml", dom.ParseLimits{
		Nodes: 32, Depth: 8, Attributes: 8, AttributeBytes: 256,
	})
	if err != nil {
		t.Fatalf("ParseXMLDocumentBounded: %v", err)
	}
	if !doc.IsXML || doc.ContentType != "application/xhtml+xml" {
		t.Fatalf("document metadata = IsXML %t ContentType %q", doc.IsXML, doc.ContentType)
	}
	if doc.HTML == nil || doc.HTML.Data != "html" || doc.HTML.NamespaceURI != "http://www.w3.org/1999/xhtml" {
		t.Fatalf("XHTML root = %#v", doc.HTML)
	}
	var script *dom.Node
	doc.Node.Walk(func(node *dom.Node) bool {
		if node.Element() && node.Data == "script" {
			script = node
			return false
		}
		return true
	})
	if script == nil {
		t.Fatal("XHTML script node is missing")
	}
	if script.TextContent() != "window.ran = true;" {
		t.Fatalf("script CDATA = %q", script.TextContent())
	}
	if script.ScriptAlreadyStarted {
		t.Fatal("active XHTML parser marked its script inert")
	}
}

func TestParseXMLDocumentPreservesProcessingInstructionsNotDeclarations(t *testing.T) {
	doc, err := js.ParseXMLDocumentBounded(`<?xml version="1.0"?><?xml-stylesheet href="x"?><root><?target data?></root>`, "application/xml", dom.ParseLimits{
		Nodes: 16, Depth: 4, Attributes: 8, AttributeBytes: 128,
	})
	if err != nil {
		t.Fatalf("ParseXMLDocumentBounded: %v", err)
	}
	first := doc.Node.FirstChild
	if first == nil || first.Type != dom.NodeProcessingInstruction || first.Data != "xml-stylesheet" || first.DataContent != "href=\"x\"" {
		t.Fatalf("document first child PI = %#v", first)
	}
	root := first.NextSibling
	if root == nil || root.Data != "root" || root.FirstChild == nil || root.FirstChild.Type != dom.NodeProcessingInstruction ||
		root.FirstChild.Data != "target" || root.FirstChild.DataContent != "data" {
		t.Fatalf("root child PI tree = %#v", root)
	}
}

func TestParseXMLDocumentBoundedRejectsTreeLimitsBeforeConstruction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		markup string
		limits dom.ParseLimits
	}{
		{name: "nodes", markup: `<r><a/><b/></r>`, limits: dom.ParseLimits{Nodes: 2, Depth: 8, Attributes: 8, AttributeBytes: 64}},
		{name: "depth", markup: `<r><a><b/></a></r>`, limits: dom.ParseLimits{Nodes: 8, Depth: 2, Attributes: 8, AttributeBytes: 64}},
		{name: "attributes", markup: `<r a="1" b="2"/>`, limits: dom.ParseLimits{Nodes: 8, Depth: 8, Attributes: 1, AttributeBytes: 64}},
		{name: "attribute bytes", markup: `<r a="long"/>`, limits: dom.ParseLimits{Nodes: 8, Depth: 8, Attributes: 8, AttributeBytes: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := js.ParseXMLDocumentBounded(tc.markup, "application/xhtml+xml", tc.limits); err == nil {
				t.Fatal("ParseXMLDocumentBounded unexpectedly accepted input beyond limits")
			}
		})
	}
}

func TestDOMParserParseFromStringHTML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	// Parse a simple HTML string and check the body's first child tag name.
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<div>hello</div>', 'text/html');
		doc.body.firstChild.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "DIV" {
		t.Errorf("body.firstChild.tagName = %q, want %q", val, "DIV")
	}
}

func TestDOMParserParseFromStringHTMLTextContent(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<p>world</p>', 'text/html');
		doc.body.firstChild.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "world" {
		t.Errorf("body.firstChild.textContent = %q, want %q", val, "world")
	}
}

func TestDocumentImportNodeAndAdoptNodeAcrossParsedDocuments(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var source = new DOMParser().parseFromString('<root><branch><leaf>text</leaf></branch></root>', 'application/xml');
		var target = document.implementation.createHTMLDocument('target');
		var sourceRoot = source.documentElement;
		var shallow = target.importNode(sourceRoot);
		var deep = target.importNode(sourceRoot, true);
		var sourceChild = sourceRoot.firstChild;
		var adopted = target.adoptNode(sourceChild);
		[
			shallow !== sourceRoot,
			shallow.ownerDocument === target,
			shallow.firstChild === null,
			deep !== sourceRoot,
			deep.ownerDocument === target,
			deep.firstChild.firstChild.textContent === 'text',
		adopted === sourceChild,
			adopted.parentNode === null,
			adopted.ownerDocument === target,
			sourceRoot.childNodes.length === 0
		].every(Boolean);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Fatalf("importNode/adoptNode invariants = %q, want true", val)
	}
}

func TestDocumentImportNodeRejectsDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	_, err := r.Evaluate(`document.importNode(document)`)
	if err == nil || !strings.Contains(err.Error(), "NotSupportedError") {
		t.Fatalf("importNode(document) error = %v, want NotSupportedError", err)
	}
}

func TestSVGTemplateNameDoesNotUseHTMLTemplateSemantics(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var parsed = new DOMParser().parseFromString('<svg><template>text</template></svg>', 'text/html');
		var svgTemplate = parsed.querySelector('template');
		var scratch = document.implementation.createHTMLDocument();
		scratch.body.innerHTML = '<svg><template></template></svg>';
	var liveSVGTemplate = scratch.querySelector('template');
		liveSVGTemplate.innerHTML = 'text';
		var htmlTemplate = scratch.createElement('template');
		htmlTemplate.innerHTML = 'inert';
		[svgTemplate.textContent, 'content' in svgTemplate,
		 svgTemplate.namespaceURI, liveSVGTemplate.textContent,
		 'content' in liveSVGTemplate, htmlTemplate.textContent,
		 htmlTemplate.content.textContent].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "text|false|http://www.w3.org/2000/svg|text|false||inert" {
		t.Fatalf("SVG/HTML template behavior = %q", val)
	}
}

func TestDOMParserHTMLDisablesScriptingForNoscript(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<!doctype html><html><head></head><body><noscript><p>visible markup</p></noscript></body></html>', 'text/html');
		var noscript = doc.querySelector('noscript');
		[noscript.firstChild.nodeType, noscript.firstChild.localName, noscript.firstChild.textContent].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "1|p|visible markup" {
		t.Errorf("DOMParser noscript child = %q, want an HTMLParagraphElement", val)
	}
}

func TestDOMParserDetachedBaseURIDoesNotInheritOuterBase(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var base = document.createElement('base');
		base.setAttribute('href', '/outer-base/');
		document.head.appendChild(base);
		var parsed = new DOMParser().parseFromString('<html><body></body></html>', 'text/html');
		[parsed.URL, parsed.baseURI, document.baseURI].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "https://example.test/|https://example.test/|https://example.test/outer-base/" {
		t.Errorf("DOMParser document URL/base URI = %q", val)
	}
}

func TestDOMParserParseFromStringXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child attr="val">text</child></root>', 'text/xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "root" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "root")
	}
}

func TestDOMParserParseFromStringXMLChild(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child attr="val">text</child></root>', 'text/xml');
		doc.documentElement.firstChild.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "child" {
		t.Errorf("firstChild.tagName = %q, want %q", val, "child")
	}
}

func TestDOMParserParseFromStringXMLAttribute(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child attr="val">text</child></root>', 'text/xml');
		doc.documentElement.firstChild.getAttribute("attr");
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "val" {
		t.Errorf("getAttribute('attr') = %q, want %q", val, "val")
	}
}

func TestDOMParserParseFromStringXMLTextContent(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child>hello world</child></root>', 'text/xml');
		doc.documentElement.firstChild.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "hello world" {
		t.Errorf("textContent = %q, want %q", val, "hello world")
	}
}

func TestDOMParserParseFromStringApplicationXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<data/>', 'application/xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "data" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "data")
	}
}

func TestDOMParserParseFromStringXHTML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<html xmlns="http://www.w3.org/1999/xhtml"><body><p>hi</p></body></html>', 'application/xhtml+xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "html" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "html")
	}
}

func TestDOMParserParseFromStringSVG(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>', 'image/svg+xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "svg" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "svg")
	}
}

func TestDOMParserParseFromStringComment(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><!-- a comment --><child/></root>', 'text/xml');
		var fc = doc.documentElement.firstChild;
		fc.nodeType;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Comment nodeType is 8.
	if val != "8" {
		t.Errorf("firstChild.nodeType = %q, want %q (comment)", val, "8")
	}
}

func TestDOMParserParseFromStringCDATA(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><![CDATA[some <data>]]></root>', 'text/xml');
		doc.documentElement.firstChild.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "some <data>" {
		t.Errorf("CDATA textContent = %q, want %q", val, "some <data>")
	}
}

func TestDOMParserXMLSerializerCDATAroundTrip(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var source = '<root><![CDATA[one < two]]><![CDATA[& three]]></root>';
		var parsed = new DOMParser().parseFromString(source, 'text/xml');
		var first = parsed.documentElement.firstChild;
		[
			first.nodeType,
			first.nodeName,
			first.textContent,
			first.nodeValue,
			new XMLSerializer().serializeToString(parsed)
		].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := "4|#cdata-section|one < two|one < two|<root><![CDATA[one < two]]><![CDATA[& three]]></root>"
	if val != want {
		t.Errorf("CDATA round-trip = %q, want %q", val, want)
	}
}

func TestDOMParserXMLDocumentMetadata(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<root/>', 'application/xml');
		[
			doc instanceof Document,
			doc instanceof XMLDocument,
			doc.characterSet,
			doc.charset,
			doc.inputEncoding,
			doc.contentType,
			doc.readyState,
			doc.location
		].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true|false|UTF-8|UTF-8|UTF-8|application/xml|complete|" {
		t.Errorf("XML document metadata = %q, want Document (not XMLDocument) with UTF-8/application/xml/complete metadata", val)
	}
}

func TestDOMParserXHTMLDocumentCreateElementNamespaceAndSerialization(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<html xmlns="http://www.w3.org/1999/xhtml"><body/></html>', 'application/xhtml+xml');
		var br = doc.createElement('br');
		var div = doc.createElement('div');
		div.appendChild(br);
		var invalidName = doc.createElement('test:test');
		doc.documentElement.appendChild(invalidName);
		var invalidInnerHTMLThrows = false;
		try { doc.documentElement.innerHTML; } catch (error) { invalidInnerHTMLThrows = error.name === 'InvalidStateError'; }
		[
			doc instanceof Document,
			!(doc instanceof XMLDocument),
			br.namespaceURI,
			div.innerHTML,
			invalidName.prefix,
			invalidName.localName,
			invalidInnerHTMLThrows
		].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true|true|http://www.w3.org/1999/xhtml|<br xmlns=\"http://www.w3.org/1999/xhtml\" />||test:test|true" {
		t.Errorf("parsed XHTML createElement/serialization = %q", val)
	}
}

func TestDOMParserXMLMalformedDocumentsProduceParserError(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var inputs = [
			'<root>&times;</root>',
			'<root unquoted=value></root>',
			'<root x:item="v"></root>',
			'<root xmlns:="urn:bad"></root>',
			'<root xmlns:xmlns=""></root>',
			'<root></root><second></second>',
			'<!DOCTYPE root [<!-- <!ENTITY fake "value"> -->]><root>&fake;</root>'
		];
		inputs.map(function(input) {
			return new DOMParser().parseFromString(input, 'application/xml').documentElement.localName;
		}).join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "parsererror|parsererror|parsererror|parsererror|parsererror|parsererror|parsererror" {
		t.Errorf("malformed XML document elements = %q", val)
	}
}

func TestDOMParserXMLInternalAndNumericEntities(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var source = '<!DOCTYPE root [<!ENTITY word "Hello">]><root>&word; &#215; &#x41;</root>';
		new DOMParser().parseFromString(source, 'text/xml').documentElement.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "Hello × A" {
		t.Errorf("XML entity expansion = %q", val)
	}
}

func TestDocumentCreateCommentAndHTMLSerialization(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var p = document.createElement('p');
		p.appendChild(document.createComment('abc-->def<!--ghj'));
		p.innerHTML;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "<!--abc-->def<!--ghj-->" {
		t.Errorf("comment serialization = %q", val)
	}
}

func TestDOMParserStyleSheetsLiveCollectionAndRuleInterfaces(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<style>div { color: green; } p { color: red; }</style>', 'text/html');
		var list = doc.styleSheets;
		var sheet = list.item(0);
		var rule = sheet.cssRules.item(0);
		var style = doc.createElement('style');
		style.textContent = 'a { display: block; }';
		doc.head.appendChild(style);
		[
			list === doc.styleSheets,
			list.length,
			sheet === list[0],
		sheet instanceof CSSStyleSheet,
		sheet.cssRules.length,
		rule instanceof CSSStyleRule,
		rule instanceof CSSRule,
		rule.selectorText
		].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true|2|true|true|2|true|true|div" {
		t.Errorf("style sheet DOM/CSSOM result = %q", val)
	}
}

func TestDOMParserParseFromStringXMLEntities(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root>&amp;&lt;&gt;</root>', 'text/xml');
		doc.documentElement.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "&<>" {
		t.Errorf("entity textContent = %q, want %q", val, "&<>")
	}
}

func TestDOMParserUnsupportedType(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	_, err := r.Evaluate(`
		var dp = new DOMParser();
		try { dp.parseFromString('x', 'text/plain'); "no-throw"; } catch(e) { "threw"; }
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	val, err := r.Evaluate(`
		var dp2 = new DOMParser();
		var result;
		try { dp2.parseFromString('x', 'text/plain'); result = "no-throw"; } catch(e) { result = "threw"; }
		result;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "threw" {
		t.Errorf("unsupported type did not throw, got %q", val)
	}
}

func TestDOMParserReturnsDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<div>hello</div>', 'text/html');
		doc.nodeType;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Document nodeType is 9.
	if val != "9" {
		t.Errorf("nodeType = %q, want %q (document)", val, "9")
	}
}

func TestDOMParserHTMLExposesDoctype(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var parsed = new DOMParser().parseFromString('<!DOCTYPE html pu', 'text/html');
		parsed.doctype && parsed.doctype.name;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "html" {
		t.Errorf("truncated doctype name = %q, want %q", val, "html")
	}
}

// ---------------------------------------------------------------------------
// XMLSerializer
// ---------------------------------------------------------------------------

func TestXMLSerializerConstructorExists(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`typeof XMLSerializer`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("typeof XMLSerializer = %q, want %q", val, "function")
	}
}

func TestXMLSerializerPrototypeMethodIsShared(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var a = new XMLSerializer();
		var b = new XMLSerializer();
		XMLSerializer.prototype.serializeToString === a.serializeToString &&
			a.serializeToString === b.serializeToString &&
			Object.getPrototypeOf(a) === XMLSerializer.prototype;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("XMLSerializer prototype method identity = %q, want true", val)
	}
}

func TestXMLSerializerSerializeEmptyElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		xs.serializeToString(document.createElement('div'));
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<div xmlns="http://www.w3.org/1999/xhtml"></div>`
	if val != want {
		t.Errorf("serializeToString(empty div) = %q, want %q", val, want)
	}
}

func TestXMLSerializerSerializeElementWithText(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var el = document.createElement('p');
		el.appendChild(document.createTextNode('hello'));
		xs.serializeToString(el);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<p xmlns="http://www.w3.org/1999/xhtml">hello</p>`
	if val != want {
		t.Errorf("serializeToString = %q, want %q", val, want)
	}
}

func TestXMLSerializerSerializeElementWithAttributes(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var el = document.createElement('div');
		el.setAttribute('id', 'main');
		el.setAttribute('class', 'active');
		xs.serializeToString(el);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.HasPrefix(val, `<div xmlns="http://www.w3.org/1999/xhtml"`) ||
		!strings.Contains(val, `id="main"`) || !strings.Contains(val, `class="active"`) || !strings.HasSuffix(val, `></div>`) {
		t.Errorf("serializeToString = %q, want attributes", val)
	}
}

func TestXMLSerializerSerializeNestedElements(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var outer = document.createElement('div');
		var inner = document.createElement('span');
		inner.appendChild(document.createTextNode('text'));
		outer.appendChild(inner);
		xs.serializeToString(outer);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<div xmlns="http://www.w3.org/1999/xhtml"><span>text</span></div>`
	if val != want {
		t.Errorf("serializeToString = %q, want %q", val, want)
	}
}

func TestXMLSerializerSerializeTextEscaping(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var el = document.createElement('p');
		el.appendChild(document.createTextNode('a < b & c > d'));
		xs.serializeToString(el);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != `<p xmlns="http://www.w3.org/1999/xhtml">a &lt; b &amp; c &gt; d</p>` {
		t.Errorf("serializeToString = %q, want escaped text", val)
	}
}

func TestXMLSerializerSerializeAttributeEscaping(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var el = document.createElement('div');
		el.setAttribute('data-x', 'a"b');
		xs.serializeToString(el);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.Contains(val, `data-x="a&quot;b"`) {
		t.Errorf("serializeToString = %q, want escaped attribute value", val)
	}
}

func TestXMLSerializerSerializeComment(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var src = dp.parseFromString('<root/>', 'text/xml');
		var div = src.createElement('div');
		var comment = src.createComment(' a comment ');
		div.appendChild(comment);
		var xs = new XMLSerializer();
		xs.serializeToString(div);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "<div><!-- a comment --></div>" {
		t.Errorf("serializeToString = %q, want %q", val, "<div><!-- a comment --></div>")
	}
}

func TestXMLSerializerSerializeParsedXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child attr="val">text</child></root>', 'text/xml');
		var xs = new XMLSerializer();
		xs.serializeToString(doc);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != `<root><child attr="val">text</child></root>` {
		t.Errorf("serializeToString(parsed XML) = %q", val)
	}
}

func TestXMLSerializerSerializeDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><a/><b/></root>', 'text/xml');
		var xs = new XMLSerializer();
		xs.serializeToString(doc.documentElement);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != `<root><a/><b/></root>` {
		t.Errorf("serializeToString = %q", val)
	}
}

func TestDOMParserAndXMLSerializerRoundTrip(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	// Parse XML, serialize it back, and verify it matches.
	input := `<data><item id="1">alpha</item><item id="2">beta</item></data>`
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var xs = new XMLSerializer();
		var doc = dp.parseFromString(` + js_string(input) + `, 'text/xml');
		xs.serializeToString(doc.documentElement);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != input {
		t.Errorf("round-trip = %q, want %q", val, input)
	}
}

// js_string is a test helper that wraps a Go string as a JS string literal.
func js_string(s string) string {
	// Escape backslashes and quotes for JS.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestDOMParserEmptyHTML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('', 'text/html');
		doc.nodeType;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "9" {
		t.Errorf("empty HTML parse nodeType = %q, want %q", val, "9")
	}
}

func TestDOMParserEmptyXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('', 'text/xml');
		doc.nodeType;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "9" {
		t.Errorf("empty XML parse nodeType = %q, want %q", val, "9")
	}
}

func TestXMLSerializerSelfClosingXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><empty/></root>', 'text/xml');
		var xs = new XMLSerializer();
		xs.serializeToString(doc.documentElement);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "<root><empty/></root>" {
		t.Errorf("serializeToString = %q, want %q", val, "<root><empty/></root>")
	}
}

func TestDOMParserXMLWithProcessingInstruction(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<?xml version="1.0"?><root/>', 'text/xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "root" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "root")
	}
}

func TestDOMParserXMLWithDOCTYPE(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<!DOCTYPE html><html/>', 'text/xml');
		doc.documentElement.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "html" {
		t.Errorf("documentElement.tagName = %q, want %q", val, "html")
	}
}

func TestDOMParserHTMLComplex(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<html><head><title>T</title></head><body><p id="x">hello</p></body></html>', 'text/html');
		doc.getElementById('x').textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "hello" {
		t.Errorf("textContent = %q, want %q", val, "hello")
	}
}

func TestXMLSerializerSerializeDocumentFragment(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var frag = document.createDocumentFragment();
		var div = document.createElement('div');
		div.appendChild(document.createTextNode('a'));
		frag.appendChild(div);
		var span = document.createElement('span');
		span.appendChild(document.createTextNode('b'));
		frag.appendChild(span);
		xs.serializeToString(frag);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != `<div xmlns="http://www.w3.org/1999/xhtml">a</div><span xmlns="http://www.w3.org/1999/xhtml">b</span>` {
		t.Errorf("serializeToString(fragment) = %q", val)
	}
}

// Verify that the parsed document's elements are accessible from the
// serializer, testing the integration between DOMParser and XMLSerializer.
func TestDOMParserXMLSerializerIntegration(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var xs = new XMLSerializer();
		var doc = dp.parseFromString('<root><a>1</a><b>2</b></root>', 'application/xml');
		var root = doc.documentElement;
		var result = [];
		for (var i = 0; i < root.childNodes.length; i++) {
			result.push(xs.serializeToString(root.childNodes[i]));
		}
		result.join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "<a>1</a>|<b>2</b>" {
		t.Errorf("integration = %q, want %q", val, "<a>1</a>|<b>2</b>")
	}
}

// Verify that a node from a DOMParser document has the correct ownerDocument.
func TestDOMParserOwnerDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root/>', 'text/xml');
		doc.documentElement.ownerDocument === doc;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("ownerDocument === doc = %q, want %q", val, "true")
	}
}

// Test that the parsed HTML document has proper structure (html/head/body).
func TestDOMParserHTMLStructure(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<p>test</p>', 'text/html');
		var has = doc.documentElement !== null && doc.body !== null;
		has;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("HTML structure = %q, want %q", val, "true")
	}
}

// Verify that XML entity decoding works in attributes.
func TestDOMParserXMLAttributeEntities(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root attr="a&amp;b"/>', 'text/xml');
		doc.documentElement.getAttribute("attr");
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "a&b" {
		t.Errorf("attribute entity = %q, want %q", val, "a&b")
	}
}

// Test that DOMParser with text/html on the main document's runtime still
// returns a DETACHED document (not the main document).
func TestDOMParserReturnsDetachedDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var parsed = dp.parseFromString('<div>new</div>', 'text/html');
		parsed !== document;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("parsed !== document = %q, want %q", val, "true")
	}
}

// Verify that the DOMParser-created document has createElement.
func TestDOMParserDocumentHasCreateElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root/>', 'text/xml');
		typeof doc.createElement;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("typeof doc.createElement = %q, want %q", val, "function")
	}
}

// Verify that the DOMParser-created document can create elements.
func TestDOMParserDocumentCreateElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root/>', 'text/xml');
		var el = doc.createElement('test');
		el.tagName;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// The tag name should be lowercase since dom.NewElement lowercases.
	if val != "test" {
		t.Errorf("createElement tagName = %q, want %q", val, "test")
	}
}

// Test that DOMParser with a malformed XML still returns a document.
func TestDOMParserMalformedXML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><unclosed>', 'text/xml');
		doc.nodeType;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "9" {
		t.Errorf("malformed XML nodeType = %q, want %q", val, "9")
	}
}

// Test that XMLSerializer handles the main document.
func TestXMLSerializerMainDocument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var xs = new XMLSerializer();
		var result = xs.serializeToString(document);
		result.indexOf('<html ') >= 0 || result.indexOf('<html>') >= 0;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("serializeToString(document) contains <html> = %q, want %q", val, "true")
	}
}

// Test that DOMParser creates a document with querySelector.
func TestDOMParserDocumentQuerySelector(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root><child id="c1"/></root>', 'text/xml');
		var el = doc.getElementById('c1');
		el !== null && el.tagName === 'child';
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("getElementById on parsed doc = %q, want %q", val, "true")
	}
}

// Test DOMParser with HTML containing special characters.
func TestDOMParserHTMLSpecialChars(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<p>&amp;&lt;&gt;</p>', 'text/html');
		doc.body.firstChild.textContent;
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "&<>" {
		t.Errorf("HTML entity decode = %q, want %q", val, "&<>")
	}
}

// Test that XMLSerializer properly serializes the parsed XML with attributes.
func TestXMLSerializerWithAttributes(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var dp = new DOMParser();
		var doc = dp.parseFromString('<root x="1" y="2"/>', 'text/xml');
		var xs = new XMLSerializer();
		xs.serializeToString(doc.documentElement);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// Check that both attributes are present.
	if !strings.Contains(val, `x="1"`) || !strings.Contains(val, `y="2"`) {
		t.Errorf("serializeToString = %q, missing attributes", val)
	}
	if !strings.HasPrefix(val, "<root") || !strings.HasSuffix(val, "/>") {
		t.Errorf("serializeToString = %q, wrong structure", val)
	}
}

func TestXMLSerializerAddsNamespaceFixup(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<svg><circle/></svg>', 'text/html');
		new XMLSerializer().serializeToString(doc.body.firstChild);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<svg xmlns="http://www.w3.org/2000/svg"><circle/></svg>`
	if val != want {
		t.Errorf("serializeToString(SVG) = %q, want %q", val, want)
	}
}

func TestXMLSerializerPreservesNamespacedAttributes(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var el = document.createElement('div');
		el.setAttributeNS('urn:custom', 'value', 'x');
		new XMLSerializer().serializeToString(el);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	want := `<div xmlns="http://www.w3.org/1999/xhtml" xmlns:ns1="urn:custom" ns1:value="x"></div>`
	if val != want {
		t.Errorf("serializeToString(namespaced attribute) = %q, want %q", val, want)
	}
}

func TestXMLSerializerIncludesDoctypeIdentifiers(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	val, err := r.Evaluate(`
		var doc = new DOMParser().parseFromString('<!DOCTYPE root PUBLIC "pub" "sys"><root/>', 'text/xml');
		new XMLSerializer().serializeToString(doc);
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !strings.HasPrefix(val, `<!DOCTYPE root PUBLIC "pub" "sys">`) {
		t.Errorf("serialized doctype = %q, missing public and system identifiers", val)
	}
}

// Ensure that the dom package is imported (used by testDOM).
var _ = dom.NewDocument

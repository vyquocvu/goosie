package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestRangeContentsConstructsPartiallyContainedCustomElementsInTreeOrder(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head></head><body><c-e id="root"><c-e id="root-0"><c-e id="root-0-0"><c-e id="root-0-0-0"></c-e><span id="start"></span></c-e></c-e><c-e id="root-1"></c-e><span id="end"></span></c-e></body></html>`)
	r, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const logs = [];
		class CE extends HTMLElement {
			constructor() { super(); logs.push(this.id); }
		}
		customElements.define('c-e', CE);
		const makeRange = () => {
			const range = new Range();
			range.setStart(document.getElementById('start'), 0);
			range.setEnd(document.getElementById('end'), 0);
			return range;
		};
		logs.length = 0;
		makeRange().cloneContents();
		if (logs.join('|') !== 'root-0|root-0-0|root-1') throw new Error('clone order: ' + logs.join('|'));
		logs.length = 0;
		makeRange().extractContents();
		if (logs.join('|') !== 'root-0|root-0-0') throw new Error('extract order: ' + logs.join('|'));
	`, "range-partial-custom-element-construction.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// rangeDOM builds a small DOM tree for Range tests:
//
//	<html>
//	  <head></head>
//	  <body>
//	    <p id="p1">Hello World</p>
//	    <p id="p2">Foo Bar</p>
//	    <div id="d1"><span id="s1">Inner</span></div>
//	  </body>
//	</html>
func rangeDOM(t *testing.T) *dom.Document {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)

	head := doc.NewElement("head")
	html.AppendChild(head)

	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.Head = head
	doc.HTML = html

	p1 := doc.NewElement("p")
	p1.SetAttribute("id", "p1")
	body.AppendChild(p1)
	p1.AppendChild(doc.NewText("Hello World"))

	p2 := doc.NewElement("p")
	p2.SetAttribute("id", "p2")
	body.AppendChild(p2)
	p2.AppendChild(doc.NewText("Foo Bar"))

	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "d1")
	body.AppendChild(d1)
	s1 := doc.NewElement("span")
	s1.SetAttribute("id", "s1")
	d1.AppendChild(s1)
	s1.AppendChild(doc.NewText("Inner"))

	return doc
}

func newRangeRuntime(t *testing.T, d *dom.Document) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		URL: "https://example.test/",
		DOM: d,
	})
	if err != nil {
		t.Fatalf("js.New with DOM: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// ---------------------------------------------------------------------------
// document.createRange()
// ---------------------------------------------------------------------------

func TestCreateRange(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("typeof document.createRange()")
	if err != nil {
		t.Fatal(err)
	}
	if val != "object" {
		t.Errorf("createRange() returned %q, want %q", val, "object")
	}
}

func TestRangeConstructorAndDetachedDocumentRange(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	got, err := r.Evaluate(`
		var live = new Range();
		var detachedDocument = new Document();
		var detached = detachedDocument.createRange();
		JSON.stringify([
			typeof Range, live instanceof Range, Range.START_TO_START,
			live.END_TO_END, detached.startContainer === detachedDocument,
			Object.getPrototypeOf(detached) === Range.prototype
		]);
	`)
	if err != nil {
		t.Fatalf("construct Range instances: %v", err)
	}
	want := `["function",true,0,2,true,true]`
	if got != want {
		t.Fatalf("Range constructor result = %s, want %s", got, want)
	}
}

func TestStaticRangeConstructorAndInputEventTargetRanges(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	got, err := r.Evaluate(`
		var text = document.getElementById('p1').firstChild;
		var range = new StaticRange({startContainer:text,startOffset:1,endContainer:text,endOffset:3});
		var event = new InputEvent('beforeinput', {targetRanges:[range]});
		var first = event.getTargetRanges();
		var second = event.getTargetRanges();
		JSON.stringify([
			range instanceof StaticRange, range.startContainer === text,
			range.startOffset, range.endOffset, range.collapsed,
			event.getTargetRanges().length, first !== second, first[0] !== range,
			Object.prototype.toString.call(range), Object.prototype.toString.call(event)
		]);
	`)
	if err != nil {
		t.Fatalf("construct StaticRange and InputEvent target ranges: %v", err)
	}
	want := `[true,true,1,3,false,1,true,true,"[object StaticRange]","[object InputEvent]"]`
	if got != want {
		t.Fatalf("StaticRange and InputEvent result = %s, want %s", got, want)
	}
}

func TestBeforeInputSelectionAcrossEditingHostsHasNoTargetRange(t *testing.T) {
	doc := domtest.Parse(`<html><body><div id="host" contenteditable><p>abc<span contenteditable="false">non-editable</span>def</p></div></body></html>`)
	r, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		const host = document.getElementById('host');
		const p = host.firstChild;
		const selection = document.getSelection();
		selection.setBaseAndExtent(p.firstChild, 2, p.childNodes[1].firstChild, 2);
		window.targetRangeCount = -1;
		host.addEventListener('beforeinput', event => {
			window.targetRangeCount = event.getTargetRanges().length;
		});
	`, "beforeinput-cross-editing-hosts-setup.js"); err != nil {
		t.Fatalf("setup cross-host selection: %v", err)
	}
	if !r.DispatchTrustedInputEvent(doc.Body.FirstChild, nil, "deleteContentBackward") {
		t.Fatal("DispatchTrustedInputEvent returned false")
	}
	got, err := r.Evaluate(`String(window.targetRangeCount)`)
	if err != nil {
		t.Fatalf("read target range count: %v", err)
	}
	if got != "0" {
		t.Fatalf("cross-host beforeinput target range count = %s, want 0", got)
	}
}

func TestStaticRangeRejectsMissingAndDoctypeBoundaries(t *testing.T) {
	d := rangeDOM(t)
	d.DocType = d.NewDoctype("html")
	d.Node.InsertBefore(d.DocType, d.Node.FirstChild)
	r := newRangeRuntime(t, d)
	got, err := r.Evaluate(`
		var missing = false, doctype = false;
		try { new StaticRange({startContainer:document.body,endContainer:document.body,startOffset:0}); }
		catch (e) { missing = e instanceof TypeError; }
		try { new StaticRange({startContainer:document.childNodes[0],endContainer:document.body,startOffset:0,endOffset:0}); }
		catch (e) { doctype = e.name === 'InvalidNodeTypeError'; }
		JSON.stringify([missing,doctype]);
	`)
	if err != nil {
		t.Fatalf("validate StaticRange constructor: %v", err)
	}
	if got != `[true,true]` {
		t.Fatalf("StaticRange validation = %s, want [true,true]", got)
	}
}

func TestNodeCompareDocumentPosition(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	got, err := r.Evaluate(`
		var first = document.getElementById('p1');
		var second = document.getElementById('p2');
		var text = first.firstChild;
		var foreign = document.implementation.createDocument(null, 'foreign', null).documentElement;
		JSON.stringify([
			first.compareDocumentPosition(second),
			second.compareDocumentPosition(first),
			first.compareDocumentPosition(text),
			text.compareDocumentPosition(first),
			first.compareDocumentPosition(first),
			foreign.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_DISCONNECTED,
			foreign.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_IMPLEMENTATION_SPECIFIC
		]);
	`)
	if err != nil {
		t.Fatalf("compare document positions: %v", err)
	}
	want := `[4,2,20,10,0,1,32]`
	if got != want {
		t.Fatalf("compareDocumentPosition values = %s, want %s", got, want)
	}
}

func TestRangeCreateContextualFragmentUsesStartContainer(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var range = document.createRange();
		var context = document.getElementById('d1');
		range.selectNodeContents(context);
		var fragment = range.createContextualFragment('<b>one</b><i>two</i>');
		[fragment.nodeType, fragment.childNodes.length, fragment.firstChild.localName,
		 fragment.lastChild.localName, fragment.firstChild.ownerDocument === document].join('|');
	`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "11|2|b|i|true" {
		t.Fatalf("contextual fragment = %q, want 11|2|b|i|true", val)
	}
}

func TestRangeCollapsed(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("var range = document.createRange(); range.collapsed")
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("new range collapsed = %q, want %q", val, "true")
	}
}

func TestRangeStartEndContainer(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var range = document.createRange();
		range.startContainer === document
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("startContainer === document: %q, want %q", val, "true")
	}

	val, err = r.Evaluate(`
		var range = document.createRange();
		range.endContainer === document
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("endContainer === document: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.setStart / setEnd
// ---------------------------------------------------------------------------

func TestRangeSetStart(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.setStart(p1, 0);
		range.startContainer === p1
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setStart: startContainer === p1: %q, want %q", val, "true")
	}
}

func TestRangeSetEnd(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.setEnd(p1, 1);
		range.endContainer === p1 && range.endOffset === 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setEnd: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.selectNode
// ---------------------------------------------------------------------------

func TestRangeSelectNode(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNode(p1);
		range.startContainer === p1.parentNode && range.startOffset === 0
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("selectNode start: %q, want %q", val, "true")
	}

	val, err = r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNode(p1);
		range.endContainer === p1.parentNode && range.endOffset === 1
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("selectNode end: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.selectNodeContents
// ---------------------------------------------------------------------------

func TestRangeSelectNodeContents(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.startContainer === p1 && range.startOffset === 0
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("selectNodeContents start: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.toString()
// ---------------------------------------------------------------------------

func TestRangeToString(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.toString()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "Hello World" {
		t.Errorf("toString() = %q, want %q", val, "Hello World")
	}
}

func TestRangeToStringCollapsed(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var range = document.createRange();
		range.toString()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("collapsed toString() = %q, want %q", val, "")
	}
}

// ---------------------------------------------------------------------------
// Range.collapse
// ---------------------------------------------------------------------------

func TestRangeCollapse(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.collapse(true);
		range.collapsed
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("collapse(true) collapsed = %q, want %q", val, "true")
	}
}

func TestRangeCollapseToEnd(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.collapse(false);
		range.collapsed && range.startOffset === range.endOffset
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("collapse(false) collapsed = %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.cloneRange
// ---------------------------------------------------------------------------

func TestRangeCloneRange(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		var clone = range.cloneRange();
		clone.startContainer === range.startContainer &&
		clone.startOffset === range.startOffset &&
		clone.endContainer === range.endContainer &&
		clone.endOffset === range.endOffset
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("cloneRange: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.detach
// ---------------------------------------------------------------------------

func TestRangeDetach(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	// detach() is a no-op in modern browsers but must not throw.
	_, err := r.Evaluate(`
		var range = document.createRange();
		range.detach();
		true
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Range.setStartBefore / setStartAfter / setEndBefore / setEndAfter
// ---------------------------------------------------------------------------

func TestRangeSetStartBefore(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.setStartBefore(p1);
		range.startContainer === p1.parentNode
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setStartBefore: %q, want %q", val, "true")
	}
}

func TestRangeSetStartAfter(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.setStartAfter(p1);
		range.startContainer === p1.parentNode
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setStartAfter: %q, want %q", val, "true")
	}
}

func TestRangeSetEndBefore(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p2 = document.getElementById('p2');
		var range = document.createRange();
		range.setEndBefore(p2);
		range.endContainer === p2.parentNode
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setEndBefore: %q, want %q", val, "true")
	}
}

func TestRangeSetEndAfter(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p2 = document.getElementById('p2');
		var range = document.createRange();
		range.setEndAfter(p2);
		range.endContainer === p2.parentNode
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("setEndAfter: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.commonAncestorContainer
// ---------------------------------------------------------------------------

func TestRangeCommonAncestorContainer(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		var range = document.createRange();
		range.setStartBefore(p1);
		range.setEndAfter(p2);
		range.commonAncestorContainer === p1.parentNode
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("commonAncestorContainer: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Range.deleteContents
// ---------------------------------------------------------------------------

func TestRangeDeleteContents(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	_, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.deleteContents();
	`)
	if err != nil {
		t.Fatal(err)
	}

	// After deleting, p1 should have no children.
	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		p1.childNodes.length
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "0" {
		t.Errorf("after deleteContents, childNodes.length = %q, want %q", val, "0")
	}
}

// ---------------------------------------------------------------------------
// Range.cloneContents
// ---------------------------------------------------------------------------

func TestRangeCloneContents(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		var frag = range.cloneContents();
		frag.nodeType === 11
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("cloneContents returns DocumentFragment: %q, want %q", val, "true")
	}
}

func TestRangeExtractContentsPartialNodes(t *testing.T) {
	d := domtest.Parse(`<html><body><div id="root">A<b>B<i>CD</i>E</b><u>X</u>F</div></body></html>`)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var root = document.getElementById('root');
		var originalBold = root.querySelector('b');
		var originalItalic = root.querySelector('i');
		var originalUnderline = root.querySelector('u');
		var start = originalBold.firstChild;
		var end = root.lastChild;
		var range = document.createRange();
		range.setStart(start, 0);
		range.setEnd(end, 0);
		var fragment = range.extractContents();
		JSON.stringify([
			fragment.nodeType, fragment.textContent, fragment.firstChild.nodeName,
			fragment.firstChild.textContent, fragment.firstChild !== originalBold,
			fragment.firstChild.childNodes[1] === originalItalic,
			fragment.childNodes[1] === originalUnderline,
			fragment.lastChild.nodeType, fragment.lastChild.data,
			root.textContent, range.collapsed, range.startContainer === start,
			range.startOffset
		]);
	`)
	if err != nil {
		t.Fatalf("extract partially selected contents: %v", err)
	}
	want := `[11,"BCDEX","B","BCDE",true,true,true,3,"","AF",true,false,2]`
	if val != want {
		t.Fatalf("partial extract result = %s, want %s", val, want)
	}
}

func TestRangeCloneContentsPartialNodes(t *testing.T) {
	d := domtest.Parse(`<html><body><div id="root">A<b>B<i>CD</i>E</b><u>X</u>F</div></body></html>`)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var root = document.getElementById('root');
		var originalBold = root.querySelector('b');
		var originalItalic = root.querySelector('i');
		var originalUnderline = root.querySelector('u');
		var range = document.createRange();
		range.setStart(originalBold.firstChild, 0);
		range.setEnd(root.lastChild, 0);
		var fragment = range.cloneContents();
		JSON.stringify([
			fragment.nodeType, fragment.textContent, fragment.firstChild.nodeName,
			fragment.firstChild.textContent, fragment.firstChild !== originalBold,
			fragment.firstChild.childNodes[1] !== originalItalic,
			fragment.lastChild !== originalUnderline,
			root.textContent, root.querySelector('i') === originalItalic
		]);
	`)
	if err != nil {
		t.Fatalf("clone partially selected contents: %v", err)
	}
	want := `[11,"BCDEX","B","BCDE",true,true,true,"ABCDEXF",true]`
	if val != want {
		t.Fatalf("partial clone result = %s, want %s", val, want)
	}
}

func TestRangeCloneContentsPreservesFullyContainedElementAcrossPartialAncestors(t *testing.T) {
	d := domtest.Parse(`<html><body><div id="test"><p id="d">Yzabcdef</p><p id="e">Ghijklmn</p><!--Alphabet soup?--></div></body></html>`)
	r := newRangeRuntime(t, d)
	value, err := r.Evaluate(`(() => {
		const paragraphs = document.querySelectorAll("p");
		const comment = document.getElementById("test").lastChild;
		const range = document.createRange();
		range.setStart(paragraphs[0], 1);
		range.setEnd(comment, 8);
		const fragment = range.cloneContents();
		return JSON.stringify(Array.from(fragment.childNodes, node => [node.nodeName, node.id || "", node.data || node.textContent]));
	})()`)
	if err != nil {
		t.Fatalf("clone contents: %v", err)
	}
	if want := `[["P","d",""],["P","e","Ghijklmn"],["#comment","","Alphabet"]]`; value != want {
		t.Fatalf("cloned fully contained element = %s, want %s", value, want)
	}
}

func TestRangeExtractContentsFromDetachedCharacterData(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var text = document.createTextNode('abc');
		var range = document.createRange();
		range.setStart(text, 1);
		range.setEnd(text, 2);
		var fragment = range.extractContents();
		JSON.stringify([fragment.textContent, text.data, range.collapsed,
			range.startContainer === text, range.startOffset]);
	`)
	if err != nil {
		t.Fatalf("extract from detached character data: %v", err)
	}
	want := `["b","ac",true,true,1]`
	if val != want {
		t.Fatalf("detached character-data extract = %s, want %s", val, want)
	}
}

// ---------------------------------------------------------------------------
// Range.insertNode
// ---------------------------------------------------------------------------

func TestRangeInsertNode(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	_, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.collapse(true);
		var span = document.createElement('span');
		span.textContent = 'X';
		range.insertNode(span);
	`)
	if err != nil {
		t.Fatal(err)
	}

	// p1 should now have 2 children: the span and the text node.
	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		p1.childNodes.length
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "2" {
		t.Errorf("after insertNode, childNodes.length = %q, want %q", val, "2")
	}
}

// ---------------------------------------------------------------------------
// Range.isPointInRange
// ---------------------------------------------------------------------------

func TestRangeIsPointInRange(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.isPointInRange(p1, 0)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("isPointInRange(p1, 0): %q, want %q", val, "true")
	}
}

func TestRangeIsPointInRangeOutside(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.isPointInRange(p2, 0)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "false" {
		t.Errorf("isPointInRange(p2, 0) outside: %q, want %q", val, "false")
	}
}

// ---------------------------------------------------------------------------
// Range.comparePoint
// ---------------------------------------------------------------------------

func TestRangeComparePoint(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		range.comparePoint(p1, 0)
	`)
	if err != nil {
		t.Fatal(err)
	}
	// 0 means the point is in the range.
	if val != "0" {
		t.Errorf("comparePoint(p1, 0) = %q, want %q", val, "0")
	}
}

// ---------------------------------------------------------------------------
// Range.compareBoundaryPoints
// ---------------------------------------------------------------------------

func TestRangeCompareBoundaryPoints(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		var range1 = document.createRange();
		range1.selectNodeContents(p1);
		var range2 = document.createRange();
		range2.selectNodeContents(p2);
		range1.compareBoundaryPoints(0, range2)
	`)
	if err != nil {
		t.Fatal(err)
	}
	// START_TO_START: range1 start < range2 start, so result should be -1.
	if val != "-1" {
		t.Errorf("compareBoundaryPoints(START_TO_START) = %q, want %q", val, "-1")
	}
}

func TestRangeCompareBoundaryPointsAncestorAndDescendant(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var p = document.getElementById('p1');
		var text = p.firstChild;
		var elementRange = document.createRange();
		elementRange.setStart(p, 0);
		elementRange.setEnd(p, 1);
		var textRange = document.createRange();
		textRange.setStart(text, 0);
		textRange.setEnd(text, text.length);
		JSON.stringify([
			elementRange.compareBoundaryPoints(0, textRange),
			elementRange.compareBoundaryPoints(2, textRange),
			elementRange.isPointInRange(text, 0),
			elementRange.isPointInRange(text, text.length)
		]);
	`)
	if err != nil {
		t.Fatalf("compare ancestor and descendant boundaries: %v", err)
	}
	if val != `[-1,1,true,true]` {
		t.Fatalf("ancestor/descendant comparisons = %s", val)
	}
}

func TestRangeDeleteContentsAcrossTextNodes(t *testing.T) {
	d := domtest.Parse(`<html><body><div id="root">A<b>B<i>DEF</i>G</b>H</div></body></html>`)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var root = document.getElementById('root');
		var first = root.firstChild.nextSibling.firstChild;
		var last = root.firstChild.nextSibling.lastChild.previousSibling.firstChild;
		var range = document.createRange();
		range.setStart(first, 1);
		range.setEnd(last, 2);
		range.deleteContents();
		JSON.stringify([root.innerHTML, root.textContent, range.collapsed,
			range.startContainer === first, range.startOffset]);
	`)
	if err != nil {
		t.Fatalf("delete range across text nodes: %v", err)
	}
	want := `["A<b>B<i>F</i>G</b>H","ABFGH",true,false,1]`
	if val != want {
		t.Fatalf("cross-node deleteContents = %s, want %s", val, want)
	}
}

// ---------------------------------------------------------------------------
// window.getSelection()
// ---------------------------------------------------------------------------

func TestGetSelection(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("typeof window.getSelection()")
	if err != nil {
		t.Fatal(err)
	}
	if val != "object" {
		t.Errorf("getSelection() returned %q, want %q", val, "object")
	}
}

func TestSelectionIsCollapsed(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("window.getSelection().isCollapsed")
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("selection.isCollapsed = %q, want %q", val, "true")
	}
}

func TestSelectionRangeCount(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("window.getSelection().rangeCount")
	if err != nil {
		t.Fatal(err)
	}
	if val != "0" {
		t.Errorf("selection.rangeCount = %q, want %q", val, "0")
	}
}

func TestSelectionType(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate("window.getSelection().type")
	if err != nil {
		t.Fatal(err)
	}
	if val != "None" {
		t.Errorf("selection.type = %q, want %q", val, "None")
	}
}

// ---------------------------------------------------------------------------
// Selection.addRange / getRangeAt
// ---------------------------------------------------------------------------

func TestSelectionAddRange(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		sel.addRange(range);
		sel.rangeCount
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "1" {
		t.Errorf("after addRange, rangeCount = %q, want %q", val, "1")
	}
}

func TestSelectionGetRangeAt(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		sel.addRange(range);
		var retrieved = sel.getRangeAt(0);
		retrieved.startContainer === p1
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("getRangeAt(0).startContainer === p1: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Selection.collapse
// ---------------------------------------------------------------------------

func TestSelectionCollapse(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		sel.collapse(p1, 0);
		sel.isCollapsed
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("after collapse, isCollapsed = %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Selection.selectAllChildren
// ---------------------------------------------------------------------------

func TestSelectionSelectAllChildren(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		sel.selectAllChildren(p1);
		sel.rangeCount === 1 && sel.type === 'Range'
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("selectAllChildren: %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Selection.toString()
// ---------------------------------------------------------------------------

func TestSelectionToString(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		sel.selectAllChildren(p1);
		sel.toString()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "Hello World" {
		t.Errorf("selection.toString() = %q, want %q", val, "Hello World")
	}
}

// ---------------------------------------------------------------------------
// Selection.removeAllRanges
// ---------------------------------------------------------------------------

func TestSelectionRemoveAllRanges(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var range = document.createRange();
		range.selectNodeContents(p1);
		sel.addRange(range);
		sel.removeAllRanges();
		sel.rangeCount
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "0" {
		t.Errorf("after removeAllRanges, rangeCount = %q, want %q", val, "0")
	}
}

// ---------------------------------------------------------------------------
// Selection.extend
// ---------------------------------------------------------------------------

func TestSelectionExtend(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		sel.collapse(p1, 0);
		sel.extend(p2, 1);
		sel.type === 'Range'
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("after extend, type = %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Selection.collapseToStart / collapseToEnd
// ---------------------------------------------------------------------------

func TestSelectionCollapseToStart(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		sel.selectAllChildren(p1);
		sel.collapseToStart();
		sel.isCollapsed
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("after collapseToStart, isCollapsed = %q, want %q", val, "true")
	}
}

func TestSelectionCollapseToEnd(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		sel.selectAllChildren(p1);
		sel.collapseToEnd();
		sel.isCollapsed
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("after collapseToEnd, isCollapsed = %q, want %q", val, "true")
	}
}

// ---------------------------------------------------------------------------
// Selection.containsNode
// ---------------------------------------------------------------------------

func TestSelectionContainsNode(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		sel.selectAllChildren(p1);
		sel.containsNode(p1, false)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("containsNode(p1) = %q, want %q", val, "true")
	}

	val, err = r.Evaluate(`
		sel.containsNode(p2, false)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "false" {
		t.Errorf("containsNode(p2) = %q, want %q", val, "false")
	}
}

// ---------------------------------------------------------------------------
// Selection.anchorNode / focusNode
// ---------------------------------------------------------------------------

func TestSelectionAnchorFocusNodes(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)

	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		sel.collapse(p1, 0);
		sel.extend(p2, 1);
		sel.anchorNode === p1 && sel.focusNode === p2
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true" {
		t.Errorf("anchorNode/focusNode: %q, want %q", val, "true")
	}
}

func TestSelectionSetBaseAndExtentPreservesDirectionAndRangeOrder(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var sel = window.getSelection();
		var p1 = document.getElementById('p1');
		var p2 = document.getElementById('p2');
		sel.setBaseAndExtent(p1, 0, p2, 1);
		var forward = sel.anchorNode === p1 && sel.anchorOffset === 0 &&
		  sel.focusNode === p2 && sel.focusOffset === 1 &&
		  sel.getRangeAt(0).startContainer === p1 && sel.getRangeAt(0).endContainer === p2;
		sel.setBaseAndExtent(p2, 1, p1, 0);
		forward + '|' + (sel.anchorNode === p2 && sel.anchorOffset === 1 &&
		  sel.focusNode === p1 && sel.focusOffset === 0 &&
		  sel.getRangeAt(0).startContainer === p1 && sel.getRangeAt(0).endContainer === p2)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true|true" {
		t.Fatalf("setBaseAndExtent forward/backward state = %q, want true|true", val)
	}
}

func TestSelectionSetBaseAndExtentValidatesBoundaryPoints(t *testing.T) {
	d := rangeDOM(t)
	r := newRangeRuntime(t, d)
	val, err := r.Evaluate(`
		var sel = getSelection(), p = document.getElementById('p1');
		var offset = false, root = false;
		try { sel.setBaseAndExtent(p, 999, p, 0); } catch (e) { offset = e.name === 'IndexSizeError'; }
		try { sel.setBaseAndExtent(p, 0, document.createElement('div'), 0); } catch (e) { root = e.name === 'WrongDocumentError'; }
		offset + '|' + root
	`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "true|true" {
		t.Fatalf("setBaseAndExtent boundary validation = %q, want true|true", val)
	}
}

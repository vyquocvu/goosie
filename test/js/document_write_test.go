package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestDocumentWriteAfterParsingClearsAndWrites(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	r.FireDocumentLifecycle()

	if err := r.Run(`document.write('<p id="new">written</p>')`, "test"); err != nil {
		t.Fatalf("document.write: %v", err)
	}

	val, err := r.Evaluate(`document.getElementById('main')`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "null" {
		t.Errorf("old element #main still present after document.write, got %q", val)
	}

	val, err = r.Evaluate(`document.getElementById('new')`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val == "null" {
		t.Errorf("new element #new not found after document.write")
	}
}

func TestDocumentWriteDuringParsing(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write('<span class="injected">hi</span>')`, "test"); err != nil {
		t.Fatalf("document.write during loading: %v", err)
	}

	val, err := r.Evaluate(`document.querySelector('span.injected') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("expected span.injected to be in the document, got %q", val)
	}

	val, err = r.Evaluate(`document.getElementById('main') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("original #main element missing after document.write during loading")
	}
}

func TestParserScriptCombinesChunkedDocumentWriteInput(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	if err := r.RunParserScript(`var markup = '<div id="written-one"><a>one</a></div><div id="written-two"><p>two</p></div>'; for (var i = 0; i < markup.length; i++) document.write(markup.charAt(i));`, "parser-script"); err != nil {
		t.Fatalf("parser script: %v", err)
	}
	got, err := r.Evaluate(`document.querySelectorAll('div#written-one, div#written-two').length`)
	if err != nil {
		t.Fatalf("evaluate written divs: %v", err)
	}
	if got != "2" {
		t.Fatalf("chunked parser writes produced %s matching divs, want 2", got)
	}
	got, err = r.Evaluate(`document.getElementById('written-two').firstChild.firstChild.textContent`)
	if err != nil {
		t.Fatalf("evaluate written content: %v", err)
	}
	if got != "two" {
		t.Fatalf("chunked parser write content = %q, want two", got)
	}
}

func TestParserScriptSupportsHTMLLegacyComments(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	source := `<!-- HTML-compatible scripts may begin with this legacy comment
window.legacyCommentScriptRan = true;
--> this line is also commented out
window.legacyCommentString = "<!-- stays text";`
	if err := r.RunParserScript(source, "legacy-comment-script"); err != nil {
		t.Fatalf("parser script with legacy comments: %v", err)
	}
	for expression, want := range map[string]string{
		"String(window.legacyCommentScriptRan)":            "true",
		"window.legacyCommentString":                       "<!-- stays text",
		"String(typeof window.thisLineIsAlsoCommentedOut)": "undefined",
	} {
		if got, err := r.Evaluate(expression); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, err, want)
		}
	}
}

func TestDocumentWriteRunsDeclarativeShadowRootParserScripts(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		document.open();
		document.write('<div id="host"><template shadowrootmode="open" shadowrootcustomelementregistry><script>' +
			'window.writeParserRan = true; window.writeRegistry = new CustomElementRegistry(); writeRegistry.initialize(host.shadowRoot); ' +
			'writeRegistry.define("write-defined", class extends HTMLElement {});' +
			'</' + 'script><write-defined id="candidate"></write-defined></template></div>');
		document.close();
	`, "document-write-declarative-shadow-script.js"); err != nil {
		t.Fatalf("document.write with declarative shadow script: %v", err)
	}
	for expression, want := range map[string]string{
		"String(window.writeParserRan)": "true",
		"String(host.shadowRoot.querySelector('#candidate').customElementRegistry === writeRegistry)": "true",
	} {
		if got, err := r.Evaluate(expression); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, err, want)
		}
	}
}

func TestIframeDocumentWriteRunsScriptsInChildWindow(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		window.frame = document.body.appendChild(document.createElement('iframe'));
		var childDoc = frame.contentDocument;
		childDoc.open();
		childDoc.write('<!doctype html><html><body><div id="host"><template shadowrootmode="open" shadowrootcustomelementregistry><script>' +
			'window.childScriptRan = true; window.childRegistry = new CustomElementRegistry(); childRegistry.initialize(host.shadowRoot); ' +
			'childRegistry.define("throws-element", class extends HTMLElement { constructor() { super(); throw TypeError; } }); ' +
			'childRegistry.define("returns-element", class extends HTMLElement { constructor() { super(); return document.createElement("span"); } });' +
			'</' + 'script><throws-element id="throws"></throws-element><returns-element id="returns"></returns-element></template></div></body></html>');
		childDoc.close();
		window.childHostPresent = frame.contentWindow.host !== undefined;
		window.childScriptPresent = frame.contentWindow.childScriptRan === true;
	`, "iframe-document-write-script.js"); err != nil {
		t.Fatalf("iframe document.write: %v", err)
	}
	for expression, want := range map[string]string{
		"String(window.childHostPresent)":   "true",
		"String(window.childScriptPresent)": "true",
		"String(frame.contentWindow.host.shadowRoot.querySelector('#throws').customElementRegistry === null)":                               "false",
		"String(frame.contentWindow.host.shadowRoot.querySelector('#throws').customElementRegistry === frame.contentWindow.childRegistry)":  "true",
		"String(frame.contentWindow.host.shadowRoot.querySelector('#returns').customElementRegistry === frame.contentWindow.childRegistry)": "true",
	} {
		if got, err := r.Evaluate(expression); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, err, want)
		}
	}
}

func TestDocumentWriteSynchronouslyUpgradesDefinedCustomElements(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		window.connectedChildren = -1;
		window.parserReactionOrder = [];
		class WrittenElement extends HTMLElement {
			constructor() {
				super();
				window.parserConstructorSawAttribute = this.hasAttribute('data-x');
			}
			attributeChangedCallback(name, oldValue, newValue) {
				window.parserReactionOrder.push(name + ':' + oldValue + '>' + newValue);
			}
			connectedCallback() {
				window.parserReactionOrder.push('connected:' + this.childNodes.length);
				window.connectedChildren = this.childNodes.length;
			}
		}
		WrittenElement.observedAttributes = ['data-x'];
		customElements.define('written-element', WrittenElement, { observedAttributes: ['data-x'] });
		class WrittenButton extends HTMLButtonElement {}
		customElements.define('written-button', WrittenButton, { extends: 'button' });
		document.write('<written-element data-x="yes">hello <b>world</b></written-element><button is="written-button">button</button>');
		window.writtenElement = document.querySelector('written-element');
		window.isWrittenElement = window.writtenElement instanceof WrittenElement;
		window.writtenButton = document.querySelector('button[is="written-button"]');
		window.isWrittenButton = window.writtenButton instanceof WrittenButton;
		window.directElement = document.createElement('written-element');
		window.isDirectElement = window.directElement instanceof WrittenElement;
	`, "document-write-custom-element.js"); err != nil {
		t.Fatalf("document.write parser callbacks: %v", err)
	}
	for expression, want := range map[string]string{
		"String(window.isWrittenElement)":                 "true",
		"String(window.isWrittenButton)":                  "true",
		"String(window.isDirectElement)":                  "true",
		"String(window.parserConstructorSawAttribute)":    "false",
		"String(window.connectedChildren)":                "0",
		"String(window.parserReactionOrder.join(','))":    "data-x:null>yes,connected:0",
		"String(window.writtenElement.childNodes.length)": "2",
	} {
		if got, err := r.Evaluate(expression); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", expression, got, err, want)
		}
	}
}

func TestDocumentOpenWriteQueuesDisconnectAndConstructsOnce(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		window.writeReactions=[];
		class WrittenElement extends HTMLElement {
			constructor(){ super(); writeReactions.push('constructed'); }
			connectedCallback(){ writeReactions.push('connected'); }
			disconnectedCallback(){ writeReactions.push('disconnected'); }
		}
		customElements.define('x-document-write', WrittenElement);
		var old=document.createElement('x-document-write');
		document.body.appendChild(old);
		document.open();
		var afterOpen=writeReactions.join(',');
		document.write('<x-document-write></x-document-write>');
		window.documentWriteReactionSummary=afterOpen+'|'+writeReactions.join(',');
	`, "document-open-write-reactions.js"); err != nil {
		t.Fatalf("document open/write lifecycle: %v", err)
	}
	got, err := r.Evaluate("window.documentWriteReactionSummary")
	if err != nil {
		t.Fatalf("evaluate document open/write summary: %v", err)
	}
	want := "constructed,connected,disconnected|constructed,connected,disconnected,constructed,connected"
	if got != want {
		t.Fatalf("document open/write reactions = %q, want %q", got, want)
	}
}

func TestDocumentWriteConcatenatesArguments(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	if err := r.Run(`document.write('<span id="joined">', 12, null, undefined, '</span>')`, "test"); err != nil {
		t.Fatalf("document.write: %v", err)
	}
	got, err := r.Evaluate(`document.getElementById('joined').textContent`)
	if err != nil {
		t.Fatalf("evaluate written content: %v", err)
	}
	if got != "12nullundefined" {
		t.Fatalf("document.write concatenated value = %q, want 12nullundefined", got)
	}
}

func TestDetachedDocumentOpenWriteCloseAndExistingChildren(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		var frame = document.createElement('iframe');
		document.body.appendChild(frame);
		var doc = frame.contentDocument;
		var api = typeof doc.open === 'function' && typeof doc.write === 'function' && typeof doc.close === 'function';
		doc.open();
		doc.write('<!doctype html>');
		var doctypeAppearsDuringWrite = doc.doctype !== null;
		var onlyDoctypeBeforeClose = doc.childNodes.length === 1 && doc.firstChild === doc.doctype;
		doc.close();
		var closeCompletesHTMLDocument = doc.documentElement !== null;
		doc.open();
		var existing = doc.createElement('div');
		doc.appendChild(existing);
		doc.write('<!doctype html>');
		var preservedExistingElement = doc.firstChild === existing && doc.doctype === null;
		doc.close();
	`, "test"); err != nil {
		t.Fatalf("detached document writer APIs: %v", err)
	}
	for _, expression := range []string{"api", "doctypeAppearsDuringWrite", "onlyDoctypeBeforeClose", "closeCompletesHTMLDocument", "preservedExistingElement"} {
		got, err := r.Evaluate(expression)
		if err != nil {
			t.Fatalf("evaluate %s: %v", expression, err)
		}
		if got != "true" {
			t.Errorf("%s = %q, want true", expression, got)
		}
	}
}

func TestDocumentWriteEmptyString(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write('')`, "test"); err != nil {
		t.Fatalf("document.write(''): %v", err)
	}

	val, err := r.Evaluate(`document.getElementById('main') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("document.write('') should be a no-op, but #main is missing")
	}
}

func TestDocumentWriteNoDOM(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	if err := r.Run(`document.write('hello')`, "test"); err != nil {
		t.Fatalf("document.write without DOM: %v", err)
	}
}

func TestDocumentWritelnAppendsNewline(t *testing.T) {
	doc := dom.NewDocument()
	r := newDOMRuntime(t, doc)

	if err := r.Run(`document.writeln('line1')`, "test"); err != nil {
		t.Fatalf("document.writeln: %v", err)
	}

	val, err := r.Evaluate(`document.body.textContent`)
	if err != nil {
		t.Fatalf("evaluate body.textContent: %v", err)
	}
	if !strings.Contains(val, "line1") {
		t.Errorf("body text should contain 'line1', got %q", val)
	}
	if !strings.Contains(val, "\n") {
		t.Errorf("writeln should append newline, body text: %q", val)
	}
}

func TestDocumentWritelnAfterParsing(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	r.FireDocumentLifecycle()

	if err := r.Run(`document.writeln('<p>written</p>')`, "test"); err != nil {
		t.Fatalf("document.writeln after parse: %v", err)
	}

	val, err := r.Evaluate(`document.querySelector('p') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("writeln after parsing should insert content")
	}
}

func TestDocumentWriteMultipleElements(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write('<div id="d1">one</div><div id="d2">two</div>')`, "test"); err != nil {
		t.Fatalf("document.write multiple: %v", err)
	}

	val, err := r.Evaluate(`document.getElementById('d1') !== null && document.getElementById('d2') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("expected both #d1 and #d2 to be present")
	}
}

func TestDocumentWriteNestedHTML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write('<ul><li class="item">A</li><li class="item">B</li></ul>')`, "test"); err != nil {
		t.Fatalf("document.write nested: %v", err)
	}

	val, err := r.Evaluate(`document.querySelectorAll('li.item').length`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "2" {
		t.Errorf("expected 2 li.item elements, got %q", val)
	}
}

func TestDocumentWriteCoercesArgument(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write(42)`, "test"); err != nil {
		t.Fatalf("document.write(42): %v", err)
	}

	val, err := r.Evaluate(`document.body.textContent.indexOf('42') >= 0`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("document.write(42) should insert text '42'")
	}
}

func TestDocumentWriteUpdatesBodyReference(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	r.FireDocumentLifecycle()

	if err := r.Run(`document.write('<p>new content</p>')`, "test"); err != nil {
		t.Fatalf("document.write: %v", err)
	}

	val, err := r.Evaluate(`document.body !== null && document.body.tagName === 'BODY'`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("document.body should be a BODY element after document.write, got %q", val)
	}
}

func TestDocumentWriteSpecialCharacters(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`document.write('<p id="special">&amp; &lt; &gt;</p>')`, "test"); err != nil {
		t.Fatalf("document.write special chars: %v", err)
	}

	val, err := r.Evaluate(`document.getElementById('special') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("element with special characters not found")
	}
}

func TestDocumentWriteFreshDOM(t *testing.T) {
	doc := dom.NewDocument()
	r := newDOMRuntime(t, doc)

	if err := r.Run(`document.write('<p>hello</p>')`, "test"); err != nil {
		t.Fatalf("document.write on fresh DOM: %v", err)
	}

	val, err := r.Evaluate(`document.body !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("document.body should exist after document.write on fresh DOM")
	}

	val, err = r.Evaluate(`document.querySelector('p') !== null`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "true" {
		t.Errorf("paragraph should exist in document after write on fresh DOM")
	}
}

func TestDocumentWriteReturnsUndefined(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	val, err := r.Evaluate(`typeof document.write('<p>test</p>')`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "undefined" {
		t.Errorf("document.write should return undefined, got %q", val)
	}
}

func TestDocumentWritelnReturnsUndefined(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	val, err := r.Evaluate(`typeof document.writeln('test')`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "undefined" {
		t.Errorf("document.writeln should return undefined, got %q", val)
	}
}

func TestDocumentWriteAfterParsingClearsEverything(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)
	r.FireDocumentLifecycle()

	val, _ := r.Evaluate(`document.getElementById('main') !== null`)
	if val != "true" {
		t.Fatalf("precondition: #main should exist before document.write")
	}

	if err := r.Run(`document.write('<div id="replacement">new</div>')`, "test"); err != nil {
		t.Fatalf("document.write: %v", err)
	}

	val, _ = r.Evaluate(`document.getElementById('main')`)
	if val != "null" {
		t.Errorf("old #main should be cleared, got %q", val)
	}

	val, _ = r.Evaluate(`document.getElementById('replacement') !== null`)
	if val != "true" {
		t.Errorf("#replacement should exist after document.write")
	}
}

func TestDocumentWritelnTextContentIncludesNewline(t *testing.T) {
	doc := dom.NewDocument()
	r := newDOMRuntime(t, doc)

	if err := r.Run(`document.writeln('hello')`, "test"); err != nil {
		t.Fatalf("writeln: %v", err)
	}

	val, err := r.Evaluate(`document.body !== null`)
	if err != nil {
		t.Fatalf("evaluate body existence: %v", err)
	}
	if val != "true" {
		t.Fatalf("document.body should exist after writeln on fresh DOM")
	}

	val, err = r.Evaluate(`document.body.textContent`)
	if err != nil {
		t.Fatalf("evaluate textContent: %v", err)
	}
	if !strings.Contains(val, "hello") {
		t.Errorf("body text should contain 'hello', got %q", val)
	}
	if !strings.Contains(val, "\n") {
		t.Errorf("writeln should append newline, body text: %q", val)
	}
}

func TestDocumentWriteExists(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	val, err := r.Evaluate(`typeof document.write`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("document.write should be a function, got %q", val)
	}

	val, err = r.Evaluate(`typeof document.writeln`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("document.writeln should be a function, got %q", val)
	}
}

func TestDocumentWriteNoDOMExists(t *testing.T) {
	r := newRuntime(t, js.Options{URL: "https://example.test/"})

	val, err := r.Evaluate(`typeof document.write`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("document.write should be a function even without DOM, got %q", val)
	}

	val, err = r.Evaluate(`typeof document.writeln`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "function" {
		t.Errorf("document.writeln should be a function even without DOM, got %q", val)
	}
}

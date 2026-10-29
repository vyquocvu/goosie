package engine_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestScriptExecutionSetsGlobal(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><head><script>var greeting = "hello";</script></head><body></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	v, ok := rt.Global("greeting")
	if !ok {
		t.Fatal("script did not set global greeting")
	}
	if v.Str != "hello" {
		t.Errorf("greeting = %q, want %q", v.Str, "hello")
	}
}

func TestParserDocumentWriteConstructsCustomElementBeforeChildren(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><body><script>
		var writtenConnectedChildCount = -1;
		class WrittenParserElement extends HTMLElement {
			connectedCallback() { writtenConnectedChildCount = this.childNodes.length; }
		}
		customElements.define('written-parser-element', WrittenParserElement);
		document.write('<written-parser-element>hello <b>world</b></written-parser-element>');
		window.writtenParserElementVisibleSynchronously = document.querySelector('written-parser-element') !== null;
	</script><script>
		window.writtenParserElement = document.querySelector('written-parser-element');
		window.writtenParserElementIsInstance = window.writtenParserElement instanceof WrittenParserElement;
	</script></body>`
	session, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if value, ok := rt.Global("writtenParserElementIsInstance"); !ok || !value.Bool {
		t.Fatalf("document.write element is a custom-element instance: %+v (present=%v)", value, ok)
	}
	if value, ok := rt.Global("writtenParserElementVisibleSynchronously"); !ok || !value.Bool {
		t.Fatalf("document.write element was not synchronously inserted: %+v (present=%v)", value, ok)
	}
	if value, ok := rt.Global("writtenConnectedChildCount"); !ok || value.Num != 0 {
		t.Fatalf("connectedCallback child count = %+v (present=%v), want 0", value, ok)
	}
}

func TestCompletedParserDocumentExposesImpliedBodyToJavaScript(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	session, err := engine.NewSession(`<!doctype html><meta charset="utf-8"><title>upload</title><script>void 0</script>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := rt.Run(`window.impliedBodyConnected = document.body !== null && document.body.parentNode === document.documentElement`, "implied-body"); err != nil {
		t.Fatal(err)
	}
	value, ok := rt.Global("impliedBodyConnected")
	if !ok || !value.Bool {
		t.Fatal("completed parsed document did not expose its implied body to JavaScript")
	}
}

func TestMisnestedFormInsideTemplateInnerHTML(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<script>0</script><div>A<template><br>BC<form>D<div>E</form>F</div>G</template>H</div><script>window.parsedMarkup = document.querySelector('div').innerHTML.trim();</script>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	value, ok := rt.Global("parsedMarkup")
	if !ok {
		t.Fatal("parser script did not record wrapper.innerHTML")
	}
	const want = `A<template><br>BC<form>D<div>E</div></form>FG</template>H`
	if value.Str != want {
		t.Fatalf("wrapper.innerHTML = %q, want %q", value.Str, want)
	}
}

func TestNetworkDocumentEncodingIsVisibleBeforeScriptsRun(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<meta charset="windows-1252"><script>var observedCharset = document.characterSet;</script>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithDocumentEncoding("utf-8"),
		engine.WithJS(rt),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got, want := sess.Doc.CharacterSet, "UTF-8"; got != want {
		t.Errorf("Document.CharacterSet = %q, want %q", got, want)
	}
	value, ok := rt.Global("observedCharset")
	if !ok {
		t.Fatal("script did not read document.characterSet")
	}
	if got, want := value.Str, "UTF-8"; got != want {
		t.Errorf("script observed characterSet = %q, want %q", got, want)
	}
}

// qc:BR-088
func TestScriptsRunInDocumentOrder(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>var order = "first";</script>
<script>order += ",second";</script>
<script>order += ",third";</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	v, ok := rt.Global("order")
	if !ok {
		t.Fatal("scripts did not set global order")
	}
	if v.Str != "first,second,third" {
		t.Errorf("order = %q, want %q", v.Str, "first,second,third")
	}
}

func TestParserScriptRunsBeforeFollowingInputIsProcessed(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><script>window.beforeFollowingNode = document.getElementById('later') === null;</script><p id="later"></p><script>window.followingNodeExists = document.getElementById('later') !== null;</script>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	for name, want := range map[string]bool{"beforeFollowingNode": true, "followingNodeExists": true} {
		got, ok := rt.Global(name)
		if !ok || got.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, got.Bool, ok, want)
		}
	}
}

func TestClosedSVGParserScriptRunsBeforeFollowingHTMLScript(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<svg><script>window.svgParserRan = true;</script></svg><script>window.svgObservedBeforeNextScript = window.svgParserRan === true;</script>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	for name, want := range map[string]bool{
		"svgParserRan":                true,
		"svgObservedBeforeNextScript": true,
	} {
		value, ok := rt.Global(name)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, value.Bool, ok, want)
		}
	}
}

func TestParserScriptRunsInsideDeclarativeShadowRootTemplate(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><div id="host"><template shadowrootmode="open"><script>window.dsdScriptRan = true; window.dsdSawFollowingNode = document.getElementById('later') === null; window.dsdSawShadowRoot = document.getElementById('host').shadowRoot !== null;</script><p id="inside"></p></template></div><p id="later"></p>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	for name, want := range map[string]bool{
		"dsdScriptRan":        true,
		"dsdSawFollowingNode": true,
		"dsdSawShadowRoot":    true,
	} {
		value, ok := rt.Global(name)
		if !ok || value.Bool != want {
			t.Errorf("%s = %v (present %v), want %v", name, value.Bool, ok, want)
		}
	}
}

func TestScriptCanChangeDocumentTitle(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/", Title: "Initial"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><head><script>document.title = "Changed by JS";</script></head><body></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := sess.JSTitle(); got != "Changed by JS" {
		t.Errorf("JSTitle() = %q, want %q", got, "Changed by JS")
	}
}

func TestBrokenScriptDoesNotStopRendering(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>throw new Error("boom");</script>
<script>var afterError = true;</script>
<p>Content</p>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("session build failed after broken script: %v", err)
	}
	defer sess.Close()

	v, ok := rt.Global("afterError")
	if !ok || !v.Bool {
		t.Error("script after a throwing script did not run")
	}
}

func TestConsoleOutputReachesHostWriter(t *testing.T) {
	var buf bytes.Buffer
	rt, err := js.New(js.Options{URL: "https://example.test/", Console: &buf})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!DOCTYPE html><html><body>
<script>console.log("from page");</script>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if got := strings.TrimSpace(buf.String()); got != "from page" {
		t.Errorf("console output = %q, want %q", got, "from page")
	}
}

func TestNoJSRuntimeSkipsScriptExecution(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
<script>var shouldNotExist = true;</script>
<p>Content</p>
</body></html>`
	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if sess.JSTitle() != "" {
		t.Error("JSTitle() should be empty without a JS runtime")
	}
}

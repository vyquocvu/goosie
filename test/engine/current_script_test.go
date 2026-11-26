package engine_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func newCurrentScriptRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	return rt
}

func currentScriptGlobalString(t *testing.T, rt *js.Runtime, name, want string) {
	t.Helper()
	got, ok := rt.Global(name)
	if !ok || got.Kind != js.KindString || got.Str != want {
		t.Fatalf("%s = %+v, present %v; want %q", name, got, ok, want)
	}
}

func currentScriptGlobalBool(t *testing.T, rt *js.Runtime, name string, want bool) {
	t.Helper()
	got, ok := rt.Global(name)
	if !ok || got.Kind != js.KindBool || got.Bool != want {
		t.Fatalf("%s = %+v, present %v; want %v", name, got, ok, want)
	}
}

func currentScriptGlobalNull(t *testing.T, rt *js.Runtime, name string) {
	t.Helper()
	got, ok := rt.Global(name)
	if !ok || got.Kind != js.KindNull {
		t.Fatalf("%s = %+v, present %v; want null", name, got, ok)
	}
}

func currentScriptIsNullOutsideExecution(t *testing.T, rt *js.Runtime) {
	t.Helper()
	got, err := rt.Evaluate(`document.currentScript`)
	if err != nil || got != "null" {
		t.Fatalf("document.currentScript outside execution = %q, %v; want null", got, err)
	}
}

func TestCurrentScriptInlineExternalAndDefer(t *testing.T) {
	rt := newCurrentScriptRuntime(t)
	fetcher := func(base, src string) ([]byte, error) {
		switch src {
		case "https://example.test/external.js":
			return []byte(`
				window.externalIdentity = document.currentScript === document.getElementById('external');
				window.externalSrc = document.currentScript.src;
				window.externalRawSrc = document.currentScript.getAttribute('src');
				window.externalAttribute = document.currentScript.getAttribute('data-probe');
			`), nil
		case "https://example.test/deferred.js":
			return []byte(`
				window.deferredIdentity = document.currentScript === document.getElementById('deferred');
				window.deferredSrc = document.currentScript.src;
				window.deferredRawSrc = document.currentScript.getAttribute('src');
				window.deferredAttribute = document.currentScript.getAttribute('data-probe');
			`), nil
		default:
			return nil, fmt.Errorf("unexpected script fetch %q from %q", src, base)
		}
	}
	html := `<!doctype html><html><body>
		<script id="inline" data-probe="inline">
			window.inlineIdentity = document.currentScript === document.getElementById('inline');
			window.inlineSrc = document.currentScript.src;
			window.inlineAttribute = document.currentScript.getAttribute('data-probe');
			window.currentScriptHasNoSetter = Object.getOwnPropertyDescriptor(document, 'currentScript').set === undefined;
			document.currentScript = document.body;
			window.inlineReadOnly = document.currentScript === document.getElementById('inline');
		</script>
		<script id="external" src="/external.js" data-probe="external"></script>
		<script id="deferred" defer src="/deferred.js" data-probe="deferred"></script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	currentScriptGlobalBool(t, rt, "inlineIdentity", true)
	currentScriptGlobalBool(t, rt, "inlineReadOnly", true)
	currentScriptGlobalBool(t, rt, "currentScriptHasNoSetter", true)
	currentScriptGlobalString(t, rt, "inlineSrc", "")
	currentScriptGlobalString(t, rt, "inlineAttribute", "inline")
	currentScriptGlobalBool(t, rt, "externalIdentity", true)
	currentScriptGlobalString(t, rt, "externalSrc", "https://example.test/external.js")
	currentScriptGlobalString(t, rt, "externalRawSrc", "/external.js")
	currentScriptGlobalString(t, rt, "externalAttribute", "external")
	currentScriptGlobalBool(t, rt, "deferredIdentity", true)
	currentScriptGlobalString(t, rt, "deferredSrc", "https://example.test/deferred.js")
	currentScriptGlobalString(t, rt, "deferredRawSrc", "/deferred.js")
	currentScriptGlobalString(t, rt, "deferredAttribute", "deferred")
	currentScriptIsNullOutsideExecution(t, rt)
}

func TestCurrentScriptDynamic(t *testing.T) {
	rt := newCurrentScriptRuntime(t)
	fetcher := func(base, src string) ([]byte, error) {
		if base != "https://example.test/" || src != "https://example.test/dynamic.js" {
			return nil, fmt.Errorf("unexpected dynamic script fetch %q from %q", src, base)
		}
		return []byte(`
			window.dynamicIdentity = document.currentScript === document.getElementById('dynamic');
			window.dynamicSrc = document.currentScript.src;
			window.dynamicRawSrc = document.currentScript.getAttribute('src');
			window.dynamicAttribute = document.currentScript.getAttribute('data-probe');
		`), nil
	}
	html := `<!doctype html><html><body><script>
		var dynamic = document.createElement('script');
		dynamic.id = 'dynamic';
		dynamic.src = '/dynamic.js';
		dynamic.setAttribute('data-probe', 'dynamic');
		dynamic.addEventListener('load', function () { window.dynamicLoadCurrent = document.currentScript; });
		document.body.appendChild(dynamic);
	</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	sess.PumpJS(800)
	if errs := rt.DynamicScriptErrors(); len(errs) != 0 {
		t.Fatalf("dynamic script errors = %v", errs)
	}

	currentScriptGlobalBool(t, rt, "dynamicIdentity", true)
	currentScriptGlobalString(t, rt, "dynamicSrc", "https://example.test/dynamic.js")
	currentScriptGlobalString(t, rt, "dynamicRawSrc", "/dynamic.js")
	currentScriptGlobalString(t, rt, "dynamicAttribute", "dynamic")
	currentScriptGlobalNull(t, rt, "dynamicLoadCurrent")
	currentScriptIsNullOutsideExecution(t, rt)
}

func TestCurrentScriptClearsAfterThrow(t *testing.T) {
	rt := newCurrentScriptRuntime(t)
	html := `<!doctype html><html><body>
		<script id="throws">window.throwingCurrentId = document.currentScript.id; throw new Error('boom');</script>
		<script id="after-throw">window.afterThrowCurrentId = document.currentScript.id;</script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	if got := sess.ScriptErrorCount(); got != 1 {
		t.Fatalf("ScriptErrorCount = %d, want 1", got)
	}

	currentScriptGlobalString(t, rt, "throwingCurrentId", "throws")
	currentScriptGlobalString(t, rt, "afterThrowCurrentId", "after-throw")
	currentScriptIsNullOutsideExecution(t, rt)
}

func TestCurrentScriptClearsAfterTimeout(t *testing.T) {
	rt := newTimeoutRuntime(t)
	html := `<!doctype html><html><body>
		<script id="times-out">window.timeoutCurrentId = document.currentScript.id; while (true) {}</script>
		<script id="after-timeout">window.afterTimeoutCurrentId = document.currentScript.id;</script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	if got := sess.ScriptTimeoutCount(); got != 1 {
		t.Fatalf("ScriptTimeoutCount = %d, want 1", got)
	}

	currentScriptGlobalString(t, rt, "timeoutCurrentId", "times-out")
	currentScriptGlobalString(t, rt, "afterTimeoutCurrentId", "after-timeout")
	currentScriptIsNullOutsideExecution(t, rt)
}

func TestCurrentScriptCallbacksSeeNull(t *testing.T) {
	rt := newCurrentScriptRuntime(t)
	html := `<!doctype html><html><body><script>
		setTimeout(function () { window.callbackTimerCurrent = document.currentScript; }, 0);
		document.addEventListener('current-script-probe', function () { window.callbackEventCurrent = document.currentScript; });
		Promise.resolve().then(function () { window.callbackPromiseCurrent = document.currentScript; });
		new Promise(function (resolve) { resolve(); }).then(function () { window.callbackMicrotaskCurrent = document.currentScript; });
	</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })

	currentScriptGlobalNull(t, rt, "callbackPromiseCurrent")
	currentScriptGlobalNull(t, rt, "callbackMicrotaskCurrent")
	sess.PumpJS(800)
	currentScriptGlobalNull(t, rt, "callbackTimerCurrent")
	dom.DispatchEvent(&sess.Doc.Node, dom.NewTrustedEvent("current-script-probe", false, false))
	currentScriptGlobalNull(t, rt, "callbackEventCurrent")
	currentScriptIsNullOutsideExecution(t, rt)
}

func TestCurrentScriptModulesSeeNull(t *testing.T) {
	rt := newCurrentScriptRuntime(t)
	fetcher := func(base, src string) ([]byte, error) {
		if base != "https://example.test/" || src != "https://example.test/module.js" {
			return nil, fmt.Errorf("unexpected module fetch %q from %q", src, base)
		}
		return []byte(`window.externalModuleCurrent = document.currentScript;`), nil
	}
	html := `<!doctype html><html><body>
		<script type="module">window.inlineModuleCurrent = document.currentScript;</script>
		<script type="module" src="/module.js"></script>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithJS(rt), engine.WithScripts("https://example.test/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	if got := sess.ScriptErrorCount(); got != 0 {
		t.Fatalf("ScriptErrorCount = %d, want 0", got)
	}

	currentScriptGlobalNull(t, rt, "inlineModuleCurrent")
	currentScriptGlobalNull(t, rt, "externalModuleCurrent")
	currentScriptIsNullOutsideExecution(t, rt)
}

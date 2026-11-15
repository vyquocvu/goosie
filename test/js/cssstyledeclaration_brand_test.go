package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssStyleDeclBrandPage = `<!doctype html><html><head>
<style id="S">p.a1 { color: rgb(10,0,0); }</style>
</head><body><p id="P" class="a1" style="width:10px">hi</p></body></html>`

func cssStyleDeclBrandRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssStyleDeclBrandPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const cssStyleDeclBrandJS = `
	var rule = document.styleSheets[0].cssRules[0];
	var es = document.getElementById('P').style;
	var rs = rule.style;
	var ctorErr = '';
	try { new CSSStyleDeclaration(); } catch (e) { ctorErr = e.constructor.name + ': ' + e.message; }
`

// qc:BR-313
func TestCSSStyleDeclarationGlobalAndBranding(t *testing.T) {
	r := cssStyleDeclBrandRuntime(t)
	got := mustCSSOM(t, r, cssStyleDeclBrandJS+`
		[
			typeof CSSStyleDeclaration,
			CSSStyleDeclaration.name,
			CSSStyleDeclaration.length,
			String(Object.getOwnPropertyDescriptor(CSSStyleDeclaration, 'prototype').writable),
			String(Object.getOwnPropertyDescriptor(CSSStyleDeclaration, 'prototype').configurable),
			CSSStyleDeclaration.prototype === Object.getPrototypeOf(es),
			CSSStyleDeclaration.prototype === Object.getPrototypeOf(rs),
			CSSStyleDeclaration.prototype.constructor === CSSStyleDeclaration,
			Object.getPrototypeOf(CSSStyleDeclaration.prototype) === Object.prototype,
			ctorErr
		].join(' | ')
	`)
	want := "function | CSSStyleDeclaration | 0 | false | false | true | true | true | true | TypeError: Failed to construct 'CSSStyleDeclaration': Illegal constructor"
	if got != want {
		t.Errorf("CSSStyleDeclaration global surface\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-313
func TestInlineAndRuleStyleShareCSSStyleDeclarationBrand(t *testing.T) {
	r := cssStyleDeclBrandRuntime(t)
	got := mustCSSOM(t, r, cssStyleDeclBrandJS+`
		[
			Object.prototype.toString.call(es),
			String(es),
			Object.prototype.toString.call(rs),
			String(rs),
			String(es instanceof CSSStyleDeclaration),
			String(rs instanceof CSSStyleDeclaration),
			es.item(0),
			es.getPropertyValue('width'),
			rs.getPropertyValue('color')
		].join(' | ')
	`)
	want := "[object CSSStyleDeclaration] | [object CSSStyleDeclaration] | [object CSSStyleDeclaration] | [object CSSStyleDeclaration] | true | true | width | 10px | rgb(10, 0, 0)"
	if got != want {
		t.Errorf("element.style and rule.style brand parity\ngot:  %s\nwant: %s", got, want)
	}
}

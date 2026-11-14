package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssomWrapperPage = `<!doctype html><html><head>
<style id="A1">p.a1 { color: rgb(10,0,0); }</style>
</head><body><p class="a1">alpha</p></body></html>`

func cssomWrapperRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssomWrapperPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const wrapperJS = `
	var sheet = document.styleSheets[0];
	var rules = sheet.cssRules;
	var rule = rules[0];
	function own(o) { return Object.getOwnPropertyNames(o).join('+') }
	function keys(o) { return Object.keys(o).join('+') }
	function err(f) { try { return String(f()) } catch (e) { return e.constructor.name + ': ' + e.message } }
	function desc(iface, prop) { var d = Object.getOwnPropertyDescriptor(window[iface].prototype, prop); if (!d) return 'missing'; return [typeof d.get, String(d.set), String(d.enumerable), String(d.configurable)].join(',') }
`

// qc:BR-299
func TestStyleSheetWrapperOwnsNoInternals(t *testing.T) {
	r := cssomWrapperRuntime(t)
	got := mustCSSOM(t, r, wrapperJS+`
		[
			own(sheet),
			keys(sheet),
			JSON.stringify(sheet),
			Object.prototype.toString.call(sheet),
			String(sheet.constructor === CSSStyleSheet),
			sheet.ownerNode.id,
			String(sheet.cssRules.length),
			err(function () { return CSSStyleSheet.prototype[Symbol.toStringTag] })
		].join(' | ')
	`)
	want := " |  | {} | [object CSSStyleSheet] | true | A1 | 1 | CSSStyleSheet"
	if got != want {
		t.Errorf("CSSStyleSheet own internals\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-299
func TestCSSStyleRuleWrapperBrandsItsPrototype(t *testing.T) {
	r := cssomWrapperRuntime(t)
	got := mustCSSOM(t, r, wrapperJS+`
		[
			own(rule),
			keys(rule),
			JSON.stringify(rule),
			Object.prototype.toString.call(rule),
			rule.selectorText,
			String(rule.type),
			String(rule.parentStyleSheet === sheet),
			err(function () { return rule.parentRule }),
			String(rule.hasOwnProperty('selectorText')),
			String(rule.hasOwnProperty('cssText')),
			String(Object.getPrototypeOf(rule) === CSSStyleRule.prototype),
			String(Object.getPrototypeOf(CSSStyleRule.prototype) === CSSRule.prototype),
			String(Object.getPrototypeOf(CSSRule.prototype) === Object.prototype),
			String(CSSStyleRule.prototype[Symbol.toStringTag]),
			String(CSSRule.prototype[Symbol.toStringTag])
		].join(' | ')
	`)
	want := " |  | {} | [object CSSStyleRule] | p.a1 | 1 | true | null | false | false | true | true | true | CSSStyleRule | CSSRule"
	if got != want {
		t.Errorf("CSSStyleRule wrapper branding\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-299
func TestCSSRuleMembersLiveOnThePrototype(t *testing.T) {
	r := cssomWrapperRuntime(t)
	got := mustCSSOM(t, r, wrapperJS+`
		[
			desc('CSSRule', 'type'),
			desc('CSSRule', 'cssText'),
			desc('CSSRule', 'parentStyleSheet'),
			desc('CSSRule', 'parentRule'),
			desc('CSSStyleRule', 'selectorText'),
			rule.cssText,
			String(sheet.cssRules[0].selectorText === 'p.a1')
		].join(' | ')
	`)
	want := "function,undefined,true,true | function,undefined,true,true | function,undefined,true,true | function,undefined,true,true | function,undefined,true,true | p.a1 { color: rgb(10,0,0); } | true"
	if got != want {
		t.Errorf("CSSRule prototype accessors\ngot:  %s\nwant: %s", got, want)
	}
}

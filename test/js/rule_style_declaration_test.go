package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleStylePage = `<!doctype html><html><head>
<style id="A1">p.a1 { color: rgb(10,0,0); }</style>
<style id="B2">p.b2 { margin-top: 4px; font-weight: 700 !important; background: red; }</style>
</head><body><p class="a1">alpha</p><p class="b2">beta</p></body></html>`

func ruleStyleRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleStylePage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleStyleJS = `
	function err(f) { try { return String(f()) } catch (e) { return e.constructor.name + ': ' + e.message } }
	var one = document.styleSheets[0].cssRules[0];
	var multi = document.styleSheets[1].cssRules[0];
	function desc(iface, prop) { var d = Object.getOwnPropertyDescriptor(window[iface].prototype, prop); if (!d) return 'missing'; return [typeof d.get, String(d.set), String(d.enumerable), String(d.configurable)].join(',') }
`

// qc:BR-301
func TestRuleStyleIsACssStyleDeclaration(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, ruleStyleJS+`
		[
			typeof one.style,
			Object.prototype.toString.call(multi.style),
			String(multi.style),
			String(one.style === one.style),
			String(multi.style === multi.style),
			desc('CSSStyleRule', 'style'),
			err(function () { return Object.getOwnPropertyDescriptor(CSSStyleRule.prototype, 'style').get.call({}) })
		].join(' | ')
	`)
	want := "object | [object CSSStyleDeclaration] | [object CSSStyleDeclaration] | true | true | function,undefined,true,true | TypeError: Illegal invocation"
	if got != want {
		t.Errorf("rule.style shape\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-301
func TestRuleStyleReadsItsDeclarations(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, ruleStyleJS+`
		[
			one.style.cssText,
			String(one.style.length),
			one.style.getPropertyValue('color'),
			String(multi.style.length),
			multi.style.item(0),
			multi.style[1],
			multi.style.getPropertyValue('margin-top'),
			multi.style.marginTop,
			multi.style.getPropertyPriority('font-weight'),
			multi.style.getPropertyPriority('margin-top'),
			multi.style.cssText
		].join(' | ')
	`)
	want := "color: rgb(10, 0, 0); | 1 | rgb(10, 0, 0) | 3 | margin-top | font-weight | 4px | 4px | important |  | margin-top: 4px; background: red; font-weight: 700 !important;"
	if got != want {
		t.Errorf("rule.style declaration reads\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-301
func TestRuleStyleAbsentAndUnindexedReads(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, ruleStyleJS+`
		[
			multi.style.getPropertyValue('z-index'),
			multi.style.item(3),
			String(multi.style[3]),
			multi.style.getPropertyValue(''),
			String(multi.style.propertyIsEnumerable('0')),
			String(multi.style.propertyIsEnumerable('2'))
		].join(' | ')
	`)
	want := " |  | undefined |  | true | true"
	if got != want {
		t.Errorf("rule.style absent reads\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-309
func TestRuleStyleWritesMutateItsDeclarations(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, ruleStyleJS+`
		var rule = document.styleSheets[0].cssRules[0];
		var st = rule.style;
		var out = [];
		st.setProperty('margin-top', '4px', '');
		out.push(rule.cssText);
		out.push(String(st.length));
		st.setProperty('color', 'rgb(0,0,9)', '');
		out.push(rule.cssText);
		st.borderLeftWidth = '3px';
		out.push(st.getPropertyValue('border-left-width'));
		out.push(st.item(st.length - 1));
		out.push(st.removeProperty('margin-top'));
		out.push(rule.cssText);
		st.setProperty('border-left-width', '', '');
		out.push(rule.cssText);
		st.cssText = 'width: 5px; height: 6px';
		out.push(rule.cssText);
		out.push(String(st.length));
		out.push(st.getPropertyValue('width'));
		out.push(String(st === st));
		out.join(' | ')
	`)
	want := "p.a1 { color: rgb(10, 0, 0); margin-top: 4px; } | 2 | p.a1 { color: rgb(0, 0, 9); margin-top: 4px; } | 3px | border-left-width | 4px | p.a1 { color: rgb(0, 0, 9); border-left-width: 3px; } | p.a1 { color: rgb(0, 0, 9); } | p.a1 { width: 5px; height: 6px; } | 2 | 5px | true"
	if got != want {
		t.Errorf("rule.style writes\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-309
func TestRuleStylePropertyPriorityAndAbsentRemoval(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, ruleStyleJS+`
		var st = document.styleSheets[0].cssRules[0].style;
		var out = [];
		out.push(JSON.stringify(st.removeProperty('z-index')));
		st.setProperty('color', 'rgb(0, 0, 9)', 'important');
		out.push(st.getPropertyPriority('color'));
		out.push(st.getPropertyValue('color'));
		st.setProperty('color', 'rgb(1, 1, 1)', '');
		out.push(st.getPropertyPriority('color'));
		out.push(st.getPropertyValue('color'));
		out.join(' | ')
	`)
	want := "\"\" | important | rgb(0, 0, 9) |  | rgb(1, 1, 1)"
	if got != want {
		t.Errorf("rule.style priority/removal\ngot:  %s\nwant: %s", got, want)
	}
}

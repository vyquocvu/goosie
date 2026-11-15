package js_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssomIterPage = `<!doctype html><html><head>
<style id="I">.r { color: red; background: blue; }</style>
</head><body>
<div id="d" style="width: 10px; height: 20px; color: green"></div>
</body></html>`

func cssomIterRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssomIterPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func mustCSSOMJSON(t *testing.T, r *js.Runtime, expr string) string {
	t.Helper()
	got, err := r.Evaluate(expr)
	if err != nil {
		t.Fatalf("Evaluate(%s): %v", expr, err)
	}
	var v interface{}
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("invalid JSON from engine %q: %v", got, err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

const cssomIterJS = `
	var d = document.getElementById('d');
	var rule = document.styleSheets[0].cssRules[0];
`

// qc:BR-316
func TestCssStyleDeclarationIsIterable(t *testing.T) {
	r := cssomIterRuntime(t)
	got := mustCSSOMJSON(t, r, cssomIterJS+`
		JSON.stringify({
			iterType: typeof d.style[Symbol.iterator],
			inlineSpread: [...d.style],
			ruleSpread: [...rule.style],
			forOf: (function(){ var a=[]; for (var x of d.style) a.push(x); return a; })(),
			arrayFrom: Array.from(d.style)
		})
	`)
	want := `{"arrayFrom":["width","height","color"],"forOf":["width","height","color"],"inlineSpread":["width","height","color"],"iterType":"function","ruleSpread":["color","background"]}`
	if got != want {
		t.Errorf("CSSStyleDeclaration iteration\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-316
func TestCssStyleDeclarationParentRule(t *testing.T) {
	r := cssomIterRuntime(t)
	got := mustCSSOMJSON(t, r, cssomIterJS+`
		JSON.stringify({
			inlineIsNull: d.style.parentRule === null,
			inlineIsUndef: d.style.parentRule === undefined,
			inlineIn: ('parentRule' in d.style),
			ruleIdentity: rule.style.parentRule === rule,
			ruleSelector: rule.style.parentRule.selectorText,
			assignKeepsNull: (function(){ d.style.parentRule = 5; return d.style.parentRule === null; })()
		})
	`)
	want := `{"assignKeepsNull":true,"inlineIn":true,"inlineIsNull":true,"inlineIsUndef":false,"ruleIdentity":true,"ruleSelector":".r"}`
	if got != want {
		t.Errorf("CSSStyleDeclaration parentRule\ngot:  %s\nwant: %s", got, want)
	}
}

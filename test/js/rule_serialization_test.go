package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleSerializePage = `<!doctype html><html><head>
<style id="S">
p { color: rgb(10,0,0); }
q { background: rgb(1,2,3) !important; color: rgb(4,5,6); }
r { }
s { margin: 4px !important; padding: 8px; color: rgb(7,8,9) !important }
u { font-family: "A,B",serif; color: rgb(1,2,3) }
</style>
</head><body></body></html>`

func ruleSerializeRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleSerializePage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleSerializeJS = `
	var rules = document.styleSheets[0].cssRules;
	function cssTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].cssText); return a.join(' || ') }
	function styleTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].style.cssText); return a.join(' || ') }
`

// qc:BR-302
func TestRuleCSSTextReSerialisesDeclarationLists(t *testing.T) {
	r := ruleSerializeRuntime(t)
	got := mustCSSOM(t, r, ruleSerializeJS+`cssTexts()`)
	want := "p { color: rgb(10, 0, 0); } || q { color: rgb(4, 5, 6); background: rgb(1, 2, 3) !important; } || r { } || s { padding: 8px; margin: 4px !important; color: rgb(7, 8, 9) !important; } || u { font-family: \"A,B\", serif; color: rgb(1, 2, 3); }"
	if got != want {
		t.Errorf("rule.cssText re-serialisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-302
func TestRuleStyleCSSTextSharesTheReSerialisation(t *testing.T) {
	r := ruleSerializeRuntime(t)
	got := mustCSSOM(t, r, ruleSerializeJS+`styleTexts()`)
	want := "color: rgb(10, 0, 0); || color: rgb(4, 5, 6); background: rgb(1, 2, 3) !important; ||  || padding: 8px; margin: 4px !important; color: rgb(7, 8, 9) !important; || font-family: \"A,B\", serif; color: rgb(1, 2, 3);"
	if got != want {
		t.Errorf("rule.style.cssText re-serialisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-302
func TestRuleStyleValueReadsReSerialise(t *testing.T) {
	r := ruleSerializeRuntime(t)
	got := mustCSSOM(t, r, ruleSerializeJS+`
		[
			rules[1].style.getPropertyValue('background'),
			rules[1].style.background,
			rules[4].style.fontFamily,
			rules[0].style.getPropertyValue('color')
		].join(' || ')
	`)
	want := "rgb(1, 2, 3) || rgb(1, 2, 3) || \"A,B\", serif || rgb(10, 0, 0)"
	if got != want {
		t.Errorf("rule.style value read re-serialisation\ngot:  %s\nwant: %s", got, want)
	}
}

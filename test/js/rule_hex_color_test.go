package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleHexColorPage = `<!doctype html><html><head>
<style id="H">
.c1 { color: #ABC; }
.c2 { background-color: #102030; }
.c3 { border-color: #ABCD; }
.c4 { color: #102030F0; }
.c5 { color: #102030FF; }
.c6 { outline-color: #1234; }
.c7 { border: 1px solid #ABC; }
.g1 { content: "#102030"; }
.g2 { --c: #ABC; }
.g3 { color: red; }
</style>
</head><body></body></html>`

func ruleHexColorRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleHexColorPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleHexColorJS = `
	var rules = document.styleSheets[0].cssRules;
	function cssTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].cssText); return a.join(' || ') }
`

// qc:BR-312
func TestRuleHexColorCanonicalisesCssText(t *testing.T) {
	r := ruleHexColorRuntime(t)
	got := mustCSSOM(t, r, ruleHexColorJS+`cssTexts()`)
	want := ".c1 { color: rgb(170, 187, 204); } || .c2 { background-color: rgb(16, 32, 48); } || .c3 { border-color: rgba(170, 187, 204, 0.867); } || .c4 { color: rgba(16, 32, 48, 0.94); } || .c5 { color: rgb(16, 32, 48); } || .c6 { outline-color: rgba(17, 34, 51, 0.267); } || .c7 { border: 1px solid rgb(170, 187, 204); } || .g1 { content: \"#102030\"; } || .g2 { --c: #ABC; } || .g3 { color: red; }"
	if got != want {
		t.Errorf("rule.cssText hex canonicalisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-312
func TestRuleHexColorReadPathsAndAlphaRounding(t *testing.T) {
	r := ruleHexColorRuntime(t)
	got := mustCSSOM(t, r, ruleHexColorJS+`
		[
			rules[0].style.color,
			rules[2].style.borderColor,
			rules[3].style.getPropertyValue('color'),
			rules[5].style.outlineColor
		].join(' || ')
	`)
	want := "rgb(170, 187, 204) || rgba(170, 187, 204, 0.867) || rgba(16, 32, 48, 0.94) || rgba(17, 34, 51, 0.267)"
	if got != want {
		t.Errorf("rule.style hex reads\ngot:  %s\nwant: %s", got, want)
	}
}

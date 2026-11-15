package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleValueNormPage = `<!doctype html><html><head>
<style id="N">
p.a { width: .5px; height: 4.0px; margin: 50.0%; padding: +3px; line-height: 1.50; }
p.c { transform: translate(10.0px, 20.5px) rotate(45.0deg); grid-template-columns: repeat(2, 1fr); }
p.z { opacity: .4; font-size: 1.20rem; color: rgb(10,0,0); }
p.n { --custom: 4.0px; content: "v1.0"; }
p.e { top: -0; left: 0.00; right: 10.0px; bottom: 20.5px; width: calc(100% - 4.0px); }
</style>
</head><body></body></html>`

func ruleValueNormRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleValueNormPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleValueNormJS = `
	var rules = document.styleSheets[0].cssRules;
	function cssTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].cssText); return a.join(' || ') }
`

// qc:BR-311
func TestRuleValueNormalisesDecimalNumbers(t *testing.T) {
	r := ruleValueNormRuntime(t)
	got := mustCSSOM(t, r, ruleValueNormJS+`cssTexts()`)
	want := "p.a { width: 0.5px; height: 4px; margin: 50%; padding: 3px; line-height: 1.5; } || p.c { transform: translate(10px, 20.5px) rotate(45deg); grid-template-columns: repeat(2, 1fr); } || p.z { opacity: 0.4; font-size: 1.2rem; color: rgb(10, 0, 0); } || p.n { --custom: 4.0px; content: \"v1.0\"; } || p.e { top: 0; left: 0; right: 10px; bottom: 20.5px; width: calc(100% - 4px); }"
	if got != want {
		t.Errorf("rule.cssText decimal normalisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-311
func TestRuleStyleValueReadsNormaliseNumbers(t *testing.T) {
	r := ruleValueNormRuntime(t)
	got := mustCSSOM(t, r, ruleValueNormJS+`
		[
			rules[0].style.getPropertyValue('width'),
			rules[0].style.lineHeight,
			rules[2].style.opacity,
			rules[4].style.top,
			rules[4].style.getPropertyValue('width'),
			rules[3].style.getPropertyValue('--custom'),
			rules[3].style.content
		].join(' || ')
	`)
	want := "0.5px || 1.5 || 0.4 || 0 || calc(100% - 4px) || 4.0px || \"v1.0\""
	if got != want {
		t.Errorf("rule.style value read normalisation\ngot:  %s\nwant: %s", got, want)
	}
}

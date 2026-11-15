package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleURLPage = `<!doctype html><html><head>
<style id="U">
.u1 { background: url(#frag); }
.u2 { background: url(a.png); }
.u3 { background: url('a.png'); }
.u4 { background: url("a.png"); }
.u5 { background: url( a.png ); }
.u6 { background: url(); }
.u7 { background: URL(a.png); }
.u8 { background: url(a.png), url(b.png); }
.u9 { background: url(a\(b); }
.u10 { background: url("a\"b"); }
.u11 { background: url(#frag) no-repeat; }
.g1 { content: "url(a.png)"; }
.g2 { --u: url(a.png); }
</style>
</head><body></body></html>`

func ruleURLRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleURLPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleURLJS = `
	var rules = document.styleSheets[0].cssRules;
	function cssTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].cssText); return a.join(' || ') }
`

// qc:BR-315
func TestRuleUrlCanonicalisesCssText(t *testing.T) {
	r := ruleURLRuntime(t)
	got := mustCSSOM(t, r, ruleURLJS+`cssTexts()`)
	want := ".u1 { background: url(\"#frag\"); } || .u2 { background: url(\"a.png\"); } || .u3 { background: url(\"a.png\"); } || .u4 { background: url(\"a.png\"); } || .u5 { background: url(\"a.png\"); } || .u6 { background: url(\"\"); } || .u7 { background: url(\"a.png\"); } || .u8 { background: url(\"a.png\"), url(\"b.png\"); } || .u9 { background: url(\"a(b\"); } || .u10 { background: url(\"a\\\"b\"); } || .u11 { background: url(\"#frag\") no-repeat; } || .g1 { content: \"url(a.png)\"; } || .g2 { --u: url(a.png); }"
	if got != want {
		t.Errorf("rule.cssText url canonicalisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-315
func TestRuleUrlReadPaths(t *testing.T) {
	r := ruleURLRuntime(t)
	got := mustCSSOM(t, r, ruleURLJS+`
		[
			rules[0].style.getPropertyValue('background'),
			rules[1].style.background,
			rules[5].style.background,
			rules[8].style.background,
			rules[9].style.background
		].join(' || ')
	`)
	want := "url(\"#frag\") || url(\"a.png\") || url(\"\") || url(\"a(b\") || url(\"a\\\"b\")"
	if got != want {
		t.Errorf("rule.style url reads\ngot:  %s\nwant: %s", got, want)
	}
}

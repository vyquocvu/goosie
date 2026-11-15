package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const ruleHslColorPage = `<!doctype html><html><head>
<style id="HL">
.h1 { color: hsl(0,100%,50%); }
.h2 { color: hsl(120,50%,50%); }
.h3 { color: hsl(192,100%,50%); }
.h4 { color: hsl(-60,50%,50%); }
.h5 { color: hsl(420,50%,50%); }
.h6 { color: hsla(200,40%,30%,.4); }
.h7 { color: hsl(120 50% 50%); }
.h8 { color: hsl(120deg 50% 50%); }
.h9 { color: hsl(0.3turn 50% 50%); }
.h10 { color: hsl(200grad 40% 60%); }
.h11 { color: hsl(3.14rad 40% 60%); }
.h12 { color: hsl(120 50% 50% / 0.5); }
.h13 { color: hsl(120 50% 50% / 50%); }
.h14 { color: hsl(120 50% 50% / none); }
.h15 { color: hsl(none 50% 50%); }
.h16 { color: hsl(120 none none); }
.h17 { color: hsl(120 200% 50%); }
.h18 { color: hsl(120 -50% 50%); }
.h19 { color: hsl(120 50% 200%); }
.h20 { color: hwb(90 0% 0%); }
.h21 { color: hwb(190 0% 0%); }
.h22 { color: hwb(270 20% 10%); }
.h23 { color: hwb(90 40% 40%); }
.h24 { color: hwb(0 0% 0% / 0.5); }
.h25 { color: hwb(90deg 10% 20%); }
.h26 { color: HSL(120,50%,50%); }
.h27 { border: 1px solid hsl(120 50% 50%); }
.h28 { color: hsl(120,33.3333%,66.6666%); }
.h29 { color: hsl(192 100% 50% / 0.501960784); }
.h30 { color: hsl(0 0% 0% / 0.501); }
.h31 { color: hsl(10 20% 30% / 0.94); }
.h32 { color: hsl(120,50%,50%,40%); }
.h33 { color: hsl(120,50%,50%,1); }
.h34 { color: hsl(120,50%,50%,0); }
.g1 { content: "hsl(1 2 3)"; }
.g2 { --c: hsl(120 50% 50%); }
</style>
</head><body></body></html>`

func ruleHslColorRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(ruleHslColorPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const ruleHslColorJS = `
	var rules = document.styleSheets[0].cssRules;
	function cssTexts() { var a = []; for (var i = 0; i < rules.length; i++) a.push(rules[i].cssText); return a.join(' || ') }
`

// qc:BR-317
func TestRuleHslColorCanonicalisesCssText(t *testing.T) {
	r := ruleHslColorRuntime(t)
	got := mustCSSOM(t, r, ruleHslColorJS+`cssTexts()`)
	want := ".h1 { color: rgb(255, 0, 0); } || .h2 { color: rgb(64, 191, 64); } || .h3 { color: rgb(0, 204, 255); } || .h4 { color: rgb(191, 64, 191); } || .h5 { color: rgb(191, 191, 64); } || .h6 { color: rgba(46, 87, 107, 0.4); } || .h7 { color: rgb(64, 191, 64); } || .h8 { color: rgb(64, 191, 64); } || .h9 { color: rgb(89, 191, 64); } || .h10 { color: rgb(112, 194, 194); } || .h11 { color: rgb(112, 194, 194); } || .h12 { color: rgba(64, 191, 64, 0.5); } || .h13 { color: rgba(64, 191, 64, 0.5); } || .h14 { color: rgba(64, 191, 64, 0); } || .h15 { color: rgb(191, 64, 64); } || .h16 { color: rgb(0, 0, 0); } || .h17 { color: rgb(0, 255, 0); } || .h18 { color: rgb(128, 128, 128); } || .h19 { color: rgb(255, 255, 255); } || .h20 { color: rgb(128, 255, 0); } || .h21 { color: rgb(0, 212, 255); } || .h22 { color: rgb(140, 51, 230); } || .h23 { color: rgb(128, 153, 102); } || .h24 { color: rgba(255, 0, 0, 0.5); } || .h25 { color: rgb(115, 204, 26); } || .h26 { color: rgb(64, 191, 64); } || .h27 { border: 1px solid rgb(64, 191, 64); } || .h28 { color: rgb(142, 198, 142); } || .h29 { color: rgba(0, 204, 255, 0.5); } || .h30 { color: rgba(0, 0, 0, 0.5); } || .h31 { color: rgba(92, 66, 61, 0.94); } || .h32 { color: rgba(64, 191, 64, 0.4); } || .h33 { color: rgb(64, 191, 64); } || .h34 { color: rgba(64, 191, 64, 0); } || .g1 { content: \"hsl(1 2 3)\"; } || .g2 { --c: hsl(120 50% 50%); }"
	if got != want {
		t.Errorf("rule.cssText hsl/hwb canonicalisation\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-317
func TestRuleHslColorReadPaths(t *testing.T) {
	r := ruleHslColorRuntime(t)
	got := mustCSSOM(t, r, ruleHslColorJS+`
		[
			rules[0].style.color,
			rules[5].style.getPropertyValue('color'),
			rules[13].style.color,
			rules[19].style.color,
			rules[22].style.color,
			rules[31].style.color,
			rules[33].style.color
		].join(' || ')
	`)
	want := "rgb(255, 0, 0) || rgba(46, 87, 107, 0.4) || rgba(64, 191, 64, 0) || rgb(128, 255, 0) || rgb(128, 153, 102) || rgba(64, 191, 64, 0.4) || rgba(64, 191, 64, 0)"
	if got != want {
		t.Errorf("rule.style hsl/hwb reads\ngot:  %s\nwant: %s", got, want)
	}
}

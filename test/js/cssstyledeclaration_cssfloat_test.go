package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssFloatPage = `<!doctype html><html><head>
<style id="S">.r { color: rgb(1,2,3); }</style>
</head><body><div id="D" style="width:10px">hi</div></body></html>`

func cssFloatRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssFloatPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

const cssFloatJS = `
	var st = document.getElementById('D').style;
	var rule = document.styleSheets[0].cssRules[0];
`

// qc:BR-314
func TestInlineStyleCssFloatAliasesFloat(t *testing.T) {
	r := cssFloatRuntime(t)
	got := mustCSSOM(t, r, cssFloatJS+`
		st.cssFloat = 'left';
		var set = [
			st.getPropertyValue('float'),
			st.cssFloat,
			st.float,
			'[' + st.getPropertyValue('cssFloat') + ']',
			String(st.length),
			st.item(0),
			st.cssText
		].join(' | ');
		st.cssFloat = '';
		var cleared = [String(st.length), st.cssText].join(' | ');
		st.setProperty('float', 'right');
		var viaRealName = st.cssFloat;
		[set, cleared, viaRealName].join(' || ')
	`)
	want := "left | left | left | [] | 2 | width | width: 10px; float: left; || 1 | width: 10px; || right"
	if got != want {
		t.Errorf("element.style cssFloat aliasing\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-314
func TestRuleStyleCssFloatAliasesFloat(t *testing.T) {
	r := cssFloatRuntime(t)
	got := mustCSSOM(t, r, cssFloatJS+`
		rule.style.cssFloat = 'left';
		var set = [
			rule.style.getPropertyValue('float'),
			rule.style.cssFloat,
			rule.cssText
		].join(' | ');
		var removed = rule.style.removeProperty('float');
		var after = rule.style.getPropertyValue('float');
		[set, removed, '[' + after + ']'].join(' || ')
	`)
	want := "left | left | .r { color: rgb(1, 2, 3); float: left; } || left || []"
	if got != want {
		t.Errorf("rule.style cssFloat aliasing\ngot:  %s\nwant: %s", got, want)
	}
}

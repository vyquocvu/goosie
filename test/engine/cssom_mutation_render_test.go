package engine_test

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

const cssomMutationPage = `<!doctype html><html><head>
<style id="sheet">#target { color: rgb(1, 2, 3); width: 40px; }</style>
</head><body><div id="target">TARGET</div></body></html>`

func cssomMutationSession(t *testing.T) *engine.Session {
	t.Helper()
	return newCSSSession(t, newCSSRuntime(t), cssomMutationPage, nil)
}

func cssomTargetNode(t *testing.T, sess *engine.Session) *dom.Node {
	t.Helper()
	nodes := sess.Doc.ElementsByTagName("div")
	if len(nodes) != 1 {
		t.Fatalf("found %d target divs, want 1", len(nodes))
	}
	return nodes[0]
}

func cssomTargetPaintColor(t *testing.T, sess *engine.Session) css.Color {
	t.Helper()
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	for _, command := range list.All() {
		if command.Kind != paint.CmdText {
			continue
		}
		var text strings.Builder
		for _, glyph := range command.Text.Glyphs {
			text.WriteRune(glyph.Rune)
		}
		if text.String() == "TARGET" {
			return css.Color{R: command.Text.Color.R(), G: command.Text.Color.G(), B: command.Text.Color.B(), A: command.Text.Color.A()}
		}
	}
	t.Fatalf("target text was not painted")
	return css.Color{}
}

func assertCSSOMMutationRender(t *testing.T, sess *engine.Session, color string, width int) {
	t.Helper()
	node := cssomTargetNode(t, sess)
	style := sess.Styles[node.ID]
	if style == nil {
		t.Fatal("target has no computed style")
	}
	want, ok := css.ParseColor(color)
	if !ok {
		t.Fatalf("invalid expected color %q", color)
	}
	wantColor := want
	if style.Color != wantColor {
		t.Fatalf("resolved color = %v, want %v", style.Color, wantColor)
	}
	if got := cssomTargetPaintColor(t, sess); got != wantColor {
		t.Fatalf("painted text color = %v, want %v", got, wantColor)
	}
	id, ok := sess.Arena.ForNode(node.ID)
	if !ok {
		t.Fatal("target has no layout box")
	}
	box := sess.Arena.Get(id)
	if box == nil || int(box.W+0.5) != width {
		t.Fatalf("layout width = %v, want %d", box, width)
	}
}

func TestCSSOMMutationsReachComputedStyleLayoutAndPaintInOneTask(t *testing.T) {
	tests := []struct {
		name   string
		script string
		color  string
		width  int
	}{
		{
			name: "insert and delete rule",
			script: `(function(){
				var sheet = document.styleSheets[0];
				sheet.insertRule('#target { color: rgb(10, 20, 30); width: 60px; }', sheet.cssRules.length);
				var inserted = getComputedStyle(document.getElementById('target')).color;
				sheet.deleteRule(sheet.cssRules.length - 1);
				var cs = getComputedStyle(document.getElementById('target'));
				return inserted + '|' + cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(1, 2, 3)",
			width: 40,
		},
		{
			name: "setProperty",
			script: `(function(){
				var style = document.styleSheets[0].cssRules[0].style;
				style.setProperty('color', 'rgb(11, 21, 31)');
				style.setProperty('width', '61px');
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(11, 21, 31)",
			width: 61,
		},
		{
			name: "setProperty replaces duplicate declarations",
			script: `(function(){
				document.getElementById('sheet').textContent = '#target { color: rgb(2, 3, 4); color: rgb(3, 4, 5); width: 40px; }';
				var style = document.styleSheets[0].cssRules[0].style;
				style.setProperty('color', 'rgb(26, 36, 46)');
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width + '|' + style.getPropertyValue('color');
			})()`,
			color: "rgb(26, 36, 46)",
			width: 40,
		},
		{
			name: "removeProperty",
			script: `(function(){
				var style = document.styleSheets[0].cssRules[0].style;
				style.setProperty('color', 'rgb(12, 22, 32)');
				style.setProperty('width', '62px');
				style.removeProperty('color');
				style.removeProperty('width');
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(0, 0, 0)",
			width: 784,
		},
		{
			name: "named setters",
			script: `(function(){
				var style = document.styleSheets[0].cssRules[0].style;
				style.color = 'rgb(13, 23, 33)';
				style.width = '63px';
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(13, 23, 33)",
			width: 63,
		},
		{
			name: "cssText",
			script: `(function(){
				document.styleSheets[0].cssRules[0].style.cssText = 'color: rgb(14, 24, 34); width: 64px;';
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(14, 24, 34)",
			width: 64,
		},
		{
			name: "style textContent",
			script: `(function(){
				document.getElementById('sheet').textContent = '#target { color: rgb(15, 25, 35); width: 65px; }';
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(15, 25, 35)",
			width: 65,
		},
		{
			name: "style textContent restores the source text",
			script: `(function(){
				var style = document.getElementById('sheet');
				var original = style.textContent;
				document.styleSheets[0].cssRules[0].style.color = 'rgb(24, 34, 44)';
				style.textContent = original;
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(1, 2, 3)",
			width: 40,
		},
		{
			name: "grouped rule style write",
			script: `(function(){
				document.getElementById('sheet').textContent = '@media screen { #target { color: rgb(1, 2, 3); width: 40px; } }';
				var style = document.styleSheets[0].cssRules[0].style;
				style.setProperty('color', 'rgb(25, 35, 45)');
				style.setProperty('width', '72px');
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(25, 35, 45)",
			width: 72,
		},
		{
			name: "constructed adopted sheet",
			script: `(function(){
				var sheet = new CSSStyleSheet();
				sheet.replaceSync('#target { color: rgb(16, 26, 36); width: 66px; }');
				document.adoptedStyleSheets = [sheet];
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(16, 26, 36)",
			width: 66,
		},
		{
			name: "constructed insertRule",
			script: `(function(){
				var sheet = new CSSStyleSheet();
				sheet.insertRule('#target { color: rgb(19, 29, 39); width: 69px; }');
				document.adoptedStyleSheets = [sheet];
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(19, 29, 39)",
			width: 69,
		},
		{
			name: "CSS in JS insertion journey",
			script: `(function(){
				var style = document.createElement('style');
				style.textContent = '#target { color: rgb(17, 27, 37); width: 67px; }';
				document.head.appendChild(style);
				var sheet = style.sheet;
				sheet.insertRule('#target { color: rgb(18, 28, 38); width: 68px; }', sheet.cssRules.length);
				var cs = getComputedStyle(document.getElementById('target'));
				return cs.color + '|' + cs.width + '|' + document.getElementById('target').getBoundingClientRect().width;
			})()`,
			color: "rgb(18, 28, 38)",
			width: 68,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sess := cssomMutationSession(t)
			got, err := sess.EvaluateJS(test.script)
			if err != nil {
				t.Fatalf("EvaluateJS: %v", err)
			}
			wantPrefix := test.color + "|" + fmt.Sprintf("%dpx|%d", test.width, test.width)
			if test.name == "insert and delete rule" {
				wantPrefix = "rgb(10, 20, 30)|" + wantPrefix
			}
			if !strings.HasPrefix(got, wantPrefix) {
				t.Fatalf("same-task computed style = %q, want prefix %q", got, wantPrefix)
			}
			assertCSSOMMutationRender(t, sess, test.color, test.width)
		})
	}
}

func TestCSSOMMutationAdmissionCaps(t *testing.T) {
	sess := cssomMutationSession(t)
	tests := []struct {
		name string
		js   string
		want string
	}{
		{
			name: "bytes",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('#target{--x:' + 'a'.repeat(%d) + '}')", engine.MaxCSSBytes+1),
			want: "CSS byte limit",
		},
		{
			name: "rules",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('p{}'.repeat(%d))", engine.MaxCSSRules+1),
			want: "CSS rule limit",
		},
		{
			name: "tokens",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('#target{--x:' + 'a '.repeat(%d) + '}')", engine.MaxCSSTokens+1),
			want: "CSS token limit",
		},
		{
			name: "declarations",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('#target{' + 'x:y;'.repeat(%d) + '}')", engine.MaxCSSDeclarations+1),
			want: "CSS declaration limit",
		},
		{
			name: "selectors",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('p,'.repeat(%d) + 'p{}')", engine.MaxCSSSelectors+1),
			want: "CSS selector count limit",
		},
		{
			name: "selector parts",
			js:   fmt.Sprintf("document.styleSheets[0].insertRule('p > '.repeat(%d) + 'p{}')", engine.MaxSelectorParts+1),
			want: "CSS selector part limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := sess.EvaluateJS(`(function(){try{` + test.js + `;return 'accepted'}catch(e){return e.message}})()`)
			if err != nil {
				t.Fatalf("EvaluateJS: %v", err)
			}
			if !strings.Contains(got, test.want) {
				t.Fatalf("admission error = %q, want %q", got, test.want)
			}
			assertCSSOMMutationRender(t, sess, "rgb(1, 2, 3)", 40)
		})
	}
}

func TestCSSOMTextContentAdmissionKeepsTheLastValidCascade(t *testing.T) {
	sess := cssomMutationSession(t)
	got, err := sess.EvaluateJS(fmt.Sprintf(`(function(){
		document.getElementById('sheet').textContent = '#target{--x:' + 'a'.repeat(%d) + '}';
		return getComputedStyle(document.getElementById('target')).color + '|' + getComputedStyle(document.getElementById('target')).width;
	})()`, engine.MaxCSSBytes+1))
	if err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if got != "rgb(1, 2, 3)|40px" {
		t.Fatalf("computed style after rejected textContent = %q", got)
	}
	assertCSSOMMutationRender(t, sess, "rgb(1, 2, 3)", 40)
	found := false
	for _, scriptError := range sess.ScriptErrors() {
		if strings.Contains(scriptError.Message, "CSS byte limit") {
			found = true
		}
	}
	if !found {
		t.Fatalf("CSS byte limit was not recorded: %v", sess.ScriptErrors())
	}
}

func TestCSSOMGeneratedImportUsesCSSFetchAndCSPGuards(t *testing.T) {
	for _, test := range []struct {
		name      string
		csp       string
		importURL string
		wantFetch int
		wantColor string
	}{
		{name: "allowed", csp: "style-src 'self' 'unsafe-inline'", importURL: "/theme.css", wantFetch: 1, wantColor: "rgb(21, 31, 41)"},
		{name: "blocked", csp: "style-src 'self' 'unsafe-inline'", importURL: "https://other.test/theme.css", wantFetch: 0, wantColor: "rgb(0, 0, 0)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			srv := newSheetServer()
			srv.text["https://example.test/theme.css"] = "#target { color: rgb(21, 31, 41); width: 69px; }"
			rt, err := js.New(js.Options{URL: "https://example.test/", CSP: js.ParseCSP(test.csp)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = rt.Close() })
			sess := newCSSSession(t, rt, cssomMutationPage, srv.link)
			script := fmt.Sprintf(`(function(){
				document.getElementById('sheet').textContent = '@import url("%s");';
				return getComputedStyle(document.getElementById('target')).color;
			})()`, test.importURL)
			got, err := sess.EvaluateJS(script)
			if err != nil {
				t.Fatalf("EvaluateJS: %v", err)
			}
			if got != test.wantColor {
				t.Fatalf("computed color = %q, want %q", got, test.wantColor)
			}
			if fetched := srv.count("https://example.test/theme.css"); fetched != test.wantFetch {
				t.Fatalf("same-origin import fetches = %d, want %d", fetched, test.wantFetch)
			}
			if fetched := srv.count("https://other.test/theme.css"); fetched != 0 {
				t.Fatalf("cross-origin import fetches = %d, want 0", fetched)
			}
			if test.wantFetch == 1 {
				assertCSSOMMutationRender(t, sess, test.wantColor, 69)
			} else {
				assertCSSOMMutationRender(t, sess, test.wantColor, 784)
			}
		})
	}
}

func TestCSSOMMutationRepaintsWithoutAComputedStyleRead(t *testing.T) {
	sess := cssomMutationSession(t)
	if _, err := sess.EvaluateJS(`document.styleSheets[0].insertRule('#target { color: rgb(23, 33, 43); width: 71px; }', document.styleSheets[0].cssRules.length)`); err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	assertCSSOMMutationRender(t, sess, "rgb(23, 33, 43)", 71)
}

func TestCSSOMMutationsOnLinkedSheetsReachTheRender(t *testing.T) {
	srv := newSheetServer()
	srv.text["https://example.test/a.css"] = "#target { color: rgb(1, 2, 3); width: 40px; }"
	rt := newCSSRuntime(t)
	html := `<!doctype html><html><head><link rel="stylesheet" href="/a.css"></head><body><div id="target">TARGET</div></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)
	got, err := sess.EvaluateJS(`(function(){
		var rule = document.styleSheets[0].cssRules[0];
		rule.style.setProperty('color', 'rgb(22, 32, 42)');
		rule.style.width = '70px';
		var cs = getComputedStyle(document.getElementById('target'));
		return cs.color + '|' + cs.width;
	})()`)
	if err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if got != "rgb(22, 32, 42)|70px" {
		t.Fatalf("linked-sheet computed style = %q", got)
	}
	assertCSSOMMutationRender(t, sess, "rgb(22, 32, 42)", 70)
	if fetched := srv.count("https://example.test/a.css"); fetched != 1 {
		t.Fatalf("linked sheet fetches = %d, want 1", fetched)
	}
}

func TestCSSOMGeneratedURLFetchPaintsInTheSameTask(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var fetches atomic.Int32
	rt, err := js.New(js.Options{URL: "https://example.test/", CSP: js.ParseCSP("style-src 'unsafe-inline'; img-src 'self'")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	html := `<!doctype html><html><head><style>#target { width: 40px; height: 40px; }</style></head><body><div id="target">TARGET</div></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithImages("https://example.test/", func(base, rawURL string) ([]byte, error) {
			if rawURL != "https://example.test/pixel.png" {
				return nil, fmt.Errorf("image URL = %q", rawURL)
			}
			fetches.Add(1)
			return png, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	if _, err := sess.EvaluateJS(`(function(){
		document.styleSheets[0].cssRules[0].style.backgroundImage = 'url("/pixel.png")';
		return getComputedStyle(document.getElementById('target')).width;
	})()`); err != nil {
		t.Fatalf("EvaluateJS: %v", err)
	}
	if fetches.Load() != 1 {
		t.Fatalf("generated URL fetches = %d, want 1", fetches.Load())
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	for _, command := range list.All() {
		if command.Kind == paint.CmdImage && command.Image.Src != nil {
			return
		}
	}
	t.Fatalf("generated URL image was not painted")
}

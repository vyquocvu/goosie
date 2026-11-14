package js_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const selectorFixture = `<!DOCTYPE html><html><head></head><body><div id="o" data-x="y"><p class="a">1</p><span class="s"><p class="b">2</p></span></div><p class="c">3</p></body></html>`

const selectorPrelude = `
var o = document.getElementById("o");
function id(n) {
	if (n === null) return "null";
	var s = n.tagName.toLowerCase();
	if (n.id) s += "#" + n.id;
	if (n.className) s += "." + n.className;
	return s;
}
function list(l) {
	var a = [];
	for (var i = 0; i < l.length; i++) a.push(id(l[i]));
	return a.join(",");
}
function thrown(fn) {
	try { fn(); return "no-throw"; } catch (e) { return e.name; }
}
function message(fn) {
	try { fn(); return "no-throw"; } catch (e) { return e.message; }
}
`

func runSelectorScript(t *testing.T, body string) string {
	t.Helper()
	doc := domtest.Parse(selectorFixture)
	r := newRuntime(t, js.Options{URL: "https://example.test/", DOM: doc})
	if err := r.Run(selectorPrelude+body, "inline"); err != nil {
		t.Fatalf("run: %v", err)
	}
	v, ok := r.Global("out")
	if !ok {
		t.Fatalf("script did not assign out")
	}
	return v.Str
}

// qc:BR-213
func TestSelectorMatchingSection1Forms(t *testing.T) {
	cases := []struct {
		sel       string
		docQS     string
		docQSA    string
		scopedQS  string
		scopedQSA string
		matchA    bool
		matchB    bool
		closestB  string
		closestC  string
	}{
		{"*", "html", "html,head,body,div#o,p.a,span.s,p.b,p.c", "p.a", "p.a,span.s,p.b", true, true, "p.b", "p.c"},
		{"div", "div#o", "div#o", "null", "", false, false, "div#o", "null"},
		{"p.a", "p.a", "p.a", "p.a", "p.a", true, false, "null", "null"},
		{"div#o", "div#o", "div#o", "null", "", false, false, "div#o", "null"},
		{"[data-x]", "div#o", "div#o", "null", "", false, false, "div#o", "null"},
		{`[data-x="y"]`, "div#o", "div#o", "null", "", false, false, "div#o", "null"},
		{"div > p", "p.a", "p.a", "p.a", "p.a", true, false, "null", "null"},
		{".a, .c", "p.a", "p.a,p.c", "p.a", "p.a", true, false, "null", "p.c"},
		{"p:first-child", "p.a", "p.a,p.b", "p.a", "p.a,p.b", true, true, "p.b", "null"},
		{"div span", "span.s", "span.s", "span.s", "span.s", false, false, "span.s", "null"},
		{".a", "p.a", "p.a", "p.a", "p.a", true, false, "null", "null"},
		{"p", "p.a", "p.a,p.b,p.c", "p.a", "p.a,p.b", true, true, "p.b", "p.c"},
		{"span > p", "p.b", "p.b", "p.b", "p.b", false, true, "p.b", "null"},
		{"#o", "div#o", "div#o", "null", "", false, false, "div#o", "null"},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			body := fmt.Sprintf(`
var a = document.querySelector(".a"), b = document.querySelector(".b"), c = document.querySelector(".c");
out = [
	id(document.querySelector(%[1]s)),
	list(document.querySelectorAll(%[1]s)),
	id(o.querySelector(%[1]s)),
	list(o.querySelectorAll(%[1]s)),
	String(a.matches(%[1]s)),
	String(b.matches(%[1]s)),
	id(b.closest(%[1]s)),
	id(c.closest(%[1]s))
].join("|");`, strconv.Quote(c.sel))
			want := strings.Join([]string{
				c.docQS, c.docQSA, c.scopedQS, c.scopedQSA,
				strconv.FormatBool(c.matchA), strconv.FormatBool(c.matchB),
				c.closestB, c.closestC,
			}, "|")
			if got := runSelectorScript(t, body); got != want {
				t.Fatalf("querySelector(All)/matches/closest %q = %q, want %q", c.sel, got, want)
			}
		})
	}
}

// qc:BR-214
func TestSelectorMatchingCombinatorsAndLists(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"div > p", "p.a"},
		{"div span", "span.s"},
		{"#o span p", "p.b"},
		{"#o > span", "span.s"},
		{"div + p", "p.c"},
		{"p + p", ""},
		{"div ~ p", "p.c"},
		{"p ~ p", ""},
		{"div > *", "p.a,span.s"},
		{"body > :first-child", "div#o"},
		{".a, .c", "p.a,p.c"},
		{".c, .a", "p.a,p.c"},
		{"p,p,p", "p.a,p.b,p.c"},
		{":is(p, .s)", "p.a,span.s,p.b,p.c"},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			body := fmt.Sprintf(`out = list(document.querySelectorAll(%s));`, strconv.Quote(c.sel))
			if got := runSelectorScript(t, body); got != c.want {
				t.Fatalf("querySelectorAll(%q) = %q, want %q", c.sel, got, c.want)
			}
		})
	}
}

// qc:BR-215
func TestSelectorMatchingAttributes(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"[data-x]", "div#o"},
		{`[data-x="y"]`, "div#o"},
		{"[data-x='y']", "div#o"},
		{"[data-x=y]", "div#o"},
		{`[data-x="Y"]`, ""},
		{`[data-x="Y" i]`, "div#o"},
		{`[data-x^="y"]`, "div#o"},
		{`[data-x$="y"]`, "div#o"},
		{`[data-x*="y"]`, "div#o"},
		{`[data-x|="y"]`, "div#o"},
		{`[class~="a"]`, "p.a"},
		{`[class="a b"]`, ""},
		{`[DATA-X]`, "div#o"},
		{`[data-x]:not([data-x=z])`, "div#o"},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			body := fmt.Sprintf(`out = list(document.querySelectorAll(%s));`, strconv.Quote(c.sel))
			if got := runSelectorScript(t, body); got != c.want {
				t.Fatalf("querySelectorAll(%q) = %q, want %q", c.sel, got, c.want)
			}
		})
	}
}

// qc:BR-216
func TestSelectorMatchingPseudoClasses(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"p:first-child", "p.a,p.b"},
		{"p:last-child", "p.b,p.c"},
		{"p:only-child", "p.b"},
		{"p:empty", ""},
		{"p:first-of-type", "p.a,p.b,p.c"},
		{"p:nth-last-of-type(1)", "p.a,p.b,p.c"},
		{"p:nth-child(2)", "p.c"},
		{"p:nth-child(odd)", "p.a,p.b"},
		{"p:nth-child(1 of .b)", "p.b"},
		{"p:nth-child(2 of .b)", ""},
		{"p:nth-last-child(1)", "p.b,p.c"},
		{"p:not(.a)", "p.b,p.c"},
		{"p:not(.a, .c)", "p.b"},
		{"*:not(p)", "html,head,body,div#o,span.s"},
		{"p:is(:first-child)", "p.a,p.b"},
		{"div:has(> p)", "div#o"},
		{"div:has(span p)", "div#o"},
		{"div:has(+ p)", "div#o"},
		{"span:has(p)", "span.s"},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			body := fmt.Sprintf(`out = list(document.querySelectorAll(%s));`, strconv.Quote(c.sel))
			if got := runSelectorScript(t, body); got != c.want {
				t.Fatalf("querySelectorAll(%q) = %q, want %q", c.sel, got, c.want)
			}
		})
	}
}

// qc:BR-217
func TestSelectorMatchingScopeSemantics(t *testing.T) {
	body := `
var a = document.querySelector(".a"), b = document.querySelector(".b");
out = [
	id(document.querySelector(":scope")),
	document.querySelectorAll(":scope").length,
	document.querySelectorAll(":scope p").length,
	id(document.querySelector(":scope > p")),
	o.querySelectorAll(":scope > p").length,
	o.querySelectorAll(":scope p").length,
	o.querySelectorAll(":scope").length,
	String(o.matches(":scope")),
	String(a.matches(":scope")),
	String(a.matches(":scope > p")),
	id(a.closest(":scope")),
	id(b.closest(":scope")),
	id(b.closest(":scope > span")),
	id(document.querySelector(":scope :scope"))
].join("|");`
	want := "html|1|3|null|1|2|0|true|true|false|p.a|p.b|null|null"
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("scope semantics = %q, want %q", got, want)
	}
}

// qc:BR-218
func TestSelectorMatchingMatchesAndClosest(t *testing.T) {
	body := `
var a = document.querySelector(".a"), b = document.querySelector(".b"), c = document.querySelector(".c");
out = [
	String(o.matches("div")),
	String(o.matches("p")),
	String(o.matches("div:has(p)")),
	String(o.matches("div > p")),
	String(a.matches("div p")),
	id(b.closest("p")),
	id(b.closest("span")),
	id(b.closest("div")),
	id(c.closest("div")),
	id(c.closest("body")),
	id(b.closest("p.a, p.b")),
	id(a.closest("p, div")),
	id(a.closest("html")),
	typeof document.matches
].join("|");`
	want := "true|false|true|false|true|p.b|span.s|div#o|null|body|p.b|p.a|html|undefined"
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("matches/closest = %q, want %q", got, want)
	}
}

// qc:BR-219
func TestSelectorMatchingCaseRules(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"DIV", "div#o"},
		{"DIV > P", "p.a"},
		{"#O", ""},
		{".A", ""},
		{"div#O", ""},
		{"[DATA-X]", "div#o"},
		{"[data-x=Y]", ""},
		{"[data-x=y]", "div#o"},
	}
	for _, c := range cases {
		t.Run(c.sel, func(t *testing.T) {
			body := fmt.Sprintf(`out = list(document.querySelectorAll(%s));`, strconv.Quote(c.sel))
			if got := runSelectorScript(t, body); got != c.want {
				t.Fatalf("querySelectorAll(%q) = %q, want %q", c.sel, got, c.want)
			}
		})
	}
}

// qc:BR-220
func TestSelectorMatchingSyntaxErrors(t *testing.T) {
	invalid := []string{
		"p:not()",
		"p:unknown",
		"div >",
		"> div",
		"div >> p",
		"#1",
		".1",
		"[a b]",
		"[a=]",
		"[a=1]",
		"[.]",
		"[1a]",
		"[]",
		"p:not(.a))",
		"p:nth-child(foo)",
		"p:nth-child(2 of )",
		"p:not(::before)",
		`[a="v" s]`,
		".a,",
		",.a",
		"p ,",
	}
	for _, sel := range invalid {
		t.Run(sel, func(t *testing.T) {
			q := strconv.Quote(sel)
			body := fmt.Sprintf(`
out = [
	thrown(function() { document.querySelector(%[1]s); }),
	thrown(function() { document.querySelectorAll(%[1]s); }),
	thrown(function() { o.querySelector(%[1]s); }),
	thrown(function() { o.querySelectorAll(%[1]s); }),
	thrown(function() { document.querySelector(".a").matches(%[1]s); }),
	thrown(function() { document.querySelector(".a").closest(%[1]s); })
].join("|");`, q)
			want := "SyntaxError|SyntaxError|SyntaxError|SyntaxError|SyntaxError|SyntaxError"
			if got := runSelectorScript(t, body); got != want {
				t.Fatalf("invalid selector %q = %q, want %q", sel, got, want)
			}
		})
	}
}

// qc:BR-221
func TestSelectorMatchingValidButEmptySelectors(t *testing.T) {
	valid := []string{":is()", ":where()", "::before", "p::before", "p:empty", "p:has(> figure)"}
	for _, sel := range valid {
		t.Run(sel, func(t *testing.T) {
			q := strconv.Quote(sel)
			body := fmt.Sprintf(`
out = [
	thrown(function() { document.querySelector(%[1]s); }),
	document.querySelectorAll(%[1]s).length,
	thrown(function() { o.querySelectorAll(%[1]s); }),
	o.querySelectorAll(%[1]s).length
].join("|");`, q)
			want := "no-throw|0|no-throw|0"
			if got := runSelectorScript(t, body); got != want {
				t.Fatalf("valid selector %q = %q, want %q", sel, got, want)
			}
		})
	}
}

// qc:BR-222
func TestSelectorMatchingEmptyQuotedAttributeIsAccepted(t *testing.T) {
	body := `
out = [
	thrown(function() { document.querySelectorAll('[data-x=""]'); }),
	thrown(function() { o.querySelectorAll('[data-x=""]'); }),
	thrown(function() { document.querySelectorAll("[data-x=]"); })
].join("|");`
	want := "no-throw|no-throw|SyntaxError"
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("empty quoted attribute = %q, want %q", got, want)
	}
}

// qc:BR-223
func TestSelectorMatchingSyntaxErrorMessages(t *testing.T) {
	body := `
var a = document.querySelector(".a");
out = [
	message(function() { document.querySelector(""); }),
	message(function() { o.querySelector(" "); }),
	message(function() { o.querySelectorAll("p:not()"); }),
	message(function() { a.matches("p:not()"); }),
	message(function() { a.closest("p:not()"); })
].join("\n");`
	want := strings.Join([]string{
		`Failed to execute 'querySelector' on 'Document': The provided selector is empty.`,
		`Failed to execute 'querySelector' on 'Element': ' ' is not a valid selector.`,
		`Failed to execute 'querySelectorAll' on 'Element': 'p:not()' is not a valid selector.`,
		`Failed to execute 'matches' on 'Element': 'p:not()' is not a valid selector.`,
		`Failed to execute 'closest' on 'Element': 'p:not()' is not a valid selector.`,
	}, "\n")
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("SyntaxError messages =\n%s\nwant\n%s", got, want)
	}
}

// qc:BR-224
func TestSelectorMatchingArgumentCoercion(t *testing.T) {
	body := `
out = [
	document.querySelectorAll(undefined).length,
	document.querySelectorAll(null).length,
	document.querySelectorAll(["p"]).length,
	document.querySelectorAll({toString: function() { return "p"; }}).length,
	thrown(function() { document.querySelectorAll(1); }),
	thrown(function() { document.querySelectorAll(); }),
	thrown(function() { document.querySelector(".a").matches(); }),
	thrown(function() { document.querySelector(".a").closest(); }),
	message(function() { document.querySelectorAll(); })
].join("|");`
	want := strings.Join([]string{
		"0", "0", "3", "3", "SyntaxError", "TypeError", "TypeError", "TypeError",
		`Failed to execute 'querySelectorAll' on 'Document': 1 argument required, but only 0 present.`,
	}, "|")
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("argument coercion = %q, want %q", got, want)
	}
}

func BenchmarkSelectorMatchingQueryAll(b *testing.B) {
	doc := domtest.Parse(selectorFixture)
	r, err := js.New(js.Options{URL: "https://example.test/", DOM: doc})
	if err != nil {
		b.Fatalf("js.New: %v", err)
	}
	defer r.Close()
	src := `
var o = document.getElementById("o");
var n = 0;
for (var i = 0; i < 100; i++) {
	n += o.querySelectorAll("div > p, p.a, [data-x=\"y\"], p:nth-child(2)").length;
}
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Run(src, "inline"); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

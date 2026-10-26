package js_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// selectorFixture is the TASK-051 §1 document plus a doctype and an explicit
// <head>. Both additions are load-bearing:
//
//   - The doctype puts the document in standards mode. In quirks mode
//     Chromium matches #id and .class case-insensitively (measured: #O and .A
//     both hit on a doctype-less copy of this document), so a fixture without
//     it would pin a case rule browsers do not apply.
//   - The <head> makes the element count 8 and gives * a non-body first
//     descendant, matching the 8 in §1.
//
// Elements in document order: html, head, body, div#o, p.a, span.s, p.b, p.c.
const selectorFixture = `<!DOCTYPE html><html><head></head><body><div id="o" data-x="y"><p class="a">1</p><span class="s"><p class="b">2</p></span></div><p class="c">3</p></body></html>`

// selectorPrelude gives every case the same identity helper: an element is
// named tag#id.class so a "null" in an expectation can only mean the API
// returned null, never "an element I could not name".
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

// runSelectorScript runs body against a fresh copy of selectorFixture and
// returns the string the script assigned to out.
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

// TestSelectorMatchingSection1Forms pins the fourteen §1 selector forms on all
// four entry points, against the numbers measured in Chromium 153.0.8010.12
// with Playwright 1.63.0 (NODE_PATH=<repo>/node_modules node /tmp/t51/
// measure.js, rc 0):
//
//	"*"             docQS html        docQSA 8   scoped 3   matchesA/B true/true   closestB/C p.b/p.c
//	"div"           docQS div#o       docQSA 1   scoped 0   matches false/false     closestB/C div#o/null
//	"p.a"           docQS p.a         docQSA 1   scoped 1   matches true/false      closestB/C null/null
//	"div#o"         docQS div#o       docQSA 1   scoped 0   matches false/false     closestB/C div#o/null
//	"[data-x]"      docQS div#o       docQSA 1   scoped 0   matches false/false     closestB/C div#o/null
//	`[data-x="y"]`  docQS div#o       docQSA 1   scoped 0   matches false/false     closestB/C div#o/null
//	"div > p"       docQS p.a         docQSA 1   scoped 1   matches true/false      closestB/C null/null
//	".a, .c"        docQS p.a         docQSA 2   scoped 1   matches true/false      closestB/C null/p.c
//	"p:first-child" docQS p.a         docQSA 2   scoped 2   matches true/true       closestB/C p.b/null
//	"div span"      docQS span.s      docQSA 1   scoped 1   matches false/false     closestB/C span.s/null
//	".a"            docQS p.a         docQSA 1   scoped 1   matches true/false      closestB/C null/null
//	"p"             docQS p.a         docQSA 3   scoped 2   matches true/true       closestB/C p.b/p.c
//	"span > p"      docQS p.b         docQSA 1   scoped 1   matches false/true      closestB/C p.b/null
//	"#o"            docQS div#o       docQSA 1   scoped 0   matches false/false     closestB/C div#o/null
//
// §1 calls the table "fourteen forms" but lists thirteen rows; #o is added as
// the fourteenth so the id fast path has a row of its own. The §1 Goosie
// column reproduced exactly on main (13 rows: p, div and .a hit; the other ten
// answered null/0) and matches/closest did not exist at all.
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

// TestSelectorMatchingCombinatorsAndLists pins the four combinators, selector
// lists, document order, and de-duplication. Measured on the fixture with
// Chromium 153.0.8010.12 (/tmp/t51/measure10.js, rc 0): the values in each
// case are the count/order Chromium reported.
// qc:BR-214
func TestSelectorMatchingCombinatorsAndLists(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"div > p", "p.a"},                   // child combinator, measured 1
		{"div span", "span.s"},               // descendant combinator, measured 1
		{"#o span p", "p.b"},                 // descendant chain, measured 1
		{"#o > span", "span.s"},              // child of the scoped root, measured 1
		{"div + p", "p.c"},                   // adjacent sibling, measured 1
		{"p + p", ""},                        // no p has a p sibling, measured 0
		{"div ~ p", "p.c"},                   // general sibling, measured 1
		{"p ~ p", ""},                        // measured 0
		{"div > *", "p.a,span.s"},            // universal on the right, measured 2
		{"body > :first-child", "div#o"},     // measured 1
		{".a, .c", "p.a,p.c"},                // list in document order, measured 2
		{".c, .a", "p.a,p.c"},                // selector order does not reorder, measured 2
		{"p,p,p", "p.a,p.b,p.c"},             // duplicates in the list are de-duplicated, measured 3
		{":is(p, .s)", "p.a,span.s,p.b,p.c"}, // :is list, document order, measured 4
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

// TestSelectorMatchingAttributes pins every attribute form in §3.2 item 3 and
// the operator set the cascade supports. Measured with Chromium 153.0.8010.12
// (/tmp/t51/measure9.js and measure10.js, rc 0); [data-x="y"] is the quoted
// form the pre-fix code could not match, while element_query_test.go keeps the
// unquoted [jsname=sIJmDf] form alive.
// qc:BR-215
func TestSelectorMatchingAttributes(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"[data-x]", "div#o"},     // presence, measured 1
		{`[data-x="y"]`, "div#o"}, // equality, double quoted, measured 1
		{"[data-x='y']", "div#o"}, // equality, single quoted, measured 1
		{"[data-x=y]", "div#o"},   // equality, unquoted, measured 1
		// [data-x=""] is deliberately absent: the cascade matcher counts an
		// absent attribute as an empty value, so the count diverges from
		// Chromium (GOOSIE-CSSATTR-EMPTY-001, §8). Its validity is pinned by
		// TestSelectorMatchingEmptyQuotedAttributeIsAccepted.
		{`[data-x="Y"]`, ""},                  // values are case-sensitive, measured 0
		{`[data-x="Y" i]`, "div#o"},           // i flag folds case, measured 1
		{`[data-x^="y"]`, "div#o"},            // prefix, measured 1
		{`[data-x$="y"]`, "div#o"},            // suffix, measured 1
		{`[data-x*="y"]`, "div#o"},            // substring, measured 1
		{`[data-x|="y"]`, "div#o"},            // dash-prefix, measured 1
		{`[class~="a"]`, "p.a"},               // word match, measured 1
		{`[class="a b"]`, ""},                 // class is "a", not "a b", measured 0
		{`[DATA-X]`, "div#o"},                 // attribute names are case-insensitive, measured 1
		{`[data-x]:not([data-x=z])`, "div#o"}, // compound with :not, measured 1
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

// TestSelectorMatchingPseudoClasses pins the pseudo-classes the cascade
// supports, including a list argument to :not() (Selectors 4; the cascade
// matcher already evaluates it as a list). Measured with Chromium 153.0.8010.12
// (/tmp/t51/measure10.js and measure12.js, rc 0).
// qc:BR-216
func TestSelectorMatchingPseudoClasses(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"p:first-child", "p.a,p.b"},                // measured 2
		{"p:last-child", "p.b,p.c"},                 // measured 2
		{"p:only-child", "p.b"},                     // measured 1
		{"p:empty", ""},                             // measured 0
		{"p:first-of-type", "p.a,p.b,p.c"},          // measured 3
		{"p:nth-last-of-type(1)", "p.a,p.b,p.c"},    // measured 3
		{"p:nth-child(2)", "p.c"},                   // 2nd element child of body, measured 1
		{"p:nth-child(odd)", "p.a,p.b"},             // measured 2
		{"p:nth-child(1 of .b)", "p.b"},             // measured 1
		{"p:nth-child(2 of .b)", ""},                // measured 0
		{"p:nth-last-child(1)", "p.b,p.c"},          // measured 2
		{"p:not(.a)", "p.b,p.c"},                    // measured 2
		{"p:not(.a, .c)", "p.b"},                    // :not list, measured 1
		{"*:not(p)", "html,head,body,div#o,span.s"}, // measured 5
		{"p:is(:first-child)", "p.a,p.b"},           // pseudo in :is, measured 2
		{"div:has(> p)", "div#o"},                   // measured 1
		{"div:has(span p)", "div#o"},                // measured 1
		{"div:has(+ p)", "div#o"},                   // measured 1
		{"span:has(p)", "span.s"},                   // measured 1
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

// TestSelectorMatchingScopeSemantics pins what :scope means at each entry
// point: the document element for a document query, the element for an
// element query, matches, and closest. Measured with Chromium 153.0.8010.12
// (/tmp/t51/measure1.js and measure8.js, rc 0):
//
//	document.querySelector(":scope")        -> html
//	document.querySelectorAll(":scope")     -> 1
//	document.querySelectorAll(":scope p")   -> 3
//	document.querySelector(":scope > p")    -> null
//	o.querySelectorAll(":scope > p")        -> 1
//	o.querySelectorAll(":scope p")          -> 2
//	o.querySelectorAll(":scope")            -> 0 (the root is never a candidate)
//	o.matches(":scope"), a.matches(":scope")-> true
//	a.matches(":scope > p")                 -> false
//	a.closest(":scope"), b.closest(":scope")-> the element itself
//	b.closest(":scope > span")              -> null (its own children only)
//	document.querySelector(":scope :scope") -> null
//
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

// TestSelectorMatchingMatchesAndClosest pins the two entry points TASK-051
// found missing entirely (proposed defect GOOSIE-DOMMATCHCLOSEST-001):
// matches tests the element itself and its ancestors, never its descendants;
// closest starts at the element and stops at the first match. Measured with
// Chromium 153.0.8010.12 (/tmp/t51/measure1.js and measure8.js, rc 0).
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
	// o.matches("div > p") is false: matches never looks at descendants.
	// b.closest("p") is b itself: self is included. a.closest("p, div")
	// answers p.a, the nearest match, not the div above it. document has no
	// matches: undefined (measured: typeof document.matches === "undefined").
	want := "true|false|true|false|true|p.b|span.s|div#o|null|body|p.b|p.a|html|undefined"
	if got := runSelectorScript(t, body); got != want {
		t.Fatalf("matches/closest = %q, want %q", got, want)
	}
}

// TestSelectorMatchingCaseRules pins the case rules §3.2 requires: tag names
// and attribute names ASCII-case-insensitive in HTML, id/class/attribute
// values case-sensitive. Measured in standards mode with Chromium 153.0.8010.12
// (/tmp/t51/measure8.js, rc 0). The white-box test
// TestSelectorFastPathPredicate keeps the uppercase tag name off the index
// path, which is isLowerTag's original purpose.
// qc:BR-219
func TestSelectorMatchingCaseRules(t *testing.T) {
	cases := []struct{ sel, want string }{
		{"DIV", "div#o"},        // tag name folds, measured 1
		{"DIV > P", "p.a"},      // every tag in the compound folds, measured 1
		{"#O", ""},              // id is case-sensitive, measured 0
		{".A", ""},              // class is case-sensitive, measured 0
		{"div#O", ""},           // measured 0
		{"[DATA-X]", "div#o"},   // attribute name folds, measured 1
		{"[data-x=Y]", ""},      // attribute value is case-sensitive, measured 0
		{"[data-x=y]", "div#o"}, // measured 1
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

// TestSelectorMatchingSyntaxErrors pins §3.2 item 6: an invalid selector
// raises a SyntaxError at every entry point instead of returning an empty
// result. Measured with Chromium 153.0.8010.12 (/tmp/t51/measure8.js and
// measure9.js, rc 0): every selector in wantInvalid throws a DOMException
// whose .name is "SyntaxError" on Document and on Element queries.
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

// TestSelectorMatchingValidButEmptySelectors is the other half of the
// SyntaxError rule: a selector a browser accepts must answer empty, not throw.
// Measured with Chromium 153.0.8010.12: ":is()" and ":where()" match nothing
// (measure12.js), "::before", "p::before" and "p:has(> figure)" match nothing
// (/tmp/t51/measure8.js, measure13.js, rc 0).
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

// TestSelectorMatchingEmptyQuotedAttributeIsAccepted pins the validity half of
// "[a=]" vs `[a=""]`: Chromium accepts the empty quoted value and rejects the
// missing one (measured with Chromium 153.0.8010.12, /tmp/t51/measure9.js and
// measure10.js, rc 0). The *count* is not pinned: the cascade matcher treats an
// absent attribute as an empty value, so Goosie answers 7 where Chromium
// answers 0. That is the cascade's bug, reported as GOOSIE-CSSATTR-EMPTY-001
// in §8 and out of scope here per §6, so this case asserts only that the query
// API accepts the selector and that the missing-value form throws.
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

// TestSelectorMatchingSyntaxErrorMessages pins the two Chromium message
// shapes: the empty string and whitespace both raise the "empty" wording for
// "" only, and everything else is quoted back. Measured with Chromium
// 153.0.8010.12 (/tmp/t51/measure8.js, rc 0):
//
//	document.querySelector("") ->
//	  Failed to execute 'querySelector' on 'Document': The provided selector is empty.
//	o.querySelector(" ") ->
//	  Failed to execute 'querySelector' on 'Element': ' ' is not a valid selector.
//	o.querySelectorAll("p:not()") ->
//	  Failed to execute 'querySelectorAll' on 'Element': 'p:not()' is not a valid selector.
//	a.matches("p:not()") ->
//	  Failed to execute 'matches' on 'Element': 'p:not()' is not a valid selector.
//	a.closest("p:not()") ->
//	  Failed to execute 'closest' on 'Element': 'p:not()' is not a valid selector.
//
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

// TestSelectorMatchingArgumentCoercion pins the argument handling measured on
// Chromium 153.0.8010.12 (/tmp/t51/measure8.js, rc 0): the selector is
// String(v), so undefined/null become type selectors matching nothing and an
// array's own toString is used; a missing argument is a TypeError with
// Chromium's wording.
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

// BenchmarkSelectorMatchingQueryAll measures the general-path query with the
// compiled-selector cache in place. M05 in the task doc removes the cache and
// runs this same benchmark; the recorded numbers live in §8 of TASK-051 and no
// test asserts a duration.
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

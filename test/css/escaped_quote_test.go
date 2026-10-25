package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// A backslash escapes the next byte anywhere in the token stream (CSS Syntax L3
// 4.3.8), so an escaped quote can neither open nor close a string - including
// inside an unquoted url(), where minified data URIs are full of `\'`. Goosie
// skipped escapes only once a string was already open, so the first `\'` still
// started one and nothing after it in the sheet survived. Every expectation here
// is what Chromium computed on the same input, measured 2026-10-26
// (GOOSIE-CSSSTRING-003).

// qc:BR-191
func TestEscapedQuoteInUnquotedURLKeepsEveryRule(t *testing.T) {
	sheet := css.Parse(".u{background-image:url(data:image/svg+xml,%3csvg width=\\'26\\' height=\\'26\\'%3e%3c/svg%3e);" +
		"width:120px;height:120px}\n.after{background:#0f0;width:120px;height:120px}\n")
	mustNotLeakCSS(t, sheet)

	u := declarationsFor(t, sheet, ".u")
	if len(u) != 3 {
		t.Errorf(".u kept %d declaration(s), want 3 (Chromium applies the two sizes and drops only the unparseable url): %+v", len(u), u)
	}
	after := declarationsFor(t, sheet, ".after")
	if len(after) != 3 {
		t.Errorf("the rule after an escaped-quote url kept %d declaration(s), want 3: %+v", len(after), after)
	}
}

// qc:BR-191
func TestOddApostropheCountInURLDoesNotEatTheSheet(t *testing.T) {
	// The shape the old scanner survived only by luck: with an even number of
	// quote characters the state happened to close again.
	sheet := css.Parse(".o{background:url(data:x\\'y);width:30px}\n.p{color:#00f;width:20px}\n.q{color:#0f0}\n")
	mustNotLeakCSS(t, sheet)

	for sel, want := range map[string]int{".o": 2, ".p": 2, ".q": 1} {
		if got := len(declarationsFor(t, sheet, sel)); got != want {
			t.Errorf("%s kept %d declaration(s), want %d: %+v", sel, got, want, declarationsFor(t, sheet, sel))
		}
	}
}

// qc:BR-191
func TestEscapedDoubleQuoteInsideStringContinuesIt(t *testing.T) {
	// Chromium computes content as the literal `"a\"b"`, so the escape belongs to
	// the value and the string ends at the last quote: all three declarations stay.
	sheet := css.Parse(".c{content:\"a\\\"b\"; width:40px; height:40px}\n.d{color:#0f0}\n")
	mustNotLeakCSS(t, sheet)

	c := declarationsFor(t, sheet, ".c")
	if len(c) != 3 {
		t.Fatalf(".c kept %d declaration(s), want 3: %+v", len(c), c)
	}
	if c[0].Property != "content" || !strings.Contains(c[0].Value, "b") {
		t.Errorf("first declaration is %+v, want the content declaration holding the escaped quote", c[0])
	}
	if len(declarationsFor(t, sheet, ".d")) != 1 {
		t.Errorf("rule after an escaped quote kept %d declaration(s), want 1", len(declarationsFor(t, sheet, ".d")))
	}
}

// qc:BR-191
func TestEscapedQuoteInsideSelectorAttributeKeepsFollowingRules(t *testing.T) {
	sheet := css.Parse(".s[data-x=\"a\\\"b\"]{background:#f00;width:50px}\n.t{background:#000;width:50px}\n")
	mustNotLeakCSS(t, sheet)

	const sel = `.s[data-x="a\"b"]`
	if got := declarationsFor(t, sheet, sel); len(got) != 2 {
		t.Errorf("rule whose selector quotes an escaped quote kept %d declaration(s), want 2: %+v (%+v)", len(got), got, selectorStrs(sheet))
	}
	if len(declarationsFor(t, sheet, ".t")) != 2 {
		t.Errorf("rule after it kept %d declaration(s), want 2", len(declarationsFor(t, sheet, ".t")))
	}
}

// qc:BR-191
func TestUnescapedApostropheInURLLosesOnlyItsDeclaration(t *testing.T) {
	// Control for the case above: an unescaped apostrophe is not an escape, and
	// Chromium drops that one declaration (background-image computes to none)
	// while the sizes and the next rule survive.
	sheet := css.Parse(".o{background:url(data:x'y);width:30px;height:30px}\n.p{color:#00f;width:20px}\n")
	mustNotLeakCSS(t, sheet)

	if got := len(declarationsFor(t, sheet, ".o")); got != 3 {
		t.Errorf(".o kept %d declaration(s), want 3 (the url is invalid, the sizes are not): %+v", got, declarationsFor(t, sheet, ".o"))
	}
	if got := len(declarationsFor(t, sheet, ".p")); got != 2 {
		t.Errorf(".p kept %d declaration(s), want 2: %+v", got, declarationsFor(t, sheet, ".p"))
	}
}

// selectorStrs is a failure-message helper: an attribute selector keeps its
// whole text, so a lookup that returns nothing needs the real keys printed.
func selectorStrs(sheet *css.Stylesheet) []string {
	var out []string
	for _, r := range sheet.Rules {
		out = append(out, r.SelectorStrs...)
	}
	return out
}

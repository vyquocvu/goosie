package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// A string with no closing quote is a CSS bad-string, and the spec ends it at
// the newline so only the declaration that contains it is lost. Goosie instead
// let the quote run to end of input, so `content: """;` swallowed the rest of
// the stylesheet and the page painted that CSS as generated text. Every
// expectation below is what Chromium computed on the same input, measured
// 2026-10-26 (TASK-041 §1.2, §2.1).
func declarationsFor(t *testing.T, sheet *css.Stylesheet, selector string) []css.Declaration {
	t.Helper()
	for _, r := range sheet.Rules {
		for _, s := range r.SelectorStrs {
			if s == selector {
				return r.Declarations
			}
		}
	}
	return nil
}

func mustNotLeakCSS(t *testing.T, sheet *css.Stylesheet) {
	t.Helper()
	for _, r := range sheet.Rules {
		for _, d := range r.Declarations {
			if strings.ContainsAny(d.Value, "{}") || strings.Contains(d.Value, "\n") {
				t.Errorf("rule %q declaration %q kept raw stylesheet text as its value: %q",
					r.SelectorStrs, d.Property, d.Value)
			}
		}
	}
}

// qc:BR-186
func TestBadStringEndsAtNewline(t *testing.T) {
	// The broken content declaration and the one after it are both invalid (the
	// bad-string ate the `;`), but the rules that follow must still parse.
	sheet := css.Parse(".q::before {\n content: \"\"\";\n font-size: 48px;\n color: #3498db;\n}\n" +
		".q { font-family: monospace; color: #000; }\n")
	mustNotLeakCSS(t, sheet)

	if got := declarationsFor(t, sheet, ".q::before"); len(got) != 1 || got[0].Property != "color" || got[0].Value != "#3498db" {
		t.Errorf("bad-string rule parsed to %+v, want exactly the one declaration that follows the dropped one (color: #3498db)", got)
	}
	q := declarationsFor(t, sheet, ".q")
	if len(q) != 2 {
		t.Fatalf("rule after the bad-string kept %d declaration(s), want 2: %+v", len(q), q)
	}
	if q[0].Property != "font-family" || q[0].Value != "monospace" {
		t.Errorf("following rule parsed as %q: %q, want font-family: monospace", q[0].Property, q[0].Value)
	}
}

// qc:BR-186
func TestBadStringDropsOneDeclarationNotTheSheet(t *testing.T) {
	sheet := css.Parse(".u{font-family:\"x;\n color:#00f;}\n.v{color:#0f0;}\n.w{color:#00f;}")
	mustNotLeakCSS(t, sheet)

	if got := declarationsFor(t, sheet, ".u"); len(got) != 0 {
		t.Errorf(".u kept %d declaration(s), want 0: %+v", len(got), got)
	}
	for _, sel := range []string{".v", ".w"} {
		got := declarationsFor(t, sheet, sel)
		if len(got) != 1 {
			t.Fatalf("%s kept %d declaration(s), want 1: %+v", sel, len(got), got)
		}
		if got[0].Value != "#00f" && got[0].Value != "#0f0" {
			t.Errorf("%s parsed value %q, want the colour it declares", sel, got[0].Value)
		}
	}
}

// qc:BR-186
func TestBadStringSingleQuoteMatchesDoubleQuote(t *testing.T) {
	dbl := css.Parse(".u{font-family:\"x;\n color:#00f;}\n.v{color:#0f0;}")
	sgl := css.Parse(".u{font-family:'x;\n color:#00f;}\n.v{color:#0f0;}")
	mustNotLeakCSS(t, dbl)
	mustNotLeakCSS(t, sgl)

	if len(declarationsFor(t, dbl, ".u")) != len(declarationsFor(t, sgl, ".u")) {
		t.Errorf("quote character changed the outcome: double kept %d declaration(s), single kept %d",
			len(declarationsFor(t, dbl, ".u")), len(declarationsFor(t, sgl, ".u")))
	}
	if len(declarationsFor(t, sgl, ".v")) != 1 {
		t.Errorf("single-quoted bad-string lost the following rule: %+v", sgl.Rules)
	}
}

// qc:BR-186
func TestEmptyStringStillGeneratesABox(t *testing.T) {
	// The control for the cases above: a properly closed string must not be
	// treated as a bad-string, or every `content: ""` rule would start dropping.
	sheet := css.Parse(".t::before{content: \"\"; font-size: 48px;}")
	mustNotLeakCSS(t, sheet)

	got := declarationsFor(t, sheet, ".t::before")
	if len(got) != 2 {
		t.Fatalf("closed string parsed to %d declaration(s), want 2: %+v", len(got), got)
	}
	if got[0].Property != "content" {
		t.Fatalf("first declaration is %q, want content", got[0].Property)
	}
	if got[1].Property != "font-size" || got[1].Value != "48px" {
		t.Errorf("second declaration is %q: %q, want font-size: 48px", got[1].Property, got[1].Value)
	}
}

// qc:BR-186
func TestBadStringInUnterminatedLastDeclaration(t *testing.T) {
	// The common malformed shape: the last declaration of a block has no closing
	// semicolon, so the bad string reaches the block end instead of a ';'.
	// Chromium measured on this input drops the declaration and keeps the rules
	// after it (.v colour, .w monospace), so nothing may carry stylesheet text.
	sheet := css.Parse(".u{font-family:\"x;\n serif}\n.v{color:#0f0;}\n.w{font-family:monospace}")
	mustNotLeakCSS(t, sheet)

	if got := declarationsFor(t, sheet, ".u"); len(got) != 0 {
		t.Errorf(".u kept %d declaration(s), want 0: %+v", len(got), got)
	}
	if got := declarationsFor(t, sheet, ".v"); len(got) != 1 || got[0].Value != "#0f0" {
		t.Errorf("rule after the bad string parsed as %+v, want the one declaration color: #0f0", got)
	}
	if got := declarationsFor(t, sheet, ".w"); len(got) != 1 || got[0].Value != "monospace" {
		t.Errorf("last rule parsed as %+v, want the one declaration font-family: monospace", got)
	}
}

// qc:BR-186
func TestUnterminatedStringAtEOFStaysLenient(t *testing.T) {
	// Chromium keeps a string that runs off the end of the sheet as a string
	// (measured: font-family computes to the literal `x;`), so this must NOT be
	// turned into a bad-string. The rule before it is unaffected.
	sheet := css.Parse(".v{color:#0f0;}\n.u{font-family:\"x;")
	mustNotLeakCSS(t, sheet)

	v := declarationsFor(t, sheet, ".v")
	if len(v) != 1 {
		t.Fatalf("rule before the EOF string lost its declaration: %+v", v)
	}
	u := declarationsFor(t, sheet, ".u")
	if len(u) != 1 || u[0].Property != "font-family" {
		t.Fatalf("EOF string changed %d declaration(s), want the one it declares: %+v", len(u), u)
	}
}

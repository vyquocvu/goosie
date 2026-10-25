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

// qc:BR-186
func TestEscapedNewlineContinuesAString(t *testing.T) {
	// CSS Syntax L3 4.3.8: `\` + newline is a valid escape *inside* a string, so
	// the token keeps going and the declaration is fine. Chromium measured on
	// this exact shape keeps both declarations and computes the URL as
	// `data:image/svg+xml,%3Csvg xmlns='a'%3E%3C/svg%3E` - the escape pair
	// contributes no character - and the image loads. Goosie treated the escaped
	// break as the line break that ends a bad-string and threw the declaration
	// away, which is how every multi-line quoted data URI stopped painting.
	sheet := css.Parse(".a{background:url(\"data:image/svg+xml,%3Csvg\\\n xmlns='a'%3E%3C/svg%3E\"); color:#0f0;}\n.b{color:#00f;}")

	a := declarationsFor(t, sheet, ".a")
	if len(a) != 2 {
		t.Fatalf("escaped newline left %d declaration(s) in .a, want 2 (background, color): %+v", len(a), a)
	}
	if a[0].Property != "background" || !strings.Contains(a[0].Value, "%3Csvg") {
		t.Errorf("background parsed as %q: %q, want the data URI the escape held together", a[0].Property, a[0].Value)
	}
	if strings.Contains(a[0].Value, "\\\n") || strings.Contains(a[0].Value, "\\\r") {
		t.Errorf("background value still carries the escape pair: %q, want it removed as Chromium computes it", a[0].Value)
	}
	if a[1].Property != "color" || a[1].Value != "#0f0" {
		t.Errorf("declaration after the escaped newline is %q: %q, want color: #0f0", a[1].Property, a[1].Value)
	}
	b := declarationsFor(t, sheet, ".b")
	if len(b) != 1 || b[0].Value != "#00f" {
		t.Errorf("rule after the escaped string parsed as %+v, want the one declaration color: #00f", b)
	}
}

// qc:BR-186
func TestEscapedNewlineVersusRawNewline(t *testing.T) {
	// The pair that makes the test above an oracle rather than a loosening: the
	// same sheet with the backslash removed must still lose the declarations.
	// Chromium measured on the raw form keeps the *rule* (selectorText `.a`, one
	// cssRule) and nothing inside it, so the block still cannot leak CSS bytes.
	escaped := css.Parse(".a{font-family:\"Goosie\\\nSerif\"; color:#0f0;}")
	raw := css.Parse(".a{font-family:\"Goosie\nSerif\"; color:#0f0;}")

	if got := declarationsFor(t, escaped, ".a"); len(got) != 2 {
		t.Errorf("\\ + newline kept %d declaration(s), want 2: %+v", len(got), got)
	}
	if got := declarationsFor(t, raw, ".a"); len(got) != 0 {
		t.Errorf("raw newline kept %d declaration(s), want 0 as Chromium computes it: %+v", len(got), got)
	}
	mustNotLeakCSS(t, raw)
}

// qc:BR-186
func TestEscapedNewlineInSelectorString(t *testing.T) {
	// The selector scanner reads the same escape rule, so an attribute value with
	// an escaped line break must not make the prelude run past the `{`: Chromium
	// measured 2 rules and the following `.z` still applying its colour.
	sheet := css.Parse("a[title=\"x\\\ny\"]{color:#0f0;}\n.z{color:#00f;}")

	if len(sheet.Rules) < 2 {
		t.Fatalf("escaped newline in the selector collapsed the sheet to %d rule(s), want 2: %+v", len(sheet.Rules), sheet.Rules)
	}
	if got := declarationsFor(t, sheet, ".z"); len(got) != 1 || got[0].Value != "#00f" {
		t.Errorf("rule after the escaped selector string parsed as %+v, want the one declaration color: #00f", got)
	}
}

package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

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
	sheet := css.Parse("a[title=\"x\\\ny\"]{color:#0f0;}\n.z{color:#00f;}")

	if len(sheet.Rules) < 2 {
		t.Fatalf("escaped newline in the selector collapsed the sheet to %d rule(s), want 2: %+v", len(sheet.Rules), sheet.Rules)
	}
	if got := declarationsFor(t, sheet, ".z"); len(got) != 1 || got[0].Value != "#00f" {
		t.Errorf("rule after the escaped selector string parsed as %+v, want the one declaration color: #00f", got)
	}
}

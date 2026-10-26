package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// svgParen is a quoted data-URI background whose payload carries a ')' — the
// GOOSIE-CSSURLPAREN-001 discriminator shared by the §2.3 site oracles below.
const svgParen = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='rgb(0,0,255)'/></svg>`

// TestAbsolutizeCSSURLsKeepsParenPayload is the per-site oracle for
// internal/engine/limits.go's absolutizeCSSURLs (§2.3 row 3). The old code cut
// each url() at the first ')' and then re-emitted the leftover tail as new sheet
// text, so a quoted data URI whose payload contains a ')' was BOTH truncated AND
// rewritten. A data: URI must now round-trip byte for byte, while relative
// references still resolve against the sheet URL.
// qc:BR-212
func TestAbsolutizeCSSURLsKeepsParenPayload(t *testing.T) {
	const sheetURL = `https://e/x/s.css`
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"quoted data uri verbatim", `.a{background:url("` + svgParen + `")}`, `.a{background:url("` + svgParen + `")}`},
		{"single-quoted data uri verbatim", `.a{background:url('` + svgParen + `')}`, `.a{background:url('` + svgParen + `')}`},
		{"unquoted data uri verbatim", `.a{background:url(` + svgParen + `)}`, `.a{background:url(` + svgParen + `)}`},
		{"relative unquoted rewritten", `.a{background:url(icon.png)}`, `.a{background:url(https://e/x/icon.png)}`},
		{"relative quoted rewritten", `.a{background:url("icon.png")}`, `.a{background:url("https://e/x/icon.png")}`},
		{"relative single-quoted keeps its quote", `.a{background:url('icon.png')}`, `.a{background:url('https://e/x/icon.png')}`},
		{"two tokens one rewritten one kept", `.a{background:url(icon.png),url("` + svgParen + `")}`, `.a{background:url(https://e/x/icon.png),url("` + svgParen + `")}`},
		{"no url untouched", `.a{color:red}`, `.a{color:red}`},
		{"tail after a kept uri survives", `.a{background:url("` + svgParen + `")}.b{color:red}`, `.a{background:url("` + svgParen + `")}.b{color:red}`},
		// Malformed token: fail closed and stop rewriting, emitting the rest of
		// the sheet verbatim rather than inventing text.
		{"unterminated quote stops rewriting", `.a{background:url("icon.png)}.b{color:red}`, `.a{background:url("icon.png)}.b{color:red}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := absolutizeCSSURLs(tc.in, sheetURL); got != tc.want {
				t.Errorf("absolutizeCSSURLs(%q) =\n  %q\nwant\n  %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAbsolutizeCSSURLsNoTailGarbage pins the "and worse" clause of §2.3 row 3:
// the old scan advanced past the FIRST ')' inside the payload, so everything from
// the cut point to the real ')' leaked back into the sheet as stray text. The
// ')' count and the intact rgb(...) marker prove the tail is not re-emitted.
// qc:BR-212
func TestAbsolutizeCSSURLsNoTailGarbage(t *testing.T) {
	in := `.a{background:url("` + svgParen + `");color:red}`
	out := absolutizeCSSURLs(in, `https://e/x/s.css`)
	if !strings.Contains(out, `rgb(0,0,255)`) {
		t.Errorf("payload truncated, rgb() marker lost:\n%s", out)
	}
	if strings.Count(out, ")") != strings.Count(in, ")") {
		t.Errorf("paren count changed (in=%d out=%d):\n%s", strings.Count(in, ")"), strings.Count(out, ")"), out)
	}
	if !strings.Contains(out, `color:red}`) {
		t.Errorf("declaration after the url() was corrupted:\n%s", out)
	}
}

// TestImportRefURLParen is the per-site oracle for internal/engine/imports.go's
// importRef (§2.3 row 4): @import url("...") payloads containing a ')' must not be
// truncated, and media conditions after the url() must still be honoured. Low
// blast radius, stated by measurement rather than assumed.
// qc:BR-212
func TestImportRefURLParen(t *testing.T) {
	cases := []struct {
		name   string
		stmt   string
		want   string
		wantOK bool
	}{
		{"plain quoted", `@import url("theme.css");`, "theme.css", true},
		{"payload with ')' kept", `@import url("a)b.css");`, "a)b.css", true},
		{"payload with ';' kept", `@import url("a;b.css");`, "a;b.css", true},
		{"unquoted payload", `@import url(theme.css);`, "theme.css", true},
		{"screen condition", `@import url("t.css") screen;`, "t.css", true},
		{"condition after a paren payload", `@import url("a)b.css") screen;`, "a)b.css", true},
		{"print condition rejected", `@import url("t.css") print;`, "", false},
		{"print after a paren payload rejected", `@import url("a)b.css") print;`, "", false},
		{"leading string form", `@import "theme.css";`, "theme.css", true},
		{"empty url", `@import url();`, "", false},
		{"unterminated fails closed", `@import url("abc`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := importRef(tc.stmt)
			if ok != tc.wantOK {
				t.Fatalf("importRef(%q) ok = %v, want %v", tc.stmt, ok, tc.wantOK)
			}
			if ref != tc.want {
				t.Errorf("importRef(%q) ref = %q, want %q", tc.stmt, ref, tc.want)
			}
		})
	}
}

// TestExtractFontURLKeepsParenPayload is the per-site oracle for the reference
// implementation at internal/engine/engine.go:706 (§2.3 row 5). That site already
// handled a leading quote before the shared helper existed, so routing it through
// css.ScanURL must keep that behaviour AND fix two things it did not do: a
// stray-quote payload (the vector-background shape) and an unquoted payload with
// balanced parentheses.
// qc:BR-212
func TestExtractFontURLKeepsParenPayload(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"quoted payload with ')'", `url("a)b.woff2") format("woff2")`, "a)b.woff2"},
		{"single quoted payload with ')'", `url('a)b.woff2') format("woff2")`, "a)b.woff2"},
		{"single quoted", `url('font.woff2')`, "font.woff2"},
		{"plain unquoted", `url(font.woff2)`, "font.woff2"},
		{"unquoted balanced parens", `url(a(b)c.woff2)`, "a(b)c.woff2"},
		{"local then url", `local(Arial), url("a)b.woff2")`, "a)b.woff2"},
		{"first of many formats", `url(a.woff2) format("woff2"), url(b.woff) format("woff")`, "a.woff2"},
		{"stray quotes in a single-quoted payload", `url('data:font/woff2;charset=x,y;base64,AAA')`, `data:font/woff2;charset=x,y;base64,AAA`},
		{"no url", `local(Arial)`, ""},
		{"unterminated quote fails closed", `url("abc`, ""},
		{"unterminated unquoted fails closed", `url(abc`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFontURL(tc.in); got != tc.want {
				t.Errorf("extractFontURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCollectFontJobsParenPayload drives the same site through its real entry:
// an @font-face rule parsed from a sheet, collected by collectFontJobs and
// resolved against the sheet base. A '(' in an unquoted src payload used to cut
// the URL at the wrong place and drop the job outright.
// qc:BR-212
func TestCollectFontJobsParenPayload(t *testing.T) {
	sheet := css.Parse(`@font-face{font-family:"Paren";src:url("a)b.woff2") format("woff2");}`)
	jobs := collectFontJobs([]*css.Stylesheet{sheet}, "https://e/x/s.css")
	if len(jobs) != 1 {
		t.Fatalf("font jobs = %d, want 1 (%+v)", len(jobs), jobs)
	}
	if jobs[0].srcURL != "https://e/x/a)b.woff2" {
		t.Errorf("font src URL = %q, want the payload kept whole", jobs[0].srcURL)
	}
	if jobs[0].family != "Paren" {
		t.Errorf("font family = %q, want Paren", jobs[0].family)
	}
}

// TestExpandCSSImportsURLKeepsParenPayload is the end-to-end INT for
// absolutizeCSSURLs demanded by §2.3 row 3: a sheet that @imports another sheet
// whose quoted data-URI background contains a ')' must splice in with the payload
// whole and the declaration around it intact. The base must be http(s):
// resolveSubresourceURL deliberately refuses any other scheme for a subresource
// (limits.go), so a file:// base would test the security rule instead of the
// scanner.
// qc:BR-212
func TestExpandCSSImportsURLKeepsParenPayload(t *testing.T) {
	linker := func(base, href string) (string, error) {
		if href == "https://e/p/inner.css" {
			return ".x{background:url(\"" + svgParen + "\");color:red}", nil
		}
		return "", errors.New("missing")
	}
	in := `@import url("inner.css");`
	out := expandCSSImports(linker, "https://e/p/page.css", in, map[string]bool{}, &subBudget{}, 1)
	if strings.Contains(out, "@import") {
		t.Fatalf("statement was not spliced out:\n%s", out)
	}
	if !strings.Contains(out, `rgb(0,0,255)`) {
		t.Fatalf("imported sheet lost the rgb() payload:\n%s", out)
	}
	if !strings.Contains(out, `.x{background:url("`+svgParen+`");color:red}`) {
		t.Errorf("imported rule not spliced in intact:\n%s", out)
	}
	if got, want := strings.Count(out, ")"), strings.Count(svgParen, ")")+1; got != want {
		t.Errorf("paren count = %d, want %d (payload plus the url() close); tail probably re-emitted:\n%s", got, want, out)
	}
}

// BenchmarkAbsolutizeCSSURLsBigSheet is the cap-discipline guard for §3's rule
// that a quote-aware scan must not grow the work beyond one linear pass per
// value: a 256 KiB sheet with a url() every 4 KiB is rewritten with zero
// allocations beyond the output buffer.
func BenchmarkAbsolutizeCSSURLsBigSheet(b *testing.B) {
	var sb strings.Builder
	chunk := strings.Repeat("a", 4096)
	for i := 0; i < 64; i++ {
		sb.WriteString(chunk)
		sb.WriteString(`.c{background:url(icon.png)}`)
	}
	sb.WriteString(`.d{background:url("` + svgParen + `")}`)
	sheet := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := absolutizeCSSURLs(sheet, "https://e/x/s.css"); len(got) < len(sheet) {
			b.Fatal("sheet shrank")
		}
	}
}

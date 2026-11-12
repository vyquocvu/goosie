package engine

import (
	"errors"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestExtractFontURLDecodesEscapes is the per-site oracle for extractFontURL:
// the src list is scanned for the first url() and the fetched URL is the one the
// escapes denote, while an unterminated token still yields no URL at all.
//
// The measured rows come from driving Chromium over a local HTTP server on
// 2026-11-12 and reading the requests it made for a sheet at /sub.css:
// `url("i\63 on.png")` fetched /icon.png, `url("ic\6fn.png")` fetched /icon.png
// as well (a hex escape stops at the first non-hex digit, so no terminator is
// needed), `url("br\"dr.png")` fetched /br"dr.png, `url(no\2dsuch.png)` fetched
// /no-such.png, `url("a\ b.png")` fetched /a b.png, `@import url("b\42 .css")`
// fetched /bB.css, and an escaped `fo\6f.ttf` in @font-face fetched /foo.ttf.
// Every reader below pins the same expectation one layer inside this engine.
// qc:BR-269
func TestExtractFontURLDecodesEscapes(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"hex escape", `url("fo\6f.woff2") format("woff2")`, "foo.woff2"},
		{"hex escape, unquoted", `url(fo\6f.woff2)`, "foo.woff2"},
		{"hex escape with no terminator", `url("ic\6fn.woff2")`, "icon.woff2"},
		{"escaped quote", `url("fo\"o.woff2")`, `fo"o.woff2`},
		{"escaped paren", `url(fo\)o.woff2)`, "fo)o.woff2"},
		{"escaped space", `url("fo\ o.woff2")`, "fo o.woff2"},
		{"local then escaped url", `local(Arial), url("fo\6f.woff2")`, "foo.woff2"},
		{"no escape is untouched", `url("foo.woff2") format("woff2")`, "foo.woff2"},
		{"unterminated fails closed", `url("fo\`, ""},
		{"local only", `local(Arial)`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractFontURL(tc.in); got != tc.want {
				t.Errorf("extractFontURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCollectFontJobsResolvesAnEscapedSrc drives the same site through its real
// entry: an @font-face parsed from a sheet, collected and resolved against the
// sheet base. The URL that leaves here is the one a font fetch is asked for, so
// a payload that kept its backslashes would be a 404 for a file whose name
// contains them.
// qc:BR-269
func TestCollectFontJobsResolvesAnEscapedSrc(t *testing.T) {
	sheet := css.Parse(`@font-face{font-family:"Esc";src:url("fo\6f.woff2") format("woff2");}`)
	jobs := collectFontJobs([]*css.Stylesheet{sheet}, "https://e/x/s.css")
	if len(jobs) != 1 {
		t.Fatalf("font jobs = %d, want 1 (%+v)", len(jobs), jobs)
	}
	if jobs[0].srcURL != "https://e/x/foo.woff2" {
		t.Errorf("font src URL = %q, want the escaped name resolved", jobs[0].srcURL)
	}
	if strings.Contains(jobs[0].srcURL, `\`) {
		t.Errorf("font src URL kept a backslash: %q", jobs[0].srcURL)
	}
	if jobs[0].family != "Esc" {
		t.Errorf("font family = %q, want Esc", jobs[0].family)
	}
}

// TestImportRefDecodesEscapes is the per-site oracle for importRef, including
// the leading-string form, which never passes through ScanURL and decodes
// through unquote instead.
// qc:BR-269
func TestImportRefDecodesEscapes(t *testing.T) {
	cases := []struct {
		name   string
		stmt   string
		want   string
		wantOK bool
	}{
		{"hex escape with terminator", `@import url("b\42 .css");`, "bB.css", true},
		{"hex escape with no terminator", `@import url("su\62.css");`, "sub.css", true},
		{"hex escape, unquoted url", `@import url(su\62.css);`, "sub.css", true},
		{"hex escape, single-quoted url", `@import url('su\62.css');`, "sub.css", true},
		{"two escapes in one ref", `@import url("su\62\2e css");`, "sub.css", true},
		{"hex escape in the string form", `@import "b\42 .css";`, "bB.css", true},
		{"escaped quote", `@import url("a\"b.css");`, `a"b.css`, true},
		{"escaped paren, unquoted", `@import url(a\)b.css);`, "a)b.css", true},
		{"screen condition after an escape", `@import url("b\42 .css") screen;`, "bB.css", true},
		{"print condition still rejected", `@import url("b\42 .css") print;`, "", false},
		{"unterminated fails closed", `@import url("b\42`, "", false},
		{"plain ref is untouched", `@import url("theme.css");`, "theme.css", true},
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

// TestAbsolutizeCSSURLsResolvesEscapedRefs is the one place that must do both
// halves at once: resolve the URL the escapes denote, and re-emit sheet text
// that a second pass reads back identically. Each row asserts the round trip on
// the output token rather than on the output bytes, because the bytes are the
// same URL either way and only the rescan can show an emission that cuts early.
// Chromium's own resolution of these payloads is the want column (measured
// 2026-11-12: /icon.png, /no-such.png, /a b.png for the same three forms).
// qc:BR-269
func TestAbsolutizeCSSURLsResolvesEscapedRefs(t *testing.T) {
	const sheetURL = "https://e/x/s.css"
	cases := []struct{ name, in, want string }{
		{"hex escape with terminator", `.a{background:url("i\63 on.png")}`, "https://e/x/icon.png"},
		{"hex escape without terminator", `.a{background:url("ic\6fn.png")}`, "https://e/x/icon.png"},
		{"escaped hyphen, unquoted", `.a{background:url(no\2dsuch.png)}`, "https://e/x/no-such.png"},
		{"escaped quote", `.a{background:url("br\"dr.png")}`, `https://e/x/br%22dr.png`},
		{"escaped space", `.a{background:url("a\ b.png")}`, "https://e/x/a%20b.png"},
		{"escaped paren, unquoted", `.a{background:url(a\)b.png)}`, `https://e/x/a)b.png`},
		{"plain relative ref", `.a{background:url(icon.png)}`, "https://e/x/icon.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := absolutizeCSSURLs(tc.in, sheetURL)
			tok, ok := css.ScanURL(out)
			if !ok {
				t.Fatalf("rewritten sheet does not carry a readable url() token:\n%s", out)
			}
			if got := css.URLPayload(tok); got != tc.want {
				t.Errorf("absolutizeCSSURLs(%q) =\n  %q\nrescan decodes to %q, want %q", tc.in, out, got, tc.want)
			}
			if strings.Contains(css.URLPayload(tok), `\`) {
				t.Errorf("resolved URL still carries an escape: %q", tok.Payload)
			}
		})
	}
}

// TestAbsolutizeCSSURLsKeepsEscapedDataURIBytes pins the other half of the same
// function: a reference that is not rewritten is re-emitted exactly as written,
// escapes and all. Decoding here would turn `url("a\"b")` into `url("a"b")` and
// break the token the scanner had just matched, which is why Payload stays raw.
// qc:BR-269
func TestAbsolutizeCSSURLsKeepsEscapedDataURIBytes(t *testing.T) {
	const sheetURL = "https://e/x/s.css"
	cases := []string{
		`.a{background:url("data:image/svg+xml,<svg xmlns=\"http://www.w3.org/2000/svg\"><rect fill='rgb(0,0,255)'/></svg>")}`,
		`.a{background:url('data:font/woff2;base64,AAA\41=')}`,
		`.a{background:url(data\:image/png;base64,AAA)}`,
		`.a{background:url("#frag\41")}`,
	}
	for _, in := range cases {
		if got := absolutizeCSSURLs(in, sheetURL); got != in {
			t.Errorf("absolutizeCSSURLs rewrote a kept reference:\n got %q\nwant %q", got, in)
		}
		tok, ok := css.ScanURL(in)
		if !ok {
			t.Errorf("input is not a readable url() token: %q", in)
			continue
		}
		if !strings.Contains(in, tok.Payload) {
			t.Errorf("payload %q is not the sheet text it came from: %q", tok.Payload, in)
		}
	}
}

// TestExpandCSSImportsEscapedInnerRef is the end-to-end INT: a page sheet that
// @imports an inner sheet whose escaped relative reference must resolve against
// the INNER sheet's URL, the way Chromium fetched /icon.png from /sub.css
// rather than from the page. Both the import ref and the inner ref are escaped,
// so a single wrong layer would show up as a missing or misnamed fetch.
// qc:BR-269
func TestExpandCSSImportsEscapedInnerRef(t *testing.T) {
	const innerURL = "https://e/p/inner.css"
	var fetched []string
	linker := func(base, href string) (string, error) {
		fetched = append(fetched, href)
		if href == innerURL {
			return `.x{background:url("i\63 on.png")}`, nil
		}
		return "", errors.New("missing")
	}
	out := expandCSSImports(linker, "https://e/p/page.css", `@import url("in\6e er.css");`, map[string]bool{}, &subBudget{}, 1, nil)
	if len(fetched) != 1 || fetched[0] != innerURL {
		t.Fatalf("imported sheet URL = %q, want [%s] (the escaped ref must resolve to inner.css)", fetched, innerURL)
	}
	tok, ok := css.ScanURL(out)
	if !ok {
		t.Fatalf("spliced sheet carries no readable url() token:\n%s", out)
	}
	if got := css.URLPayload(tok); got != "https://e/p/icon.png" {
		t.Errorf("inner escaped reference resolved to %q, want https://e/p/icon.png:\n%s", got, out)
	}
}

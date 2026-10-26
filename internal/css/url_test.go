package css

import (
	"strings"
	"testing"
)

// TestScanURLPayload pins the payload each url() declaration form yields. These
// rows are the GOOSIE-CSSURLPAREN-001 discriminator: a ')' inside a quoted
// payload, balanced '(' ')/' pairs in an unquoted payload, and the malformed
// forms that must fail closed rather than emit half a URI. Each row maps to a
// behaviour rule in docs/tasks/TASK-050-CSS-URL-PAREN.md §3.
// qc:BR-211
func TestScanURLPayload(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
		wantQ  byte
	}{
		// Rule 1: quotes stripped, payload kept whole.
		{"quoted closing paren", `url("a)b")`, `a)b`, true, '"'},
		{"single-quoted closing paren", `url('a)b')`, `a)b`, true, '\''},
		{"quoted open paren", `url("a(b)c")`, `a(b)c`, true, '"'},
		{"quoted payload with parens semicolon comma", `url("data:image/svg+xml;utf8,<svg fill='rgb(0,0,255)'/></svg>")`, `data:image/svg+xml;utf8,<svg fill='rgb(0,0,255)'/></svg>`, true, '"'},
		// Rule 2: unquoted with balanced parens.
		{"unquoted balanced parens", `url(a(b)c)`, `a(b)c`, true, 0},
		{"unquoted nested balanced parens", `url(a(b(c))d)`, `a(b(c))d`, true, 0},
		{"unquoted data uri with rgb", `url(data:image/svg+xml,<svg fill='rgb(0,0,255)'/></svg>)`, `data:image/svg+xml,<svg fill='rgb(0,0,255)'/></svg>`, true, 0},
		// Rule 3: an escaped quote inside a quoted payload is a bad string.
		{"escaped quote fails closed", `url("a\"b")`, "", false, 0},
		{"escaped quote single fails closed", "url('a\\'b')", "", false, 0},
		// Rule 4: fail-closed recovery, never the rest of the declaration.
		{"unterminated quote fails closed", `url("abc`, "", false, 0},
		{"unterminated quote with trailing decl", `url("abc;color:red)`, "", false, 0},
		{"unterminated unquoted fails closed", `url(abc`, "", false, 0},
		// Rule 5: behaviour that must not change.
		{"payload free url", `url()`, "", true, 0},
		{"payload free with spaces", `url(   )`, "", true, 0},
		{"none keyword", `none`, "", false, 0},
		{"empty", ``, "", false, 0},
		{"no url function", `linear-gradient(red, blue)`, "", false, 0},
		{"first of many", `url(a) url(b)`, `a`, true, 0},
		{"case-insensitive function name", `URL(a.png)`, `a.png`, true, 0},
		{"case-insensitive mixed", `UrL("a)b.png")`, `a)b.png`, true, '"'},
		{"leading and trailing space quoted", `url( "a b" )`, `a b`, true, '"'},
		{"leading and trailing space unquoted", `url(  foo.png  )`, `foo.png`, true, 0},
		{"quoted then layer", `url("a)b.png") , red`, `a)b.png`, true, '"'},
		// The k0/k1 forms of §1 that painted before the fix and must keep painting.
		{"quoted pct-encoded hash", `url("data:image/svg+xml,<svg fill='%230000ff'/></svg>")`, `data:image/svg+xml,<svg fill='%230000ff'/></svg>`, true, '"'},
		{"unquoted pct-encoded hash", `url(data:image/svg+xml,<svg fill='%230000ff'/></svg>)`, `data:image/svg+xml,<svg fill='%230000ff'/></svg>`, true, 0},
		// Stray quotes inside a single-quoted payload: pre-existing leniency that
		// test/engine/vector_bg_test.go depends on. The payload is not a valid
		// §4.3.9 string, so the token is read unquoted and only the matched outer
		// pair is stripped.
		{
			"stray single quotes inside single-quoted payload",
			`url('data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' height='50%25'%3E%3Crect fill='lime'/%3E%3C/svg%3E')`,
			`data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' height='50%25'%3E%3Crect fill='lime'/%3E%3C/svg%3E`,
			true, '\'',
		},
		{
			"stray double quotes inside double-quoted payload with paren",
			`url("data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"%3E%3Crect fill="rgb(0,0,255)"/%3E%3C/svg%3E")`,
			`data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg"%3E%3Crect fill="rgb(0,0,255)"/%3E%3C/svg%3E`,
			true, '"',
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok, ok := ScanURL(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ScanURL(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if tok.Payload != tc.want {
				t.Errorf("ScanURL(%q) payload = %q, want %q", tc.in, tok.Payload, tc.want)
			}
			if ok && tok.Quote != tc.wantQ {
				t.Errorf("ScanURL(%q) quote = %q, want %q", tc.in, tok.Quote, tc.wantQ)
			}
		})
	}
}

// TestScanURLOffsets pins the token indices so callers that slice the original
// declaration (absolutizeCSSURLs re-emits the untouched payload; importRef takes
// the remainder after the url()) can do it without re-scanning.
// qc:BR-211
func TestScanURLOffsets(t *testing.T) {
	for _, in := range []string{
		`x: url("a)b") screen`,
		`x: url(a(b)c) screen`,
		`x: url('data:%3Csvg xmlns='x'/%3E') print`,
	} {
		tok, ok := ScanURL(in)
		if !ok {
			t.Fatalf("ScanURL(%q) reported no url()", in)
		}
		if in[tok.Start:tok.Start+4] != "url(" {
			t.Errorf("Start = %d, want the index of \"url(\"", tok.Start)
		}
		if tok.AfterOpen != tok.Start+4 {
			t.Errorf("AfterOpen = %d, want Start+4 = %d", tok.AfterOpen, tok.Start+4)
		}
		if in[tok.Close] != ')' {
			t.Errorf("Close = %d points at %q, want ')'", tok.Close, in[tok.Close])
		}
		if tok.End != tok.Close+1 {
			t.Errorf("End = %d, want Close+1 = %d", tok.End, tok.Close+1)
		}
		// The raw inner bytes must round-trip the payload (or its quoted form).
		raw := in[tok.AfterOpen:tok.Close]
		if !strings.Contains(raw, tok.Payload) {
			t.Errorf("raw inner %q does not contain payload %q", raw, tok.Payload)
		}
		if tok.Quote != 0 && raw != string(tok.Quote)+tok.Payload+string(tok.Quote) {
			t.Errorf("raw inner = %q, want quote+payload+quote", raw)
		}
		// Slicing at End yields the declaration text after the token.
		if !strings.HasPrefix(in[tok.End:], " ") {
			t.Errorf("remainder %q should start after the ')'", in[tok.End:])
		}
	}
}

// TestIndexURL pins the allocation-free occurrence search that
// absolutizeCSSURLs iterates with.
// qc:BR-211
func TestIndexURL(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{`url(a)`, 0},
		{`URL(a)`, 0},
		{`x: url(a)`, 3},
		{`no function here`, -1},
		{``, -1},
		{`ur`, -1},
		{`curl(a)`, 1},
	}
	for _, tc := range cases {
		if got := IndexURL(tc.in); got != tc.want {
			t.Errorf("IndexURL(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestScanURLBigPayload guards the cap rule of §3: the scanner adds no second
// size limit and returns a huge payload whole rather than truncating or
// growing work beyond a linear pass.
// qc:BR-211
func TestScanURLBigPayload(t *testing.T) {
	body := strings.Repeat("a", 200_000)
	tok, ok := ScanURL(`url("` + body + `")`)
	if !ok {
		t.Fatal("big quoted payload must parse")
	}
	if tok.Payload != body {
		t.Fatalf("big quoted payload truncated: got %d bytes, want %d", len(tok.Payload), len(body))
	}
}

// TestScanURLBalancedParensBigPayload checks a pathological unquoted value with
// many nested parens still terminates in one pass.
// qc:BR-211
func TestScanURLBalancedParensBigPayload(t *testing.T) {
	var b strings.Builder
	b.WriteString("url(")
	for i := 0; i < 10_000; i++ {
		b.WriteString("x(")
	}
	for i := 0; i < 10_000; i++ {
		b.WriteByte(')')
	}
	b.WriteString(")")
	in := b.String()
	tok, ok := ScanURL(in)
	if !ok {
		t.Fatal("deep balanced unquoted payload must parse")
	}
	if tok.End != len(in) {
		t.Errorf("End = %d, want %d", tok.End, len(in))
	}
}

func BenchmarkScanURLDataURI(b *testing.B) {
	in := `background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='100' height='20'%3E%3Crect width='100' height='20' fill='rgb(0,0,255)'/%3E%3C/svg%3E")`
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := ScanURL(in); !ok {
			b.Fatal("must parse")
		}
	}
}

// BenchmarkIndexURLSheetOccurrences is the cap-discipline guard for
// absolutizeCSSURLs' search: a 256 KiB sheet with a url() every 4 KiB must be
// scanned with zero allocations.
func BenchmarkIndexURLSheetOccurrences(b *testing.B) {
	var sb strings.Builder
	chunk := strings.Repeat("a", 4096)
	for i := 0; i < 64; i++ {
		sb.WriteString(chunk)
		sb.WriteString(`url(x.png)`)
	}
	sheet := sb.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		j := 0
		for k := 0; ; k++ {
			o := IndexURL(sheet[j:])
			if o < 0 {
				break
			}
			j += o + 4
		}
	}
}

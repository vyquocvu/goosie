package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// Three parses of `a`, with an unrelated stylesheet parsed in between, is the
// point: css.Parse must not carry anything from one stylesheet into the next.
// A single parse-twice comparison would only catch a parser that is unstable on
// its own input, which is a much weaker claim than the one this makes.
func FuzzParseStylesheet(f *testing.F) {
	f.Add(`p { color: red }`, `@media all{}`)
	f.Add(`@media (min-width: 1px){ .a > #b:not(:hover)::before { content: "\2014" } }`, `a{color:#fff}`)
	f.Add(`p{color:rgb(0,0,0;;`+strings.Repeat("@", 64)+`}}`, `b{}`)
	f.Add(`@font-face{font-family:"x";src:url(://`, `@font-face{src:local(a)}`)
	f.Add(`:is(:not(:matches(p))){}/*@`, `x{color:red`)
	f.Add(`@media{{{ @supports(())) [lang^=]{&{color:red}}}`, `p,{}:{`)
	f.Fuzz(func(t *testing.T, a, b string) {
		first := css.ParseForViewport(a, 1440)
		if first == nil {
			t.Fatal("ParseForViewport returned nil")
		}
		other := css.ParseForViewport(b, 900)
		if other == nil {
			t.Fatal("ParseForViewport returned nil for the unrelated sheet")
		}
		again := css.ParseForViewport(a, 1440)
		if got, want := len(again.Rules), len(first.Rules); got != want {
			t.Fatalf("parsing %q after %q produced %d rules, want %d", a, b, got, want)
		}
		for i := range first.Rules {
			if strings.Join(again.Rules[i].SelectorStrs, ",") != strings.Join(first.Rules[i].SelectorStrs, ",") {
				t.Fatalf("rule %d of %q changed after parsing %q: %v, want %v", i, a, b,
					again.Rules[i].SelectorStrs, first.Rules[i].SelectorStrs)
			}
			if len(again.Rules[i].Declarations) != len(first.Rules[i].Declarations) {
				t.Fatalf("rule %d of %q keeps %d declarations after an intervening parse, want %d",
					i, a, len(again.Rules[i].Declarations), len(first.Rules[i].Declarations))
			}
		}
		if len(again.FontFaces) != len(first.FontFaces) {
			t.Fatalf("%q yields %d @font-face rules after an intervening parse, want %d", a, len(again.FontFaces), len(first.FontFaces))
		}
	})
}

package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseBorderImageSourceURLParen is the per-site INT oracle for
// internal/css/border_image.go's borderImageURL (§2.3 row 2). border-image-source
// is parsed by the same second-pass url() reader shape that cut a payload at the
// first ')': strings.IndexByte(rest, ')') followed by a quote trim. A quoted
// payload containing ')' must reach BorderImage.Source whole, the
// parenthesis-free and unquoted forms must keep working, and the malformed forms
// must fail closed.
// qc:BR-212
func TestParseBorderImageSourceURLParen(t *testing.T) {
	const svgRGB = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='rgb(0,0,255)'/></svg>`
	const svgHash = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='%230000ff'/></svg>`
	// Stray single quotes inside a single-quoted payload: the lenient form the
	// engine accepted before the shared scanner landed, and the one
	// test/engine/vector_bg_test.go depends on for backgrounds.
	const svgStray = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'%3E%3Crect fill='rgb(0,0,255)'/%3E%3C/svg%3E`
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"double-quoted payload with ')'", `url("` + svgRGB + `")`, svgRGB},
		{"single-quoted payload with ')'", `url('` + svgRGB + `')`, svgRGB},
		{"unquoted payload with ')'", `url(` + svgRGB + `)`, svgRGB},
		{"unquoted payload with balanced '(' ')'", `url(a(b)c.png)`, "a(b)c.png"},
		{"quoted parenthesis-free hash form", `url("` + svgHash + `")`, svgHash},
		{"unquoted parenthesis-free hash form", `url(` + svgHash + `)`, svgHash},
		{"stray quotes inside single-quoted payload", `url('` + svgStray + `')`, svgStray},
		{"bare paren inside quotes", `url("a)b")`, "a)b"},
		{"simple quoted", `url("a.png")`, "a.png"},
		{"simple unquoted", `url(a.png)`, "a.png"},
		{"uppercase URL()", `URL("a)b.png")`, "a)b.png"},
		{"first of many", `url("a)b.png"), url(c.png)`, "a)b.png"},
		{"none", `none`, ""},
		{"gradient", `linear-gradient(red, blue)`, ""},
		{"empty url()", `url()`, ""},
		{"empty value", ``, ""},
		{"unterminated quote fails closed", `url("abc`, ""},
		{"escaped quote fails closed", `url("a\"b")`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := css.ParseBorderImageSource(tc.value); got != tc.want {
				t.Errorf("ParseBorderImageSource(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestBorderImageShorthandURLParen drives the border-image shorthand splitter,
// which splits on whitespace before it knows where the url() token ends. Both
// payload shapes matter: with the inner ')' glued to the url close, and with a
// space after it — the second one used to end the source at the interior ')'
// and pour the rest of the URI into the slice section.
// qc:BR-212
func TestBorderImageShorthandURLParen(t *testing.T) {
	cases := []struct{ name, svg string }{
		{"inner ')' glued to the url close", `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='rgb(0,0,255)'/></svg>`},
		{"space after the inner ')'", `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='rgb(0,0,255)' /></svg>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, slice, _, _, _, ok := css.ParseBorderImageShorthand(`url("` + tc.svg + `") 7 fill`)
			if !ok {
				t.Fatal("border-image shorthand with a ')' payload must parse")
			}
			if got := css.ParseBorderImageSource(source); got != tc.svg {
				t.Errorf("border-image shorthand Source = %q, want the whole payload", got)
			}
			if slice != "7 fill" {
				t.Errorf("border-image slice section after a ')' payload = %q, want %q", slice, "7 fill")
			}
		})
	}
}

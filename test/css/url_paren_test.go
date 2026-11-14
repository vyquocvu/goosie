package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// qc:BR-212
func TestParseBorderImageSourceURLParen(t *testing.T) {
	const svgRGB = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='rgb(0,0,255)'/></svg>`
	const svgHash = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg'><rect fill='%230000ff'/></svg>`
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
		{"escaped quote decodes", `url("a\"b")`, `a"b`},
		{"escaped paren in an unquoted token decodes", `url(a\)b.png)`, "a)b.png"},
		{"hex escape decodes", `url("a\41 b.png")`, "aAb.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := css.ParseBorderImageSource(tc.value); got != tc.want {
				t.Errorf("ParseBorderImageSource(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

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

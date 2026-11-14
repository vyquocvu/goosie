package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// qc:BR-212
func TestBackgroundImageURLParenDataImage(t *testing.T) {
	const svgHash = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><rect width='20' height='20' fill='%230000ff'/></svg>`
	const svgRGB = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><rect width='20' height='20' fill='rgb(0,0,255)'/></svg>`
	const svgParen = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><text x='2' y='14'>(100%)</text></svg>`
	const svgStray = `data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='20'%3E%3Crect width='20' height='20' fill='rgb(0,0,255)'/%3E%3C/svg%3E`

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"k0 quoted, parenthesis-free", `url("` + svgHash + `")`, svgHash},
		{"k1 unquoted, parenthesis-free", `url(` + svgHash + `)`, svgHash},
		{"k2 quoted, payload has ')'", `url("` + svgRGB + `")`, svgRGB},
		{"k3 unquoted, payload has ')'", `url(` + svgRGB + `)`, svgRGB},
		{"k4 quoted, payload has '('", `url("` + svgParen + `")`, svgParen},
		{"k5 single-quoted, payload has ')'", `url('` + svgRGB + `')`, svgRGB},
		{"k6 stray quotes inside a single-quoted payload", `url('` + svgStray + `')`, svgStray},
		{"k7 uppercase URL()", `URL("` + svgRGB + `")`, svgRGB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sheet := `.x { background-image: ` + tc.value + ` }`
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
			if s.BackgroundImage != tc.want {
				t.Errorf("BackgroundImage =\n  %q\nwant\n  %q", s.BackgroundImage, tc.want)
			}
		})
	}
}

// qc:BR-212
func TestBackgroundShorthandURLParen(t *testing.T) {
	const svgRGB = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><rect width='20' height='20' fill='rgb(0,0,255)'/></svg>`
	sheet := `.x { background: url("` + svgRGB + `") no-repeat center / contain rgb(1,2,3) }`
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
	if s.BackgroundImage != svgRGB {
		t.Errorf("BackgroundImage = %q, want the whole payload", s.BackgroundImage)
	}
	if s.BackgroundRepeat != style.BgRepeatNoRepeat || s.BackgroundRepeatY != style.BgRepeatNoRepeat {
		t.Errorf("repeat after a ')' payload = %v/%v, want no-repeat on both axes", s.BackgroundRepeat, s.BackgroundRepeatY)
	}
	if got := s.BackgroundColor; got.R != 1 || got.G != 2 || got.B != 3 {
		t.Errorf("color after a ')' payload = %+v, want rgb(1,2,3)", got)
	}
}

// qc:BR-212
func TestMaskImageURLParen(t *testing.T) {
	const svgRGB = `data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' width='20' height='20'><rect width='20' height='20' fill='rgb(0,0,255)'/></svg>`
	cases := []struct{ name, prop, want string }{
		{"mask-image", "mask-image", svgRGB},
		{"-webkit-mask-image", "-webkit-mask-image", svgRGB},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sheet := `.x { ` + tc.prop + `: url("` + tc.want + `") }`
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
			if s.MaskImage != tc.want {
				t.Errorf("MaskImage = %q, want the whole payload", s.MaskImage)
			}
		})
	}
}

// qc:BR-212
func TestBackgroundImageURLParenRegression(t *testing.T) {
	cases := []struct{ name, value, want string }{
		{"empty url()", `url()`, ""},
		{"none", `none`, ""},
		{"empty value", ``, ""},
		{"first of many layers", `url(first.png), url(second.png)`, "first.png"},
		{"first of many with paren payload", `url("a)b.png"), url(c.png)`, "a)b.png"},
		{"gradient instead of url", `linear-gradient(red, blue)`, ""},
		{"plain relative url", `url(invU.png)`, "invU.png"},
		{"quoted relative url", `url("invU.png")`, "invU.png"},
		{"unterminated url fails closed", `url("abc`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sheet := `.x { background-image: ` + tc.value + ` }`
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
			if s.BackgroundImage != tc.want {
				t.Errorf("BackgroundImage = %q, want %q", s.BackgroundImage, tc.want)
			}
		})
	}
}

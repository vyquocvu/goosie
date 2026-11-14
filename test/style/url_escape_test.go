package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

const svgEscapedQuote = `data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="rgb(0,0,255)"/></svg>`

// qc:BR-269
func TestBackgroundImageURLDecodesEscapes(t *testing.T) {
	cases := []struct{ name, value, want string }{
		{"escaped double quote", `url("a\"b.png")`, `a"b.png`},
		{"escaped single quote", `url('a\'b.png')`, `a'b.png`},
		{"hex escape", `url("a\41 b.png")`, "aAb.png"},
		{"doubled backslash", `url("a\\b.png")`, `a\b.png`},
		{"escaped space", `url("a\ b.png")`, "a b.png"},
		{"escaped paren, unquoted", `url(a\)b.png)`, "a)b.png"},
		{"no escape is untouched", `url("ab.png")`, "ab.png"},
		{"unterminated fails closed", `url("a\`, ""},
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

// qc:BR-269
func TestBackgroundImageEscapedQuoteSVGDataURI(t *testing.T) {
	cases := []struct{ name, value string }{
		{"double-quoted payload, escaped double quotes", `url("` + replacePlainWithEscaped(svgEscapedQuote, `"`, `\"`) + `")`},
		{"single-quoted payload, escaped single quotes", `url('` + replacePlainWithEscaped(svgEscapedQuote, `'`, `\'`) + `')`},
		{"double-quoted payload, hex-escaped quotes", `url("` + replacePlainWithEscaped(svgEscapedQuote, `"`, `\22 `) + `")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sheet := `.x { background-image: ` + tc.value + ` }`
			s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
			if s.BackgroundImage != svgEscapedQuote {
				t.Errorf("BackgroundImage =\n  %q\nwant the URI it denotes\n  %q", s.BackgroundImage, svgEscapedQuote)
			}
		})
	}
}

// qc:BR-269
func TestMaskImageURLDecodesEscapes(t *testing.T) {
	for _, prop := range []string{"mask-image", "-webkit-mask-image"} {
		sheet := `.x { ` + prop + `: url("m\41 sk.png") }`
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
		if s.MaskImage != "mAsk.png" {
			t.Errorf("%s: MaskImage = %q, want mAsk.png - the hex escape denotes that URL", prop, s.MaskImage)
		}
	}
}

// qc:BR-269
func TestBackgroundShorthandWithEscapedURL(t *testing.T) {
	sheet := `.x { background: url("a\")b.png") no-repeat center / contain rgb(1,2,3) }`
	s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
	if s.BackgroundImage != `a")b.png` {
		t.Errorf("BackgroundImage = %q, want a\")b.png", s.BackgroundImage)
	}
	if s.BackgroundRepeat != style.BgRepeatNoRepeat || s.BackgroundRepeatY != style.BgRepeatNoRepeat {
		t.Errorf("repeat after an escaped-paren payload = %v/%v, want no-repeat on both axes", s.BackgroundRepeat, s.BackgroundRepeatY)
	}
	if got := s.BackgroundColor; got.R != 1 || got.G != 2 || got.B != 3 {
		t.Errorf("color after an escaped-paren payload = %+v, want rgb(1,2,3)", got)
	}
}

func replacePlainWithEscaped(url, plain, esc string) string {
	out := make([]byte, 0, len(url)+8)
	for i := 0; i < len(url); i++ {
		if url[i] == plain[0] {
			out = append(out, esc...)
			continue
		}
		out = append(out, url[i])
	}
	return string(out)
}

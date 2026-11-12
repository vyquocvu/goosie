package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// svgEscapedQuote is a background whose data URI quotes its own SVG attributes
// with `\"` inside a double-quoted url() - the shape GOOSIE-CSSURLESCAPE-001
// dropped whole, and the shape an inline SVG written by hand in a stylesheet
// actually takes. The URL it denotes has plain quotes, so the cascade must store
// the plain form for the image loader to fetch.
const svgEscapedQuote = `data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="rgb(0,0,255)"/></svg>`

// TestBackgroundImageURLDecodesEscapes reads escapes back out of the computed
// value: the declaration text carries `\"`, `\'`, `\41`, `\\` and `\ `, and the
// URL the cascade hands on is the one those escapes denote (CSS Syntax L3 4.3.9,
// every row measured against Chromium's getComputedStyle().backgroundImage on
// 2026-11-12). Before this the escaped-quote rows produced no background at all -
// the scanner failed the token closed on the backslash - so the element painted
// its background-colour where every other browser paints the image.
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

// TestBackgroundImageEscapedQuoteSVGDataURI is the defect's own document, one
// layer up from a token: an SVG data URI that quotes its attributes with `\"`,
// with `\'` inside a single-quoted url(), and with `\22 ` (the hex form, whose
// terminator space is what keeps it from swallowing the next digits). All three
// denote the same URL, and the image loader must receive it whole - the exact
// spelling never reaches the network or the decoder, so a payload that survives
// with its backslashes is a broken URI, not a picky detail.
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

// TestMaskImageURLDecodesEscapes covers the second consumer of the same reader:
// mask-image stores its URL in Style.MaskImage and has to decode too, or a mask
// written with escapes silently masks nothing.
// qc:BR-269
func TestMaskImageURLDecodesEscapes(t *testing.T) {
	for _, prop := range []string{"mask-image", "-webkit-mask-image"} {
		sheet := `.x { ` + prop + `: url("m\41 sk.png") }`
		s := styleFor(t, `<html><body><div class="x">x</div></body></html>`, sheet, "div")
		// `\41` is "A" and the space after it is the escape's terminator, so it
		// is consumed rather than kept as a literal gap (Syntax L3 4.3.8).
		if s.MaskImage != "mAsk.png" {
			t.Errorf("%s: MaskImage = %q, want mAsk.png - the hex escape denotes that URL", prop, s.MaskImage)
		}
	}
}

// TestBackgroundShorthandWithEscapedURL keeps the splitter honest: the escapes
// belong to the url() token, and the keywords and colour after it still land in
// their own fields instead of being read out of a mis-cut payload.
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

// replacePlainWithEscaped writes every byte of `plain` in the URL as the CSS
// escape `esc`, so a fixture can be built from the URL it is meant to denote.
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

package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

const svgPlain = `data:image/svg+xml,<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"><rect width="20" height="20" fill="rgb(0,0,255)"/></svg>`

var (
	svgEscapedDQ = `url("` + escAll(svgPlain, `"`, `\"`) + `")`
	svgEscapedSQ = `url('` + escAll(strings.ReplaceAll(svgPlain, `"`, `'`), `'`, `\'`) + `')`
	svgHexQuote  = `url("` + escAll(svgPlain, `"`, `\22 `) + `")`
)

func escAll(s, from, to string) string {
	return strings.ReplaceAll(s, from, to)
}

// qc:BR-269
func TestBorderImageSourceURLDecodesEscapes(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"escaped double quote", `url("a\"b.png")`, `a"b.png`},
		{"escaped single quote", `url('a\'b.png')`, `a'b.png`},
		{"hex escape with its terminator", `url("a\41 b.png")`, "aAb.png"},
		{"greedy hex escape", `url("a\2240.png")`, "a\u2240.png"},
		{"escaped space", `url("a\ b.png")`, "a b.png"},
		{"doubled backslash", `url("a\\b.png")`, `a\b.png`},
		{"escaped paren, unquoted", `url(a\)b.png)`, "a)b.png"},
		{"balanced parens, unquoted", `url(a(b)c.png)`, "a(b)c.png"},
		{"SVG with escaped double quotes", svgEscapedDQ, svgPlain},
		{"SVG with escaped single quotes", svgEscapedSQ, strings.ReplaceAll(svgPlain, `"`, `'`)},
		{"SVG with hex-escaped quotes", svgHexQuote, svgPlain},
		{"double-quoted SVG holding plain single quotes", `url("` + strings.ReplaceAll(svgPlain, `"`, `'`) + `")`, strings.ReplaceAll(svgPlain, `"`, `'`)},
		{"no escape is untouched", `url("a.png")`, "a.png"},
		{"unterminated fails closed", `url("a\`, ""},
		{"empty payload", `url()`, ""},
		{"none keyword", `none`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := css.ParseBorderImageSource(tc.value); got != tc.want {
				t.Errorf("ParseBorderImageSource(%q) =\n  %q\nwant\n  %q", tc.value, got, tc.want)
			}
		})
	}
}

// qc:BR-269
func TestBorderImageShorthandKeepsEscapedSource(t *testing.T) {
	cases := []struct {
		name       string
		value      string
		wantSource string
		wantSlice  string
	}{
		{"escaped quote in the payload", `url("a\"b.png") 7 fill`, `a"b.png`, "7 fill"},
		{"escaped paren in an unquoted payload", `url(a\)b.png) 7 fill`, "a)b.png", "7 fill"},
		{"hex escape in the payload", `url("a\41 b.png") 3 4`, "aAb.png", "3 4"},
		{"SVG with escaped double quotes", svgEscapedDQ + ` 7 fill`, svgPlain, "7 fill"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, slice, _, _, _, ok := css.ParseBorderImageShorthand(tc.value)
			if !ok {
				t.Fatalf("ParseBorderImageShorthand(%q) did not parse", tc.value)
			}
			if got := css.ParseBorderImageSource(source); got != tc.wantSource {
				t.Errorf("source section %q decodes to\n  %q\nwant\n  %q", source, got, tc.wantSource)
			}
			if slice != tc.wantSlice {
				t.Errorf("slice section = %q, want %q", slice, tc.wantSlice)
			}
		})
	}
}

// qc:BR-269
func TestBorderImageEscapedSVGReachesTheParsedStruct(t *testing.T) {
	sourceText, sliceText, _, _, _, ok := css.ParseBorderImageShorthand(svgEscapedDQ + ` 7 fill stretch`)
	if !ok {
		t.Fatal("border-image with an escaped-quote SVG source must parse")
	}
	var bi css.BorderImage
	bi.Source = css.ParseBorderImageSource(sourceText)
	s, ok := css.ParseBorderImageSlice(sliceText)
	if !ok {
		t.Fatalf("slice section %q did not parse", sliceText)
	}
	bi.Slice = s.Slice
	bi.Fill = s.Fill
	bi.HasSlice = s.HasSlice
	if bi.Source != svgPlain {
		t.Errorf("BorderImage.Source =\n  %q\nwant\n  %q", bi.Source, svgPlain)
	}
	if strings.Contains(bi.Source, `\`) {
		t.Errorf("BorderImage.Source kept raw escapes: %q", bi.Source)
	}
	if !bi.HasSlice || !bi.Fill {
		t.Errorf("slice/fill after an escaped payload = HasSlice %v, Fill %v; want both set", bi.HasSlice, bi.Fill)
	}
	if bi.Slice != [4]float32{7, 7, 7, 7} {
		t.Errorf("slice after an escaped payload = %v, want 7 on every edge", bi.Slice)
	}
}

package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// CSS Color 4 lets rgb()/hsl() take an alpha and be written space-separated
// (`rgb(0 255 0 / 50%)`). Only the comma form parsed, and a value that fails
// ParseColor discards the whole declaration - so a space-separated
// `background: rgb(0 0 255 / 50%)` painted nothing while its text color still
// applied, which reads as a blank band rather than a broken color.
func TestParseColorColor4Syntax(t *testing.T) {
	tests := []struct {
		in string
		r  uint8
		g  uint8
		b  uint8
		a  uint8
	}{
		{"rgb(255, 0, 0)", 255, 0, 0, 255},
		{"rgb(255 0 0)", 255, 0, 0, 255},
		{"RGB(0 255 0)", 0, 255, 0, 255},
		{"rgb(0% 50% 100%)", 0, 128, 255, 255},
		{"rgb(0 0 255 / 50%)", 0, 0, 255, 128},
		{"rgb(0 0 255 / .25)", 0, 0, 255, 64},
		{"rgba(1, 2, 3, 0.5)", 1, 2, 3, 128},
		{"hsl(30, 100%, 50%)", 255, 128, 0, 255},
		{"hsl(30 100% 50%)", 255, 128, 0, 255},
		{"hsl(200deg 100% 50%)", 0, 170, 255, 255},
		{"hsl(0.5turn 100% 50%)", 0, 255, 255, 255},
		{"hsl(200 100% 50% / .5)", 0, 170, 255, 128},
		{"hsla(120, 100%, 50%, .25)", 0, 255, 0, 64},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := css.ParseColor(tc.in)
			if !ok {
				t.Fatalf("ParseColor(%q) rejected a valid color", tc.in)
			}
			if got != (css.Color{R: tc.r, G: tc.g, B: tc.b, A: tc.a}) {
				t.Errorf("ParseColor(%q) = %+v, want {%d %d %d %d}", tc.in, got, tc.r, tc.g, tc.b, tc.a)
			}
		})
	}
}

// Malformed argument lists must stay unparseable: a permissive color parser
// would turn an invalid declaration into an arbitrary painted color.
func TestParseColorRejectsMalformedFunctions(t *testing.T) {
	for _, in := range []string{
		"rgb(255 0)",
		"rgb(255, 0, 0, 0, 0)",
		"rgb(255 0 0 /)",
		"rgb(255 0 0 / 50% 50%)",
		"hsl(200 100%)",
		"hsl(200deg)",
		"rgb(255 0 0",
		"notacolor(1 2 3)",
	} {
		if got, ok := css.ParseColor(in); ok {
			t.Errorf("ParseColor(%q) = %+v, want rejected", in, got)
		}
	}
}

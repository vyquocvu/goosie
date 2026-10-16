package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestColor4SpacesResolveToSRGB pins the one-way CSS Color 4 conversions:
// every predefined space answers the sRGB it displays as instead of dropping
// the declaration (which fell back to the previous value, usually red, and
// cost the WPT css-color directory most of its failures).
func TestColor4SpacesResolveToSRGB(t *testing.T) {
	for _, c := range []struct {
		src     string
		r, g, b uint8
	}{
		// The WPT xyz swatch: green #008000 converted to xyz-d65.
		{"color(xyz-d65 0.07719 0.15438 0.02573)", 0, 128, 0},
		{"hwb(120 0% 0%)", 0, 255, 0},
		{"hwb(0 100% 0%)", 255, 255, 255},
		{"color(display-p3 1 0 0)", 255, 0, 0},
		{"color(srgb 1 0 0)", 255, 0, 0},
		{"color(srgb-linear 1 0 0)", 255, 0, 0},
		{"lab(100% 0 0)", 255, 255, 255},
		{"lab(0% 0 0)", 0, 0, 0},
		{"oklab(1 0 0)", 255, 255, 255},
		{"oklab(0 0 0)", 0, 0, 0},
	} {
		got, ok := css.ParseColor(c.src)
		if !ok {
			t.Errorf("ParseColor(%q) reported itself unsupported", c.src)
			continue
		}
		if got.R != c.r || got.G != c.g || got.B != c.b || got.A != 255 {
			t.Errorf("ParseColor(%q) = %+v, want {%d %d %d 255}", c.src, got, c.r, c.g, c.b)
		}
	}
}

// TestColor4AlphaForm pins the slash-alpha form shared by all the spaces.
func TestColor4AlphaForm(t *testing.T) {
	got, ok := css.ParseColor("lab(50% 40 59.5 / 50%)")
	if !ok {
		t.Fatal("ParseColor(lab with alpha) reported itself unsupported")
	}
	if got.A != 128 {
		t.Errorf("alpha = %d, want 128", got.A)
	}
}

// TestColor4UnknownSpaceDrops pins the boundary: an unknown colorspace keeps
// the old behavior (unsupported, declaration drops) instead of guessing.
func TestColor4UnknownSpaceDrops(t *testing.T) {
	for _, src := range []string{
		"color(bogus-space 1 0 0)",
		"color(display-p3 1 0)",
		"color-mix(in srgb, red, blue)",
	} {
		if _, ok := css.ParseColor(src); ok {
			t.Errorf("ParseColor(%q) succeeded, want unsupported", src)
		}
	}
}

// TestColorCommentsAsSeparators pins comment handling inside values: a
// comment separates tokens rather than fusing them, so `120/*c*/75%` reads
// as 120 and 75%, not 12075%. The old strip dropped the comment outright
// and every commented swatch lost its background.
func TestColorCommentsAsSeparators(t *testing.T) {
	for _, src := range []string{
		"hsla(120/* comment */75%/* comment */50%/1.0)",
		"hsl(120/* comment */75%/* comment */50%)",
	} {
		got, ok := css.ParseColor(src)
		if !ok {
			t.Errorf("ParseColor(%q) reported itself unsupported", src)
			continue
		}
		if got.R != 32 || got.G != 223 || got.B != 32 || got.A != 255 {
			t.Errorf("ParseColor(%q) = %+v, want {32 223 32 255}", src, got)
		}
	}
}

// TestDisplayP3Linear pins the linear-light P3 space: same primaries as
// display-p3, no transfer curve on the way in.
func TestDisplayP3Linear(t *testing.T) {
	got, ok := css.ParseColor("color(display-p3-linear 1 0 0)")
	if !ok {
		t.Fatal("ParseColor(display-p3-linear) reported itself unsupported")
	}
	if got.R != 255 || got.G != 0 || got.B != 0 {
		t.Errorf("ParseColor(display-p3-linear red) = %+v, want {255 0 0 255}", got)
	}
}

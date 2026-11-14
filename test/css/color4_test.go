package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestColor4SpacesResolveToSRGB(t *testing.T) {
	for _, c := range []struct {
		src     string
		r, g, b uint8
	}{
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
		{"oklab(51.975% -35.075% 26.92%)", 0, 128, 0},
		{"oklch(51.975% 44.215% 142.495)", 0, 128, 0},
		{"lch(100% 110 60)", 255, 255, 255},
		{"lch(0% 110 60)", 0, 0, 0},
		{"oklch(100% 110 60)", 255, 255, 255},
		{"oklch(0% 1.1 60)", 0, 0, 0},
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

func TestColor4AlphaForm(t *testing.T) {
	got, ok := css.ParseColor("lab(50% 40 59.5 / 50%)")
	if !ok {
		t.Fatal("ParseColor(lab with alpha) reported itself unsupported")
	}
	if got.A != 128 {
		t.Errorf("alpha = %d, want 128", got.A)
	}
}

func TestColor4UnknownSpaceDrops(t *testing.T) {
	for _, src := range []string{
		"color(bogus-space 1 0 0)",
		"color(display-p3 1 0)",
		"color-contrast(red vs white)",
	} {
		if _, ok := css.ParseColor(src); ok {
			t.Errorf("ParseColor(%q) succeeded, want unsupported", src)
		}
	}
}

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

func TestDisplayP3Linear(t *testing.T) {
	got, ok := css.ParseColor("color(display-p3-linear 1 0 0)")
	if !ok {
		t.Fatal("ParseColor(display-p3-linear) reported itself unsupported")
	}
	if got.R != 255 || got.G != 0 || got.B != 0 {
		t.Errorf("ParseColor(display-p3-linear red) = %+v, want {255 0 0 255}", got)
	}
}

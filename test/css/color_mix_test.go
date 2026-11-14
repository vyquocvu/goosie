package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func closeByte(a, b uint8, maxDiff int) bool {
	diff := int(a) - int(b)
	if diff < 0 {
		diff = -diff
	}
	return diff <= maxDiff
}

func closeColor(c1, c2 css.Color, maxDiff int) bool {
	return closeByte(c1.R, c2.R, maxDiff) &&
		closeByte(c1.G, c2.G, maxDiff) &&
		closeByte(c1.B, c2.B, maxDiff) &&
		closeByte(c1.A, c2.A, maxDiff)
}

func TestColorMix_SRGB_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  css.Color
	}{
		{
			input: "color-mix(in srgb, red, blue)",
			want:  css.Color{R: 128, G: 0, B: 128, A: 255},
		},
		{
			input: "color-mix(in srgb, red 40%, blue)",
			want:  css.Color{R: 102, G: 0, B: 153, A: 255},
		},
		{
			input: "color-mix(in srgb, 40% red, blue)",
			want:  css.Color{R: 102, G: 0, B: 153, A: 255},
		},
		{
			input: "color-mix(in srgb, red 80%, blue 80%)",
			want:  css.Color{R: 128, G: 0, B: 128, A: 255},
		},
		{
			input: "color-mix(in srgb, red 20%, blue 30%)",
			want:  css.Color{R: 102, G: 0, B: 153, A: 128},
		},
		{
			input: "color-mix(in srgb, white, black)",
			want:  css.Color{R: 128, G: 128, B: 128, A: 255},
		},
	}

	for _, tt := range tests {
		got, ok := css.ParseColor(tt.input)
		if !ok {
			t.Fatalf("ParseColor(%q) failed", tt.input)
		}
		if !closeColor(got, tt.want, 2) {
			t.Errorf("ParseColor(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestColorMix_ColorSpaces(t *testing.T) {
	spaces := []string{
		"srgb-linear",
		"oklab",
		"oklch",
		"lab",
		"lch",
		"hsl",
		"hwb",
		"display-p3",
		"xyz",
		"xyz-d50",
	}

	for _, sp := range spaces {
		input := "color-mix(in " + sp + ", red, blue)"
		c, ok := css.ParseColor(input)
		if !ok {
			t.Errorf("failed to parse color-mix in space %q", sp)
			continue
		}
		if c.A != 255 {
			t.Errorf("space %q: expected alpha 255, got %d", sp, c.A)
		}
		if c.R == 0 && c.B == 0 {
			t.Errorf("space %q: expected non-zero color components, got %+v", sp, c)
		}
	}
}

func TestColorMix_HueInterpolationMethods(t *testing.T) {
	methods := []string{"shorter hue", "longer hue", "increasing hue", "decreasing hue"}

	for _, m := range methods {
		input := "color-mix(in oklch " + m + ", red, blue)"
		c, ok := css.ParseColor(input)
		if !ok {
			t.Errorf("failed to parse color-mix with hue method %q", m)
			continue
		}
		if c.A != 255 {
			t.Errorf("method %q: expected alpha 255, got %d", m, c.A)
		}
	}
}

func TestColorMix_TransparentHandling(t *testing.T) {
	input := "color-mix(in srgb, red, transparent)"
	c, ok := css.ParseColor(input)
	if !ok {
		t.Fatalf("ParseColor(%q) failed", input)
	}
	if c.R < 250 || c.G != 0 || c.B != 0 {
		t.Errorf("expected red component preserved, got R=%d G=%d B=%d", c.R, c.G, c.B)
	}
	if !closeByte(c.A, 128, 2) {
		t.Errorf("expected alpha ~128, got %d", c.A)
	}
}

func TestColorMix_Nested(t *testing.T) {
	input := "color-mix(in srgb, color-mix(in srgb, red, blue), green)"
	c, ok := css.ParseColor(input)
	if !ok {
		t.Fatalf("ParseColor(%q) failed", input)
	}
	if !closeByte(c.R, 64, 2) || !closeByte(c.G, 64, 2) || !closeByte(c.B, 64, 2) {
		t.Errorf("nested color-mix = %+v, want ~(64, 64, 64)", c)
	}
}

func TestColorMix_InvalidInputs(t *testing.T) {
	invalids := []string{
		"color-mix()",
		"color-mix(in srgb)",
		"color-mix(in srgb, red)",
		"color-mix(srgb, red, blue)",
		"color-mix(in unknownSpace, red, blue)",
		"color-mix(in srgb, invalidColor, blue)",
		"color-mix(in srgb, red 0%, blue 0%)",
	}

	for _, inv := range invalids {
		if c, ok := css.ParseColor(inv); ok {
			t.Errorf("expected invalid for %q, got %+v", inv, c)
		}
	}
}

var _ = math.Pi

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
			// 50% #ff0000 + 50% #0000ff = (128, 0, 128, 255)
			want: css.Color{R: 128, G: 0, B: 128, A: 255},
		},
		{
			input: "color-mix(in srgb, red 40%, blue)",
			// 40% #ff0000 + 60% #0000ff = (102, 0, 153, 255)
			want: css.Color{R: 102, G: 0, B: 153, A: 255},
		},
		{
			input: "color-mix(in srgb, 40% red, blue)",
			// Percentage prefix: 40% #ff0000 + 60% #0000ff
			want: css.Color{R: 102, G: 0, B: 153, A: 255},
		},
		{
			input: "color-mix(in srgb, red 80%, blue 80%)",
			// Both > 100%: 80/160 = 50%, 80/160 = 50%, alphaScale = 1.0
			want: css.Color{R: 128, G: 0, B: 128, A: 255},
		},
		{
			input: "color-mix(in srgb, red 20%, blue 30%)",
			// Both < 100%: 20/50 = 40%, 30/50 = 60%, alphaScale = 50/100 = 0.5
			// R = 102, G = 0, B = 153, A = 128
			want: css.Color{R: 102, G: 0, B: 153, A: 128},
		},
		{
			input: "color-mix(in srgb, white, black)",
			// 50% white + 50% black = (128, 128, 128, 255)
			want: css.Color{R: 128, G: 128, B: 128, A: 255},
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
	// srgb-linear, oklab, oklch, lab, lch, hsl, hwb, display-p3
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
	// In polar spaces (oklch, lch, hsl, hwb), test hue interpolation modifiers:
	// shorter hue (default), longer hue, increasing hue, decreasing hue.
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
	// 50% red + 50% transparent
	input := "color-mix(in srgb, red, transparent)"
	c, ok := css.ParseColor(input)
	if !ok {
		t.Fatalf("ParseColor(%q) failed", input)
	}
	// Red is preserved, alpha is halved (128)
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
	// First mix is purple (128, 0, 128), mixed 50/50 with green (0, 128, 0)
	// => (64, 64, 64)
	if !closeByte(c.R, 64, 2) || !closeByte(c.G, 64, 2) || !closeByte(c.B, 64, 2) {
		t.Errorf("nested color-mix = %+v, want ~(64, 64, 64)", c)
	}
}

func TestColorMix_InvalidInputs(t *testing.T) {
	invalids := []string{
		"color-mix()",
		"color-mix(in srgb)",
		"color-mix(in srgb, red)",
		"color-mix(srgb, red, blue)",             // missing "in"
		"color-mix(in unknownSpace, red, blue)",  // invalid space
		"color-mix(in srgb, invalidColor, blue)", // invalid color
		"color-mix(in srgb, red 0%, blue 0%)",    // sum 0
	}

	for _, inv := range invalids {
		if c, ok := css.ParseColor(inv); ok {
			t.Errorf("expected invalid for %q, got %+v", inv, c)
		}
	}
}

// Suppress unused math import warning if any
var _ = math.Pi

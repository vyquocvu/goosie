package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestRelativeColorSyntax_RGB(t *testing.T) {
	// Identity
	c, ok := css.ParseColor("rgb(from red r g b)")
	if !ok || c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Fatalf("expected pure red, got %+v (ok=%v)", c, ok)
	}

	// Channel replacement
	c, ok = css.ParseColor("rgb(from red 0 g b)")
	if !ok || c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Fatalf("expected black, got %+v (ok=%v)", c, ok)
	}

	// Alpha modification
	c, ok = css.ParseColor("rgb(from red r g b / 0.5)")
	if !ok || c.R != 255 || c.G != 0 || c.B != 0 || !closeByte(c.A, 128, 2) {
		t.Fatalf("expected red with 50%% alpha, got %+v (ok=%v)", c, ok)
	}

	// Arithmetic in calc()
	c, ok = css.ParseColor("rgb(from #ff0000 calc(r * 0.5) g b)")
	if !ok || !closeByte(c.R, 128, 2) || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Fatalf("expected half-red ~128, got %+v (ok=%v)", c, ok)
	}
}

func TestRelativeColorSyntax_HSL(t *testing.T) {
	// Original blue is hsl(240, 100%, 50%)
	// Modify lightness to 30%
	c, ok := css.ParseColor("hsl(from blue h s 30%)")
	if !ok {
		t.Fatalf("ParseColor failed: %v", ok)
	}
	// Dark blue: R=0, G=0, B ~ 153
	if c.R != 0 || c.G != 0 || !closeByte(c.B, 153, 5) {
		t.Fatalf("expected dark blue ~153, got %+v", c)
	}

	// Shift hue: 120deg green + 180 = 300deg (magenta)
	c, ok = css.ParseColor("hsl(from lime calc(h + 180) s l)")
	if !ok {
		t.Fatalf("ParseColor failed: %v", ok)
	}
	// Magenta has R ~ 255, B ~ 255, G ~ 0
	if c.R < 250 || c.B < 250 || c.G > 10 {
		t.Fatalf("expected magenta, got %+v", c)
	}
}

func TestRelativeColorSyntax_Oklch(t *testing.T) {
	// Lightness adjustment in Oklch: red l * 0.5
	c, ok := css.ParseColor("oklch(from red calc(l * 0.5) c h)")
	if !ok {
		t.Fatalf("ParseColor failed: %v", ok)
	}
	// Dark red: R is smaller than 255, G and B near 0
	if c.R == 0 || c.R > 200 || c.G > 10 || c.B > 10 {
		t.Fatalf("expected dark red, got %+v", c)
	}

	// Alpha calc
	c, ok = css.ParseColor("oklch(from red l c h / calc(alpha * 0.5))")
	if !ok || c.R < 250 || !closeByte(c.A, 128, 2) {
		t.Fatalf("expected red with 50%% alpha, got %+v", c)
	}
}

func TestRelativeColorSyntax_OtherSpaces(t *testing.T) {
	// oklab
	c, ok := css.ParseColor("oklab(from green calc(l * 1.1) a b)")
	if !ok || c.A != 255 {
		t.Fatalf("oklab RCS failed: %+v (ok=%v)", c, ok)
	}

	// lab
	c, ok = css.ParseColor("lab(from blue calc(l + 10) a b)")
	if !ok || c.A != 255 {
		t.Fatalf("lab RCS failed: %+v (ok=%v)", c, ok)
	}

	// lch
	c, ok = css.ParseColor("lch(from red l c calc(h + 90))")
	if !ok || c.A != 255 {
		t.Fatalf("lch RCS failed: %+v (ok=%v)", c, ok)
	}

	// hwb
	c, ok = css.ParseColor("hwb(from red h 20% 10%)")
	if !ok || c.A != 255 {
		t.Fatalf("hwb RCS failed: %+v (ok=%v)", c, ok)
	}
}

func TestLightDark_Function(t *testing.T) {
	// Default (light) scheme
	c, ok := css.ParseColor("light-dark(white, black)")
	if !ok || c.R != 255 || c.G != 255 || c.B != 255 {
		t.Fatalf("expected white for default light-dark, got %+v", c)
	}

	// Explicit dark scheme
	cDark, ok := css.ParseColorWithScheme("light-dark(white, black)", "dark")
	if !ok || cDark.R != 0 || cDark.G != 0 || cDark.B != 0 {
		t.Fatalf("expected black for dark scheme light-dark, got %+v", cDark)
	}

	// Explicit light scheme
	cLight, ok := css.ParseColorWithScheme("light-dark(white, black)", "light")
	if !ok || cLight.R != 255 || cLight.G != 255 || cLight.B != 255 {
		t.Fatalf("expected white for light scheme light-dark, got %+v", cLight)
	}
}

package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// TestParseBoxShadowBasic verifies that a simple box-shadow with offset, blur,
// spread, and color parses correctly.
func TestParseBoxShadowBasic(t *testing.T) {
	shadows := css.ParseBoxShadow("2px 4px 6px 2px rgba(0, 0, 0, 0.5)")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 2 {
		t.Errorf("OffsetX = %v, want 2", s.OffsetX)
	}
	if s.OffsetY != 4 {
		t.Errorf("OffsetY = %v, want 4", s.OffsetY)
	}
	if s.Blur != 6 {
		t.Errorf("Blur = %v, want 6", s.Blur)
	}
	if s.Spread != 2 {
		t.Errorf("Spread = %v, want 2", s.Spread)
	}
	if s.Color.R != 0 || s.Color.G != 0 || s.Color.B != 0 {
		t.Errorf("Color RGB = (%d,%d,%d), want (0,0,0)", s.Color.R, s.Color.G, s.Color.B)
	}
	if s.Color.A != 128 {
		t.Errorf("Color A = %d, want ~128", s.Color.A)
	}
	if s.Inset {
		t.Error("Inset should be false")
	}
}

// TestParseBoxShadowInset verifies the inset keyword is recognized.
func TestParseBoxShadowInset(t *testing.T) {
	shadows := css.ParseBoxShadow("inset 3px 3px 5px black")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	if !shadows[0].Inset {
		t.Error("expected Inset to be true")
	}
	if shadows[0].OffsetX != 3 {
		t.Errorf("OffsetX = %v, want 3", shadows[0].OffsetX)
	}
	if shadows[0].OffsetY != 3 {
		t.Errorf("OffsetY = %v, want 3", shadows[0].OffsetY)
	}
	if shadows[0].Blur != 5 {
		t.Errorf("Blur = %v, want 5", shadows[0].Blur)
	}
}

// TestParseBoxShadowNone verifies that "none" returns nil.
func TestParseBoxShadowNone(t *testing.T) {
	shadows := css.ParseBoxShadow("none")
	if shadows != nil {
		t.Errorf("expected nil for 'none', got %v", shadows)
	}
}

// TestParseBoxShadowEmpty verifies that empty string returns nil.
func TestParseBoxShadowEmpty(t *testing.T) {
	shadows := css.ParseBoxShadow("")
	if shadows != nil {
		t.Errorf("expected nil for empty string, got %v", shadows)
	}
}

// TestParseBoxShadowMultiple verifies comma-separated shadows parse correctly.
func TestParseBoxShadowMultiple(t *testing.T) {
	shadows := css.ParseBoxShadow("2px 2px 4px red, -1px -1px 3px blue")
	if len(shadows) != 2 {
		t.Fatalf("expected 2 shadows, got %d", len(shadows))
	}
	// First shadow.
	if shadows[0].OffsetX != 2 || shadows[0].OffsetY != 2 {
		t.Errorf("shadow[0] offset = (%v,%v), want (2,2)", shadows[0].OffsetX, shadows[0].OffsetY)
	}
	if shadows[0].Color.R != 255 || shadows[0].Color.G != 0 || shadows[0].Color.B != 0 {
		t.Errorf("shadow[0] color = (%d,%d,%d), want red", shadows[0].Color.R, shadows[0].Color.G, shadows[0].Color.B)
	}
	// Second shadow.
	if shadows[1].OffsetX != -1 || shadows[1].OffsetY != -1 {
		t.Errorf("shadow[1] offset = (%v,%v), want (-1,-1)", shadows[1].OffsetX, shadows[1].OffsetY)
	}
	if shadows[1].Color.R != 0 || shadows[1].Color.G != 0 || shadows[1].Color.B != 255 {
		t.Errorf("shadow[1] color = (%d,%d,%d), want blue", shadows[1].Color.R, shadows[1].Color.G, shadows[1].Color.B)
	}
}

// TestParseBoxShadowNoBlur verifies that blur and spread are optional.
func TestParseBoxShadowNoBlur(t *testing.T) {
	shadows := css.ParseBoxShadow("5px 10px green")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 5 {
		t.Errorf("OffsetX = %v, want 5", s.OffsetX)
	}
	if s.OffsetY != 10 {
		t.Errorf("OffsetY = %v, want 10", s.OffsetY)
	}
	if s.Blur != 0 {
		t.Errorf("Blur = %v, want 0", s.Blur)
	}
	if s.Spread != 0 {
		t.Errorf("Spread = %v, want 0", s.Spread)
	}
	if s.Color.R != 0 || s.Color.G != 128 || s.Color.B != 0 {
		t.Errorf("Color = (%d,%d,%d), want green", s.Color.R, s.Color.G, s.Color.B)
	}
}

// TestParseBoxShadowColorBeforeLengths verifies that the color can appear
// anywhere in the layer.
func TestParseBoxShadowColorBeforeLengths(t *testing.T) {
	shadows := css.ParseBoxShadow("red 4px 6px 8px")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 4 || s.OffsetY != 6 {
		t.Errorf("offset = (%v,%v), want (4,6)", s.OffsetX, s.OffsetY)
	}
	if s.Blur != 8 {
		t.Errorf("Blur = %v, want 8", s.Blur)
	}
	if s.Color.R != 255 || s.Color.G != 0 {
		t.Errorf("Color = (%d,%d,%d), want red", s.Color.R, s.Color.G, s.Color.B)
	}
}

// TestParseBoxShadowDefaultColor verifies that when no color is specified,
// the default is opaque black.
func TestParseBoxShadowDefaultColor(t *testing.T) {
	shadows := css.ParseBoxShadow("3px 3px 5px")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if !s.ColorIsCurrent {
		t.Errorf("omitted color should mark ColorIsCurrent, got %+v", s)
	}
}

// TestParseBoxShadowNegativeOffsets verifies negative offset values.
func TestParseBoxShadowNegativeOffsets(t *testing.T) {
	shadows := css.ParseBoxShadow("-3px -5px 10px #000")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != -3 {
		t.Errorf("OffsetX = %v, want -3", s.OffsetX)
	}
	if s.OffsetY != -5 {
		t.Errorf("OffsetY = %v, want -5", s.OffsetY)
	}
}

// TestParseBoxShadowInsufficientLengths verifies that a shadow with fewer
// than two length values is rejected.
func TestParseBoxShadowInsufficientLengths(t *testing.T) {
	shadows := css.ParseBoxShadow("5px red")
	if len(shadows) != 0 {
		t.Errorf("expected 0 shadows for single-length value, got %d", len(shadows))
	}
}

// TestParseBoxShadowWithRGBColor verifies parsing with rgb() color function.
func TestParseBoxShadowWithRGBColor(t *testing.T) {
	shadows := css.ParseBoxShadow("2px 2px 4px rgb(100, 200, 50)")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.Color.R != 100 || s.Color.G != 200 || s.Color.B != 50 {
		t.Errorf("Color = (%d,%d,%d), want (100,200,50)", s.Color.R, s.Color.G, s.Color.B)
	}
}

// TestParseTextShadowBasic verifies a simple text-shadow parses correctly.
func TestParseTextShadowBasic(t *testing.T) {
	shadows := css.ParseTextShadow("1px 2px 3px rgba(0,0,0,0.5)")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 1 {
		t.Errorf("OffsetX = %v, want 1", s.OffsetX)
	}
	if s.OffsetY != 2 {
		t.Errorf("OffsetY = %v, want 2", s.OffsetY)
	}
	if s.Blur != 3 {
		t.Errorf("Blur = %v, want 3", s.Blur)
	}
	if s.Color.A != 128 {
		t.Errorf("Color A = %d, want ~128", s.Color.A)
	}
}

// TestParseTextShadowNone verifies that "none" returns nil.
func TestParseTextShadowNone(t *testing.T) {
	shadows := css.ParseTextShadow("none")
	if shadows != nil {
		t.Errorf("expected nil for 'none', got %v", shadows)
	}
}

// TestParseTextShadowEmpty verifies that empty string returns nil.
func TestParseTextShadowEmpty(t *testing.T) {
	shadows := css.ParseTextShadow("")
	if shadows != nil {
		t.Errorf("expected nil for empty string, got %v", shadows)
	}
}

// TestParseTextShadowMultiple verifies comma-separated text shadows.
func TestParseTextShadowMultiple(t *testing.T) {
	shadows := css.ParseTextShadow("1px 1px 2px black, 0 0 1em blue")
	if len(shadows) != 2 {
		t.Fatalf("expected 2 shadows, got %d", len(shadows))
	}
	if shadows[0].OffsetX != 1 || shadows[0].OffsetY != 1 {
		t.Errorf("shadow[0] offset = (%v,%v), want (1,1)", shadows[0].OffsetX, shadows[0].OffsetY)
	}
	if shadows[1].OffsetX != 0 || shadows[1].OffsetY != 0 {
		t.Errorf("shadow[1] offset = (%v,%v), want (0,0)", shadows[1].OffsetX, shadows[1].OffsetY)
	}
}

// TestParseTextShadowNoBlur verifies that blur is optional for text-shadow.
func TestParseTextShadowNoBlur(t *testing.T) {
	shadows := css.ParseTextShadow("2px 2px red")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 2 || s.OffsetY != 2 {
		t.Errorf("offset = (%v,%v), want (2,2)", s.OffsetX, s.OffsetY)
	}
	if s.Blur != 0 {
		t.Errorf("Blur = %v, want 0", s.Blur)
	}
	if s.Color.R != 255 || s.Color.G != 0 || s.Color.B != 0 {
		t.Errorf("Color = (%d,%d,%d), want red", s.Color.R, s.Color.G, s.Color.B)
	}
}

// TestParseTextShadowDefaultColor verifies omitted color marks currentcolor.
func TestParseTextShadowDefaultColor(t *testing.T) {
	shadows := css.ParseTextShadow("1px 1px")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if !s.ColorIsCurrent {
		t.Errorf("omitted color should mark ColorIsCurrent, got %+v", s)
	}
}

// TestParseTextShadowInsufficientLengths verifies that a text-shadow with
// fewer than two length values is rejected.
func TestParseTextShadowInsufficientLengths(t *testing.T) {
	shadows := css.ParseTextShadow("5px red")
	if len(shadows) != 0 {
		t.Errorf("expected 0 shadows for single-length value, got %d", len(shadows))
	}
}

// TestParseTextShadowColorFirst verifies that color can come before lengths.
func TestParseTextShadowColorFirst(t *testing.T) {
	shadows := css.ParseTextShadow("blue 2px 3px 1px")
	if len(shadows) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(shadows))
	}
	s := shadows[0]
	if s.OffsetX != 2 || s.OffsetY != 3 {
		t.Errorf("offset = (%v,%v), want (2,3)", s.OffsetX, s.OffsetY)
	}
	if s.Blur != 1 {
		t.Errorf("Blur = %v, want 1", s.Blur)
	}
	if s.Color.R != 0 || s.Color.G != 0 || s.Color.B != 255 {
		t.Errorf("Color = (%d,%d,%d), want blue", s.Color.R, s.Color.G, s.Color.B)
	}
}

// TestParseBoxShadowOneBadLayerDropsAll pins whole-declaration invalidity: a
// `none` layer among valid ones drops everything, so a previous valid
// declaration (not these layers) is what the cascade keeps.
func TestParseBoxShadowOneBadLayerDropsAll(t *testing.T) {
	for _, v := range []string{
		"none, red 0px -100px",
		"red 0px -100px, none",
		"red 0px -100px, bogus!!!",
		"10px",
		"10px 10px 10px 10px 10px",
		"10px 10px -5px red",
		"10px 10px 5px -2px red",
	} {
		if got := css.ParseBoxShadow(v); len(got) != 0 {
			t.Errorf("ParseBoxShadow(%q) = %v, want nil (whole declaration invalid)", v, got)
		}
	}
}

// TestParseTextShadowOneBadLayerDropsAll pins the same rule for text-shadow,
// including the 3-length cap and negative blur.
func TestParseTextShadowOneBadLayerDropsAll(t *testing.T) {
	for _, v := range []string{
		"1px 1px red, none",
		"1px 1px 2px 3px red",
		"1px 1px -2px red",
		"1px 1px inset red",
	} {
		if got := css.ParseTextShadow(v); len(got) != 0 {
			t.Errorf("ParseTextShadow(%q) = %v, want nil (whole declaration invalid)", v, got)
		}
	}
}

// TestParseShadowCurrentColor pins the currentcolor marker for both an
// explicit keyword and the omitted-color default.
func TestParseShadowCurrentColor(t *testing.T) {
	for _, v := range []string{"10px 5px 5px currentcolor", "-3em 0em"} {
		got := css.ParseBoxShadow(v)
		if len(got) != 1 || !got[0].ColorIsCurrent {
			t.Errorf("ParseBoxShadow(%q) should mark ColorIsCurrent, got %+v", v, got)
		}
	}
}

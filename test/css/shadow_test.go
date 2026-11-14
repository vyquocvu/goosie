package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

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

// qc:BR-281
func TestParseBoxShadowNegativeSpread(t *testing.T) {
	shadows := css.ParseBoxShadow("0 0 8px -4px rgb(0, 0, 255)")
	if len(shadows) != 1 {
		t.Fatalf("ParseBoxShadow with a negative spread = %v, want one layer", shadows)
	}
	if shadows[0].Spread != -4 {
		t.Errorf("Spread = %v, want -4", shadows[0].Spread)
	}
	if shadows[0].Blur != 8 {
		t.Errorf("Blur = %v, want 8", shadows[0].Blur)
	}
	if got := css.ParseBoxShadow("0 0 -8px red"); len(got) != 0 {
		t.Errorf("ParseBoxShadow(%q) = %v, want nil (a negative blur is invalid)", "0 0 -8px red", got)
	}
}

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

func TestParseBoxShadowNone(t *testing.T) {
	shadows := css.ParseBoxShadow("none")
	if shadows != nil {
		t.Errorf("expected nil for 'none', got %v", shadows)
	}
}

func TestParseBoxShadowEmpty(t *testing.T) {
	shadows := css.ParseBoxShadow("")
	if shadows != nil {
		t.Errorf("expected nil for empty string, got %v", shadows)
	}
}

func TestParseBoxShadowMultiple(t *testing.T) {
	shadows := css.ParseBoxShadow("2px 2px 4px red, -1px -1px 3px blue")
	if len(shadows) != 2 {
		t.Fatalf("expected 2 shadows, got %d", len(shadows))
	}
	if shadows[0].OffsetX != 2 || shadows[0].OffsetY != 2 {
		t.Errorf("shadow[0] offset = (%v,%v), want (2,2)", shadows[0].OffsetX, shadows[0].OffsetY)
	}
	if shadows[0].Color.R != 255 || shadows[0].Color.G != 0 || shadows[0].Color.B != 0 {
		t.Errorf("shadow[0] color = (%d,%d,%d), want red", shadows[0].Color.R, shadows[0].Color.G, shadows[0].Color.B)
	}
	if shadows[1].OffsetX != -1 || shadows[1].OffsetY != -1 {
		t.Errorf("shadow[1] offset = (%v,%v), want (-1,-1)", shadows[1].OffsetX, shadows[1].OffsetY)
	}
	if shadows[1].Color.R != 0 || shadows[1].Color.G != 0 || shadows[1].Color.B != 255 {
		t.Errorf("shadow[1] color = (%d,%d,%d), want blue", shadows[1].Color.R, shadows[1].Color.G, shadows[1].Color.B)
	}
}

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

func TestParseBoxShadowInsufficientLengths(t *testing.T) {
	shadows := css.ParseBoxShadow("5px red")
	if len(shadows) != 0 {
		t.Errorf("expected 0 shadows for single-length value, got %d", len(shadows))
	}
}

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

func TestParseTextShadowNone(t *testing.T) {
	shadows := css.ParseTextShadow("none")
	if shadows != nil {
		t.Errorf("expected nil for 'none', got %v", shadows)
	}
}

func TestParseTextShadowEmpty(t *testing.T) {
	shadows := css.ParseTextShadow("")
	if shadows != nil {
		t.Errorf("expected nil for empty string, got %v", shadows)
	}
}

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

func TestParseTextShadowInsufficientLengths(t *testing.T) {
	shadows := css.ParseTextShadow("5px red")
	if len(shadows) != 0 {
		t.Errorf("expected 0 shadows for single-length value, got %d", len(shadows))
	}
}

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

func TestParseBoxShadowOneBadLayerDropsAll(t *testing.T) {
	for _, v := range []string{
		"none, red 0px -100px",
		"red 0px -100px, none",
		"red 0px -100px, bogus!!!",
		"10px",
		"10px 10px 10px 10px 10px",
		"10px 10px -5px red",
	} {
		if got := css.ParseBoxShadow(v); len(got) != 0 {
			t.Errorf("ParseBoxShadow(%q) = %v, want nil (whole declaration invalid)", v, got)
		}
	}
}

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

func TestParseShadowCurrentColor(t *testing.T) {
	for _, v := range []string{"10px 5px 5px currentcolor", "-3em 0em"} {
		got := css.ParseBoxShadow(v)
		if len(got) != 1 || !got[0].ColorIsCurrent {
			t.Errorf("ParseBoxShadow(%q) should mark ColorIsCurrent, got %+v", v, got)
		}
	}
}

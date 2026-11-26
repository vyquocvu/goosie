package style_test

import (
	"math"
	"testing"
)

// qc:BR-328
func TestNestedHeadingUsesScopedUserAgentMargin(t *testing.T) {
	sheet := `h1 { color: rgb(1, 2, 3); }`
	nested := styleFor(t, `<html><body><section><h1>x</h1></section></body></html>`, sheet, "h1")
	plain := styleFor(t, `<html><body><h1>x</h1></body></html>`, sheet, "h1")
	if nested == nil || plain == nil {
		t.Fatal("missing computed heading styles")
	}
	near := func(name string, got, want float32) {
		t.Helper()
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("nested font-size", nested.FontSize, 24)
	near("nested margin-top", nested.MarginTop, 19.92)
	near("nested margin-bottom", nested.MarginBottom, 19.92)
	near("plain font-size", plain.FontSize, 32)
	near("plain margin-top", plain.MarginTop, 21.44)
	if nested.Color.R != 1 || nested.Color.G != 2 || nested.Color.B != 3 {
		t.Errorf("author heading color must still win over UA style, got %+v", nested.Color)
	}
}

// qc:BR-328
func TestCustomPropertyInitialUsesVarFallback(t *testing.T) {
	cs := styleFor(t, `<html><body><div id="x">x</div></body></html>`, `
		:root {
			--color: initial;
			--fallback: var(--color, rgb(4, 5, 6));
		}
		div { color: var(--fallback, rgb(7, 8, 9)); }
	`, "div")
	if cs == nil {
		t.Fatal("missing computed div style")
	}
	if cs.Color.R != 4 || cs.Color.G != 5 || cs.Color.B != 6 {
		t.Fatalf("color = %+v, want the var fallback (4,5,6)", cs.Color)
	}
}

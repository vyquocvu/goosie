package style_test

import (
	"math"
	"testing"
)

// rem is relative to the root font size (16px by default), never the element's
// own font size. Resolving it like em inflated every rem length on a large-font
// element: an h1 at font-size:2.5rem with margin-top:6.5rem took 260px instead
// of 104px, which pushed whole page headers far too tall.
func TestRemResolvesAgainstRootNotElement(t *testing.T) {
	sheet := `h1 { font-size: 2.5rem; margin-top: 6.5rem; padding: 1rem; line-height: 3.25rem; }`
	cs := styleFor(t, `<html><body><h1>x</h1></body></html>`, sheet, "h1")
	if cs == nil {
		t.Fatal("no computed style for h1")
	}
	near := func(name string, got, want float32) {
		t.Helper()
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("FontSize", cs.FontSize, 40)     // 2.5rem * 16
	near("MarginTop", cs.MarginTop, 104)  // 6.5rem * 16
	near("PaddingTop", cs.PaddingTop, 16) // 1rem * 16
}

func TestRemUsesComputedRootFontSize(t *testing.T) {
	sheet := `:root { font-size: 62.5%; } body { font-size: 1.6rem; } h1 { font-size: 2.4rem; }`
	body := styleFor(t, `<html><body><h1>x</h1></body></html>`, sheet, "body")
	h1 := styleFor(t, `<html><body><h1>x</h1></body></html>`, sheet, "h1")
	if body == nil || h1 == nil {
		t.Fatal("missing computed style")
	}
	if math.Abs(float64(body.FontSize-16)) > 0.01 {
		t.Errorf("body FontSize = %v, want 16 (1.6rem of a 10px root)", body.FontSize)
	}
	if math.Abs(float64(h1.FontSize-24)) > 0.01 {
		t.Errorf("h1 FontSize = %v, want 24 (2.4rem of a 10px root)", h1.FontSize)
	}
}

// em, by contrast, stays relative to the element's own font size.
func TestEmStillResolvesAgainstElementFontSize(t *testing.T) {
	sheet := `h1 { font-size: 40px; margin-top: 2em; }`
	cs := styleFor(t, `<html><body><h1>x</h1></body></html>`, sheet, "h1")
	if cs == nil {
		t.Fatal("no computed style for h1")
	}
	if math.Abs(float64(cs.MarginTop-80)) > 0.01 {
		t.Errorf("MarginTop = %v, want 80 (2em of a 40px element)", cs.MarginTop)
	}
}

// TestRemBoxPropertiesUseComputedRootFontSize verifies that box model properties
// (margin, padding, width, height, gap) properly resolve rem against the computed root font size.
func TestRemBoxPropertiesUseComputedRootFontSize(t *testing.T) {
	sheet := `
		:root { font-size: 10px; }
		div {
			margin-top: 2rem;
			padding-left: 1.5rem;
			width: 10rem;
			gap: 1rem 2.5rem;
		}
	`
	cs := styleFor(t, `<html><body><div>x</div></body></html>`, sheet, "div")
	if cs == nil {
		t.Fatal("missing computed style for div")
	}
	near := func(name string, got, want float32) {
		t.Helper()
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("MarginTop", cs.MarginTop, 20)     // 2rem * 10px
	near("PaddingLeft", cs.PaddingLeft, 15) // 1.5rem * 10px
	near("Width", cs.Width, 100)            // 10rem * 10px
	near("RowGap", cs.RowGap, 10)           // 1rem * 10px
	near("ColumnGap", cs.ColumnGap, 25)     // 2.5rem * 10px
}

// TestCalcWithRemUsesComputedRootFontSize verifies that calc() and math functions
// properly resolve rem units against the root element's font size.
func TestCalcWithRemUsesComputedRootFontSize(t *testing.T) {
	sheet := `
		:root { font-size: 10px; }
		div {
			margin-top: calc(2rem + 15px);
			padding-top: min(3rem, 50px);
			padding-bottom: max(1rem, 5px);
		}
	`
	cs := styleFor(t, `<html><body><div>x</div></body></html>`, sheet, "div")
	if cs == nil {
		t.Fatal("missing computed style for div")
	}
	near := func(name string, got, want float32) {
		t.Helper()
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("MarginTop", cs.MarginTop, 35)         // 2*10 + 15 = 35px
	near("PaddingTop", cs.PaddingTop, 30)       // min(3*10, 50) = 30px
	near("PaddingBottom", cs.PaddingBottom, 10) // max(1*10, 5) = 10px
}

// TestTwoValueGapShorthand verifies row-gap and column-gap in 2-value gap shorthand.
func TestTwoValueGapShorthand(t *testing.T) {
	sheet := `div { gap: 12px 24px; }`
	cs := styleFor(t, `<html><body><div>x</div></body></html>`, sheet, "div")
	if cs == nil {
		t.Fatal("missing computed style for div")
	}
	if cs.RowGap != 12 || cs.ColumnGap != 24 {
		t.Errorf("gap got (row=%v, col=%v), want (12, 24)", cs.RowGap, cs.ColumnGap)
	}
}

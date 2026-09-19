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

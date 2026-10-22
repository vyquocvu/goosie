package layout_test

import (
	"testing"
)

func TestLineClampTruncation(t *testing.T) {
	// A narrow container that forces multiple lines of text
	arena := session(t, `<html><body style="margin: 0;">
<div id="target" style="width: 80px; font-size: 16px; line-height: 20px; line-clamp: 2;">
  One two three four five six seven eight nine ten
</div>
</body></html>`, 800)

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("target div not found")
	}
	// With line-clamp: 2 and line-height: 20px, height should be clamped to 40px (2 lines)
	if !almostEqual(div.H, 40) {
		t.Errorf("div clamped height = %v, want 40", div.H)
	}
}

func TestTextWrapNowrap(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div id="target" style="width: 80px; font-size: 16px; line-height: 20px; text-wrap: nowrap;">
  One two three four
</div>
</body></html>`, 800)

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("target div not found")
	}
	// With text-wrap: nowrap, all words stay on a single line -> height is 20px
	if !almostEqual(div.H, 20) {
		t.Errorf("div height = %v, want 20 (single line)", div.H)
	}
}

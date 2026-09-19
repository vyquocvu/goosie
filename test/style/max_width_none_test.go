package style_test

import (
	"testing"
)

// `max-width: none` states that there is no cap. Falling through the length
// resolver treated the keyword as a zero-pixel one, so every box carrying it -
// Wikipedia's infobox among them - collapsed to nothing.
func TestMaxWidthNoneLeavesTheBoxUncapped(t *testing.T) {
	s := styleFor(t, `<html><body><main class="p"></main></body></html>`,
		`.p{max-width:none;max-height:none;min-width:none;min-height:none}`, "main")
	if s == nil {
		t.Fatal("main style not found")
	}
	if s.MaxWidth != -1 {
		t.Errorf("MaxWidth = %v, want -1 (no cap)", s.MaxWidth)
	}
	if s.MaxHeight != -1 {
		t.Errorf("MaxHeight = %v, want -1 (no cap)", s.MaxHeight)
	}
	if s.MinWidth != 0 {
		t.Errorf("MinWidth = %v, want 0", s.MinWidth)
	}
	if s.MinHeight != 0 {
		t.Errorf("MinHeight = %v, want 0", s.MinHeight)
	}
}

package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// Blink's user-agent sheet zeroes the block-axis margins of every list nested
// inside another list, so a `<ul>` hanging off an `<ol>` or a wiki `dl` starts
// flush against its parent's text instead of pushing a blank line into it.
// Only the block-axis margins go: the parent's indentation survives.
func TestNestedListLosesItsBlockMargins(t *testing.T) {
	nested := styleFor(t, `<html><body><ol><li><ul></ul></li></ol></body></html>`, "", "ul")
	if nested == nil {
		t.Fatal("nested ul style not found")
	}
	if nested.MarginTop != 0 || nested.MarginBottom != 0 {
		t.Errorf("nested ul margins = %v/%v, want 0/0", nested.MarginTop, nested.MarginBottom)
	}

	top := styleFor(t, `<html><body><ul></ul></body></html>`, "", "ul")
	if top == nil {
		t.Fatal("top-level ul style not found")
	}
	if top.MarginTop != 16 || top.MarginBottom != 16 {
		t.Errorf("top-level ul margins = %v/%v, want 16/16", top.MarginTop, top.MarginBottom)
	}

	olInUl := styleFor(t, `<html><body><ul><li><ol></ol></li></ul></body></html>`, "", "ol")
	if olInUl == nil {
		t.Fatal("nested ol style not found")
	}
	if olInUl.MarginTop != 0 {
		t.Errorf("nested ol margin-top = %v, want 0", olInUl.MarginTop)
	}

	defInDl := styleFor(t, `<html><body><ul><li><dl></dl></li></ul></body></html>`, "", "dl")
	if defInDl == nil {
		t.Fatal("nested dl style not found")
	}
	if defInDl.MarginTop != 0 {
		t.Errorf("nested dl margin-top = %v, want 0", defInDl.MarginTop)
	}
	if defInDl.Display != style.DisplayBlock {
		t.Errorf("nested dl display = %v, want block (only the block-axis margins go)", defInDl.Display)
	}
}

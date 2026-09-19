package style_test

import (
	"testing"
)

// `ch` is the width of a "0" glyph, so it tracks the element's font size the way
// `em` does. Falling through the unit table untreated made `max-width: 60ch` a
// 60-pixel column, and CSS-Tricks' prose wrapped one word per line.
func TestChAndEmTrackFontSizeInDimensions(t *testing.T) {
	s := styleFor(t, `<html><body><main class="p"></main></body></html>`,
		`.p{font-size:20px;max-width:30ch;width:10em;min-height:2ch;max-height:4em}`, "main")
	if s == nil {
		t.Fatal("main style not found")
	}
	for _, c := range []struct {
		name string
		got  float32
		want float32
	}{
		{"MaxWidth", s.MaxWidth, 300},
		{"Width", s.Width, 200},
		{"MinHeight", s.MinHeight, 20},
		{"MaxHeight", s.MaxHeight, 80},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// With no font-size of its own the element inherits 16px, and `ch` lands on
	// half of it: the width of a zero in the default sans face.
	def := styleFor(t, `<html><body><main class="p"></main></body></html>`, `.p{max-width:60ch}`, "main")
	if def == nil {
		t.Fatal("default-font main style not found")
	}
	if def.MaxWidth != 480 {
		t.Errorf("MaxWidth = %v, want 480 (60ch at 16px)", def.MaxWidth)
	}
}

// Wikipedia's infobox declares `width: 22em` and `font-size: 88%` in the same
// rule. The em has to resolve against the element's own computed font size, so
// the order the two declarations happen to sit in cannot decide whether the box
// comes out 352px or 310px wide.
func TestFontRelativeLengthIgnoresDeclarationOrder(t *testing.T) {
	first := styleFor(t, `<html><body><main class="p"></main></body></html>`,
		`.p{width:22em;font-size:88%}`, "main")
	last := styleFor(t, `<html><body><main class="p"></main></body></html>`,
		`.p{font-size:88%;width:22em}`, "main")
	if first == nil || last == nil {
		t.Fatal("main style not found")
	}
	if first.Width != last.Width {
		t.Errorf("width = %v when declared before font-size, %v when after; want both %v",
			first.Width, last.Width, last.Width)
	}
	if d := last.Width - 309.76; d > 0.01 || d < -0.01 {
		t.Errorf("Width = %v, want 309.76 (22em at 88%% of 16px)", last.Width)
	}
}

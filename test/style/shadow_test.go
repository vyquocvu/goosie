package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/style"
)

// TestBoxShadowResolved verifies that box-shadow declarations resolve through
// the cascade into the computed style's BoxShadow field.
func TestBoxShadowResolved(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		`.x { box-shadow: 3px 5px 10px 2px rgba(0,0,0,0.5) }`,
		"div")
	if len(s.BoxShadow) != 1 {
		t.Fatalf("expected 1 box shadow, got %d", len(s.BoxShadow))
	}
	sh := s.BoxShadow[0]
	if sh.OffsetX != 3 {
		t.Errorf("OffsetX = %v, want 3", sh.OffsetX)
	}
	if sh.OffsetY != 5 {
		t.Errorf("OffsetY = %v, want 5", sh.OffsetY)
	}
	if sh.Blur != 10 {
		t.Errorf("Blur = %v, want 10", sh.Blur)
	}
	if sh.Spread != 2 {
		t.Errorf("Spread = %v, want 2", sh.Spread)
	}
}

// TestBoxShadowNone verifies that box-shadow:none results in nil.
func TestBoxShadowNone(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		`.x { box-shadow: none }`,
		"div")
	if s.BoxShadow != nil {
		t.Errorf("expected nil BoxShadow for 'none', got %v", s.BoxShadow)
	}
}

// TestBoxShadowMultiple verifies that multiple comma-separated shadows resolve.
func TestBoxShadowMultiple(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		`.x { box-shadow: 2px 2px 4px red, -1px -1px 3px blue }`,
		"div")
	if len(s.BoxShadow) != 2 {
		t.Fatalf("expected 2 box shadows, got %d", len(s.BoxShadow))
	}
	if s.BoxShadow[0].Color.R != 255 {
		t.Errorf("shadow[0] should be red, got (%d,%d,%d)",
			s.BoxShadow[0].Color.R, s.BoxShadow[0].Color.G, s.BoxShadow[0].Color.B)
	}
	if s.BoxShadow[1].Color.B != 255 {
		t.Errorf("shadow[1] should be blue, got (%d,%d,%d)",
			s.BoxShadow[1].Color.R, s.BoxShadow[1].Color.G, s.BoxShadow[1].Color.B)
	}
}

// TestBoxShadowInset verifies the inset keyword resolves.
func TestBoxShadowInset(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		`.x { box-shadow: inset 2px 2px 4px black }`,
		"div")
	if len(s.BoxShadow) != 1 {
		t.Fatalf("expected 1 box shadow, got %d", len(s.BoxShadow))
	}
	if !s.BoxShadow[0].Inset {
		t.Error("expected Inset to be true")
	}
}

// TestBoxShadowDefault verifies that an element with no box-shadow has nil.
func TestBoxShadowDefault(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		``,
		"div")
	if s.BoxShadow != nil {
		t.Errorf("expected nil BoxShadow by default, got %v", s.BoxShadow)
	}
}

// TestTextShadowResolved verifies that text-shadow declarations resolve.
func TestTextShadowResolved(t *testing.T) {
	s := styleFor(t,
		`<html><body><p class="x">text</p></body></html>`,
		`.x { text-shadow: 1px 2px 3px rgba(0,0,0,0.5) }`,
		"p")
	if len(s.TextShadow) != 1 {
		t.Fatalf("expected 1 text shadow, got %d", len(s.TextShadow))
	}
	sh := s.TextShadow[0]
	if sh.OffsetX != 1 {
		t.Errorf("OffsetX = %v, want 1", sh.OffsetX)
	}
	if sh.OffsetY != 2 {
		t.Errorf("OffsetY = %v, want 2", sh.OffsetY)
	}
	if sh.Blur != 3 {
		t.Errorf("Blur = %v, want 3", sh.Blur)
	}
}

// TestTextShadowNone verifies that text-shadow:none results in nil.
func TestTextShadowNone(t *testing.T) {
	s := styleFor(t,
		`<html><body><p class="x">text</p></body></html>`,
		`.x { text-shadow: none }`,
		"p")
	if s.TextShadow != nil {
		t.Errorf("expected nil TextShadow for 'none', got %v", s.TextShadow)
	}
}

// TestTextShadowMultiple verifies multiple text shadows resolve.
func TestTextShadowMultiple(t *testing.T) {
	s := styleFor(t,
		`<html><body><p class="x">text</p></body></html>`,
		`.x { text-shadow: 1px 1px 2px black, 0 0 5px white }`,
		"p")
	if len(s.TextShadow) != 2 {
		t.Fatalf("expected 2 text shadows, got %d", len(s.TextShadow))
	}
}

// TestTextShadowDefault verifies that an element with no text-shadow has nil.
func TestTextShadowDefault(t *testing.T) {
	s := styleFor(t,
		`<html><body><p class="x">text</p></body></html>`,
		``,
		"p")
	if s.TextShadow != nil {
		t.Errorf("expected nil TextShadow by default, got %v", s.TextShadow)
	}
}

// TestTextShadowNoBlur verifies text-shadow works without blur.
func TestTextShadowNoBlur(t *testing.T) {
	s := styleFor(t,
		`<html><body><p class="x">text</p></body></html>`,
		`.x { text-shadow: 2px 2px red }`,
		"p")
	if len(s.TextShadow) != 1 {
		t.Fatalf("expected 1 text shadow, got %d", len(s.TextShadow))
	}
	sh := s.TextShadow[0]
	if sh.OffsetX != 2 || sh.OffsetY != 2 {
		t.Errorf("offset = (%v,%v), want (2,2)", sh.OffsetX, sh.OffsetY)
	}
	if sh.Blur != 0 {
		t.Errorf("Blur = %v, want 0", sh.Blur)
	}
	if sh.Color.R != 255 || sh.Color.G != 0 || sh.Color.B != 0 {
		t.Errorf("Color = (%d,%d,%d), want red", sh.Color.R, sh.Color.G, sh.Color.B)
	}
}

// TestShadowNotInherited verifies that box-shadow and text-shadow are not
// inherited from parent to child (they are not inherited CSS properties).
func TestShadowNotInherited(t *testing.T) {
	const html = `<html><body><div class="parent"><span class="child">text</span></div></body></html>`
	const sheet = `.parent { box-shadow: 5px 5px 10px black; text-shadow: 2px 2px 4px red; }`
	child := styleFor(t, html, sheet, "span")
	if len(child.BoxShadow) != 0 {
		t.Errorf("box-shadow should not inherit, child got %d shadows", len(child.BoxShadow))
	}
	if len(child.TextShadow) != 0 {
		t.Errorf("text-shadow should not inherit, child got %d shadows", len(child.TextShadow))
	}
}

// TestBoxShadowHexColor verifies box-shadow with hex color works.
func TestBoxShadowHexColor(t *testing.T) {
	s := styleFor(t,
		`<html><body><div class="x">x</div></body></html>`,
		`.x { box-shadow: 4px 4px #ff0000 }`,
		"div")
	if len(s.BoxShadow) != 1 {
		t.Fatalf("expected 1 box shadow, got %d", len(s.BoxShadow))
	}
	sh := s.BoxShadow[0]
	if sh.Color.R != 255 || sh.Color.G != 0 || sh.Color.B != 0 {
		t.Errorf("Color = (%d,%d,%d), want red", sh.Color.R, sh.Color.G, sh.Color.B)
	}
}

// TestShadowPanicsNoPanic verifies that various shadow declarations do not
// panic the resolver.
func TestShadowPanicsNoPanic(t *testing.T) {
	decls := []string{
		"box-shadow: none",
		"box-shadow: 0 0 0 black",
		"box-shadow: 1px 1px",
		"box-shadow: inset 0 0 10px white",
		"box-shadow: 2px 2px 4px 2px rgba(0,0,0,0.3), 0 0 8px rgba(255,0,0,0.5)",
		"text-shadow: none",
		"text-shadow: 1px 1px 2px black",
		"text-shadow: 0 0 red, 1px 1px blue, 2px 2px green",
		"text-shadow: -1px -1px 0 white",
	}
	for _, decl := range decls {
		t.Run(decl, func(t *testing.T) {
			// Must not panic.
			_ = styleFor(t,
				`<html><body><div class="x">x</div></body></html>`,
				`.x { `+decl+` }`,
				"div")
		})
	}
	// Suppress unused import.
	_ = style.DisplayNone
}

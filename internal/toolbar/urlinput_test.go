package toolbar

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/surface"
)

type memClip struct{ text string }

func (m *memClip) Read() string   { return m.text }
func (m *memClip) Write(s string) { m.text = s }

func addrPos(s *State, frac float64) frame.Point {
	w := s.Bounds.W()
	lr := AddressBarRect(w)
	x := int32(float64(lr.X0) + frac*float64(lr.X1-lr.X0))
	return frame.Point{X: x, Y: 20}
}

func TestVerifyURLInputBehavesLikeNormalInput(t *testing.T) {
	s := NewState(800, nil)
	s.Clipboard = &memClip{}
	s.Navigate("https://example.com/hello")

	// 1. First click focuses and selects all.
	s.HandlePointer(addrPos(s, 0.5), surface.ButtonLeft, surface.PointerPress)
	if s.GetFocus() != FocusAddress {
		t.Fatal("click did not focus address bar")
	}
	if !s.hasSelection() {
		t.Fatal("first click did not select all")
	}
	s.HandlePointer(addrPos(s, 0.5), surface.ButtonLeft, surface.PointerRelease)

	// 2. Second click places the caret (no selection).
	s.HandlePointer(addrPos(s, 0.05), surface.ButtonLeft, surface.PointerPress)
	s.HandlePointer(addrPos(s, 0.05), surface.ButtonLeft, surface.PointerRelease)
	if s.hasSelection() {
		t.Fatal("click on focused bar should place caret, not keep selection")
	}
	if s.Cursor != s.hitCursorDev(addrPos(s, 0.05).X, s.Bounds.W()) {
		t.Fatal("caret not placed at click")
	}

	// 3. Drag selects a range.
	start := addrPos(s, 0.2)
	end := addrPos(s, 0.6)
	s.HandlePointer(start, surface.ButtonLeft, surface.PointerPress)
	s.HandlePointer(end, surface.ButtonLeft, surface.PointerMotion)
	if !s.hasSelection() {
		t.Fatal("drag did not select")
	}
	s.HandlePointer(end, surface.ButtonLeft, surface.PointerRelease)

	// 4. Cmd+C copies the selection (this was broken: editKeyLocked dropped it).
	s.HandleKeyEvent('c', surface.ModCommand)
	mc := s.Clipboard.(*memClip)
	if mc.text == "" {
		t.Fatal("Cmd+C copied nothing")
	}

	// 5. Cmd+X cuts, Cmd+V pastes.
	before := len([]rune(s.Input))
	s.HandleKeyEvent('x', surface.ModCommand)
	if len([]rune(s.Input)) >= before {
		t.Fatal("Cmd+X did not cut")
	}
	s.HandleKeyEvent('v', surface.ModCommand)
	if len([]rune(s.Input)) != before {
		t.Fatalf("Cmd+V did not restore, got %q", s.Input)
	}

	// 6. Cmd+A selects all.
	s.HandlePointer(addrPos(s, 0.05), surface.ButtonLeft, surface.PointerPress)
	s.HandlePointer(addrPos(s, 0.05), surface.ButtonLeft, surface.PointerRelease)
	s.HandleKeyEvent('a', surface.ModCommand)
	if !s.hasSelection() || s.selMin() != 0 || s.selMax() != len([]rune(s.Input)) {
		t.Fatal("Cmd+A did not select all")
	}

	// 7. Esc reverts an edit and blurs.
	s.HandleKeyEvent('z', 0) // type something (inserts 'z' over selection)
	if s.Input == "https://example.com/hello" {
		t.Fatal("typing did not replace selection")
	}
	s.HandleKeyEvent(0x1b, 0)
	if s.GetFocus() != FocusNone {
		t.Fatal("Esc did not blur")
	}
	if s.Input != "https://example.com/hello" {
		t.Fatalf("Esc did not revert edit, got %q", s.Input)
	}

	// 8. FocusURL selects all (Cmd+L path).
	s.FocusURL()
	if s.GetFocus() != FocusAddress || !s.hasSelection() {
		t.Fatal("FocusURL did not focus+select-all")
	}

	// 9. Pasted newlines are stripped.
	s.pasteTextLocked("https://example.com/a\nb\r\nc")
	for _, r := range s.Input {
		if r == '\n' || r == '\r' {
			t.Fatalf("paste left newline in %q", s.Input)
		}
	}
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// Unicode text input contract: every layer of the edit pipeline — value
// storage, caret math, backspace, painting, selection — is rune-based, so an
// astral emoji counts as one character and never splits into lone surrogates.

func unicodeInputSession(t *testing.T, value string) (*engine.Session, float32, float32) {
	t.Helper()
	s := newFocusSession(t, `<html><body style="margin: 0;"><input id="a" value="`+value+`"></body></html>`)
	x, y := clickPoint(t, s, "a")
	if !s.FocusControl(x, y) {
		t.Fatal("focus not gained")
	}
	return s, x, y
}

func TestUnicodeInsertion(t *testing.T) {
	s, _, _ := unicodeInputSession(t, "")
	for _, r := range []rune{'é', '中', '\U0001F600'} {
		if !s.Edit(engine.EditRune, r) {
			t.Fatalf("Edit(%q) declined", r)
		}
	}
	if got := s.Focused().GetAttribute("value"); got != "é中😀" {
		t.Fatalf("value = %q, want %q", got, "é中😀")
	}
}

func TestUnicodeBackspaceWholeRune(t *testing.T) {
	s, _, _ := unicodeInputSession(t, "中😀")
	if !s.Edit(engine.EditBackspace, 0) || !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace declined")
	}
	if got := s.Focused().GetAttribute("value"); got != "" {
		t.Fatalf("value = %q, want empty; backspace must delete whole runes", got)
	}
}

func TestUnicodeCaretByRune(t *testing.T) {
	s, _, _ := unicodeInputSession(t, "a中b")
	for i := 0; i < 3; i++ {
		s.Edit(engine.EditLeft, 0)
	}
	s.Edit(engine.EditRight, 0)
	s.Edit(engine.EditRune, 'x')
	if got := s.Focused().GetAttribute("value"); got != "ax中b" {
		t.Fatalf("value = %q, want %q", got, "ax中b")
	}
}

func TestUnicodePaintsValueRun(t *testing.T) {
	s, _, _ := unicodeInputSession(t, "")
	s.Edit(engine.EditRune, 'é')
	s.Edit(engine.EditRune, '中')
	dl := s.Paint(1).Build(1)
	runes := map[rune]bool{}
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			for _, g := range c.Text.Glyphs {
				runes[g.Rune] = true
			}
		}
	}
	for _, r := range []rune{'é', '中'} {
		if !runes[r] {
			t.Fatalf("painted text missing %q", r)
		}
	}
}

func TestUnicodeSelectionText(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<p style="font-size: 16px;">café naïve résumé</p></body></html>`)
	cx0, cy0, _, cy1 := selWord(t, s, "café")
	_, ny0, nx1, ny1 := selWord(t, s, "naïve")
	s.SelectAt(cx0-1, (cy0+cy1)/2)
	s.SelectTo(nx1+1, (ny0+ny1)/2)
	if got := s.SelectionText(); got != "café naïve" {
		t.Fatalf("SelectionText = %q, want %q", got, "café naïve")
	}
}

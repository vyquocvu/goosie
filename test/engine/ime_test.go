package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// IME composition contract: the engine holds marked (composing) text at the
// focused control's caret until an IME commit replaces it with real value
// changes. Marked text never becomes the value until CommitText runs, blur
// discards it, and maxlength applies to commits, not previews.

func TestSetMarkedWithoutFocus(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.SetMarked("に") {
		t.Fatal("SetMarked without focus should not report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want empty", s.Marked())
	}
}

func TestSetMarkedAndCommit(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a") // value "ab", caret ends at 2
	s.FocusControl(x, y)
	if !s.SetMarked("ni") {
		t.Fatal("SetMarked should report change")
	}
	if s.Marked() != "ni" {
		t.Fatalf("Marked = %q, want %q", s.Marked(), "ni")
	}
	if got := s.Focused().GetAttribute("value"); got != "ab" {
		t.Fatalf("value during composition = %q, want unchanged %q", got, "ab")
	}
	if !s.CommitText("日") {
		t.Fatal("CommitText should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked after commit = %q, want empty", s.Marked())
	}
	if got := s.Focused().GetAttribute("value"); got != "ab日" {
		t.Fatalf("value = %q, want %q", got, "ab日")
	}
}

func TestSetMarkedUnchangedNoChange(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("ni")
	if s.SetMarked("ni") {
		t.Fatal("SetMarked with identical text should not report change")
	}
	if !s.SetMarked("") {
		t.Fatal("clearing marked should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want empty", s.Marked())
	}
}

func TestCommitTextRespectsMaxlength(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="ab" maxlength="2"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("xyz")
	if s.CommitText("xyz") {
		t.Fatal("commit past maxlength should not report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked = %q, want cleared even on declined commit", s.Marked())
	}
	if got := s.Focused().GetAttribute("value"); got != "ab" {
		t.Fatalf("value = %q, want %q", got, "ab")
	}
}

func TestMarkedClearsOnBlurAndSwitch(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("かな")
	s.FocusControl(790, 590) // blur
	if s.Marked() != "" {
		t.Fatalf("Marked after blur = %q, want empty", s.Marked())
	}
	s.FocusControl(x, y)
	s.SetMarked("かな")
	bx, by := clickPoint(t, s, "b")
	s.FocusControl(bx, by) // switch
	if s.Marked() != "" {
		t.Fatalf("Marked after switch = %q, want empty", s.Marked())
	}
}

func TestEditEscapeClearsMarked(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.SetMarked("かな")
	if !s.Edit(engine.EditEscape, 0) {
		t.Fatal("escape should report change")
	}
	if s.Marked() != "" {
		t.Fatalf("Marked after escape = %q, want empty", s.Marked())
	}
}

func TestPaintMarkedPreviewAtCaret(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="ab"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y) // caret at end
	s.Edit(engine.EditHome, 0)
	s.SetMarked("か")
	dl := s.Paint(1).Build(1)
	runes := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(runes) != "かab" {
		t.Fatalf("painted value = %q, want marked preview spliced at caret: %q", string(runes), "かab")
	}
}

func TestPaintMarkedOnEmptyControlSuppressesPlaceholder(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="empty" placeholder="hint"></body></html>`)
	x, y := clickPoint(t, s, "empty")
	s.FocusControl(x, y)
	s.SetMarked("か")
	dl := s.Paint(1).Build(1)
	grey := frame.RGB(117, 117, 117)
	hints := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == grey {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'h' {
					hints++
				}
			}
		}
	}
	if hints != 0 {
		t.Fatalf("placeholder glyphs while composing = %d, want 0", hints)
	}
	marked := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'か' {
					marked++
				}
			}
		}
	}
	if marked == 0 {
		t.Fatal("marked preview not painted on empty control")
	}
}

func TestPaintPasswordMasksMarked(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="pw" type="password" value="ab"></body></html>`)
	x, y := clickPoint(t, s, "pw")
	s.FocusControl(x, y)
	s.SetMarked("xyz")
	dl := s.Paint(1).Build(1)
	got := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(got) != "•••••" {
		t.Fatalf("password glyphs = %q, want dots masking value and marked text", string(got))
	}
}

func TestCommitTextarea(t *testing.T) {
	s := newFocusSession(t, textareaHTML)
	x, y := clickPoint(t, s, "ta")
	s.FocusControl(x, y)
	s.SetMarked("に")
	if !s.CommitText("日") {
		t.Fatal("CommitText should report change")
	}
	if got := s.Focused().FirstChild.DataContent; got != "hi日" {
		t.Fatalf("textarea content = %q, want %q", got, "hi日")
	}
}

func TestPaintMarkedTextareaPreview(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<textarea id="ta">hi</textarea></body></html>`)
	x, y := clickPoint(t, s, "ta")
	s.FocusControl(x, y)
	s.Edit(engine.EditEnd, 0)
	s.SetMarked("か")
	dl := s.Paint(1).Build(1)
	runes := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(runes) != "hiか" {
		t.Fatalf("textarea painted text = %q, want %q", string(runes), "hiか")
	}
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func findElement(root *dom.Node, data string) *dom.Node {
	if root.Data == data && root.Element() {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if got := findElement(c, data); got != nil {
			return got
		}
	}
	return nil
}

func newFocusSession(t *testing.T, html string) *engine.Session {
	t.Helper()
	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return s
}

// clickPoint returns the centre of the laid-out element with the given id.
func clickPoint(t *testing.T, s *engine.Session, id string) (float32, float32) {
	t.Helper()
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return (x0 + x1) / 2, (y0 + y1) / 2
		}
	}
	t.Fatalf("element %q not found in arena", id)
	return 0, 0
}

const twoInputsHTML = `<html><body style="margin: 0;">` +
	`<input id="a" value="ab"><input id="b" value="cd"></body></html>`

func TestFocusControlHit(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.Focused() != nil {
		t.Fatal("session starts with focus")
	}
	x, y := clickPoint(t, s, "a")
	if !s.FocusControl(x, y) {
		t.Fatal("focusing a control should report change")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "a" {
		t.Fatalf("focused = %v, want input#a", n)
	}
}

func TestFocusControlBlur(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.FocusControl(790, 590) {
		t.Fatal("blur should report change")
	}
	if s.Focused() != nil {
		t.Fatalf("focused = %v, want nil", s.Focused())
	}
	if s.FocusControl(790, 590) {
		t.Fatal("blur while unfocused should not report change")
	}
}

func TestFocusControlSwitch(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	x, y = clickPoint(t, s, "b")
	if !s.FocusControl(x, y) {
		t.Fatal("switching focus should report change")
	}
	if n := s.Focused(); n == nil || n.GetAttribute("id") != "b" {
		t.Fatalf("focused = %v, want input#b", n)
	}
}

func TestEditInsert(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditRune, 'x') {
		t.Fatal("insert should report change")
	}
	if got := s.Focused().GetAttribute("value"); got != "abx" {
		t.Fatalf("value = %q, want %q", got, "abx")
	}
}

func TestEditBackspace(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace should report change")
	}
	if got := s.Focused().GetAttribute("value"); got != "a" {
		t.Fatalf("value = %q, want %q", got, "a")
	}
	s.Edit(engine.EditBackspace, 0)
	if got := s.Focused().GetAttribute("value"); got != "" {
		t.Fatalf("value = %q, want empty", got)
	}
	if s.Edit(engine.EditBackspace, 0) {
		t.Fatal("backspace on empty should not report change")
	}
}

func TestEditArrowsAndHomeEnd(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y) // caret at 2
	if s.Edit(engine.EditLeft, 0) {
		t.Fatal("caret move should not report change")
	}
	s.Edit(engine.EditRune, '-') // "a-b"
	s.Edit(engine.EditHome, 0)
	s.Edit(engine.EditRune, '>') // ">a-b"
	if got := s.Focused().GetAttribute("value"); got != ">a-b" {
		t.Fatalf("value = %q, want %q", got, ">a-b")
	}
	s.Edit(engine.EditEnd, 0)
	s.Edit(engine.EditRune, '<') // ">a-b<"
	if got := s.Focused().GetAttribute("value"); got != ">a-b<" {
		t.Fatalf("value = %q, want %q", got, ">a-b<")
	}
	if s.Edit(engine.EditRight, 0) {
		t.Fatal("right at end should not report change")
	}
}

func TestEditEscapeBlurs(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if !s.Edit(engine.EditEscape, 0) {
		t.Fatal("escape should report change")
	}
	if s.Focused() != nil {
		t.Fatal("escape should blur")
	}
}

func TestEditEnterInputIgnored(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if s.Edit(engine.EditEnter, 0) {
		t.Fatal("enter on input should not report change")
	}
}

const textareaHTML = `<html><body style="margin: 0;">` +
	`<textarea id="ta">hi</textarea></body></html>`

func TestEditEnterTextarea(t *testing.T) {
	s := newFocusSession(t, textareaHTML)
	x, y := clickPoint(t, s, "ta")
	if !s.FocusControl(x, y) {
		t.Fatal("textarea not focused")
	}
	if !s.Edit(engine.EditEnter, 0) {
		t.Fatal("enter on textarea should report change")
	}
	ta := s.Focused()
	if got := ta.FirstChild.DataContent; got != "hi\n" {
		t.Fatalf("textarea content = %q, want %q", got, "hi\n")
	}
}

func TestEditMaxlength(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">`+
		`<input id="a" value="abc" maxlength="3"></body></html>`)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	if s.Edit(engine.EditRune, 'd') {
		t.Fatal("insert past maxlength should not report change")
	}
	if got := s.Focused().GetAttribute("value"); got != "abc" {
		t.Fatalf("value = %q, want %q", got, "abc")
	}
}

func TestEditNoFocusNoop(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	if s.Edit(engine.EditRune, 'x') {
		t.Fatal("edit without focus should not report change")
	}
}

func TestValueSurvivesReflow(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	s.FocusControl(x, y)
	s.Edit(engine.EditRune, 'z')
	if err := s.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	if got := s.Focused().GetAttribute("value"); got != "abz" {
		t.Fatalf("value after reflow = %q, want %q", got, "abz")
	}
}

func textRunesIn(t *testing.T, dl *paint.LayerDL, color frame.Color) []rune {
	t.Helper()
	var out []rune
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == color {
			for _, g := range c.Text.Glyphs {
				out = append(out, g.Rune)
			}
		}
	}
	return out
}

func countFills(t *testing.T, dl *paint.LayerDL, color frame.Color) int {
	t.Helper()
	n := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color == color {
			n++
		}
	}
	return n
}

func TestPaintFocusedShowsValueCaretAndRing(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	x, y := clickPoint(t, s, "a")
	if !s.FocusControl(x, y) {
		t.Fatal("focus not gained")
	}
	dl := s.Paint(1).Build(1)
	// Each input paints its value as one black text run.
	valueRun := ""
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			run := make([]rune, 0, len(c.Text.Glyphs))
			for _, g := range c.Text.Glyphs {
				run = append(run, g.Rune)
			}
			if string(run) == "ab" {
				valueRun = string(run)
			}
		}
	}
	if valueRun != "ab" {
		t.Fatalf("no black text run with the focused value %q", valueRun)
	}
	if n := countFills(t, dl, frame.RGB(0, 103, 244)); n < 4 {
		t.Fatalf("focus ring strips = %d, want >= 4", n)
	}
	caret := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color == frame.RGB(0, 0, 0) &&
			c.Rect.X1-c.Rect.X0 == 1 && c.Rect.Y1 > c.Rect.Y0 {
			caret++
		}
	}
	if caret == 0 {
		t.Fatal("no 1px caret bar in display list")
	}
}

func TestPaintUnfocusedNoRing(t *testing.T) {
	s := newFocusSession(t, twoInputsHTML)
	dl := s.Paint(1).Build(1)
	if n := countFills(t, dl, frame.RGB(0, 103, 244)); n != 0 {
		t.Fatalf("focus ring present without focus (%d strips)", n)
	}
}

func TestPaintPlaceholderOnlyWhenEmpty(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<input id="full" value="ab" placeholder="hint"><input id="empty" placeholder="hint"></body></html>`)
	grey := frame.RGB(117, 117, 117)
	x, y := clickPoint(t, s, "full")
	s.FocusControl(x, y)
	dl := s.Paint(1).Build(1)
	foundValue := false
	hints := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == frame.RGB(0, 0, 0) {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'a' || g.Rune == 'b' {
					foundValue = true
				}
			}
		}
	}
	if !foundValue {
		t.Fatal("value glyphs missing")
	}
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color == grey {
			for _, g := range c.Text.Glyphs {
				if g.Rune == 'h' {
					hints++
				}
			}
		}
	}
	if hints != 1 {
		t.Fatalf("placeholder 'h' glyphs = %d, want 1 (empty control only)", hints)
	}
}

func TestPaintPasswordDots(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<input id="pw" type="password" value="abc"></body></html>`)
	x, y := clickPoint(t, s, "pw")
	s.FocusControl(x, y)
	dl := s.Paint(1).Build(1)
	got := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(got) != "•••" {
		t.Fatalf("password glyphs = %q, want dots", string(got))
	}
}

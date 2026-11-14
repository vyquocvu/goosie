package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
)

const selParagraphHTML = `<html><body style="margin: 0;">` +
	`<p style="font-size: 16px;">hello brave world</p></body></html>`

const selBreakHTML = `<html><body style="margin: 0;">` +
	`<p style="font-size: 16px;">first line<br>second line</p></body></html>`

func selWord(t *testing.T, s *engine.Session, text string) (x0, y0, x1, y1 float32) {
	t.Helper()
	found := false
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.Type == 2 && obj.Node.DataContent == text {
			x0, y0, x1, y1 = obj.BorderRect()
			found = true
		}
	}
	if found {
		return x0, y0, x1, y1
	}
	t.Fatalf("word %q not found in arena", text)
	return 0, 0, 0, 0
}

func TestSelectDragSelectsWords(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	if s.HasSelection() {
		t.Fatal("press alone should not select")
	}
	s.SelectTo(wx1+1, (wy0+wy1)/2)
	if !s.HasSelection() {
		t.Fatal("drag across the line should select")
	}
	if got := s.SelectionText(); got != "hello brave world" {
		t.Fatalf("SelectionText = %q, want %q", got, "hello brave world")
	}
}

func TestSelectBackwardDragSameText(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(wx1+1, (wy0+wy1)/2)
	s.SelectTo(hx0-1, (hy0+hy1)/2)
	if got := s.SelectionText(); got != "hello brave world" {
		t.Fatalf("backward SelectionText = %q, want %q", got, "hello brave world")
	}
}

func TestSelectPartialWord(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	s.SelectTo(hx0+20, (hy0+hy1)/2)
	if got := s.SelectionText(); got != "he" {
		t.Fatalf("partial SelectionText = %q, want %q", got, "he")
	}
}

func TestSelectAcrossLineBreak(t *testing.T) {
	s := newFocusSession(t, selBreakHTML)
	fx0, fy0, _, fy1 := selWord(t, s, "first")
	_, sy0, sx1, sy1 := selWord(t, s, "line")
	s.SelectAt(fx0-1, (fy0+fy1)/2)
	s.SelectTo(sx1+1, (sy0+sy1)/2+1)
	if got := s.SelectionText(); got != "first line\nsecond line" {
		t.Fatalf("SelectionText = %q, want %q", got, "first line\nsecond line")
	}
}

func TestSelectAtReplacesSelection(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	s.SelectTo(wx1+1, (wy0+wy1)/2)
	if !s.HasSelection() {
		t.Fatal("setup: selection missing")
	}
	bx0, by0, bx1, by1 := selWord(t, s, "brave")
	s.SelectAt((bx0+bx1)/2, (by0+by1)/2)
	if s.HasSelection() {
		t.Fatal("new press should collapse the old selection")
	}
	if got := s.SelectionText(); got != "" {
		t.Fatalf("SelectionText after re-press = %q, want empty", got)
	}
}

func TestReflowClearsSelection(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	s.SelectTo(wx1+1, (wy0+wy1)/2)
	if err := s.Reflow(800); err != nil {
		t.Fatalf("Reflow: %v", err)
	}
	if s.HasSelection() {
		t.Fatal("reflow should clear selection whose word positions died")
	}
}

func TestPaintSelectionHighlight(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	s.SelectTo(wx1+1, (wy0+wy1)/2)
	dl := s.Paint(1).Build(1)
	if n := countFills(t, dl, frame.RGB(178, 215, 255)); n == 0 {
		t.Fatal("no selection highlight fill in display list")
	}
	runes := textRunesIn(t, dl, frame.RGB(0, 0, 0))
	if string(runes) == "" {
		t.Fatal("selection paint dropped the text runs")
	}
}

func TestPaintWithoutSelectionNoHighlight(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	dl := s.Paint(1).Build(1)
	if n := countFills(t, dl, frame.RGB(178, 215, 255)); n != 0 {
		t.Fatalf("unselected page painted %d highlight fills", n)
	}
}

func TestSelectionBoundsUnion(t *testing.T) {
	s := newFocusSession(t, selParagraphHTML)
	if _, ok := s.SelectionBounds(); ok {
		t.Fatal("SelectionBounds ok with no selection")
	}
	hx0, hy0, _, hy1 := selWord(t, s, "hello")
	_, wy0, wx1, wy1 := selWord(t, s, "world")
	s.SelectAt(hx0-1, (hy0+hy1)/2)
	s.SelectTo(wx1+1, (wy0+wy1)/2)
	r, ok := s.SelectionBounds()
	if !ok {
		t.Fatal("SelectionBounds not ok after drag")
	}
	if float32(r.X0) > hx0 || float32(r.Y0) > hy0+1 || float32(r.X1) < wx1 || float32(r.Y1) < wy1-1 {
		t.Fatalf("bounds %+v do not cover hello->world", r)
	}
}

// qc:BR-108
func TestShiftArrowExtendsPageSelectionByRuneAndLine(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin:0;">`+
		`<p style="font-size:16px;">alpha beta<br>gamma delta</p></body></html>`)
	if s.HasSelection() {
		t.Fatal("fresh page already has a selection")
	}
	for i := 0; i < 5; i++ {
		if !s.ExtendSelectionByKey(engine.SelectRight) {
			t.Fatalf("Shift+Right %d did not move the selection", i)
		}
	}
	if got := s.SelectionText(); got != "alpha" {
		t.Fatalf("selection after five right steps = %q, want alpha", got)
	}
	if !s.ExtendSelectionByKey(engine.SelectLeft) {
		t.Fatal("Shift+Left did not shrink the selection")
	}
	if got := s.SelectionText(); got != "alph" {
		t.Fatalf("selection after one left step = %q, want alph", got)
	}
	if !s.ExtendSelectionByKey(engine.SelectRight) {
		t.Fatal("Shift+Right did not restore the fifth rune")
	}
	if !s.ExtendSelectionByKey(engine.SelectDown) {
		t.Fatal("Shift+Down did not move to the second line")
	}
	if got := s.SelectionText(); got != "alpha beta\ngamma" {
		t.Fatalf("selection after down = %q, want the first line plus gamma", got)
	}
	if !s.ExtendSelectionByKey(engine.SelectUp) {
		t.Fatal("Shift+Up did not return to the first line")
	}
	if got := s.SelectionText(); got != "alpha" {
		t.Fatalf("selection after up = %q, want alpha", got)
	}

	if !s.ClearSelection() {
		t.Fatal("ClearSelection reported no selection")
	}
	if s.ExtendSelectionByKey(engine.SelectLeft) {
		t.Fatal("Shift+Left at the document start moved the selection")
	}
	if !s.SelectAll() {
		t.Fatal("SelectAll found no text")
	}
	if s.ExtendSelectionByKey(engine.SelectRight) {
		t.Fatal("Shift+Right at the document end moved the selection")
	}
	if got := s.SelectionText(); got != "alpha beta\ngamma delta" {
		t.Fatalf("full selection = %q, want the whole paragraph", got)
	}
}

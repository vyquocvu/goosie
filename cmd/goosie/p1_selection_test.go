package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
)

func newSelectionTestPath(t *testing.T, html string, opts ...engine.Option) (*framePath, *engine.Session) {
	t.Helper()
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(html, nil, 800, opts...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetURL("https://selection.example.test/")
	tab.SetSession(sess)
	return f, sess
}

// selectionWordRect returns the border rect of the last laid-out word box with
// this text, matching the document-order walk the engine selects by.
func selectionWordRect(t *testing.T, s *engine.Session, text string) frame.Rect {
	t.Helper()
	var out frame.Rect
	found := false
	for i := range s.Arena.Objects {
		obj := &s.Arena.Objects[i]
		if obj.Node != nil && obj.Node.Type == 2 && obj.Node.DataContent == text {
			x0, y0, x1, y1 := obj.BorderRect()
			out = frame.Rect{X0: int32(x0), Y0: int32(y0), X1: int32(x1), Y1: int32(y1)}
			found = true
		}
	}
	if !found {
		t.Fatalf("word %q was not laid out", text)
	}
	return out
}

func paintsSelectionHighlight(s *engine.Session) bool {
	for _, cmd := range s.Paint(1).All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == frame.RGB(178, 215, 255) {
			return true
		}
	}
	return false
}

func clipboardHTML(cb interface{ Read() string }) string {
	if rich, ok := cb.(interface{ ReadHTML() string }); ok {
		return rich.ReadHTML()
	}
	return ""
}

// qc:BR-108
//
// A mouse drag selects the words under the pointer and paints the highlight,
// and Shift+Arrow then continues that selection from the pressed anchor: right
// grows one rune at a time, left shrinks, and no form control steals focus.
func TestMouseDragAndShiftArrowSelectPageText(t *testing.T) {
	f, sess := newSelectionTestPath(t,
		`<html><body style="margin:0;"><p style="font-size:16px;">hello brave world</p></body></html>`)
	hello := selectionWordRect(t, sess, "hello")
	world := selectionWordRect(t, sess, "world")
	midY := (hello.Y0 + hello.Y1) / 2

	if !f.dragSelect(surface.PointerPress, hello.X0-1, midY) {
		t.Fatal("press was not taken by the selection gesture")
	}
	if !f.dragSelect(surface.PointerMotion, world.X1+1, midY) {
		t.Fatal("drag motion was not taken by the selection gesture")
	}
	if !f.dragSelect(surface.PointerRelease, world.X1+1, midY) {
		t.Fatal("release was not taken by the selection gesture")
	}
	if got := sess.SelectionText(); got != "hello brave world" {
		t.Fatalf("mouse selection = %q, want hello brave world", got)
	}
	if _, ok := sess.SelectionBounds(); !ok {
		t.Fatal("mouse selection has no damage bounds")
	}
	if !paintsSelectionHighlight(sess) {
		t.Fatal("mouse selection painted no highlight")
	}

	if !sess.ClearSelection() {
		t.Fatal("ClearSelection reported no selection")
	}
	for i, want := range []string{"h", "he", "hel"} {
		if !f.contentKey(0xF703, surface.ModShift) {
			t.Fatalf("Shift+Right %d was not handled", i)
		}
		if got := sess.SelectionText(); got != want {
			t.Fatalf("selection after Shift+Right %d = %q, want %q", i, got, want)
		}
	}
	if !f.contentKey(0xF702, surface.ModShift) {
		t.Fatal("Shift+Left was not handled")
	}
	if got := sess.SelectionText(); got != "he" {
		t.Fatalf("selection after Shift+Left = %q, want he", got)
	}
	if sess.Focused() != nil {
		t.Fatalf("page selection focused %q", sess.Focused().Data)
	}
}

// qc:BR-109
//
// Copying a page selection through the shortcut's handler puts exactly the
// visible text on the clipboard with no markup, and pasting it into a form
// control yields that text unchanged.
func TestPageSelectionCopyIsPlainTextAndPastesIntoAControl(t *testing.T) {
	cb := platform.NewClipboard()
	if cb == nil {
		t.Skip("platform clipboard unavailable in test environment")
	}
	f, sess := newSelectionTestPath(t,
		`<html><body style="margin:0;">`+
			`<p style="font-size:16px;">copy me exactly</p>`+
			`<input id="dest" value=""></body></html>`)

	copyRect := selectionWordRect(t, sess, "copy")
	exactly := selectionWordRect(t, sess, "exactly")
	midY := (copyRect.Y0 + copyRect.Y1) / 2
	sess.SelectAt(float32(copyRect.X0-1), float32(midY))
	sess.SelectTo(float32(exactly.X1+1), float32(midY))
	if got := sess.SelectionText(); got != "copy me exactly" {
		t.Fatalf("setup selection = %q, want copy me exactly", got)
	}

	// copySelection is the handler the Cmd+C shortcut and the context menu
	// both call for a page selection.
	if !f.copySelection() {
		t.Fatal("copy declined an active page selection")
	}
	if got := cb.Read(); got != "copy me exactly" {
		t.Fatalf("clipboard text = %q, want copy me exactly", got)
	}
	if got := clipboardHTML(cb); got != "" {
		t.Fatalf("page copy carried markup %q", got)
	}

	dest := sess.Doc.ElementByID("dest")
	if !sess.FocusNode(dest) {
		t.Fatal("could not focus the paste target")
	}
	if !f.contentKey('v', surface.ModCommand) {
		t.Fatal("Cmd+V was not handled by the focused control")
	}
	if got := dest.InputControlValue(); got != "copy me exactly" {
		t.Fatalf("pasted value = %q, want copy me exactly", got)
	}
}

// qc:BR-110
//
// Cmd+A in a text control selects that control's value and nothing else; the
// page selection stays empty so the paragraph outside it is not copied.
func TestCmdASelectsOnlyTheFocusedTextControl(t *testing.T) {
	f, sess := newSelectionTestPath(t,
		`<html><body><p>outside words</p><input id="field" value="inside value"></body></html>`)
	if !sess.FocusNode(sess.Doc.ElementByID("field")) {
		t.Fatal("could not focus the text control")
	}
	if !f.contentKey('a', surface.ModCommand) {
		t.Fatal("Cmd+A was not handled in a text control")
	}
	if got := sess.FocusedText(); got != "inside value" {
		t.Fatalf("selected control text = %q, want inside value", got)
	}
	if got := sess.SelectionText(); got != "" {
		t.Fatalf("Cmd+A in a control also selected page text %q", got)
	}
}

// qc:BR-110
//
// Cmd+A in a contenteditable selects the editable subtree only: the script's
// getSelection excludes sibling page text, and the engine's page selection
// stays empty.
func TestCmdASelectsOnlyTheContentEditableSubtree(t *testing.T) {
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://selection.example.test/"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })
	f, sess := newSelectionTestPath(t,
		`<html><body><p id="outside">outside words</p>`+
			`<div id="ed" contenteditable="true">inside <b>bold</b> text</div></body></html>`,
		engine.WithJS(rt))
	if !sess.FocusNode(sess.Doc.ElementByID("ed")) {
		t.Fatal("could not focus the contenteditable")
	}
	if !f.contentKey('a', surface.ModCommand) {
		t.Fatal("Cmd+A was not handled in a contenteditable")
	}
	got, err := sess.EvaluateJS(`getSelection().toString()`)
	if err != nil {
		t.Fatalf("read selection: %v", err)
	}
	if got != "inside bold text" {
		t.Fatalf("contenteditable selection = %q, want inside bold text", got)
	}
	if strings.Contains(got, "outside") {
		t.Fatalf("contenteditable selection escaped into sibling text: %q", got)
	}
	if sel := sess.SelectionText(); sel != "" {
		t.Fatalf("Cmd+A in a contenteditable also selected page text %q", sel)
	}
}

package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestSessionInsertText(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<input id="inp" type="text" value="hello " maxlength="20">
</body></html>`

	sess, rt := newJSSession(t, html)
	_ = rt

	// Focus the input control
	if !focusCenter(t, sess, "inp") {
		t.Fatalf("failed to focus inp")
	}

	// Insert text at caret (which defaults to end)
	ok := sess.InsertText("world!")
	if !ok {
		t.Fatalf("InsertText returned false")
	}

	val := sess.FocusedText()
	if val != "hello world!" {
		t.Errorf("expected 'hello world!', got %q", val)
	}

	// Test maxlength enforcement: length is 12, max is 20 -> 8 chars fit
	sess.InsertText("1234567890EXTRA")
	valWithMax := sess.FocusedText()
	if len(valWithMax) > 20 {
		t.Errorf("expected max length <= 20, got length %d (%q)", len(valWithMax), valWithMax)
	}
	if !strings.HasPrefix(valWithMax, "hello world!12345678") {
		t.Errorf("expected clamped insertion, got %q", valWithMax)
	}
}

func TestSessionSelectAllAndClear(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<p id="p1">First paragraph of text</p>
<p id="p2">Second line of content</p>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	if sess.HasSelection() {
		t.Fatalf("expected no initial selection")
	}

	ok := sess.SelectAll()
	if !ok {
		t.Fatalf("SelectAll returned false")
	}

	if !sess.HasSelection() {
		t.Fatalf("expected active selection after SelectAll")
	}

	selText := sess.SelectionText()
	if !strings.Contains(selText, "First paragraph") || !strings.Contains(selText, "Second line") {
		t.Errorf("expected full document text selected, got %q", selText)
	}

	cleared := sess.ClearSelection()
	if !cleared {
		t.Fatalf("ClearSelection returned false")
	}

	if sess.HasSelection() {
		t.Fatalf("expected no selection after ClearSelection")
	}
}

func TestSessionSelectAllControl(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body>
<input id="inp" type="text" value="some sample input text">
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	if !focusCenter(t, sess, "inp") {
		t.Fatalf("failed to focus inp")
	}

	ok := sess.SelectAllControl()
	if !ok {
		t.Fatalf("SelectAllControl failed")
	}

	txt := sess.FocusedText()
	if txt != "some sample input text" {
		t.Errorf("expected 'some sample input text', got %q", txt)
	}
}

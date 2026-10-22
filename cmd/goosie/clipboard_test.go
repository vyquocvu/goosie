package main

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/surface"
)

func TestContentKeyCmdPaste(t *testing.T) {
	cb := platform.NewClipboard()
	if cb == nil {
		t.Skip("platform clipboard unavailable in test environment")
	}

	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	html := `<!DOCTYPE html>
<html><body>
<input id="inp" type="text" value="start:">
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/")
	tab.SetSession(sess)

	// Focus the input
	var inpX, inpY int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "inp" {
			x0, y0, x1, y1 := obj.BorderRect()
			inpX = int32((x0 + x1) / 2)
			inpY = int32((y0 + y1) / 2)
			break
		}
	}

	if !f.focusClick(inpX, inpY) {
		t.Fatal("focusClick failed to focus input")
	}

	// Write text to platform clipboard
	cb.Write("clipdata")

	// Trigger Cmd+V
	handled := f.contentKey('v', surface.ModCommand)
	if !handled {
		t.Fatalf("contentKey Cmd+V was not handled")
	}

	val := sess.FocusedText()
	if val != "start:clipdata" {
		t.Errorf("expected 'start:clipdata', got %q", val)
	}
}

func TestContentKeyCmdSelectAll(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	html := `<!DOCTYPE html>
<html><body>
<p>Document text for selection test</p>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/")
	tab.SetSession(sess)

	// No control focused: Cmd+A selects document
	handled := f.contentKey('a', surface.ModCommand)
	if !handled {
		t.Fatalf("contentKey Cmd+A was not handled")
	}

	if !sess.HasSelection() {
		t.Errorf("expected document selection after Cmd+A")
	}
	selText := sess.SelectionText()
	if selText != "Document text for selection test" {
		t.Errorf("expected full text selected, got %q", selText)
	}
}

func TestContentKeyCmdCopy(t *testing.T) {
	cb := platform.NewClipboard()
	if cb == nil {
		t.Skip("platform clipboard unavailable in test environment")
	}

	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	html := `<!DOCTYPE html>
<html><body>
<input id="inp" type="text" value="secret-to-copy">
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/")
	tab.SetSession(sess)

	var inpX, inpY int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "inp" {
			x0, y0, x1, y1 := obj.BorderRect()
			inpX = int32((x0 + x1) / 2)
			inpY = int32((y0 + y1) / 2)
			break
		}
	}

	if !f.focusClick(inpX, inpY) {
		t.Fatal("focusClick failed to focus input")
	}

	// Trigger Cmd+C
	handled := f.contentKey('c', surface.ModCommand)
	if !handled {
		t.Fatalf("contentKey Cmd+C was not handled")
	}

	clipGot := cb.Read()
	if clipGot != "secret-to-copy" {
		t.Errorf("clipboard expected 'secret-to-copy', got %q", clipGot)
	}
}

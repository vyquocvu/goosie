package main

import (
	"context"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

type testClipboardProvider struct {
	text string
	html string
}

func (c *testClipboardProvider) Read() string                { return c.text }
func (c *testClipboardProvider) ReadHTML() string            { return c.html }
func (c *testClipboardProvider) Write(text string)           { c.WriteRich(text, "") }
func (c *testClipboardProvider) WriteRich(text, html string) { c.text, c.html = text, html }

func TestDocumentClipboardAPIUsesHostProvider(t *testing.T) {
	const pageURL = "https://clipboard.example.test/"
	client := &recordingClient{docs: map[string]string{
		pageURL: `<!doctype html><html><body><button id="copy">Copy</button><script>
			document.getElementById('copy').addEventListener('click', function() {
				var item = new ClipboardItem({
					'text/plain': Promise.resolve(new Blob(['page text'], {type: 'text/plain'})),
					'text/html': Promise.resolve(new Blob(['<b>page text</b>'], {type: 'text/html'}))
				});
				navigator.clipboard.write([item]).then(function() {
					return navigator.clipboard.read();
				}).then(function(items) {
					return items[0].getType('text/html');
				}).then(function(blob) {
					return blob.text();
				}).then(function(html) {
					document.body.setAttribute('data-clipboard', html);
				});
			});
		</script></body></html>`,
	}}
	clipboard := &testClipboardProvider{text: "initial"}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("create fonts: %v", err)
	}
	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, pageURL, 800, 600, 1, t.TempDir(), false, false, clipboard)
	if err != nil {
		t.Fatalf("loadURLCtx: %v", err)
	}
	defer sess.Close()
	var clickX, clickY float32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "copy" {
			x0, y0, x1, y1 := obj.BorderRect()
			clickX, clickY = (x0+x1)/2, (y0+y1)/2
			break
		}
	}
	if !sess.ClickAt(clickX, clickY) {
		t.Fatalf("click at clipboard button (%v,%v) was not handled", clickX, clickY)
	}
	sess.PumpJS(800)
	if clipboard.text != "page text" || clipboard.html != "<b>page text</b>" {
		t.Fatalf("host clipboard = %q / %q, want plain and HTML page text", clipboard.text, clipboard.html)
	}
	got, err := sess.EvaluateJS(`document.body.getAttribute('data-clipboard')`)
	if err != nil {
		t.Fatalf("read page clipboard result: %v", err)
	}
	if got != "<b>page text</b>" {
		t.Fatalf("page clipboard HTML round trip = %q, want <b>page text</b>", got)
	}
}

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

func TestContentKeyCmdCopyAndCutDispatchEditingClipboardEvents(t *testing.T) {
	cb := platform.NewClipboard()
	if cb == nil {
		t.Skip("platform clipboard unavailable in test environment")
	}
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<!DOCTYPE html><html><body><input id="inp" type="text" value="selected"></body></html>`, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/")
	tab.SetSession(sess)
	field := sess.Doc.ElementByID("inp")
	if !sess.FocusNode(field) || !sess.SelectAllControl() {
		t.Fatal("text input selection setup failed")
	}
	if !f.contentKey('c', surface.ModCommand) {
		t.Fatal("Cmd+C was not handled for selected text")
	}
	if got := cb.Read(); got != "selected" {
		t.Fatalf("clipboard after Cmd+C = %q, want selected", got)
	}
	if !sess.SelectAllControl() {
		t.Fatal("could not select text before Cmd+X")
	}
	if !f.contentKey('x', surface.ModCommand) {
		t.Fatal("Cmd+X was not handled for selected text")
	}
	if got := field.InputControlValue(); got != "" {
		t.Fatalf("input value after Cmd+X = %q, want empty", got)
	}
	if got := cb.Read(); got != "selected" {
		t.Fatalf("clipboard after Cmd+X = %q, want selected", got)
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

func TestContentKeyShiftArrowSelectsAndReplacesInputText(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	sess, err := engine.NewSession(`<html><body><input id="field" value="abcd"></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.SetSession(sess)
	var x, y int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "field" {
			x0, y0, x1, y1 := obj.BorderRect()
			x, y = int32((x0+x1)/2), int32((y0+y1)/2)
			break
		}
	}
	if x == 0 && y == 0 {
		t.Fatal("input was not laid out")
	}
	if !f.focusClick(x, y) {
		t.Fatal("focusClick did not focus the input")
	}
	if !f.contentKey(0xF702, surface.ModShift) { // Shift+Left
		t.Fatal("contentKey did not handle Shift+Left")
	}
	if got := sess.FocusedText(); got != "d" {
		t.Fatalf("selected text = %q, want d", got)
	}
	if !f.contentKey('X', 0) {
		t.Fatal("contentKey did not handle replacement text")
	}
	if got := sess.Focused().InputControlValue(); got != "abcX" {
		t.Fatalf("value after replacement = %q, want abcX", got)
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
	if !sess.SelectAllControl() {
		t.Fatal("could not select input text before Cmd+C")
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

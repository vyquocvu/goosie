package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-111
// TestIMECompositionPaintsSelectionHighlight verifies that when IME composition
// is active, the marked range is painted with a selection highlight. This pins
// the fix for GOOSIE-IMESELECTION-001: Chromium shows the marked passage as a
// live selection tinted #b3d7ff (178, 215, 255), and Goosie must do the same.
func TestIMECompositionPaintsSelectionHighlight(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin:0">
<input id="a" value="hello" style="font:16px monospace">
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	// Focus the input at position 0.
	sess.FocusControl(10, 10)

	// Start IME composition with marked text "ti".
	if !sess.SetMarked("ti") {
		t.Fatal("SetMarked reported no change")
	}

	// Paint and look for the selection highlight color.
	list := sess.Paint(1)
	dl := list.Build(1)
	foundHighlight := false
	for _, c := range dl.All() {
		// Selection highlight is RGB(178, 215, 255).
		if c.Kind == paint.CmdFill && c.Color.R() == 178 && c.Color.G() == 215 && c.Color.B() == 255 {
			foundHighlight = true
			break
		}
	}
	if !foundHighlight {
		t.Error("no selection highlight found in display list during IME composition")
	}

	// Commit the text and verify the highlight is gone.
	if !sess.CommitText("ti") {
		t.Fatal("CommitText reported no change")
	}

	list = sess.Paint(1)
	dl = list.Build(1)
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 178 && c.Color.G() == 215 && c.Color.B() == 255 {
			t.Error("selection highlight should not be present after commit")
			break
		}
	}
}

// qc:BR-111
// TestIMECompositionContentEditablePaintsSelectionHighlight verifies that when
// IME composition is active in a contenteditable, the marked range is painted
// with a selection highlight.
func TestIMECompositionContentEditablePaintsSelectionHighlight(t *testing.T) {
	rt, err := js.New(js.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := engine.NewSession(`<html><body style="margin:0">
<div id="c" contenteditable style="font:16px monospace">hello</div>
</body></html>`, nil, 800, engine.WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	defer sess.Close()

	// Focus the contenteditable.
	edit := sess.Doc.ElementByID("c")
	if !sess.FocusNode(edit) || !sess.FocusedEditable() {
		t.Fatal("contenteditable host should be focused and editable")
	}

	// Position the caret at offset 0 using JS.
	if _, err := sess.EvaluateJS(`(function(){
		var t = document.getElementById("c").firstChild;
		var r = document.createRange();
		r.setStart(t, 0);
		r.collapse(true);
		var q = getSelection();
		q.removeAllRanges();
		q.addRange(r);
	})()`); err != nil {
		t.Fatalf("position composition caret: %v", err)
	}

	// Start IME composition with marked text "ti".
	if !sess.SetMarked("ti") {
		t.Fatal("SetMarked reported no change")
	}

	// Paint and look for the selection highlight color.
	list := sess.Paint(1)
	dl := list.Build(1)
	foundHighlight := false
	for _, c := range dl.All() {
		// Selection highlight is RGB(178, 215, 255).
		if c.Kind == paint.CmdFill && c.Color.R() == 178 && c.Color.G() == 215 && c.Color.B() == 255 {
			foundHighlight = true
			break
		}
	}
	if !foundHighlight {
		t.Error("no selection highlight found in display list during IME composition in contenteditable")
	}
}

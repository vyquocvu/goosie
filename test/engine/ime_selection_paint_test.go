package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/paint"
)

// qc:BR-111
func TestIMECompositionPaintsSelectionHighlight(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin:0">
<input id="a" value="hello" style="font:16px monospace">
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	sess.FocusControl(10, 10)

	if !sess.SetMarked("ti") {
		t.Fatal("SetMarked reported no change")
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	foundHighlight := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 178 && c.Color.G() == 215 && c.Color.B() == 255 {
			foundHighlight = true
			break
		}
	}
	if !foundHighlight {
		t.Error("no selection highlight found in display list during IME composition")
	}

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

	edit := sess.Doc.ElementByID("c")
	if !sess.FocusNode(edit) || !sess.FocusedEditable() {
		t.Fatal("contenteditable host should be focused and editable")
	}

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

	if !sess.SetMarked("ti") {
		t.Fatal("SetMarked reported no change")
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	foundHighlight := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 178 && c.Color.G() == 215 && c.Color.B() == 255 {
			foundHighlight = true
			break
		}
	}
	if !foundHighlight {
		t.Error("no selection highlight found in display list during IME composition in contenteditable")
	}
}

package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func TestDialog_TopmostModalClosing(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <dialog id="d1"><p>First dialog</p></dialog>
  <dialog id="d2"><p>Second dialog</p></dialog>
</body>
</html>`

	sess, rt := newJSSession(t, html)
	_ = rt

	d1 := sess.Doc.ElementByID("d1")
	d2 := sess.Doc.ElementByID("d2")
	if d1 == nil || d2 == nil {
		t.Fatal("dialog elements not found")
	}

	// Open both as modal
	_, err := rt.Evaluate("document.getElementById('d1').showModal()")
	if err != nil {
		t.Fatalf("showModal d1: %v", err)
	}
	_, err = rt.Evaluate("document.getElementById('d2').showModal()")
	if err != nil {
		t.Fatalf("showModal d2: %v", err)
	}

	if !d1.HasAttribute("open") || !d1.IsModal {
		t.Fatal("d1 should be open modal")
	}
	if !d2.HasAttribute("open") || !d2.IsModal {
		t.Fatal("d2 should be open modal")
	}

	// Close topmost modal (should be d2)
	closed := sess.CloseTopmostModalDialog()
	if !closed {
		t.Fatal("expected CloseTopmostModalDialog to return true")
	}
	if d2.HasAttribute("open") || d2.IsModal {
		t.Fatal("d2 should have been closed")
	}
	if !d1.HasAttribute("open") || !d1.IsModal {
		t.Fatal("d1 should remain open")
	}

	// Close next topmost modal (should be d1)
	closed = sess.CloseTopmostModalDialog()
	if !closed {
		t.Fatal("expected second CloseTopmostModalDialog to return true")
	}
	if d1.HasAttribute("open") || d1.IsModal {
		t.Fatal("d1 should have been closed")
	}

	// Calling again when none open returns false
	closed = sess.CloseTopmostModalDialog()
	if closed {
		t.Fatal("expected CloseTopmostModalDialog on empty top layer to return false")
	}
}

func TestDialog_EscapeKeyInteraction(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <dialog id="modal"><p>Modal</p></dialog>
  <script>
    window.events = [];
    var dlg = document.getElementById('modal');
    dlg.addEventListener('cancel', function(e) { window.events.push('cancel'); });
    dlg.addEventListener('close', function(e) { window.events.push('close'); });
  </script>
</body>
</html>`

	sess, rt := newJSSession(t, html)
	_, err := rt.Evaluate("document.getElementById('modal').showModal()")
	if err != nil {
		t.Fatalf("showModal: %v", err)
	}

	dlg := sess.Doc.ElementByID("modal")
	if !dlg.HasAttribute("open") || !dlg.IsModal {
		t.Fatal("modal should be open")
	}

	// Press Escape
	sess.PressKey("Escape", false, false, false, false)

	if dlg.HasAttribute("open") || dlg.IsModal {
		t.Fatal("modal should be closed after Escape key")
	}

	evs, err := rt.Evaluate("window.events.join(',')")
	if err != nil {
		t.Fatalf("Evaluate events: %v", err)
	}
	if evs != "cancel,close" {
		t.Fatalf("expected events 'cancel,close', got %q", evs)
	}
}

func TestDialog_EscapePreventDefaultBlocksClose(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <dialog id="modal"><p>Unclosable Modal</p></dialog>
  <script>
    var dlg = document.getElementById('modal');
    dlg.addEventListener('cancel', function(e) {
      e.preventDefault();
    });
  </script>
</body>
</html>`

	sess, rt := newJSSession(t, html)
	_, err := rt.Evaluate("document.getElementById('modal').showModal()")
	if err != nil {
		t.Fatalf("showModal: %v", err)
	}

	dlg := sess.Doc.ElementByID("modal")
	// Press Escape
	sess.PressKey("Escape", false, false, false, false)

	if !dlg.HasAttribute("open") || !dlg.IsModal {
		t.Fatal("modal should remain open because cancel event was preventDefault-ed")
	}
}

func TestDialog_EditEscapeDirectly(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <dialog id="modal" open><p>Modal</p></dialog>
</body>
</html>`
	sess, rt := newJSSession(t, html)
	_ = rt
	dlg := sess.Doc.ElementByID("modal")
	dlg.IsModal = true

	changed := sess.Edit(engine.EditEscape, 0x1b)
	if !changed {
		t.Fatal("expected EditEscape to return changed=true")
	}
	if dlg.HasAttribute("open") || dlg.IsModal {
		t.Fatal("expected dialog to be closed by EditEscape")
	}
}

func TestDialog_FormMethodDialog(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
  <dialog id="dlg">
    <form id="f" method="dialog">
      <input type="text" name="val" value="test">
      <button type="submit" id="btn">Submit</button>
    </form>
  </dialog>
</body>
</html>`

	sess, rt := newJSSession(t, html)
	_, err := rt.Evaluate("document.getElementById('dlg').showModal()")
	if err != nil {
		t.Fatalf("showModal: %v", err)
	}

	dlg := sess.Doc.ElementByID("dlg")
	form := sess.Doc.ElementByID("f")
	if dlg == nil || form == nil {
		t.Fatal("dialog or form not found")
	}

	closeFired := false
	dom.AddEventListener(dlg, "close", func(e *dom.Event) {
		closeFired = true
	}, false)

	sess.SubmitForm(form)

	if dlg.HasAttribute("open") || dlg.IsModal {
		t.Fatal("dialog should be closed after method=dialog submission")
	}
	if !closeFired {
		t.Fatal("expected close event to fire on dialog")
	}
	if sub, ok := sess.PopSubmittedForm(); ok || sub != nil {
		t.Fatalf("expected no submittedForm for method=dialog, got %v", sub)
	}
}

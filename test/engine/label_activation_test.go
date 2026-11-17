package engine_test

import (
	"testing"
)

func TestLabelClickTogglesAndFocusesItsCheckbox(t *testing.T) {
	s := newFormSession(t, `<!doctype html><body style="margin:0">
		<input id="box" type="checkbox" style="position:absolute;left:10px;top:10px">
		<label id="words" for="box" style="position:absolute;left:10px;top:40px;width:120px;height:24px">Agree</label>
	</body>`)
	t.Cleanup(func() { s.Close() })
	box := s.Doc.ElementByID("box")
	clickCenter(t, s, "words")
	if !box.InputChecked() {
		t.Fatal("clicking the label left the checkbox unchecked")
	}
	if s.Focused() != box {
		t.Fatal("clicking the label did not focus the checkbox")
	}
	clickCenter(t, s, "words")
	if box.InputChecked() {
		t.Fatal("a second label click did not clear the checkbox")
	}
}

func TestWrappedLabelClickTogglesTheDescendantCheckbox(t *testing.T) {
	s := newFormSession(t, `<!doctype html><body style="margin:0">
		<label style="position:absolute;left:8px;top:8px;display:block;width:200px;height:40px">
			<input id="box" type="checkbox">
			<span id="words" style="display:inline-block;width:80px;height:20px">Agree</span>
		</label>
	</body>`)
	t.Cleanup(func() { s.Close() })
	clickCenter(t, s, "words")
	if !s.Doc.ElementByID("box").InputChecked() {
		t.Fatal("clicking words inside the label left the checkbox unchecked")
	}
}

func TestClickOnTheCheckboxInsideALabelTogglesOnce(t *testing.T) {
	s := newFormSession(t, `<!doctype html><body style="margin:0">
		<label style="position:absolute;left:8px;top:8px">
			<input id="box" type="checkbox" style="width:20px;height:20px">
		</label>
	</body>`)
	t.Cleanup(func() { s.Close() })
	if !clickCenter(t, s, "box") {
		t.Fatal("checkbox has no box")
	}
	if !s.Doc.ElementByID("box").InputChecked() {
		t.Fatal("clicking the checkbox inside its label left it unchecked")
	}
}

func TestLabelClickDoesNotToggleADisabledCheckbox(t *testing.T) {
	s := newFormSession(t, `<!doctype html><body style="margin:0">
		<input id="box" type="checkbox" disabled style="position:absolute;left:10px;top:10px">
		<label id="words" for="box" style="position:absolute;left:10px;top:40px;width:120px;height:24px">No</label>
	</body>`)
	t.Cleanup(func() { s.Close() })
	if !clickCenter(t, s, "words") {
		t.Fatal("label has no box")
	}
	if s.Doc.ElementByID("box").InputChecked() {
		t.Fatal("clicking the label toggled a disabled checkbox")
	}
}

package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

const fileInputHTML = `<html><body style="margin: 0">` +
	`<input id="upload" type="file" name="upload"></body></html>`

func TestClickFileInputRequestsPicker(t *testing.T) {
	s, _ := newJSSession(t, fileInputHTML)
	if !clickCenter(t, s, "upload") {
		t.Fatal("file input was not laid out")
	}
	node, ok := s.PopFilePickerRequest()
	if !ok || node != s.Doc.ElementByID("upload") {
		t.Fatalf("PopFilePickerRequest = (%v, %v), want the upload input", node, ok)
	}
	if _, ok := s.PopFilePickerRequest(); ok {
		t.Fatal("PopFilePickerRequest did not clear the request")
	}
}

func TestKeyboardActivationRequestsPicker(t *testing.T) {
	for _, key := range []string{"Enter", " "} {
		s, _ := newJSSession(t, fileInputHTML)
		input := s.Doc.ElementByID("upload")
		if !s.FocusNode(input) {
			t.Fatal("file input should receive focus")
		}
		if !s.PressKey(key, false, false, false, false) {
			t.Fatalf("%q did not activate the file input", key)
		}
		if node, ok := s.PopFilePickerRequest(); !ok || node != input {
			t.Fatalf("%q: PopFilePickerRequest = (%v, %v), want the upload input", key, node, ok)
		}
	}
}

func TestDisabledFileInputDoesNotRequestPicker(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0">`+
		`<input id="upload" type="file" disabled></body></html>`)
	clickCenter(t, s, "upload")
	if node, ok := s.PopFilePickerRequest(); ok {
		t.Fatalf("disabled file input requested a picker for %v", node)
	}
}

func TestScriptShowPickerRequestsHostPicker(t *testing.T) {
	s, _ := newJSSession(t, fileInputHTML)
	s.SetUserActivation(true)
	if _, err := s.EvaluateJS(`document.getElementById("upload").showPicker()`); err != nil {
		t.Fatalf("showPicker: %v", err)
	}
	s.SetUserActivation(false)
	node, ok := s.PopFilePickerRequest()
	if !ok || node != s.Doc.ElementByID("upload") {
		t.Fatalf("PopFilePickerRequest = (%v, %v), want the upload input", node, ok)
	}
}

func TestSetInputFilesPopulatesFileListAndFiresEvents(t *testing.T) {
	s, _ := newJSSession(t, fileInputHTML)
	if _, err := s.EvaluateJS(`
		window.events = [];
		var input = document.getElementById("upload");
		input.addEventListener("input", function() { events.push("input:" + input.files.length); });
		input.addEventListener("change", function() { events.push("change:" + input.files.length); });
		document.body.addEventListener("change", function(event) { events.push("bubbled:" + event.target.id); });
	`); err != nil {
		t.Fatal(err)
	}
	input := s.Doc.ElementByID("upload")
	if !s.SetInputFiles(input, []js.FileSelection{
		{Name: "a.txt", ContentType: "text/plain", Data: []byte("abc")},
	}) {
		t.Fatal("SetInputFiles rejected the file input")
	}
	if input.InputFileCount != 1 {
		t.Fatalf("InputFileCount = %d, want 1", input.InputFileCount)
	}
	got, err := s.EvaluateJS(`input.files.length + "|" + input.files[0].name + "|" + input.files[0].size + "|" + events.join(",")`)
	if err != nil {
		t.Fatal(err)
	}
	const want = "1|a.txt|3|input:1,change:1,bubbled:upload"
	if got != want {
		t.Fatalf("selection and events = %q, want %q", got, want)
	}

	if !s.SetInputFiles(input, nil) {
		t.Fatal("SetInputFiles rejected clearing the selection")
	}
	if input.InputFileCount != 0 {
		t.Fatalf("InputFileCount after clear = %d, want 0", input.InputFileCount)
	}
	got, err = s.EvaluateJS(`input.files.length + "|" + events.slice(3).join(",")`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0|input:0,change:0,bubbled:upload"; got != want {
		t.Fatalf("cleared selection and events = %q, want %q", got, want)
	}
}

func TestSetInputFilesRejectsNonFileControl(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0"><input id="name" type="text"></body></html>`)
	if s.SetInputFiles(s.Doc.ElementByID("name"), []js.FileSelection{{Name: "a.txt"}}) {
		t.Fatal("SetInputFiles accepted a text input")
	}
}

func TestPickedFilesAreUploadedByMultipartForm(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0">`+
		`<form id="f" method="post" enctype="multipart/form-data" action="/upload">`+
		`<input id="upload" type="file" name="upload">`+
		`<button id="send" type="submit">Send</button></form></body></html>`)
	input := s.Doc.ElementByID("upload")
	if !s.SetInputFiles(input, []js.FileSelection{
		{Name: "a.txt", ContentType: "text/plain", Data: []byte("abc")},
	}) {
		t.Fatal("SetInputFiles rejected the file input")
	}
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("form did not submit")
	}
	if !strings.Contains(sub.Encoded, `filename="a.txt"`) || !strings.Contains(sub.Encoded, "abc") {
		t.Fatalf("multipart body did not carry the picked file:\n%s", sub.Encoded)
	}
	if !strings.HasPrefix(sub.Enctype, "multipart/form-data;") {
		t.Fatalf("Enctype = %q, want multipart/form-data", sub.Enctype)
	}
}

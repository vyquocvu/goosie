package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func newFileSelectionRuntime(t *testing.T) (*Runtime, *dom.Document, *dom.Node) {
	t.Helper()
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Body = html, body
	runtime, err := New(Options{DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close() })
	input := doc.NewElement("input")
	input.SetAttribute("type", "file")
	input.SetAttribute("id", "upload")
	body.AppendChild(input)
	return runtime, doc, input
}

func TestSetSelectedFilesPopulatesFileList(t *testing.T) {
	runtime, _, input := newFileSelectionRuntime(t)
	if !runtime.SetSelectedFiles(input, []FileSelection{
		{Name: "a.txt", ContentType: "text/plain", Data: []byte("abc")},
		{Name: "b.bin", ContentType: "application/octet-stream", Data: []byte{0, 1}},
	}) {
		t.Fatal("SetSelectedFiles reported the file input was not a file control")
	}
	if input.InputFileCount != 2 {
		t.Fatalf("InputFileCount = %d, want 2", input.InputFileCount)
	}
	got, err := runtime.Evaluate(`(function() {
		var i = document.getElementById('upload');
		var f = i.files;
		return [f instanceof FileList, f.length, f[0].name, f[0].type, f[0].size,
			f[0] instanceof File, f.item(1).name,
			Array.from(f).map(function(x) { return x.name; }).join(',')].join('|');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	const want = "true|2|a.txt|text/plain|3|true|b.bin|a.txt,b.bin"
	if got != want {
		t.Fatalf("FileList = %q, want %q", got, want)
	}

	if !runtime.SetSelectedFiles(input, nil) {
		t.Fatal("clearing a file input should succeed")
	}
	if input.InputFileCount != 0 {
		t.Fatalf("InputFileCount after clear = %d, want 0", input.InputFileCount)
	}
	got, err = runtime.Evaluate(`document.getElementById('upload').files.length`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0" {
		t.Fatalf("FileList length after clear = %q, want 0", got)
	}
}

func TestSetSelectedFilesRejectsNonFileControls(t *testing.T) {
	runtime, doc, _ := newFileSelectionRuntime(t)
	text := doc.NewElement("input")
	text.SetAttribute("type", "text")
	doc.Body.AppendChild(text)
	if runtime.SetSelectedFiles(text, []FileSelection{{Name: "a.txt", Data: []byte("a")}}) {
		t.Fatal("SetSelectedFiles accepted a text input")
	}
	if runtime.SetSelectedFiles(nil, []FileSelection{{Name: "a.txt"}}) {
		t.Fatal("SetSelectedFiles accepted a nil node")
	}
}

func TestShowPickerOnFileInputRecordsHostRequest(t *testing.T) {
	runtime, _, input := newFileSelectionRuntime(t)
	runtime.SetTransientUserActivation(true)
	if _, err := runtime.Evaluate(`document.getElementById('upload').showPicker()`); err != nil {
		t.Fatal(err)
	}
	node, ok := runtime.TakeFilePickerRequest()
	if !ok || node != input {
		t.Fatalf("TakeFilePickerRequest = (%v, %v), want the upload input", node, ok)
	}
	if _, ok := runtime.TakeFilePickerRequest(); ok {
		t.Fatal("TakeFilePickerRequest did not clear the request")
	}
}

func TestClickOnFileInputRecordsHostRequest(t *testing.T) {
	runtime, _, input := newFileSelectionRuntime(t)
	runtime.SetTransientUserActivation(true)
	got, err := runtime.Evaluate(`(function() {
		document.getElementById('upload').click();
		return navigator.userActivation.isActive;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "false" {
		t.Fatalf("file click left transient activation active: %q", got)
	}
	node, ok := runtime.TakeFilePickerRequest()
	if !ok || node != input {
		t.Fatalf("TakeFilePickerRequest = (%v, %v), want the upload input", node, ok)
	}
}

func TestClickOnFileInputWithoutActivationRecordsNothing(t *testing.T) {
	runtime, _, _ := newFileSelectionRuntime(t)
	if _, err := runtime.Evaluate(`document.getElementById('upload').click()`); err != nil {
		t.Fatal(err)
	}
	if node, ok := runtime.TakeFilePickerRequest(); ok {
		t.Fatalf("click without user activation recorded a picker request for %v", node)
	}
}

func TestHiddenFileInputPickerRequestFollowsActivation(t *testing.T) {
	for _, tc := range []struct {
		name string
		hide func(*dom.Node)
	}{
		{"self", func(input *dom.Node) { input.SetAttribute("style", "display:none") }},
		{"ancestor", func(input *dom.Node) { input.Parent.SetAttribute("style", "display:none") }},
	} {
		t.Run(tc.name+" with activation", func(t *testing.T) {
			runtime, _, input := newFileSelectionRuntime(t)
			tc.hide(input)
			runtime.SetTransientUserActivation(true)
			got, err := runtime.Evaluate(`(function() {
				document.getElementById('upload').showPicker();
				return navigator.userActivation.isActive;
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			if got != "false" {
				t.Fatalf("hidden showPicker left transient activation active: %q", got)
			}
			node, ok := runtime.TakeFilePickerRequest()
			if !ok || node != input {
				t.Fatalf("TakeFilePickerRequest = (%v, %v), want the upload input", node, ok)
			}
		})

		t.Run(tc.name+" without activation", func(t *testing.T) {
			runtime, _, input := newFileSelectionRuntime(t)
			tc.hide(input)
			got, err := runtime.Evaluate(`(function() {
				try { document.getElementById('upload').showPicker(); return 'none' }
				catch (e) { return e.name }
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			if got != "NotAllowedError" {
				t.Fatalf("hidden showPicker without activation = %q, want NotAllowedError", got)
			}
			if node, ok := runtime.TakeFilePickerRequest(); ok {
				t.Fatalf("hidden showPicker without activation recorded a request for %v", node)
			}
		})
	}
}

func TestHiddenSelectShowPickerThrowsNotSupported(t *testing.T) {
	runtime, doc, _ := newFileSelectionRuntime(t)
	selectNode := doc.NewElement("select")
	selectNode.SetAttribute("id", "choice")
	selectNode.SetAttribute("style", "display:none")
	doc.Body.AppendChild(selectNode)
	runtime.SetTransientUserActivation(true)
	got, err := runtime.Evaluate(`(function() {
		try { document.getElementById('choice').showPicker(); return 'none' }
		catch (e) { return e.name + '|' + navigator.userActivation.isActive }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "NotSupportedError|true" {
		t.Fatalf("hidden select showPicker = %q, want NotSupportedError|true", got)
	}
}

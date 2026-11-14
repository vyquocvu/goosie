package engine_test

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

type multipartPart struct {
	Name        string
	Filename    string
	ContentType string
	Body        string
}

func multipartParts(t *testing.T, contentType, body string) []multipartPart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("Content-Type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(strings.NewReader(body), params["boundary"])
	var parts []multipartPart
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part %q: %v", part.FormName(), err)
		}
		parts = append(parts, multipartPart{
			Name:        part.FormName(),
			Filename:    part.FileName(),
			ContentType: part.Header.Get("Content-Type"),
			Body:        string(data),
		})
	}
	return parts
}

func selectFile(t *testing.T, s *engine.Session, script string) {
	t.Helper()
	if _, err := s.EvaluateJS(script); err != nil {
		t.Fatalf("select file: %v", err)
	}
}

const selectNoteTxt = `(function(){
	var transfer = new DataTransfer();
	transfer.items.add(new File(["hello world"], "note.txt", {type: "text/plain"}));
	document.getElementById("upload").files = transfer.files;
	return document.getElementById("upload").files.length;
})()`

// qc:BR-106
func TestSubmitMultipartFormIncludesSelectedFile(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="text" name="title" value="Report">
		<input type="file" id="upload" name="attachment">
	</form>`)

	selectFile(t, s, selectNoteTxt)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("multipart form should submit")
	}
	if !strings.HasPrefix(sub.Enctype, "multipart/form-data; boundary=") {
		t.Fatalf("enctype = %q, want multipart/form-data with a boundary", sub.Enctype)
	}

	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if got := parts[0].Name; got != "title" {
		t.Fatalf("first part name = %q, want title", got)
	}
	if got := parts[0].Body; got != "Report" {
		t.Fatalf("first part body = %q, want Report", got)
	}
	if got := parts[1].Name; got != "attachment" {
		t.Fatalf("file part name = %q, want attachment", got)
	}
	if got := parts[1].Filename; got != "note.txt" {
		t.Fatalf("file part filename = %q, want note.txt", got)
	}
	if got := parts[1].ContentType; got != "text/plain" {
		t.Fatalf("file part content type = %q, want text/plain", got)
	}
	if got := parts[1].Body; got != "hello world" {
		t.Fatalf("file part body = %q, want hello world", got)
	}
}

// qc:BR-106
func TestSubmitMultipartFormEmptyFileControlSubmitsEmptyPart(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment">
	</form>`)

	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("multipart form should submit")
	}
	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	if got := parts[0].Name; got != "attachment" {
		t.Fatalf("part name = %q, want attachment", got)
	}
	if got := parts[0].Filename; got != "" {
		t.Fatalf("empty file control filename = %q, want empty", got)
	}
	if got := parts[0].ContentType; got != "application/octet-stream" {
		t.Fatalf("empty file control content type = %q, want application/octet-stream", got)
	}
	if got := parts[0].Body; got != "" {
		t.Fatalf("empty file control body = %q, want empty", got)
	}
}

// qc:BR-106
func TestSubmitMultipartFormPreservesControlOrder(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="text" name="first" value="1">
		<input type="file" id="upload" name="file">
		<input type="text" name="last" value="2">
	</form>`)

	selectFile(t, s, selectNoteTxt)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("multipart form should submit")
	}
	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	var names []string
	for _, part := range parts {
		names = append(names, part.Name)
	}
	if got, want := strings.Join(names, ","), "first,file,last"; got != want {
		t.Fatalf("part order = %q, want %q", got, want)
	}
}

// qc:BR-106
func TestSubmitFormGetIgnoresMultipartEnctype(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="GET" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment">
	</form>`)

	selectFile(t, s, selectNoteTxt)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("GET form should submit")
	}
	if sub.Enctype != "application/x-www-form-urlencoded" {
		t.Fatalf("GET enctype = %q, want urlencoded", sub.Enctype)
	}
	if sub.Encoded != "attachment=note.txt" {
		t.Fatalf("GET body = %q, want attachment=note.txt", sub.Encoded)
	}
}

// qc:BR-106
func TestSubmitterFormenctypeChoosesMultipart(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="application/x-www-form-urlencoded">
		<input type="file" id="upload" name="attachment">
		<button type="submit" id="send" formenctype="multipart/form-data">Upload</button>
	</form>`)

	selectFile(t, s, selectNoteTxt)
	s.SubmitForm(s.Doc.ElementByID("f"), s.Doc.ElementByID("send"))
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("form should submit")
	}
	if !strings.HasPrefix(sub.Enctype, "multipart/form-data; boundary=") {
		t.Fatalf("enctype = %q, want multipart/form-data with a boundary", sub.Enctype)
	}
	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	if len(parts) != 1 || parts[0].Filename != "note.txt" {
		t.Fatalf("multipart parts = %d, first filename = %q", len(parts), parts[0].Filename)
	}
}

// qc:BR-106
func TestEncodeFormMultipartEscapesDispositionValues(t *testing.T) {
	boundary, body, err := engine.EncodeFormMultipart([]engine.FormData{
		{Name: "a\"b\r\nc", Value: "line1\r\nline2"},
		{Name: "upload", Value: `x"y.txt`, File: &engine.FormFile{Filename: `x"y.txt`}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Contains(text, `name="a"b`) {
		t.Fatalf("form name kept a raw quote:\n%s", text)
	}
	if !strings.Contains(text, `name="a%22b%0D%0Ac"`) {
		t.Fatalf("form name not escaped per HTML:\n%s", text)
	}
	if !strings.Contains(text, `filename="x%22y.txt"`) {
		t.Fatalf("filename not escaped:\n%s", text)
	}
	if !strings.Contains(text, "line1\r\nline2") {
		t.Fatalf("text value line breaks not normalized:\n%s", text)
	}
	parts := multipartParts(t, "multipart/form-data; boundary="+boundary, text)
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if got, want := parts[1].ContentType, "application/octet-stream"; got != want {
		t.Fatalf("file part content type = %q, want %q", got, want)
	}
}

// qc:BR-106
func TestSubmitMultipartFormPreservesBinaryFileBytes(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment">
	</form>`)

	selectFile(t, s, `(function(){
		var bytes = new Uint8Array([0, 10, 13, 255, 128, 45, 45]);
		var transfer = new DataTransfer();
		transfer.items.add(new File([bytes], "raw.bin", {type: "application/octet-stream"}));
		document.getElementById("upload").files = transfer.files;
		return true;
	})()`)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("multipart form should submit")
	}
	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	if got, want := []byte(parts[0].Body), []byte{0, 10, 13, 255, 128, 45, 45}; !bytes.Equal(got, want) {
		t.Fatalf("file bytes = %v, want %v", got, want)
	}
}

// qc:BR-106
func TestRequiredFileInputAllowsSubmissionOnceFileSelected(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment" required>
	</form>`)

	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	if _, ok := s.PopSubmittedForm(); ok {
		t.Fatal("an empty required file control must block submission")
	}
	if s.Focused() != s.Doc.ElementByID("upload") {
		t.Fatal("the blocked submission should focus the empty file control")
	}

	selectFile(t, s, selectNoteTxt)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("a selected file should satisfy required and submit")
	}
	parts := multipartParts(t, sub.Enctype, sub.Encoded)
	if len(parts) != 1 || parts[0].Filename != "note.txt" {
		t.Fatalf("multipart parts = %d, first filename = %q", len(parts), parts[0].Filename)
	}
}

// qc:BR-106
func TestRequiredFileInputStaysInvalidWithoutScripting(t *testing.T) {
	s, err := engine.NewSession(`<form id="f" action="/upload" method="POST">
		<input type="file" name="attachment" required>
	</form>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	if _, ok := s.PopSubmittedForm(); ok {
		t.Fatal("without a scripting runtime no file can be selected, so required must block")
	}
}

// qc:BR-106
func TestOversizedMultipartFileFallsBackToURLEncoded(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment">
	</form>`)

	selectFile(t, s, `(function(){
		var transfer = new DataTransfer();
		transfer.items.add(new File([new Uint8Array(1048577)], "big.bin"));
		document.getElementById("upload").files = transfer.files;
		return true;
	})()`)
	s.SubmitForm(s.Doc.ElementByID("f"), nil)
	sub, ok := s.PopSubmittedForm()
	if !ok {
		t.Fatal("the form should still submit")
	}
	if sub.Enctype != "application/x-www-form-urlencoded" {
		t.Fatalf("enctype = %q, want the urlencoded fallback", sub.Enctype)
	}
	if sub.Encoded != "attachment=big.bin" {
		t.Fatalf("body = %q, want attachment=big.bin", sub.Encoded)
	}
}

// qc:BR-106
func TestRequiredFileInputValidityFollowsSelection(t *testing.T) {
	s := newUserInvalidSession(t, `<form id="f" action="/upload" method="POST" enctype="multipart/form-data">
		<input type="file" id="upload" name="attachment" required>
	</form>`)

	if !matchesPseudo(t, s, "upload", ":invalid") {
		t.Fatal("an empty required file control should match :invalid")
	}
	selectFile(t, s, selectNoteTxt)
	if matchesPseudo(t, s, "upload", ":invalid") {
		t.Fatal("a selected file should clear :invalid")
	}
	if !matchesPseudo(t, s, "upload", ":valid") {
		t.Fatal("a selected file should make the control :valid")
	}

	if _, err := s.EvaluateJS(`document.getElementById("upload").value = ""`); err != nil {
		t.Fatalf("clear file selection: %v", err)
	}
	if !matchesPseudo(t, s, "upload", ":invalid") {
		t.Fatal("clearing the selection should restore :invalid")
	}
}

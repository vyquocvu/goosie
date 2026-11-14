package net_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/net"
)

// qc:BR-145
func TestResponseTextUTF8Passthrough(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    []byte("<p>Hello, world</p>"),
	}
	if got := r.Text(); got != "<p>Hello, world</p>" {
		t.Errorf("Text() = %q, want the raw bytes as a string", got)
	}
}

// qc:BR-145
func TestResponseTextExplicitUTF8Charset(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=utf-8"},
		Body:    []byte("<p>caf\u00e9</p>"),
	}
	if got := r.Text(); got != "<p>caf\u00e9</p>" {
		t.Errorf("Text() = %q, want the UTF-8 body preserved", got)
	}
}

func TestResponseTextWindows1252Decodes(t *testing.T) {
	body := []byte{0x80, 0x93, 'h', 'i', 0x94}
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=windows-1252"},
		Body:    body,
	}
	got, label, certain := r.TextWithEncoding()
	want := "\u20AC\u201Chi\u201D"
	if got != want {
		t.Errorf("Text() = %q, want %q (euro + smart quotes around 'hi')", got, want)
	}
	if label != "windows-1252" || !certain {
		t.Errorf("selected encoding = %q (certain %v), want windows-1252 (certain)", label, certain)
	}
}

func TestResponseTextLatin1Decodes(t *testing.T) {
	body := []byte{0xE9, 0xE8, 0xEA}
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=iso-8859-1"},
		Body:    body,
	}
	got := r.Text()
	want := "\u00E9\u00E8\u00EA"
	if got != want {
		t.Errorf("Text() = %q, want %q (Latin-1 accented e's)", got, want)
	}
}

// qc:BR-146
func TestResponseTextSniffsCharsetFromMeta(t *testing.T) {
	body := []byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1252"><p>` + "\x93hello\x94</p>")
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    body,
	}
	got := r.Text()
	if got[0] != '<' {
		t.Errorf("Text() starts with %q, expected '<'", got)
	}
	if !containsRune(got, '\u201C') || !containsRune(got, '\u201D') {
		t.Errorf("Text() = %q, want smart quotes from Windows-1252 sniffing", got)
	}
}

// qc:BR-146
func TestResponseTextUnknownCharsetPassesThrough(t *testing.T) {
	body := []byte("hello world")
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=koi8-r"},
		Body:    body,
	}
	if got := r.Text(); got != "hello world" {
		t.Errorf("Text() = %q, want raw passthrough for unknown charset", got)
	}
}

// qc:BR-146
func TestResponseTextMissingContentTypeUsesHTMLDefault(t *testing.T) {
	body := []byte{'<', 'p', '>', 0x93, 'h', 'i', 0x94, '<', '/', 'p', '>'}
	r := &net.Response{Body: body}
	got, label, certain := r.TextWithEncoding()
	if want := "<p>\u201Chi\u201D</p>"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if label != "windows-1252" || certain {
		t.Errorf("selected encoding = %q (certain %v), want windows-1252 (tentative)", label, certain)
	}
}

func TestResponseTextEmptyBody(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    []byte{},
	}
	if got := r.Text(); got != "" {
		t.Errorf("Text() = %q, want empty string", got)
	}
}

func TestResponseTextHighBytesLatin1(t *testing.T) {
	body := []byte{0xA0, 0xFF}
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=iso-8859-1"},
		Body:    body,
	}
	got := r.Text()
	if got != "\u00A0\u00FF" {
		t.Errorf("Text() = %q, want U+00A0 U+00FF", got)
	}
}

func TestResponseTextBOMOverridesTransportCharset(t *testing.T) {
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte("<p>€</p>")...)
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=windows-1252"},
		Body:    body,
	}
	got, label, certain := r.TextWithEncoding()
	if want := "<p>€</p>"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if label != "utf-8" || !certain {
		t.Errorf("selected encoding = %q (certain %v), want utf-8 (certain)", label, certain)
	}
}

func TestResponseTextUTF16BOM(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=windows-1252"},
		Body:    []byte{0xFF, 0xFE, '<', 0, 'p', 0, '>', 0, 0xAC, 0x20, '<', 0, '/', 0, 'p', 0, '>', 0},
	}
	got, label, certain := r.TextWithEncoding()
	if want := "<p>€</p>"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if label != "utf-16le" || !certain {
		t.Errorf("selected encoding = %q (certain %v), want utf-16le (certain)", label, certain)
	}
}

func TestResponseTextMetaSniffUsesHTMLPrescanWindow(t *testing.T) {
	meta := `<meta charset="windows-1252">`
	body := append([]byte(meta+"<p>"), 0x80)
	body = append(body, []byte("</p>")...)
	r := &net.Response{Headers: map[string]string{"Content-Type": "text/html"}, Body: body}
	got, label, certain := r.TextWithEncoding()
	if want := meta + "<p>€</p>"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if label != "windows-1252" || certain {
		t.Errorf("selected encoding = %q (certain %v), want windows-1252 (tentative)", label, certain)
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

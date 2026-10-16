package net_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/net"
)

// Response.Text() is the bridge between raw response bytes and the UTF-8
// string the DOM parser consumes. Most of the web is UTF-8, but the legacy
// pages that declare Latin-1 or Windows-1252 must decode correctly: the
// bytes 0x80-0x9F are the divergence point, where Windows-1252 has
// printable characters (euro sign, smart quotes) that Latin-1 maps to
// invisible control codes. A wrong decode here turns a real page's
// punctuation into garbage.

// TestResponseTextUTF8Passthrough verifies that a UTF-8 body with no
// charset declaration passes through unchanged. This is the common case:
// the vast majority of pages are UTF-8 and must not be touched.
func TestResponseTextUTF8Passthrough(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    []byte("<p>Hello, world</p>"),
	}
	if got := r.Text(); got != "<p>Hello, world</p>" {
		t.Errorf("Text() = %q, want the raw bytes as a string", got)
	}
}

// TestResponseTextExplicitUTF8Charset verifies that an explicit UTF-8
// charset declaration is honoured and the body passes through.
func TestResponseTextExplicitUTF8Charset(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=utf-8"},
		Body:    []byte("<p>caf\u00e9</p>"),
	}
	if got := r.Text(); got != "<p>caf\u00e9</p>" {
		t.Errorf("Text() = %q, want the UTF-8 body preserved", got)
	}
}

// TestResponseTextWindows1252Decodes verifies that a Windows-1252 body
// decodes the 0x80-0x9F range to the correct Unicode characters. This
// is the range where Windows-1252 diverges from Latin-1: byte 0x80 is
// the euro sign (U+20AC), not the control code U+0080.
func TestResponseTextWindows1252Decodes(t *testing.T) {
	// Build a body with Windows-1252 high bytes:
	// 0x80 = euro sign, 0x93 = left double quote, 0x94 = right double quote
	body := []byte{0x80, 0x93, 'h', 'i', 0x94}
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html; charset=windows-1252"},
		Body:    body,
	}
	got := r.Text()
	want := "\u20AC\u201Chi\u201D"
	if got != want {
		t.Errorf("Text() = %q, want %q (euro + smart quotes around 'hi')", got, want)
	}
}

// TestResponseTextLatin1Decodes verifies that an ISO-8859-1 body decodes
// the full 0x80-0xFF range as identity-mapped Unicode code points. In
// Latin-1 the 0x80-0x9F range are control codes, unlike Windows-1252.
func TestResponseTextLatin1Decodes(t *testing.T) {
	// Latin-1 0xE9 = e-acute, 0xE8 = e-grave, 0xEA = e-circumflex
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

// TestResponseTextSniffsCharsetFromMeta verifies that when no Content-Type
// charset is present, the decoder sniffs a charset= declaration from the
// first bytes of the body, matching what real pages serve via the
// http-equiv Content-Type meta tag.
func TestResponseTextSniffsCharsetFromMeta(t *testing.T) {
	// The http-equiv form puts charset= inside a quoted content attribute,
	// which the sniffer can parse: after "charset=" the value runs until
	// the closing quote.
	body := []byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1252"><p>` + "\x93hello\x94</p>")
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    body,
	}
	got := r.Text()
	// The meta tag bytes are ASCII and pass through; the 0x93/0x94 become
	// smart quotes under Windows-1252.
	if got[0] != '<' {
		t.Errorf("Text() starts with %q, expected '<'", got)
	}
	// Verify the smart quotes decoded: 0x93 -> U+201C, 0x94 -> U+201D
	if !containsRune(got, '\u201C') || !containsRune(got, '\u201D') {
		t.Errorf("Text() = %q, want smart quotes from Windows-1252 sniffing", got)
	}
}

// TestResponseTextUnknownCharsetPassesThrough verifies that an unknown
// charset is treated as UTF-8 (passthrough), which is the safe default:
// the web is overwhelmingly UTF-8 and an unrecognized name should not
// trigger a lossy conversion.
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

// TestResponseTextEmptyBody verifies that an empty body with no charset
// returns an empty string without error.
func TestResponseTextEmptyBody(t *testing.T) {
	r := &net.Response{
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    []byte{},
	}
	if got := r.Text(); got != "" {
		t.Errorf("Text() = %q, want empty string", got)
	}
}

// TestResponseTextHighBytesLatin1 verifies that the full 0xA0-0xFF range
// maps to the same code points in both Latin-1 and Windows-1252, since
// the divergence is only in 0x80-0x9F.
func TestResponseTextHighBytesLatin1(t *testing.T) {
	// 0xA0 = non-breaking space, 0xFF = y-dieresis
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

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

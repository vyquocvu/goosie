package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

func TestRCDATAElementsDecodeCharacterReferences(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><head><title>A &amp; B</title></head><body><textarea>A &amp; B &lt; C</textarea><script>const x = "&amp;";</script></body></html>`)
	if got, want := doc.Head.FirstChild.TextContent(), "A & B"; got != want {
		t.Errorf("title text = %q, want %q", got, want)
	}
	textarea := doc.Body.FirstChild
	if textarea == nil || textarea.Data != "textarea" {
		t.Fatalf("first body child = %#v, want textarea", textarea)
	}
	if got, want := textarea.TextContent(), "A & B < C"; got != want {
		t.Errorf("textarea text = %q, want %q", got, want)
	}
	script := textarea.NextSibling
	if script == nil || script.Data != "script" {
		t.Fatalf("textarea next sibling = %#v, want script", script)
	}
	if got, want := script.TextContent(), `const x = "&amp;";`; got != want {
		t.Errorf("script text = %q, want %q", got, want)
	}
}

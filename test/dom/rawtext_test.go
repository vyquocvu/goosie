package dom_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestRawTextElementsDoNotParseMarkupAsChildren(t *testing.T) {
	for _, tag := range []string{"iframe", "noembed", "noframes", "noscript", "xmp"} {
		t.Run(tag, func(t *testing.T) {
			doc := domtest.Parse(fmt.Sprintf(`<!doctype html><body><%[1]s><b>raw &amp; literal</b></%[1]s><p>after</p>`, tag))
			el := doc.Body.FirstChild
			if el == nil || el.Data != tag {
				t.Fatalf("body first child = %#v, want <%s>", el, tag)
			}
			if got, want := el.FirstChild.DataContent, `<b>raw &amp; literal</b>`; got != want {
				t.Errorf("<%s> text = %q, want literal raw text %q", tag, got, want)
			}
			if el.FirstChild.NextSibling != nil {
				t.Errorf("<%s> parsed markup into multiple children", tag)
			}
			if after := el.NextSibling; after == nil || after.Data != "p" {
				t.Errorf("element after raw text = %#v, want <p>", after)
			}
		})
	}
}

func TestScriptDataDoubleEscapedEndTagDoesNotCloseScript(t *testing.T) {
	markup := `<script><!--
const html = "<script>inner</script>";
//-->
window.afterEmbeddedScript = true;
</script><p id="after">after</p>`
	doc := domtest.Parse(markup)
	scripts := doc.ElementsByTagName("script")
	if len(scripts) != 1 {
		t.Fatalf("script count = %d, want 1", len(scripts))
	}
	if got := scripts[0].TextContent(); !strings.Contains(got, `"<script>inner</script>"`) || !strings.Contains(got, "window.afterEmbeddedScript") {
		t.Fatalf("script text lost double-escaped content: %q", got)
	}
	paragraphs := doc.ElementsByTagName("p")
	if len(paragraphs) != 1 || paragraphs[0].GetAttribute("id") != "after" {
		t.Fatal("content after the actual script end tag was not parsed")
	}
}

func TestPlaintextConsumesTheRestOfTheInput(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><body><plaintext><p>literal</p></plaintext><p>still literal</p>`)
	plaintext := doc.Body.FirstChild
	if plaintext == nil || plaintext.Data != "plaintext" {
		t.Fatalf("body first child = %#v, want plaintext", plaintext)
	}
	if got, want := plaintext.TextContent(), `<p>literal</p></plaintext><p>still literal</p>`; got != want {
		t.Errorf("plaintext contents = %q, want %q", got, want)
	}
}

func TestRawTextFragmentContextsKeepEndTagsAsText(t *testing.T) {
	for _, tag := range []string{"title", "textarea", "style", "xmp", "iframe", "noembed", "noframes", "script", "noscript", "plaintext"} {
		t.Run(tag, func(t *testing.T) {
			doc := dom.NewDocument()
			context := doc.NewElement(tag)
			input := "</" + tag + "><div>"
			fragment, err := dom.ParseFragmentBounded(context, input, dom.ParseLimits{
				Nodes: 16, Depth: 8, Attributes: 16, AttributeBytes: 1024,
			})
			if err != nil {
				t.Fatalf("parse fragment: %v", err)
			}
			if fragment.FirstChild == nil || fragment.FirstChild.Type != dom.NodeText {
				t.Fatalf("fragment first child = %#v, want one text node", fragment.FirstChild)
			}
			if got := fragment.FirstChild.TextContent(); got != input {
				t.Errorf("fragment text = %q, want literal %q", got, input)
			}
			if fragment.FirstChild.NextSibling != nil {
				t.Errorf("fragment parsed an end tag or element as markup: %#v", fragment.FirstChild.NextSibling)
			}
		})
	}
}

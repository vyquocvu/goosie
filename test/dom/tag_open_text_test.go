package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/test/domtest"
)

// TestLessThanFollowedByNonLetterIsText pins the tag-open state: only a
// letter after "<" starts a tag. "a <= b", "2 < 3" and a trailing "<" are
// literal text. Routing "<=" into the tag reader swallowed the real close tag
// that followed and nested every later sibling inside the box - which is how
// one "<=" in a media-queries parity fixture collapsed the whole lower page
// into a single div.
func TestLessThanFollowedByNonLetterIsText(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body>
<div class="a">viewport <= 500px</div>
<div class="b">after</div>
</body></html>`)

	a := find(&doc.Node, "a")
	if a == nil {
		t.Fatal("no .a element found")
	}
	if txt := a.TextContent(); !strings.Contains(txt, "viewport <= 500px") {
		t.Errorf(".a text = %q, want it to keep the literal \"<=\"", txt)
	}
	b := find(&doc.Node, "b")
	if b == nil {
		t.Fatal("no .b element found")
	}
	if b.Parent != a.Parent {
		t.Error(".b is not a sibling of .a (it nested inside after \"<=\")")
	}
	if txt := b.TextContent(); txt != "after" {
		t.Errorf(".b text = %q, want %q", txt, "after")
	}
}

// TestBareLessThanAtEndIsText pins the end-of-input half of the same state:
// a "<" with nothing after it is text, not a tag that eats the document.
func TestBareLessThanAtEndIsText(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><html><body><div class="a">trailing <</div></body></html>`)
	a := find(&doc.Node, "a")
	if a == nil {
		t.Fatal("no .a element found")
	}
	if txt := a.TextContent(); !strings.Contains(txt, "trailing <") {
		t.Errorf(".a text = %q, want the trailing \"<\" kept as text", txt)
	}
}

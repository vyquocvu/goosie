package dom_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

// find returns the first element whose class attribute contains wantClass.
func find(root *dom.Node, wantClass string) *dom.Node {
	var el *dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if el != nil {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				for _, cl := range c.ClassList() {
					if cl == wantClass {
						el = c
						return
					}
				}
				walk(c)
			}
		}
	}
	walk(root)
	return el
}

// TestNestedListItemStaysInSublist pins HTML5 list-item scope: an <li> may only
// close an <li> in the same list, never one in an ancestor list. Bootstrap-style
// navbars rely on this — the malformed <li><li> markup below must keep the
// dropdown items inside the (display:none) .dropdown-menu <ul>, not leak them
// into the outer .navbar-nav list where they would render.
func TestNestedListItemStaysInSublist(t *testing.T) {
	doc := dom.Parse(`<!doctype html><html><body>
<ul class="navbar-nav">
<li class="nav-item dropdown">
<a href="#">Design</a>
<ul class="dropdown-menu">
<li class="nav-item"><a href="#">Freebies</a>
<li><li class="nav-item"><a href="#">Products</a>
</ul>
</ul>
</body></html>`)

	dropdown := find(&doc.Node, "dropdown-menu")
	if dropdown == nil {
		t.Fatal("no .dropdown-menu element found")
	}
	txt := dropdown.TextContent()
	for _, want := range []string{"Freebies", "Products"} {
		if !strings.Contains(txt, want) {
			t.Errorf(".dropdown-menu does not contain %q (leaked to ancestor list); subtree text = %q", want, txt)
		}
	}

	// The outer navbar-nav <li> must hold only the "Design" toggle directly;
	// the dropdown items belong one level deeper.
	navbar := find(&doc.Node, "navbar-nav")
	if navbar == nil {
		t.Fatal("no .navbar-nav element found")
	}
	outerText := ""
	for _, li := range navbar.ChildElements() {
		outerText += li.TextContent()
	}
	if strings.Count(outerText, "Freebies") != 1 {
		t.Errorf("expected exactly one Freebies under navbar-nav (inside the sublist), got %q", outerText)
	}
}

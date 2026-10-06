package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// TestNthChildOfSelectorList pins the Selectors 4 `of` form: only siblings
// matching the selector count, and the element itself must match one. Before,
// the whole argument (including `of .foo`) went to the An+B parser, failed,
// and no `of` form ever matched.
func TestNthChildOfSelectorList(t *testing.T) {
	doc := dom.NewDocument()
	ul := doc.NewElement("ul")
	doc.Node.AppendChild(ul)
	mk := func(class, text string) *dom.Node {
		li := doc.NewElement("li")
		if class != "" {
			li.SetAttribute("class", class)
		}
		li.AppendChild(doc.NewText(text))
		ul.AppendChild(li)
		return li
	}
	mk("", "a")
	foo1 := mk("foo", "b")
	mk("", "c")
	foo2 := mk("foo", "d")

	sel := css.ParseSelector("li:nth-child(2 of .foo)")
	if sel.Matches(foo1) {
		t.Error("first .foo is 1st of .foo, must not match (2 of .foo)")
	}
	if !sel.Matches(foo2) {
		t.Error("second .foo must match (2 of .foo)")
	}

	// Plain forms keep working through the same path: foo1 is the 2nd child
	// overall, so a bare (2) matches it.
	plain := css.ParseSelector("li:nth-child(2)")
	if !plain.Matches(foo1) {
		t.Error("plain li:nth-child(2) must still match the 2nd child")
	}
}

// TestNthOfSpecificity pins the cascade half: the `of` selector contributes
// its most specific argument on top of the pseudo-class itself.
func TestNthOfSpecificity(t *testing.T) {
	sel := css.ParseSelector("li:nth-child(2 of #x.y)")
	a, b, c := sel.Specificity()
	// (0,0,1) for li + (0,1,0) for the pseudo-class + (1,1,0) for #x.y.
	if a != 1 || b != 2 || c != 1 {
		t.Errorf("li:nth-child(2 of #x.y) specificity = (%d,%d,%d), want (1,2,1)", a, b, c)
	}
	plain := css.ParseSelector("li:nth-child(2)")
	a, b, c = plain.Specificity()
	if a != 0 || b != 1 || c != 1 {
		t.Errorf("li:nth-child(2) specificity = (%d,%d,%d), want (0,1,1)", a, b, c)
	}
}

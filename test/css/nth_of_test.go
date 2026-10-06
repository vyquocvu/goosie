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

// TestLangPseudoClass pins :lang() matching: the nearest ancestor-or-self
// language wins, lang beats xml:lang on the same element, and BCP47 prefixes
// match (en covers en-US but not english).
func TestLangPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	root := doc.NewElement("div")
	root.SetAttribute("lang", "nl")
	doc.Node.AppendChild(root)
	mk := func(parent *dom.Node, tag, lang, xmllang string) *dom.Node {
		el := doc.NewElement(tag)
		if lang != "" {
			el.SetAttribute("lang", lang)
		}
		if xmllang != "" {
			el.SetAttribute("xml:lang", xmllang)
		}
		parent.AppendChild(el)
		return el
	}
	enPara := mk(root, "p", "en", "")
	nlPara := mk(root, "p", "", "")
	dePara := mk(root, "p", "de-CH", "")
	mk(root, "p", "english", "")

	for _, c := range []struct {
		sel string
		el  *dom.Node
		ok  bool
	}{
		{"p:lang(en)", enPara, true},
		{"p:lang(nl)", enPara, false},
		{"p:lang(nl)", nlPara, true}, // inherited from root
		{"p:lang(de)", dePara, true},
		{"p:lang(de-CH)", dePara, true},
		{"p:lang(ch)", dePara, false},
		{"p:lang(en-US)", enPara, false},
	} {
		if got := css.ParseSelector(c.sel).Matches(c.el); got != c.ok {
			t.Errorf("%s on lang=%q xml:lang=%q = %v, want %v",
				c.sel, c.el.GetAttribute("lang"), c.el.GetAttribute("xml:lang"), got, c.ok)
		}
	}
}

// TestLangPlainXmlLangInvisible pins what the reference engine does with
// parser-produced attributes: a literal xml:lang never created by
// setAttributeNS carries no namespace and :lang() ignores it, so lang alone
// decides.
func TestLangPlainXmlLangInvisible(t *testing.T) {
	doc := dom.NewDocument()
	root := doc.NewElement("div")
	doc.Node.AppendChild(root)
	p := doc.NewElement("p")
	p.SetAttribute("lang", "de")
	p.SetAttribute("xml:lang", "en")
	root.AppendChild(p)

	for _, c := range []struct {
		sel string
		ok  bool
	}{
		{"p:lang(de)", true},
		{"p:lang(en)", false},
	} {
		if got := css.ParseSelector(c.sel).Matches(p); got != c.ok {
			t.Errorf("%s = %v, want %v (plain xml:lang is invisible)", c.sel, got, c.ok)
		}
	}
}

// TestLangNamespacedXmlLangWins pins the setAttributeNS half: an
// XML-namespaced xml:lang decides its level ahead of a plain lang.
func TestLangNamespacedXmlLangWins(t *testing.T) {
	doc := dom.NewDocument()
	root := doc.NewElement("div")
	doc.Node.AppendChild(root)
	p := doc.NewElement("p")
	p.SetAttribute("lang", "de")
	p.Attr = append(p.Attr, dom.Attribute{Name: "xml:lang", Namespace: dom.NSXML, Value: "en"})
	root.AppendChild(p)

	for _, c := range []struct {
		sel string
		ok  bool
	}{
		{"p:lang(en)", true},
		{"p:lang(de)", false},
	} {
		if got := css.ParseSelector(c.sel).Matches(p); got != c.ok {
			t.Errorf("%s = %v, want %v", c.sel, got, c.ok)
		}
	}
}

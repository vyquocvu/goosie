package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestDirSelectorWhitespaceAndInheritance(t *testing.T) {
	doc := dom.NewDocument()
	root := doc.NewElement("html")
	doc.Node.AppendChild(root)
	root.SetAttribute("dir", "rtl")
	parent := doc.NewElement("div")
	root.AppendChild(parent)
	child := doc.NewElement("span")
	parent.AppendChild(child)

	for _, selector := range []string{":dir(rtl)", ":dir( rtl)", ":dir(rtl )", ":dir( rtl )"} {
		if !css.ParseSelector(selector).Matches(child) {
			t.Errorf("%s should match a child inheriting dir=rtl", selector)
		}
	}
	if css.ParseSelector(":dir(ltr)").Matches(child) {
		t.Error(":dir(ltr) must not match a child inheriting dir=rtl")
	}

	child.SetAttribute("dir", "ltr")
	if !css.ParseSelector(":dir(ltr)").Matches(child) {
		t.Error(":dir(ltr) should match the explicit direction on the element")
	}
	if css.ParseSelector(":dir(rtl)").Matches(child) {
		t.Error(":dir(rtl) must not override the element's explicit dir=ltr")
	}
}

func TestDirSelectorDefaultsToLTRAndIgnoresInvalidDir(t *testing.T) {
	doc := dom.NewDocument()
	parent := doc.NewElement("div")
	doc.Node.AppendChild(parent)
	parent.SetAttribute("dir", "sideways")
	child := doc.NewElement("span")
	parent.AppendChild(child)

	if !css.ParseSelector(":dir(ltr)").Matches(child) {
		t.Error("invalid dir value should inherit the document's default ltr direction")
	}
	if css.ParseSelector(":dir(rtl)").Matches(child) {
		t.Error("invalid dir value must not create rtl directionality")
	}
}

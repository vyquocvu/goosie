package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestMovingShadowRootChildIntoLightTreeUnlinksShadowList(t *testing.T) {
	doc := dom.NewDocument()
	host := doc.NewElement("host")
	doc.Node.AppendChild(host)
	root := &dom.ShadowRoot{Host: host, Mode: "open"}
	host.ShadowRoot = root
	child := doc.NewElement("x-child")
	root.AppendChild(child)
	lightParent := doc.NewElement("div")
	doc.Node.AppendChild(lightParent)

	lightParent.AppendChild(child)
	if root.FirstChild != nil || root.LastChild != nil {
		t.Fatal("moving a shadow child with append left it in the shadow-root child list")
	}
	if child.Parent != lightParent || lightParent.FirstChild != child {
		t.Fatal("append did not move the shadow child into the light tree")
	}

	root.AppendChild(child)
	otherLightParent := doc.NewElement("section")
	before := doc.NewElement("before")
	doc.Node.AppendChild(otherLightParent)
	otherLightParent.AppendChild(before)
	otherLightParent.InsertBefore(child, before)
	if root.FirstChild != nil || child.Parent != otherLightParent || otherLightParent.FirstChild != child || child.NextSibling != before {
		t.Fatal("moving a shadow child with insert-before left stale shadow-tree ownership")
	}
}

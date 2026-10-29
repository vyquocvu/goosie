package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestCloneNodeShallow(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.NewElement("div")
	div.SetAttribute("class", "container")
	p := doc.NewElement("p")
	p.AppendChild(doc.NewText("hello"))
	div.AppendChild(p)

	clone := div.CloneNode(false)
	if clone == nil {
		t.Fatal("expected clone to be non-nil")
	}
	if clone.Data != "div" {
		t.Fatalf("expected Data to be 'div', got %q", clone.Data)
	}
	if clone.GetAttribute("class") != "container" {
		t.Fatalf("expected class='container', got %q", clone.GetAttribute("class"))
	}
	if clone.FirstChild != nil {
		t.Fatalf("expected shallow clone to have no children, got %v", clone.FirstChild)
	}
	if clone.ID == div.ID {
		t.Fatalf("expected fresh ID on clone, got %d matching original", clone.ID)
	}
}

func TestCloneNodeDeep(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.NewElement("div")
	div.SetAttribute("id", "box")
	p := doc.NewElement("p")
	p.SetAttribute("class", "text")
	txt := doc.NewText("content")
	p.AppendChild(txt)
	div.AppendChild(p)

	clone := div.CloneNode(true)
	if clone == nil {
		t.Fatal("expected clone to be non-nil")
	}
	if clone.FirstChild == nil {
		t.Fatal("expected clone to have children")
	}
	clonedP := clone.FirstChild
	if clonedP.Data != "p" || clonedP.GetAttribute("class") != "text" {
		t.Fatalf("unexpected cloned child: %+v", clonedP)
	}
	if clonedP.FirstChild == nil || clonedP.FirstChild.DataContent != "content" {
		t.Fatalf("unexpected cloned grandchild: %+v", clonedP.FirstChild)
	}
	if clonedP.ID == p.ID || clonedP.FirstChild.ID == txt.ID {
		t.Fatal("deep cloned nodes must allocate unique IDs")
	}
}

func TestTemplateContentDoesNotStealTemplateElementChildren(t *testing.T) {
	doc := dom.NewDocument()
	tmpl := doc.NewElement("template")
	p := doc.NewElement("p")
	p.AppendChild(doc.NewText("Template Child"))
	tmpl.AppendChild(p)

	frag := tmpl.TemplateContent()
	if frag == nil {
		t.Fatal("expected non-nil template content fragment")
	}
	if !frag.DocumentFragment() {
		t.Fatalf("expected DocumentFragment type, got %v", frag.Type)
	}
	if frag.FirstChild != nil {
		t.Fatalf("template.content unexpectedly acquired ordinary template children: %v", frag.FirstChild)
	}
	if tmpl.FirstChild != p || p.Parent != tmpl || p.FirstChild == nil || p.FirstChild.DataContent != "Template Child" {
		t.Fatalf("template.appendChild-style insertion should stay in the template element: parent=%v child=%v", p.Parent, tmpl.FirstChild)
	}
}

func TestTemplateContentUsesInertOwnerDocument(t *testing.T) {
	doc := dom.NewDocument()
	first := doc.NewElement("template")
	second := doc.NewElement("template")
	firstContent := first.TemplateContent()
	secondContent := second.TemplateContent()
	if firstContent.Doc == nil || firstContent.Doc == doc {
		t.Fatal("template contents must use a separate inert owner document")
	}
	if secondContent.Doc != firstContent.Doc {
		t.Fatal("templates from one document must share their inert contents owner document")
	}
	child := doc.NewElement("div")
	firstContent.AppendChild(child)
	if child.Doc != firstContent.Doc {
		t.Fatal("inserted template contents child must be adopted into the inert owner document")
	}

	otherDoc := dom.NewDocument()
	first.AdoptInto(otherDoc)
	if first.TemplateContent().Doc != otherDoc.TemplateContentsOwnerDocument() {
		t.Fatal("adopting the template must move its contents to the destination document's inert owner document")
	}
	if child.Doc != first.TemplateContent().Doc {
		t.Fatal("adopting the template must update template content descendants' owner document")
	}
}

func TestImportNode(t *testing.T) {
	docA := dom.NewDocument()
	docB := dom.NewDocument()

	source := docA.NewElement("article")
	h1 := docA.NewElement("h1")
	h1.AppendChild(docA.NewText("Heading"))
	source.AppendChild(h1)

	imported := docB.ImportNode(source, true)
	if imported == nil {
		t.Fatal("expected imported node to be non-nil")
	}
	if imported.Doc != docB {
		t.Fatalf("expected imported node to belong to docB, got %v", imported.Doc)
	}
	if imported.FirstChild == nil || imported.FirstChild.Doc != docB {
		t.Fatalf("expected child to belong to docB, got %v", imported.FirstChild)
	}
	if imported.FirstChild.FirstChild == nil || imported.FirstChild.FirstChild.Doc != docB {
		t.Fatalf("expected grandchild to belong to docB, got %v", imported.FirstChild.FirstChild)
	}
	if imported.FirstChild.FirstChild.DataContent != "Heading" {
		t.Fatalf("expected text 'Heading', got %q", imported.FirstChild.FirstChild.DataContent)
	}
}

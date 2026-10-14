package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestNodeAdoption(t *testing.T) {
	doc1 := dom.NewDocument()
	div1 := doc1.NewElement("div")
	doc1.Node.AppendChild(div1)

	doc2 := dom.NewDocument()
	div2 := doc2.NewElement("div")
	p2 := doc2.NewElement("p")
	div2.AppendChild(p2)

	// In doc2, div2 and p2 have their own IDs.
	oldDiv2ID := div2.ID
	oldP2ID := p2.ID

	// Moving div2 into doc1 must adopt div2 and all its children into doc1.
	div1.AppendChild(div2)

	if div2.Doc != doc1 {
		t.Fatalf("expected div2.Doc to be doc1, got %v", div2.Doc)
	}
	if p2.Doc != doc1 {
		t.Fatalf("expected p2.Doc to be doc1, got %v", p2.Doc)
	}
	if div2.ID == oldDiv2ID && doc1 != doc2 {
		t.Fatalf("expected div2 to get new ID in doc1")
	}
	if p2.ID == oldP2ID && doc1 != doc2 {
		t.Fatalf("expected p2 to get new ID in doc1")
	}
	if div2.ID == p2.ID {
		t.Fatalf("div2 and p2 have duplicate ID %v", div2.ID)
	}
	if div2.ID == div1.ID {
		t.Fatalf("div2 and div1 have duplicate ID %v", div2.ID)
	}
}

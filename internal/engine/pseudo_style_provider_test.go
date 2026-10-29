package engine

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestPseudoStyleValuesExposesTextTransform(t *testing.T) {
	doc := dom.NewDocument()
	node := doc.NewElement("div")
	session := &Session{
		Doc: doc,
		PseudoStyles: map[style.PseudoKey]*style.ComputedStyle{
			{NodeID: node.ID, Pseudo: "first-line"}: {TextTransform: "uppercase"},
		},
	}

	got := session.pseudoStyleValues(node, "first-line")
	if got["text-transform"] != "uppercase" {
		t.Fatalf("first-line pseudo text-transform = %q, want uppercase", got["text-transform"])
	}
	if session.pseudoStyleValues(node, "first-letter") != nil {
		t.Fatal("missing pseudo style should return nil")
	}
}

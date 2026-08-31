package dom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// TestM54_StoreProvidesFullTreeAccess verifies that the compact DOM store
// provides complete tree access without the M2.5 compatibility adapter.
// This test validates the precondition for removing the adapter: all
// traversal, attribute, and text operations work directly via NodeID.
func TestM54_StoreProvidesFullTreeAccess(t *testing.T) {
	s := dom.NewStore(16)

	// Build a small document: <html><body><div class="main"><p>hello</p></div></body></html>
	doc, _ := s.Allocate()
	_ = s.SetKind(doc, dom.NodeKindDocument)

	htmlID, _ := s.Allocate()
	_ = s.SetKind(htmlID, dom.NodeKindElement)
	_ = s.SetName(htmlID, atom.AtomHtml)
	_ = s.AppendChild(doc, htmlID)

	body, _ := s.Allocate()
	_ = s.SetKind(body, dom.NodeKindElement)
	_ = s.SetName(body, atom.AtomBody)
	_ = s.AppendChild(htmlID, body)

	div, _ := s.Allocate()
	_ = s.SetKind(div, dom.NodeKindElement)
	_ = s.SetName(div, atom.AtomDiv)
	classVal := atom.Intern("main")
	_ = s.AppendAttrs(div, []dom.Attr{{Name: atom.AttrClass, Value: classVal}})
	_ = s.AppendChild(body, div)

	p, _ := s.Allocate()
	_ = s.SetKind(p, dom.NodeKindElement)
	_ = s.SetName(p, atom.AtomP)
	_ = s.AppendChild(div, p)

	text, _ := s.Allocate()
	_ = s.SetKind(text, dom.NodeKindText)
	_ = s.AppendText(text, "hello")
	_ = s.AppendChild(p, text)

	// Verify tree traversal via iterators (no adapter needed).
	childCount := 0
	for it := s.Children(body); it.Next(); {
		childCount++
	}
	if childCount != 1 {
		t.Errorf("body child count = %d, want 1", childCount)
	}

	// Verify subtree traversal.
	nodeCount := 0
	for it := s.Subtree(doc); it.Next(); {
		nodeCount++
	}
	if nodeCount != 6 {
		t.Errorf("subtree node count = %d, want 6", nodeCount)
	}

	// Verify attribute access.
	attrs, err := s.Attrs(div)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 1 {
		t.Fatalf("div attr count = %d, want 1", len(attrs))
	}
	if attrs[0].Name != atom.AttrClass {
		t.Errorf("attr name = %v, want AttrClass", attrs[0].Name)
	}
	if attrs[0].Value != classVal {
		t.Errorf("attr value = %v, want %v", attrs[0].Value, classVal)
	}

	// Verify text access.
	gotText, err := s.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	if gotText != "hello" {
		t.Errorf("text = %q, want %q", gotText, "hello")
	}

	// Verify parent/child relationships.
	if s.Parent(div) != body {
		t.Errorf("parent of div = %v, want %v", s.Parent(div), body)
	}
	if s.FirstChild(body) != div {
		t.Errorf("first child of body = %v, want %v", s.FirstChild(body), div)
	}
}

// TestM54_StoreSupportsMutation verifies that the compact store supports
// all mutation operations needed by consumers that previously used the adapter.
func TestM54_StoreSupportsMutation(t *testing.T) {
	s := dom.NewStore(16)

	root, _ := s.Allocate()
	_ = s.SetKind(root, dom.NodeKindDocument)

	a, _ := s.Allocate()
	_ = s.SetKind(a, dom.NodeKindElement)
	_ = s.SetName(a, atom.AtomDiv)
	_ = s.AppendChild(root, a)

	b, _ := s.Allocate()
	_ = s.SetKind(b, dom.NodeKindElement)
	_ = s.SetName(b, atom.AtomP)
	_ = s.AppendChild(root, b)

	// InsertBefore: insert <span> before <p>.
	span, _ := s.Allocate()
	_ = s.SetKind(span, dom.NodeKindElement)
	_ = s.SetName(span, atom.AtomSpan)
	if err := s.InsertBefore(root, span, b); err != nil {
		t.Fatal(err)
	}

	// Verify order: div, span, p.
	var order []atom.Atom
	for c := s.FirstChild(root); c != dom.NodeNone; c = s.NextSibling(c) {
		order = append(order, s.Name(c))
	}
	if len(order) != 3 {
		t.Fatalf("child count = %d, want 3", len(order))
	}
	if order[0] != atom.AtomDiv || order[1] != atom.AtomSpan || order[2] != atom.AtomP {
		t.Errorf("order = %v, want [div span p]", order)
	}

	// Remove <span>.
	if err := s.RemoveChild(root, span); err != nil {
		t.Fatal(err)
	}
	childCount := 0
	for it := s.Children(root); it.Next(); {
		childCount++
	}
	if childCount != 2 {
		t.Errorf("child count after remove = %d, want 2", childCount)
	}

	// Replace <p> with <a> using Replace(old, new).
	replacement, _ := s.Allocate()
	_ = s.SetKind(replacement, dom.NodeKindElement)
	_ = s.SetName(replacement, atom.AtomA)
	if err := s.Replace(b, replacement); err != nil {
		t.Fatal(err)
	}
	if s.Name(s.FirstChild(root)) != atom.AtomDiv {
		t.Error("first child should still be div")
	}
}

// TestM54_ParserProducesValidStore verifies that the streaming parser
// produces a valid compact store that can be fully traversed without the adapter.
func TestM54_ParserProducesValidStore(t *testing.T) {
	input := `<html><head><title>Test</title></head><body><h1>Hello</h1><p class="intro">World</p></body></html>`
	p := dom.NewParser()
	doc, err := p.ParseDocumentCtx(context.Background(), strings.NewReader(input), dom.ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// Traverse the entire subtree and count nodes.
	count := 0
	for it := doc.Store.Subtree(doc.Root); it.Next(); {
		count++
	}
	if count < 6 {
		t.Errorf("node count = %d, want >= 6", count)
	}

	// Find specific elements by tag.
	foundH1 := false
	foundP := false
	for it := doc.Store.Subtree(doc.Root); it.Next(); {
		id := it.ID()
		if doc.Store.Kind(id) != dom.NodeKindElement {
			continue
		}
		switch doc.Store.Name(id) {
		case atom.AtomH1:
			foundH1 = true
		case atom.AtomP:
			foundP = true
			attrs, err := doc.Store.Attrs(id)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range attrs {
				if a.Name == atom.AttrClass {
					val := a.Value.String()
					if val != "intro" {
						t.Errorf("class = %q, want %q", val, "intro")
					}
				}
			}
		}
	}
	if !foundH1 {
		t.Error("h1 not found in parsed document")
	}
	if !foundP {
		t.Error("p not found in parsed document")
	}
}

package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

func newMutationTestStore(t *testing.T) (*dom.Store, dom.NodeID, dom.NodeID, dom.NodeID) {
	t.Helper()
	s := dom.NewStore(8)
	parent, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	text, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(parent, dom.NodeKindElement); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(child, dom.NodeKindElement); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(text, dom.NodeKindText); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendChild(parent, child); err != nil {
		t.Fatal(err)
	}
	return s, parent, child, text
}

func TestStoreApplyMutationSetText(t *testing.T) {
	s, parent, _, text := newMutationTestStore(t)
	if err := s.AppendChild(parent, text); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationSetText, Target: text, Value: "updated"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	if got != "updated" {
		t.Fatalf("text = %q, want updated", got)
	}
	if !s.HasFlag(text, dom.NodeFlagDirty) {
		t.Fatal("text node should be dirty")
	}
}

func TestStoreApplyMutationAttribute(t *testing.T) {
	s, _, child, _ := newMutationTestStore(t)
	name := atom.Intern("data-state")
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationSetAttribute, Target: child, Attribute: name, Value: "ready"}); err != nil {
		t.Fatal(err)
	}
	attrs, err := s.Attrs(child)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 1 || attrs[0].Name != name || attrs[0].Value.String() != "ready" {
		t.Fatalf("attrs = %#v, want data-state=ready", attrs)
	}
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationSetAttribute, Target: child, Attribute: name}); err != nil {
		t.Fatal(err)
	}
	attrs, err = s.Attrs(child)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 0 {
		t.Fatalf("attrs = %#v, want empty after removal", attrs)
	}
}

func TestStoreApplyMutationInsertRemoveReplace(t *testing.T) {
	s, parent, child, text := newMutationTestStore(t)
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationInsert, Parent: parent, Target: text}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationRemove, Parent: parent, Target: text}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMutation(dom.Mutation{Kind: dom.MutationReplace, Target: child, NewNode: text}); err != nil {
		t.Fatal(err)
	}
	rec, err := s.Get(text)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Parent != parent {
		t.Fatalf("parent = %v, want %v", rec.Parent, parent)
	}
	if !s.HasFlag(parent, dom.NodeFlagDirty) {
		t.Fatal("parent should be dirty")
	}
}

func BenchmarkStoreApplyMutationSetAttribute(b *testing.B) {
	s := dom.NewStore(1)
	node, err := s.Allocate()
	if err != nil {
		b.Fatal(err)
	}
	if err := s.SetKind(node, dom.NodeKindElement); err != nil {
		b.Fatal(err)
	}
	mutation := dom.Mutation{Kind: dom.MutationSetAttribute, Target: node, Attribute: atom.Intern("data-state"), Value: "ready"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.ApplyMutation(mutation); err != nil {
			b.Fatal(err)
		}
	}
}

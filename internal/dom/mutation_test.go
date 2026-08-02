package dom

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom/atom"
)

func newMutationTestStore(t *testing.T) (*Store, NodeID, NodeID, NodeID) {
	t.Helper()
	s := NewStore(8)
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
	if err := s.SetKind(parent, NodeKindElement); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(child, NodeKindElement); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(text, NodeKindText); err != nil {
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
	if err := s.ApplyMutation(Mutation{Kind: MutationSetText, Target: text, Value: "updated"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	if got != "updated" {
		t.Fatalf("text = %q, want updated", got)
	}
	if !s.HasFlag(text, NodeFlagDirty) {
		t.Fatal("text node should be dirty")
	}
}

func TestStoreApplyMutationAttribute(t *testing.T) {
	s, _, child, _ := newMutationTestStore(t)
	name := atom.Intern("data-state")
	if err := s.ApplyMutation(Mutation{Kind: MutationSetAttribute, Target: child, Attribute: name, Value: "ready"}); err != nil {
		t.Fatal(err)
	}
	attrs, err := s.Attrs(child)
	if err != nil {
		t.Fatal(err)
	}
	if len(attrs) != 1 || attrs[0].Name != name || attrs[0].Value.String() != "ready" {
		t.Fatalf("attrs = %#v, want data-state=ready", attrs)
	}
	if err := s.ApplyMutation(Mutation{Kind: MutationSetAttribute, Target: child, Attribute: name}); err != nil {
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
	if err := s.ApplyMutation(Mutation{Kind: MutationInsert, Parent: parent, Target: text}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMutation(Mutation{Kind: MutationRemove, Parent: parent, Target: text}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyMutation(Mutation{Kind: MutationReplace, Target: child, NewNode: text}); err != nil {
		t.Fatal(err)
	}
	rec, err := s.Get(text)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Parent != parent {
		t.Fatalf("parent = %v, want %v", rec.Parent, parent)
	}
	if !s.HasFlag(parent, NodeFlagDirty) {
		t.Fatal("parent should be dirty")
	}
}

func BenchmarkStoreApplyMutationSetAttribute(b *testing.B) {
	s := NewStore(1)
	node, err := s.Allocate()
	if err != nil {
		b.Fatal(err)
	}
	if err := s.SetKind(node, NodeKindElement); err != nil {
		b.Fatal(err)
	}
	mutation := Mutation{Kind: MutationSetAttribute, Target: node, Attribute: atom.Intern("data-state"), Value: "ready"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.ApplyMutation(mutation); err != nil {
			b.Fatal(err)
		}
	}
}

package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/dom/atom"
)

// TestAllocateAndBasicOperations tests basic node allocation and property access.
func TestAllocateAndBasicOperations(t *testing.T) {
	s := dom.NewStore(16)

	// Allocate a document node.
	doc, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if doc == dom.NodeNone {
		t.Fatal("allocated NodeNone")
	}

	// Set kind.
	if err := s.SetKind(doc, dom.NodeKindDocument); err != nil {
		t.Fatal(err)
	}
	if s.Kind(doc) != dom.NodeKindDocument {
		t.Errorf("Kind = %v, want Document", s.Kind(doc))
	}

	// Allocate an element node.
	div, err := s.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind(div, dom.NodeKindElement); err != nil {
		t.Fatal(err)
	}
	if err := s.SetName(div, atom.AtomDiv); err != nil {
		t.Fatal(err)
	}
	if s.Name(div) != atom.AtomDiv {
		t.Errorf("Name = %v, want Div", s.Name(div))
	}

	// Check validity.
	if !s.IsValid(doc) {
		t.Error("doc should be valid")
	}
	if !s.IsValid(div) {
		t.Error("div should be valid")
	}
	if s.IsValid(dom.NodeNone) {
		t.Error("NodeNone should be invalid")
	}
	if s.IsValid(999) {
		t.Error("out-of-bounds ID should be invalid")
	}

	// Check counts.
	if s.NodeCount() != 2 {
		t.Errorf("NodeCount = %d, want 2", s.NodeCount())
	}
}

// TestParentChildRelationships tests AppendChild, PrependChild, and sibling links.
func TestParentChildRelationships(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	p, _ := s.Allocate()
	s.SetKind(p, dom.NodeKindElement)
	s.SetName(p, atom.AtomP)

	// Append div1 to doc.
	if err := s.AppendChild(doc, div1); err != nil {
		t.Fatal(err)
	}
	if s.Parent(div1) != doc {
		t.Errorf("Parent(div1) = %v, want %v", s.Parent(div1), doc)
	}
	if s.FirstChild(doc) != div1 {
		t.Errorf("FirstChild(doc) = %v, want %v", s.FirstChild(doc), div1)
	}
	if s.LastChild(doc) != div1 {
		t.Errorf("LastChild(doc) = %v, want %v", s.LastChild(doc), div1)
	}

	// Append div2 to doc.
	if err := s.AppendChild(doc, div2); err != nil {
		t.Fatal(err)
	}
	if s.Parent(div2) != doc {
		t.Errorf("Parent(div2) = %v, want %v", s.Parent(div2), doc)
	}
	if s.NextSibling(div1) != div2 {
		t.Errorf("NextSibling(div1) = %v, want %v", s.NextSibling(div1), div2)
	}
	if s.PrevSibling(div2) != div1 {
		t.Errorf("PrevSibling(div2) = %v, want %v", s.PrevSibling(div2), div1)
	}
	if s.LastChild(doc) != div2 {
		t.Errorf("LastChild(doc) = %v, want %v", s.LastChild(doc), div2)
	}

	// Prepend p to doc.
	if err := s.PrependChild(doc, p); err != nil {
		t.Fatal(err)
	}
	if s.FirstChild(doc) != p {
		t.Errorf("FirstChild(doc) = %v, want %v", s.FirstChild(doc), p)
	}
	if s.NextSibling(p) != div1 {
		t.Errorf("NextSibling(p) = %v, want %v", s.NextSibling(p), div1)
	}
	if s.PrevSibling(div1) != p {
		t.Errorf("PrevSibling(div1) = %v, want %v", s.PrevSibling(div1), p)
	}

	// Check child count.
	if s.ChildCount(doc) != 3 {
		t.Errorf("ChildCount(doc) = %d, want 3", s.ChildCount(doc))
	}
}

// TestInsertBefore tests inserting a child before a reference node.
func TestInsertBefore(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	div3, _ := s.Allocate()
	s.SetKind(div3, dom.NodeKindElement)
	s.SetName(div3, atom.AtomDiv)

	// Build: doc -> [div1, div3]
	s.AppendChild(doc, div1)
	s.AppendChild(doc, div3)

	// Insert div2 before div3.
	if err := s.InsertBefore(doc, div2, div3); err != nil {
		t.Fatal(err)
	}

	// Check order: doc -> [div1, div2, div3]
	if s.FirstChild(doc) != div1 {
		t.Errorf("FirstChild = %v, want %v", s.FirstChild(doc), div1)
	}
	if s.NextSibling(div1) != div2 {
		t.Errorf("NextSibling(div1) = %v, want %v", s.NextSibling(div1), div2)
	}
	if s.NextSibling(div2) != div3 {
		t.Errorf("NextSibling(div2) = %v, want %v", s.NextSibling(div2), div3)
	}
	if s.ChildCount(doc) != 3 {
		t.Errorf("ChildCount = %d, want 3", s.ChildCount(doc))
	}
}

// TestStoreRemoveChild tests removing a child from its parent
// in the DOM store (the low-level tree mutation API used by the
// renderer for DOM tree operations).
func TestStoreRemoveChild(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	div3, _ := s.Allocate()
	s.SetKind(div3, dom.NodeKindElement)
	s.SetName(div3, atom.AtomDiv)

	// Build: doc -> [div1, div2, div3]
	s.AppendChild(doc, div1)
	s.AppendChild(doc, div2)
	s.AppendChild(doc, div3)

	// Remove div2 (middle).
	if err := s.RemoveChild(doc, div2); err != nil {
		t.Fatal(err)
	}
	if s.NextSibling(div1) != div3 {
		t.Errorf("NextSibling(div1) = %v, want %v", s.NextSibling(div1), div3)
	}
	if s.PrevSibling(div3) != div1 {
		t.Errorf("PrevSibling(div3) = %v, want %v", s.PrevSibling(div3), div1)
	}
	if s.Parent(div2) != dom.NodeNone {
		t.Errorf("Parent(div2) = %v, want NodeNone", s.Parent(div2))
	}
	if s.ChildCount(doc) != 2 {
		t.Errorf("ChildCount = %d, want 2", s.ChildCount(doc))
	}

	// Remove div1 (first).
	if err := s.RemoveChild(doc, div1); err != nil {
		t.Fatal(err)
	}
	if s.FirstChild(doc) != div3 {
		t.Errorf("FirstChild = %v, want %v", s.FirstChild(doc), div3)
	}
	if s.PrevSibling(div3) != dom.NodeNone {
		t.Errorf("PrevSibling(div3) = %v, want NodeNone", s.PrevSibling(div3))
	}

	// Remove div3 (only remaining).
	if err := s.RemoveChild(doc, div3); err != nil {
		t.Fatal(err)
	}
	if s.FirstChild(doc) != dom.NodeNone {
		t.Errorf("FirstChild = %v, want NodeNone", s.FirstChild(doc))
	}
	if s.LastChild(doc) != dom.NodeNone {
		t.Errorf("LastChild = %v, want NodeNone", s.LastChild(doc))
	}
}

// TestRemoveSubtree tests removing a node and all its descendants.
func TestRemoveSubtree(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	p1, _ := s.Allocate()
	s.SetKind(p1, dom.NodeKindElement)
	s.SetName(p1, atom.AtomP)

	p2, _ := s.Allocate()
	s.SetKind(p2, dom.NodeKindElement)
	s.SetName(p2, atom.AtomP)

	// Build: doc -> div -> [p1, p2]
	s.AppendChild(doc, div)
	s.AppendChild(div, p1)
	s.AppendChild(div, p2)

	if s.NodeCount() != 4 {
		t.Errorf("NodeCount = %d, want 4", s.NodeCount())
	}

	// Remove div (should free div, p1, p2).
	if err := s.Remove(div); err != nil {
		t.Fatal(err)
	}

	if s.NodeCount() != 1 {
		t.Errorf("NodeCount = %d, want 1", s.NodeCount())
	}
	if s.FirstChild(doc) != dom.NodeNone {
		t.Errorf("FirstChild(doc) = %v, want NodeNone", s.FirstChild(doc))
	}
	if s.IsValid(div) {
		t.Error("div should be invalid after removal")
	}
	if s.IsValid(p1) {
		t.Error("p1 should be invalid after removal")
	}
	if s.IsValid(p2) {
		t.Error("p2 should be invalid after removal")
	}
}

// TestReplace tests replacing a node with another.
func TestReplace(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	old, _ := s.Allocate()
	s.SetKind(old, dom.NodeKindElement)
	s.SetName(old, atom.AtomDiv)

	new, _ := s.Allocate()
	s.SetKind(new, dom.NodeKindElement)
	s.SetName(new, atom.AtomP)

	s.AppendChild(doc, old)

	// Replace old with new.
	if err := s.Replace(old, new); err != nil {
		t.Fatal(err)
	}

	if s.FirstChild(doc) != new {
		t.Errorf("FirstChild = %v, want %v", s.FirstChild(doc), new)
	}
	if s.Parent(new) != doc {
		t.Errorf("Parent(new) = %v, want %v", s.Parent(new), doc)
	}
	if s.IsValid(old) {
		t.Error("old should be invalid after replacement")
	}
}

// TestAttributes tests setting and getting attributes.
func TestAttributes(t *testing.T) {
	s := dom.NewStore(16)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	attrs := []dom.Attr{
		{Name: atom.AttrId, Value: atom.Intern("main")},
		{Name: atom.AttrClass, Value: atom.Intern("container")},
	}

	if err := s.SetAttrs(div, attrs); err != nil {
		t.Fatal(err)
	}

	got, err := s.Attrs(div)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("Attrs len = %d, want 2", len(got))
	}
	if got[0].Name != atom.AttrId || got[1].Name != atom.AttrClass {
		t.Errorf("Attrs = %v, want [id, class]", got)
	}

	if s.AttrCount() != 2 {
		t.Errorf("AttrCount = %d, want 2", s.AttrCount())
	}

	// Clear attributes.
	if err := s.SetAttrs(div, nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.Attrs(div)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Attrs len = %d, want 0", len(got))
	}
	if s.AttrCount() != 0 {
		t.Errorf("AttrCount = %d, want 0", s.AttrCount())
	}
}

// TestTextContent tests setting and getting text content.
func TestTextContent(t *testing.T) {
	s := dom.NewStore(16)

	text, _ := s.Allocate()
	s.SetKind(text, dom.NodeKindText)

	if err := s.SetText(text, "Hello, world!"); err != nil {
		t.Fatal(err)
	}

	got, err := s.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello, world!" {
		t.Errorf("Text = %q, want %q", got, "Hello, world!")
	}

	if s.TextBytes() != 13 {
		t.Errorf("TextBytes = %d, want 13", s.TextBytes())
	}

	// Update text.
	if err := s.SetText(text, "Goodbye!"); err != nil {
		t.Fatal(err)
	}
	got, err = s.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Goodbye!" {
		t.Errorf("Text = %q, want %q", got, "Goodbye!")
	}
}

// TestChildIterator tests zero-allocation child iteration.
func TestChildIterator(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	div3, _ := s.Allocate()
	s.SetKind(div3, dom.NodeKindElement)
	s.SetName(div3, atom.AtomDiv)

	s.AppendChild(doc, div1)
	s.AppendChild(doc, div2)
	s.AppendChild(doc, div3)

	// Iterate children.
	var children []dom.NodeID
	for it := s.Children(doc); it.Next(); {
		children = append(children, it.ID())
	}

	if len(children) != 3 {
		t.Fatalf("children len = %d, want 3", len(children))
	}
	if children[0] != div1 || children[1] != div2 || children[2] != div3 {
		t.Errorf("children = %v, want [%v, %v, %v]", children, div1, div2, div3)
	}
}

// TestSubtreeIterator tests zero-allocation subtree traversal.
func TestSubtreeIterator(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	p1, _ := s.Allocate()
	s.SetKind(p1, dom.NodeKindElement)
	s.SetName(p1, atom.AtomP)

	p2, _ := s.Allocate()
	s.SetKind(p2, dom.NodeKindElement)
	s.SetName(p2, atom.AtomP)

	s.AppendChild(doc, div)
	s.AppendChild(div, p1)
	s.AppendChild(div, p2)

	// Iterate subtree.
	var nodes []dom.NodeID
	for it := s.Subtree(doc); it.Next(); {
		nodes = append(nodes, it.ID())
	}

	if len(nodes) != 4 {
		t.Fatalf("subtree len = %d, want 4", len(nodes))
	}
	// Pre-order: doc, div, p1, p2
	if nodes[0] != doc || nodes[1] != div || nodes[2] != p1 || nodes[3] != p2 {
		t.Errorf("subtree = %v, want [%v, %v, %v, %v]", nodes, doc, div, p1, p2)
	}
}

// TestReverseChildIterator tests reverse child iteration.
func TestReverseChildIterator(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	div3, _ := s.Allocate()
	s.SetKind(div3, dom.NodeKindElement)
	s.SetName(div3, atom.AtomDiv)

	s.AppendChild(doc, div1)
	s.AppendChild(doc, div2)
	s.AppendChild(doc, div3)

	// Iterate in reverse.
	var children []dom.NodeID
	for it := s.ReverseChildren(doc); it.Next(); {
		children = append(children, it.ID())
	}

	if len(children) != 3 {
		t.Fatalf("children len = %d, want 3", len(children))
	}
	if children[0] != div3 || children[1] != div2 || children[2] != div1 {
		t.Errorf("children = %v, want [%v, %v, %v]", children, div3, div2, div1)
	}
}

// TestSiblingIterator tests sibling iteration.
func TestSiblingIterator(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	div3, _ := s.Allocate()
	s.SetKind(div3, dom.NodeKindElement)
	s.SetName(div3, atom.AtomDiv)

	s.AppendChild(doc, div1)
	s.AppendChild(doc, div2)
	s.AppendChild(doc, div3)

	// Iterate siblings starting from div2.
	var siblings []dom.NodeID
	for it := s.Siblings(div2); it.Next(); {
		siblings = append(siblings, it.ID())
	}

	if len(siblings) != 3 {
		t.Fatalf("siblings len = %d, want 3", len(siblings))
	}
	if siblings[0] != div1 || siblings[1] != div2 || siblings[2] != div3 {
		t.Errorf("siblings = %v, want [%v, %v, %v]", siblings, div1, div2, div3)
	}
}

// TestAncestorIterator tests ancestor iteration.
func TestAncestorIterator(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	p, _ := s.Allocate()
	s.SetKind(p, dom.NodeKindElement)
	s.SetName(p, atom.AtomP)

	s.AppendChild(doc, div)
	s.AppendChild(div, p)

	// Iterate ancestors from p.
	var ancestors []dom.NodeID
	for it := s.Ancestors(p); it.Next(); {
		ancestors = append(ancestors, it.ID())
	}

	if len(ancestors) != 3 {
		t.Fatalf("ancestors len = %d, want 3", len(ancestors))
	}
	// p, div, doc
	if ancestors[0] != p || ancestors[1] != div || ancestors[2] != doc {
		t.Errorf("ancestors = %v, want [%v, %v, %v]", ancestors, p, div, doc)
	}
}

// TestStaleHandleDetection tests that freed IDs are detected as stale.
func TestStaleHandleDetection(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	s.AppendChild(doc, div)

	// Remove div.
	if err := s.Remove(div); err != nil {
		t.Fatal(err)
	}

	// div should be invalid (Kind == 0).
	if s.IsValid(div) {
		t.Error("div should be invalid after removal")
	}

	// Allocate a new node (will reuse div's ID).
	new, _ := s.Allocate()
	s.SetKind(new, dom.NodeKindElement)
	s.SetName(new, atom.AtomP)

	// Old div ID is now valid again (reused with new Kind).
	// This is expected behavior with index-based IDs.
	// The caller must not hold onto old IDs after removal.
	if !s.IsValid(new) {
		t.Error("new node should be valid")
	}

	// But the old div ID and new ID are the same value.
	if div != new {
		t.Errorf("div (%v) should equal new (%v) since ID was reused", div, new)
	}
}

// TestReset tests clearing the store.
func TestReset(t *testing.T) {
	s := dom.NewStore(16)

	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	s.AppendChild(doc, div)
	s.SetText(div, "test")
	s.SetAttrs(div, []dom.Attr{{Name: atom.AttrId, Value: atom.Intern("test")}})

	if s.NodeCount() != 2 {
		t.Errorf("NodeCount = %d, want 2", s.NodeCount())
	}

	// Reset.
	s.Reset()

	if s.NodeCount() != 0 {
		t.Errorf("NodeCount = %d, want 0", s.NodeCount())
	}
	if s.AttrCount() != 0 {
		t.Errorf("AttrCount = %d, want 0", s.AttrCount())
	}
	if s.TextBytes() != 0 {
		t.Errorf("TextBytes = %d, want 0", s.TextBytes())
	}
}

// TestFlags tests setting and clearing flags.
func TestFlags(t *testing.T) {
	s := dom.NewStore(16)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	if s.HasFlag(div, dom.NodeFlagDirty) {
		t.Error("should not have Dirty flag initially")
	}

	if err := s.SetFlag(div, dom.NodeFlagDirty); err != nil {
		t.Fatal(err)
	}
	if !s.HasFlag(div, dom.NodeFlagDirty) {
		t.Error("should have Dirty flag after SetFlag")
	}

	if err := s.ClearFlag(div, dom.NodeFlagDirty); err != nil {
		t.Fatal(err)
	}
	if s.HasFlag(div, dom.NodeFlagDirty) {
		t.Error("should not have Dirty flag after ClearFlag")
	}
}

// TestRareData tests setting and getting rare metadata.
func TestRareData(t *testing.T) {
	s := dom.NewStore(16)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	data := dom.NewRareData(atom.Intern("svg"))
	if err := s.SetRareData(div, data); err != nil {
		t.Fatal(err)
	}

	got, ok := s.GetRareData(div)
	if !ok {
		t.Fatal("GetRareData returned false")
	}
	if got.Namespace != data.Namespace {
		t.Errorf("Namespace = %v, want %v", got.Namespace, data.Namespace)
	}
}

// TestEdgeCases tests various edge cases and error conditions.
func TestEdgeCases(t *testing.T) {
	s := dom.NewStore(16)

	// Invalid operations on NodeNone.
	if s.Kind(dom.NodeNone) != 0 {
		t.Error("Kind(NodeNone) should be 0")
	}
	if s.Name(dom.NodeNone) != atom.AtomNone {
		t.Error("Name(NodeNone) should be AtomNone")
	}
	if s.Parent(dom.NodeNone) != dom.NodeNone {
		t.Error("Parent(NodeNone) should be NodeNone")
	}
	if s.FirstChild(dom.NodeNone) != dom.NodeNone {
		t.Error("FirstChild(NodeNone) should be NodeNone")
	}
	if s.ChildCount(dom.NodeNone) != 0 {
		t.Error("ChildCount(NodeNone) should be 0")
	}

	// Operations on out-of-bounds ID.
	if s.IsValid(999) {
		t.Error("IsValid(999) should be false")
	}
	if _, err := s.Attrs(999); err != dom.ErrInvalidNodeID {
		t.Errorf("Attrs(999) error = %v, want ErrInvalidNodeID", err)
	}
	if _, err := s.Text(999); err != dom.ErrInvalidNodeID {
		t.Errorf("Text(999) error = %v, want ErrInvalidNodeID", err)
	}

	// AppendChild with invalid parent.
	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	if err := s.AppendChild(dom.NodeNone, div); err != dom.ErrInvalidParent {
		t.Errorf("AppendChild(NodeNone, div) error = %v, want ErrInvalidParent", err)
	}

	// AppendChild with invalid child.
	doc, _ := s.Allocate()
	s.SetKind(doc, dom.NodeKindDocument)
	if err := s.AppendChild(doc, dom.NodeNone); err != dom.ErrInvalidNodeID {
		t.Errorf("AppendChild(doc, NodeNone) error = %v, want ErrInvalidNodeID", err)
	}

	// RemoveChild with wrong parent.
	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	if err := s.RemoveChild(doc, div2); err != dom.ErrNodeNotFound {
		t.Errorf("RemoveChild(doc, div2) error = %v, want ErrNodeNotFound", err)
	}
}

// TestAttributeOffsetUpdates tests that attribute offsets are correctly updated
// when attributes are removed from one node, affecting other nodes.
func TestAttributeOffsetUpdates(t *testing.T) {
	s := dom.NewStore(16)

	div1, _ := s.Allocate()
	s.SetKind(div1, dom.NodeKindElement)
	s.SetName(div1, atom.AtomDiv)

	div2, _ := s.Allocate()
	s.SetKind(div2, dom.NodeKindElement)
	s.SetName(div2, atom.AtomDiv)

	// Set attrs on div1.
	attrs1 := []dom.Attr{
		{Name: atom.AttrId, Value: atom.Intern("first")},
		{Name: atom.AttrClass, Value: atom.Intern("a")},
	}
	s.SetAttrs(div1, attrs1)

	// Set attrs on div2.
	attrs2 := []dom.Attr{
		{Name: atom.AttrId, Value: atom.Intern("second")},
	}
	s.SetAttrs(div2, attrs2)

	// Clear div1's attrs (should shift div2's attrs).
	s.SetAttrs(div1, nil)

	// div2's attrs should still be accessible.
	got, err := s.Attrs(div2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("div2 attrs len = %d, want 1", len(got))
	}
	if got[0].Name != atom.AttrId {
		t.Errorf("div2 attr name = %v, want Id", got[0].Name)
	}
}

// TestTextOffsetUpdates tests that text offsets are correctly updated
// when text is removed from one node, affecting other nodes.
func TestTextOffsetUpdates(t *testing.T) {
	s := dom.NewStore(16)

	text1, _ := s.Allocate()
	s.SetKind(text1, dom.NodeKindText)
	s.SetText(text1, "First")

	text2, _ := s.Allocate()
	s.SetKind(text2, dom.NodeKindText)
	s.SetText(text2, "Second")

	// Clear text1 (should shift text2).
	s.SetText(text1, "")

	// text2 should still be accessible.
	got, err := s.Text(text2)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Second" {
		t.Errorf("text2 = %q, want %q", got, "Second")
	}
}

// TestMoveChild tests moving a child from one parent to another.
func TestMoveChild(t *testing.T) {
	s := dom.NewStore(16)

	doc1, _ := s.Allocate()
	s.SetKind(doc1, dom.NodeKindDocument)

	doc2, _ := s.Allocate()
	s.SetKind(doc2, dom.NodeKindDocument)

	div, _ := s.Allocate()
	s.SetKind(div, dom.NodeKindElement)
	s.SetName(div, atom.AtomDiv)

	// Add div to doc1.
	s.AppendChild(doc1, div)
	if s.Parent(div) != doc1 {
		t.Errorf("Parent = %v, want %v", s.Parent(div), doc1)
	}

	// Move div to doc2 (AppendChild should detach from doc1).
	s.AppendChild(doc2, div)
	if s.Parent(div) != doc2 {
		t.Errorf("Parent = %v, want %v", s.Parent(div), doc2)
	}
	if s.FirstChild(doc1) != dom.NodeNone {
		t.Errorf("FirstChild(doc1) = %v, want NodeNone", s.FirstChild(doc1))
	}
	if s.FirstChild(doc2) != div {
		t.Errorf("FirstChild(doc2) = %v, want %v", s.FirstChild(doc2), div)
	}
}

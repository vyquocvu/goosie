package dom

import (
	"context"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom/atom"
	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// TDD tests for M2.5 compatibility adapter
// ---------------------------------------------------------------------------

func TestAdapter_ElementNode(t *testing.T) {
	// Build a small document: <div id="main" class="content">hello</div>
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(`<div id="main" class="content">hello</div>`),
		ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// Find the div node in the store.
	divID := findNodeByTag(doc.Store, doc.Root, "div")
	if divID == NodeNone {
		t.Fatal("div node not found in store")
	}

	// Convert to *html.Node.
	adapter := NewNodeAdapter(doc.Store)
	htmlNode := adapter.ToHTMLNode(divID)
	if htmlNode == nil {
		t.Fatal("ToHTMLNode returned nil")
	}

	// Verify type and data.
	if htmlNode.Type != html.ElementNode {
		t.Errorf("Type = %v, want ElementNode", htmlNode.Type)
	}
	if htmlNode.Data != "div" {
		t.Errorf("Data = %q, want %q", htmlNode.Data, "div")
	}

	// Verify attributes.
	foundID, foundClass := false, false
	for _, attr := range htmlNode.Attr {
		switch attr.Key {
		case "id":
			if attr.Val == "main" {
				foundID = true
			}
		case "class":
			if attr.Val == "content" {
				foundClass = true
			}
		}
	}
	if !foundID {
		t.Error("missing id='main' attribute")
	}
	if !foundClass {
		t.Error("missing class='content' attribute")
	}

	// Verify child text node.
	if htmlNode.FirstChild == nil {
		t.Fatal("expected child text node")
	}
	if htmlNode.FirstChild.Type != html.TextNode {
		t.Errorf("child Type = %v, want TextNode", htmlNode.FirstChild.Type)
	}
	if htmlNode.FirstChild.Data != "hello" {
		t.Errorf("child Data = %q, want %q", htmlNode.FirstChild.Data, "hello")
	}
}

func TestAdapter_TextNode(t *testing.T) {
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(`plain text`), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// Find a text node in the store.
	textID := findNodeByKind(doc.Store, doc.Root, NodeKindText)
	if textID == NodeNone {
		t.Fatal("text node not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	htmlNode := adapter.ToHTMLNode(textID)
	if htmlNode == nil {
		t.Fatal("ToHTMLNode returned nil for text node")
	}
	if htmlNode.Type != html.TextNode {
		t.Errorf("Type = %v, want TextNode", htmlNode.Type)
	}
	if strings.TrimSpace(htmlNode.Data) != "plain text" {
		t.Errorf("Data = %q, want %q", htmlNode.Data, "plain text")
	}
}

func TestAdapter_CommentNode(t *testing.T) {
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(`<!-- a comment -->`), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	commentID := findNodeByKind(doc.Store, doc.Root, NodeKindComment)
	if commentID == NodeNone {
		t.Fatal("comment node not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	htmlNode := adapter.ToHTMLNode(commentID)
	if htmlNode == nil {
		t.Fatal("ToHTMLNode returned nil for comment node")
	}
	if htmlNode.Type != html.CommentNode {
		t.Errorf("Type = %v, want CommentNode", htmlNode.Type)
	}
	if !strings.Contains(htmlNode.Data, "a comment") {
		t.Errorf("Data = %q, want to contain %q", htmlNode.Data, "a comment")
	}
}

func TestAdapter_DocumentNode(t *testing.T) {
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(`<p>hi</p>`), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	adapter := NewNodeAdapter(doc.Store)
	htmlNode := adapter.ToHTMLNode(doc.Root)
	if htmlNode == nil {
		t.Fatal("ToHTMLNode returned nil for document node")
	}
	if htmlNode.Type != html.DocumentNode {
		t.Errorf("Type = %v, want DocumentNode", htmlNode.Type)
	}
}

func TestAdapter_NestedElements(t *testing.T) {
	input := `<div><p>para</p><span>text</span></div>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	divID := findNodeByTag(doc.Store, doc.Root, "div")
	if divID == NodeNone {
		t.Fatal("div not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	htmlNode := adapter.ToHTMLNode(divID)
	if htmlNode == nil {
		t.Fatal("nil result")
	}

	// Should have 2 element children: p and span.
	childCount := 0
	for c := htmlNode.FirstChild; c != nil; c = c.NextSibling {
		childCount++
	}
	if childCount != 2 {
		t.Errorf("child count = %d, want 2", childCount)
	}

	// First child should be <p>.
	if htmlNode.FirstChild.Data != "p" {
		t.Errorf("first child = %q, want %q", htmlNode.FirstChild.Data, "p")
	}
	// Second child should be <span>.
	if htmlNode.LastChild.Data != "span" {
		t.Errorf("last child = %q, want %q", htmlNode.LastChild.Data, "span")
	}
}

func TestAdapter_ParentLink(t *testing.T) {
	input := `<ul><li>item</li></ul>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	// Convert from the root to verify parent links are set correctly.
	adapter := NewNodeAdapter(doc.Store)
	root := adapter.ToHTMLNode(doc.Root)
	if root == nil {
		t.Fatal("nil root")
	}

	// Find the <li> node in the converted tree.
	liNode := findHTMLNodeByTag(root, "li")
	if liNode == nil {
		t.Fatal("li not found in converted tree")
	}
	if liNode.Parent == nil {
		t.Fatal("Parent is nil; want non-nil <ul>")
	}
	if liNode.Parent.Data != "ul" {
		t.Errorf("Parent.Data = %q, want %q", liNode.Parent.Data, "ul")
	}
}

func TestAdapter_SiblingLinks(t *testing.T) {
	input := `<ol><li>a</li><li>b</li><li>c</li></ol>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	olID := findNodeByTag(doc.Store, doc.Root, "ol")
	if olID == NodeNone {
		t.Fatal("ol not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	olNode := adapter.ToHTMLNode(olID)

	// Walk sibling chain.
	var names []string
	for c := olNode.FirstChild; c != nil; c = c.NextSibling {
		names = append(names, c.Data)
	}
	if len(names) != 3 {
		t.Fatalf("sibling count = %d, want 3", len(names))
	}
	for i, want := range []string{"li", "li", "li"} {
		if names[i] != want {
			t.Errorf("sibling[%d] = %q, want %q", i, names[i], want)
		}
	}

	// Verify PrevSibling chain.
	last := olNode.LastChild
	count := 0
	for c := last; c != nil; c = c.PrevSibling {
		count++
	}
	if count != 3 {
		t.Errorf("PrevSibling chain length = %d, want 3", count)
	}
}

func TestAdapter_InvalidNodeID(t *testing.T) {
	store := NewStore(16)
	adapter := NewNodeAdapter(store)

	// NodeNone should return nil.
	if n := adapter.ToHTMLNode(NodeNone); n != nil {
		t.Errorf("ToHTMLNode(NodeNone) = %v, want nil", n)
	}

	// Out-of-range ID should return nil.
	if n := adapter.ToHTMLNode(NodeID(9999)); n != nil {
		t.Errorf("ToHTMLNode(9999) = %v, want nil", n)
	}
}

func TestAdapter_StaleNodeID(t *testing.T) {
	store := NewStore(16)
	id, _ := store.Allocate()
	_ = store.SetKind(id, NodeKindElement)
	_ = store.SetName(id, atomDiv())

	// Remove the node (marks Kind=0).
	_ = store.Remove(id)

	adapter := NewNodeAdapter(store)
	if n := adapter.ToHTMLNode(id); n != nil {
		t.Errorf("ToHTMLNode(stale) = %v, want nil", n)
	}
}

func TestAdapter_UsageMetrics(t *testing.T) {
	// Reset global counter for test isolation.
	ResetAdapterMetrics()

	store := NewStore(16)
	id, _ := store.Allocate()
	_ = store.SetKind(id, NodeKindElement)
	_ = store.SetName(id, atomDiv())

	adapter := NewNodeAdapter(store)
	_ = adapter.ToHTMLNode(id)
	_ = adapter.ToHTMLNode(id)

	if got := AdapterUsageCount(); got != 2 {
		t.Errorf("AdapterUsageCount() = %d, want 2", got)
	}
}

func TestAdapter_FullDocument(t *testing.T) {
	input := `<!DOCTYPE html><html><head><title>T</title></head><body><p>hi</p></body></html>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	adapter := NewNodeAdapter(doc.Store)
	root := adapter.ToHTMLNode(doc.Root)
	if root == nil {
		t.Fatal("nil root")
	}
	if root.Type != html.DocumentNode {
		t.Errorf("root type = %v, want DocumentNode", root.Type)
	}

	// Walk the full tree and count nodes.
	count := countHTMLNodes(root)
	if count < 5 {
		t.Errorf("node count = %d, want >= 5", count)
	}
}

func TestAdapter_VoidElements(t *testing.T) {
	input := `<p>before<br>after</p>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	brID := findNodeByTag(doc.Store, doc.Root, "br")
	if brID == NodeNone {
		t.Fatal("br not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	brNode := adapter.ToHTMLNode(brID)
	if brNode == nil {
		t.Fatal("nil br node")
	}
	if brNode.Data != "br" {
		t.Errorf("Data = %q, want %q", brNode.Data, "br")
	}
	// Void elements should have no children.
	if brNode.FirstChild != nil {
		t.Error("br should have no children")
	}
}

func TestAdapter_MultipleAttributes(t *testing.T) {
	input := `<input type="text" name="q" value="search" disabled>`
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input), ParseConfig{})
	if err != nil {
		t.Fatal(err)
	}

	inputID := findNodeByTag(doc.Store, doc.Root, "input")
	if inputID == NodeNone {
		t.Fatal("input not found")
	}

	adapter := NewNodeAdapter(doc.Store)
	node := adapter.ToHTMLNode(inputID)
	if node == nil {
		t.Fatal("nil")
	}

	attrMap := make(map[string]string)
	for _, a := range node.Attr {
		attrMap[a.Key] = a.Val
	}
	for _, want := range []string{"type", "name", "value", "disabled"} {
		if _, ok := attrMap[want]; !ok {
			t.Errorf("missing attribute %q", want)
		}
	}
}

func TestAdapter_EmptyStore(t *testing.T) {
	store := NewStore(8)
	adapter := NewNodeAdapter(store)
	if n := adapter.ToHTMLNode(NodeNone); n != nil {
		t.Errorf("expected nil for empty store, got %v", n)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkAdapter_SmallHTML(b *testing.B) {
	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(`<div id="main"><p>hello</p></div>`), ParseConfig{})
	if err != nil {
		b.Fatal(err)
	}
	adapter := NewNodeAdapter(doc.Store)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = adapter.ToHTMLNode(doc.Root)
	}
}

func BenchmarkAdapter_LargeHTML(b *testing.B) {
	// Build a larger fixture.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for i := 0; i < 100; i++ {
		sb.WriteString(`<div class="item"><p>text</p><span>more</span></div>`)
	}
	sb.WriteString("</body></html>")

	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(sb.String()), ParseConfig{})
	if err != nil {
		b.Fatal(err)
	}
	adapter := NewNodeAdapter(doc.Store)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = adapter.ToHTMLNode(doc.Root)
	}
}

func BenchmarkAdapter_TableHeavy(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("<html><body><table>")
	for i := 0; i < 50; i++ {
		sb.WriteString("<tr><td>a</td><td>b</td><td>c</td></tr>")
	}
	sb.WriteString("</table></body></html>")

	doc, err := newTestParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(sb.String()), ParseConfig{})
	if err != nil {
		b.Fatal(err)
	}
	adapter := NewNodeAdapter(doc.Store)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = adapter.ToHTMLNode(doc.Root)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestParser() *Parser {
	return NewParser()
}

func findNodeByTag(s *Store, root NodeID, tag string) NodeID {
	tagAtom := internTag(tag)
	for it := s.Subtree(root); it.Next(); {
		id := it.ID()
		if s.Kind(id) == NodeKindElement && s.Name(id) == tagAtom {
			return id
		}
	}
	return NodeNone
}

func findNodeByKind(s *Store, root NodeID, kind NodeKind) NodeID {
	for it := s.Subtree(root); it.Next(); {
		id := it.ID()
		if s.Kind(id) == kind {
			return id
		}
	}
	return NodeNone
}

func countHTMLNodes(n *html.Node) int {
	if n == nil {
		return 0
	}
	count := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		count += countHTMLNodes(c)
	}
	return count
}

func findHTMLNodeByTag(n *html.Node, tag string) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findHTMLNodeByTag(c, tag); found != nil {
			return found
		}
	}
	return nil
}

func atomDiv() atom.Atom {
	return atom.AtomDiv
}

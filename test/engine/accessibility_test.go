package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

// axFind returns the first node in the tree with the given role and label.
func axFind(nodes []ax.Node, role ax.Role, label string) *ax.Node {
	for i := range nodes {
		if nodes[i].Role == role && nodes[i].Label == label {
			return &nodes[i]
		}
		if got := axFind(nodes[i].Children, role, label); got != nil {
			return got
		}
	}
	return nil
}

func axFlatten(nodes []ax.Node) []ax.Node {
	var out []ax.Node
	for _, n := range nodes {
		out = append(out, n)
		out = append(out, axFlatten(n.Children)...)
	}
	return out
}

func TestAccessibilityDocumentRoot(t *testing.T) {
	s := newFocusSession(t, `<html><body><p>hi</p></body></html>`)
	tree := s.AccessibilityTree()
	if len(tree) != 1 {
		t.Fatalf("tree roots = %d, want 1 document", len(tree))
	}
	if tree[0].Role != ax.RoleDocument {
		t.Fatalf("root role = %d, want document", tree[0].Role)
	}
	if tree[0].X1 <= tree[0].X0 || tree[0].Y1 <= tree[0].Y0 {
		t.Fatalf("document bounds empty: %+v", tree[0])
	}
}

const axRichHTML = `<html><body style="margin: 0;">` +
	`<p>Hello <a href="https://example.com/docs" id="docs">goosie docs</a> world</p>` +
	`<img src="missing.png" alt="Go logo" width="40" height="40">` +
	`<button id="btn">Click me</button>` +
	`<input id="field" type="text" value="typed" placeholder="Your name">` +
	`<textarea id="area">line one</textarea>` +
	`</body></html>`

func TestAccessibilityLinkRoleAndHref(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	link := axFind(s.AccessibilityTree(), ax.RoleLink, "goosie docs")
	if link == nil {
		t.Fatal("link node not found")
	}
	if link.Href != "https://example.com/docs" {
		t.Fatalf("href = %q, want https://example.com/docs", link.Href)
	}
	if link.X1 <= link.X0 || link.Y1 <= link.Y0 {
		t.Fatalf("link bounds empty: %+v", link)
	}
}

func TestAccessibilityImageAlt(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	img := axFind(s.AccessibilityTree(), ax.RoleImage, "Go logo")
	if img == nil {
		t.Fatal("image node not found")
	}
	if img.X1 <= img.X0 || img.Y1 <= img.Y0 {
		t.Fatalf("image bounds empty: %+v", img)
	}
}

func TestAccessibilityButtonLabel(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	btn := axFind(s.AccessibilityTree(), ax.RoleButton, "Click me")
	if btn == nil {
		t.Fatal("button node not found")
	}
}

func TestAccessibilityTextFieldValue(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	field := axFind(s.AccessibilityTree(), ax.RoleTextField, "Your name")
	if field == nil {
		t.Fatal("text field node not found")
	}
	if field.Value != "typed" {
		t.Fatalf("value = %q, want typed", field.Value)
	}
}

func TestAccessibilityTextareaValue(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	for _, n := range axFlatten(s.AccessibilityTree()) {
		if n.Role == ax.RoleTextField && n.Value == "line one" {
			return
		}
	}
	t.Fatalf("textarea node with value %q not found", "line one")
}

func TestAccessibilityStaticTextParagraphs(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;"><p>First</p><p>Second</p></body></html>`)
	flat := axFlatten(s.AccessibilityTree())
	first := -1
	second := -1
	groups := 0
	for i, n := range flat {
		switch {
		case n.Role == ax.RoleStaticText && n.Label == "First":
			first = i
		case n.Role == ax.RoleStaticText && n.Label == "Second":
			second = i
		case n.Role == ax.RoleGroup:
			groups++
		}
	}
	if first < 0 || second < 0 {
		t.Fatalf("static text nodes missing: First=%d Second=%d in %+v", first, second, flat)
	}
	if first >= second {
		t.Fatalf("paragraph order wrong: First at %d, Second at %d", first, second)
	}
	if groups == 0 {
		t.Fatal("no group nodes around the blocks")
	}
}

func TestAccessibilityTextRunsAroundLink(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	flat := axFlatten(s.AccessibilityTree())
	hello := -1
	link := -1
	world := -1
	for i, n := range flat {
		switch {
		case n.Role == ax.RoleStaticText && n.Label == "Hello":
			hello = i
		case n.Role == ax.RoleLink && n.Label == "goosie docs":
			link = i
		case n.Role == ax.RoleStaticText && n.Label == "world":
			world = i
		}
	}
	if hello < 0 || link < 0 || world < 0 {
		t.Fatalf("missing runs: Hello=%d link=%d world=%d in %+v", hello, link, world, flat)
	}
	if !(hello < link && link < world) {
		t.Fatalf("runs out of document order: Hello=%d link=%d world=%d", hello, link, world)
	}
}

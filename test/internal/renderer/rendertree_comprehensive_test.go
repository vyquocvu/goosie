package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestBuildRenderTree_ComprehensiveSuite provides extensive test coverage for BuildRenderTree
// with 50 test cases covering various HTML structures, edge cases, and scenarios
func TestBuildRenderTree_ComprehensiveSuite(t *testing.T) {
	tests := []struct {
		name              string
		html              string
		validate          func(t *testing.T, tree *renderer.RenderNode)
		expectNil         bool
		skipChildrenCheck bool
	}{
		// Basic HTML Elements (1-10)
		{
			name: "1. Empty div element",
			html: "<div></div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 0 {
					t.Errorf("Expected 0 children, got %d", len(tree.Children))
				}
			},
		},
		{
			name: "2. Single paragraph with text",
			html: "<p>Simple text content</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "p" {
					t.Errorf("Expected 'p', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 1 {
					t.Errorf("Expected 1 child, got %d", len(tree.Children))
				}
				if tree.Children[0].Type != renderer.NodeTypeText {
					t.Error("Expected text node child")
				}
			},
		},
		{
			name: "3. Heading h1 element",
			html: "<h1>Main Heading</h1>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "h1" {
					t.Errorf("Expected 'h1', got '%s'", tree.TagName)
				}
				if !tree.IsBlock() {
					t.Error("h1 should be block element")
				}
			},
		},
		{
			name: "4. Heading h2 element",
			html: "<h2>Subheading</h2>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "h2" {
					t.Errorf("Expected 'h2', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "5. Heading h3 element",
			html: "<h3>Section Title</h3>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "h3" {
					t.Errorf("Expected 'h3', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "6. Span inline element",
			html: "<span>Inline text</span>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "span" {
					t.Errorf("Expected 'span', got '%s'", tree.TagName)
				}
				if tree.IsBlock() {
					t.Error("span should be inline element")
				}
			},
		},
		{
			name: "7. Anchor link element",
			html: `<a href="https://example.com">Link</a>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "a" {
					t.Errorf("Expected 'a', got '%s'", tree.TagName)
				}
				href, ok := tree.GetAttribute("href")
				if !ok || href != "https://example.com" {
					t.Errorf("Expected href='https://example.com', got '%s'", href)
				}
			},
		},
		{
			name: "8. Strong element for bold",
			html: "<strong>Bold text</strong>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "strong" {
					t.Errorf("Expected 'strong', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "9. Em element for italic",
			html: "<em>Italic text</em>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "em" {
					t.Errorf("Expected 'em', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "10. Unordered list",
			html: "<ul><li>Item 1</li><li>Item 2</li></ul>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "ul" {
					t.Errorf("Expected 'ul', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 2 {
					t.Errorf("Expected 2 children, got %d", len(tree.Children))
				}
			},
		},

		// Nested Structures (11-20)
		{
			name: "11. Deeply nested divs",
			html: "<div><div><div>Nested content</div></div></div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 1 {
					t.Errorf("Expected 1 child, got %d", len(tree.Children))
				}
				if tree.Children[0].TagName != "div" {
					t.Error("Expected nested div")
				}
			},
		},
		{
			name: "12. Multiple sibling elements",
			html: "<div><p>Para 1</p><p>Para 2</p><p>Para 3</p></div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if len(tree.Children) != 3 {
					t.Errorf("Expected 3 children, got %d", len(tree.Children))
				}
			},
		},
		{
			name: "13. Mixed block and inline elements",
			html: "<div><p>Text with <strong>bold</strong> and <em>italic</em></p></div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 1 {
					t.Errorf("Expected 1 child, got %d", len(tree.Children))
				}
			},
		},
		{
			name: "14. List with nested lists",
			html: "<ul><li>Item 1<ul><li>Nested 1</li></ul></li><li>Item 2</li></ul>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "ul" {
					t.Errorf("Expected 'ul', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "15. Table structure",
			html: "<table><tr><td>Cell 1</td><td>Cell 2</td></tr></table>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "table" {
					t.Errorf("Expected 'table', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "16. Section with header",
			html: "<section><header><h1>Title</h1></header><p>Content</p></section>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "section" {
					t.Errorf("Expected 'section', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "17. Article with multiple sections",
			html: "<article><section><p>Section 1</p></section><section><p>Section 2</p></section></article>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "article" {
					t.Errorf("Expected 'article', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "18. Nav with links",
			html: `<nav><a href="/">Home</a><a href="/about">About</a></nav>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "nav" {
					t.Errorf("Expected 'nav', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "19. Footer with copyright",
			html: "<footer><p>&copy; 2024 Company</p></footer>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "footer" {
					t.Errorf("Expected 'footer', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "20. Main content area",
			html: "<main><h1>Welcome</h1><p>Content goes here</p></main>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "main" {
					t.Errorf("Expected 'main', got '%s'", tree.TagName)
				}
			},
		},

		// Attributes and Special Cases (21-30)
		{
			name: "21. Element with ID attribute",
			html: `<div id="unique-id">Content</div>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				id, ok := tree.GetAttribute("id")
				if !ok || id != "unique-id" {
					t.Errorf("Expected id='unique-id', got '%s'", id)
				}
			},
		},
		{
			name: "22. Element with class attribute",
			html: `<div class="container main">Content</div>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				class, ok := tree.GetAttribute("class")
				if !ok || class != "container main" {
					t.Errorf("Expected class='container main', got '%s'", class)
				}
			},
		},
		{
			name: "23. Element with multiple attributes",
			html: `<div id="test" class="box" data-value="123">Content</div>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if len(tree.Attrs) != 3 {
					t.Errorf("Expected 3 attributes, got %d", len(tree.Attrs))
				}
			},
		},
		{
			name: "24. Image with src and alt",
			html: `<img src="image.jpg" alt="Description">`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "img" {
					t.Errorf("Expected 'img', got '%s'", tree.TagName)
				}
				src, ok := tree.GetAttribute("src")
				if !ok || src != "image.jpg" {
					t.Errorf("Expected src='image.jpg', got '%s'", src)
				}
			},
		},
		{
			name: "25. Link with target attribute",
			html: `<a href="https://example.com" target="_blank">Link</a>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				target, ok := tree.GetAttribute("target")
				if !ok || target != "_blank" {
					t.Errorf("Expected target='_blank', got '%s'", target)
				}
			},
		},
		{
			name: "26. Element with data attributes",
			html: `<div data-id="123" data-name="test">Content</div>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				dataID, ok := tree.GetAttribute("data-id")
				if !ok || dataID != "123" {
					t.Errorf("Expected data-id='123', got '%s'", dataID)
				}
			},
		},
		{
			name: "27. Input element with type",
			html: `<input type="text" name="username">`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "input" {
					t.Errorf("Expected 'input', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "28. Button element",
			html: `<button type="submit">Click Me</button>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "button" {
					t.Errorf("Expected 'button', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "29. Form element",
			html: `<form action="/submit" method="post"><input type="text"></form>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "form" {
					t.Errorf("Expected 'form', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "30. Label element",
			html: `<label for="input1">Label Text</label>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "label" {
					t.Errorf("Expected 'label', got '%s'", tree.TagName)
				}
			},
		},

		// Text Content and Special Characters (31-40)
		{
			name: "31. Text with whitespace",
			html: "<p>  Text with   spaces  </p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if len(tree.Children) == 0 {
					t.Error("Expected text node child")
				}
			},
		},
		{
			name: "32. Text with newlines",
			html: "<p>Line 1\nLine 2\nLine 3</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if len(tree.Children) == 0 {
					t.Error("Expected text node child")
				}
			},
		},
		{
			name: "33. Text with special HTML entities",
			html: "<p>&lt;HTML&gt; &amp; &quot;quotes&quot;</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "p" {
					t.Errorf("Expected 'p', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "34. Empty text nodes filtered",
			html: "<div>   </div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "35. Unicode text content",
			html: "<p>Hello 世界 🌍</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if len(tree.Children) == 0 {
					t.Error("Expected text node child")
				}
			},
		},
		{
			name: "36. Long text content",
			html: "<p>" + strings.Repeat("Long text content. ", 50) + "</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "p" {
					t.Errorf("Expected 'p', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "37. Code element",
			html: "<code>const x = 42;</code>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "code" {
					t.Errorf("Expected 'code', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "38. Pre element with formatting",
			html: "<pre>  Preformatted\n  text  </pre>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "pre" {
					t.Errorf("Expected 'pre', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "39. Blockquote element",
			html: "<blockquote>Quote text here</blockquote>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "blockquote" {
					t.Errorf("Expected 'blockquote', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "40. Mixed text and inline elements",
			html: "<p>Text <strong>bold</strong> more text <em>italic</em> end</p>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "p" {
					t.Errorf("Expected 'p', got '%s'", tree.TagName)
				}
			},
		},

		// Edge Cases and Filtered Elements (41-50)
		{
			name: "41. Script tag not filtered",
			html: "<script>alert('test');</script>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "script" {
					t.Errorf("Expected 'script', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "42. Style tag not filtered",
			html: "<style>body { color: red; }</style>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "style" {
					t.Errorf("Expected 'style', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "43. Meta tag not filtered",
			html: `<meta name="description" content="test">`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "meta" {
					t.Errorf("Expected 'meta', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "44. Link tag not filtered",
			html: `<link rel="stylesheet" href="style.css">`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "link" {
					t.Errorf("Expected 'link', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "45. Head tag not filtered",
			html: "<head><title>Test</title></head>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "head" {
					t.Errorf("Expected 'head', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "46. Br element",
			html: "<div>Line 1<br>Line 2</div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "47. Hr element",
			html: "<div><p>Text</p><hr><p>More text</p></div>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "div" {
					t.Errorf("Expected 'div', got '%s'", tree.TagName)
				}
			},
		},
		{
			name: "48. Complex real-world structure",
			html: `<article class="post"><header><h2>Title</h2><span class="date">2024-01-01</span></header><section class="content"><p>First paragraph.</p><p>Second paragraph with <a href="/link">link</a>.</p></section><footer><div class="tags"><span>tag1</span><span>tag2</span></div></footer></article>`,
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "article" {
					t.Errorf("Expected 'article', got '%s'", tree.TagName)
				}
				class, ok := tree.GetAttribute("class")
				if !ok || class != "post" {
					t.Error("Expected class='post'")
				}
			},
		},
		{
			name: "49. Ordered list",
			html: "<ol><li>First</li><li>Second</li><li>Third</li></ol>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "ol" {
					t.Errorf("Expected 'ol', got '%s'", tree.TagName)
				}
				if len(tree.Children) != 3 {
					t.Errorf("Expected 3 children, got %d", len(tree.Children))
				}
			},
		},
		{
			name: "50. Aside element",
			html: "<aside><h3>Related</h3><ul><li>Link 1</li><li>Link 2</li></ul></aside>",
			validate: func(t *testing.T, tree *renderer.RenderNode) {
				if tree.TagName != "aside" {
					t.Errorf("Expected 'aside', got '%s'", tree.TagName)
				}
				if !tree.IsBlock() {
					t.Error("aside should be block element")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Find the target element node
			var elementNode *html.Node
			var findElement func(*html.Node)
			findElement = func(n *html.Node) {
				if elementNode != nil {
					return
				}
				if n.Type == html.ElementNode && n.Data != "html" && n.Data != "body" && (n.Data != "head" || strings.Contains(tt.name, "Head tag")) {
					elementNode = n
					return
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					findElement(c)
				}
			}
			findElement(doc)

			if elementNode == nil && !tt.expectNil {
				t.Fatal("Could not find element node in parsed HTML")
			}

			renderTree := renderer.BuildRenderTree(elementNode)

			if tt.expectNil {
				if renderTree != nil {
					t.Errorf("Expected nil render tree, got tree with tag '%s'", renderTree.TagName)
				}
				return
			}

			if renderTree == nil {
				t.Fatal("BuildRenderTree returned nil unexpectedly")
			}

			// Run custom validation
			if tt.validate != nil {
				tt.validate(t, renderTree)
			}

			// Verify basic properties
			if renderTree.ID == 0 {
				t.Error("RenderNode ID should be non-zero")
			}
			if renderTree.Attrs == nil {
				t.Error("Attrs map should be initialized")
			}
			if renderTree.Children == nil {
				t.Error("Children slice should be initialized")
			}

			// Verify parent-child relationships
			for _, child := range renderTree.Children {
				if child.Parent != renderTree {
					t.Error("Child's parent pointer not set correctly")
				}
			}
		})
	}
}

// TestBuildRenderTree_EdgeCases tests additional edge cases
func TestBuildRenderTree_EdgeCases(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		tree := renderer.BuildRenderTree(nil)
		if tree != nil {
			t.Error("Expected nil for nil input")
		}
	})

	t.Run("comment node", func(t *testing.T) {
		htmlContent := "<!-- This is a comment --><div>Content</div>"
		doc, _ := html.Parse(strings.NewReader(htmlContent))

		var commentNode *html.Node
		var findComment func(*html.Node)
		findComment = func(n *html.Node) {
			if n.Type == html.CommentNode {
				commentNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if commentNode == nil {
					findComment(c)
				}
			}
		}
		findComment(doc)

		if commentNode != nil {
			tree := renderer.BuildRenderTree(commentNode)
			if tree != nil {
				t.Error("Expected nil for comment node")
			}
		}
	})

	t.Run("doctype node", func(t *testing.T) {
		htmlContent := "<!DOCTYPE html><div>Content</div>"
		doc, _ := html.Parse(strings.NewReader(htmlContent))

		var doctypeNode *html.Node
		var findDoctype func(*html.Node)
		findDoctype = func(n *html.Node) {
			if n.Type == html.DoctypeNode {
				doctypeNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if doctypeNode == nil {
					findDoctype(c)
				}
			}
		}
		findDoctype(doc)

		if doctypeNode != nil {
			tree := renderer.BuildRenderTree(doctypeNode)
			if tree != nil {
				t.Error("Expected nil for doctype node")
			}
		}
	})
}

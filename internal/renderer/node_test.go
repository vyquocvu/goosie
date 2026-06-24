package renderer

import (
	"strings"
	"testing"
	
	"golang.org/x/net/html"
)

func TestNewRenderNode(t *testing.T) {
	node := NewRenderNode(NodeTypeElement)
	if node == nil {
		t.Fatal("NewRenderNode returned nil")
	}
	if node.Type != NodeTypeElement {
		t.Errorf("Expected NodeTypeElement, got %v", node.Type)
	}
	if node.Attrs == nil {
		t.Error("Attrs map not initialized")
	}
	if node.Children == nil {
		t.Error("Children slice not initialized")
	}
	if node.Box == nil {
		t.Error("Box not initialized")
	}
}

func TestRenderNodeAddChild(t *testing.T) {
	parent := NewRenderNode(NodeTypeElement)
	child := NewRenderNode(NodeTypeText)
	
	parent.AddChild(child)
	
	if len(parent.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(parent.Children))
	}
	if child.Parent != parent {
		t.Error("Child's parent not set correctly")
	}
}

func TestRenderNodeAttributes(t *testing.T) {
	node := NewRenderNode(NodeTypeElement)
	
	node.SetAttribute("id", "test-id")
	node.SetAttribute("class", "test-class")
	
	id, ok := node.GetAttribute("id")
	if !ok || id != "test-id" {
		t.Errorf("Expected id='test-id', got '%s' (exists: %v)", id, ok)
	}
	
	class, ok := node.GetAttribute("class")
	if !ok || class != "test-class" {
		t.Errorf("Expected class='test-class', got '%s' (exists: %v)", class, ok)
	}
	
	_, ok = node.GetAttribute("nonexistent")
	if ok {
		t.Error("GetAttribute returned true for nonexistent attribute")
	}
}

func TestRenderNodeIsBlock(t *testing.T) {
	tests := []struct {
		tagName string
		isBlock bool
	}{
		{"div", true},
		{"p", true},
		{"h1", true},
		{"h2", true},
		{"ul", true},
		{"li", true},
		{"span", false},
		{"a", false},
		{"strong", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			node := NewRenderNode(NodeTypeElement)
			node.TagName = tt.tagName
			
			if node.IsBlock() != tt.isBlock {
				t.Errorf("%s: expected IsBlock=%v, got %v", tt.tagName, tt.isBlock, node.IsBlock())
			}
		})
	}
}

func TestBuildRenderTree(t *testing.T) {
	tests := []struct {
		name          string
		html          string
		expectedTag   string
		expectedChildren int
	}{
		{
			name:          "simple div",
			html:          "<div>Hello</div>",
			expectedTag:   "div",
			expectedChildren: 1,
		},
		{
			name:          "nested elements",
			html:          "<div><p>Para 1</p><p>Para 2</p></div>",
			expectedTag:   "div",
			expectedChildren: 2,
		},
		{
			name:          "heading",
			html:          "<h1>Title</h1>",
			expectedTag:   "h1",
			expectedChildren: 1,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}
			
			// Find the first element node
			var elementNode *html.Node
			var findElement func(*html.Node)
			findElement = func(n *html.Node) {
				if n.Type == html.ElementNode && n.Data != "html" && n.Data != "head" && n.Data != "body" {
					elementNode = n
					return
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if elementNode == nil {
						findElement(c)
					}
				}
			}
			findElement(doc)
			
			if elementNode == nil {
				t.Fatal("Could not find element node in parsed HTML")
			}
			
			renderTree := BuildRenderTree(elementNode)
			if renderTree == nil {
				t.Fatal("BuildRenderTree returned nil")
			}
			
			if renderTree.TagName != tt.expectedTag {
				t.Errorf("Expected tag '%s', got '%s'", tt.expectedTag, renderTree.TagName)
			}
			
			if len(renderTree.Children) != tt.expectedChildren {
				t.Errorf("Expected %d children, got %d", tt.expectedChildren, len(renderTree.Children))
			}
		})
	}
}

func TestBuildRenderTreeWithAttributes(t *testing.T) {
	htmlContent := `<div id="main" class="container">Content</div>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	
	// Find the div element
	var divNode *html.Node
	var findDiv func(*html.Node)
	findDiv = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			divNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if divNode == nil {
				findDiv(c)
			}
		}
	}
	findDiv(doc)
	
	if divNode == nil {
		t.Fatal("Could not find div node")
	}
	
	renderTree := BuildRenderTree(divNode)
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}
	
	id, hasID := renderTree.GetAttribute("id")
	if !hasID || id != "main" {
		t.Errorf("Expected id='main', got '%s' (exists: %v)", id, hasID)
	}
	
	class, hasClass := renderTree.GetAttribute("class")
	if !hasClass || class != "container" {
		t.Errorf("Expected class='container', got '%s' (exists: %v)", class, hasClass)
	}
}

func TestBuildRenderTreeTextNode(t *testing.T) {
	htmlContent := `<p>Hello World</p>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	
	// Find the p element
	var pNode *html.Node
	var findP func(*html.Node)
	findP = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "p" {
			pNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if pNode == nil {
				findP(c)
			}
		}
	}
	findP(doc)
	
	if pNode == nil {
		t.Fatal("Could not find p node")
	}
	
	renderTree := BuildRenderTree(pNode)
	if renderTree == nil {
		t.Fatal("BuildRenderTree returned nil")
	}
	
	if len(renderTree.Children) == 0 {
		t.Fatal("Expected text node child")
	}
	
	textNode := renderTree.Children[0]
	if textNode.Type != NodeTypeText {
		t.Errorf("Expected NodeTypeText, got %v", textNode.Type)
	}
	if textNode.Text == "" {
		t.Error("Text node has empty text")
	}
}

func TestBuildRenderTreeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected []string
	}{
		{
			name:     "leading and trailing spaces preserved",
			html:     "<p> Hello <span>World</span> </p>",
			expected: []string{" Hello ", "World", " "},
		},
		{
			name:     "multiple spaces collapsed",
			html:     "<p>  Hello   World  </p>",
			expected: []string{" Hello World "},
		},
		{
			name:     "newlines converted to spaces",
			html:     "<p>\nHello\nWorld\n</p>",
			expected: []string{" Hello World "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			var pNode *html.Node
			var findP func(*html.Node)
			findP = func(n *html.Node) {
				if n.Type == html.ElementNode && n.Data == "p" {
					pNode = n
					return
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if pNode == nil {
						findP(c)
					}
				}
			}
			findP(doc)

			if pNode == nil {
				t.Fatal("Could not find p node")
			}

			renderTree := BuildRenderTree(pNode)
			if renderTree == nil {
				t.Fatal("BuildRenderTree returned nil")
			}

			// Collect all text nodes
			var texts []string
			var collectTexts func(*RenderNode)
			collectTexts = func(n *RenderNode) {
				if n.Type == NodeTypeText {
					texts = append(texts, n.Text)
				}
				for _, child := range n.Children {
					collectTexts(child)
				}
			}
			collectTexts(renderTree)

			if len(texts) != len(tt.expected) {
				t.Fatalf("Expected %d text nodes, got %d. Texts: %v", len(tt.expected), len(texts), texts)
			}

			for i, text := range texts {
				if text != tt.expected[i] {
					t.Errorf("At index %d: expected '%s', got '%s'", i, tt.expected[i], text)
				}
			}
		})
	}
}


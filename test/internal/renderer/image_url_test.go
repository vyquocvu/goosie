package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRendererResolveURL(t *testing.T) {
	r := renderer.NewRenderer(800, 600)

	tests := []struct {
		name       string
		currentURL string
		href       string
		expected   string
	}{
		{
			name:       "absolute URL",
			currentURL: "https://example.com/page",
			href:       "https://other.com/path",
			expected:   "https://other.com/path",
		},
		{
			name:       "relative path",
			currentURL: "https://example.com/path/page.html",
			href:       "other.html",
			expected:   "https://example.com/path/other.html",
		},
		{
			name:       "root relative path",
			currentURL: "https://example.com/path/page.html",
			href:       "/other/page.html",
			expected:   "https://example.com/other/page.html",
		},
		{
			name:       "no current URL",
			currentURL: "",
			href:       "/page.html",
			expected:   "/page.html",
		},
		{
			name:       "http URL",
			currentURL: "https://example.com/",
			href:       "http://other.com/",
			expected:   "http://other.com/",
		},
		{
			name:       "relative image path",
			currentURL: "https://example.com/blog/post.html",
			href:       "images/photo.jpg",
			expected:   "https://example.com/blog/images/photo.jpg",
		},
		{
			name:       "parent directory image",
			currentURL: "https://example.com/blog/2024/post.html",
			href:       "../images/photo.jpg",
			expected:   "https://example.com/blog/images/photo.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.SetCurrentURL(tt.currentURL)
			result := r.ResolveURL(tt.href)
			if result != tt.expected {
				t.Errorf("resolveURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestImageLoadingWithRelativeURL(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetCurrentURL("https://example.com/page.html")

	// Parse HTML with relative image URL
	htmlContent := `<html><body><img src="images/photo.jpg" alt="Test Image"></body></html>`
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	// Build render tree
	bodyNode := findBodyNode(doc)
	if bodyNode == nil {
		t.Fatal("Body node not found")
	}

	renderTree := renderer.BuildRenderTree(bodyNode)
	if renderTree == nil {
		t.Fatal("Render tree is nil")
	}

	// Find the image node
	var imgNode *renderer.RenderNode
	var findImg func(*renderer.RenderNode)
	findImg = func(node *renderer.RenderNode) {
		if node.TagName == "img" {
			imgNode = node
			return
		}
		for _, child := range node.Children {
			findImg(child)
		}
	}
	findImg(renderTree)

	if imgNode == nil {
		t.Fatal("Image node not found in render tree")
	}

	// Verify the image has the correct src attribute
	src, ok := imgNode.GetAttribute("src")
	if !ok || src != "images/photo.jpg" {
		t.Errorf("Image src attribute incorrect, got %v", src)
	}

	// The loadImages function should resolve this to an absolute URL
	// We can't directly test the async loading, but we can verify the resolver works
	expectedResolvedURL := "https://example.com/images/photo.jpg"
	resolvedURL := r.ResolveURL(src)
	if resolvedURL != expectedResolvedURL {
		t.Errorf("Expected resolved URL %v, got %v", expectedResolvedURL, resolvedURL)
	}
}

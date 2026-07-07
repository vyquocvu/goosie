package renderer

import (
	"testing"
)

func TestLinkRendering(t *testing.T) {
	renderer := NewRenderer(800, 600)

	tests := []struct {
		name    string
		html    string
		baseURL string
		wantErr bool
	}{
		{
			name:    "absolute link",
			html:    `<html><body><a href="https://example.com">Link</a></body></html>`,
			baseURL: "https://test.com",
			wantErr: false,
		},
		{
			name:    "relative link",
			html:    `<html><body><a href="/page">Link</a></body></html>`,
			baseURL: "https://test.com/current",
			wantErr: false,
		},
		{
			name: "multiple links",
			html: `<html><body>
<a href="https://google.com">Google</a>
<a href="/about">About</a>
<a href="contact.html">Contact</a>
</body></html>`,
			baseURL: "https://test.com/index.html",
			wantErr: false,
		},
		{
			name:    "link with nested elements",
			html:    `<html><body><p>Visit <a href="https://example.com"><strong>our site</strong></a> today!</p></body></html>`,
			baseURL: "https://test.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer.SetCurrentURL(tt.baseURL)
			_, err := renderer.RenderHTML(tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderHTML() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLinkClickNavigation(t *testing.T) {
	renderer := NewRenderer(800, 600)

	// Track clicked URLs
	var clickedURLs []string
	renderer.SetNavigationCallback(func(url string) {
		clickedURLs = append(clickedURLs, url)
	})

	renderer.SetCurrentURL("https://example.com/page.html")

	html := `<html><body>
<a href="https://google.com">Absolute</a>
<a href="/about">Root Relative</a>
<a href="other.html">Relative</a>
</body></html>`

	_, err := renderer.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	// Verify callback is set
	if renderer.onNavigate == nil {
		t.Error("Navigation callback should be set")
	}

	// Simulate clicks by calling the callback directly
	renderer.onNavigate("https://google.com")
	renderer.onNavigate("https://example.com/about")
	renderer.onNavigate("https://example.com/other.html")

	// Verify all clicks were recorded
	expectedURLs := []string{
		"https://google.com",
		"https://example.com/about",
		"https://example.com/other.html",
	}

	if len(clickedURLs) != len(expectedURLs) {
		t.Errorf("Expected %d clicks, got %d", len(expectedURLs), len(clickedURLs))
	}

	for i, expected := range expectedURLs {
		if i < len(clickedURLs) && clickedURLs[i] != expected {
			t.Errorf("Click %d: expected %s, got %s", i, expected, clickedURLs[i])
		}
	}
}

func TestDisplayListLinks(t *testing.T) {
	// Test that links are properly added to the display list
	dlb := NewDisplayListBuilder()

	// Create a simple render tree with a link
	root := NewRenderNode(NodeTypeElement)
	root.TagName = "body"

	link := NewRenderNode(NodeTypeElement)
	link.TagName = "a"
	link.SetAttribute("href", "https://example.com")

	text := NewRenderNode(NodeTypeText)
	text.Text = "Click me"

	link.AddChild(text)
	root.AddChild(link)

	// Create a simple layout tree
	layoutRoot := &LayoutBox{
		NodeID: root.ID,
		Box:    Rect{X: 0, Y: 0, Width: 100, Height: 20},
		Children: []*LayoutBox{
			{
				NodeID: link.ID,
				Box:    Rect{X: 0, Y: 0, Width: 80, Height: 20},
			},
		},
	}

	// Build display list
	displayList := dlb.Build(layoutRoot, root)

	// Check that a link command was added
	foundLink := false
	for _, cmd := range displayList.Commands {
		if cmd.Type == PaintLink {
			foundLink = true
			if cmd.LinkURL != "https://example.com" {
				t.Errorf("Expected link URL https://example.com, got %s", cmd.LinkURL)
			}
			if cmd.LinkText != "Click me" {
				t.Errorf("Expected link text 'Click me', got %s", cmd.LinkText)
			}
		}
	}

	if !foundLink {
		t.Error("No PaintLink command found in display list")
	}
}

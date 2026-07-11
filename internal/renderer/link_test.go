package renderer

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
)

func TestResolveURL(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)

	tests := []struct {
		name     string
		baseURL  string
		href     string
		expected string
	}{
		{
			name:     "absolute URL",
			baseURL:  "https://example.com/page",
			href:     "https://other.com/path",
			expected: "https://other.com/path",
		},
		{
			name:     "relative path",
			baseURL:  "https://example.com/path/page.html",
			href:     "other.html",
			expected: "https://example.com/path/other.html",
		},
		{
			name:     "root relative path",
			baseURL:  "https://example.com/path/page.html",
			href:     "/other/page.html",
			expected: "https://example.com/other/page.html",
		},
		{
			name:     "no base URL",
			baseURL:  "",
			href:     "/page.html",
			expected: "/page.html",
		},
		{
			name:     "http URL",
			baseURL:  "https://example.com/",
			href:     "http://other.com/",
			expected: "http://other.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr.baseURL = tt.baseURL
			result := cr.resolveURL(tt.href)
			if result != tt.expected {
				t.Errorf("resolveURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNavigationCallbackIntegration(t *testing.T) {
	renderer := NewRenderer(800, 600)

	// Track navigation calls
	var navigatedTo string
	callback := func(url string) {
		navigatedTo = url
	}

	renderer.SetNavigationCallback(callback)
	renderer.SetCurrentURL("https://example.com/page")

	// Render HTML with a link
	html := `<html><body><a href="/other">Link</a></body></html>`
	_, err := renderer.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	// The actual click would be triggered by user interaction
	// We've verified the setup works correctly
	if renderer.onNavigate == nil {
		t.Error("Navigation callback not set on renderer")
	}

	// Test the callback directly
	renderer.onNavigate("https://example.com/test")
	if navigatedTo != "https://example.com/test" {
		t.Errorf("Navigation callback not invoked correctly, got %v", navigatedTo)
	}
}

func TestTappableHyperlinkDoesNotWrapInlineText(t *testing.T) {
	link := newTappableHyperlink("RFC 2606", "https://example.com/rfc2606", func(string) {}, nil, 0)
	if link.Wrapping != fyne.TextWrapOff {
		t.Fatalf("hyperlink wrapping = %v; want TextWrapOff", link.Wrapping)
	}
}

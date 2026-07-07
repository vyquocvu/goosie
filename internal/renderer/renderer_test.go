package renderer

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/testutil"
	"golang.org/x/net/html"
)

func TestNewRenderer(t *testing.T) {
	r := NewRenderer(800, 600)
	if r == nil {
		t.Fatal("NewRenderer returned nil")
	}
	if r.layoutEngine == nil {
		t.Error("Layout engine not initialized")
	}
	if r.canvasRenderer == nil {
		t.Error("Canvas renderer not initialized")
	}
}

func TestRenderHTML(t *testing.T) {
	r := NewRenderer(800, 600)

	tests := []struct {
		name    string
		html    string
		wantErr bool
	}{
		{
			name:    "simple HTML",
			html:    "<html><body><h1>Hello</h1></body></html>",
			wantErr: false,
		},
		{
			name:    "paragraph",
			html:    "<html><body><p>This is a paragraph.</p></body></html>",
			wantErr: false,
		},
		{
			name:    "nested elements",
			html:    "<html><body><div><p>Text</p></div></body></html>",
			wantErr: false,
		},
		{
			name:    "empty body",
			html:    "<html><body></body></html>",
			wantErr: false,
		},
		{
			name:    "multiple elements",
			html:    "<html><body><h1>Title</h1><p>Para 1</p><p>Para 2</p></body></html>",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := r.RenderHTML(tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenderHTML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if obj == nil {
				t.Error("RenderHTML() returned nil canvas object")
			}

			// Save screenshot if enabled
			if path, err := testutil.SaveTestScreenshot(obj, "TestRenderHTML_"+tt.name, 800, 600); err != nil {
				t.Logf("Warning: failed to save screenshot: %v", err)
			} else if path != "" {
				t.Logf("Saved screenshot: %s", path)
			}
		})
	}
}

func TestRenderHTMLWithAttributes(t *testing.T) {
	r := NewRenderer(800, 600)
	html := `<html><body><div id="main" class="container"><p>Content</p></div></body></html>`

	obj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML() returned nil")
	}

	// Save screenshot if enabled
	if path, err := testutil.SaveTestScreenshot(obj, "TestRenderHTMLWithAttributes", 800, 600); err != nil {
		t.Logf("Warning: failed to save screenshot: %v", err)
	} else if path != "" {
		t.Logf("Saved screenshot: %s", path)
	}
}

func TestRenderHTMLHeadings(t *testing.T) {
	r := NewRenderer(800, 600)

	tests := []struct {
		name string
		html string
	}{
		{"h1", "<html><body><h1>Heading 1</h1></body></html>"},
		{"h2", "<html><body><h2>Heading 2</h2></body></html>"},
		{"h3", "<html><body><h3>Heading 3</h3></body></html>"},
		{"h4", "<html><body><h4>Heading 4</h4></body></html>"},
		{"h5", "<html><body><h5>Heading 5</h5></body></html>"},
		{"h6", "<html><body><h6>Heading 6</h6></body></html>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := r.RenderHTML(tt.html)
			if err != nil {
				t.Errorf("RenderHTML() error = %v", err)
			}
			if obj == nil {
				t.Error("RenderHTML() returned nil")
			}

			// Save screenshot if enabled
			if path, err := testutil.SaveTestScreenshot(obj, "TestRenderHTMLHeadings_"+tt.name, 800, 600); err != nil {
				t.Logf("Warning: failed to save screenshot: %v", err)
			} else if path != "" {
				t.Logf("Saved screenshot: %s", path)
			}
		})
	}
}

func TestRenderHTMLLists(t *testing.T) {
	r := NewRenderer(800, 600)

	html := `
		<html>
		<body>
			<ul>
				<li>Item 1</li>
				<li>Item 2</li>
				<li>Item 3</li>
			</ul>
		</body>
		</html>
	`

	obj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML() returned nil")
	}

	// Save screenshot if enabled
	if path, err := testutil.SaveTestScreenshot(obj, "TestRenderHTMLLists", 800, 600); err != nil {
		t.Logf("Warning: failed to save screenshot: %v", err)
	} else if path != "" {
		t.Logf("Saved screenshot: %s", path)
	}
}

func TestRenderHTMLLinks(t *testing.T) {
	r := NewRenderer(800, 600)

	html := `
		<html>
		<body>
			<p>Visit <a href="https://example.com">Example</a> for more info.</p>
		</body>
		</html>
	`

	obj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML() returned nil")
	}
}

func TestRenderHTMLImages(t *testing.T) {
	r := NewRenderer(800, 600)

	html := `
		<html>
		<body>
			<img src="image.png" alt="Test Image">
		</body>
		</html>
	`

	obj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML() returned nil")
	}
}

func TestRenderHTMLComplexStructure(t *testing.T) {
	r := NewRenderer(800, 600)

	html := `
		<html>
		<head><title>Test Page</title></head>
		<body>
			<div id="header">
				<h1>Welcome</h1>
			</div>
			<div id="content">
				<p>This is a paragraph with <strong>bold</strong> and <em>italic</em> text.</p>
				<ul>
					<li>First item</li>
					<li>Second item</li>
				</ul>
				<p>Another paragraph with a <a href="https://example.com">link</a>.</p>
			</div>
			<div id="footer">
				<p>Footer text</p>
			</div>
		</body>
		</html>
	`

	obj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if obj == nil {
		t.Fatal("RenderHTML() returned nil")
	}
}

func TestRenderHTMLInvalidHTML(t *testing.T) {
	r := NewRenderer(800, 600)

	// Even malformed HTML should be parsed (html.Parse is lenient)
	html := "<div><p>Unclosed tags"

	obj, err := r.RenderHTML(html)
	// html.Parse is very forgiving and won't error on malformed HTML
	if err != nil {
		t.Logf("Note: html.Parse accepted malformed HTML: %v", err)
	}
	if obj == nil {
		t.Error("RenderHTML() returned nil even for malformed HTML")
	}
}

func TestSetSize(t *testing.T) {
	r := NewRenderer(800, 600)

	r.SetSize(1024, 768)

	if r.layoutEngine.canvasWidth != 1024 {
		t.Errorf("Expected layout engine width 1024, got %f", r.layoutEngine.canvasWidth)
	}
	if r.layoutEngine.canvasHeight != 768 {
		t.Errorf("Expected layout engine height 768, got %f", r.layoutEngine.canvasHeight)
	}
	if r.canvasRenderer.canvasWidth != 1024 {
		t.Errorf("Expected canvas renderer width 1024, got %f", r.canvasRenderer.canvasWidth)
	}
	if r.canvasRenderer.canvasHeight != 768 {
		t.Errorf("Expected canvas renderer height 768, got %f", r.canvasRenderer.canvasHeight)
	}
}

func TestFindBodyNode(t *testing.T) {
	tests := []struct {
		name       string
		html       string
		shouldFind bool
	}{
		{
			name:       "explicit body",
			html:       "<html><body><p>Content</p></body></html>",
			shouldFind: true,
		},
		{
			name:       "implicit body (parser adds it)",
			html:       "<div>Content</div>",
			shouldFind: true, // html.Parse automatically adds body element
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			bodyNode := findBodyNode(doc)
			if tt.shouldFind && bodyNode == nil {
				t.Error("Expected to find body node, but got nil")
			}
			if !tt.shouldFind && bodyNode != nil {
				t.Error("Expected not to find body node, but found one")
			}
		})
	}
}

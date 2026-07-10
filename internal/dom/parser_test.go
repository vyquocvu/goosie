package dom

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("NewParser() returned nil")
	}
}

func TestParseBodyText(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		html     string
		wantText string
		wantErr  bool
	}{
		{
			name:     "simple body",
			html:     `<html><body>Hello World</body></html>`,
			wantText: "Hello World",
			wantErr:  false,
		},
		{
			name:     "body with nested elements",
			html:     `<html><body><h1>Title</h1><p>Paragraph</p></body></html>`,
			wantText: "Title Paragraph",
			wantErr:  false,
		},
		{
			name:     "empty body",
			html:     `<html><body></body></html>`,
			wantText: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseBodyText(tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBodyText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantText {
				t.Errorf("ParseBodyText() = %v, want %v", got, tt.wantText)
			}
		})
	}
}

func TestGetElementByID(t *testing.T) {
	parser := NewParser()

	html := `<html><body><div id="test">Test Content</div><p id="para">Paragraph</p></body></html>`

	tests := []struct {
		name     string
		id       string
		wantText string
		wantErr  bool
	}{
		{
			name:     "existing id",
			id:       "test",
			wantText: "Test Content",
			wantErr:  false,
		},
		{
			name:     "another existing id",
			id:       "para",
			wantText: "Paragraph",
			wantErr:  false,
		},
		{
			name:     "non-existing id",
			id:       "nonexistent",
			wantText: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetElementByID(html, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetElementByID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !strings.Contains(got, tt.wantText) && tt.wantText != "" {
				t.Errorf("GetElementByID() = %v, want to contain %v", got, tt.wantText)
			}
		})
	}
}

func TestParseBodyHTML(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name         string
		html         string
		wantContains []string
		wantErr      bool
	}{
		{
			name:         "simple body with h1",
			html:         `<html><body><h1>Hello World</h1></body></html>`,
			wantContains: []string{"# Hello World"},
			wantErr:      false,
		},
		{
			name:         "body with heading and paragraph",
			html:         `<html><body><h1>Title</h1><p>Paragraph text</p></body></html>`,
			wantContains: []string{"# Title", "Paragraph text"},
			wantErr:      false,
		},
		{
			name:         "body with link",
			html:         `<html><body><p><a href="https://example.com">Link text</a></p></body></html>`,
			wantContains: []string{"[Link text](https://example.com)"},
			wantErr:      false,
		},
		{
			name:         "body with bold and italic",
			html:         `<html><body><p><strong>Bold</strong> and <em>italic</em></p></body></html>`,
			wantContains: []string{"**Bold**", "*italic*"},
			wantErr:      false,
		},
		{
			name:         "empty body",
			html:         `<html><body></body></html>`,
			wantContains: []string{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.ParseBodyHTML(tt.html)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBodyHTML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("ParseBodyHTML() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestGetElementsByClassName(t *testing.T) {
	parser := NewParser()

	html := `<html><body>
		<div class="item">Item 1</div>
		<p class="item special">Item 2</p>
		<div class="other">Other</div>
		<span class="item">Item 3</span>
	</body></html>`

	tests := []struct {
		name      string
		className string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "class with multiple elements",
			className: "item",
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "class with single element",
			className: "special",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "non-existent class",
			className: "nonexistent",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetElementsByClassName(html, tt.className)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetElementsByClassName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("GetElementsByClassName() returned %d elements, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestGetElementsByTagName(t *testing.T) {
	parser := NewParser()

	html := `<html><body>
		<div>Div 1</div>
		<p>Paragraph 1</p>
		<div>Div 2</div>
		<span>Span 1</span>
		<p>Paragraph 2</p>
	</body></html>`

	tests := []struct {
		name      string
		tagName   string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "div elements",
			tagName:   "div",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "p elements",
			tagName:   "p",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "span elements",
			tagName:   "span",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "non-existent tag",
			tagName:   "article",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.GetElementsByTagName(html, tt.tagName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetElementsByTagName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("GetElementsByTagName() returned %d elements, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestQuerySelector(t *testing.T) {
	parser := NewParser()

	html := `<html><body>
		<div id="main" class="container">
			<p class="text">Paragraph 1</p>
			<p class="text special">Paragraph 2</p>
			<span data-test="value">Span 1</span>
		</div>
	</body></html>`

	tests := []struct {
		name     string
		selector string
		wantNil  bool
		wantText string
		wantErr  bool
	}{
		{
			name:     "ID selector",
			selector: "#main",
			wantNil:  false,
			wantText: "Paragraph 1",
			wantErr:  false,
		},
		{
			name:     "class selector",
			selector: ".text",
			wantNil:  false,
			wantText: "Paragraph 1",
			wantErr:  false,
		},
		{
			name:     "tag selector",
			selector: "span",
			wantNil:  false,
			wantText: "Span 1",
			wantErr:  false,
		},
		{
			name:     "attribute selector",
			selector: "[data-test=value]",
			wantNil:  false,
			wantText: "Span 1",
			wantErr:  false,
		},
		{
			name:     "non-matching selector",
			selector: "#nonexistent",
			wantNil:  true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.QuerySelector(html, tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("QuerySelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (got == nil) != tt.wantNil {
				t.Errorf("QuerySelector() returned nil = %v, wantNil %v", got == nil, tt.wantNil)
				return
			}
			if got != nil && tt.wantText != "" {
				if !strings.Contains(got.TextContent, tt.wantText) {
					t.Errorf("QuerySelector() TextContent = %q, want to contain %q", got.TextContent, tt.wantText)
				}
			}
		})
	}
}

func TestQuerySelectorAll(t *testing.T) {
	parser := NewParser()

	html := `<html><body>
		<div class="item">Item 1</div>
		<p class="item">Item 2</p>
		<div class="item">Item 3</div>
	</body></html>`

	tests := []struct {
		name      string
		selector  string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "class selector with multiple matches",
			selector:  ".item",
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "tag selector",
			selector:  "div",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "non-matching selector",
			selector:  ".nonexistent",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.QuerySelectorAll(html, tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("QuerySelectorAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("QuerySelectorAll() returned %d elements, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// --- ParseDocument tests (M1.3 streaming) ---

func TestParseDocumentSimpleHTML(t *testing.T) {
	parser := NewParser()
	htmlContent := `<html><head><title>Test</title></head><body><p>Hello</p></body></html>`
	doc, err := parser.ParseDocument(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}
	if doc == nil {
		t.Fatal("ParseDocument returned nil document")
	}
	// Verify the document has a body element
	var foundBody bool
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "body" {
			foundBody = true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if !foundBody {
		t.Error("parsed document did not contain body element")
	}
}

func TestParseDocumentEmptyReader(t *testing.T) {
	parser := NewParser()
	doc, err := parser.ParseDocument(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseDocument on empty reader returned error: %v", err)
	}
	if doc == nil {
		t.Fatal("ParseDocument on empty reader returned nil")
	}
}

func TestParseDocumentMalformedHTML(t *testing.T) {
	parser := NewParser()
	// Malformed HTML should still parse without error (html.Parse is lenient)
	htmlContent := `<p>unclosed paragraph<div>nested without close`
	doc, err := parser.ParseDocument(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("ParseDocument on malformed HTML returned error: %v", err)
	}
	if doc == nil {
		t.Fatal("ParseDocument on malformed HTML returned nil")
	}
}

func TestParseDocumentPreservesStructure(t *testing.T) {
	parser := NewParser()
	htmlContent := `<html><body><div id="main"><h1>Title</h1><p class="content">Text</p></div></body></html>`
	doc, err := parser.ParseDocument(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("ParseDocument returned error: %v", err)
	}

	// Find the div with id="main"
	var foundDiv *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == "main" {
					foundDiv = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if foundDiv == nil {
		t.Fatal("did not find div#main in parsed document")
	}
	// Verify children are preserved
	if foundDiv.FirstChild == nil {
		t.Fatal("div#main has no children")
	}
}

func TestParseDocumentFromBytesReader(t *testing.T) {
	parser := NewParser()
	data := []byte(`<html><body><span>from bytes</span></body></html>`)
	doc, err := parser.ParseDocument(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParseDocument from bytes.Reader returned error: %v", err)
	}
	if doc == nil {
		t.Fatal("ParseDocument from bytes.Reader returned nil")
	}
}

func BenchmarkParseDocument(b *testing.B) {
	parser := NewParser()
	htmlContent := strings.Repeat("<p>Paragraph content for benchmark testing.</p>", 100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseDocument(strings.NewReader(htmlContent))
		if err != nil {
			b.Fatal(err)
		}
	}
}

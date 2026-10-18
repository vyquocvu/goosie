package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestExtractArticle(t *testing.T) {
	htmlSource := `<!DOCTYPE html>
<html>
<head>
  <title>Understanding Modern Go Systems</title>
  <meta name="author" content="Jane Doe">
  <meta property="og:site_name" content="Tech Journal">
</head>
<body>
  <nav class="main-navigation">
    <a href="/home">Home</a>
    <a href="/about">About</a>
  </nav>
  <header>
    <h1>Site Header</h1>
  </header>
  <div class="ad-banner">
    Buy our shiny new product!
  </div>
  <article>
    <h1>Understanding Modern Go Systems</h1>
    <p>Go is an open source programming language designed for building simple, fast, and reliable software. Over the past decade, Go has become the foundation of cloud infrastructure, container orchestration, and high-performance network proxies across the modern internet.</p>
    <p>One of the language's greatest strengths is its concurrency primitives: goroutines and channels provide a clean CSP model for high-throughput I/O pipelines.</p>
    <blockquote>Software engineering is what happens to programming when you add time and other programmers.</blockquote>
    <h2>Memory Management</h2>
    <p>The Go runtime features a non-generational, concurrent tri-color mark-sweep garbage collector designed to maintain low millisecond-range pause times.</p>
    <pre><code>go test -race ./...</code></pre>
  </article>
  <div class="comments-section">
    <h3>Comments (4)</h3>
    <p>Great article!</p>
  </div>
  <footer>
    <p>&copy; 2026 Tech Journal. All rights reserved.</p>
  </footer>
</body>
</html>`

	doc, err := dom.ParseBounded(htmlSource, dom.ParseLimits{Nodes: 1000, Depth: 100, Attributes: 500, AttributeBytes: 10000})
	if err != nil || doc == nil {
		t.Fatalf("failed to parse HTML: %v", err)
	}

	art := ExtractArticle(doc, "https://example.com/posts/modern-go", "sepia")
	if art == nil {
		t.Fatalf("expected non-nil Article")
	}

	// Verify Title
	if art.Title != "Understanding Modern Go Systems" {
		t.Errorf("expected title 'Understanding Modern Go Systems', got %q", art.Title)
	}

	// Verify Byline
	if !strings.Contains(art.Byline, "Jane Doe") || !strings.Contains(art.Byline, "Tech Journal") {
		t.Errorf("expected byline to contain author and site name, got %q", art.Byline)
	}

	// Verify Boilerplate is stripped
	if strings.Contains(art.ContentHTML, "Buy our shiny new product") {
		t.Errorf("expected ad-banner to be stripped from content")
	}
	if strings.Contains(art.ContentHTML, "Site Header") {
		t.Errorf("expected header to be stripped from content")
	}
	if strings.Contains(art.ContentHTML, "main-navigation") {
		t.Errorf("expected navigation to be stripped from content")
	}
	if strings.Contains(art.ContentHTML, "Comments (4)") {
		t.Errorf("expected comments section to be stripped from content")
	}

	// Verify Article content retained
	if !strings.Contains(art.ContentHTML, "concurrency primitives") {
		t.Errorf("expected article content to be retained")
	}
	if !strings.Contains(art.ContentHTML, "<blockquote>") {
		t.Errorf("expected blockquote to be formatted")
	}
	if !strings.Contains(art.ContentHTML, "<pre><code>") {
		t.Errorf("expected code block to be formatted")
	}

	// Verify Word count and reading time
	if art.WordCount <= 0 {
		t.Errorf("expected positive word count, got %d", art.WordCount)
	}
	if art.ReadingTime < 1 {
		t.Errorf("expected at least 1 min reading time, got %d", art.ReadingTime)
	}

	// Verify Rendered HTML with Sepia theme
	fullHTML := RenderReaderPageHTML(art)
	if !strings.Contains(fullHTML, "#fbf0d9") {
		t.Errorf("expected sepia background color in rendered HTML")
	}
	if !strings.Contains(fullHTML, "Back to original page") {
		t.Errorf("expected back link in rendered HTML")
	}
}

func TestReaderThemeVariants(t *testing.T) {
	art := &Article{
		Title:       "Sample Post",
		ContentHTML: "<p>Sample content here.</p>",
		URL:         "https://example.com/test",
		ReadingTime: 2,
		WordCount:   350,
	}

	// Light theme
	art.Theme = "light"
	lightHTML := RenderReaderPageHTML(art)
	if !strings.Contains(lightHTML, "#fdfdfd") {
		t.Errorf("expected light theme background")
	}

	// Dark theme
	art.Theme = "dark"
	darkHTML := RenderReaderPageHTML(art)
	if !strings.Contains(darkHTML, "#1e1e24") {
		t.Errorf("expected dark theme background")
	}
}

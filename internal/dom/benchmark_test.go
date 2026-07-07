package dom

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/testpages"
)

func BenchmarkParseBodyTextSmall(b *testing.B) { benchmarkParseBodyText(b, smallHTML) }

func BenchmarkParseBodyTextMedium(b *testing.B) { benchmarkParseBodyText(b, mediumHTML) }

func BenchmarkParseBodyTextLarge(b *testing.B) { benchmarkParseBodyText(b, longFormHTML) }

func BenchmarkParseBodyTextTableHeavy(b *testing.B) {
	page, ok := testpages.Get("table_heavy")
	if !ok {
		b.Fatal("table_heavy page not found")
	}
	benchmarkParseBodyText(b, page.HTML)
}

func BenchmarkParseBodyTextFormHeavy(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}
	benchmarkParseBodyText(b, page.HTML)
}

func BenchmarkParseBodyHTMLSmall(b *testing.B) { benchmarkParseBodyHTML(b, smallHTML) }

func BenchmarkParseBodyHTMLMedium(b *testing.B) { benchmarkParseBodyHTML(b, mediumHTML) }

func BenchmarkParseBodyHTMLLarge(b *testing.B) { benchmarkParseBodyHTML(b, longFormHTML) }

func BenchmarkParseBodyHTMLTableHeavy(b *testing.B) {
	page, ok := testpages.Get("table_heavy")
	if !ok {
		b.Fatal("table_heavy page not found")
	}
	benchmarkParseBodyHTML(b, page.HTML)
}

func BenchmarkParseBodyHTMLFormHeavy(b *testing.B) {
	page, ok := testpages.Get("form_heavy")
	if !ok {
		b.Fatal("form_heavy page not found")
	}
	benchmarkParseBodyHTML(b, page.HTML)
}

func BenchmarkGetElementByIDFound(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.GetElementByID(mediumHTML, "main-content")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetElementByIDNotFound(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.GetElementByID(mediumHTML, "nonexistent-id")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetElementByIDFull(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.GetElementByIDFull(mediumHTML, "sidebar")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetElementsByClassName(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.GetElementsByClassName(mediumHTML, "item")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetElementsByTagName(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.GetElementsByTagName(mediumHTML, "a")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectorID(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.QuerySelector(mediumHTML, "#main-content")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectorClass(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.QuerySelector(mediumHTML, ".item")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectorTag(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.QuerySelector(mediumHTML, "p")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectorNotFound(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.QuerySelector(mediumHTML, "#nonexistent")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkQuerySelectorAllTag(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.QuerySelectorAll(mediumHTML, "div")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseEmpty(b *testing.B) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.ParseBodyText("")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkParseBodyText(b *testing.B, html string) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.ParseBodyText(html)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkParseBodyHTML(b *testing.B, html string) {
	p := NewParser()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.ParseBodyHTML(html)
		if err != nil {
			b.Fatal(err)
		}
	}
}

const smallHTML = `<html><body><h1>Hello</h1><p>World</p></body></html>`

const mediumHTML = `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
<div id="header"><h1>Test Page</h1></div>
<div id="main-content">
	<p class="intro">Welcome to the test page.</p>
	<ul id="list">
		<li class="item">Item one</li>
		<li class="item active">Item two</li>
		<li class="item">Item three</li>
	</ul>
	<div id="sidebar">
		<p>Sidebar content here.</p>
		<a href="https://example.com">Example Link</a>
	</div>
</div>
<div id="footer"><p>&copy; 2025</p></div>
</body>
</html>`

const longFormHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Long-Form Article | Documentation Site</title>
<meta name="description" content="A comprehensive guide covering all aspects of the topic with detailed examples and references.">
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<header id="site-header">
	<div class="container">
		<h1 class="site-title">Documentation Site</h1>
		<nav class="main-nav">
			<ul>
				<li><a href="/">Home</a></li>
				<li><a href="/docs">Docs</a></li>
				<li><a href="/blog">Blog</a></li>
				<li><a href="/about">About</a></li>
			</ul>
		</nav>
	</div>
</header>
<main id="main-content">
	<article class="post" id="post-1">
		<header class="post-header">
			<h2 class="post-title">Getting Started Guide</h2>
			<div class="post-meta">
				<span class="author">By Jane Doe</span>
				<time class="date" datetime="2025-06-15">June 15, 2025</time>
				<span class="category"><a href="/category/dev">Development</a></span>
			</div>
		</header>
		<div class="post-body">
			<p>This guide will walk you through the initial setup and configuration required to get started with our platform. By the end of this tutorial, you will have a working development environment and understand the core concepts.</p>
			<h3 id="prerequisites">Prerequisites</h3>
			<ul>
				<li>Go 1.21 or later installed on your system</li>
				<li>A text editor or IDE of your choice</li>
				<li>Basic understanding of command-line tools</li>
				<li>Git for version control</li>
			</ul>
			<h3 id="installation">Installation</h3>
			<p>Run the following command to install the package:</p>
			<pre><code>go get example.com/pkg</code></pre>
			<p>This will download the package and all its dependencies. The installation should complete within a few seconds depending on your internet connection.</p>
			<h3 id="configuration">Configuration</h3>
			<p>Create a configuration file named <code>config.yaml</code> in your project root:</p>
			<pre><code>server:
  host: localhost
  port: 8080
database:
  url: postgres://user:pass@localhost/db
  max_connections: 25</code></pre>
			<h3 id="usage">Basic Usage</h3>
			<p>Here is a simple example that demonstrates the core functionality:</p>
			<pre><code>package main

import (
	"fmt"
	"example.com/pkg"
)

func main() {
	client := pkg.NewClient()
	result, err := client.Process("input data")
	if err != nil {
		panic(err)
	}
	fmt.Println(result)
}</code></pre>
			<h3 id="advanced">Advanced Topics</h3>
			<p>For advanced usage patterns, refer to the following sections. These cover topics such as middleware integration, custom handlers, and performance optimization techniques.</p>
			<h4 id="middleware">Middleware Integration</h4>
			<p>The platform supports a plugin-based middleware architecture. You can chain multiple middleware components to create complex processing pipelines:</p>
			<ol>
				<li><strong>Authentication middleware</strong> — validates API keys and JWT tokens</li>
				<li><strong>Rate limiting middleware</strong> — enforces request quotas per client</li>
				<li><strong>Logging middleware</strong> — records request and response details</li>
				<li><strong>Caching middleware</strong> — serves cached responses when applicable</li>
			</ol>
			<h4 id="error-handling">Error Handling</h4>
			<p>Our library uses typed errors throughout. Always check return values and handle errors appropriately:</p>
			<pre><code>result, err := client.DoWork(input)
if err != nil {
	switch e := err.(type) {
	case *pkg.ValidationError:
		fmt.Printf("Validation failed: %v\n", e.Errors)
	case *pkg.TimeoutError:
		fmt.Println("Request timed out, retrying...")
	default:
		fmt.Printf("Unexpected error: %v\n", err)
	}
}</code></pre>
		</div>
		<footer class="post-footer">
			<div class="tags">
				<span class="tag"><a href="/tag/go">Go</a></span>
				<span class="tag"><a href="/tag/tutorial">Tutorial</a></span>
				<span class="tag"><a href="/tag/getting-started">Getting Started</a></span>
			</div>
		</footer>
	</article>
	<article class="post" id="post-2">
		<header class="post-header">
			<h2 class="post-title">API Reference</h2>
			<div class="post-meta">
				<span class="author">By John Smith</span>
				<time class="date" datetime="2025-06-20">June 20, 2025</time>
				<span class="category"><a href="/category/api">API</a></span>
			</div>
		</header>
		<div class="post-body">
			<h3 id="client">Client</h3>
			<p>The <code>Client</code> struct is the main entry point for interacting with the API.</p>
			<table>
				<thead>
					<tr><th>Method</th><th>Description</th></tr>
				</thead>
				<tbody>
					<tr><td><code>NewClient()</code></td><td>Creates a new client instance</td></tr>
					<tr><td><code>Process(input)</code></td><td>Processes the given input and returns a result</td></tr>
					<tr><td><code>Validate(data)</code></td><td>Validates the provided data structure</td></tr>
					<tr><td><code>Cancel()</code></td><td>Cancels any ongoing operation</td></tr>
				</tbody>
			</table>
		</div>
	</article>
</main>
<aside id="sidebar">
	<div class="widget search-widget">
		<h3 class="widget-title">Search</h3>
		<form action="/search" method="get">
			<input type="text" name="q" class="search-input" placeholder="Search docs...">
			<button type="submit" class="search-btn">Go</button>
		</form>
	</div>
	<div class="widget recent-posts">
		<h3 class="widget-title">Recent Posts</h3>
		<ul>
			<li><a href="/post/3">Advanced Configuration Tips</a></li>
			<li><a href="/post/4">Performance Benchmarking Guide</a></li>
			<li><a href="/post/5">Deploying to Production</a></li>
		</ul>
	</div>
</aside>
<footer id="site-footer">
	<div class="container">
		<p>&copy; 2025 Documentation Site. All rights reserved.</p>
		<nav class="footer-nav">
			<ul>
				<li><a href="/privacy">Privacy Policy</a></li>
				<li><a href="/terms">Terms of Service</a></li>
			</ul>
		</nav>
	</div>
</footer>
</body>
</html>`

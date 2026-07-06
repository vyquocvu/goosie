// Package testpages provides deterministic local HTML/CSS documents for engine
// benchmarks and scenario tests.
package testpages

import (
	"context"
	"errors"
)

// ErrNotFound reports that a named corpus page does not exist.
var ErrNotFound = errors.New("test page not found")

// Page is one deterministic HTML/CSS document in the benchmark corpus.
type Page struct {
	Name      string
	Title     string
	HTML      string
	CSS       string
	HTMLBytes int
	CSSBytes  int
}

// Summary describes a corpus page without copying its full document content.
type Summary struct {
	Name      string
	Title     string
	HTMLBytes int
	CSSBytes  int
}

var pages = []Page{
	newPage("long_article", "Long Article", longArticleHTML, longArticleCSS),
	newPage("documentation", "Documentation Page", documentationHTML, documentationCSS),
}

func newPage(name, title, html, css string) Page {
	return Page{
		Name:      name,
		Title:     title,
		HTML:      html,
		CSS:       css,
		HTMLBytes: len(html),
		CSSBytes:  len(css),
	}
}

// List returns the deterministic corpus pages in stable order.
func List() []Summary {
	summaries := make([]Summary, len(pages))
	for i, page := range pages {
		summaries[i] = Summary{
			Name:      page.Name,
			Title:     page.Title,
			HTMLBytes: page.HTMLBytes,
			CSSBytes:  page.CSSBytes,
		}
	}
	return summaries
}

// Get returns a named corpus page.
func Get(name string) (Page, bool) {
	for _, page := range pages {
		if page.Name == name {
			return page, true
		}
	}
	return Page{}, false
}

// GetContext returns a named corpus page unless ctx has been canceled.
func GetContext(ctx context.Context, name string) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	page, ok := Get(name)
	if !ok {
		return Page{}, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	return page, nil
}

const longArticleHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Long Article</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<header class="site-header">
<h1>Long Article Benchmark</h1>
<p class="deck">A deterministic article page with repeated prose, links, lists, pull quotes, and code samples.</p>
</header>
<main>
<article id="long-article" class="long-form">
<section id="chapter-01"><h2>Chapter 1</h2><p>The engine starts with navigation and moves through resource loading, parsing, style resolution, layout, paint, raster, and presentation.</p><p>Each paragraph is intentionally ordinary text so parser, DOM, style, layout, and paint benchmarks exercise realistic document flow without network access.</p><ul><li>Stable markup</li><li>Predictable text</li><li>Local resources</li></ul></section>
<section id="chapter-02"><h2>Chapter 2</h2><p>Long documents should remain scrollable without rebuilding every retained object on each viewport change.</p><p>The corpus keeps headings, anchors, inline emphasis, and nested lists close to the shape of documentation and blog articles.</p><blockquote>Measurements matter more than intuition when evaluating browser engine changes.</blockquote></section>
<section id="chapter-03"><h2>Chapter 3</h2><p>Selectors in this page are deliberately simple, allowing parser and DOM benchmarks to isolate article scale from selector complexity.</p><p>Rendering backends can use this fixture to validate that block flow, text wrapping, and vertical spacing remain stable.</p><ol><li>Parse the document</li><li>Build the render tree</li><li>Measure layout and paint</li></ol></section>
<section id="chapter-04"><h2>Chapter 4</h2><p>Images are not included in this article fixture because image-heavy scenarios are tracked as a separate roadmap item.</p><p>Keeping the asset surface small makes this page suitable for fast local checks and deterministic benchmark runs.</p><pre><code>go test -bench=. -benchmem ./internal/engine/testpages</code></pre></section>
<section id="chapter-05"><h2>Chapter 5</h2><p>Browser engines benefit from explicit ownership between phases so cancellation and cleanup can be tested independently.</p><p>This fixture is static data and does not allocate timers, start goroutines, or retain mutable global state.</p><p><a href="#chapter-12">Jump to the final chapter</a> for end-of-document traversal checks.</p></section>
<section id="chapter-06"><h2>Chapter 6</h2><p>Articles frequently contain metadata, summaries, related links, and quotes that interact with inherited CSS properties.</p><p>The markup uses semantic elements to help benchmark supported HTML without expanding the compatibility target.</p><ul><li>header</li><li>main</li><li>article</li><li>section</li><li>footer</li></ul></section>
<section id="chapter-07"><h2>Chapter 7</h2><p>Repeated structure exposes regressions in traversal cost and allocation behavior more clearly than a tiny synthetic page.</p><p>Benchmarks should compare the same corpus across branches before performance claims are made.</p><blockquote>Stable input is the quiet foundation of useful performance work.</blockquote></section>
<section id="chapter-08"><h2>Chapter 8</h2><p>The page includes enough nested text to exercise text extraction, DOM searches, and display-list generation paths.</p><p>Future scenario runners can combine this fixture with viewport sizes and scroll positions for repeatable comparisons.</p><p><strong>Note:</strong> Unsupported browser features are intentionally absent from this fixture.</p></section>
<section id="chapter-09"><h2>Chapter 9</h2><p>Documentation-style prose often mixes paragraphs with short examples and ordered steps.</p><p>The article fixture uses the same pattern while staying independent from any UI toolkit or platform adapter.</p><pre><code>Navigation -> Parse -> Style -> Layout -> Paint</code></pre></section>
<section id="chapter-10"><h2>Chapter 10</h2><p>Memory behavior is easier to review when the corpus has explicit byte counts and stable page names.</p><p>Tests assert those properties so accidental fixture drift is visible during normal package checks.</p><ul><li>No external URLs are fetched</li><li>No generated content is required</li><li>No cleanup hook is needed</li></ul></section>
<section id="chapter-11"><h2>Chapter 11</h2><p>Benchmarks using this page should report allocations with benchmem and record raw output in pull request descriptions.</p><p>The corpus itself is intentionally boring, which is a virtue for regression hunting.</p><blockquote>Predictability is a feature.</blockquote></section>
<section id="chapter-12"><h2>Chapter 12</h2><p>The final section exists so tests can confirm the whole fixture is present and not truncated.</p><p>End-of-document traversal catches off-by-one bugs in parsers, walkers, viewport filters, and summary counters.</p><p>Long article benchmark complete.</p></section>
</article>
</main>
<footer class="site-footer"><p>Goosie deterministic benchmark corpus.</p></footer>
</body>
</html>`

const longArticleCSS = `
body { margin: 0; font-family: Georgia, serif; line-height: 1.6; color: #202124; background: #ffffff; }
.site-header, .site-footer { padding: 24px 32px; background: #f4f6f8; }
.deck { max-width: 720px; color: #4b5563; }
article.long-form { max-width: 760px; margin: 0 auto; padding: 24px 32px 56px; }
article.long-form section { margin: 0 0 32px; }
article.long-form h2 { margin: 0 0 12px; font-size: 1.65rem; }
article.long-form p { margin: 0 0 14px; }
article.long-form ul, article.long-form ol { margin: 0 0 14px 24px; padding: 0; }
article.long-form blockquote { margin: 18px 0; padding: 12px 18px; border-left: 4px solid #5b7cfa; background: #f7f8ff; }
article.long-form pre { overflow: auto; padding: 14px; background: #111827; color: #f9fafb; }
a { color: #1457d9; }
`

const documentationHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Documentation Page</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<div class="docs-shell">
<nav class="sidebar" aria-label="Documentation">
<h1>Goosie Docs</h1>
<ol>
<li><a href="#overview">Overview</a></li>
<li><a href="#install">Install</a></li>
<li><a href="#api">API</a></li>
<li><a href="#limits">Limits</a></li>
</ol>
</nav>
<main class="docs-content">
<section id="overview">
<h2>Overview</h2>
<p>This deterministic documentation page represents reference material, API examples, tables, and a small feedback form.</p>
<p>It is meant for parser, DOM, layout, paint, and scenario benchmarks that need a realistic docs page without external dependencies.</p>
</section>
<section id="install">
<h2>Install</h2>
<p>Use the standard Go toolchain to run local checks.</p>
<pre><code>go test ./...
go test -bench=. -benchmem ./internal/engine/testpages</code></pre>
</section>
<section id="api">
<h2>API Reference</h2>
<table>
<thead><tr><th>Name</th><th>Purpose</th><th>Cancellation</th></tr></thead>
<tbody>
<tr><td>List</td><td>Returns stable page summaries</td><td>No work to cancel</td></tr>
<tr><td>Get</td><td>Returns a deterministic page</td><td>No work to cancel</td></tr>
<tr><td>GetContext</td><td>Returns a deterministic page with context checks</td><td>Honors canceled context</td></tr>
</tbody>
</table>
</section>
<section id="limits">
<h2>Limits</h2>
<p>The corpus does not fetch images, start workers, execute scripts, or depend on Fyne.</p>
<form class="feedback-form" action="/feedback" method="post">
<label for="feedback">Was this page useful?</label>
<textarea id="feedback" name="feedback" rows="3"></textarea>
<button type="submit">Send</button>
</form>
</section>
</main>
</div>
</body>
</html>`

const documentationCSS = `
body { margin: 0; font-family: Arial, sans-serif; color: #1f2937; background: #ffffff; }
.docs-shell { display: flex; min-height: 100vh; }
.sidebar { flex: 0 0 220px; padding: 24px; border-right: 1px solid #d1d5db; background: #f9fafb; }
.sidebar h1 { font-size: 1.25rem; margin: 0 0 16px; }
.sidebar ol { margin: 0; padding-left: 20px; }
.sidebar li { margin-bottom: 8px; }
.docs-content { flex: 1; max-width: 860px; padding: 28px 36px; }
.docs-content section { margin-bottom: 32px; }
.docs-content h2 { margin: 0 0 12px; font-size: 1.5rem; }
.docs-content p { line-height: 1.6; margin: 0 0 12px; }
.docs-content pre { padding: 14px; overflow: auto; background: #111827; color: #f9fafb; }
.docs-content table { width: 100%; border-collapse: collapse; margin: 16px 0; }
.docs-content th, .docs-content td { padding: 8px 10px; border: 1px solid #d1d5db; text-align: left; }
.feedback-form { display: grid; gap: 8px; max-width: 420px; }
.feedback-form textarea { width: 100%; }
.feedback-form button { width: max-content; padding: 6px 12px; }
`

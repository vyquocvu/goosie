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
	newPage("table_heavy", "Table-Heavy Data Grid", tableHeavyHTML, tableHeavyCSS),
	newPage("form_heavy", "Form-Heavy Settings Page", formHeavyHTML, formHeavyCSS),
	newPage("image_heavy", "Image-Heavy Page", imageHeavyHTML, imageHeavyCSS),
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

const tableHeavyHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Table-Heavy Data Grid</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<header class="table-header">
<h1>Annual Financial and Sales Performance Ledger</h1>
<p class="summary">Zebra-striped records, multi-level headers, cell spanning, and financial summaries for performance testing.</p>
</header>
<main class="table-container">
<table class="data-grid">
<thead>
<tr>
<th rowspan="2" class="col-quarter">Quarter</th>
<th rowspan="2" class="col-month">Month</th>
<th colspan="3" class="group-header">Sales Performance</th>
<th colspan="4" class="group-header">Financial Ledger</th>
<th rowspan="2" class="col-status">Audit Status</th>
</tr>
<tr>
<th class="col-rep">Representative</th>
<th class="col-region">Region</th>
<th class="col-units">Units</th>
<th class="col-price">Unit Price</th>
<th class="col-gross">Gross Value</th>
<th class="col-discount">Discount</th>
<th class="col-net">Net Total</th>
</tr>
</thead>
<tbody>
<tr>
<td rowspan="3" class="quarter-cell">Q1</td>
<td>January</td>
<td>Alice Vance</td>
<td>East</td>
<td>1,240</td>
<td>$50.00</td>
<td>$62,000.00</td>
<td>10%</td>
<td>$55,800.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>February</td>
<td>Bob Miller</td>
<td>West</td>
<td>980</td>
<td>$55.00</td>
<td>$53,900.00</td>
<td>5%</td>
<td>$51,205.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>March</td>
<td>Charlie Ross</td>
<td>North</td>
<td>1,560</td>
<td>$48.00</td>
<td>$74,880.00</td>
<td>15%</td>
<td>$63,648.00</td>
<td><span class="status-pending">Pending</span></td>
</tr>
<tr>
<td rowspan="3" class="quarter-cell">Q2</td>
<td>April</td>
<td>David King</td>
<td>South</td>
<td>1,120</td>
<td>$50.00</td>
<td>$56,000.00</td>
<td>8%</td>
<td>$51,520.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>May</td>
<td>Emma Wright</td>
<td>East</td>
<td>1,430</td>
<td>$52.00</td>
<td>$74,360.00</td>
<td>10%</td>
<td>$66,924.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>June</td>
<td>Frank Green</td>
<td>West</td>
<td>850</td>
<td>$60.00</td>
<td>$51,000.00</td>
<td>0%</td>
<td>$51,000.00</td>
<td><span class="status-flagged">Flagged</span></td>
</tr>
<tr>
<td rowspan="3" class="quarter-cell">Q3</td>
<td>July</td>
<td>Grace Hill</td>
<td>North</td>
<td>1,800</td>
<td>$45.00</td>
<td>$81,000.00</td>
<td>12%</td>
<td>$71,280.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>August</td>
<td>Henry Adams</td>
<td>South</td>
<td>1,050</td>
<td>$50.00</td>
<td>$52,500.00</td>
<td>5%</td>
<td>$49,875.00</td>
<td><span class="status-pending">Pending</span></td>
</tr>
<tr>
<td>September</td>
<td>Ivy Baker</td>
<td>East</td>
<td>1,670</td>
<td>$52.00</td>
<td>$86,840.00</td>
<td>10%</td>
<td>$78,156.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td rowspan="3" class="quarter-cell">Q4</td>
<td>October</td>
<td>Jack Carter</td>
<td>West</td>
<td>2,100</td>
<td>$48.00</td>
<td>$100,800.00</td>
<td>15%</td>
<td>$85,680.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>November</td>
<td>Karen Nelson</td>
<td>North</td>
<td>1,350</td>
<td>$55.00</td>
<td>$74,250.00</td>
<td>8%</td>
<td>$68,310.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
<tr>
<td>December</td>
<td>Leo Sanders</td>
<td>South</td>
<td>2,450</td>
<td>$50.00</td>
<td>$122,500.00</td>
<td>18%</td>
<td>$100,450.00</td>
<td><span class="status-verified">Verified</span></td>
</tr>
</tbody>
<tfoot>
<tr>
<th colspan="2">Total / Average</th>
<th colspan="2">12 Representatives</th>
<th>17,580 Units</th>
<th>$52.08 Avg</th>
<th>$890,830.00</th>
<th>9.8% Avg</th>
<th>$803,828.00</th>
<th>12/12 Audited</th>
</tr>
</tfoot>
</table>
</main>
<footer class="table-footer">
<p>End of financial records. Confidential benchmark asset.</p>
</footer>
</body>
</html>`

const tableHeavyCSS = `
body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; color: #1e293b; background: #f8fafc; }
.table-header { padding: 32px; background: #ffffff; border-bottom: 1px solid #e2e8f0; }
.table-header h1 { margin: 0 0 8px; font-size: 1.8rem; color: #0f172a; }
.summary { margin: 0; color: #64748b; font-size: 0.95rem; }
.table-container { padding: 32px; }
table.data-grid { width: 100%; border-collapse: collapse; background: #ffffff; border: 1px solid #cbd5e1; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.05); }
table.data-grid th, table.data-grid td { padding: 12px 16px; border: 1px solid #e2e8f0; font-size: 0.9rem; text-align: left; }
table.data-grid thead th { background: #f1f5f9; font-weight: 600; color: #334155; }
table.data-grid thead th.group-header { text-align: center; background: #e2e8f0; }
table.data-grid tbody tr:nth-child(even) { background: #f8fafc; }
table.data-grid tbody tr:hover { background: #f1f5f9; }
table.data-grid tfoot th { background: #f1f5f9; font-weight: 700; color: #0f172a; border-top: 2px solid #94a3b8; }
.quarter-cell { font-weight: 700; text-align: center; background: #f8fafc; color: #475569; }
.status-verified { color: #16a34a; font-weight: 600; }
.status-pending { color: #d97706; font-weight: 600; }
.status-flagged { color: #dc2626; font-weight: 600; }
.table-footer { padding: 24px 32px; background: #f1f5f9; text-align: center; font-size: 0.85rem; color: #64748b; }
`

const formHeavyHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Form-Heavy Settings Page</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<header class="form-header">
<h1>Account Preferences and Configuration</h1>
<p class="desc">A form-heavy page to benchmark parser rendering, layout positioning, and element counts for inputs, labels, and textareas.</p>
</header>
<main class="form-container">
<form id="settings-form" class="preferences-form" action="/save-settings" method="post">
<fieldset class="form-section">
<legend>Profile Information</legend>
<div class="form-group">
<label for="username">Username</label>
<input type="text" id="username" name="username" placeholder="eg. johndoe" required>
</div>
<div class="form-group">
<label for="email">Primary Email Address</label>
<input type="email" id="email" name="email" placeholder="john.doe@example.com" required>
</div>
<div class="form-group">
<label for="bio">Public Biography</label>
<textarea id="bio" name="bio" rows="4" placeholder="Tell us about yourself..."></textarea>
</div>
</fieldset>
<fieldset class="form-section">
<legend>System Settings</legend>
<div class="form-group">
<label for="language">Default Interface Language</label>
<select id="language" name="language">
<option value="en">English (US)</option>
<option value="es">Español</option>
<option value="fr">Français</option>
<option value="de">Deutsch</option>
<option value="ja">日本語</option>
</select>
</div>
<div class="form-group">
<label for="timezone">Timezone</label>
<select id="timezone" name="timezone">
<option value="utc">UTC (Coordinated Universal Time)</option>
<option value="est">EST (Eastern Standard Time)</option>
<option value="pst">PST (Pacific Standard Time)</option>
<option value="cet">CET (Central European Time)</option>
</select>
</div>
</fieldset>
<fieldset class="form-section">
<legend>Notifications & Newsletter</legend>
<div class="checkbox-group">
<input type="checkbox" id="notify-email" name="notify_email" checked>
<label for="notify-email">Send email notifications for account alerts</label>
</div>
<div class="checkbox-group">
<input type="checkbox" id="notify-security" name="notify_security" checked>
<label for="notify-security">Send critical security alerts (always active)</label>
</div>
<div class="checkbox-group">
<input type="checkbox" id="newsletter" name="newsletter">
<label for="newsletter">Subscribe to our weekly system newsletter</label>
</div>
</fieldset>
<fieldset class="form-section">
<legend>Account Type & Subscription</legend>
<p class="section-note">Select your billing tier. Plans are billed monthly.</p>
<div class="radio-group">
<input type="radio" id="plan-free" name="subscription_plan" value="free" checked>
<label for="plan-free"><strong>Free Plan</strong> — Standard features with storage limits</label>
</div>
<div class="radio-group">
<input type="radio" id="plan-pro" name="subscription_plan" value="pro">
<label for="plan-pro"><strong>Professional Plan</strong> — Priority support and unlimited projects</label>
</div>
<div class="radio-group">
<input type="radio" id="plan-ent" name="subscription_plan" value="enterprise">
<label for="plan-ent"><strong>Enterprise Plan</strong> — Dedicated instances and custom SLA</label>
</div>
</fieldset>
<fieldset class="form-section">
<legend>Marketing Opt-in</legend>
<div class="checkbox-group">
<input type="checkbox" id="opt-in-partners" name="opt_in_partners">
<label for="opt-in-partners">Opt-in to partner promotions and special deals</label>
</div>
</fieldset>
<div class="form-actions">
<button type="submit" class="btn-submit">Save Settings</button>
<button type="button" class="btn-cancel">Cancel changes</button>
</div>
</form>
</main>
<footer class="form-footer">
<p>Form-heavy benchmark profile validation complete.</p>
</footer>
</body>
</html>`

const formHeavyCSS = `
body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; color: #334155; background: #f8fafc; }
.form-header { padding: 32px; background: #ffffff; border-bottom: 1px solid #e2e8f0; }
.form-header h1 { margin: 0 0 8px; font-size: 1.8rem; color: #0f172a; }
.desc { margin: 0; color: #64748b; font-size: 0.95rem; }
.form-container { max-width: 680px; margin: 32px auto; padding: 0 24px; }
.preferences-form { display: flex; flex-direction: column; gap: 24px; }
fieldset.form-section { border: 1px solid #cbd5e1; border-radius: 8px; padding: 24px; background: #ffffff; margin: 0; }
fieldset.form-section legend { padding: 0 8px; font-weight: 600; color: #0f172a; font-size: 1.1rem; }
.form-group { display: flex; flex-direction: column; gap: 6px; margin-bottom: 16px; }
.form-group:last-child { margin-bottom: 0; }
.form-group label { font-size: 0.9rem; font-weight: 500; color: #475569; }
.form-group input[type="text"], .form-group input[type="email"], .form-group select, .form-group textarea { padding: 10px 12px; border: 1px solid #cbd5e1; border-radius: 6px; font-size: 0.95rem; color: #1e293b; background: #ffffff; }
.form-group input:focus, .form-group select:focus, .form-group textarea:focus { outline: 2px solid #3b82f6; outline-offset: -1px; }
.section-note { font-size: 0.85rem; color: #64748b; margin: 0 0 16px; }
.checkbox-group, .radio-group { display: flex; align-items: flex-start; gap: 8px; margin-bottom: 12px; }
.checkbox-group:last-child, .radio-group:last-child { margin-bottom: 0; }
.checkbox-group label, .radio-group label { font-size: 0.9rem; color: #334155; line-height: 1.4; }
.form-actions { display: flex; gap: 16px; justify-content: flex-end; padding-top: 16px; }
button { padding: 10px 20px; border-radius: 6px; font-size: 0.95rem; font-weight: 500; cursor: pointer; border: 1px solid transparent; }
.btn-submit { background: #2563eb; color: #ffffff; }
.btn-submit:hover { background: #1d4ed8; }
.btn-cancel { background: #ffffff; border-color: #cbd5e1; color: #475569; }
.btn-cancel:hover { background: #f1f5f9; }
.form-footer { padding: 32px; text-align: center; font-size: 0.85rem; color: #64748b; }
`

const imageHeavyHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Image-Heavy Page</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
<header class="gallery-header">
<h1>Deterministic Image Gallery</h1>
<p class="description">A page populated with multiple local deterministic images (using base64 PNG data URIs) to benchmark image loading, layout, and rendering pipelines.</p>
</header>
<main class="gallery-container">
<div class="gallery">
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGJhYPjPgBsw4ZEbwdKAAAAA//9C0AEWskL5ggAAAABJRU5ErkJggg==" alt="Blue Image"><p class="label">Blue Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGElEQVR4nGL5/58BD2DCJzlypQEBAAD//3epAhVNqa1uAAAAAElFTkSuQmCC" alt="Yellow Image"><p class="label">Yellow Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGL5z/CfATdgwiM3gqUBAQAA//92qgIVlts9cQAAAABJRU5ErkJggg==" alt="Magenta Image"><p class="label">Magenta Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAF0lEQVR4nGL5z4APMOGVHbHSgAAAAP//RM4BFjLZ0j4AAAAASUVORK5CYII=" alt="Red Image"><p class="label">Red Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGJh+P+fATdgwiM3gqUBAQAA//91qwIVE6EUawAAAABJRU5ErkJggg==" alt="Cyan Image"><p class="label">Cyan Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGL5//8/A27AhEduBEsDAgAA//+phQMU+N7ExgAAAABJRU5ErkJggg==" alt="White Image"><p class="label">White Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAFElEQVR4nGJiwAtGpbECQAAAAP//DogAFaNSFa8AAAAASUVORK5CYII=" alt="Black Image"><p class="label">Black Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGJpaGhgwA2Y8MiNYGlAAAAA///fAwGXFadweQAAAABJRU5ErkJggg==" alt="Gray Image"><p class="label">Gray Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGElEQVR4nGL5v5QBD2DCJzlypQEBAAD//wthAbt2sJOkAAAAAElFTkSuQmCC" alt="Orange Image"><p class="label">Orange Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGJpYGhgwA2Y8MiNYGlAAAAA//9FAwEXDxD77wAAAABJRU5ErkJggg==" alt="Purple Image"><p class="label">Purple Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGUlEQVR4nGL5f+A0A27AhEduBEsDAgAA//8fXQKh5VAawQAAAABJRU5ErkJggg==" alt="Pink Image"><p class="label">Pink Item</p></div>
  <div class="gallery-item"><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAIAAAACUFjqAAAAGBhEQVR4nGL5z4APMOGVHbHSgAAAAP//RM4BFjLZ0j4AAAAASUVORK5CYII=" alt="Green Image"><p class="label">Green Item</p></div>
</div>
</main>
<footer class="gallery-footer">
<p>End of deterministic image gallery. Confirmed 12 test assets.</p>
</footer>
</body>
</html>`

const imageHeavyCSS = `
body { margin: 0; font-family: system-ui, -apple-system, sans-serif; color: #111827; background: #f9fafb; }
.gallery-header { padding: 32px; background: #ffffff; border-bottom: 1px solid #e5e7eb; }
.gallery-header h1 { margin: 0 0 8px; font-size: 1.75rem; color: #111827; }
.description { margin: 0; color: #4b5563; font-size: 0.95rem; line-height: 1.5; }
.gallery-container { padding: 32px; max-width: 1000px; margin: 0 auto; }
.gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: 16px; }
.gallery-item { background: #ffffff; border: 1px solid #e5e7eb; border-radius: 8px; padding: 12px; display: flex; flex-direction: column; align-items: center; justify-content: center; box-shadow: 0 1px 2px rgba(0,0,0,0.05); }
.gallery-item img { width: 80px; height: 80px; border-radius: 4px; object-fit: cover; }
.gallery-item p.label { margin: 8px 0 0; font-size: 0.85rem; font-weight: 500; color: #374151; }
.gallery-footer { padding: 24px 32px; background: #f3f4f6; text-align: center; font-size: 0.85rem; color: #6b7280; margin-top: 48px; border-top: 1px solid #e5e7eb; }
`

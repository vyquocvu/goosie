package dom

import (
	"context"
	"strings"
	"testing"
)

// TestAdversarial_DOM_SelectorEngine stresses Go-side query selector matching
// across deep trees, sibling combinators, comma-separated combinations, attribute matchers,
// and malformed selectors.
func TestAdversarial_DOM_SelectorEngine(t *testing.T) {
	parser := NewParser()

	// Construct complex HTML document
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head><title>Adversarial Test</title></head>
		<body>
			<header id="main-header" class="nav header-primary" data-section="top">
				<nav class="menu main-menu" role="navigation">
					<ul>
						<li class="item active"><a href="/home" class="link">Home</a></li>
						<li class="item"><a href="/docs" class="link doc-link" target="_blank" data-topic="intro">Docs</a></li>
						<li class="item disabled"><span class="link disabled">Disabled</span></li>
					</ul>
				</nav>
			</header>
			<main id="content" class="container fluid">
				<section id="sec-1" class="section-block" data-category="featured">
					<h1 class="title main-title" data-lang="en-US">Main Title</h1>
					<p class="summary lead">First lead paragraph</p>
					<p class="body-text" data-level="1">Standard body text</p>
					<div class="nested-1">
						<div class="nested-2">
							<div class="nested-3">
								<article id="art-deep" class="article-card" data-tags="news tech goosie" data-version="v2.1.0">
									<h2 class="sub-title">Article Subheading</h2>
									<p class="article-para">Article deep paragraph text</p>
									<a href="https://example.com/art" class="cta-link" rel="noopener">Read More</a>
								</article>
							</div>
						</div>
					</div>
				</section>
				<section id="sec-2" class="section-block" data-category="archive">
					<h2 class="title section-title">Archive</h2>
					<table id="data-table" class="table striped bordered" border="1" cellpadding="5">
						<thead>
							<tr><th>ID</th><th>Name</th><th>Status</th></tr>
						</thead>
						<tbody>
							<tr class="row-even"><td class="cell id">1</td><td class="cell name">Alpha</td><td class="cell status active">Active</td></tr>
							<tr class="row-odd"><td class="cell id">2</td><td class="cell name">Beta</td><td class="cell status inactive">Inactive</td></tr>
						</tbody>
					</table>
				</section>
			</main>
			<footer id="main-footer" class="footer footer-dark">
				<p class="copy">&copy; 2026 Goosie</p>
			</footer>
		</body>
		</html>
	`

	t.Run("DeepDescendantAndChildCombinators", func(t *testing.T) {
		elem, err := parser.QuerySelector(htmlContent, "main#content.container > section#sec-1 .nested-1 .nested-2 .nested-3 > article#art-deep.article-card")
		if err != nil {
			t.Fatalf("QuerySelector failed: %v", err)
		}
		if elem == nil {
			t.Fatalf("expected element to be found, got nil")
		}
		if elem.ID != "art-deep" {
			t.Errorf("expected ID 'art-deep', got %q", elem.ID)
		}
	})

	t.Run("AdjacentAndGeneralSiblingCombinators", func(t *testing.T) {
		// h1 + p should match p.summary.lead
		elem, err := parser.QuerySelector(htmlContent, "h1.title + p.summary")
		if err != nil {
			t.Fatalf("Adjacent sibling query failed: %v", err)
		}
		if elem == nil || elem.Classes[0] != "summary" {
			t.Errorf("expected p.summary, got %v", elem)
		}

		// h1 ~ div should match div.nested-1
		elemGeneral, err := parser.QuerySelector(htmlContent, "h1.title ~ div.nested-1")
		if err != nil {
			t.Fatalf("General sibling query failed: %v", err)
		}
		if elemGeneral == nil || len(elemGeneral.Classes) == 0 || elemGeneral.Classes[0] != "nested-1" {
			t.Errorf("expected div.nested-1, got %v", elemGeneral)
		}
	})

	t.Run("CommaSeparatedSelectorLists", func(t *testing.T) {
		elements, err := parser.QuerySelectorAll(htmlContent, "h1, h2, #main-footer, .doc-link")
		if err != nil {
			t.Fatalf("QuerySelectorAll with comma list failed: %v", err)
		}
		// h1 (Main Title), h2 (Article Subheading, Archive), #main-footer, .doc-link (Docs) -> 5 elements
		if len(elements) < 5 {
			t.Errorf("expected at least 5 elements from comma list, got %d", len(elements))
		}
	})

	t.Run("AttributeOperatorExhaustiveCheck", func(t *testing.T) {
		tests := []struct {
			selector string
			wantID   string
		}{
			{`[data-section="top"]`, "main-header"},
			{`[data-category^="feat"]`, "sec-1"},
			{`[data-version$=".0"]`, "art-deep"},
			{`[data-tags*="tech"]`, "art-deep"},
			{`[data-tags~="goosie"]`, "art-deep"},
			{`[data-lang|="en"]`, ""}, // h1 has no id, but matches
			{`[target="_blank"]`, ""}, // a.doc-link
			{`table[border="1"][cellpadding="5"]`, "data-table"},
		}

		for _, tt := range tests {
			t.Run(tt.selector, func(t *testing.T) {
				elem, err := parser.QuerySelector(htmlContent, tt.selector)
				if err != nil {
					t.Fatalf("QuerySelector(%q) error: %v", tt.selector, err)
				}
				if elem == nil {
					t.Fatalf("QuerySelector(%q) returned nil", tt.selector)
				}
				if tt.wantID != "" && elem.ID != tt.wantID {
					t.Errorf("QuerySelector(%q) got ID %q, want %q", tt.selector, elem.ID, tt.wantID)
				}
			})
		}
	})

	t.Run("MalformedSelectorGracefulRecovery", func(t *testing.T) {
		malformed := []string{
			"",
			"   ",
			":::pseudo",
			"[broken",
			"div > > > p",
			",,,",
			"#",
			".",
		}

		for _, sel := range malformed {
			t.Run("Sel_"+sel, func(t *testing.T) {
				elem, err := parser.QuerySelector(htmlContent, sel)
				if err != nil {
					t.Fatalf("unexpected error on malformed selector %q: %v", sel, err)
				}
				// None should match or panic
				if elem != nil && sel == "" {
					t.Errorf("empty selector should return nil, got %v", elem)
				}
			})
		}
	})
}

// TestAdversarial_ScriptMIMETypeFiltering tests MIME type acceptance/rejection
// and mixed HTML document script extraction.
func TestAdversarial_ScriptMIMETypeFiltering(t *testing.T) {
	t.Run("MIMETypeAcceptanceMatrix", func(t *testing.T) {
		validJS := []string{
			"",
			"   ",
			"text/javascript",
			"text/javascript; charset=utf-8",
			"application/javascript",
			"application/javascript;charset=utf-8",
			"text/ecmascript",
			"application/ecmascript",
			"text/jscript",
			"text/livescript",
			"javascript",
			"module",
			"TEXT/JAVASCRIPT",
			"Application/JavaScript",
		}

		for _, mime := range validJS {
			if !IsJavaScriptMIMEType(mime) {
				t.Errorf("expected IsJavaScriptMIMEType(%q) == true, got false", mime)
			}
		}

		invalidJS := []string{
			"application/ld+json",
			"application/json",
			"text/template",
			"text/html",
			"text/plain",
			"text/xml",
			"application/xml",
			"text/vbscript",
			"vbscript",
			"text/x-handlebars-template",
			"importmap",
		}

		for _, mime := range invalidJS {
			if IsJavaScriptMIMEType(mime) {
				t.Errorf("expected IsJavaScriptMIMEType(%q) == false, got true", mime)
			}
		}
	})

	t.Run("MixedHTMLDocumentResourceDiscovery", func(t *testing.T) {
		mixedHTML := `
			<!DOCTYPE html>
			<html>
			<head>
				<script type="application/ld+json">{"@context": "https://schema.org", "@type": "WebSite"}</script>
				<script src="https://example.com/app.js"></script>
				<script type="text/template" id="tmpl"><div>{{name}}</div></script>
				<script type="text/javascript; charset=utf-8" src="https://example.com/vendor.js"></script>
				<script type="application/json" id="data">{"foo":"bar"}</script>
				<script type="module" src="https://example.com/mod.js"></script>
			</head>
			<body>
				<script>console.log("inline js");</script>
				<script type="text/html"><span>ignore me</span></script>
			</body>
			</html>
		`

		var scriptsDiscovered []Resource
		_, err := NewParser().ParseDocumentCtx(context.Background(), strings.NewReader(mixedHTML), ParseConfig{
			OnResource: func(res Resource) {
				if res.Kind == ResourceScript {
					scriptsDiscovered = append(scriptsDiscovered, res)
				}
			},
		})
		if err != nil {
			t.Fatalf("ParseDocumentCtx failed: %v", err)
		}

		// Should discover exactly 4 valid JS scripts:
		// 1. https://example.com/app.js (classic)
		// 2. https://example.com/vendor.js (classic, charset)
		// 3. https://example.com/mod.js (module)
		// 4. inline script
		if len(scriptsDiscovered) != 4 {
			t.Fatalf("expected 4 scripts discovered, got %d: %+v", len(scriptsDiscovered), scriptsDiscovered)
		}

		if scriptsDiscovered[0].URL != "https://example.com/app.js" {
			t.Errorf("expected first script app.js, got %s", scriptsDiscovered[0].URL)
		}
		if scriptsDiscovered[1].URL != "https://example.com/vendor.js" {
			t.Errorf("expected second script vendor.js, got %s", scriptsDiscovered[1].URL)
		}
		if scriptsDiscovered[2].URL != "https://example.com/mod.js" || scriptsDiscovered[2].ScriptMode != ScriptModeModule {
			t.Errorf("expected third script mod.js with ScriptModeModule, got %+v", scriptsDiscovered[2])
		}
		if !scriptsDiscovered[3].Inline {
			t.Errorf("expected fourth script to be Inline, got %+v", scriptsDiscovered[3])
		}
	})
}

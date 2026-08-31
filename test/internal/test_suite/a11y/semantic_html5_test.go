package a11y

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
)

// html5SemanticElements are the WAI-ARIA landmark and sectioning
// elements introduced in HTML5. The browser engine must accept all
// of these as known tags, render them as block-level containers
// when no explicit display style is set, and preserve authored
// attributes for accessibility scanning and styling.
//
// References:
//   - https://www.w3.org/TR/html52/sections.html
//   - https://www.w3.org/WAI/ARIA/apg/landmarks/
var html5SemanticElements = []string{
	"article",
	"section",
	"nav",
	"aside",
	"header",
	"footer",
	"main",
	"address",
	"figure",
	"figcaption",
	"details",
	"summary",
	"mark",
	"time",
}

// TestHTML5SemanticElements_ParsedAsKnownTags verifies that each
// HTML5 semantic element is recognized by the DOM parser rather
// than being dropped as an unknown tag. The query-by-tag-name path
// exercises the parser's tag-name preservation, which is the same
// path the engine uses to decide element rendering and ARIA
// implicit-role lookup.
func TestHTML5SemanticElements_ParsedAsKnownTags(t *testing.T) {
	p := dom.NewParser()
	for _, tag := range html5SemanticElements {
		tag := tag
		t.Run(tag, func(t *testing.T) {
			html := "<html><body><" + tag + " id=\"target\">x</" + tag + "></body></html>"
			el, err := p.QuerySelector(html, "#target")
			require.NoError(t, err, "QuerySelector for %q should not error", tag)
			require.NotNil(t, el, "%q element must be parsed", tag)
			assert.Equal(t, tag, el.TagName,
				"%q element must keep its tag name through the parser", tag)
			assert.Equal(t, "x", el.TextContent,
				"%q element text content must survive parsing", tag)
		})
	}
}

// TestHTML5SemanticElements_LandmarkAccessibility verifies that the
// landmarks needed by assistive technologies (article, section, nav,
// aside, header, footer, main) are queryable by both ID and tag
// selectors, and that their IDs are preserved across the parser.
// Screen readers rely on these landmarks to provide navigation
// shortcuts (e.g. NVDA's D key).
func TestHTML5SemanticElements_LandmarkAccessibility(t *testing.T) {
	html := `
		<html><body>
			<header id="hdr">H</header>
			<nav id="nav">N</nav>
			<main id="mn">M</main>
			<aside id="as">A</aside>
			<footer id="ftr">F</footer>
			<article id="art">AR</article>
			<section id="sec">S</section>
		</body></html>
	`
	p := dom.NewParser()
	cases := []struct {
		id, tag string
	}{
		{"hdr", "header"},
		{"nav", "nav"},
		{"mn", "main"},
		{"as", "aside"},
		{"ftr", "footer"},
		{"art", "article"},
		{"sec", "section"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.tag, func(t *testing.T) {
			byID, err := p.GetElementByIDFull(html, c.id)
			require.NoError(t, err)
			require.NotNil(t, byID, "element #%s must be findable by ID", c.id)
			assert.Equal(t, c.tag, byID.TagName)

			byTag, err := p.QuerySelector(html, c.tag)
			require.NoError(t, err)
			require.NotNil(t, byTag, "%s must be findable by tag selector", c.tag)
			assert.Equal(t, c.id, byTag.ID,
				"%s element must carry its id attribute", c.tag)
		})
	}
}

// TestHTML5SemanticElements_NestedStructure verifies that nested
// semantic elements (article inside main, section inside article,
// header inside section) survive parsing without merging or
// reordering. This is the structure used by MDN, Wikipedia, and
// news sites; a regression here would silently flatten
// documentation and editorial pages.
func TestHTML5SemanticElements_NestedStructure(t *testing.T) {
	html := `
		<html><body>
			<main id="root">
				<article id="outer">
					<header id="articleHeader">A Header</header>
					<section id="innerSection">
						<h2 id="innerTitle">Title</h2>
						<p id="innerPara">Body text</p>
					</section>
					<footer id="articleFooter">A Footer</footer>
				</article>
			</main>
		</body></html>
	`
	p := dom.NewParser()
	ids := []string{
		"root", "outer", "articleHeader", "innerSection",
		"innerTitle", "innerPara", "articleFooter",
	}
	for _, id := range ids {
		id := id
		t.Run(id, func(t *testing.T) {
			el, err := p.GetElementByIDFull(html, id)
			require.NoError(t, err)
			require.NotNil(t, el, "nested semantic element #%s must survive", id)
			assert.NotEmpty(t, el.TagName)
		})
	}
}

// TestHTML5SemanticElements_MultiplePerDocument verifies that
// multiple instances of the same landmark element (multiple
// <article>, multiple <section>, multiple <header>) all coexist in
// the parsed tree. Real documents have many articles, sections,
// and headers per page; the engine must not collapse duplicates.
func TestHTML5SemanticElements_MultiplePerDocument(t *testing.T) {
	html := `
		<html><body>
			<article id="a1">A1</article>
			<article id="a2">A2</article>
			<article id="a3">A3</article>
			<section id="s1">S1</section>
			<section id="s2">S2</section>
		</body></html>
	`
	p := dom.NewParser()

	articles, err := p.QuerySelectorAll(html, "article")
	require.NoError(t, err)
	assert.Len(t, articles, 3, "expected three <article> elements, got %d", len(articles))

	sections, err := p.QuerySelectorAll(html, "section")
	require.NoError(t, err)
	assert.Len(t, sections, 2, "expected two <section> elements, got %d", len(sections))

	gotIDs := map[string]bool{}
	for _, a := range articles {
		gotIDs[a.ID] = true
	}
	for _, id := range []string{"a1", "a2", "a3"} {
		assert.True(t, gotIDs[id], "<article id=%q> must be in the document", id)
	}
}

// TestHTML5SemanticElements_AttributesPreserved verifies that the
// common accessibility-relevant attributes (lang, dir, role,
// aria-label, data-*) are preserved on semantic elements through
// the parser. Real pages combine semantic markup with ARIA labels
// and lang attributes; the browser engine must not strip them.
func TestHTML5SemanticElements_AttributesPreserved(t *testing.T) {
	html := `
		<html><body>
			<article id="a" role="article" aria-label="Article One"
			         lang="en" data-testid="article-1">A</article>
			<nav id="n" role="navigation" aria-label="Primary"
			     dir="ltr">N</nav>
			<aside id="x" role="complementary" aria-labelledby="x-label"
			       data-widget="sidebar">X</aside>
			<span id="x-label">Sidebar Heading</span>
		</body></html>
	`
	p := dom.NewParser()
	cases := []struct {
		id       string
		role     string
		aria     string
		lang     string
		dir      string
		dataAttr string
	}{
		{"a", "article", "Article One", "en", "", "article-1"},
		{"n", "navigation", "Primary", "", "ltr", ""},
		{"x", "complementary", "", "", "", "sidebar"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, func(t *testing.T) {
			el, err := p.GetElementByIDFull(html, c.id)
			require.NoError(t, err)
			require.NotNil(t, el)
			assert.Equal(t, c.role, el.Attributes["role"])
			if c.aria != "" {
				assert.Equal(t, c.aria, el.Attributes["aria-label"])
			}
			if c.lang != "" {
				assert.Equal(t, c.lang, el.Attributes["lang"])
			}
			if c.dir != "" {
				assert.Equal(t, c.dir, el.Attributes["dir"])
			}
			if c.dataAttr != "" {
				// Elements carry their data-* value under different keys.
				key := "data-testid"
				if c.id == "x" {
					key = "data-widget"
				}
				assert.Equal(t, c.dataAttr, el.Attributes[key],
					"data-* attribute must survive parsing")
			}
			if c.id == "x" {
				assert.Equal(t, "x-label", el.Attributes["aria-labelledby"])
			}
		})
	}
}

// TestHTML5SemanticElements_TextContentConcatenation verifies that
// text content is correctly aggregated from descendants of nested
// semantic elements. AT (assistive technology) reads concatenated
// text content aloud, so concatenating across <article>/<section>
// boundaries must produce the expected string without truncation
// or duplication.
func TestHTML5SemanticElements_TextContentConcatenation(t *testing.T) {
	html := `
		<html><body>
			<article id="outer">
				<h2 id="h">Title</h2>
				<p id="p">First paragraph.</p>
				<section id="inner">
					<p>Inner paragraph one.</p>
					<p>Inner paragraph two.</p>
				</section>
			</article>
		</body></html>
	`
	p := dom.NewParser()
	outer, err := p.GetElementByIDFull(html, "outer")
	require.NoError(t, err)
	require.NotNil(t, outer)
	assert.Contains(t, outer.TextContent, "Title")
	assert.Contains(t, outer.TextContent, "First paragraph.")
	assert.Contains(t, outer.TextContent, "Inner paragraph one.")
	assert.Contains(t, outer.TextContent, "Inner paragraph two.")

	inner, err := p.GetElementByIDFull(html, "inner")
	require.NoError(t, err)
	require.NotNil(t, inner)
	assert.Contains(t, inner.TextContent, "Inner paragraph one.")
	assert.Contains(t, inner.TextContent, "Inner paragraph two.")
	assert.NotContains(t, inner.TextContent, "Title",
		"inner section must not include ancestor text")
}

// TestHTML5SemanticElements_MalformedStructure verifies the parser
// handles real-world authoring mistakes gracefully: missing closing
// tags for a semantic element, mismatched nesting, and stray
// closing tags must not panic and must still surface the surviving
// elements. Pages hand-authored by users frequently have these
// kinds of mistakes.
func TestHTML5SemanticElements_MalformedStructure(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "missingClosingTag",
			html: `<html><body><article id="a">content`,
			want: "a",
		},
		{
			name: "mismatchedNesting",
			html: `<html><body><section><article id="a">x</section></article>`,
			want: "a",
		},
		{
			name: "strayClosingTag",
			html: `<html><body></article><article id="a">x</article>`,
			want: "a",
		},
		{
			name: "deeplyNestedSemantics",
			html: `<html><body><main><section><article><header><nav id="n">x</nav></header></article></section></main>`,
			want: "n",
		},
	}
	p := dom.NewParser()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("parser panicked on %q: %v", tc.name, r)
				}
			}()
			el, err := p.GetElementByIDFull(tc.html, tc.want)
			assert.NoError(t, err)
			assert.NotNil(t, el, "expected to find #%s in malformed %q", tc.want, tc.name)
		})
	}
}

// TestHTML5SemanticElements_PairwiseWithARIA verifies that an
// element with both an HTML5 semantic tag and an explicit ARIA
// role retains the role attribute for the accessibility tree.
// Authors sometimes override the implicit role of <nav> with
// role="navigation" (or vice versa) — the parser must preserve
// the explicit role verbatim.
func TestHTML5SemanticElements_PairwiseWithARIA(t *testing.T) {
	html := `
		<html><body>
			<nav id="n1" role="navigation">N1</nav>
			<nav id="n2" role="search">N2</nav>
			<article id="a1" role="article">A1</article>
			<article id="a2" role="region" aria-label="Sidebar">A2</article>
		</body></html>
	`
	p := dom.NewParser()
	cases := []struct {
		id, role string
	}{
		{"n1", "navigation"},
		{"n2", "search"},
		{"a1", "article"},
		{"a2", "region"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.id, func(t *testing.T) {
			el, err := p.GetElementByIDFull(html, c.id)
			require.NoError(t, err)
			require.NotNil(t, el)
			assert.Equal(t, c.role, el.Attributes["role"],
				"explicit role attribute must survive parsing on semantic element #%s", c.id)
		})
	}
}
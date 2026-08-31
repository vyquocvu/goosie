package integration

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestLinkExtraction_AnchorElements verifies that the DOM parser
// correctly extracts <a href> attributes from a parsed HTML
// document. The test uses the same parser the browser engine
// drives at load time, so a regression here directly affects
// navigation behaviour on online pages.
//
// We assert on multiple page shapes:
//   - simple absolute URLs
//   - relative paths
//   - root-relative paths
//   - mailto links
//   - fragment-only links
//   - nested anchors (link inside a paragraph)
//
// The contract under test is: every link that an author can
// legally write in HTML must round-trip through the parser and
// produce an element with the matching href attribute.
func TestLinkExtraction_AnchorElements(t *testing.T) {
	p := dom.NewParser()

	html := `
		<html><body>
			<a href="https://example.com/">Example</a>
			<a href="/about">About</a>
			<a href="contact.html">Contact</a>
			<a href="mailto:test@example.com">Email</a>
			<a href="#top">Top</a>
			<p>Visit <a href="https://nested.example/">our site</a> today.</p>
		</body></html>
	`
	links, err := p.QuerySelectorAll(html, "a")
	require.NoError(t, err)
	require.Len(t, links, 6, "expected six <a> elements, got %d", len(links))

	gotHrefs := make([]string, 0, len(links))
	for _, link := range links {
		gotHrefs = append(gotHrefs, link.Attributes["href"])
	}

	wantHrefs := []string{
		"https://example.com/",
		"/about",
		"contact.html",
		"mailto:test@example.com",
		"#top",
		"https://nested.example/",
	}
	assert.ElementsMatch(t, wantHrefs, gotHrefs,
		"every author-supplied href must round-trip through the parser")
}

// TestLinkExtraction_LinkRelAttributes verifies that <link> elements
// (used for stylesheets, icons, preloads, etc.) are also extracted
// with their href attributes. Online pages routinely ship dozens
// of <link rel="stylesheet"> and <link rel="icon"> elements;
// losing any of them silently degrades the page.
func TestLinkExtraction_LinkRelAttributes(t *testing.T) {
	p := dom.NewParser()

	html := `
		<html><head>
			<link rel="stylesheet" href="/css/main.css">
			<link rel="icon" href="/favicon.ico">
			<link rel="preload" href="/fonts/inter.woff2" as="font">
		</head><body></body></html>
	`
	links, err := p.QuerySelectorAll(html, "link")
	require.NoError(t, err)
	require.Len(t, links, 3)

	for _, link := range links {
		href, ok := link.Attributes["href"]
		assert.True(t, ok, "<link> missing href: %v", link.Attributes)
		assert.NotEmpty(t, href)
	}

	gotHrefs := map[string]bool{}
	for _, link := range links {
		gotHrefs[link.Attributes["href"]] = true
	}
	assert.True(t, gotHrefs["/css/main.css"])
	assert.True(t, gotHrefs["/favicon.ico"])
	assert.True(t, gotHrefs["/fonts/inter.woff2"])
}

// TestLinkResolution_AllShapes is the deterministic unit-level
// guard for the renderer's ResolveURL. It exercises every common
// link shape and compares the renderer's output to Go's stdlib
// url.URL.Parse. If Goosie's resolver ever diverges from the
// stdlib on a real-world shape, this test fails immediately.
//
// Each case is intentionally a closed HTML snippet the test
// renders, then queries for the link, then resolves. This makes
// the test self-contained: no network, no Playwright, no
// Fyne-specific code paths.
func TestLinkResolution_AllShapes(t *testing.T) {
	cases := []struct {
		name string
		base string
		href string
		want string
	}{
		{
			name: "absoluteHttps",
			base: "https://example.com/foo/bar",
			href: "https://other.example/path",
			want: "https://other.example/path",
		},
		{
			name: "absoluteHttpCrossScheme",
			base: "https://example.com/",
			href: "http://insecure.example/",
			want: "http://insecure.example/",
		},
		{
			name: "rootRelative",
			base: "https://example.com/foo/bar.html",
			href: "/other",
			want: "https://example.com/other",
		},
		{
			name: "directoryRelative",
			base: "https://example.com/foo/bar.html",
			href: "other.html",
			want: "https://example.com/foo/other.html",
		},
		{
			name: "dotDot",
			base: "https://example.com/foo/bar/baz.html",
			href: "../qux",
			want: "https://example.com/foo/qux",
		},
		{
			name: "fragmentOnly",
			base: "https://example.com/foo",
			href: "#section-2",
			want: "https://example.com/foo#section-2",
		},
		{
			name: "queryOnly",
			base: "https://example.com/foo",
			href: "?page=2",
			want: "https://example.com/foo?page=2",
		},
		{
			name: "fragmentAndQuery",
			base: "https://example.com/foo",
			href: "?page=2#top",
			want: "https://example.com/foo?page=2#top",
		},
		{
			name: "mailto",
			base: "https://example.com/",
			href: "mailto:test@example.com",
			want: "mailto:test@example.com",
		},
		{
			name: "emptyHref",
			base: "https://example.com/foo",
			href: "",
			want: "https://example.com/foo",
		},
		{
			name: "deepDotDot",
			base: "https://example.com/a/b/c/d/page.html",
			href: "../../../target",
			want: "https://example.com/a/target",
		},
		{
			name: "absolutePathWithQuery",
			base: "https://example.com/foo",
			href: "/path?query=1",
			want: "https://example.com/path?query=1",
		},
	}

	r := renderer.NewRenderer(1280, 800)
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r.SetCurrentURL(c.base)

			got := r.ResolveURL(c.href)

			// The renderer contract: ResolveURL matches the stdlib.
			base, err := url.Parse(c.base)
			require.NoError(t, err)
			stdlibWant, err := base.Parse(c.href)
			require.NoError(t, err)
			assert.Equal(t, stdlibWant.String(), got,
				"ResolveURL(%q) from %s must match stdlib", c.href, c.base)

			// The renderer contract: ResolveURL produces the
			// expected hand-checked result for hand-picked cases.
			assert.Equal(t, c.want, got,
				"ResolveURL(%q) from %s", c.href, c.base)
		})
	}
}

// TestLinkResolution_FromParsedHTMLPage combines the parser and
// resolver so a single test exercises the full "extract link then
// resolve it" pipeline. This is the deterministic sibling of the
// online-pages integration test — same shape, no network.
func TestLinkResolution_FromParsedHTMLPage(t *testing.T) {
	html := `
		<html>
		<head><base href="https://example.com/"></head>
		<body>
			<a href="/about">About</a>
			<a href="contact.html">Contact</a>
			<a href="https://other.example/path">Other</a>
		</body>
	</html>
	`
	p := dom.NewParser()
	links, err := p.QuerySelectorAll(html, "a")
	require.NoError(t, err)
	require.Len(t, links, 3)

	r := renderer.NewRenderer(1280, 800)
	r.SetCurrentURL("https://example.com/")

	got := make([]string, 0, len(links))
	for _, link := range links {
		href := link.Attributes["href"]
		got = append(got, r.ResolveURL(href))
	}

	want := []string{
		"https://example.com/about",
		"https://example.com/contact.html",
		"https://other.example/path",
	}
	assert.Equal(t, want, got,
		"parser+resolver must produce these resolved URLs")
}

// TestLinkResolution_EmptyAndNilInputs verifies the renderer's
// resolve function does not panic on pathological inputs. A panic
// in ResolveURL would crash the browser any time a malformed
// document is loaded, which is a routine occurrence on the open
// web.
func TestLinkResolution_EmptyAndNilInputs(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	r.SetCurrentURL("")

	assert.NotPanics(t, func() {
		_ = r.ResolveURL("")
	}, "ResolveURL(\"\") with empty base must not panic")
	assert.NotPanics(t, func() {
		_ = r.ResolveURL("not a url")
	}, "ResolveURL with garbage href must not panic")
	assert.NotPanics(t, func() {
		_ = r.ResolveURL("://malformed")
	}, "ResolveURL with malformed scheme must not panic")
}

// TestLinkResolution_AgainstRealisticBaseURL covers the
// high-traffic edge case where the page URL has a non-trivial
// path component. Online links frequently resolve against a base
// URL like /articles/2024/01/post-title, and a buggy resolver can
// drop the path prefix.
func TestLinkResolution_AgainstRealisticBaseURL(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)

	cases := []struct {
		base, href, want string
	}{
		{
			base: "https://blog.example.com/articles/2024/01/hello-world",
			href: "related-article.html",
			want: "https://blog.example.com/articles/2024/01/related-article.html",
		},
		{
			base: "https://blog.example.com/articles/2024/01/hello-world",
			href: "/archive",
			want: "https://blog.example.com/archive",
		},
		{
			base: "https://blog.example.com/articles/2024/01/hello-world",
			href: "../02/another-article",
			want: "https://blog.example.com/articles/2024/02/another-article",
		},
		{
			base: "https://blog.example.com/articles/2024/01/hello-world",
			href: "../../2023/legacy",
			want: "https://blog.example.com/articles/2023/legacy",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(strings.ReplaceAll(c.href, "/", "_"), func(t *testing.T) {
			r.SetCurrentURL(c.base)
			got := r.ResolveURL(c.href)
			assert.Equal(t, c.want, got,
				"ResolveURL(%q) from %s", c.href, c.base)
		})
	}
}

// TestLinkResolution_ImageSrcAndScriptSrc verifies that the
// resolver works for non-anchor elements that also need URL
// resolution: <img src>, <script src>, and <link href>. These
// all flow through the same resolver internally, so we lock the
// contract here for the non-anchor shapes.
func TestLinkResolution_ImageSrcAndScriptSrc(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	r.SetCurrentURL("https://example.com/articles/2024/index.html")

	cases := []struct {
		href, want string
	}{
		{"hero.png", "https://example.com/articles/2024/hero.png"},
		{"/img/logo.svg", "https://example.com/img/logo.svg"},
		{"https://cdn.example.com/lib.js", "https://cdn.example.com/lib.js"},
		{"../shared/script.js", "https://example.com/articles/shared/script.js"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.href, func(t *testing.T) {
			assert.Equal(t, c.want, r.ResolveURL(c.href),
				"ResolveURL(%q)", c.href)
		})
	}
}
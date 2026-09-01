package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSourceCache implements sourceCacheProvider for testing.
type mockSourceCache struct {
	bodies map[string]string
	calls  []string
}

func (m *mockSourceCache) CachedBody(rawURL string) (string, bool) {
	m.calls = append(m.calls, rawURL)
	body, ok := m.bodies[rawURL]
	return body, ok
}

func sourcesTestContext() *devtools.TabContext {
	return &devtools.TabContext{
		CurrentURL: "https://example.com/index.html",
		RawSource:  "<!DOCTYPE html>\n<html>\n<body>hi</body>\n</html>",
		RequestLog: &mockRequestLog{entries: []devtools.NetRequestEntry{
			{Method: "GET", URL: "https://example.com/index.html", Status: 200, ContentType: "text/html", Bytes: 2048, Duration: 80 * time.Millisecond},
			{Method: "GET", URL: "https://example.com/styles.css", Status: 200, ContentType: "text/css", Bytes: 1024, CacheHit: true, Duration: 30 * time.Millisecond},
			{Method: "GET", URL: "https://example.com/app.js", Status: 200, ContentType: "application/javascript", Bytes: 4096, Duration: 50 * time.Millisecond},
			{Method: "GET", URL: "https://example.com/logo.png", Status: 200, ContentType: "image/png", Bytes: 8192, Duration: 60 * time.Millisecond},
		}},
	}
}

// ---------------------------------------------------------------------------
// Initial / empty states
// ---------------------------------------------------------------------------

func TestSourcesPanel_InitialEmptyState(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	require.NotNil(t, p)
	assert.Empty(t, p.All())
	assert.Empty(t, p.VisibleResources())
	assert.Equal(t, "No resources", p.StatusLabel().Text)
	assert.Equal(t, devtools.SourcesSelectHint, p.SourceView().Text)
	assert.Empty(t, p.GutterView().Text)
	assert.Empty(t, p.SelectedURL())
}

func TestSourcesPanel_RefreshFromNilContext(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	assert.NotPanics(t, func() { p.RefreshFrom(nil) })
	assert.Empty(t, p.All())
	assert.Equal(t, "No resources", p.StatusLabel().Text)
}

func TestSourcesPanel_RefreshFromEmptyContext(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(&devtools.TabContext{})
	assert.Empty(t, p.All())
	assert.Equal(t, "No resources", p.StatusLabel().Text)
	assert.Equal(t, devtools.SourcesSelectHint, p.SourceView().Text)
}

func TestSourcesPanel_NilRequestLogKeepsDocument(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(&devtools.TabContext{RawSource: "<html>only doc</html>"})
	require.Len(t, p.All(), 1)
	assert.Equal(t, "document", p.All()[0].Type)
}

// ---------------------------------------------------------------------------
// Resource mapping
// ---------------------------------------------------------------------------

func TestSourcesPanel_DocumentFromRawSource(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(&devtools.TabContext{RawSource: "<html>hello</html>"})
	require.Len(t, p.All(), 1)
	doc := p.All()[0]
	assert.Equal(t, "document", doc.Type)
	assert.True(t, doc.HasContent)
	assert.Contains(t, doc.Content, "<html>")
	assert.NotEmpty(t, doc.Name)
}

func TestSourcesPanel_DocumentNamedFromURL(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(&devtools.TabContext{
		CurrentURL: "https://example.com/path/index.html",
		RawSource:  "<html>x</html>",
	})
	require.Len(t, p.All(), 1)
	assert.Equal(t, "index.html", p.All()[0].Name)
}

func TestSourcesPanel_SubResourcesClassified(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	require.Len(t, p.All(), 4)
	byType := map[string]devtools.SourceResource{}
	for _, r := range p.All() {
		byType[r.Type] = r
	}
	assert.Equal(t, "index.html", byType["document"].Name)
	assert.Equal(t, "styles.css", byType["stylesheet"].Name)
	assert.Equal(t, "app.js", byType["script"].Name)
	assert.Equal(t, "logo.png", byType["image"].Name)
	// Document is always listed first.
	assert.Equal(t, "document", p.All()[0].Type)
	// Metadata carried from the request log.
	assert.Equal(t, int64(1024), byType["stylesheet"].Bytes)
	assert.True(t, byType["stylesheet"].CacheHit)
}

func TestSourcesPanel_DeduplicatesMainDocument(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	count := 0
	for _, r := range p.All() {
		if r.Type == "document" {
			count++
			// The log entry enriches the document resource.
			assert.Equal(t, 200, r.Status)
			assert.Equal(t, int64(2048), r.Bytes)
		}
	}
	assert.Equal(t, 1, count)
}

// ---------------------------------------------------------------------------
// Selection: source viewing, line numbers, metadata
// ---------------------------------------------------------------------------

func TestSourcesPanel_SelectDocumentShowsNumberedSource(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.Tree().Select("res:0")
	assert.Equal(t, "https://example.com/index.html", p.SelectedURL())
	assert.Contains(t, p.SourceView().Text, "<body>hi</body>")
	assert.Equal(t, "1\n2\n3\n4", p.GutterView().Text)
	assert.Contains(t, p.DetailLabel().Text, "200")
	assert.Contains(t, p.DetailLabel().Text, "text/html")
	assert.True(t, p.Tree().IsBranchOpen("cat:document"))
}

func TestSourcesPanel_SelectStylesheetWithCachedBody(t *testing.T) {
	test.NewApp()
	cache := &mockSourceCache{bodies: map[string]string{
		"https://example.com/styles.css": "body { color: red; }",
	}}
	ctx := sourcesTestContext()
	ctx.SourceCache = cache
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(ctx)

	var cssIdx = -1
	for i, r := range p.VisibleResources() {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	require.NotEqual(t, -1, cssIdx)
	require.True(t, p.VisibleResources()[cssIdx].HasContent)

	p.Tree().Select(devtools.TreeResourceUID(cssIdx))
	assert.Equal(t, "body { color: red; }", p.SourceView().Text)
	assert.Equal(t, "1", p.GutterView().Text)
	assert.Contains(t, p.DetailLabel().Text, "text/css")
	assert.Contains(t, p.DetailLabel().Text, "1.0 KB")
}

func TestSourcesPanel_CacheOnlyQueriedForTextualResources(t *testing.T) {
	cache := &mockSourceCache{bodies: map[string]string{}}
	ctx := sourcesTestContext()
	ctx.SourceCache = cache
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(ctx)

	for _, url := range cache.calls {
		assert.NotContains(t, url, "logo.png")
	}
}

func TestSourcesPanel_TextResourceWithoutCachedBody(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil) // no SourceCache wired
	p.RefreshFrom(sourcesTestContext())

	cssIdx := -1
	for i, r := range p.VisibleResources() {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	require.NotEqual(t, -1, cssIdx)
	assert.False(t, p.VisibleResources()[cssIdx].HasContent)

	p.Tree().Select(devtools.TreeResourceUID(cssIdx))
	assert.Contains(t, p.SourceView().Text, "not captured")
	assert.Empty(t, p.GutterView().Text)
	// Metadata is still fully shown.
	assert.Contains(t, p.DetailLabel().Text, "styles.css")
}

func TestSourcesPanel_BinaryResourceShowsMetadata(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	imgIdx := -1
	for i, r := range p.VisibleResources() {
		if r.Type == "image" {
			imgIdx = i
		}
	}
	require.NotEqual(t, -1, imgIdx)

	p.Tree().Select(devtools.TreeResourceUID(imgIdx))
	assert.Contains(t, p.SourceView().Text, "Binary resource")
	assert.Contains(t, p.SourceView().Text, "image/png")
	assert.Contains(t, p.SourceView().Text, "8.0 KB")
	assert.Empty(t, p.GutterView().Text)
}

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

func TestSourcesPanel_FilterNarrowsResources(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	require.Len(t, p.VisibleResources(), 4)

	p.FilterEntry().SetText("styles")
	require.Len(t, p.VisibleResources(), 1)
	assert.Equal(t, "styles.css", p.VisibleResources()[0].Name)
	assert.Contains(t, p.StatusLabel().Text, "1 of 4")
}

func TestSourcesPanel_ClearFilterRestoresAll(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.FilterEntry().SetText("app")
	require.Len(t, p.VisibleResources(), 1)
	p.FilterEntry().SetText("")
	assert.Len(t, p.VisibleResources(), 4)
	assert.Contains(t, p.StatusLabel().Text, "4 resources")
}

func TestSourcesPanel_FilterMatchesNothing(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.FilterEntry().SetText("zzzz-no-match")
	assert.Empty(t, p.VisibleResources())
}

// ---------------------------------------------------------------------------
// Refresh behaviour / state lifecycle
// ---------------------------------------------------------------------------

func TestSourcesPanel_RefreshButtonPullsLatest(t *testing.T) {
	test.NewApp()
	src := "<html>v1</html>"
	p := devtools.NewSourcesPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RawSource: src, CurrentURL: "https://example.com/"}
	})

	test.Tap(p.RefreshBtn())
	require.Len(t, p.All(), 1)
	p.Tree().Select("res:0")
	assert.Contains(t, p.SourceView().Text, "v1")

	src = "<html>v2</html>"
	test.Tap(p.RefreshBtn())
	assert.Contains(t, p.SourceView().Text, "v2")
}

func TestSourcesPanel_StatusBarSummary(t *testing.T) {
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	assert.Equal(t, "4 resources · 15.0 KB", p.StatusLabel().Text)
}

func TestSourcesPanel_TabSwitchClearsSelection(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	p.Tree().Select("res:0")
	require.NotEmpty(t, p.SelectedURL())

	// Switching to a tab with no page loaded resets the panel.
	p.RefreshFrom(&devtools.TabContext{})
	assert.Empty(t, p.All())
	assert.Empty(t, p.SelectedURL())
	assert.Equal(t, devtools.SourcesSelectHint, p.SourceView().Text)
	assert.Empty(t, p.GutterView().Text)
	assert.Equal(t, "No resources", p.StatusLabel().Text)
}

func TestSourcesPanel_SelectionPreservedAcrossRefresh(t *testing.T) {
	test.NewApp()
	p := devtools.NewSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	cssIdx := -1
	for i, r := range p.VisibleResources() {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	p.Tree().Select(devtools.TreeResourceUID(cssIdx))
	require.Equal(t, "https://example.com/styles.css", p.SelectedURL())

	// Same data refreshed: selection by URL survives.
	p.RefreshFrom(sourcesTestContext())
	assert.Equal(t, "https://example.com/styles.css", p.SelectedURL())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestLineNumberGutter(t *testing.T) {
	assert.Equal(t, "1", devtools.LineNumberGutter("single"))
	assert.Equal(t, "1\n2\n3", devtools.LineNumberGutter("a\nb\nc"))
	assert.Equal(t, "1", devtools.LineNumberGutter("a\n")) // no phantom trailing line
	assert.Empty(t, devtools.LineNumberGutter(""))
}

func TestClassifySourceType(t *testing.T) {
	assert.Equal(t, "stylesheet", devtools.ClassifySourceType("text/css", ""))
	assert.Equal(t, "script", devtools.ClassifySourceType("application/javascript", ""))
	assert.Equal(t, "image", devtools.ClassifySourceType("image/png", ""))
	assert.Equal(t, "font", devtools.ClassifySourceType("font/woff2", ""))
	assert.Equal(t, "document", devtools.ClassifySourceType("text/html", ""))
	// Extension fallback when content type is missing.
	assert.Equal(t, "stylesheet", devtools.ClassifySourceType("", "https://x.test/a.css"))
	assert.Equal(t, "script", devtools.ClassifySourceType("", "https://x.test/b.js"))
	assert.Equal(t, "image", devtools.ClassifySourceType("", "https://x.test/c.svg"))
	assert.Equal(t, "other", devtools.ClassifySourceType("", "https://x.test/d.bin"))
	assert.Equal(t, "other", devtools.ClassifySourceType("application/json", ""))
}

func TestSourceBaseName(t *testing.T) {
	assert.Equal(t, "index.html", devtools.SourceBaseName("https://example.com/path/index.html"))
	assert.Equal(t, "styles.css", devtools.SourceBaseName("https://example.com/styles.css?v=2"))
	assert.Equal(t, "example.com", devtools.SourceBaseName("https://example.com/"))
	assert.Equal(t, "(current page)", devtools.SourceBaseName(""))
}

func TestTreeResourceUIDScheme(t *testing.T) {
	assert.Equal(t, "res:3", devtools.TreeResourceUID(3))
	assert.True(t, strings.HasPrefix(devtools.TreeCategoryUID("stylesheet"), "cat:"))
}

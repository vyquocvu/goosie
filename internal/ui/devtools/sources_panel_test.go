package devtools

import (
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

func sourcesTestContext() *TabContext {
	return &TabContext{
		CurrentURL: "https://example.com/index.html",
		RawSource:  "<!DOCTYPE html>\n<html>\n<body>hi</body>\n</html>",
		RequestLog: &mockRequestLog{entries: []NetRequestEntry{
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
	p := newSourcesPanel(nil)
	require.NotNil(t, p)
	assert.Empty(t, p.all)
	assert.Empty(t, p.visible)
	assert.Equal(t, "No resources", p.statusLabel.Text)
	assert.Equal(t, sourcesSelectHint, p.sourceView.Text)
	assert.Empty(t, p.gutterView.Text)
	assert.Empty(t, p.selectedURL)
}

func TestSourcesPanel_RefreshFromNilContext(t *testing.T) {
	p := newSourcesPanel(nil)
	assert.NotPanics(t, func() { p.RefreshFrom(nil) })
	assert.Empty(t, p.all)
	assert.Equal(t, "No resources", p.statusLabel.Text)
}

func TestSourcesPanel_RefreshFromEmptyContext(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(&TabContext{})
	assert.Empty(t, p.all)
	assert.Equal(t, "No resources", p.statusLabel.Text)
	assert.Equal(t, sourcesSelectHint, p.sourceView.Text)
}

func TestSourcesPanel_NilRequestLogKeepsDocument(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(&TabContext{RawSource: "<html>only doc</html>"})
	require.Len(t, p.all, 1)
	assert.Equal(t, "document", p.all[0].Type)
}

// ---------------------------------------------------------------------------
// Resource mapping
// ---------------------------------------------------------------------------

func TestSourcesPanel_DocumentFromRawSource(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(&TabContext{RawSource: "<html>hello</html>"})
	require.Len(t, p.all, 1)
	doc := p.all[0]
	assert.Equal(t, "document", doc.Type)
	assert.True(t, doc.HasContent)
	assert.Contains(t, doc.Content, "<html>")
	assert.NotEmpty(t, doc.Name)
}

func TestSourcesPanel_DocumentNamedFromURL(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(&TabContext{
		CurrentURL: "https://example.com/path/index.html",
		RawSource:  "<html>x</html>",
	})
	require.Len(t, p.all, 1)
	assert.Equal(t, "index.html", p.all[0].Name)
}

func TestSourcesPanel_SubResourcesClassified(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	require.Len(t, p.all, 4)
	byType := map[string]sourceResource{}
	for _, r := range p.all {
		byType[r.Type] = r
	}
	assert.Equal(t, "index.html", byType["document"].Name)
	assert.Equal(t, "styles.css", byType["stylesheet"].Name)
	assert.Equal(t, "app.js", byType["script"].Name)
	assert.Equal(t, "logo.png", byType["image"].Name)
	// Document is always listed first.
	assert.Equal(t, "document", p.all[0].Type)
	// Metadata carried from the request log.
	assert.Equal(t, int64(1024), byType["stylesheet"].Bytes)
	assert.True(t, byType["stylesheet"].CacheHit)
}

func TestSourcesPanel_DeduplicatesMainDocument(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	count := 0
	for _, r := range p.all {
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
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.tree.Select("res:0")
	assert.Equal(t, "https://example.com/index.html", p.selectedURL)
	assert.Contains(t, p.sourceView.Text, "<body>hi</body>")
	assert.Equal(t, "1\n2\n3\n4", p.gutterView.Text)
	assert.Contains(t, p.detailLabel.Text, "200")
	assert.Contains(t, p.detailLabel.Text, "text/html")
	assert.True(t, p.tree.IsBranchOpen("cat:document"))
}

func TestSourcesPanel_SelectStylesheetWithCachedBody(t *testing.T) {
	test.NewApp()
	cache := &mockSourceCache{bodies: map[string]string{
		"https://example.com/styles.css": "body { color: red; }",
	}}
	ctx := sourcesTestContext()
	ctx.SourceCache = cache
	p := newSourcesPanel(nil)
	p.RefreshFrom(ctx)

	var cssIdx = -1
	for i, r := range p.visible {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	require.NotEqual(t, -1, cssIdx)
	require.True(t, p.visible[cssIdx].HasContent)

	p.tree.Select(treeResourceUID(cssIdx))
	assert.Equal(t, "body { color: red; }", p.sourceView.Text)
	assert.Equal(t, "1", p.gutterView.Text)
	assert.Contains(t, p.detailLabel.Text, "text/css")
	assert.Contains(t, p.detailLabel.Text, "1.0 KB")
}

func TestSourcesPanel_CacheOnlyQueriedForTextualResources(t *testing.T) {
	cache := &mockSourceCache{bodies: map[string]string{}}
	ctx := sourcesTestContext()
	ctx.SourceCache = cache
	p := newSourcesPanel(nil)
	p.RefreshFrom(ctx)

	for _, url := range cache.calls {
		assert.NotContains(t, url, "logo.png")
	}
}

func TestSourcesPanel_TextResourceWithoutCachedBody(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil) // no SourceCache wired
	p.RefreshFrom(sourcesTestContext())

	cssIdx := -1
	for i, r := range p.visible {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	require.NotEqual(t, -1, cssIdx)
	assert.False(t, p.visible[cssIdx].HasContent)

	p.tree.Select(treeResourceUID(cssIdx))
	assert.Contains(t, p.sourceView.Text, "not captured")
	assert.Empty(t, p.gutterView.Text)
	// Metadata is still fully shown.
	assert.Contains(t, p.detailLabel.Text, "styles.css")
}

func TestSourcesPanel_BinaryResourceShowsMetadata(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	imgIdx := -1
	for i, r := range p.visible {
		if r.Type == "image" {
			imgIdx = i
		}
	}
	require.NotEqual(t, -1, imgIdx)

	p.tree.Select(treeResourceUID(imgIdx))
	assert.Contains(t, p.sourceView.Text, "Binary resource")
	assert.Contains(t, p.sourceView.Text, "image/png")
	assert.Contains(t, p.sourceView.Text, "8.0 KB")
	assert.Empty(t, p.gutterView.Text)
}

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

func TestSourcesPanel_FilterNarrowsResources(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	require.Len(t, p.visible, 4)

	p.filterEntry.SetText("styles")
	require.Len(t, p.visible, 1)
	assert.Equal(t, "styles.css", p.visible[0].Name)
	assert.Contains(t, p.statusLabel.Text, "1 of 4")
}

func TestSourcesPanel_ClearFilterRestoresAll(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.filterEntry.SetText("app")
	require.Len(t, p.visible, 1)
	p.filterEntry.SetText("")
	assert.Len(t, p.visible, 4)
	assert.Contains(t, p.statusLabel.Text, "4 resources")
}

func TestSourcesPanel_FilterMatchesNothing(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	p.filterEntry.SetText("zzzz-no-match")
	assert.Empty(t, p.visible)
}

// ---------------------------------------------------------------------------
// Refresh behaviour / state lifecycle
// ---------------------------------------------------------------------------

func TestSourcesPanel_RefreshButtonPullsLatest(t *testing.T) {
	test.NewApp()
	src := "<html>v1</html>"
	p := newSourcesPanel(func() *TabContext {
		return &TabContext{RawSource: src, CurrentURL: "https://example.com/"}
	})

	test.Tap(p.refreshBtn)
	require.Len(t, p.all, 1)
	p.tree.Select("res:0")
	assert.Contains(t, p.sourceView.Text, "v1")

	src = "<html>v2</html>"
	test.Tap(p.refreshBtn)
	assert.Contains(t, p.sourceView.Text, "v2")
}

func TestSourcesPanel_StatusBarSummary(t *testing.T) {
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	assert.Equal(t, "4 resources · 15.0 KB", p.statusLabel.Text)
}

func TestSourcesPanel_TabSwitchClearsSelection(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())
	p.tree.Select("res:0")
	require.NotEmpty(t, p.selectedURL)

	// Switching to a tab with no page loaded resets the panel.
	p.RefreshFrom(&TabContext{})
	assert.Empty(t, p.all)
	assert.Empty(t, p.selectedURL)
	assert.Equal(t, sourcesSelectHint, p.sourceView.Text)
	assert.Empty(t, p.gutterView.Text)
	assert.Equal(t, "No resources", p.statusLabel.Text)
}

func TestSourcesPanel_SelectionPreservedAcrossRefresh(t *testing.T) {
	test.NewApp()
	p := newSourcesPanel(nil)
	p.RefreshFrom(sourcesTestContext())

	cssIdx := -1
	for i, r := range p.visible {
		if r.Type == "stylesheet" {
			cssIdx = i
		}
	}
	p.tree.Select(treeResourceUID(cssIdx))
	require.Equal(t, "https://example.com/styles.css", p.selectedURL)

	// Same data refreshed: selection by URL survives.
	p.RefreshFrom(sourcesTestContext())
	assert.Equal(t, "https://example.com/styles.css", p.selectedURL)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestLineNumberGutter(t *testing.T) {
	assert.Equal(t, "1", lineNumberGutter("single"))
	assert.Equal(t, "1\n2\n3", lineNumberGutter("a\nb\nc"))
	assert.Equal(t, "1", lineNumberGutter("a\n")) // no phantom trailing line
	assert.Empty(t, lineNumberGutter(""))
}

func TestClassifySourceType(t *testing.T) {
	assert.Equal(t, "stylesheet", classifySourceType("text/css", ""))
	assert.Equal(t, "script", classifySourceType("application/javascript", ""))
	assert.Equal(t, "image", classifySourceType("image/png", ""))
	assert.Equal(t, "font", classifySourceType("font/woff2", ""))
	assert.Equal(t, "document", classifySourceType("text/html", ""))
	// Extension fallback when content type is missing.
	assert.Equal(t, "stylesheet", classifySourceType("", "https://x.test/a.css"))
	assert.Equal(t, "script", classifySourceType("", "https://x.test/b.js"))
	assert.Equal(t, "image", classifySourceType("", "https://x.test/c.svg"))
	assert.Equal(t, "other", classifySourceType("", "https://x.test/d.bin"))
	assert.Equal(t, "other", classifySourceType("application/json", ""))
}

func TestSourceBaseName(t *testing.T) {
	assert.Equal(t, "index.html", sourceBaseName("https://example.com/path/index.html"))
	assert.Equal(t, "styles.css", sourceBaseName("https://example.com/styles.css?v=2"))
	assert.Equal(t, "example.com", sourceBaseName("https://example.com/"))
	assert.Equal(t, "(current page)", sourceBaseName(""))
}

func TestTreeResourceUIDScheme(t *testing.T) {
	assert.Equal(t, "res:3", treeResourceUID(3))
	assert.True(t, strings.HasPrefix(treeCategoryUID("stylesheet"), "cat:"))
}

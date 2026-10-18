package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

func TestNormalizeURLInternalPages(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"goosie://version", "goosie://version"},
		{"GOOSIE://VERSION", "goosie://version"},
		{"  goosie://bookmarks  ", "goosie://bookmarks"},
		{"goosie://history", "goosie://history"},
		{"goosie://downloads", "goosie://downloads"},
		{"about:version", "goosie://version"},
		{"ABOUT:HISTORY", "goosie://history"},
		{"about:", "goosie://version"},
	}

	for _, tt := range tests {
		got := normalizeURL(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildInternalPageHTML(t *testing.T) {
	tmpDir := t.TempDir()
	bmStore := bookmarks.NewStore(filepath.Join(tmpDir, "bookmarks.json"))
	bmStore.Toggle("https://golang.org", "Go Language")

	histStore, err := history.Load(filepath.Join(tmpDir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	histStore.Record("https://example.com", "Example Domain")

	f := newNavTestPath(t)
	f.bookmarks = bmStore
	f.history = histStore

	// 1. Version page
	title, htmlContent := f.buildInternalPageHTML("goosie://version")
	if title != "About Goosie" {
		t.Errorf("version title = %q, want 'About Goosie'", title)
	}
	if !strings.Contains(htmlContent, "16 MiB") {
		t.Error("version page missing 16 MiB limit")
	}
	if !strings.Contains(htmlContent, "Go Version") {
		t.Error("version page missing Go Version label")
	}

	// 2. Bookmarks page
	title, htmlContent = f.buildInternalPageHTML("goosie://bookmarks")
	if title != "Bookmarks" {
		t.Errorf("bookmarks title = %q, want 'Bookmarks'", title)
	}
	if !strings.Contains(htmlContent, "https://golang.org") {
		t.Error("bookmarks page missing saved bookmark link")
	}

	// 3. History page
	title, htmlContent = f.buildInternalPageHTML("goosie://history")
	if title != "Browsing History" {
		t.Errorf("history title = %q, want 'Browsing History'", title)
	}
	if !strings.Contains(htmlContent, "https://example.com") {
		t.Error("history page missing visited entry")
	}

	// 4. Downloads page
	title, htmlContent = f.buildInternalPageHTML("goosie://downloads")
	if title != "Downloads" {
		t.Errorf("downloads title = %q, want 'Downloads'", title)
	}

	// 5. Unknown internal page falls back to index
	title, htmlContent = f.buildInternalPageHTML("goosie://nonexistent")
	if title != "Goosie Pages" {
		t.Errorf("index title = %q, want 'Goosie Pages'", title)
	}
	if !strings.Contains(htmlContent, "goosie://version") {
		t.Error("index page missing goosie://version link")
	}
}

func TestRenderInternalPage(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}

	f := newNavTestPath(t)
	f.fonts = fonts
	f.config = config{width: 800, height: 600, dpr: 1}

	layer, _, bg, sess, finalURL, err := f.renderInternalPage(context.Background(), "goosie://version", 800, 600, 1.0)
	if err != nil {
		t.Fatalf("renderInternalPage error: %v", err)
	}
	if layer == nil {
		t.Fatal("renderInternalPage returned nil layer")
	}
	if sess == nil {
		t.Fatal("renderInternalPage returned nil session")
	}
	if bg == 0 {
		t.Fatal("renderInternalPage returned 0 background color")
	}
	if finalURL != "goosie://version" {
		t.Errorf("finalURL = %q, want 'goosie://version'", finalURL)
	}
}

func TestNavigateToInternalPage(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	resCh := make(chan navResult, 1)
	f.navResults = resCh

	f.navigateTab("goosie://version")

	res := <-resCh
	if res.err != nil {
		t.Fatalf("navigation error: %v", res.err)
	}
	if res.layer == nil {
		t.Fatal("navigation produced nil layer")
	}

	f.applyNavResult(res)

	tab := f.tabMgr.Active()
	if tab.URL() != "goosie://version" {
		t.Errorf("tab.URL() = %q, want 'goosie://version'", tab.URL())
	}
	if tab.Title() != "About Goosie" {
		t.Errorf("tab.Title() = %q, want 'About Goosie'", tab.Title())
	}
	if tab.Error() != "" {
		t.Errorf("tab.Error() = %q, want empty", tab.Error())
	}
}

func TestInspectPageHTMLWithoutTabs(t *testing.T) {
	f := newNavTestPath(t)
	title, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if title != "Web Inspector" {
		t.Errorf("title = %q, want 'Web Inspector'", title)
	}
	if !strings.Contains(htmlContent, "Web Inspector") {
		t.Errorf("HTML should contain 'Web Inspector'")
	}
	if !strings.Contains(htmlContent, "No active web document found to inspect") {
		t.Errorf("HTML should indicate no active document")
	}
}

func TestInspectPageHTMLWithActiveTab(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	// Create a web tab with an actual HTML document
	sampleHTML := `<!DOCTYPE html><html><head><title>Test Subject</title><style>h1 { color: red; }</style></head><body><h1 id="headline" class="title">Hello World</h1><p class="desc">A sample paragraph.</p></body></html>`
	sess, err := engine.NewSession(sampleHTML, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.NewTab()
	tab.SetURL("https://example.com/test")
	tab.SetTitle("Test Subject")
	tab.SetSession(sess)
	f.tabMgr.SwitchTo(tab.ID())

	// Build inspector page inspecting this active tab
	title, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if title != "Web Inspector" {
		t.Errorf("title = %q, want 'Web Inspector'", title)
	}
	if !strings.Contains(htmlContent, "Test Subject") {
		t.Errorf("HTML should show title 'Test Subject'")
	}
	if !strings.Contains(htmlContent, "https://example.com/test") {
		t.Errorf("HTML should show URL 'https://example.com/test'")
	}
	// Verify DOM tree serialization
	if !strings.Contains(htmlContent, "headline") || !strings.Contains(htmlContent, "title") {
		t.Errorf("HTML should show DOM node attributes 'headline' and 'title'")
	}
	if !strings.Contains(htmlContent, "Hello World") {
		t.Errorf("HTML should show text content 'Hello World'")
	}
	// Verify metrics summary
	if !strings.Contains(htmlContent, "DOM Nodes") || !strings.Contains(htmlContent, "Tree Depth") {
		t.Errorf("HTML should show DOM metrics summary")
	}
	// Verify CSS Box Model
	if !strings.Contains(htmlContent, "Computed CSS Box Model") {
		t.Errorf("HTML should contain box model visualizer")
	}
}

func TestInspectPageHTMLEval(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.NewTab()
	tab.SetURL("https://example.com/eval")
	tab.SetTitle("Eval Tab")

	// Session with JS runtime
	rt, err := js.New(js.Options{URL: "https://example.com/eval"})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	sess, err := engine.NewSession("<html><body><h1>JS</h1></body></html>", nil, 800, engine.WithJS(rt))
	if err != nil {
		t.Fatalf("NewSession with JS failed: %v", err)
	}
	tab.SetSession(sess)
	f.tabMgr.SwitchTo(tab.ID())

	// Evaluate 10 + 20
	_, htmlContent := f.buildInternalPageHTML(fmt.Sprintf("goosie://inspect?tab=%d&eval=10+%%2B+20", tab.ID()))
	if !strings.Contains(htmlContent, "10 + 20") {
		t.Errorf("HTML should contain eval expression '10 + 20'")
	}
	if !strings.Contains(htmlContent, "30") {
		t.Errorf("HTML should contain evaluated result '30'")
	}
}

func TestInspectShortcut(t *testing.T) {
	inspectCalls := 0
	cw := &chromeWindow{
		onInspect: func() { inspectCalls++ },
	}

	// Cmd+Option+I -> onInspect
	handled := cw.handleTabShortcut('i', surface.ModCommand|surface.ModOption)
	if !handled || inspectCalls != 1 {
		t.Fatalf("Cmd+Option+I ('i'): handled=%v, inspectCalls=%d", handled, inspectCalls)
	}

	// Cmd+Option+I (uppercase 'I') -> onInspect
	handled = cw.handleTabShortcut('I', surface.ModCommand|surface.ModOption)
	if !handled || inspectCalls != 2 {
		t.Fatalf("Cmd+Option+I ('I'): handled=%v, inspectCalls=%d", handled, inspectCalls)
	}

	// Plain Cmd+I (without Option) should not trigger inspect
	handled = cw.handleTabShortcut('i', surface.ModCommand)
	if handled || inspectCalls != 2 {
		t.Fatalf("Plain Cmd+I should not trigger inspect: handled=%v, inspectCalls=%d", handled, inspectCalls)
	}
}

func TestPerformanceMemoryMeter(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.NewTab()
	tab.SetURL("https://example.com/meter")
	tab.SetTitle("Meter Tab")

	sess, err := engine.NewSession("<html><body><h1>Stats</h1></body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)
	f.tabMgr.SwitchTo(tab.ID())

	_, htmlContent := f.buildInternalPageHTML(fmt.Sprintf("goosie://inspect?tab=%d", tab.ID()))
	if !strings.Contains(htmlContent, "Performance &amp; Memory Meter") {
		t.Errorf("HTML should contain Performance & Memory Meter section")
	}
	if !strings.Contains(htmlContent, "Heap In-Use") {
		t.Errorf("HTML should contain Heap In-Use metric")
	}
	if !strings.Contains(htmlContent, "OS Reserved") {
		t.Errorf("HTML should contain OS Reserved metric")
	}
	if !strings.Contains(htmlContent, "Goroutines / CPUs") {
		t.Errorf("HTML should contain Goroutines / CPUs metric")
	}
}

func TestReaderInternalPage(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.NewTab()
	tab.SetURL("https://example.com/article")
	tab.SetTitle("Sample Article")

	articleHTML := `<html><head><title>Article Title</title></head><body>
		<article>
			<h1>Article Title</h1>
			<p>First paragraph of the substantive article text here.</p>
			<p>Second paragraph expanding on the topic with key details.</p>
		</article>
	</body></html>`
	sess, err := engine.NewSession(articleHTML, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)
	f.tabMgr.SwitchTo(tab.ID())

	title, htmlContent := f.buildInternalPageHTML("goosie://reader?url=https%3A%2F%2Fexample.com%2Farticle&theme=dark")
	if !strings.Contains(title, "Article Title") {
		t.Errorf("reader title %q should contain Article Title", title)
	}
	if !strings.Contains(htmlContent, "#1e1e24") {
		t.Errorf("dark theme reader page should contain dark background #1e1e24")
	}
	if !strings.Contains(htmlContent, "substantive article text") {
		t.Errorf("reader page should contain article text")
	}
	if !strings.Contains(htmlContent, "Back to original page") {
		t.Errorf("reader page should contain Back to original page")
	}
}

func TestShortcutsDuplicateAndReader(t *testing.T) {
	dupCalls := 0
	readerCalls := 0
	cw := &chromeWindow{
		onDuplicateTab: func() { dupCalls++ },
		onReaderMode:   func() { readerCalls++ },
	}

	// Cmd+Shift+D -> onDuplicateTab
	handled := cw.handleTabShortcut('d', surface.ModCommand|surface.ModShift)
	if !handled || dupCalls != 1 {
		t.Fatalf("Cmd+Shift+D ('d'): handled=%v, dupCalls=%d", handled, dupCalls)
	}

	// Cmd+Shift+D uppercase 'D' -> onDuplicateTab
	handled = cw.handleTabShortcut('D', surface.ModCommand|surface.ModShift)
	if !handled || dupCalls != 2 {
		t.Fatalf("Cmd+Shift+D ('D'): handled=%v, dupCalls=%d", handled, dupCalls)
	}

	// Cmd+Shift+R -> onReaderMode
	handled = cw.handleTabShortcut('r', surface.ModCommand|surface.ModShift)
	if !handled || readerCalls != 1 {
		t.Fatalf("Cmd+Shift+R ('r'): handled=%v, readerCalls=%d", handled, readerCalls)
	}

	// Cmd+Shift+R uppercase 'R' -> onReaderMode
	handled = cw.handleTabShortcut('R', surface.ModCommand|surface.ModShift)
	if !handled || readerCalls != 2 {
		t.Fatalf("Cmd+Shift+R ('R'): handled=%v, readerCalls=%d", handled, readerCalls)
	}
}

func TestInspectInternalPageNetworkActivity(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com")
	sess, err := engine.NewSession("<html><body>Hello Inspect</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	netClient := net.DefaultClient()
	defer netClient.Close()
	netLog := netClient.NetworkLog()
	netLog.Record(net.RequestTrace{
		URL:         "https://example.com/api/data.json",
		Method:      "GET",
		StatusCode:  200,
		ContentType: "application/json",
		Bytes:       1234,
		Duration:    15 * time.Millisecond,
		Timestamp:   time.Now(),
	})
	netLog.Record(net.RequestTrace{
		URL:           "https://ad.tracker.com/track.js",
		Method:        "GET",
		Blocked:       true,
		BlockedReason: "ad-tracker",
		Timestamp:     time.Now(),
	})
	f.client = netClient

	title, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if !strings.Contains(title, "Web Inspector") {
		t.Errorf("expected title Web Inspector, got %q", title)
	}
	if !strings.Contains(htmlContent, "Network Activity") {
		t.Error("expected html to contain Network Activity card")
	}
	if !strings.Contains(htmlContent, "https://example.com/api/data.json") {
		t.Error("expected html to contain logged request URL")
	}
	if !strings.Contains(htmlContent, "BLOCKED") {
		t.Error("expected html to contain BLOCKED badge for blocked request")
	}
}

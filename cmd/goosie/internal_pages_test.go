package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/raster"
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

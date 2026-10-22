package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/history"
)

func TestBookmarksPageDeleteActionPersistsAndRerenders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	store := bookmarks.NewStore(path)
	store.Toggle("https://golang.org", "Go")
	store.Toggle("https://example.com", "Example")
	if err := store.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	f := &framePath{bookmarks: store}
	action := "goosie://bookmarks?action=delete&url=" + url.QueryEscape("https://golang.org")
	_, html := buildBookmarksPageHTML(f, action)
	if !strings.Contains(html, "Bookmark removed and saved") {
		t.Error("expected success feedback after delete")
	}
	if strings.Contains(html, "https://golang.org") {
		t.Error("deleted bookmark still rendered after action")
	}
	if !strings.Contains(html, "https://example.com") {
		t.Error("surviving bookmark missing after delete")
	}

	// Persistence: a fresh store loaded from disk lost the deleted entry.
	reloaded, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reloaded.Contains("https://golang.org") {
		t.Error("deleted bookmark survived on disk")
	}
	if !reloaded.Contains("https://example.com") {
		t.Error("surviving bookmark lost on disk")
	}

	// Deleting an unknown URL reports an error, not silent success.
	_, html = buildBookmarksPageHTML(f, "goosie://bookmarks?action=delete&url="+url.QueryEscape("https://never.example"))
	if !strings.Contains(html, "Bookmark not found") {
		t.Error("expected not-found feedback for unknown delete target")
	}
}

func TestBookmarksPageRenameActionEscapesHostileTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookmarks.json")
	store := bookmarks.NewStore(path)
	hostileURL := `https://x.example/"><img src=x onerror=alert(1)>`
	store.Toggle(hostileURL, "Old Title")
	store.Toggle("javascript:alert(document.domain)", "JS Scheme")
	f := &framePath{bookmarks: store}

	hostileTitle := `<script>alert("xss")</script>`
	action := "goosie://bookmarks?action=rename&url=" + url.QueryEscape(hostileURL) + "&title=" + url.QueryEscape(hostileTitle)
	_, html := buildBookmarksPageHTML(f, action)

	if !strings.Contains(html, "Bookmark title updated and saved") {
		t.Error("expected success feedback after rename")
	}
	if strings.Contains(html, `<script>alert`) {
		t.Error("hostile title was not escaped in rendered HTML")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("escaped hostile title missing from rendered HTML")
	}
	if strings.Contains(html, `"><img src=x onerror=`) {
		t.Error("hostile URL broke out of its attribute in rendered HTML")
	}
	// A non-navigable scheme must render as inert text, never as an href.
	if strings.Contains(html, `href="javascript:`) {
		t.Error("javascript: URL rendered as a clickable link")
	}

	// Persistence keeps the verbatim (unescaped) title in JSON.
	reloaded, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, e := range reloaded.List() {
		if e.URL == hostileURL {
			found = true
			if e.Title != hostileTitle {
				t.Errorf("persisted title = %q, want verbatim %q", e.Title, hostileTitle)
			}
		}
	}
	if !found {
		t.Fatal("renamed bookmark missing after reload")
	}

	// Rename with an empty title is rejected with feedback.
	_, html = buildBookmarksPageHTML(f, "goosie://bookmarks?action=rename&url="+url.QueryEscape(hostileURL)+"&title=%20%20")
	if !strings.Contains(html, "Rename failed") {
		t.Error("expected rejection feedback for empty rename title")
	}
}

func TestBookmarksPageSearchFilter(t *testing.T) {
	dir := t.TempDir()
	store := bookmarks.NewStore(filepath.Join(dir, "bookmarks.json"))
	store.Toggle("https://golang.org/doc", "Go Documentation")
	store.Toggle("https://example.com", "Example Domain")
	f := &framePath{bookmarks: store}

	_, html := buildBookmarksPageHTML(f, "goosie://bookmarks?q=documentation")
	if !strings.Contains(html, "Go Documentation") {
		t.Error("search should match the documentation bookmark by title")
	}
	if strings.Contains(html, "Example Domain") {
		t.Error("non-matching bookmark rendered during search")
	}
	if !strings.Contains(html, "Clear search") {
		t.Error("search results should offer a clear-search link")
	}

	_, html = buildBookmarksPageHTML(f, "goosie://bookmarks?q=nomatchxyz")
	if !strings.Contains(html, "No bookmarks match your search") {
		t.Error("expected empty-search state")
	}

	// Search query survives into the delete action link.
	_, html = buildBookmarksPageHTML(f, "goosie://bookmarks?q=golang")
	if !strings.Contains(html, "action=delete&url="+url.QueryEscape("https://golang.org/doc")+"&q=golang") {
		t.Error("delete link should preserve the active search query")
	}
}

func TestHistoryPageDeleteEntryAndClearFlow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	store, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Record("https://a.example", "A")
	store.Record("https://b.example", "B")
	at := store.Entries()[0].At

	f := &framePath{history: store}

	// Per-entry delete via the RFC3339Nano timestamp the UI embeds.
	action := "goosie://history?action=delete&url=" + url.QueryEscape("https://a.example") + "&at=" + url.QueryEscape(at.Format(time.RFC3339Nano))
	_, html := buildHistoryPageHTML(f, action)
	if !strings.Contains(html, "History entry removed and saved") {
		t.Error("expected success feedback after entry delete")
	}
	if strings.Contains(html, "https://a.example") {
		t.Error("deleted entry still rendered")
	}
	reloaded, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Entries(); len(got) != 1 || got[0].URL != "https://b.example" {
		t.Fatalf("reloaded entries = %+v, want only b.example", got)
	}

	// Malformed timestamp is rejected with feedback.
	_, html = buildHistoryPageHTML(f, "goosie://history?action=delete&url="+url.QueryEscape("https://b.example")+"&at=notatime")
	if !strings.Contains(html, "missing or malformed") {
		t.Error("expected malformed-reference feedback")
	}
	if len(store.Entries()) != 1 {
		t.Error("malformed delete must not mutate the store")
	}

	// Clear requires the confirmation step.
	_, html = buildHistoryPageHTML(f, "goosie://history?action=clear")
	if !strings.Contains(html, "Clear all browsing history?") {
		t.Error("expected confirmation prompt before clearing")
	}
	if len(store.Entries()) != 1 {
		t.Error("confirmation step must not clear the log")
	}

	_, html = buildHistoryPageHTML(f, "goosie://history?action=clear&confirm=1")
	if !strings.Contains(html, "Browsing history cleared and saved") {
		t.Error("expected success feedback after confirmed clear")
	}
	if len(store.Entries()) != 0 {
		t.Error("confirmed clear left entries behind")
	}
	reloaded, err = history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Entries(); len(got) != 0 {
		t.Fatalf("reloaded entries after clear = %+v, want empty", got)
	}
}

func TestHistoryPageSearchFilter(t *testing.T) {
	dir := t.TempDir()
	store, err := history.Load(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.Record("https://golang.org/doc", "Go Documentation")
	store.Record("https://example.com", "Example Domain")
	f := &framePath{history: store}

	_, html := buildHistoryPageHTML(f, "goosie://history?q=golang")
	if !strings.Contains(html, "https://golang.org/doc") {
		t.Error("search should match the golang visit")
	}
	if strings.Contains(html, "Example Domain") {
		t.Error("non-matching entry rendered during search")
	}

	_, html = buildHistoryPageHTML(f, "goosie://history?q=nomatchxyz")
	if !strings.Contains(html, "No history entries match your search") {
		t.Error("expected empty-search state")
	}
}

// TestMgmtPagesPrivateSessionNilStores pins privacy isolation: with no
// profile stores (private session or failed load) the management pages show
// an explanation, offer no mutation controls, and actions are inert no-ops.
func TestMgmtPagesPrivateSessionNilStores(t *testing.T) {
	f := &framePath{}

	_, html := buildBookmarksPageHTML(f, "goosie://bookmarks")
	if !strings.Contains(html, "Bookmark storage is unavailable") {
		t.Error("expected private-session explanation on bookmarks page")
	}
	if strings.Contains(html, "action=delete") || strings.Contains(html, `name="title"`) {
		t.Error("mutation controls rendered without a store")
	}
	// Hostile action against a nil store must not panic.
	_, html = buildBookmarksPageHTML(f, "goosie://bookmarks?action=delete&url=https%3A%2F%2Fexample.com")
	if !strings.Contains(html, "Bookmark storage is unavailable") {
		t.Error("expected inert no-op page for delete action without a store")
	}

	_, html = buildHistoryPageHTML(f, "goosie://history")
	if !strings.Contains(html, "History storage is unavailable") {
		t.Error("expected private-session explanation on history page")
	}
	if strings.Contains(html, "Clear All History") || strings.Contains(html, "action=delete") {
		t.Error("mutation controls rendered without a store")
	}
	_, html = buildHistoryPageHTML(f, "goosie://history?action=clear&confirm=1")
	if !strings.Contains(html, "History storage is unavailable") {
		t.Error("expected inert no-op page for clear action without a store")
	}
}

// TestMgmtPagesSaveErrorFeedback pins that a persistence failure surfaces in
// the UI instead of silently pretending the action succeeded.
func TestMgmtPagesSaveErrorFeedback(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "occupied")
	if err := os.MkdirAll(badPath, 0o755); err != nil {
		t.Fatal(err)
	}
	store := bookmarks.NewStore(badPath)
	store.Toggle("https://a.example", "A")

	f := &framePath{bookmarks: store}
	_, html := buildBookmarksPageHTML(f, "goosie://bookmarks?action=delete&url="+url.QueryEscape("https://a.example"))
	if !strings.Contains(html, "saving to disk failed") {
		t.Error("expected save-failure feedback banner")
	}
}

// TestMgmtPagesRoutedThroughInternalPageHTML checks the pages are reachable
// through the shared goosie:// router with their query strings intact.
func TestMgmtPagesRoutedThroughInternalPageHTML(t *testing.T) {
	dir := t.TempDir()
	bmPath := filepath.Join(dir, "bookmarks.json")
	store := bookmarks.NewStore(bmPath)
	store.Toggle("https://golang.org", "Go")
	histPath := filepath.Join(dir, "history.json")
	hist, err := history.Load(histPath)
	if err != nil {
		t.Fatal(err)
	}
	hist.Record("https://example.com", "Example")

	f := &framePath{bookmarks: store, history: hist}

	title, html := f.buildInternalPageHTML("goosie://bookmarks?action=delete&url=" + url.QueryEscape("https://golang.org"))
	if title != "Bookmarks" {
		t.Errorf("routed bookmarks title = %q", title)
	}
	if !strings.Contains(html, "Bookmark removed and saved") {
		t.Error("routed delete action did not execute")
	}

	title, html = f.buildInternalPageHTML("goosie://history?q=example")
	if title != "Browsing History" {
		t.Errorf("routed history title = %q", title)
	}
	if !strings.Contains(html, "https://example.com") {
		t.Error("routed history search did not render the match")
	}
}

// TestNormalizeURLGoosieQueryCasePreserved pins the fix that keeps
// case-sensitive action parameters intact through navigation.
func TestNormalizeURLGoosieQueryCasePreserved(t *testing.T) {
	in := "GOOSIE://Bookmarks?action=rename&url=https%3A%2F%2FExample.COM%2FPath&title=My+Title"
	want := "goosie://bookmarks?action=rename&url=https%3A%2F%2FExample.COM%2FPath&title=My+Title"
	if got := normalizeURL(in); got != want {
		t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
	}
	// Bare page names still lowercase as before.
	if got := normalizeURL("GOOSIE://HISTORY"); got != "goosie://history" {
		t.Errorf("normalizeURL(GOOSIE://HISTORY) = %q, want goosie://history", got)
	}
}

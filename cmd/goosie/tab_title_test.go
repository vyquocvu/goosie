package main

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/history"
)

func titleNavPath(t *testing.T) (*framePath, *history.Store) {
	t.Helper()
	f := newNavTestPath(t)
	f.config.width, f.config.dpr, f.zoom = 256, 1, 1
	store, err := history.Load(t.TempDir() + "/history.json")
	if err != nil {
		t.Fatal(err)
	}
	f.history = store
	return f, store
}

func navigateWithTitle(t *testing.T, f *framePath, html, url string) {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 256)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 7
	f.applyNavResult(navResult{tabID: tab.ID(), serial: 7, url: url, layer: tab.Layer(), session: sess})
}

// Bug C2: the tab bar painted the bare URL because a completed navigation
// never looked at the document's <title>.
func TestTabTitleShowsDocumentTitleAfterNavigation(t *testing.T) {
	f, store := titleNavPath(t)
	navigateWithTitle(t, f, `<html><head><title>Hello Goosie</title></head><body>hi</body></html>`, "https://example.com/page")

	tab := f.tabMgr.Active()
	if got := tab.Title(); got != "Hello Goosie" {
		t.Errorf("tab.Title() = %q, want %q", got, "Hello Goosie")
	}
	if got := tab.URL(); got != "https://example.com/page" {
		t.Errorf("tab.URL() = %q, want the navigation URL", got)
	}
	// The visit log carries the same title the tab displays.
	entries := store.Entries()
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	if entries[0].Title != "Hello Goosie" {
		t.Errorf("history title = %q, want %q", entries[0].Title, "Hello Goosie")
	}
	if got := f.tabMgr.Tabs()[0].Title(); got != "Hello Goosie" {
		t.Errorf("tab bar title = %q, want %q", got, "Hello Goosie")
	}
}

// Markup usually breaks a title over several lines; the tab shows one line.
func TestTabTitleCollapsesWhitespace(t *testing.T) {
	f, _ := titleNavPath(t)
	navigateWithTitle(t, f, "<html><head><title>\n\t Welcome\n  to Goosie \n</title></head></html>", "https://example.com/ws")

	if got, want := f.tabMgr.Active().Title(), "Welcome to Goosie"; got != want {
		t.Errorf("tab.Title() = %q, want %q", got, want)
	}
}

// Without a usable title the tab keeps falling back to the URL.
func TestTabTitleFallsBackToURLWhenDocumentHasNoTitle(t *testing.T) {
	for _, tt := range []struct{ name, html string }{
		{"missing", `<html><head></head><body>no title</body></html>`},
		{"empty", `<html><head><title></title></head><body>blank title</body></html>`},
		{"whitespace", `<html><head><title>   </title></head><body>spaces</body></html>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := titleNavPath(t)
			navigateWithTitle(t, f, tt.html, "https://example.com/untitled")
			if got, want := f.tabMgr.Active().Title(), "https://example.com/untitled"; got != want {
				t.Errorf("tab.Title() = %q, want the URL %q", got, want)
			}
		})
	}
}

// A hostile document must not hand the tab bar — and the saved session — an
// arbitrarily long title.
func TestTabTitleTruncatesOverlongTitle(t *testing.T) {
	f, _ := titleNavPath(t)
	long := strings.Repeat("g", 160*3)
	navigateWithTitle(t, f, "<html><head><title>"+long+"</title></head></html>", "https://example.com/long")

	got := f.tabMgr.Active().Title()
	if r := []rune(got); len(r) > 160 {
		t.Errorf("tab.Title() rune length = %d, want at most %d", len(r), 160)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("tab.Title() = %q, want it to end with an ellipsis", got)
	}
	if !strings.HasPrefix(got, "gggg") {
		t.Errorf("tab.Title() = %q, want the start of the document title", got)
	}
}

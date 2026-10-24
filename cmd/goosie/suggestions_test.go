package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	internalnet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// profileSuggestionStores builds bookmarks and history stores whose URLs all
// match the query "go" so a leak into a private tab is impossible to miss.
func profileSuggestionStores(t *testing.T) (*bookmarks.Store, *history.Store) {
	t.Helper()
	tmpDir := t.TempDir()
	bmStore := bookmarks.NewStore(filepath.Join(tmpDir, "bookmarks.json"))
	bmStore.Toggle("https://golang.org", "Go Programming Language")
	bmStore.Toggle("https://github.com/vyquocvu/goosie", "Goosie Browser")
	histStore, err := history.Load(filepath.Join(tmpDir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	histStore.Record("https://news.ycombinator.com", "Hacker News")
	histStore.Record("https://golang.org/doc", "Go Documentation")
	return bmStore, histStore
}

// countProfileMatches reports how many suggestions came from the profile's
// bookmarks or history stores (internal goosie:// pages are bookmark-kind but
// carry goosie:// URLs, so they are excluded).
func countProfileMatches(sugs []toolbar.Suggestion) (bookmarksHits, historyHits int) {
	for _, s := range sugs {
		if strings.HasPrefix(s.URL, "goosie://") {
			continue
		}
		switch s.Kind {
		case toolbar.SuggestBookmark:
			bookmarksHits++
		case toolbar.SuggestHistory:
			historyHits++
		}
	}
	return
}

// TestQuerySuggestionsPrivateTabSuppressesProfileData guards the omnibox
// privacy contract: an incognito tab may show search suggestions and internal
// pages, but must never read the profile's bookmarks or history. Switching
// back to a normal tab must restore them.
func TestQuerySuggestionsPrivateTabSuppressesProfileData(t *testing.T) {
	bmStore, histStore := profileSuggestionStores(t)
	f := newNavTestPath(t)
	f.bookmarks = bmStore
	f.history = histStore

	regular := f.tabMgr.Active()
	sugs := f.querySuggestions("go")
	if bm, hist := countProfileMatches(sugs); bm == 0 || hist == 0 {
		t.Fatalf("normal tab suggestions missing profile data: bookmarks=%d history=%d", bm, hist)
	}

	incog := f.newIncognitoTab()
	if !incog.Incognito() {
		t.Fatal("newIncognitoTab did not create an incognito tab")
	}
	priv := f.querySuggestions("go")
	if bm, hist := countProfileMatches(priv); bm != 0 || hist != 0 {
		t.Errorf("incognito tab leaked profile data into suggestions: bookmarks=%d history=%d (%+v)", bm, hist, priv)
	}
	if len(priv) == 0 || priv[0].Kind != toolbar.SuggestSearch {
		t.Errorf("incognito tab lost the search suggestion: %+v", priv)
	}
	foundSettings := false
	for _, s := range f.querySuggestions("settings") {
		if s.URL == "goosie://settings" {
			foundSettings = true
		}
	}
	if !foundSettings {
		t.Error("internal pages must stay discoverable from an incognito tab")
	}

	f.switchTab(regular.ID())
	back := f.querySuggestions("go")
	if bm, hist := countProfileMatches(back); bm == 0 || hist == 0 {
		t.Errorf("profile suggestions lost after returning from incognito: bookmarks=%d history=%d", bm, hist)
	}
}

// TestQuerySuggestionsPrivateSessionSuppressesProfileData covers the whole-run
// -private mode: even a non-incognito tab object must not surface profile data.
func TestQuerySuggestionsPrivateSessionSuppressesProfileData(t *testing.T) {
	bmStore, histStore := profileSuggestionStores(t)
	f := newNavTestPath(t)
	f.bookmarks = bmStore
	f.history = histStore
	f.config.private = true
	priv := f.querySuggestions("go")
	if bm, hist := countProfileMatches(priv); bm != 0 || hist != 0 {
		t.Errorf("-private run leaked profile data into suggestions: bookmarks=%d history=%d", bm, hist)
	}
}

// TestSuggestionsClearedOnTabSwitch pins that switching tabs drops the
// dropdown the previous tab produced, so a fast switch cannot show stale data
// (and never keeps an incognito-produced list visible on a normal tab).
func TestSuggestionsClearedOnTabSwitch(t *testing.T) {
	bmStore, histStore := profileSuggestionStores(t)
	f := newNavTestPath(t)
	f.bookmarks = bmStore
	f.history = histStore
	tb := toolbar.NewState(800, nil)
	f.toolbar = tb

	tb.Suggestions = f.querySuggestions("go")
	if !tb.HasSuggestions() {
		t.Fatal("setup: expected a populated dropdown")
	}
	f.newIncognitoTab()
	if tb.HasSuggestions() {
		t.Error("stale suggestions survived switching to an incognito tab")
	}

	tb.Suggestions = f.querySuggestions("go")
	if !tb.HasSuggestions() {
		t.Fatal("setup: expected a populated dropdown")
	}
	incogTab := f.tabMgr.Active()
	f.switchTab(incogTab.ID())
	if tb.HasSuggestions() {
		t.Error("stale suggestions survived a tab switch")
	}
}

// TestSuggestionsClearedOnNavigationStart pins that a programmatic navigation
// (a link click, not an omnibox submit) also invalidates the dropdown.
func TestSuggestionsClearedOnNavigationStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!DOCTYPE html><html><body><h1>loaded</h1></body></html>")
	}))
	defer server.Close()

	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()
	f.client = internalnet.DefaultClient()
	defer f.client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts
	tb := toolbar.NewState(800, nil)
	f.toolbar = tb

	tb.Suggestions = []toolbar.Suggestion{
		{Title: "stale", URL: "https://stale.invalid", Kind: toolbar.SuggestHistory},
	}
	f.navigateTab(server.URL + "/index.html")
	if tb.HasSuggestions() {
		t.Error("stale suggestions survived a navigation start")
	}
	// Drain the load so the test leaves no in-flight navigation behind.
	res := <-f.navResults
	if res.err != nil {
		t.Fatalf("navigation failed: %v", res.err)
	}
}

func TestQuerySuggestions(t *testing.T) {
	tmpDir := t.TempDir()
	bmStore := bookmarks.NewStore(filepath.Join(tmpDir, "bookmarks.json"))
	bmStore.Toggle("https://golang.org", "Go Programming Language")
	bmStore.Toggle("https://github.com/vyquocvu/goosie", "Goosie Browser")

	histStore, err := history.Load(filepath.Join(tmpDir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	histStore.Record("https://news.ycombinator.com", "Hacker News")
	histStore.Record("https://golang.org/doc", "Go Documentation")

	f := newNavTestPath(t)
	f.bookmarks = bmStore
	f.history = histStore

	// Query for "go"
	sugs := f.querySuggestions("go")
	if len(sugs) == 0 {
		t.Fatal("querySuggestions('go') returned no suggestions")
	}

	// First should be Google Search suggestion
	if sugs[0].Kind != toolbar.SuggestSearch || !strings.Contains(sugs[0].URL, "q=go") {
		t.Errorf("sugs[0] = %v, want SuggestSearch with q=go", sugs[0])
	}

	// Should contain bookmarked golang.org or goosie
	foundBookmark := false
	foundHistory := false
	for _, s := range sugs {
		if s.Kind == toolbar.SuggestBookmark && strings.Contains(s.URL, "golang.org") {
			foundBookmark = true
		}
		if s.Kind == toolbar.SuggestHistory && strings.Contains(s.URL, "golang.org/doc") {
			foundHistory = true
		}
	}
	if !foundBookmark {
		t.Errorf("expected bookmark in suggestions: %v", sugs)
	}
	if !foundHistory {
		t.Errorf("expected history in suggestions: %v", sugs)
	}

	// Empty query returns nil
	if empty := f.querySuggestions("   "); len(empty) != 0 {
		t.Errorf("querySuggestions('') = %v, want nil", empty)
	}

	f.setPreferences(preferences.Settings{SearchURLTemplate: "https://search.example/?q={query}"})
	settings := f.querySuggestions("settings")
	if len(settings) < 2 || settings[0].URL != "https://search.example/?q=settings" {
		t.Fatalf("query suggestion did not use the profile search engine: %+v", settings)
	}
	foundSettings := false
	for _, suggestion := range settings {
		if suggestion.URL == "goosie://settings" {
			foundSettings = true
		}
	}
	if !foundSettings {
		t.Fatalf("settings page not discoverable from omnibox: %+v", settings)
	}
}

func TestChromeWindowSuggestionClickIntercept(t *testing.T) {
	mgr := tabs.NewManager(nil)
	mgr.NewTab()

	tb := toolbar.NewState(800, nil)
	tb.Suggestions = []toolbar.Suggestion{
		{Title: "Golang", URL: "https://golang.org", Kind: toolbar.SuggestBookmark},
	}

	var clickedLink string
	cw := &chromeWindow{
		toolbar:     tb,
		tabMgr:      mgr,
		scale:       1,
		onLinkClick: func(href string) { clickedLink = href },
	}

	// Click on suggestion row directly beneath toolbar (topOffset = chromeHeight)
	chromeH := cw.chromeHeight()
	sugRect := tb.SuggestionsRect(chromeH)
	if sugRect.Empty() {
		t.Fatal("SuggestionsRect is empty")
	}

	ev := surface.Event{
		Kind:   surface.EvPointer,
		Action: surface.PointerPress,
		Button: surface.ButtonLeft,
		Pos:    frame.Point{X: (sugRect.X0 + sugRect.X1) / 2, Y: (sugRect.Y0 + sugRect.Y1) / 2},
	}

	intercepted := cw.intercept(ev)
	if !intercepted {
		t.Fatal("click on suggestion should be intercepted")
	}
	if clickedLink != "https://golang.org" {
		t.Fatalf("clickedLink = %q, want https://golang.org", clickedLink)
	}
	if tb.HasSuggestions() {
		t.Error("suggestions should be closed after clicking a suggestion")
	}
}

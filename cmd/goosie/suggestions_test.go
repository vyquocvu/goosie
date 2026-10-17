package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/history"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

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

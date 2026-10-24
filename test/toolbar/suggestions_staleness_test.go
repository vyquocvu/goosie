package toolbar_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

func fakeSuggest(_ string) []toolbar.Suggestion {
	return []toolbar.Suggestion{
		{Title: "Search the web", URL: "https://www.google.com/search?q=golang", Kind: toolbar.SuggestSearch},
		{Title: "Golang", URL: "https://golang.org", Kind: toolbar.SuggestBookmark},
	}
}

// TestNavigateClearsStaleSuggestions pins that committing a navigation ends
// the dropdown: a suggestion list produced for the typed query must not
// survive into the next page.
func TestNavigateClearsStaleSuggestions(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "golang"
	s.OnQuerySuggest = fakeSuggest
	s.FocusURL()
	if !s.HasSuggestions() {
		t.Fatal("setup: focusing the bar should surface suggestions")
	}
	s.Navigate("https://www.google.com/search?q=golang")
	if s.HasSuggestions() {
		t.Error("Navigate must clear the suggestions dropdown")
	}
}

// TestSyncFromTabClearsStaleSuggestions pins that a tab switch (which syncs
// the new tab's state into the shared toolbar) drops the previous tab's
// suggestions, so a fast switch never shows a stale dropdown.
func TestSyncFromTabClearsStaleSuggestions(t *testing.T) {
	s := toolbar.NewState(800, nil)
	s.URL = "golang"
	s.OnQuerySuggest = fakeSuggest
	s.FocusURL()
	if !s.HasSuggestions() {
		t.Fatal("setup: focusing the bar should surface suggestions")
	}
	s.SyncFromTab("https://example.com/", false, "", toolbar.NewHistory())
	if s.HasSuggestions() {
		t.Error("SyncFromTab must clear the suggestions dropdown")
	}
}

package ui

import (
	"testing"
)

func TestNewBrowserState(t *testing.T) {
	state := NewBrowserState()
	if state == nil {
		t.Fatal("NewBrowserState() returned nil")
	}
	if state.currentIndex != -1 {
		t.Errorf("NewBrowserState() currentIndex = %d, want -1", state.currentIndex)
	}
	if len(state.history) != 0 {
		t.Errorf("NewBrowserState() history length = %d, want 0", len(state.history))
	}
}

func TestAddToHistory(t *testing.T) {
	state := NewBrowserState()

	state.AddToHistory("https://example.com")
	if len(state.history) != 1 {
		t.Errorf("After adding one URL, history length = %d, want 1", len(state.history))
	}
	if state.currentIndex != 0 {
		t.Errorf("After adding one URL, currentIndex = %d, want 0", state.currentIndex)
	}
	if state.GetCurrentURL() != "https://example.com" {
		t.Errorf("GetCurrentURL() = %s, want https://example.com", state.GetCurrentURL())
	}

	state.AddToHistory("https://example.org")
	if len(state.history) != 2 {
		t.Errorf("After adding two URLs, history length = %d, want 2", len(state.history))
	}
	if state.currentIndex != 1 {
		t.Errorf("After adding two URLs, currentIndex = %d, want 1", state.currentIndex)
	}
}

func TestNavigationBackForward(t *testing.T) {
	state := NewBrowserState()

	// Initially should not be able to go back or forward
	if state.CanGoBack() {
		t.Error("CanGoBack() = true, want false on empty history")
	}
	if state.CanGoForward() {
		t.Error("CanGoForward() = true, want false on empty history")
	}

	// Add URLs
	state.AddToHistory("https://example.com")
	state.AddToHistory("https://example.org")
	state.AddToHistory("https://example.net")

	// Should be able to go back but not forward
	if !state.CanGoBack() {
		t.Error("CanGoBack() = false, want true after adding URLs")
	}
	if state.CanGoForward() {
		t.Error("CanGoForward() = true, want false at end of history")
	}

	// Go back
	url, ok := state.GoBack()
	if !ok {
		t.Error("GoBack() returned false, want true")
	}
	if url != "https://example.org" {
		t.Errorf("GoBack() returned %s, want https://example.org", url)
	}

	// Now should be able to go both back and forward
	if !state.CanGoBack() {
		t.Error("CanGoBack() = false, want true in middle of history")
	}
	if !state.CanGoForward() {
		t.Error("CanGoForward() = false, want true in middle of history")
	}

	// Go forward
	url, ok = state.GoForward()
	if !ok {
		t.Error("GoForward() returned false, want true")
	}
	if url != "https://example.net" {
		t.Errorf("GoForward() returned %s, want https://example.net", url)
	}
}

func TestHistoryBranching(t *testing.T) {
	state := NewBrowserState()

	state.AddToHistory("https://example.com")
	state.AddToHistory("https://example.org")
	state.AddToHistory("https://example.net")

	// Go back twice
	state.GoBack()
	state.GoBack()

	// Add new URL - should remove forward history
	state.AddToHistory("https://newsite.com")

	// Should not be able to go forward
	if state.CanGoForward() {
		t.Error("CanGoForward() = true, want false after branching")
	}

	// History should be [example.com, newsite.com]
	history := state.GetHistory()
	if len(history) != 2 {
		t.Errorf("History length = %d, want 2 after branching", len(history))
	}
	if history[1] != "https://newsite.com" {
		t.Errorf("History[1] = %s, want https://newsite.com", history[1])
	}
}

func TestBookmarks(t *testing.T) {
	state := NewBrowserState()

	// Initially no bookmarks
	if len(state.GetBookmarks()) != 0 {
		t.Errorf("Initial bookmarks length = %d, want 0", len(state.GetBookmarks()))
	}

	// Add bookmark
	state.AddBookmark("https://example.com")
	if len(state.GetBookmarks()) != 1 {
		t.Errorf("After adding bookmark, length = %d, want 1", len(state.GetBookmarks()))
	}
	if !state.IsBookmarked("https://example.com") {
		t.Error("IsBookmarked() = false, want true for added bookmark")
	}

	// Try adding duplicate - should not add
	state.AddBookmark("https://example.com")
	if len(state.GetBookmarks()) != 1 {
		t.Errorf("After adding duplicate bookmark, length = %d, want 1", len(state.GetBookmarks()))
	}

	// Add another bookmark
	state.AddBookmark("https://example.org")
	if len(state.GetBookmarks()) != 2 {
		t.Errorf("After adding second bookmark, length = %d, want 2", len(state.GetBookmarks()))
	}

	// Remove bookmark
	state.RemoveBookmark("https://example.com")
	if len(state.GetBookmarks()) != 1 {
		t.Errorf("After removing bookmark, length = %d, want 1", len(state.GetBookmarks()))
	}
	if state.IsBookmarked("https://example.com") {
		t.Error("IsBookmarked() = true, want false for removed bookmark")
	}
	if !state.IsBookmarked("https://example.org") {
		t.Error("IsBookmarked() = false, want true for remaining bookmark")
	}
}

func TestBookmarkRemoveNonExistent(t *testing.T) {
	state := NewBrowserState()

	state.AddBookmark("https://example.com")

	// Try removing non-existent bookmark - should not panic
	state.RemoveBookmark("https://nonexistent.com")

	if len(state.GetBookmarks()) != 1 {
		t.Errorf("After removing non-existent bookmark, length = %d, want 1", len(state.GetBookmarks()))
	}
}

func TestGetCurrentURLEmptyHistory(t *testing.T) {
	state := NewBrowserState()

	url := state.GetCurrentURL()
	if url != "" {
		t.Errorf("GetCurrentURL() on empty history = %s, want empty string", url)
	}
}

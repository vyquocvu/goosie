package e2e

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestBrowserNavigationWorkflow(t *testing.T) {
	// Create a test app to satisfy any Fyne dependencies if needed
	testApp := test.NewApp()
	defer testApp.Quit()

	// Initialize Browser State (Model)
	state := ui.NewBrowserState()

	// 1. Initial State
	assert.Equal(t, "", state.GetCurrentURL())
	assert.False(t, state.CanGoBack())
	assert.False(t, state.CanGoForward())

	// 2. Navigate to Home
	homeURL := "https://home.com"
	state.AddToHistory(homeURL)
	assert.Equal(t, homeURL, state.GetCurrentURL())
	assert.False(t, state.CanGoBack()) // First page, nowhere to go back

	// 3. Navigate to Page 1
	page1URL := "https://page1.com"
	state.AddToHistory(page1URL)
	assert.Equal(t, page1URL, state.GetCurrentURL())
	assert.True(t, state.CanGoBack())
	assert.False(t, state.CanGoForward())

	// 4. Go Back to Home
	url, ok := state.GoBack()
	assert.True(t, ok)
	assert.Equal(t, homeURL, url)
	assert.Equal(t, homeURL, state.GetCurrentURL())
	assert.False(t, state.CanGoBack())
	assert.True(t, state.CanGoForward())

	// 5. Go Forward to Page 1
	url, ok = state.GoForward()
	assert.True(t, ok)
	assert.Equal(t, page1URL, url)
	assert.Equal(t, page1URL, state.GetCurrentURL())

	// 6. Navigate to Page 2 (should clear forward history if any, but we are at the end)
	// Let's go back first to demonstrate clearing forward history
	state.GoBack() // At Home
	page2URL := "https://page2.com"
	state.AddToHistory(page2URL) // At Page 2

	assert.Equal(t, page2URL, state.GetCurrentURL())
	assert.True(t, state.CanGoBack())     // Can go back to Home
	assert.False(t, state.CanGoForward()) // Forward history (Page 1) should be cleared

	// Verify we can't go forward to Page 1
	_, ok = state.GoForward()
	assert.False(t, ok)

	// Verify back goes to Home
	url, ok = state.GoBack()
	assert.Equal(t, homeURL, url)
}

func TestBookmarkWorkflow(t *testing.T) {
	state := ui.NewBrowserState()
	url := "https://example.com"

	// 1. Add Bookmark
	state.AddBookmark(url)
	assert.True(t, state.IsBookmarked(url))

	// 2. Verify List
	bookmarks := state.GetBookmarks()
	assert.Contains(t, bookmarks, url)
	assert.Equal(t, 1, len(bookmarks))

	// 3. Remove Bookmark
	state.RemoveBookmark(url)
	assert.False(t, state.IsBookmarked(url))
	assert.Empty(t, state.GetBookmarks())
}

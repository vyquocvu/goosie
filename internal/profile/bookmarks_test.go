package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBookmarkStoreAddRemoveAndPersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewBookmarkStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Add("https://example.com", "Example"))
	require.NoError(t, store.Add("https://example.com", "Example Updated"))

	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	require.Equal(t, "Example Updated", bookmarks[0].Title)

	reloaded, err := NewBookmarkStore(p)
	require.NoError(t, err)
	require.True(t, reloaded.Contains("https://example.com"))

	require.NoError(t, reloaded.Remove("https://example.com"))
	require.False(t, reloaded.Contains("https://example.com"))
}

func TestBookmarkStoreLoadsSnakeCaseTimestampSchema(t *testing.T) {
	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "bookmarks.json"), []byte(`[
  {
    "url": "https://example.com",
    "title": "Example",
    "created_at": "2026-06-19T01:02:03Z",
    "updated_at": "2026-06-19T04:05:06Z"
  }
]`), 0o600)
	require.NoError(t, err)

	p, err := Open(Options{Root: root})
	require.NoError(t, err)

	store, err := NewBookmarkStore(p)
	require.NoError(t, err)
	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	require.Equal(t, "https://example.com", bookmarks[0].URL)
	require.Equal(t, "Example", bookmarks[0].Title)
	require.Equal(t, time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC), bookmarks[0].CreatedAt)
	require.Equal(t, time.Date(2026, 6, 19, 4, 5, 6, 0, time.UTC), bookmarks[0].UpdatedAt)
}

func TestBookmarkStoreListReturnsCopy(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewBookmarkStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Add("https://example.com", "Example"))

	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	bookmarks[0].Title = "Changed"

	require.Equal(t, "Example", store.List()[0].Title)
}

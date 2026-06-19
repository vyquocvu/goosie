package profile

import (
	"testing"

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

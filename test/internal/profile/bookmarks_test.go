package profile_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	prof "github.com/vyquocvu/goosie/internal/profile"
)

func TestBookmarkStoreAddRemoveAndPersist(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Add("https://example.com", "Example"))
	require.NoError(t, store.Add("https://example.com", "Example Updated"))

	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	require.Equal(t, "Example Updated", bookmarks[0].Title)

	reloaded, err := prof.NewBookmarkStore(p)
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

	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	require.Equal(t, "https://example.com", bookmarks[0].URL)
	require.Equal(t, "Example", bookmarks[0].Title)
	require.Equal(t, time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC), bookmarks[0].CreatedAt)
	require.Equal(t, time.Date(2026, 6, 19, 4, 5, 6, 0, time.UTC), bookmarks[0].UpdatedAt)
}

func TestBookmarkStoreListReturnsCopy(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Add("https://example.com", "Example"))

	bookmarks := store.List()
	require.Len(t, bookmarks, 1)
	bookmarks[0].Title = "Changed"

	require.Equal(t, "Example", store.List()[0].Title)
}

func TestBookmarkStoreConcurrentInstancesMergeAdds(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	first, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	second, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)

	require.NoError(t, first.Add("https://one.test", "One"))
	require.NoError(t, second.Add("https://two.test", "Two"))

	reloaded, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	require.True(t, reloaded.Contains("https://one.test"))
	require.True(t, reloaded.Contains("https://two.test"))
}

func TestBookmarkStoreConcurrentInstanceRemoveReloadsBeforePersist(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	first, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	second, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)

	require.NoError(t, first.Add("https://one.test", "One"))
	require.NoError(t, second.Remove("https://one.test"))

	reloaded, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	require.False(t, reloaded.Contains("https://one.test"))
}

func TestBookmarkStoreConcurrentIndependentInstancesMergeAdds(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	const count = 20
	stores := make([]*prof.BookmarkStore, count)
	for i := range stores {
		stores[i], err = prof.NewBookmarkStore(p)
		require.NoError(t, err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i, store := range stores {
		wg.Add(1)
		go func(i int, store *prof.BookmarkStore) {
			defer wg.Done()
			<-start
			errs <- store.Add(fmt.Sprintf("https://%02d.test", i), fmt.Sprintf("Title %02d", i))
		}(i, store)
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	reloaded, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	for i := 0; i < count; i++ {
		require.True(t, reloaded.Contains(fmt.Sprintf("https://%02d.test", i)))
	}
}

func TestBookmarkStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write bookmarks as a normal profile.
	normal, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	normalStore, err := prof.NewBookmarkStore(normal)
	require.NoError(t, err)
	require.NoError(t, normalStore.Add("https://persisted.example.com", "Persisted"))
	require.NoError(t, normal.Close())

	// Open a private profile in the same root — should not read persisted data.
	priv, err := prof.Open(prof.Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := prof.NewBookmarkStore(priv)
	require.NoError(t, err)
	require.Empty(t, privStore.List(),
		"private profile must not read bookmarks from disk")

	// Mutate and verify nothing is written to disk.
	require.NoError(t, privStore.Add("https://private.example.com", "Private"))
	require.NoError(t, priv.Close())

	// Re-open as normal — should still have only the original bookmark.
	normal2, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer normal2.Close()
	normal2Store, err := prof.NewBookmarkStore(normal2)
	require.NoError(t, err)
	require.True(t, normal2Store.Contains("https://persisted.example.com"))
	require.False(t, normal2Store.Contains("https://private.example.com"),
		"private writes must not persist to disk")
}

package profile_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	prof "github.com/vyquocvu/goosie/internal/profile"
)

func TestHistoryStoreVisits(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewHistoryStore(p)
	require.NoError(t, err)
	require.NoError(t, store.AddVisit("https://one.test", "One"))
	require.NoError(t, store.AddVisit("https://two.test", "Two"))

	reloaded, err := prof.NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, reloaded.VisitURLs())
}

func TestHistoryStoreLoadsSnakeCaseTimestampSchema(t *testing.T) {
	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "history.json"), []byte(`{
  "visits": [
    {
      "url": "https://one.test",
      "title": "One",
      "visited_at": "2026-06-19T07:08:09Z"
    }
  ]
}`), 0o600)
	require.NoError(t, err)

	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test"}, store.VisitURLs())
	require.Equal(t, time.Date(2026, 6, 19, 7, 8, 9, 0, time.UTC), store.Doc.Visits[0].VisitedAt)
}

func TestHistoryStoreConcurrentInstancesMergeVisits(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	first, err := prof.NewHistoryStore(p)
	require.NoError(t, err)
	second, err := prof.NewHistoryStore(p)
	require.NoError(t, err)

	require.NoError(t, first.AddVisit("https://one.test", "One"))
	require.NoError(t, second.AddVisit("https://two.test", "Two"))

	reloaded, err := prof.NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, reloaded.VisitURLs())
}

func TestHistoryStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write history as a normal profile.
	normal, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	normalStore, err := prof.NewHistoryStore(normal)
	require.NoError(t, err)
	require.NoError(t, normalStore.AddVisit("https://persisted.example.com", "Persisted"))
	require.NoError(t, normal.Close())

	// Open a private profile in the same root — should not read persisted data.
	priv, err := prof.Open(prof.Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := prof.NewHistoryStore(priv)
	require.NoError(t, err)
	require.Empty(t, privStore.VisitURLs(),
		"private profile must not read history from disk")

	// Mutate and verify nothing is written to disk.
	require.NoError(t, privStore.AddVisit("https://private.example.com", "Private"))
	require.NoError(t, priv.Close())

	// Re-open as normal — should still have only the original visit.
	normal2, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer normal2.Close()
	normal2Store, err := prof.NewHistoryStore(normal2)
	require.NoError(t, err)
	require.Equal(t, []string{"https://persisted.example.com"}, normal2Store.VisitURLs(),
		"private writes must not persist to disk")
}

func TestHistoryStoreBatchWrites(t *testing.T) {
	root := t.TempDir()
	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewHistoryStore(p)
	require.NoError(t, err)

	initialWriteCount := p.WriteCount()

	// Write 10 visits rapidly
	for i := 0; i < 10; i++ {
		require.NoError(t, store.AddVisit(fmt.Sprintf("https://example.com/%d", i), "Test"))
	}

	// Trigger sync to force the background writer to flush the debounced writes to disk
	require.NoError(t, p.Sync())

	// The actual write count increase should be exactly 1, because all 10 visits
	// were coalesced into a single write!
	finalWriteCount := p.WriteCount()
	require.Equal(t, initialWriteCount+1, finalWriteCount, "rapid history writes must be batched/coalesced into a single disk write")

	// Verify we can read all of them back
	reloaded, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer reloaded.Close()
	reloadedStore, err := prof.NewHistoryStore(reloaded)
	require.NoError(t, err)
	require.Len(t, reloadedStore.VisitURLs(), 10)
}

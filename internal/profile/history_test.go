package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHistoryStoreVisits(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.NoError(t, store.AddVisit("https://one.test", "One"))
	require.NoError(t, store.AddVisit("https://two.test", "Two"))

	reloaded, err := NewHistoryStore(p)
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

	p, err := Open(Options{Root: root})
	require.NoError(t, err)

	store, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test"}, store.VisitURLs())
	require.Equal(t, time.Date(2026, 6, 19, 7, 8, 9, 0, time.UTC), store.doc.Visits[0].VisitedAt)
}

func TestHistoryStoreConcurrentInstancesMergeVisits(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	first, err := NewHistoryStore(p)
	require.NoError(t, err)
	second, err := NewHistoryStore(p)
	require.NoError(t, err)

	require.NoError(t, first.AddVisit("https://one.test", "One"))
	require.NoError(t, second.AddVisit("https://two.test", "Two"))

	reloaded, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, reloaded.VisitURLs())
}

func TestHistoryStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write history as a normal profile.
	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	normalStore, err := NewHistoryStore(normal)
	require.NoError(t, err)
	require.NoError(t, normalStore.AddVisit("https://persisted.example.com", "Persisted"))

	// Open a private profile in the same root — should not read persisted data.
	priv, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := NewHistoryStore(priv)
	require.NoError(t, err)
	require.Empty(t, privStore.VisitURLs(),
		"private profile must not read history from disk")

	// Mutate and verify nothing is written to disk.
	require.NoError(t, privStore.AddVisit("https://private.example.com", "Private"))

	// Re-open as normal — should still have only the original visit.
	normal2, err := Open(Options{Root: root})
	require.NoError(t, err)
	normal2Store, err := NewHistoryStore(normal2)
	require.NoError(t, err)
	require.Equal(t, []string{"https://persisted.example.com"}, normal2Store.VisitURLs(),
		"private writes must not persist to disk")
}

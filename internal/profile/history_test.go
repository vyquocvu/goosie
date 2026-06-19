package profile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHistoryStoreVisitsAndSessionTabs(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.NoError(t, store.AddVisit("https://one.test", "One"))
	require.NoError(t, store.AddVisit("https://two.test", "Two"))
	require.NoError(t, store.SaveSession([]SessionTab{
		{URL: "https://two.test", Title: "Two", Active: true},
	}))

	reloaded, err := NewHistoryStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"https://one.test", "https://two.test"}, reloaded.VisitURLs())
	require.Equal(t, []SessionTab{{URL: "https://two.test", Title: "Two", Active: true}}, reloaded.SessionTabs())
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
  ],
  "session": [
    {
      "url": "https://one.test",
      "title": "One",
      "active": true
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
	require.Equal(t, []SessionTab{{URL: "https://one.test", Title: "One", Active: true}}, store.SessionTabs())
}

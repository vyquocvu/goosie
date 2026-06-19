package profile

import (
	"testing"

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

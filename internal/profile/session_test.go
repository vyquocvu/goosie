package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionStoreSessionTabsAreSavedAndReloaded(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewSessionStore(p)
	require.NoError(t, err)
	require.NoError(t, store.SaveSession([]SessionTab{
		{URL: "https://two.test", Title: "Two", Active: true},
	}))

	reloaded, err := NewSessionStore(p)
	require.NoError(t, err)
	require.Equal(t, []SessionTab{{URL: "https://two.test", Title: "Two", Active: true}}, reloaded.SessionTabs())
}

func TestSessionStoreLoadsJSONSchema(t *testing.T) {
	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "session.json"), []byte(`{
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

	store, err := NewSessionStore(p)
	require.NoError(t, err)
	require.Equal(t, []SessionTab{{URL: "https://one.test", Title: "One", Active: true}}, store.SessionTabs())
}

func TestSessionStoreSessionTabsAreCopied(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewSessionStore(p)
	require.NoError(t, err)

	tabs := []SessionTab{
		{URL: "https://one.test", Title: "One", Active: true},
	}
	require.NoError(t, store.SaveSession(tabs))
	tabs[0].Title = "Changed"
	tabs[0].Active = false

	require.Equal(t, []SessionTab{{URL: "https://one.test", Title: "One", Active: true}}, store.SessionTabs())

	saved := store.SessionTabs()
	saved[0].Title = "Changed Again"
	saved[0].Active = false

	require.Equal(t, []SessionTab{{URL: "https://one.test", Title: "One", Active: true}}, store.SessionTabs())
}

func TestSessionStoreConcurrentInstanceSaveSessionWorks(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	first, err := NewSessionStore(p)
	require.NoError(t, err)
	second, err := NewSessionStore(p)
	require.NoError(t, err)

	require.NoError(t, first.SaveSession([]SessionTab{
		{URL: "https://one.test", Title: "One", Active: true},
	}))
	require.NoError(t, second.SaveSession([]SessionTab{
		{URL: "https://two.test", Title: "Two", Active: true},
	}))

	reloaded, err := NewSessionStore(p)
	require.NoError(t, err)
	// Last write wins for session
	require.Equal(t, []SessionTab{{URL: "https://two.test", Title: "Two", Active: true}}, reloaded.SessionTabs())
}

func TestSessionStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write session as a normal profile.
	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	normalStore, err := NewSessionStore(normal)
	require.NoError(t, err)
	require.NoError(t, normalStore.SaveSession([]SessionTab{
		{URL: "https://persisted.example.com", Title: "Persisted", Active: true},
	}))

	// Open a private profile in the same root — should not read persisted data.
	priv, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := NewSessionStore(priv)
	require.NoError(t, err)
	require.Empty(t, privStore.SessionTabs(),
		"private profile must not read session from disk")

	// Mutate and verify nothing is written to disk.
	require.NoError(t, privStore.SaveSession([]SessionTab{
		{URL: "https://private.example.com", Title: "Private", Active: true},
	}))

	// Re-open as normal — should still have only the original session.
	normal2, err := Open(Options{Root: root})
	require.NoError(t, err)
	normal2Store, err := NewSessionStore(normal2)
	require.NoError(t, err)
	require.Equal(t, []SessionTab{{URL: "https://persisted.example.com", Title: "Persisted", Active: true}}, normal2Store.SessionTabs(),
		"private writes must not persist to disk")
}

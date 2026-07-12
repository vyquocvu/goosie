package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionStoreVisitsAndTabs(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := NewSessionStore(p)
	require.NoError(t, err)
	require.NoError(t, store.SaveSession([]SessionTab{
		{URL: "https://one.test", Title: "One", Active: false},
		{URL: "https://two.test", Title: "Two", Active: true},
	}))

	reloaded, err := NewSessionStore(p)
	require.NoError(t, err)
	require.Equal(t, []SessionTab{
		{URL: "https://one.test", Title: "One", Active: false},
		{URL: "https://two.test", Title: "Two", Active: true},
	}, reloaded.SessionTabs())
}

func TestSessionStoreTabsAreCopied(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

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
	require.NoError(t, normal.Close())

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
	require.NoError(t, priv.Close())

	// Re-open as normal — should still have only the original session.
	normal2, err := Open(Options{Root: root})
	require.NoError(t, err)
	defer normal2.Close()
	normal2Store, err := NewSessionStore(normal2)
	require.NoError(t, err)
	require.Equal(t, []SessionTab{
		{URL: "https://persisted.example.com", Title: "Persisted", Active: true},
	}, normal2Store.SessionTabs(),
		"private writes must not persist to disk")
}

func TestSessionStoreConcurrentInstancesMergeSets(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

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
	require.Equal(t, []SessionTab{
		{URL: "https://two.test", Title: "Two", Active: true},
	}, reloaded.SessionTabs())
}

func BenchmarkSessionStoreSaveSession(b *testing.B) {
	p, err := Open(Options{Root: b.TempDir()})
	if err != nil {
		b.Fatalf("failed to open profile: %v", err)
	}
	defer p.Close()
	store, err := NewSessionStore(p)
	if err != nil {
		b.Fatalf("failed to create session store: %v", err)
	}

	tabs := []SessionTab{
		{URL: "https://one.test", Title: "One", Active: false},
		{URL: "https://two.test", Title: "Two", Active: true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.SaveSession(tabs); err != nil {
			b.Fatalf("failed to save session: %v", err)
		}
	}
}

func BenchmarkSessionStoreSessionTabs(b *testing.B) {
	p, err := Open(Options{Root: b.TempDir()})
	if err != nil {
		b.Fatalf("failed to open profile: %v", err)
	}
	defer p.Close()
	store, err := NewSessionStore(p)
	if err != nil {
		b.Fatalf("failed to create session store: %v", err)
	}

	tabs := []SessionTab{
		{URL: "https://one.test", Title: "One", Active: false},
		{URL: "https://two.test", Title: "Two", Active: true},
	}
	if err := store.SaveSession(tabs); err != nil {
		b.Fatalf("failed to save session: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.SessionTabs()
	}
}

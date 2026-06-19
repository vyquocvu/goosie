package profile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorageStoreIsOriginScopedAndPersistent(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "theme", "dark"))
	require.NoError(t, store.Set("https://two.test", "theme", "light"))

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	one, ok := reloaded.Get("https://one.test", "theme")
	require.True(t, ok)
	require.Equal(t, "dark", one)
	two, ok := reloaded.Get("https://two.test", "theme")
	require.True(t, ok)
	require.Equal(t, "light", two)
	require.Equal(t, []string{"theme"}, reloaded.Keys("https://one.test"))
}

func TestStorageStorePrivateDoesNotPersist(t *testing.T) {
	root := t.TempDir()
	p, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "token", "secret"))

	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	reloaded, err := NewStorageStore(normal)
	require.NoError(t, err)
	_, ok := reloaded.Get("https://one.test", "token")
	require.False(t, ok)
}

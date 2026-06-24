package profile

import (
	"fmt"
	"sync"
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

func TestStorageStoreRemoveClearKeysAndSnapshot(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "zeta", "last"))
	require.NoError(t, store.Set("https://one.test", "alpha", "first"))
	require.NoError(t, store.Set("https://one.test", "middle", "center"))
	require.NoError(t, store.Set("https://two.test", "theme", "light"))

	require.Equal(t, []string{"alpha", "middle", "zeta"}, store.Keys("https://one.test"))
	require.Equal(t, map[string]map[string]string{
		"https://one.test": {
			"alpha":  "first",
			"middle": "center",
			"zeta":   "last",
		},
		"https://two.test": {
			"theme": "light",
		},
	}, store.Snapshot())

	snapshot := store.Snapshot()
	snapshot["https://one.test"]["alpha"] = "changed"
	snapshot["https://one.test"]["new"] = "value"
	snapshot["https://new.test"] = map[string]string{"token": "secret"}

	value, ok := store.Get("https://one.test", "alpha")
	require.True(t, ok)
	require.Equal(t, "first", value)
	_, ok = store.Get("https://one.test", "new")
	require.False(t, ok)
	require.Empty(t, store.Keys("https://new.test"))

	require.NoError(t, store.Remove("https://one.test", "middle"))
	_, ok = store.Get("https://one.test", "middle")
	require.False(t, ok)
	require.Equal(t, []string{"alpha", "zeta"}, store.Keys("https://one.test"))

	require.NoError(t, store.Clear("https://one.test"))
	require.Empty(t, store.Keys("https://one.test"))
	_, ok = store.Get("https://two.test", "theme")
	require.True(t, ok)

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	require.Empty(t, reloaded.Keys("https://one.test"))
	require.Equal(t, []string{"theme"}, reloaded.Keys("https://two.test"))
}

func TestStorageStoreConcurrentInstancesMergeSets(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	first, err := NewStorageStore(p)
	require.NoError(t, err)
	second, err := NewStorageStore(p)
	require.NoError(t, err)

	require.NoError(t, first.Set("https://one.test", "theme", "dark"))
	require.NoError(t, second.Set("https://one.test", "token", "secret"))

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	require.Equal(t, []string{"theme", "token"}, reloaded.Keys("https://one.test"))
	theme, ok := reloaded.Get("https://one.test", "theme")
	require.True(t, ok)
	require.Equal(t, "dark", theme)
	token, ok := reloaded.Get("https://one.test", "token")
	require.True(t, ok)
	require.Equal(t, "secret", token)
}

func TestStorageStoreConcurrentInstanceRemoveReloadsBeforePersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	first, err := NewStorageStore(p)
	require.NoError(t, err)
	second, err := NewStorageStore(p)
	require.NoError(t, err)

	require.NoError(t, first.Set("https://one.test", "theme", "dark"))
	require.NoError(t, second.Remove("https://one.test", "theme"))

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	require.Empty(t, reloaded.Keys("https://one.test"))
}

func TestStorageStoreConcurrentInstanceClearReloadsBeforePersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	first, err := NewStorageStore(p)
	require.NoError(t, err)
	second, err := NewStorageStore(p)
	require.NoError(t, err)

	require.NoError(t, first.Set("https://one.test", "theme", "dark"))
	require.NoError(t, first.Set("https://two.test", "theme", "light"))
	require.NoError(t, second.Clear("https://one.test"))

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	require.Empty(t, reloaded.Keys("https://one.test"))
	require.Equal(t, []string{"theme"}, reloaded.Keys("https://two.test"))
}

func TestStorageStoreConcurrentIndependentInstancesMergeSets(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)

	const count = 20
	stores := make([]*StorageStore, count)
	for i := range stores {
		stores[i], err = NewStorageStore(p)
		require.NoError(t, err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i, store := range stores {
		wg.Add(1)
		go func(i int, store *StorageStore) {
			defer wg.Done()
			<-start
			errs <- store.Set("https://one.test", fmt.Sprintf("key-%02d", i), fmt.Sprintf("value-%02d", i))
		}(i, store)
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	reloaded, err := NewStorageStore(p)
	require.NoError(t, err)
	require.Len(t, reloaded.Keys("https://one.test"), count)
	for i := 0; i < count; i++ {
		value, ok := reloaded.Get("https://one.test", fmt.Sprintf("key-%02d", i))
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("value-%02d", i), value)
	}
}

func TestStorageStorePrivateDoesNotPersist(t *testing.T) {
	root := t.TempDir()
	p, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)

	store, err := NewStorageStore(p)
	require.NoError(t, err)
	require.NoError(t, store.Set("https://one.test", "token", "secret"))
	token, ok := store.Get("https://one.test", "token")
	require.True(t, ok)
	require.Equal(t, "secret", token)

	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	reloaded, err := NewStorageStore(normal)
	require.NoError(t, err)
	_, ok = reloaded.Get("https://one.test", "token")
	require.False(t, ok)
}

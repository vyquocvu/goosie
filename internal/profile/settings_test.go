package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsStoreDefaultsAndPersist(t *testing.T) {
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://example.com", settings.Homepage)
	require.True(t, settings.EnableJavaScript)
	require.True(t, settings.EnableImages)

	settings.Homepage = "https://go.dev"
	settings.DefaultSearchEngine = "https://duckduckgo.com/?q="
	require.NoError(t, store.Set(settings))

	reloaded, err := NewSettingsStore(p)
	require.NoError(t, err)
	require.Equal(t, "https://go.dev", reloaded.Get().Homepage)
	require.Equal(t, "https://duckduckgo.com/?q=", reloaded.Get().DefaultSearchEngine)
}

func TestSettingsStoreLoadsSnakeCaseSchema(t *testing.T) {
	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{
  "homepage": "https://go.dev",
  "default_search_engine": "https://duckduckgo.com/?q=",
  "enable_javascript": false,
  "enable_images": false
}`), 0o600)
	require.NoError(t, err)

	p, err := Open(Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://go.dev", settings.Homepage)
	require.Equal(t, "https://duckduckgo.com/?q=", settings.DefaultSearchEngine)
	require.False(t, settings.EnableJavaScript)
	require.False(t, settings.EnableImages)
}

func TestSettingsStoreLoadsPartialFileOverDefaults(t *testing.T) {
	root := t.TempDir()
	err := os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{
  "homepage": "https://go.dev"
}`), 0o600)
	require.NoError(t, err)

	p, err := Open(Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://go.dev", settings.Homepage)
	require.Equal(t, DefaultSettings().DefaultSearchEngine, settings.DefaultSearchEngine)
	require.Equal(t, DefaultSettings().EnableJavaScript, settings.EnableJavaScript)
	require.Equal(t, DefaultSettings().EnableImages, settings.EnableImages)
}

func TestSettingsStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write settings as a normal profile.
	normal, err := Open(Options{Root: root})
	require.NoError(t, err)
	normalStore, err := NewSettingsStore(normal)
	require.NoError(t, err)
	settings := normalStore.Get()
	settings.Homepage = "https://persisted.example.com"
	require.NoError(t, normalStore.Set(settings))
	require.NoError(t, normal.Close())

	// Open a private profile in the same root — should not read persisted data.
	priv, err := Open(Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := NewSettingsStore(priv)
	require.NoError(t, err)
	got := privStore.Get()
	require.Equal(t, DefaultSettings().Homepage, got.Homepage,
		"private profile must not read settings from disk")

	// Mutate and verify nothing is written to disk.
	got.Homepage = "https://private.example.com"
	require.NoError(t, privStore.Set(got))
	require.NoError(t, priv.Close())

	// Re-open as normal — should still have the original persisted value.
	normal2, err := Open(Options{Root: root})
	require.NoError(t, err)
	defer normal2.Close()
	normal2Store, err := NewSettingsStore(normal2)
	require.NoError(t, err)
	require.Equal(t, "https://persisted.example.com", normal2Store.Get().Homepage,
		"private writes must not persist to disk")
}

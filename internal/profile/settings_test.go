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

	store, err := NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://go.dev", settings.Homepage)
	require.Equal(t, DefaultSettings().DefaultSearchEngine, settings.DefaultSearchEngine)
	require.Equal(t, DefaultSettings().EnableJavaScript, settings.EnableJavaScript)
	require.Equal(t, DefaultSettings().EnableImages, settings.EnableImages)
}

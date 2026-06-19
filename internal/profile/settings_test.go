package profile

import (
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

package profile_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	prof "github.com/vyquocvu/goosie/internal/profile"
)

func TestSettingsStoreDefaultsAndPersist(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://example.com", settings.Homepage)
	require.True(t, settings.EnableJavaScript)
	require.True(t, settings.EnableImages)

	settings.Homepage = "https://go.dev"
	settings.DefaultSearchEngine = "https://duckduckgo.com/?q="
	require.NoError(t, store.Set(settings))

	reloaded, err := prof.NewSettingsStore(p)
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

	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
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

	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)
	settings := store.Get()
	require.Equal(t, "https://go.dev", settings.Homepage)
	require.Equal(t, prof.DefaultSettings().DefaultSearchEngine, settings.DefaultSearchEngine)
	require.Equal(t, prof.DefaultSettings().EnableJavaScript, settings.EnableJavaScript)
	require.Equal(t, prof.DefaultSettings().EnableImages, settings.EnableImages)
}

func TestSettingsStorePrivateDoesNotReadOrWrite(t *testing.T) {
	root := t.TempDir()

	// Write settings as a normal profile.
	normal, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	normalStore, err := prof.NewSettingsStore(normal)
	require.NoError(t, err)
	settings := normalStore.Get()
	settings.Homepage = "https://persisted.example.com"
	require.NoError(t, normalStore.Set(settings))
	require.NoError(t, normal.Close())

	// Open a private profile in the same root — should not read persisted data.
	priv, err := prof.Open(prof.Options{Root: root, Private: true})
	require.NoError(t, err)
	privStore, err := prof.NewSettingsStore(priv)
	require.NoError(t, err)
	got := privStore.Get()
	require.Equal(t, prof.DefaultSettings().Homepage, got.Homepage,
		"private profile must not read settings from disk")

	// Mutate and verify nothing is written to disk.
	got.Homepage = "https://private.example.com"
	require.NoError(t, privStore.Set(got))
	require.NoError(t, priv.Close())

	// Re-open as normal — should still have the original persisted value.
	normal2, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer normal2.Close()
	normal2Store, err := prof.NewSettingsStore(normal2)
	require.NoError(t, err)
	require.Equal(t, "https://persisted.example.com", normal2Store.Get().Homepage,
		"private writes must not persist to disk")
}

func TestSettingsStoreExport(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)

	settings := store.Get()
	settings.Homepage = "https://export.example.com"
	settings.EnableJavaScript = false
	err = store.Set(settings)
	require.NoError(t, err)

	exportPath := filepath.Join(t.TempDir(), "exported_settings.json")
	err = store.Export(exportPath)
	require.NoError(t, err)
	require.FileExists(t, exportPath)

	// Read and verify the file content
	content, err := os.ReadFile(exportPath)
	require.NoError(t, err)

	var exported prof.Settings
	err = json.Unmarshal(content, &exported)
	require.NoError(t, err)
	require.Equal(t, "https://export.example.com", exported.Homepage)
	require.False(t, exported.EnableJavaScript)
}

func TestSettingsStoreImport(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)

	importPath := filepath.Join(t.TempDir(), "import_settings.json")
	importedJSON := `{
  "homepage": "https://import.example.com",
  "default_search_engine": "https://duckduckgo.com/?q=",
  "enable_javascript": false,
  "enable_images": true
}
`
	err = os.WriteFile(importPath, []byte(importedJSON), 0o600)
	require.NoError(t, err)

	err = store.Import(importPath)
	require.NoError(t, err)

	// Verify in-memory settings are updated
	settings := store.Get()
	require.Equal(t, "https://import.example.com", settings.Homepage)
	require.False(t, settings.EnableJavaScript)
	require.True(t, settings.EnableImages)

	// Verify persistence in the profile
	err = p.Sync()
	require.NoError(t, err)

	reloadedStore, err := prof.NewSettingsStore(p)
	require.NoError(t, err)
	reloadedSettings := reloadedStore.Get()
	require.Equal(t, "https://import.example.com", reloadedSettings.Homepage)
	require.False(t, reloadedSettings.EnableJavaScript)
}

func TestSettingsStoreImportExportErrors(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)

	// 1. Import non-existent path
	err = store.Import(filepath.Join(dir, "non-existent.json"))
	require.Error(t, err)

	// 2. Import malformed JSON
	malformedPath := filepath.Join(dir, "malformed.json")
	err = os.WriteFile(malformedPath, []byte(`{homepage: "invalid"`), 0o600)
	require.NoError(t, err)
	err = store.Import(malformedPath)
	require.Error(t, err)

	// 3. Export to an invalid target (e.g., using a directory path itself)
	err = os.Mkdir(filepath.Join(dir, "dir-target"), 0o700)
	require.NoError(t, err)
	err = store.Export(filepath.Join(dir, "dir-target"))
	require.Error(t, err)
}

func TestSettingsStoreImportPrivate(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir, Private: true})
	require.NoError(t, err)
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	require.NoError(t, err)

	importPath := filepath.Join(t.TempDir(), "import_settings.json")
	importedJSON := `{
  "homepage": "https://private-import.example.com",
  "default_search_engine": "https://duckduckgo.com/?q=",
  "enable_javascript": true,
  "enable_images": true
}
`
	err = os.WriteFile(importPath, []byte(importedJSON), 0o600)
	require.NoError(t, err)

	err = store.Import(importPath)
	require.NoError(t, err)

	// In-memory settings must be updated
	require.Equal(t, "https://private-import.example.com", store.Get().Homepage)

	// Verify nothing is written to the profile directory
	require.NoFileExists(t, filepath.Join(dir, "settings.json"))
}

func BenchmarkSettingsStoreExport(b *testing.B) {
	dir := b.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	if err != nil {
		b.Fatal(err)
	}

	exportPath := filepath.Join(dir, "exported_settings.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := store.Export(exportPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSettingsStoreImport(b *testing.B) {
	dir := b.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()

	store, err := prof.NewSettingsStore(p)
	if err != nil {
		b.Fatal(err)
	}

	importPath := filepath.Join(dir, "import_settings.json")
	settings := prof.DefaultSettings()
	data, err := json.Marshal(settings)
	if err != nil {
		b.Fatal(err)
	}
	err = os.WriteFile(importPath, data, 0o600)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := store.Import(importPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

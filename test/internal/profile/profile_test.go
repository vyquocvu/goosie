package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	prof "github.com/vyquocvu/goosie/internal/profile"
)

type sampleDocument struct {
	Name string `json:"name"`
}

func TestOpenCreatesNormalProfileDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")

	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()
	require.False(t, p.Private())
	require.Equal(t, dir, p.Root())
	require.DirExists(t, dir)
}

func TestOpenWithDefaultRootUsesUserConfigDir(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	configDir := filepath.Join(base, "config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)

	p, err := prof.Open(prof.Options{Private: true})
	require.NoError(t, err)
	defer p.Close()
	require.True(t, p.Private())
	require.Contains(t, p.Root(), "goosie")
	require.Contains(t, p.Root(), base)
	require.NoDirExists(t, p.Root())
}

func TestPrivateProfileDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()

	p, err := prof.Open(prof.Options{Root: dir, Private: true})
	require.NoError(t, err)
	defer p.Close()
	require.True(t, p.Private())

	err = p.SaveJSON("state.json", sampleDocument{Name: "secret"})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "state.json"))
}

func TestSaveJSONCreatesMissingRootAndWritesIndentedJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	p, err := prof.Open(prof.Options{Root: root})
	require.NoError(t, err)
	defer p.Close()
	err = os.RemoveAll(root)
	require.NoError(t, err)

	err = p.SaveJSON("state.json", sampleDocument{Name: "goosie"})
	require.NoError(t, err)

	err = p.Sync()
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(root, "state.json"))
	require.NoError(t, err)
	require.Equal(t, "{\n  \"name\": \"goosie\"\n}\n", string(content))
}

func TestSaveAndLoadJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	err = p.SaveJSON("state.json", sampleDocument{Name: "goosie"})
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "goosie", loaded.Name)
}

func TestLoadMissingJSONLeavesTargetUnchanged(t *testing.T) {
	p, err := prof.Open(prof.Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	loaded := sampleDocument{Name: "default"}
	err = p.LoadJSON("missing.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "default", loaded.Name)
}

func TestCorruptJSONIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	path := filepath.Join(dir, "state.json")
	err = os.WriteFile(path, []byte("{not-json"), 0o600)
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.Error(t, err)
	require.FileExists(t, path+".corrupt")
	require.NoFileExists(t, path)
}

func TestJSONWithTrailingGarbageIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	path := filepath.Join(dir, "state.json")
	err = os.WriteFile(path, []byte(`{"name":"ok"} nope`), 0o600)
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.Error(t, err)
	require.FileExists(t, path+".corrupt")
	require.NoFileExists(t, path)
}

func TestCorruptJSONBackupFailureIsReturned(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	path := filepath.Join(dir, "state.json")
	err = os.WriteFile(path, []byte("{not-json"), 0o600)
	require.NoError(t, err)
	err = os.Mkdir(path+".corrupt", 0o700)
	require.NoError(t, err)

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.Error(t, err)
	require.ErrorContains(t, err, "decode")
	require.ErrorContains(t, err, "back up corrupt JSON")
	require.FileExists(t, path)
}

func TestProfileAsyncWrites(t *testing.T) {
	dir := t.TempDir()
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	err = p.SaveJSON("state.json", sampleDocument{Name: "async"})
	require.NoError(t, err)

	// Since SaveJSON is asynchronous, calling Sync() guarantees that it is written.
	err = p.Sync()
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "state.json"))

	var loaded sampleDocument
	err = p.LoadJSON("state.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "async", loaded.Name)
}

func TestSchemaVersioningAndMigration0to1(t *testing.T) {
	dir := t.TempDir()

	// Write legacy bookmarks (Version 0) as a raw array of strings
	legacyPath := filepath.Join(dir, "bookmarks.json")
	legacyData := []byte(`[
  "https://example.com",
  "https://google.com"
]`)
	err := os.WriteFile(legacyPath, legacyData, 0o600)
	require.NoError(t, err)

	// Open the profile. This should trigger the migration from v0 to v1.
	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	// Verify schema.json was created with version 1
	var schema prof.SchemaConfig
	err = p.LoadJSON("schema.json", &schema)
	require.NoError(t, err)
	require.Equal(t, 1, schema.Version)

	// Load bookmarks using the BookmarkStore and verify they are correctly migrated
	store, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)

	bookmarks := store.List()
	require.Len(t, bookmarks, 2)
	require.Equal(t, "https://example.com", bookmarks[0].URL)
	require.Equal(t, "https://example.com", bookmarks[0].Title)
	require.False(t, bookmarks[0].CreatedAt.IsZero())
	require.Equal(t, "https://google.com", bookmarks[1].URL)
}

func TestCorruptionRecoveryStoreFlow(t *testing.T) {
	dir := t.TempDir()

	// Write corrupt bookmarks
	bookmarksPath := filepath.Join(dir, "bookmarks.json")
	err := os.WriteFile(bookmarksPath, []byte("{corrupt-json"), 0o600)
	require.NoError(t, err)

	p, err := prof.Open(prof.Options{Root: dir})
	require.NoError(t, err)
	defer p.Close()

	// Initial store creation should fail because the JSON is corrupt
	store, err := prof.NewBookmarkStore(p)
	require.Error(t, err)
	require.Nil(t, store)

	// The corrupt file should now be renamed to bookmarks.json.corrupt, and the original file deleted
	require.FileExists(t, bookmarksPath+".corrupt")
	require.NoFileExists(t, bookmarksPath)

	// A subsequent store creation should succeed, loading an empty store
	store2, err := prof.NewBookmarkStore(p)
	require.NoError(t, err)
	require.NotNil(t, store2)
	require.Empty(t, store2.List())

	// Add an item to make sure it works fine after recovery
	err = store2.Add("https://recovered.com", "Recovered")
	require.NoError(t, err)

	// Force sync and verify the file exists now
	err = p.Sync()
	require.NoError(t, err)
	require.FileExists(t, bookmarksPath)
}

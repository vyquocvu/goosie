package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type sampleDocument struct {
	Name string `json:"name"`
}

func TestOpenCreatesNormalProfileDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")

	p, err := Open(Options{Root: dir})
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

	p, err := Open(Options{Private: true})
	require.NoError(t, err)
	defer p.Close()
	require.True(t, p.Private())
	require.Contains(t, p.Root(), "goosie")
	require.Contains(t, p.Root(), base)
	require.NoDirExists(t, p.Root())
}

func TestPrivateProfileDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()

	p, err := Open(Options{Root: dir, Private: true})
	require.NoError(t, err)
	defer p.Close()
	require.True(t, p.Private())

	err = p.SaveJSON("state.json", sampleDocument{Name: "secret"})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "state.json"))
}

func TestSaveJSONCreatesMissingRootAndWritesIndentedJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "profile")
	p, err := Open(Options{Root: root})
	require.NoError(t, err)
	defer p.Close()
	err = os.Remove(root)
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
	p, err := Open(Options{Root: dir})
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
	p, err := Open(Options{Root: t.TempDir()})
	require.NoError(t, err)
	defer p.Close()

	loaded := sampleDocument{Name: "default"}
	err = p.LoadJSON("missing.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "default", loaded.Name)
}

func TestCorruptJSONIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(Options{Root: dir})
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
	p, err := Open(Options{Root: dir})
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
	p, err := Open(Options{Root: dir})
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
	p, err := Open(Options{Root: dir})
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

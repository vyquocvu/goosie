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
	dir := t.TempDir()

	p, err := Open(Options{Root: dir})
	require.NoError(t, err)
	require.False(t, p.Private())
	require.Equal(t, dir, p.Root())
	require.DirExists(t, dir)
}

func TestPrivateProfileDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()

	p, err := Open(Options{Root: dir, Private: true})
	require.NoError(t, err)
	require.True(t, p.Private())

	err = p.SaveJSON("state.json", sampleDocument{Name: "secret"})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "state.json"))
}

func TestSaveAndLoadJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(Options{Root: dir})
	require.NoError(t, err)

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

	loaded := sampleDocument{Name: "default"}
	err = p.LoadJSON("missing.json", &loaded)
	require.NoError(t, err)
	require.Equal(t, "default", loaded.Name)
}

func TestCorruptJSONIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	p, err := Open(Options{Root: dir})
	require.NoError(t, err)

	path := filepath.Join(dir, "state.json")
	err = os.WriteFile(path, []byte("{not-json"), 0o600)
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

package js_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func storageJSONFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			files = append(files, e.Name())
		}
	}
	return files
}

// qc:BR-058
func TestClearAllLocalStorageClearsMemoryAndDisk(t *testing.T) {
	storageDir := filepath.Join(t.TempDir(), "storage")
	reg := js.NewOriginStorageRegistry(storageDir, false)
	a := "https://a.example:443"
	b := "https://b.example:443"
	reg.SetLocalStorageItem(a, "token", "one")
	reg.SetLocalStorageItem(b, "theme", "dark")

	if err := os.WriteFile(filepath.Join(storageDir, "legacy.json"), []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	origins, files, err := reg.ClearAllLocalStorage()
	if err != nil {
		t.Fatalf("ClearAllLocalStorage: %v", err)
	}
	if origins != 2 {
		t.Errorf("origins cleared = %d, want 2", origins)
	}
	if files != 3 {
		t.Errorf("files removed = %d, want 3", files)
	}
	for _, origin := range []string{a, b} {
		if got := reg.DumpLocalStorage(origin); len(got) != 0 {
			t.Errorf("localStorage for %s after clear = %+v, want empty", origin, got)
		}
	}
	if remaining := storageJSONFiles(t, storageDir); len(remaining) != 0 {
		t.Errorf("storage files remain after clear: %v", remaining)
	}
	if got := js.NewOriginStorageRegistry(storageDir, false).DumpLocalStorage(a); len(got) != 0 {
		t.Errorf("a restarted profile still sees %+v", got)
	}

	reg.SetLocalStorageItem(a, "after", "clear")
	fresh := js.NewOriginStorageRegistry(storageDir, false).DumpLocalStorage(a)
	if len(fresh) != 1 || fresh[0].Key != "after" || fresh[0].Value != "clear" {
		t.Fatalf("write after clear = %+v, want the durable after=clear entry", fresh)
	}
}

// qc:BR-058
func TestClearAllLocalStorageMissingDirectoryIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	reg := js.NewOriginStorageRegistry(dir, false)
	for attempt := 0; attempt < 2; attempt++ {
		origins, files, err := reg.ClearAllLocalStorage()
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if origins != 0 || files != 0 {
			t.Fatalf("attempt %d cleared %d origins / %d files, want 0/0", attempt, origins, files)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("clear created %s: %v", dir, err)
	}
}

// qc:BR-058
func TestClearAllLocalStorageIncognitoLeavesProfileFiles(t *testing.T) {
	storageDir := filepath.Join(t.TempDir(), "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(storageDir, "profile.json")
	if err := os.WriteFile(decoy, []byte(`{"k":"keep"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := js.NewOriginStorageRegistry(storageDir, true)
	reg.SetLocalStorageItem("https://private.example:443", "k", "v")
	origins, files, err := reg.ClearAllLocalStorage()
	if err != nil {
		t.Fatalf("ClearAllLocalStorage: %v", err)
	}
	if origins != 1 {
		t.Errorf("origins cleared = %d, want 1", origins)
	}
	if files != 0 {
		t.Errorf("files removed = %d, want 0 for an incognito registry", files)
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Fatalf("incognito clear touched a profile file: %v", err)
	}
}

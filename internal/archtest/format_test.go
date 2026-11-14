package archtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

func TestRepoIsGofmtFormatted(t *testing.T) {
	root := moduleRoot(t)
	drift, err := archtest.UnformattedGoFiles(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, file := range relPaths(t, root, drift) {
		t.Errorf("%s: not gofmt-formatted", file)
	}
}

const (
	cleanGo = "package p\n\nfunc F(a int, b int) int {\n\treturn a + b\n}\n"
	dirtyGo = "package p\n\nfunc F(a int,b int) int {\n  return a + b\n}\n"
)

func writeGo(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func relPaths(t *testing.T, root string, files []string) []string {
	t.Helper()
	out := make([]string, len(files))
	for i, f := range files {
		rel, err := filepath.Rel(root, f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

func TestUnformattedGoFilesFlagsDriftedFiles(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "clean.go", cleanGo)
	writeGo(t, root, "drift.go", dirtyGo)
	writeGo(t, root, filepath.Join("pkg", "drift.go"), dirtyGo)

	got := relPaths(t, root, archtestMust(t, root))
	want := []string{"drift.go", "pkg/drift.go"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("drift = %v, want %v", got, want)
	}
}

func TestUnformattedGoFilesPrunesExcludedDirs(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "clean.go", cleanGo)
	for _, dir := range []string{".worktrees/stale", "node_modules/pkg", "testdata/fixture"} {
		path := writeGo(t, root, filepath.Join(dir, "drift.go"), dirtyGo)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	if got := archtestMust(t, root); len(got) != 0 {
		t.Errorf("excluded dirs reported as drift: %v", relPaths(t, root, got))
	}
}

func TestUnformattedGoFilesErrorsOnUnparseable(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "clean.go", cleanGo)
	broken := writeGo(t, root, "broken.go", "package p\n\nfunc F( {\n\treturn 1\n}\n")

	_, err := archtest.UnformattedGoFiles(root)
	if err == nil {
		t.Fatal("an unparseable file reported no error")
	}
	if !strings.Contains(err.Error(), filepath.Base(broken)) {
		t.Errorf("error does not name the file: %v", err)
	}
}

func archtestMust(t *testing.T, root string) []string {
	t.Helper()
	files, err := archtest.UnformattedGoFiles(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return files
}

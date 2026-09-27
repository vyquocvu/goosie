package archtest_test

// CONTRIBUTING.md tells every contributor to run `gofmt -w .` before committing, and that
// command cannot fail: it rewrites the files and exits 0 either way. A documented check
// with no way to report a miss is not a check, so skipping it was free, and 17 of this
// module's files are committed unformatted - one of them indented with spaces throughout.
//
// The rule lives in this package's non-test file like the others, so the gate suite can
// apply it too. What is here is the reporting, plus the cases that prove the rule can
// fail: a gate that reported nothing for any input would pass TestRepoIsGofmtFormatted
// and be worthless.

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

// The two bodies a formatter test needs: gofmt's own output for a function, and the same
// function written without it. The dirty one is a realistic miss rather than a
// contrived one - space indentation and no space after a comma are what an editor with
// tabs off produces.
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

// relPaths renders the rule's answer relative to the tree it walked, so an assertion
// reads as the directory layout rather than as a wall of temp-dir prefixes.
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

// The non-vacuity case: a tree with one clean file and two drifted ones has to report
// exactly the drifted two. Reporting nothing would pass the repo-wide check above, and
// reporting everything would be a gate nobody can satisfy.
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

// .worktrees/ holds a second full checkout of this module, so the rule that skipped it
// would be the only thing in CI that reads a stale copy as current - and the rule that
// walked it would fail on files nobody is editing. `gofmt -l .` at the repo root does
// report that checkout's 17 files, which is why this rule is not that command.
// testdata is what `go list ./...` never builds: a deliberately malformed fixture is
// input for a parser test, not drift.
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

// A file gofmt cannot parse is not "unformatted", and quietly skipping it is the same
// fail-open in a new costume: the tree would read clean while one file is not Go at all.
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

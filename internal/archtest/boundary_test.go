package archtest_test

// These are the architecture rules as they are enforced on every commit. The rules
// themselves - the import table, the cgo walk, the `go list` calls - live in this
// package's non-test file, because the M1 gate suite has to apply the same rules and
// "M1 CI green" is one command rather than two. What is left here is the reporting:
// turning a []Violation into test failures with the file names a reader needs.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// guiToolkits are the imports that would put somebody else's window on screen. The
// design is a pure Go browser with one thin cgo shim, so a GUI toolkit in the graph
// is not a dependency preference but a change of architecture.
var guiToolkits = []string{"fyne.io/"}

// moduleRoot is the directory `go list ./...` and a walk of v2 mean something in.
// The test binary's working directory is its own package directory, so this walks up
// from this file's path rather than relying on where the test was invoked from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the module root")
	}
	root, err := archtest.RepoRoot(here)
	if err != nil {
		t.Fatalf("%s: %v", here, err)
	}
	return root
}

func packages(t *testing.T) []archtest.Package {
	t.Helper()
	pkgs, err := archtest.List(moduleRoot(t), "./...")
	if err != nil {
		t.Fatalf("go list ./...: %v", err)
	}
	return pkgs
}

func TestNoFyneImports(t *testing.T) {
	for _, v := range archtest.CheckBannedImports(packages(t), guiToolkits...) {
		t.Errorf("%s imports %s: %s", v.Importer, v.Import, v.Why)
	}
}

func TestLayeredImports(t *testing.T) {
	for _, v := range archtest.CheckLayers(packages(t)) {
		t.Errorf("%v", v)
	}
}

// TestCgoConfinedToDarwinPlatform is the mechanical half of the "pure Go except for
// one thin shim" decision.
func TestCgoConfinedToDarwinPlatform(t *testing.T) {
	root := moduleRoot(t)
	darwinDir := filepath.Join(root, "internal", "platform", "darwin")
	found, err := archtest.CheckCgo(root, darwinDir)
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for _, path := range found {
		t.Errorf("%s uses cgo; only internal/platform/darwin may", path)
	}
}

// TestRulesAreNotVacuous fails if the table could pass without reading anything. A
// checker whose input is empty reports success, so the one assertion worth having is
// that the input is the subtree it claims to cover.
func TestRulesAreNotVacuous(t *testing.T) {
	pkgs := packages(t)
	if len(pkgs) < 8 {
		t.Fatalf("go listed %d v2 packages; the tree has more, so the pattern is wrong", len(pkgs))
	}
	for _, dir := range []string{"internal/frame", "internal/paint", "internal/surface", "internal/raster"} {
		if _, ok := archtest.Allowed[dir]; !ok {
			t.Errorf("the allowed-imports table has no entry for %s", dir)
		}
	}
	// The walk has to be able to see a file, or CheckCgo passes for every shim.
	if _, err := os.Stat(filepath.Join(moduleRoot(t), "internal", "frame", "grid.go")); err != nil {
		t.Fatalf("the cgo walk would find nothing: %v", err)
	}
}

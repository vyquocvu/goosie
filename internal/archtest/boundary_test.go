package archtest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

var guiToolkits = []string{"fyne.io/"}

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

func TestRulesAreNotVacuous(t *testing.T) {
	pkgs := packages(t)
	if len(pkgs) < 8 {
		t.Fatalf("go listed %d v2 packages; the tree has more, so the pattern is wrong", len(pkgs))
	}
	for _, dir := range []string{"internal/frame", "internal/paint", "internal/surface", "internal/raster", "internal/tabs"} {
		if _, ok := archtest.Allowed[dir]; !ok {
			t.Errorf("the allowed-imports table has no entry for %s", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(moduleRoot(t), "internal", "frame", "grid.go")); err != nil {
		t.Fatalf("the cgo walk would find nothing: %v", err)
	}
}

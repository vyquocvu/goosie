package archtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// modulePrefix is the import path every v2 package lives under. It is spelled
// out rather than derived so a module rename fails these tests loudly instead of
// silently turning them into no-ops.
const modulePrefix = "github.com/vyquocvu/goosie/v2/"

// allowed lists, for each v2 package, the v2 packages it may import. Anything
// absent from this table is a violation, and a package with no entry has no
// v2 imports allowed at all.
//
// This is the import graph from the M1 spec as data. Encoding it here rather
// than relying on review is the point: the architecture's claims ("frame is a
// dependency leaf", "surface knows nothing about platforms") are the ones a
// later contributor is most likely to break by accident while fixing something
// urgent.
var allowed = map[string][]string{
	"internal/frame":             nil,
	"internal/paint":             {"internal/frame"},
	"internal/surface":           {"internal/frame", "internal/paint"},
	"internal/raster":            {"internal/frame", "internal/paint", "internal/surface"},
	"internal/platform/headless": {"internal/frame", "internal/surface"},
	"internal/platform/darwin":   {"internal/frame", "internal/surface"},
	"internal/platform":          {"internal/frame", "internal/surface", "internal/platform/headless", "internal/platform/darwin"},
	"internal/archtest":          nil,
}

// repoRoot walks up from this file's directory to the module root, which is the
// only place where `go list ./v2/...` and a walk of `v2` mean what they say. The
// test binary's working directory is its own package directory, so without this
// every path here would be relative to the wrong place.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the module root")
	}
	dir := filepath.Dir(here)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", filepath.Dir(here))
		}
		dir = parent
	}
}

type pkg struct {
	path    string
	imports []string
}

// listPackages runs `go list` over ./v2/... from the module root. Each call costs
// about a second, so callers use it once per test.
func listPackages(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", "./v2/...")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./v2/... failed: %v", err)
	}
	var pkgs []pkg
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		path, imports, ok := strings.Cut(line, "|")
		if !ok {
			t.Fatalf("unexpected go list output %q", line)
		}
		pkgs = append(pkgs, pkg{path: path, imports: strings.Fields(imports)})
	}
	if len(pkgs) == 0 {
		t.Fatal("go list ./v2/... reported no packages; the pattern or module path is wrong")
	}
	return pkgs
}

func v2Rel(path string) string { return strings.TrimPrefix(path, modulePrefix) }

func TestNoFyneImports(t *testing.T) {
	for _, p := range listPackages(t) {
		for _, imp := range p.imports {
			if strings.HasPrefix(imp, "fyne.io/") {
				t.Errorf("%s imports %s: no v2 package may depend on a GUI toolkit", p.path, imp)
			}
		}
	}
}

func TestLayeredImports(t *testing.T) {
	for _, p := range listPackages(t) {
		rel := v2Rel(p.path)
		switch {
		case strings.HasPrefix(rel, "test/"), strings.HasPrefix(rel, "cmd/"):
			// Tests and binaries may reach anything under v2; that is how the gate
			// tests drive the whole frame path.
			continue
		case !strings.HasPrefix(rel, "internal/"):
			t.Errorf("%s is not under internal/, test/, or cmd/", p.path)
			continue
		}
		if _, ok := allowed[rel]; !ok {
			t.Errorf("package %s has no entry in the allowed-imports table; add it deliberately", rel)
			continue
		}
		ok := map[string]bool{}
		for _, a := range allowed[rel] {
			ok[modulePrefix+a] = true
		}
		for _, imp := range p.imports {
			if strings.HasPrefix(imp, modulePrefix) && !ok[imp] {
				t.Errorf("%s imports %s, which the M1 import graph forbids (allowed: %v)",
					rel, v2Rel(imp), allowed[rel])
			}
		}
	}
}

// TestCgoConfinedToDarwinPlatform is the mechanical half of the "pure Go except
// for one thin shim" decision. It reads files rather than the import graph
// because cgo can be introduced by a single //go:build cgo tag with no new
// import at all.
func TestCgoConfinedToDarwinPlatform(t *testing.T) {
	root := repoRoot(t)
	v2Dir := filepath.Join(root, "v2")
	darwinDir := filepath.Join(v2Dir, "internal", "platform", "darwin")
	var violations []string
	err := filepath.Walk(v2Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !usesCgo(path) {
			return nil
		}
		if filepath.Dir(path) != darwinDir {
			violations = append(violations, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", v2Dir, err)
	}
	for _, v := range violations {
		t.Errorf("%s uses cgo; only v2/internal/platform/darwin may", v)
	}
}

// usesCgo reports whether a file pulls in the C pseudo-package or builds only
// when cgo is enabled. Both count: the second is how a shim sneaks back in.
func usesCgo(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == `import "C"` || trimmed == `"C"` {
			return true
		}
		if strings.HasPrefix(trimmed, "//go:build") && strings.Contains(trimmed, "cgo") {
			return true
		}
	}
	return false
}

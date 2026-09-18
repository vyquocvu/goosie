package archtest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The rules are here rather than in boundary_test.go so that more than one test
// can enforce them. "M1 CI green" is one command, and that command is the gate
// suite, which has to check the import graph too - by calling these functions, not
// by copying the table. A duplicated rule table is a rule that gets updated in one
// place and forgotten in the other.
//
// Nothing outside a test imports this file's package, and no binary links it:
// `go test ./test/gate/` is the only thing that pays for the `go list` calls.

// ModulePrefix is the import path every v2 package lives under. It is spelled out
// rather than derived so a module rename fails these tests loudly instead of
// silently turning them into no-ops.
const ModulePrefix = "github.com/vyquocvu/goosie/"

// Allowed lists, for each v2 package, the v2 packages it may import. Anything
// absent from this table is a violation, and a package with no entry has no v2
// imports allowed at all.
//
// This is the import graph from the M1 spec as data. Encoding it here rather than
// relying on review is the point: the architecture's claims ("frame is a
// dependency leaf", "surface knows nothing about platforms") are the ones a later
// contributor is most likely to break by accident while fixing something urgent.
var Allowed = map[string][]string{
	"internal/frame":             nil,
	"internal/paint":             {"internal/frame", "internal/css", "internal/layout", "internal/style"},
	"internal/surface":           {"internal/frame", "internal/paint"},
	"internal/raster":            {"internal/frame", "internal/paint", "internal/surface"},
	"internal/platform/headless": {"internal/frame", "internal/surface"},
	"internal/platform/darwin":   {"internal/frame", "internal/surface"},
	"internal/platform":          {"internal/frame", "internal/surface", "internal/platform/headless", "internal/platform/darwin"},
	"internal/archtest":          nil,
	"internal/dom":               nil,
	"internal/css":               {"internal/dom"},
	"internal/net":               nil,
	"internal/image":             nil,
	"internal/style":             {"internal/css", "internal/dom", "internal/frame"},
	"internal/layout":            {"internal/dom", "internal/style", "internal/frame"},
	"internal/engine":            {"internal/dom", "internal/css", "internal/style", "internal/layout", "internal/paint", "internal/frame"},
	"internal/toolbar":           {"internal/frame", "internal/raster", "internal/surface"},
}

// FreeDirs are the subtrees exempt from the table. Tests and binaries may reach
// anything under v2; that is how the gate tests drive the whole frame path.
var FreeDirs = []string{"test/", "cmd/"}

// errEmptyList is the way `go list` fails that looks like success: a pattern that
// matches nothing exits zero, and a rule that checked nothing would report a clean
// architecture for a subtree it never read.
var errEmptyList = errors.New("archtest: go list reported no packages; the pattern or module path is wrong")

type errBadListLine string

func (e errBadListLine) Error() string {
	return fmt.Sprintf("archtest: unexpected go list output %q", string(e))
}

type errNoGoMod struct{ file string }

func (e errNoGoMod) Error() string {
	return fmt.Sprintf("archtest: no go.mod found above %s", e.file)
}

// errGoTool carries a go subcommand's stderr, which is the part that says *why* - a
// syntax error in one file fails a whole subtree's listing, and a test that only
// reports the exit status sends its reader to the wrong file.
type errGoTool struct{ args, stderr string }

func (e errGoTool) Error() string {
	return "archtest: go " + e.args + " failed: " + strings.TrimSpace(e.stderr)
}

func run(args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", errGoTool{args: strings.Join(args, " "), stderr: string(ee.Stderr)}
		}
		return "", err
	}
	return string(out), nil
}

// Symbols runs `go tool nm` on a compiled binary and returns the symbol names it
// lists. `go tool nm` ships with the toolchain, so the check needs no external
// binutils and reports the same thing on every runner.
//
// Names only. A linked binary's symbol table is the truth about what was pulled in -
// a cgo shim's frameworks never appear as a Go import anywhere - and for a rule about
// which libraries are present, a name is the whole question.
func Symbols(binary string) ([]string, error) {
	out, err := run("tool", "nm", binary)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[len(fields)-1])
	}
	return names, nil
}

// Package is one `go list` entry: a package and what it imports directly. Test-only
// imports are excluded, because `go list` reports them separately and the boundary
// rules are about the shipped graph.
type Package struct {
	Path    string
	Imports []string
}

// Rel returns the package path as the table names it, without the module prefix.
func (p Package) Rel() string { return strings.TrimPrefix(p.Path, ModulePrefix) }

// Violation is one edge the rules reject. Why carries the rule's own excuse, so a
// failure message reads as an explanation rather than as a denied assertion.
type Violation struct {
	Importer string
	Import   string
	Why      string
}

// String renders one violation as a single line.
func (v Violation) String() string {
	if v.Import == "" {
		return v.Importer + ": " + v.Why
	}
	return v.Importer + " imports " + v.Import + ": " + v.Why
}

// List runs `go list` for pattern in dir and returns one entry per package. dir is
// usually the module root, since a pattern like ./... means nothing elsewhere.
func List(dir, pattern string) ([]Package, error) {
	out, err := list(dir, "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", pattern)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		path, imports, ok := strings.Cut(line, "|")
		if !ok {
			return nil, errBadListLine(line)
		}
		pkgs = append(pkgs, Package{Path: path, Imports: strings.Fields(imports)})
	}
	if len(pkgs) == 0 {
		return nil, errEmptyList
	}
	return pkgs, nil
}

// Deps runs `go list -deps` for pattern in dir: the transitive closure of every
// import path the matched packages need, which is what a linkage check has to read.
// The direct imports are not enough - a dependency two hops down is as linked into
// the binary as one hop down is.
func Deps(dir, pattern string) ([]string, error) {
	out, err := list(dir, "-deps", pattern)
	if err != nil {
		return nil, err
	}
	var deps []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			deps = append(deps, line)
		}
	}
	return deps, nil
}

func list(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", append([]string{"list"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", errGoTool{args: "list " + strings.Join(args, " "), stderr: string(ee.Stderr)}
		}
		return "", err
	}
	return string(out), nil
}

// CheckLayers reports every v2-to-v2 import edge in pkgs that the Allowed table
// does not permit, plus every internal package the table does not know about: an
// unlisted package is unchecked, and unchecked is how a boundary ends up crossed by
// nobody noticing rather than by somebody deciding to.
func CheckLayers(pkgs []Package) []Violation {
	var v []Violation
	for _, p := range pkgs {
		rel := p.Rel()
		if isFreeDir(rel) {
			continue
		}
		if !strings.HasPrefix(rel, "internal/") {
			v = append(v, Violation{Importer: p.Path, Why: "not under internal/, test/, or cmd/"})
			continue
		}
		ok, exist := Allowed[rel]
		if !exist {
			v = append(v, Violation{Importer: rel, Why: "no entry in the allowed-imports table; add it deliberately"})
			continue
		}
		allowed := make(map[string]bool, len(ok))
		for _, a := range ok {
			allowed[ModulePrefix+a] = true
		}
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, ModulePrefix) && !allowed[imp] {
				v = append(v, Violation{
					Importer: rel,
					Import:   strings.TrimPrefix(imp, ModulePrefix),
					Why:      "the M1 import graph allows " + strings.Join(ok, ", "),
				})
			}
		}
	}
	return v
}

func isFreeDir(rel string) bool {
	for _, d := range FreeDirs {
		if strings.HasPrefix(rel, d) {
			return true
		}
	}
	return false
}

// CheckBanned reports every occurrence of a banned substring among the given import
// paths, attributed to importer when one is known. It backs both the toolkit check
// (direct imports of a GUI framework) and the linkage check (anything in the
// transitive closure of a graphics API, or in a binary's symbol table), which differ
// only in what they pass.
//
// The match is case-insensitive on purpose. A rule that missed "Metal" because it was
// written lowercase would pass on the day it mattered most, and the names it is looking
// for come from Apple and Khronos rather than from Go's naming.
func CheckBanned(importer string, paths []string, banned ...string) []Violation {
	var v []Violation
	low := make([]string, len(banned))
	for i, b := range banned {
		low[i] = strings.ToLower(b)
	}
	for _, p := range paths {
		ps := strings.ToLower(p)
		for i, b := range low {
			if strings.Contains(ps, b) {
				v = append(v, Violation{Importer: importer, Import: p, Why: "banned substring " + banned[i]})
				break
			}
		}
	}
	return v
}

// CheckBannedImports applies CheckBanned to each package's own direct imports, so a
// violation names the package that pulled the dependency in rather than the flat list
// it was found in.
func CheckBannedImports(pkgs []Package, banned ...string) []Violation {
	var v []Violation
	for _, p := range pkgs {
		v = append(v, CheckBanned(p.Rel(), p.Imports, banned...)...)
	}
	return v
}

// CheckBannedDeps is the linkage rule over a whole subtree: no package v2 builds
// may transitively depend on a path naming any of the banned substrings.
func CheckBannedDeps(deps []string, banned ...string) []Violation {
	var v []Violation
	for _, d := range deps {
		v = append(v, CheckBanned("", []string{d}, banned...)...)
	}
	return v
}

// RepoRoot walks up from file - normally a test's own path from runtime.Caller - to
// the directory holding go.mod, which is the only place where `go list ./...`
// and a walk of v2 mean what they say.
func RepoRoot(file string) (string, error) {
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errNoGoMod{file: file}
		}
		dir = parent
	}
}

// UsesCgo reports whether a file pulls in the C pseudo-package or builds only when
// cgo is enabled. Both count: the second is how a shim sneaks back in, because a
// //go:build cgo tag adds no import for an import graph to catch.
func UsesCgo(path string) bool {
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

// CheckCgo walks every .go file under v2Dir and returns the paths that use cgo but
// do not live in allowedDir. It reads files rather than the import graph because
// cgo needs no new import at all.
//
// Dot-directories and node_modules are pruned: a nested git worktree under
// .worktrees/ is a second full checkout, and walking it reports that checkout's
// own legal internal/platform/darwin files as violations.
func CheckCgo(v2Dir, allowedDir string) ([]string, error) {
	var found []string
	err := filepath.Walk(v2Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := filepath.Base(path)
			if path != v2Dir && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !UsesCgo(path) {
			return nil
		}
		if filepath.Dir(path) != allowedDir {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

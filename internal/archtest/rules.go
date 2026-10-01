package archtest

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	"internal/surface":           {"internal/ax", "internal/frame", "internal/paint"},
	"internal/raster":            {"internal/frame", "internal/paint", "internal/surface"},
	"internal/platform/headless": {"internal/ax", "internal/frame", "internal/surface"},
	"internal/platform/darwin":   {"internal/ax", "internal/frame", "internal/surface"},
	"internal/platform":          {"internal/frame", "internal/surface", "internal/platform/headless", "internal/platform/darwin"},
	"internal/archtest":          nil,
	"internal/dom":               nil,
	"internal/ax":                nil,
	"internal/css":               {"internal/dom"},
	"internal/net":               nil,
	"internal/download":          nil,
	"internal/session":           nil,
	"internal/image":             nil,
	"internal/js":                nil,
	"internal/bookmarks":         nil,
	"internal/history":           nil,
	"internal/style":             {"internal/css", "internal/dom", "internal/frame"},
	"internal/layout":            {"internal/dom", "internal/style", "internal/frame"},
	"internal/engine":            {"internal/ax", "internal/dom", "internal/css", "internal/style", "internal/layout", "internal/paint", "internal/frame", "internal/image", "internal/js"},
	"internal/toolbar":           {"internal/frame", "internal/raster", "internal/surface"},
	"internal/tabs":              {"internal/frame", "internal/toolbar", "internal/raster", "internal/engine"},
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

// UnformattedGoFiles returns every .go file under root whose bytes are not gofmt's own
// output for them. It is the check CONTRIBUTING.md's `gofmt -w .` never was: -w rewrites
// the file and exits 0 either way, so a contributor who skipped it saw no failure and the
// drift accumulated with nobody able to be told which file.
//
// go/format is the package cmd/gofmt is built from, so the verdict is the one an editor's
// format-on-save reaches, with no gofmt binary to find on PATH and no second implementation
// to keep in step with the first.
//
// Dot-directories, node_modules and testdata are pruned. The first two are CheckCgo's
// reasons and are sharper here: .worktrees/ holds a second full checkout, and `gofmt -l .`
// at the repo root reports that stale copy's files among the current ones - which is why
// the documented pre-commit command cannot double as the gate. testdata is what
// `go list ./...` never builds, and a deliberately malformed fixture is a parser test's
// input rather than drift.
//
// A file that does not parse is an error, not a clean report and not a violation: gofmt has
// no opinion about how such a file should look, and skipping it would leave the tree reading
// green while one file is not Go at all.
func UnformattedGoFiles(root string) ([]string, error) {
	var drift []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := filepath.Base(path)
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := format.Source(data)
		if err != nil {
			return errUnparseable{file: path, why: "does not parse: " + strings.TrimSpace(err.Error())}
		}
		if !bytes.Equal(data, out) {
			drift = append(drift, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return drift, nil
}

type errUnparseable struct{ file, why string }

func (e errUnparseable) Error() string {
	return "archtest: " + e.file + " " + e.why
}

// GlobalWrite is one assignment to a package-level var at file:line, attributed to the
// function that writes it so a reader finds the mutation rather than the declaration.
type GlobalWrite struct {
	// Pkg is the directory of the package whose var is written; File is the file holding
	// the assignment, which is not always the same one under a multi-file package.
	Pkg  string
	File string
	Line int
	Fn   string
	Var  string
	// Escape marks the address of a var being handed out (`&counter`) rather than the
	// var itself being assigned: the write then happens through the pointer, somewhere
	// this file cannot see.
	Escape bool
}

// String renders one write as a single locator line.
func (g GlobalWrite) String() string {
	if g.Escape {
		return fmt.Sprintf("%s:%d: &%s taken in %s()", filepath.ToSlash(g.File), g.Line, g.Var, g.Fn)
	}
	return fmt.Sprintf("%s:%d: %s assigned in %s()", filepath.ToSlash(g.File), g.Line, g.Var, g.Fn)
}

// MutableGlobals walks the .go files under dir - test files excluded, since a test may
// stage a package var freely - and reports every assignment to a package-level var that is
// neither part of that var's own declaration nor inside init().
//
// Mutation is read as a property of the code, not of a run: the two defects this rule
// exists to prevent were both invisible to -race until a test happened to drive concurrent
// document loads, and a global that no current test reaches is still shared by the next tab.
// A var declared without an initializer and written only by init() is accepted, because
// after start-up it is a constant: that is `dom.atomLookup` and the platform backend hooks,
// and it is the difference between state and configuration.
//
// An assignment through a composite (`settings[k] = v`, `rows = append(rows, …)`) counts,
// which is the shape a "don't reassign globals" review comment does not catch: the variable
// is never re-pointed, and the map behind it still accumulates across documents.
//
// What it cannot see: a write from another package into an exported var, which needs
// whole-program alias analysis, and a write through a pointer that left the package by some
// route other than `&pkgVar` in a function body (a var whose declaration itself is
// `var p = &T{}`, which is a value, not an escape). The address-of shape - `return &counter`
// - is reported since 2026-09-27, which is what made the first blind spot the expensive one
// rather than the common one.
//
// Files are grouped per package rather than analysed one at a time because a package var is
// visible from every file of its package: the setters this rule would have blocked assigned
// a var declared in a different file.
func MutableGlobals(dir string) ([]GlobalWrite, error) {
	pkgs, err := goFilesByPackage(dir)
	if err != nil {
		return nil, err
	}
	var found []GlobalWrite
	// Sorted package dirs so a failure lists the same violations in the same order twice.
	dirs := make([]string, 0, len(pkgs))
	for d := range pkgs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		vars := map[string]bool{}
		for _, path := range pkgs[d] {
			names, err := packageVarNames(path)
			if err != nil {
				return nil, err
			}
			for _, n := range names {
				vars[n] = true
			}
		}
		if len(vars) == 0 {
			continue
		}
		for _, path := range pkgs[d] {
			writes, err := globalWrites(path, vars, d)
			if err != nil {
				return nil, err
			}
			found = append(found, writes...)
		}
	}
	return found, nil
}

// goFilesByPackage maps each directory holding buildable Go code to its non-test files. A
// directory is taken to be one package: the only multi-package directories in this module
// separate a package from its _test variant, and those files are already excluded.
//
// Dot-directories, node_modules and testdata are pruned for UnformattedGoFiles' reasons -
// a nested worktree is a second full checkout whose stale files are not this tree's, and
// testdata is input rather than code.
func goFilesByPackage(dir string) (map[string][]string, error) {
	pkgs := map[string][]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := filepath.Base(path)
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parent := filepath.Dir(path)
		pkgs[parent] = append(pkgs[parent], path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pkgs, nil
}

func parseGo(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, errUnparseable{file: path, why: "does not parse: " + strings.TrimSpace(err.Error())}
	}
	return fset, f, nil
}

// packageVarNames returns the names of the vars declared at file scope.
func packageVarNames(path string) ([]string, error) {
	_, f, err := parseGo(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, s := range gd.Specs {
			for _, id := range s.(*ast.ValueSpec).Names {
				names = append(names, id.Name)
			}
		}
	}
	return names, nil
}

// globalWrites reports the assignments in one file whose left-hand side resolves to a name
// in vars. A name bound locally anywhere in the enclosing function - a receiver, a parameter,
// a :=, a range key, a named result - is treated as a shadow and skipped: the rule is about
// writes that reach the package, and a function that rebinds the name locally reaches nothing.
func globalWrites(path string, vars map[string]bool, pkgDir string) ([]GlobalWrite, error) {
	fset, f, err := parseGo(path)
	if err != nil {
		return nil, err
	}
	var found []GlobalWrite
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		// A method named init on a type is an ordinary function; only the package-level
		// init is the place a table is allowed to be filled.
		if fd.Recv == nil && fd.Name.Name == "init" {
			continue
		}
		locals := localNames(fd.Body)
		for _, name := range fieldNames(fd.Recv, fd.Type.Params, fd.Type.Results) {
			locals[name] = true
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.AND {
				// The address of a package var is the var's state on the loose: whoever
				// holds it can write without ever naming the var, which is the one shape
				// the assignment scan below structurally cannot see.
				if name, ok := rootIdent(u.X); ok && name != "_" && !locals[name] && vars[name] {
					found = append(found, GlobalWrite{
						Pkg: pkgDir, File: path, Line: fset.Position(n.Pos()).Line,
						Fn: fd.Name.Name, Var: name, Escape: true,
					})
				}
			}
			lhs := assignTargets(n)
			if lhs == nil {
				return true
			}
			for _, expr := range lhs {
				name, ok := rootIdent(expr)
				if !ok || name == "_" || locals[name] {
					continue
				}
				if !vars[name] {
					continue
				}
				found = append(found, GlobalWrite{
					Pkg:  pkgDir,
					File: path,
					Line: fset.Position(n.Pos()).Line,
					Fn:   fd.Name.Name,
					Var:  name,
				})
			}
			return true
		})
	}
	return found, nil
}

// assignTargets returns the written-to expressions of an assignment or increment, or nil for
// any other node.
func assignTargets(n ast.Node) []ast.Expr {
	switch x := n.(type) {
	case *ast.AssignStmt:
		return x.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{x.X}
	}
	return nil
}

// localNames collects every identifier a function body binds for itself.
func localNames(body *ast.BlockStmt) map[string]bool {
	locals := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, l := range x.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						locals[id.Name] = true
					}
				}
			}
		case *ast.DeclStmt:
			gd, ok := x.Decl.(*ast.GenDecl)
			if ok && gd.Tok == token.VAR {
				for _, s := range gd.Specs {
					for _, id := range s.(*ast.ValueSpec).Names {
						locals[id.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			for _, l := range []ast.Expr{x.Key, x.Value} {
				if id, ok := l.(*ast.Ident); ok {
					locals[id.Name] = true
				}
			}
		case *ast.FuncLit:
			for _, name := range fieldNames(x.Type.Params, x.Type.Results) {
				locals[name] = true
			}
		}
		return true
	})
	return locals
}

// fieldNames returns every identifier bound by the given parameter and result lists. A name
// so bound shadows a package-level var of the same name for the whole function, so writing to
// it writes a local. nil lists - an absent receiver, an unparenthesized signature - are skipped.
func fieldNames(lists ...*ast.FieldList) []string {
	var out []string
	for _, fl := range lists {
		if fl == nil {
			continue
		}
		for _, p := range fl.List {
			for _, id := range p.Names {
				out = append(out, id.Name)
			}
		}
	}
	return out
}

// rootIdent resolves an assignment target to the identifier it writes through: the name for
// `counter = 1`, the map or slice for `settings[k] = v`, the struct for `cfg.field = v`.
func rootIdent(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name, true
	case *ast.SelectorExpr:
		return rootIdent(x.X)
	case *ast.IndexExpr:
		return rootIdent(x.X)
	case *ast.IndexListExpr:
		return rootIdent(x.X)
	case *ast.StarExpr:
		return rootIdent(x.X)
	case *ast.ParenExpr:
		return rootIdent(x.X)
	}
	return "", false
}

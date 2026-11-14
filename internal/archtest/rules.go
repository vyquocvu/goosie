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

const ModulePrefix = "github.com/vyquocvu/goosie/"

var Allowed = map[string][]string{
	"internal/frame":             nil,
	"internal/paint":             {"internal/frame", "internal/css", "internal/layout", "internal/style"},
	"internal/surface":           {"internal/ax", "internal/frame", "internal/paint"},
	"internal/raster":            {"internal/frame", "internal/paint", "internal/surface"},
	"internal/platform/headless": {"internal/ax", "internal/frame", "internal/surface"},
	"internal/platform/darwin":   {"internal/ax", "internal/frame", "internal/media", "internal/surface"},
	"internal/platform":          {"internal/frame", "internal/media", "internal/surface", "internal/platform/headless", "internal/platform/darwin"},
	"internal/archtest":          nil,
	"internal/dom":               nil,
	"internal/ax":                nil,
	"internal/css":               {"internal/dom"},
	"internal/net":               nil,
	"internal/download":          nil,
	"internal/session":           nil,
	"internal/image":             nil,
	"internal/js":                {"internal/css", "internal/dom", "internal/media"},
	"internal/media":             nil,
	"internal/bookmarks":         nil,
	"internal/history":           nil,
	"internal/userscript":        nil,
	"internal/pdf":               nil,
	"internal/manifest":          nil,
	"internal/viewsource":        nil,
	"internal/flags":             nil,
	"internal/perms":             nil,
	"internal/preferences":       nil,
	"internal/style":             {"internal/css", "internal/dom", "internal/frame"},
	"internal/layout":            {"internal/dom", "internal/style", "internal/frame"},
	"internal/engine":            {"internal/ax", "internal/dom", "internal/css", "internal/style", "internal/layout", "internal/paint", "internal/frame", "internal/image", "internal/js", "internal/media", "internal/userscript"},
	"internal/toolbar":           {"internal/frame", "internal/raster", "internal/surface"},
	"internal/tabs":              {"internal/frame", "internal/toolbar", "internal/raster", "internal/engine", "internal/manifest"},
}

var FreeDirs = []string{"test/", "cmd/"}

var errEmptyList = errors.New("archtest: go list reported no packages; the pattern or module path is wrong")

type errBadListLine string

func (e errBadListLine) Error() string {
	return fmt.Sprintf("archtest: unexpected go list output %q", string(e))
}

type errNoGoMod struct{ file string }

func (e errNoGoMod) Error() string {
	return fmt.Sprintf("archtest: no go.mod found above %s", e.file)
}

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

type Package struct {
	Path    string
	Imports []string
}

func (p Package) Rel() string { return strings.TrimPrefix(p.Path, ModulePrefix) }

type Violation struct {
	Importer string
	Import   string
	Why      string
}

func (v Violation) String() string {
	if v.Import == "" {
		return v.Importer + ": " + v.Why
	}
	return v.Importer + " imports " + v.Import + ": " + v.Why
}

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

func CheckBannedImports(pkgs []Package, banned ...string) []Violation {
	var v []Violation
	for _, p := range pkgs {
		v = append(v, CheckBanned(p.Rel(), p.Imports, banned...)...)
	}
	return v
}

func CheckBannedDeps(deps []string, banned ...string) []Violation {
	var v []Violation
	for _, d := range deps {
		v = append(v, CheckBanned("", []string{d}, banned...)...)
	}
	return v
}

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

type GlobalWrite struct {
	Pkg    string
	File   string
	Line   int
	Fn     string
	Var    string
	Escape bool
}

func (g GlobalWrite) String() string {
	if g.Escape {
		return fmt.Sprintf("%s:%d: &%s taken in %s()", filepath.ToSlash(g.File), g.Line, g.Var, g.Fn)
	}
	return fmt.Sprintf("%s:%d: %s assigned in %s()", filepath.ToSlash(g.File), g.Line, g.Var, g.Fn)
}

func MutableGlobals(dir string) ([]GlobalWrite, error) {
	pkgs, err := goFilesByPackage(dir)
	if err != nil {
		return nil, err
	}
	var found []GlobalWrite
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

type ArenaPointerMap struct {
	File string
	Line int
}

func (m ArenaPointerMap) String() string {
	return fmt.Sprintf("%s:%d: map keyed by *layout.Object", filepath.ToSlash(m.File), m.Line)
}

func ArenaPointerMaps(dir string) ([]ArenaPointerMap, error) {
	pkgs, err := goFilesByPackage(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(pkgs))
	for d := range pkgs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	var found []ArenaPointerMap
	for _, d := range dirs {
		for _, path := range pkgs[d] {
			fset, file, err := parseGo(path)
			if err != nil {
				return nil, err
			}
			aliases := layoutImportNames(file)
			if len(aliases) == 0 {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				mt, ok := n.(*ast.MapType)
				if !ok {
					return true
				}
				if isLayoutObjectPointer(mt.Key, aliases) {
					found = append(found, ArenaPointerMap{File: path, Line: fset.Position(mt.Pos()).Line})
				}
				return true
			})
		}
	}
	return found, nil
}

type ArenaPointerField struct {
	File string
	Line int
}

func (f ArenaPointerField) String() string {
	return fmt.Sprintf("%s:%d: struct field holds *layout.Object", filepath.ToSlash(f.File), f.Line)
}

func ArenaPointerFields(dir string) ([]ArenaPointerField, error) {
	pkgs, err := goFilesByPackage(dir)
	if err != nil {
		return nil, err
	}
	dirs := make([]string, 0, len(pkgs))
	for d := range pkgs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	var found []ArenaPointerField
	for _, d := range dirs {
		for _, path := range pkgs[d] {
			fset, file, err := parseGo(path)
			if err != nil {
				return nil, err
			}
			aliases := layoutImportNames(file)
			if len(aliases) == 0 {
				continue
			}
			ast.Inspect(file, func(n ast.Node) bool {
				st, ok := n.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, fld := range st.Fields.List {
					if isLayoutObjectPointer(fld.Type, aliases) {
						found = append(found, ArenaPointerField{File: path, Line: fset.Position(fld.Pos()).Line})
					}
				}
				return true
			})
		}
	}
	return found, nil
}

func layoutImportNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != ModulePrefix+"internal/layout" {
			continue
		}
		if imp.Name != nil {
			names[imp.Name.Name] = true
		} else {
			names["layout"] = true
		}
	}
	return names
}

func isLayoutObjectPointer(e ast.Expr, aliases map[string]bool) bool {
	star, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Object" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && aliases[ident.Name]
}

func parseGo(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, errUnparseable{file: path, why: "does not parse: " + strings.TrimSpace(err.Error())}
	}
	return fset, f, nil
}

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
		if fd.Recv == nil && fd.Name.Name == "init" {
			continue
		}
		locals := localNames(fd.Body)
		for _, name := range fieldNames(fd.Recv, fd.Type.Params, fd.Type.Results) {
			locals[name] = true
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.AND {
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

func assignTargets(n ast.Node) []ast.Expr {
	switch x := n.(type) {
	case *ast.AssignStmt:
		return x.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{x.X}
	}
	return nil
}

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

package dom_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// TestNoUnboundedParseEntry is the fence behind this round's deletion of dom.Parse. A
// package-level function that turns an input string into a Document and reports no error
// is the exact shape that lets a caller skip the resource caps, and deleting it once is
// not enough - the next urgent fix re-adds it. So the shape itself is what fails here.
//
// It reads signatures rather than names on purpose: an unchecked entry renamed to
// MustParseHTML or buildDocument is the same hole, and NewDocument is legal because it
// takes no input at all.
func TestNoUnboundedParseEntry(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repo, err := archtest.RepoRoot(thisFile)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, "internal", "dom")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !fd.Name.IsExported() {
				continue
			}
			if takesString(fd.Type.Params) && returnsDocument(fd.Type.Results) && !returnsError(fd.Type.Results) {
				found = append(found, filepath.Base(name)+":"+fd.Name.Name)
			}
		}
	}
	sort.Strings(found)
	if len(found) > 0 {
		t.Errorf("internal/dom exports %d parse entries that accept input and cannot report a bound being crossed: %v. ParseBounded is the bounded entry; a test fixture wants internal/test-only domtest.Parse over it", len(found), found)
	}
}

func takesString(fl *ast.FieldList) bool {
	return anyField(fl, "string")
}

func returnsDocument(fl *ast.FieldList) bool {
	return anyField(fl, "Document")
}

func returnsError(fl *ast.FieldList) bool {
	return anyField(fl, "error")
}

// anyField reports whether a parameter or result list names one of its types with the
// given identifier. Reading the syntax rather than resolving types keeps this rule in the
// same go/ast parse the archtest rules already do.
func anyField(fl *ast.FieldList, name string) bool {
	if fl == nil {
		return false
	}
	for _, f := range fl.List {
		for _, t := range flatten(f.Type) {
			if id, ok := t.(*ast.Ident); ok && id.Name == name {
				return true
			}
		}
	}
	return false
}

// flatten returns the types one result or parameter field covers: `(a, b *Node)` is two
// fields' worth of types under one ast.Field.
func flatten(e ast.Expr) []ast.Expr {
	switch x := e.(type) {
	case *ast.StarExpr:
		return []ast.Expr{x.X}
	case *ast.ParenExpr:
		return flatten(x.X)
	default:
		return []ast.Expr{e}
	}
}

package archtest_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

const cleanGlobalsGo = `package p

var table = map[string]int{"seed": 1}

var filledByInit []string

func init() {
	filledByInit = append(filledByInit, "a")
}

func Shadow() int {
	table := map[string]int{}
	table["local"] = 2
	return len(table) + len(filledByInit) + len(table)
}
`

const dirtyGlobalsGo = `package p

var cached *string

var rows []int

var settings map[string]int

func Warm() {
	cached = new(string)
	rows = append(rows, 1)
	settings["x"] = 1
}
`

func writePackageGo(t *testing.T, dir string, bodies map[string]string) {
	t.Helper()
	for name, body := range bodies {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func writes(t *testing.T, root string, found []archtest.GlobalWrite) []string {
	t.Helper()
	out := make([]string, 0, len(found))
	for _, g := range found {
		rel, err := filepath.Rel(root, g.Pkg)
		if err != nil {
			t.Fatalf("%s: %v", g.Pkg, err)
		}
		out = append(out, filepath.ToSlash(rel)+"."+g.Fn+"."+g.Var)
	}
	sort.Strings(out)
	return out
}

func mustGlobals(t *testing.T, root string) []archtest.GlobalWrite {
	t.Helper()
	found, err := archtest.MutableGlobals(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return found
}

func TestMutableGlobalsFlagsWritesAndAcceptsInit(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "p"), map[string]string{
		"clean.go":     cleanGlobalsGo,
		"dirty.go":     dirtyGlobalsGo,
		"other.go":     "package p\n\nfunc Unused() {}\n",
		"skip_test.go": "package p\n\nfunc TestThing(t any) {\n\tcached = new(string)\n}\n",
	})

	got := writes(t, root, mustGlobals(t, root))
	want := []string{"p.Warm.cached", "p.Warm.rows", "p.Warm.settings"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("mutable globals = %v, want %v", got, want)
	}
}

func TestMutableGlobalsSeesAcrossPackageFiles(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "q"), map[string]string{
		"vars.go": "package q\n\nvar counter int\n",
		"set.go":  "package q\n\nfunc Bump() { counter++ }\n",
	})

	got := writes(t, root, mustGlobals(t, root))
	want := []string{"q.Bump.counter"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("mutable globals = %v, want %v", got, want)
	}
}

func TestMutableGlobalsPrunesExcludedDirs(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, root, map[string]string{"clean.go": cleanGlobalsGo})
	for _, dir := range []string{".worktrees/stale", "node_modules/pkg", "testdata/fixture"} {
		writePackageGo(t, filepath.Join(root, dir), map[string]string{"dirty.go": dirtyGlobalsGo})
	}
	if got := writes(t, root, mustGlobals(t, root)); len(got) != 0 {
		t.Errorf("excluded dirs reported: %v", got)
	}
}

func TestMutableGlobalsSkipsReceiverParamAndResultNames(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "r"), map[string]string{
		"vars.go": "package r\n\nvar rows []int\n\ntype Set struct{ rows []int }\n\n" +
			"func (s *Set) Clear() { s.rows = nil }\n\n" +
			"func (rows *Set) Take() { rows.rows = []int{1} }\n\n" +
			"func Count() (rows int) { rows = 3; return }\n\n" +
			"func Filter(rows []int) { rows[0] = 1 }\n",
	})
	if got := writes(t, root, mustGlobals(t, root)); len(got) != 0 {
		t.Errorf("mutable globals = %v, want none", got)
	}
}

func TestInternalHasNoMutableGlobals(t *testing.T) {
	root := moduleRoot(t)
	for _, g := range mustGlobals(t, filepath.Join(root, "internal")) {
		t.Errorf("%s: package-level var assigned outside its initializer and outside init()", g)
	}
}

func TestMutableGlobalsFlagsAddressOf(t *testing.T) {
	root := t.TempDir()
	writePackageGo(t, filepath.Join(root, "e"), map[string]string{
		"vars.go": "package e\n\nvar counter int\nvar cfg struct{ N int }\nvar p *int\n\n" +
			"type T struct{ v int }\n\nfunc (t *T) M() *int { return &t.v }\n",
		"esc.go": "package e\n\n" +
			"func Handle() *int { return &counter }\n" +
			"func Field() *int  { return &cfg.N }\n" +
			"func Local() *int  { var x int; return &x }\n" +
			"func Read() int    { return counter }\n" +
			"func init()        { p = &counter }\n",
	})

	got := writes(t, root, mustGlobals(t, root))
	want := []string{"e.Field.cfg", "e.Handle.counter"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("escaped globals = %v, want %v", got, want)
	}
}

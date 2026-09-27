package archtest_test

// The mission brief asks the engine for "zero global state", and rounds 15 and 16 deleted
// the last three mutable process-wide globals in internal/ by making each value travel with
// the document that owns it. Rounds cannot regress on their own: `style.SetCustomFonts` was
// written by somebody who thought it was the simple shape, and nothing in the suite objected
// until a document loaded in a second tab.
//
// So the rule is the deliverable, not the refactor. It reads the tree rather than a runtime
// registry because mutation is a property of the code: an assignment to a package-level var
// outside that var's initializer and outside init() is global state whether or not a test
// happens to reach it, and the two defects that were fixed were invisible to -race until a
// test drove concurrent loads.
//
// The exemption the rule grants is exactly `init()`, and it is a real one: a table filled
// once by init and read-only after is immutable for the life of the process, which is the
// letter of the clause without its intent being at risk. `dom.atomLookup` and the platform
// backend hooks are that shape.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
)

// The clean fixture is the shape the rule must accept, not an empty file: a var with an
// initializer, a var written only by init(), and a local that shadows a global name so a
// rule keyed on identifiers alone would report a violation that is not one.
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

// The three shapes of one defect: a plain assignment, a whole-value rewrite, and a write
// through a composite, which is the one a "don't assign to globals" review comment misses
// because the variable itself is never re-pointed.
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

// writes renders the rule's answer as "pkg.fn.var" strings, sorted, so a failure reads as
// the three names it has to check rather than as a wall of temp-dir paths.
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

// The non-vacuity cases. A rule that reported nothing for any input would pass the repo-wide
// check below and be worthless, so the drifted tree has to report exactly the three writes
// and the clean tree exactly none.
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

// A package-level var can be written from any file of its package, so a rule that collected
// the var set per file would report nothing for the shape that motivated it: the setters in
// internal/style and internal/css assigned a var declared in a different file.
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

// .worktrees/ holds a second full checkout of this module, so walking it would report that
// copy's files, and `go list ./...` never builds testdata, whose fixtures are deliberately
// malformed input for a parser test rather than code to police.
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

// A receiver, parameter or named result that happens to share a package var's name writes
// through itself, not through the global. Treating only the body's bindings as local would
// flag `rows.rows = nil`, `rows["x"] = 1` and `rows = 3` as writes to the package-level
// `rows`, and a rule that reports writes nobody made is a rule people switch off.
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

// The clause itself: internal/ is the engine, and it is the directory the brief's §B is
// about. cmd/ is exempt for the same reason the import table exempts it - a binary keeps its
// own flags and its own counters, and nothing else reads them.
func TestInternalHasNoMutableGlobals(t *testing.T) {
	root := moduleRoot(t)
	for _, g := range mustGlobals(t, filepath.Join(root, "internal")) {
		t.Errorf("%s: package-level var assigned outside its initializer and outside init()", g)
	}
}

// The address of a package var is that var's state with no name attached to it, so the
// assignment scan cannot see the write that follows. This is the shape the rule was
// documented as blind to until 2026-09-27, and it is now the one this case pins: a
// package var's address escapes from a function, while a local's, a receiver field's and
// an init() one do not.
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

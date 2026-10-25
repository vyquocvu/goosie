package qc

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate traceability.golden.json")

type goldenCase struct {
	ID       string   `json:"id"`
	Priority string   `json:"priority"`
	Methods  []string `json:"methods"`
	Status   string   `json:"status"`
	Tests    []string `json:"tests"`
	Enforced bool     `json:"enforced"`
}

func TestTraceability(t *testing.T) {
	root := repositoryRoot(t)
	catalog, err := parseCatalog(filepath.Join(root, "docs", "qa-test-catalog.md"))
	if err != nil {
		t.Fatal(err)
	}
	tests, skipping, err := scanTests(root)
	if err != nil {
		t.Fatal(err)
	}

	byID := make(map[string]Case, len(catalog))
	for _, c := range catalog {
		byID[c.ID] = c
	}
	enforced := make(map[string]bool, len(enforcedCases))
	for _, id := range enforcedCases {
		if _, ok := byID[id]; !ok {
			t.Errorf("enforced allowlist references unknown QC case %s", id)
		}
		if enforced[id] {
			t.Errorf("duplicate enforced QC case %s", id)
		}
		enforced[id] = true
	}

	for id, names := range tests {
		if _, ok := byID[id]; !ok {
			t.Errorf("unknown QC ID %s referenced by %s", id, strings.Join(names, ", "))
		}
	}

	var rows []goldenCase
	var covered, uncovered, cuaOnly, awaitingSignOff int
	for _, c := range catalog {
		refs := append([]string(nil), tests[c.ID]...)
		sort.Strings(refs)
		for _, ref := range refs {
			if why, ok := skipping[ref]; ok {
				t.Errorf("%s is claimed by %s, which %s: a tag is not evidence", c.ID, ref, why)
			}
		}
		if len(refs) > 0 {
			covered++
		} else {
			uncovered++
		}
		if len(refs) == 0 && onlyCUAOrGAP(c.Methods) {
			cuaOnly++
		}
		if c.Status == "PASS" && len(refs) == 0 {
			t.Errorf("%s is PASS but has no tagged automated test", c.ID)
		}
		if enforced[c.ID] && needsTest(c.Methods) && len(refs) == 0 {
			t.Errorf("%s is an enforced case requiring an INT/PARITY/SEC test but has no tagged test", c.ID)
		}
		if c.Status == "NOT RUN" && len(refs) > 0 {
			awaitingSignOff++
			t.Logf("%s has tagged tests (%s) but is still NOT RUN: automation present, awaiting QC sign-off", c.ID, strings.Join(refs, ", "))
		}
		rows = append(rows, goldenCase{
			ID: c.ID, Priority: c.Priority, Methods: c.Methods, Status: c.Status,
			Tests: refs, Enforced: enforced[c.ID],
		})
	}
	t.Logf("QC traceability: %d cases have tagged tests, %d lack tagged tests, %d are CUA/GAP-only without tags, %d have automation awaiting QC sign-off", covered, uncovered, cuaOnly, awaitingSignOff)

	goldenPath := filepath.Join(root, "test", "qc", "traceability.golden.json")
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if *updateGolden {
		if err := os.WriteFile(goldenPath, data, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read traceability artifact: %v (run go test ./test/qc -update)", err)
	}
	if string(got) != string(data) {
		t.Errorf("traceability.golden.json is stale; run go test ./test/qc -update")
	}
}

func TestCatalogRejectsMalformedCaseRow(t *testing.T) {
	root := repositoryRoot(t)
	path := filepath.Join(root, "docs", "qa-test-catalog.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(data), "- **BR-001 | P0 |", "- **BR-001 | PX |", 1)
	if broken == string(data) {
		t.Fatal("fixture did not contain the expected first case row")
	}
	copyPath := filepath.Join(t.TempDir(), "catalog.md")
	if err := os.WriteFile(copyPath, []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCatalog(copyPath); err == nil || !strings.Contains(err.Error(), "malformed QC case row") {
		t.Fatalf("parseCatalog error = %v, want malformed row error", err)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root (go.mod)")
		}
		dir = parent
	}
}

func scanTests(root string) (refs map[string][]string, skipping map[string]string, err error) {
	refs = make(map[string][]string)
	skipping = make(map[string]string)
	for _, base := range []string{
		filepath.Join(root, "test"),
		filepath.Join(root, "internal"),
		filepath.Join(root, "cmd", "goosie"),
	} {
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				fn, ok := node.(*ast.FuncDecl)
				if !ok || fn.Doc == nil || !(strings.HasPrefix(fn.Name.Name, "Test") || strings.HasPrefix(fn.Name.Name, "Fuzz")) {
					return true
				}
				why := unconditionalSkip(fn)
				for _, comment := range fn.Doc.List {
					text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
					if !strings.HasPrefix(text, "qc:") {
						continue
					}
					ref := fmt.Sprintf("%s:%s", filepath.ToSlash(rel), fn.Name.Name)
					for _, id := range strings.Fields(strings.TrimPrefix(text, "qc:")) {
						refs[id] = append(refs[id], ref)
					}
					if why != "" {
						skipping[ref] = why
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return refs, skipping, nil
}

func unconditionalSkip(fn *ast.FuncDecl) string {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return ""
	}
	expr, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return ""
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != "t" || (sel.Sel.Name != "Skip" && sel.Sel.Name != "Skipf") {
		return ""
	}
	return "skips unconditionally as its first statement"
}

func needsTest(methods []string) bool {
	for _, method := range methods {
		if method == "INT" || method == "PARITY" || method == "SEC" {
			return true
		}
	}
	return false
}

func onlyCUAOrGAP(methods []string) bool {
	for _, method := range methods {
		if method != "CUA" && method != "GAP" {
			return false
		}
	}
	return true
}

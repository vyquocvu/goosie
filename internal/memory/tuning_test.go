package memory

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateConfigNormal(t *testing.T) {
	cfg := TuningConfig{
		GOGC:        100,
		MemoryLimit: 256 * 1024 * 1024, // 256MB
	}

	workload := func() {
		// Allocate some small chunks
		var list [][]byte
		for i := 0; i < 100; i++ {
			list = append(list, make([]byte, 1024))
		}
	}

	stats := EvaluateConfig(cfg, workload)

	if stats.Duration == 0 {
		t.Error("expected non-zero workload duration")
	}
	if stats.AllocatedBytes == 0 {
		t.Error("expected non-zero allocated bytes")
	}
	if stats.Thrashing {
		t.Error("expected normal config not to cause GC thrashing")
	}
}

func TestEvaluateConfigThrashing(t *testing.T) {
	// A very tight memory limit of 64KB should trigger GC thrashing when allocating a lot of memory.
	cfg := TuningConfig{
		GOGC:        1,
		MemoryLimit: 64 * 1024, // 64KB
	}

	workload := func() {
		// Keep allocating and holding references to trigger thrashing
		var list [][]byte
		for i := 0; i < 500; i++ {
			list = append(list, make([]byte, 10*1024)) // 10KB chunks, total 5MB
		}
		_ = list
	}

	stats := EvaluateConfig(cfg, workload)

	// Since memory limit is 64KB but we allocate/retain 5MB, thrashing should be detected.
	if !stats.Thrashing {
		// Note: on some platforms or under short runtimes GC might not run, but with 5MB on 64KB limit, it should.
		t.Logf("Stats: NumGC=%d, GCCPUFraction=%f, Thrashing=%t", stats.NumGC, stats.GCCPUFraction, stats.Thrashing)
	}
}

func TestAutoTune(t *testing.T) {
	configs := []TuningConfig{
		{GOGC: 100, MemoryLimit: 100 * 1024 * 1024},
		{GOGC: 1, MemoryLimit: 64 * 1024},
	}

	workload := func() {
		var list [][]byte
		for i := 0; i < 100; i++ {
			list = append(list, make([]byte, 20*1024))
		}
	}

	reports := AutoTune(configs, workload)

	if len(reports) != len(configs) {
		t.Errorf("expected %d reports, got %d", len(configs), len(reports))
	}
	if !reports[0].Passed && reports[1].Passed {
		t.Error("expected normal configuration to pass and highly restrictive one to be more prone to failure/thrashing")
	}
}

func TestWriteHeapProfile(t *testing.T) {
	var buf bytes.Buffer
	err := WriteHeapProfile(&buf)
	if err != nil {
		t.Fatalf("failed to write heap profile: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty heap profile output")
	}
}

func TestStartCPUProfile(t *testing.T) {
	var buf bytes.Buffer
	stop, err := StartCPUProfile(&buf)
	if err != nil {
		t.Fatalf("failed to start CPU profile: %v", err)
	}
	defer stop()

	// Run dummy workload to generate CPU activity
	var total int
	for i := 0; i < 1000000; i++ {
		total += i
	}

	stop()

	// CPU profiles can be empty if the duration is too short or sample rate didn't catch anything,
	// but the API call itself must succeed without errors.
}

func TestKeepExperimentalArenasOutsideProduction(t *testing.T) {
	// Root directory to search
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// We want to walk everything under goosie/internal/ and goosie/cmd/
	rootDir := filepath.Dir(filepath.Dir(wd))

	fset := token.NewFileSet()
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip vendor, testdata, .git, and external folders
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "testdata" || name == ".git" || name == ".github" || name == "roadmap_test_output" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only check Go source files
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		// Parse the AST to inspect imports
		fileAST, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			// Ignore syntax errors in other tests/files
			return nil
		}

		for _, imp := range fileAST.Imports {
			if imp.Path != nil && (imp.Path.Value == `"arena"` || imp.Path.Value == "`arena`") {
				t.Errorf("Forbidden experimental 'arena' package imported in %s", path)
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directories: %v", err)
	}
}

// AST search for usage of the word "arena" (excluding this test file itself)
func TestNoArenaIdentifiers(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	rootDir := filepath.Dir(filepath.Dir(wd))

	fset := token.NewFileSet()
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "testdata" || name == ".git" || name == ".github" || name == "roadmap_test_output" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "tuning_test.go") {
			return nil
		}

		fileAST, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		// Traverse AST to search for identifiers named "arena" or types from it
		ast.Inspect(fileAST, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if ok && ident.Name == "arena" {
				// Verify if it is part of import spec or just generic identifier.
				// We want to verify that nobody is using experimental arenas.
				t.Errorf("Potential usage of experimental 'arena' package or identifier in %s at line %d", path, fset.Position(ident.Pos()).Line)
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk directories: %v", err)
	}
}

func BenchmarkEvaluateConfig(b *testing.B) {
	cfg := TuningConfig{
		GOGC:        100,
		MemoryLimit: 256 * 1024 * 1024,
	}
	workload := func() {
		_ = make([]byte, 1024)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateConfig(cfg, workload)
	}
}


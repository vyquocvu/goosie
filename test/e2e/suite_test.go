package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestComprehensiveSuite runs Visual and Structure subtests over all generated
// HTML files in testdata/. Each file gets:
//   - Visual: screenshot compared against a stored baseline (screenshotTolerance)
//   - Structure: Playwright Expect assertions appropriate for the file's category
//
// Goosie-vs-browser parity tests are in TestGoosieVsBrowser (opt-in via RUN_PARITY_TESTS=true).
func TestComprehensiveSuite(t *testing.T) {
	config := VisualTestConfig{
		UpdateBase:     os.Getenv("UPDATE_SNAPSHOTS") == "true",
		OutputDir:      filepath.Join("testdata", "results"),
		ViewportWidth:  1280,
		ViewportHeight: 800,
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	testDataDir := filepath.Join(projectRoot, "testdata")

	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		t.Fatalf("testdata directory not found at %s. Run 'make generate-test-data' first.", testDataDir)
	}

	files, err := filepath.Glob(filepath.Join(testDataDir, "test_*.html"))
	require.NoError(t, err)

	if len(files) == 0 {
		t.Fatal("No test_*.html files found. Run 'make generate-test-data' first.")
	}

	t.Logf("Found %d test cases", len(files))

	for _, file := range files {
		file := file // capture loop variable
		baseName := filepath.Base(file)
		testName := strings.TrimSuffix(baseName, ".html")

		t.Run(testName, func(t *testing.T) {
			page := newPage(t)
			defer page.Close()

			_, err := page.Goto("file://" + file)
			require.NoError(t, err, "failed to load page")

			t.Run("Visual", func(t *testing.T) {
				CompareScreenshot(t, page, testName, config)
			})

			t.Run("Structure", func(t *testing.T) {
				ValidateStructure(t, page, testName)
			})
		})
	}
}

// TestGoosieVsBrowser compares Goosie's rendering against Chromium for all
// generated HTML files. Only runs when RUN_PARITY_TESTS=true.
func TestGoosieVsBrowser(t *testing.T) {
	if !isParityTestEnabled() {
		t.Skip("parity tests disabled; set RUN_PARITY_TESTS=true to enable")
	}

	config := VisualTestConfig{
		OutputDir:     filepath.Join("testdata", "results"),
		ViewportWidth: 1280,
		ViewportHeight: 800,
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	testDataDir := filepath.Join(projectRoot, "testdata")

	if _, err := os.Stat(testDataDir); os.IsNotExist(err) {
		t.Fatalf("testdata directory not found at %s. Run 'make generate-test-data' first.", testDataDir)
	}

	files, err := filepath.Glob(filepath.Join(testDataDir, "test_*.html"))
	require.NoError(t, err)

	if len(files) == 0 {
		t.Fatal("No test_*.html files found. Run 'make generate-test-data' first.")
	}

	for _, file := range files {
		file := file
		baseName := filepath.Base(file)
		testName := strings.TrimSuffix(baseName, ".html")

		t.Run(testName, func(t *testing.T) {
			page := newPage(t)
			defer page.Close()

			CompareGoosieVsBrowser(t, page, file, testName, config)
		})
	}
}

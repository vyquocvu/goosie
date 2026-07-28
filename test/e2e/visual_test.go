//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOMMutationGoosieVsBrowser(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fixturePath := filepath.Join(cwd, "fixtures", "dom_mutation_flexbox.html")

	page := newPage(t)
	defer page.Close()
	config := VisualTestConfig{
		DiffThreshold:  0.08,
		OutputDir:      filepath.Join("testdata", "results"),
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	CompareGoosieVsBrowser(t, page, fixturePath, "dom_mutation_flexbox", config)
}

func TestHTML5SemanticLayoutGoosieVsBrowser(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	fixturePath := filepath.Join(cwd, "fixtures", "html5_semantic_layout.html")

	page := newPage(t)
	defer page.Close()
	config := VisualTestConfig{
		DiffThreshold:  0.10,
		OutputDir:      filepath.Join("testdata", "results"),
		ViewportWidth:  1280,
		ViewportHeight: 800,
	}
	CompareGoosieVsBrowser(t, page, fixturePath, "html5_semantic_layout", config)
}

func TestGeneratedHTML(t *testing.T) {
	// Verify testdata/output.html exists
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Since we run tests from test/e2e, the root is two levels up
	projectRoot := filepath.Dir(filepath.Dir(cwd))
	htmlPath := filepath.Join(projectRoot, "testdata", "output.html")

	info, err := os.Stat(htmlPath)
	require.NoError(t, err, "output.html should exist")
	require.False(t, info.IsDir(), "output.html should be a file")

	// Create a new page
	page, err := browser.NewPage()
	require.NoError(t, err)
	defer page.Close()

	// Load the HTML file
	// We need a file:// URL
	fileURL := "file://" + htmlPath
	_, err = page.Goto(fileURL)
	require.NoError(t, err)

	// Verify Title
	title, err := page.Title()
	require.NoError(t, err)
	assert.Equal(t, "Goosie Test Output", title)

	// Verify DOM Structure
	// Check for the container
	container := page.Locator(".container")
	count, err := container.Count()
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Check for 3 boxes
	boxes := page.Locator(".box")
	count, err = boxes.Count()
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify Box Contents
	redBox := boxes.First()
	text, err := redBox.TextContent()
	require.NoError(t, err)
	assert.Equal(t, "Red", text)

	// Visual Regression (Screenshot)
	// Taking a screenshot
	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String("screenshot.png"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, screenshot)

	// Note: To implement true visual regression (comparing against a baseline),
	// we would compare 'screenshot' bytes with a stored 'baseline.png'.
	// For this task, we ensure we can take the screenshot.
	// If playwright-go's Expect().ToHaveScreenshot() is available and configured, use it:
	// require.NoError(t, playwright.NewPlaywrightAssertions(t).Page(page).ToHaveScreenshot())
}

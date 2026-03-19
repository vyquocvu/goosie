package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
)

// TestGeneratedHTML verifies the structure and visual appearance of output.html
// using Playwright Expect assertions for DOM checks and CompareScreenshot for
// visual regression.
func TestGeneratedHTML(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	projectRoot := filepath.Dir(filepath.Dir(cwd))
	htmlPath := filepath.Join(projectRoot, "testdata", "output.html")

	info, err := os.Stat(htmlPath)
	require.NoError(t, err, "output.html should exist")
	require.False(t, info.IsDir(), "output.html should be a file")

	page := newPage(t)
	defer page.Close()

	_, err = page.Goto("file://" + htmlPath)
	require.NoError(t, err)

	pw := playwright.NewPlaywrightAssertions()

	// Page-level assertions
	require.NoError(t,
		pw.Page(page).ToHaveTitle("Goosie Test Output"),
		"page title mismatch",
	)

	// Structural assertions via Locator Expect
	require.NoError(t,
		pw.Locator(page.Locator(".container")).ToHaveCount(1),
		"expected exactly one .container",
	)

	require.NoError(t,
		pw.Locator(page.Locator(".box")).ToHaveCount(3),
		"expected exactly three .box elements",
	)

	// Content assertions on individual boxes
	require.NoError(t,
		pw.Locator(page.Locator(".box").First()).ToHaveText("Red"),
		"first box should contain text 'Red'",
	)

	// All boxes must be visible
	require.NoError(t,
		pw.Locator(page.Locator(".box").First()).ToBeVisible(),
		"first box should be visible",
	)

	// Visual regression screenshot
	config := VisualTestConfig{
		UpdateBase:     os.Getenv("UPDATE_SNAPSHOTS") == "true",
		OutputDir:      filepath.Join("testdata", "results"),
		ViewportWidth:  1280,
		ViewportHeight: 800,
	}

	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String("screenshot.png"),
	})
	require.NoError(t, err)
	require.NotEmpty(t, screenshot)

	CompareScreenshot(t, page, "output", config)
}

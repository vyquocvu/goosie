package e2e

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

// screenshotTolerance is the maximum fraction of pixels that may differ between
// a baseline and an actual screenshot. 0.0001 (0.01%) allows for sub-pixel
// anti-aliasing differences while catching real visual regressions.
const screenshotTolerance = 0.0001

// VisualTestConfig holds configuration for visual testing.
type VisualTestConfig struct {
	UpdateBase     bool // When true, overwrite baseline images instead of comparing
	OutputDir      string
	ViewportWidth  int
	ViewportHeight int
}

// isParityTestEnabled reports whether the Goosie-vs-browser parity tests should run.
// Set RUN_PARITY_TESTS=true to enable.
func isParityTestEnabled() bool {
	return os.Getenv("RUN_PARITY_TESTS") == "true"
}

// CompareScreenshot takes a full-page screenshot and compares it against a stored
// baseline using screenshotTolerance. On first run (or when UpdateBase is true)
// the screenshot is saved as the new baseline.
func CompareScreenshot(t *testing.T, page playwright.Page, name string, config VisualTestConfig) {
	t.Helper()

	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
	})
	require.NoError(t, err, "failed to take screenshot")

	baselinePath := filepath.Join("testdata", "baselines", name+".png")
	diffPath := filepath.Join(config.OutputDir, "diffs", name+".png")
	actualPath := filepath.Join(config.OutputDir, "actual", name+".png")

	os.MkdirAll(filepath.Dir(baselinePath), 0755)
	os.MkdirAll(filepath.Dir(diffPath), 0755)
	os.MkdirAll(filepath.Dir(actualPath), 0755)

	require.NoError(t, os.WriteFile(actualPath, screenshot, 0644), "failed to save actual screenshot")

	if config.UpdateBase {
		require.NoError(t, os.WriteFile(baselinePath, screenshot, 0644), "failed to update baseline")
		t.Logf("baseline updated: %s", name)
		return
	}

	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		require.NoError(t, os.WriteFile(baselinePath, screenshot, 0644), "failed to create initial baseline")
		t.Logf("baseline created: %s", name)
		return
	}

	baselineData, err := os.ReadFile(baselinePath)
	require.NoError(t, err, "failed to read baseline")

	diffPercent, err := compareImages(baselineData, screenshot, diffPath)
	require.NoError(t, err, "failed to compare images")

	if diffPercent > screenshotTolerance {
		t.Errorf("visual regression in %s: %.4f%% pixel diff (tolerance %.4f%%)",
			name, diffPercent*100, screenshotTolerance*100)
	} else {
		os.Remove(diffPath)
	}
}

// compareImages compares two PNG images pixel-by-pixel and returns the fraction
// of differing pixels. A diff image is written to diffPath when pixels differ.
func compareImages(img1Data, img2Data []byte, diffPath string) (float64, error) {
	img1, _, err := image.Decode(bytes.NewReader(img1Data))
	if err != nil {
		return 0, fmt.Errorf("failed to decode image 1: %w", err)
	}

	img2, _, err := image.Decode(bytes.NewReader(img2Data))
	if err != nil {
		return 0, fmt.Errorf("failed to decode image 2: %w", err)
	}

	bounds := img1.Bounds()
	if !bounds.Eq(img2.Bounds()) {
		return 1.0, fmt.Errorf("image dimensions mismatch: %v vs %v", bounds, img2.Bounds())
	}

	diffImg := image.NewRGBA(bounds)
	diffPixels := 0
	totalPixels := bounds.Dx() * bounds.Dy()

	red := color.RGBA{R: 255, A: 255}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, a1 := img1.At(x, y).RGBA()
			r2, g2, b2, a2 := img2.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diffPixels++
				diffImg.Set(x, y, red)
			}
		}
	}

	if diffPixels > 0 && diffPath != "" {
		f, err := os.Create(diffPath)
		if err != nil {
			return 0, fmt.Errorf("failed to create diff file: %w", err)
		}
		defer f.Close()
		if err := png.Encode(f, diffImg); err != nil {
			return 0, fmt.Errorf("failed to encode diff image: %w", err)
		}
	}

	return float64(diffPixels) / float64(totalPixels), nil
}

// ValidateStructure runs category-appropriate Playwright Expect assertions against
// the loaded page. Category is inferred from the test name.
func ValidateStructure(t *testing.T, page playwright.Page, name string) {
	t.Helper()

	pw := playwright.NewPlaywrightAssertions()

	// body must always be attached to the DOM (ToBeAttached rather than ToBeVisible
	// because some pages only contain absolutely-positioned elements with no text,
	// which Playwright considers "hidden" even though the page loaded correctly).
	require.NoError(t,
		pw.Locator(page.Locator("body")).ToBeAttached(),
		"body not attached in %s", name,
	)

	switch {
	case strings.Contains(name, "typography"):
		headings := page.Locator("h1, h2, h3, h4, h5, h6, p")
		count, err := headings.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(headings.First()).ToBeVisible(),
				"no visible heading/paragraph in %s", name,
			)
		}

	case strings.Contains(name, "layout"):
		// Use ToBeAttached: layout tests may contain only absolutely-positioned
		// empty elements which Playwright considers "hidden" despite being rendered.
		blocks := page.Locator("div, section, article, main, header, footer")
		count, err := blocks.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(blocks.First()).ToBeAttached(),
				"no attached block element in %s", name,
			)
		}

	case strings.Contains(name, "flexbox"):
		children := page.Locator("div > div, div > span, div > p")
		count, err := children.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(children.First()).ToBeAttached(),
				"no attached flex child in %s", name,
			)
		}

	case strings.Contains(name, "forms"):
		// Form controls are interactive elements — ToBeVisible is appropriate here.
		controls := page.Locator("input, button, select, textarea")
		count, err := controls.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(controls.First()).ToBeVisible(),
				"no visible form control in %s", name,
			)
		}

	case strings.Contains(name, "tables"):
		rows := page.Locator("tr")
		count, err := rows.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(rows.First()).ToBeVisible(),
				"no visible table row in %s", name,
			)
		}

	case strings.Contains(name, "grid"):
		grids := page.Locator("div, section")
		count, err := grids.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(grids.First()).ToBeAttached(),
				"no attached grid container in %s", name,
			)
		}

	case strings.Contains(name, "media"):
		imgs := page.Locator("img")
		count, err := imgs.Count()
		require.NoError(t, err)
		if count > 0 {
			require.NoError(t,
				pw.Locator(imgs.First()).ToBeAttached(),
				"no attached img in %s", name,
			)
		}

	default:
		// css_advanced, edge_cases: body visibility is sufficient
	}
}

// CompareGoosieVsBrowser renders the HTML with Goosie and Playwright, saves both
// screenshots, and reports the pixel diff. Only called when RUN_PARITY_TESTS=true.
func CompareGoosieVsBrowser(t *testing.T, page playwright.Page, filePath string, name string, config VisualTestConfig) {
	t.Helper()

	width := config.ViewportWidth
	if width <= 0 {
		width = 1280
	}
	height := config.ViewportHeight
	if height <= 0 {
		height = 800
	}

	htmlBytes, err := os.ReadFile(filePath)
	require.NoError(t, err)

	r := renderer.NewRenderer(float32(width), float32(height))
	abs, _ := filepath.Abs(filePath)
	r.SetCurrentURL("file://" + abs)
	obj, err := r.RenderHTML(string(htmlBytes))
	require.NoError(t, err)

	h := int(r.GetContentHeight())
	if h > 0 {
		height = h
	}

	goosieImg, err := testutil.RenderToImage(obj, width, height)
	require.NoError(t, err)

	require.NoError(t, page.SetViewportSize(width, height))

	_, err = page.Goto("file://" + filePath)
	require.NoError(t, err)

	// Normalise fonts to reduce renderer-agnostic differences
	_, _ = page.Evaluate(`() => {
		const style = document.createElement('style');
		style.textContent = "* { font-family: -apple-system, BlinkMacSystemFont, \"Segoe UI\", Roboto, \"Helvetica Neue\", Arial, sans-serif !important; line-height: 1.2; } body { background: #ffffff; }";
		document.head.appendChild(style);
	}`)

	browserPNG, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(false),
	})
	require.NoError(t, err)

	var goosieBuf bytes.Buffer
	require.NoError(t, png.Encode(&goosieBuf, goosieImg))

	goosiePath := filepath.Join(config.OutputDir, "goosie", name+".png")
	browserPath := filepath.Join(config.OutputDir, "browser", name+".png")
	diffPath := filepath.Join(config.OutputDir, "diffs_compare", name+".png")

	os.MkdirAll(filepath.Dir(goosiePath), 0755)
	os.MkdirAll(filepath.Dir(browserPath), 0755)
	os.MkdirAll(filepath.Dir(diffPath), 0755)

	require.NoError(t, os.WriteFile(goosiePath, goosieBuf.Bytes(), 0644))
	require.NoError(t, os.WriteFile(browserPath, browserPNG, 0644))

	diffPercent, err := compareImages(goosieBuf.Bytes(), browserPNG, diffPath)
	require.NoError(t, err)

	// Parity tests use a relaxed threshold — Goosie and Chromium are different renderers
	const parityTolerance = 0.30
	if diffPercent > parityTolerance {
		t.Errorf("Goosie vs browser mismatch for %s: %.2f%% difference (tolerance %.0f%%)",
			name, diffPercent*100, parityTolerance*100)
	} else {
		os.Remove(diffPath)
	}
}

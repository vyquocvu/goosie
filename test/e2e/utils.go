//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goosiejs "github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
	ghtml "golang.org/x/net/html"
)

// VisualTestConfig holds configuration for visual testing
type VisualTestConfig struct {
	DiffThreshold  float64 // Percentage of pixels that can differ (0.0 - 1.0)
	UpdateBase     bool    // Whether to update baseline images
	OutputDir      string  // Directory for test artifacts
	ViewportWidth  int
	ViewportHeight int
}

// CompareScreenshot takes a screenshot of the current page and compares it with a baseline
func CompareScreenshot(t *testing.T, page playwright.Page, name string, config VisualTestConfig) {
	// Take screenshot
	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
	})
	require.NoError(t, err, "failed to take screenshot")

	baselinePath := filepath.Join("testdata", "baselines", name+".png")
	diffPath := filepath.Join(config.OutputDir, "diffs", name+".png")
	actualPath := filepath.Join(config.OutputDir, "actual", name+".png")

	// Ensure directories exist
	os.MkdirAll(filepath.Dir(baselinePath), 0755)
	os.MkdirAll(filepath.Dir(diffPath), 0755)
	os.MkdirAll(filepath.Dir(actualPath), 0755)

	// Save actual screenshot
	err = os.WriteFile(actualPath, screenshot, 0644)
	require.NoError(t, err, "failed to save actual screenshot")

	// If update baseline is requested or baseline doesn't exist, save it and skip comparison
	if config.UpdateBase {
		err = os.WriteFile(baselinePath, screenshot, 0644)
		require.NoError(t, err, "failed to update baseline")
		t.Logf("Baseline updated for %s", name)
		return
	}

	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		// If baseline doesn't exist, treat it as a new test case
		err = os.WriteFile(baselinePath, screenshot, 0644)
		require.NoError(t, err, "failed to create initial baseline")
		t.Logf("Created initial baseline for %s", name)
		return
	}

	// Read baseline
	baselineData, err := os.ReadFile(baselinePath)
	require.NoError(t, err, "failed to read baseline")

	// Compare images
	diffPercent, err := compareImages(baselineData, screenshot, diffPath)
	require.NoError(t, err, "failed to compare images")

	if diffPercent > config.DiffThreshold {
		t.Errorf("Visual regression detected for %s: %.2f%% difference (threshold: %.2f%%)", name, diffPercent*100, config.DiffThreshold*100)
	} else {
		// Clean up diff if passed
		os.Remove(diffPath)
	}
}

// compareImages compares two PNG images and returns the percentage of different pixels.
// It also generates a diff image if a diffPath is provided.
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

	red := color.RGBA{255, 0, 0, 255}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, a1 := img1.At(x, y).RGBA()
			r2, g2, b2, a2 := img2.At(x, y).RGBA()

			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diffPixels++
				// Draw difference in red on diff image
				diffImg.Set(x, y, red)
			}
		}
	}

	// Save diff image if there are differences
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

// CompareGoosieVsBrowser renders with Goosie and Playwright and compares screenshots
func CompareGoosieVsBrowser(t *testing.T, page playwright.Page, filePath string, name string, config VisualTestConfig) {
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
	htmlSource := string(htmlBytes)
	if strings.Contains(htmlSource, "data-goosie-execute-scripts") {
		htmlSource, err = executeInlineScriptsForComparison(htmlSource)
		require.NoError(t, err)
	}
	r := renderer.NewRenderer(float32(width), float32(height))
	abs, _ := filepath.Abs(filePath)
	r.SetCurrentURL("file://" + abs)
	obj, err := r.RenderHTML(context.Background(), htmlSource)
	require.NoError(t, err)
	h := int(r.GetContentHeight())
	if h > 0 {
		height = h
	}
	goosieImg, err := testutil.RenderToImage(obj, width, height)
	require.NoError(t, err)
	err = page.SetViewportSize(width, height)
	require.NoError(t, err)
	fileURL := "file://" + filePath
	_, err = page.Goto(fileURL)
	require.NoError(t, err)
	// Inject CSS to normalize fonts closer to Goosie/Fyne rendering
	_, _ = page.Evaluate(`() => {
		const style = document.createElement('style');
		style.textContent = "* { font-family: -apple-system, BlinkMacSystemFont, \\"Segoe UI\\", Roboto, \\"Helvetica Neue\\", Arial, sans-serif !important; line-height: 1.2; } body { background: #ffffff; }";
		document.head.appendChild(style);
	}`)
	browserPNG, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(false),
	})
	require.NoError(t, err)
	var goosieBuf bytes.Buffer
	err = png.Encode(&goosieBuf, goosieImg)
	require.NoError(t, err)
	goosiePath := filepath.Join(config.OutputDir, "goosie", name+".png")
	browserPath := filepath.Join(config.OutputDir, "browser", name+".png")
	diffPath := filepath.Join(config.OutputDir, "diffs_compare", name+".png")
	os.MkdirAll(filepath.Dir(goosiePath), 0755)
	os.MkdirAll(filepath.Dir(browserPath), 0755)
	os.MkdirAll(filepath.Dir(diffPath), 0755)
	err = os.WriteFile(goosiePath, goosieBuf.Bytes(), 0644)
	require.NoError(t, err)
	err = os.WriteFile(browserPath, browserPNG, 0644)
	require.NoError(t, err)
	diffPercent, err := compareImages(goosieBuf.Bytes(), browserPNG, diffPath)
	require.NoError(t, err)
	if diffPercent > config.DiffThreshold {
		t.Errorf("Goosie vs Browser mismatch for %s: %.2f%% difference", name, diffPercent*100)
	} else {
		os.Remove(diffPath)
	}
}

func executeInlineScriptsForComparison(source string) (string, error) {
	runtime := goosiejs.NewRuntime()
	defer runtime.Cleanup()
	runtime.SetHTMLContent(source)
	mutated := source
	runtime.SetDOMMutationCallback(func(html string) {
		mutated = html
	})

	doc, err := ghtml.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var runErr error
	var walk func(*ghtml.Node)
	walk = func(node *ghtml.Node) {
		if node == nil || runErr != nil {
			return
		}
		if node.Type == ghtml.ElementNode && node.Data == "script" {
			hasSource := false
			for _, attr := range node.Attr {
				if attr.Key == "src" && attr.Val != "" {
					hasSource = true
					break
				}
			}
			if !hasSource {
				var body strings.Builder
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == ghtml.TextNode {
						body.WriteString(child.Data)
					}
				}
				if _, err := runtime.RunScript(body.String()); err != nil {
					runErr = err
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if runErr != nil {
		return "", runErr
	}
	return mutated, nil
}

// ValidateDOMSnapshot captures the computed style and structure of elements
// and compares it against a baseline JSON file.
func ValidateDOMSnapshot(t *testing.T, page playwright.Page, name string, config VisualTestConfig) {
	// Execute script to extract DOM structure and computed styles
	// This is a simplified version; a full version would traverse the tree recursively
	script := `
	() => {
		function serialize(node) {
			if (node.nodeType === Node.TEXT_NODE) {
				return { type: 'text', content: node.textContent.trim() };
			}
			if (node.nodeType !== Node.ELEMENT_NODE) return null;
			
			const style = window.getComputedStyle(node);
			const rect = node.getBoundingClientRect();
			
			const data = {
				tag: node.tagName.toLowerCase(),
				rect: {
					x: rect.x, y: rect.y, width: rect.width, height: rect.height
				},
				style: {
					display: style.display,
					position: style.position,
					color: style.color,
					backgroundColor: style.backgroundColor,
					fontSize: style.fontSize,
					fontFamily: style.fontFamily
				},
				children: []
			};
			
			for (let child of node.childNodes) {
				const serializedChild = serialize(child);
				if (serializedChild && (serializedChild.type !== 'text' || serializedChild.content)) {
					data.children.push(serializedChild);
				}
			}
			return data;
		}
		return serialize(document.body);
	}
	`

	result, err := page.Evaluate(script)
	require.NoError(t, err, "failed to evaluate DOM snapshot script")

	snapshotJSON, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err, "failed to marshal snapshot")

	baselinePath := filepath.Join("testdata", "snapshots", name+".json")

	// Update or create baseline
	if config.UpdateBase {
		os.MkdirAll(filepath.Dir(baselinePath), 0755)
		err = os.WriteFile(baselinePath, snapshotJSON, 0644)
		require.NoError(t, err, "failed to update DOM snapshot baseline")
		return
	}

	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(baselinePath), 0755)
		err = os.WriteFile(baselinePath, snapshotJSON, 0644)
		require.NoError(t, err, "failed to create initial DOM snapshot baseline")
		return
	}

	// Compare
	baselineData, err := os.ReadFile(baselinePath)
	require.NoError(t, err, "failed to read DOM snapshot baseline")

	assert.JSONEq(t, string(baselineData), string(snapshotJSON), "DOM snapshot mismatch for %s", name)
}

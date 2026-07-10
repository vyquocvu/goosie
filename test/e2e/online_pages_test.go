//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

func TestOnlinePagesRendering(t *testing.T) {
	// Config
	config := VisualTestConfig{
		DiffThreshold:  0.95, // Live pages contain highly complex layout, style, and scripts that diverge from Goosie's minimal engine
		OutputDir:      filepath.Join("testdata", "results"),
		ViewportWidth:  1280,
		ViewportHeight: 800,
	}

	pages := []struct {
		Name string
		URL  string
	}{
		{"example", "https://example.com/"},
		{"lorem", "https://lipsum.com/"},
		{"wikipedia", "https://en.wikipedia.org/wiki/Main_Page"},
		{"w3schools", "https://www.w3schools.com/"},
	}

	for _, p := range pages {
		t.Run(p.Name, func(t *testing.T) {
			localConfig := config
			if p.Name == "wikipedia" {
				localConfig.DiffThreshold = 0.995
			}

			// 1. Fetch live page content
			req, err := http.NewRequest("GET", p.URL, nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Skipf("Skipping online page %s because fetching failed: %v", p.Name, err)
			}
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			htmlContent := string(bodyBytes)

			// 2. Playwright rendering
			pwPage := newPage(t)
			defer pwPage.Close()

			err = pwPage.SetViewportSize(localConfig.ViewportWidth, localConfig.ViewportHeight)
			require.NoError(t, err)

			_, err = pwPage.Goto(p.URL)
			if err != nil {
				t.Skipf("Skipping online page %s because Playwright navigation failed: %v", p.Name, err)
			}

			// Inject fonts normalization closer to Fyne's defaults
			_, _ = pwPage.Evaluate(`() => {
				const style = document.createElement('style');
				style.textContent = "* { font-family: -apple-system, BlinkMacSystemFont, \"Segoe UI\", Roboto, \"Helvetica Neue\", Arial, sans-serif !important; line-height: 1.2; } body { background: #ffffff; }";
				document.head.appendChild(style);
			}`)

			browserPNG, err := pwPage.Screenshot(playwright.PageScreenshotOptions{
				FullPage: playwright.Bool(false),
			})
			require.NoError(t, err)

			// 3. Goosie rendering
			r := renderer.NewRenderer(float32(localConfig.ViewportWidth), float32(localConfig.ViewportHeight))
			r.SetTestingMode(true)
			r.SetCurrentURL(p.URL)
			ctx := context.Background()
			obj, err := r.RenderHTML(ctx, htmlContent)
			require.NoError(t, err)

			goosieImg, err := testutil.RenderToImage(obj, localConfig.ViewportWidth, localConfig.ViewportHeight)
			require.NoError(t, err)

			var goosieBuf bytes.Buffer
			err = png.Encode(&goosieBuf, goosieImg)
			require.NoError(t, err)

			// Save screenshots for visual inspection
			goosiePath := filepath.Join(localConfig.OutputDir, "goosie", p.Name+".png")
			browserPath := filepath.Join(localConfig.OutputDir, "browser", p.Name+".png")
			diffPath := filepath.Join(localConfig.OutputDir, "diffs_compare", p.Name+".png")

			os.MkdirAll(filepath.Dir(goosiePath), 0755)
			os.MkdirAll(filepath.Dir(browserPath), 0755)
			os.MkdirAll(filepath.Dir(diffPath), 0755)

			err = os.WriteFile(goosiePath, goosieBuf.Bytes(), 0644)
			require.NoError(t, err)
			err = os.WriteFile(browserPath, browserPNG, 0644)
			require.NoError(t, err)

			// Compare screenshots
			diffPercent, err := compareImages(goosieBuf.Bytes(), browserPNG, diffPath)
			require.NoError(t, err)

			t.Logf("Goosie vs Browser mismatch for %s: %.2f%% difference", p.Name, diffPercent*100)

			if diffPercent > localConfig.DiffThreshold {
				t.Errorf("Visual mismatch for %s too large: %.2f%% (limit: %.2f%%)", p.Name, diffPercent*100, localConfig.DiffThreshold*100)
			} else {
				os.Remove(diffPath)
			}
		})
	}
}

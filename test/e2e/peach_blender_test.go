//go:build e2e && online

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

func TestPeachBlenderRendering(t *testing.T) {
	url := "https://peach.blender.org/"
	name := "peach_blender"
	outputDir := filepath.Join("testdata", "results")
	width := 1280
	height := 1000

	// 1. Fetch live page content
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to fetch %s", url)
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	htmlContent := string(bodyBytes)

	// Also save the HTML fixture for inspection/offline testing
	os.MkdirAll(filepath.Join("testdata", "peach_blender"), 0755)
	_ = os.WriteFile(filepath.Join("testdata", "peach_blender", "index.html"), bodyBytes, 0644)

	// 2. Playwright rendering
	pwPage := newPage(t)
	defer pwPage.Close()

	err = pwPage.SetViewportSize(width, height)
	require.NoError(t, err)

	_, err = pwPage.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	require.NoError(t, err)

	browserPNG, err := pwPage.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(false),
	})
	require.NoError(t, err)

	// 3. Goosie rendering
	r := renderer.NewRenderer(float32(width), float32(height))
	r.SetTestingMode(true)
	r.SetCurrentURL(url)
	ctx := context.Background()
	obj, err := r.RenderHTML(ctx, htmlContent)
	require.NoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for !imagesSettled(r.GetRoot()) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	obj = r.UpdateViewport()

	goosieImg, err := testutil.RenderToImage(obj, width, height)
	require.NoError(t, err)

	var goosieBuf bytes.Buffer
	err = png.Encode(&goosieBuf, goosieImg)
	require.NoError(t, err)

	// Save screenshots
	goosiePath := filepath.Join(outputDir, "goosie", name+".png")
	browserPath := filepath.Join(outputDir, "browser", name+".png")
	diffPath := filepath.Join(outputDir, "diffs_compare", name+".png")

	os.MkdirAll(filepath.Dir(goosiePath), 0755)
	os.MkdirAll(filepath.Dir(browserPath), 0755)
	os.MkdirAll(filepath.Dir(diffPath), 0755)

	err = os.WriteFile(goosiePath, goosieBuf.Bytes(), 0644)
	require.NoError(t, err)
	err = os.WriteFile(browserPath, browserPNG, 0644)
	require.NoError(t, err)

	diffPercent, err := compareImages(goosieBuf.Bytes(), browserPNG, diffPath)
	require.NoError(t, err)

	t.Logf("Goosie vs Browser mismatch for %s: %.2f%% difference", name, diffPercent*100)
	require.Less(t, diffPercent, 0.60, "diff exceeds acceptable threshold")
}

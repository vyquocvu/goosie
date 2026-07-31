//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

func TestReproIANAExampleDomains(t *testing.T) {
	url := "https://www.iana.org/help/example-domains"
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	htmlContent := buf.String()

	width, height := 1280, 800

	pwPage := newPage(t)
	defer pwPage.Close()
	require.NoError(t, pwPage.SetViewportSize(width, height))
	_, err = pwPage.Goto(url)
	require.NoError(t, err)
	_, _ = pwPage.Evaluate(`() => {
		const style = document.createElement('style');
		style.textContent = "* { font-family: -apple-system, BlinkMacSystemFont, \\"Segoe UI\\", Roboto, \\"Helvetica Neue\\", Arial, sans-serif !important; line-height: 1.2; } body { background: #ffffff; }";
		document.head.appendChild(style);
	}`)
	browserPNG, err := pwPage.Screenshot(playwright.PageScreenshotOptions{FullPage: playwright.Bool(false)})
	require.NoError(t, err)

	r := renderer.NewRenderer(float32(width), float32(height))
	r.SetTestingMode(true)
	r.SetCurrentURL(url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	obj, err := r.RenderHTML(ctx, htmlContent)
	require.NoError(t, err)
	// Wait for async image loading so the logo has its intrinsic size before
	// the screenshot, then re-render with the loaded image.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if imgLoaded(r) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !imgLoaded(r) {
		t.Log("timed out waiting for image load")
	}
	obj = r.UpdateViewport()
	goosieImg, err := testutil.RenderToImage(obj, width, height)
	require.NoError(t, err)
	var goosieBuf bytes.Buffer
	require.NoError(t, png.Encode(&goosieBuf, goosieImg))

	os.MkdirAll("testdata/results/repro", 0755)
	require.NoError(t, os.WriteFile("testdata/results/repro/iana-goosie.png", goosieBuf.Bytes(), 0644))
	require.NoError(t, os.WriteFile("testdata/results/repro/iana-browser.png", browserPNG, 0644))

	diff, err := compareImages(goosieBuf.Bytes(), browserPNG, "testdata/results/repro/iana-diff.png")
	require.NoError(t, err)
	t.Logf("diff percent: %.4f", diff)
}

func imgLoaded(r *renderer.Renderer) bool {
	root := r.GetRoot()
	if root == nil {
		return false
	}
	var found bool
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil || found {
			return
		}
		if n.TagName == "img" && n.ImageData != nil {
			found = true
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return found
}

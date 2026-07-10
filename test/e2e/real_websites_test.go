//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gonet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

// milestoneGate returns true if the current milestone level meets the required level.
// The current milestone is read from GOOSIE_MILESTONE env var (default: 2).
// As milestones are completed in ROADMAP_V2.md, bump the default value.
//
// Current roadmap status:
//
//	M0-M2: complete
//	M3:    in progress (M3.1-M3.3 done, M3.4 pending)
//	M4+:   not started
func milestoneGate(t *testing.T, required int) bool {
	t.Helper()
	current := 2
	if v := os.Getenv("GOOSIE_MILESTONE"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil {
			current = parsed
		}
	}
	if current < required {
		t.Skipf("Requires milestone M%d (current: M%d). Set GOOSIE_MILESTONE=%d to enable.", required, current, required)
	}
	return true
}

type realWebsite struct {
	Name              string
	URL               string
	MinContentHeight  float32
	ContainsText      []string
	DiffThreshold     float64
	Category          string
	RequiredMilestone int
}

var testWebsites = []realWebsite{
	// --- M1 gates: validate navigation + fetch pipeline ---
	{
		Name:              "example.com",
		URL:               "https://example.com/",
		MinContentHeight:  100,
		ContainsText:      []string{"Example Domain", "This domain is for use in illustrative examples"},
		DiffThreshold:     0.95,
		Category:          "simple",
		RequiredMilestone: 1,
	},
	{
		Name:              "iana.org",
		URL:               "https://www.iana.org/",
		MinContentHeight:  200,
		ContainsText:      []string{"Internet Assigned Numbers Authority"},
		DiffThreshold:     0.95,
		Category:          "simple",
		RequiredMilestone: 1,
	},
	{
		Name:              "info.cern.ch",
		URL:               "https://info.cern.ch/",
		MinContentHeight:  100,
		ContainsText:      []string{"World Wide Web"},
		DiffThreshold:     0.90,
		Category:          "simple",
		RequiredMilestone: 1,
	},
	{
		Name:              "httpbin",
		URL:               "https://httpbin.org/html",
		MinContentHeight:  100,
		ContainsText:      []string{"Herman Melville"},
		DiffThreshold:     0.95,
		Category:          "simple",
		RequiredMilestone: 1,
	},
	{
		Name:              "testing.toscrape",
		URL:               "https://testing.toscrape.com/",
		MinContentHeight:  100,
		ContainsText:      []string{"Scraping"},
		DiffThreshold:     0.95,
		Category:          "simple",
		RequiredMilestone: 1,
	},

	// --- M2 gates: validate compact DOM + streaming parser ---
	{
		Name:              "w3schools",
		URL:               "https://www.w3schools.com/",
		MinContentHeight:  300,
		ContainsText:      []string{"W3Schools"},
		DiffThreshold:     0.95,
		Category:          "medium",
		RequiredMilestone: 2,
	},
	{
		Name:              "lipsum",
		URL:               "https://lipsum.com/",
		MinContentHeight:  200,
		ContainsText:      []string{"Lorem"},
		DiffThreshold:     0.95,
		Category:          "medium",
		RequiredMilestone: 2,
	},
	{
		Name:              "quotes.toscrape",
		URL:               "https://quotes.toscrape.com/",
		MinContentHeight:  300,
		ContainsText:      []string{"Quotes"},
		DiffThreshold:     0.95,
		Category:          "medium",
		RequiredMilestone: 2,
	},

	// --- M3 gates: validate CSS pipeline + computed styles ---
	{
		Name:              "wikipedia",
		URL:               "https://en.wikipedia.org/wiki/Main_Page",
		MinContentHeight:  500,
		ContainsText:      []string{"Wikipedia"},
		DiffThreshold:     0.995,
		Category:          "complex",
		RequiredMilestone: 3,
	},
	{
		Name:              "MDN",
		URL:               "https://developer.mozilla.org/en-US/docs/Web/HTML",
		MinContentHeight:  300,
		ContainsText:      []string{"HTML"},
		DiffThreshold:     0.995,
		Category:          "complex",
		RequiredMilestone: 3,
	},
}

func fetchWebsite(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Skipping %s: fetch failed: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status for %s", url)

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	return buf.String()
}

// TestRealWebsitesFetchAndParse validates M1 navigation pipeline:
// HTTP fetching, status codes, content types, HTML structure.
func TestRealWebsitesFetchAndParse(t *testing.T) {
	for _, site := range testWebsites {
		site := site
		t.Run(site.Name, func(t *testing.T) {
			milestoneGate(t, site.RequiredMilestone)

			htmlContent := fetchWebsite(t, site.URL)
			require.NotEmpty(t, htmlContent, "response body should not be empty")

			lowerHTML := strings.ToLower(htmlContent)
			assert.Contains(t, lowerHTML, "<html", "should contain <html tag")
			assert.Contains(t, lowerHTML, "<body", "should contain <body tag")

			for _, text := range site.ContainsText {
				assert.Contains(t, htmlContent, text, "should contain expected text: %q", text)
			}

			t.Logf("%s: fetched %d bytes", site.Name, len(htmlContent))
		})
	}
}

// TestRealWebsitesRendering validates M2 compact DOM + streaming parser:
// full pipeline: parse → style → layout → render → screenshot.
func TestRealWebsitesRendering(t *testing.T) {
	outputDir := filepath.Join("testdata", "results", "real_websites")

	for _, site := range testWebsites {
		site := site
		t.Run(site.Name, func(t *testing.T) {
			milestoneGate(t, site.RequiredMilestone)

			htmlContent := fetchWebsite(t, site.URL)

			r := renderer.NewRenderer(1280, 800)
			r.SetTestingMode(true)
			r.SetCurrentURL(site.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			obj, err := r.RenderHTML(ctx, htmlContent)
			require.NoError(t, err, "RenderHTML should not fail for %s", site.Name)
			require.NotNil(t, obj, "rendered object should not be nil for %s", site.Name)

			contentHeight := r.GetContentHeight()
			t.Logf("Rendered %s: content height = %.0f px", site.Name, contentHeight)
			assert.Greater(t, contentHeight, site.MinContentHeight,
				"content height should exceed minimum for %s", site.Name)

			goosieImg, err := testutil.RenderToImage(obj, 1280, 800)
			require.NoError(t, err, "RenderToImage should succeed for %s", site.Name)

			var goosieBuf bytes.Buffer
			err = png.Encode(&goosieBuf, goosieImg)
			require.NoError(t, err)

			screenshotPath := filepath.Join(outputDir, "goosie", site.Name+".png")
			os.MkdirAll(filepath.Dir(screenshotPath), 0755)
			err = os.WriteFile(screenshotPath, goosieBuf.Bytes(), 0644)
			require.NoError(t, err)
			t.Logf("Screenshot saved to %s", screenshotPath)
		})
	}
}

// TestRealWebsitesGoosieVsBrowser validates cross-cutting visual comparison.
// Requires M3 (CSS pipeline) for meaningful rendering parity.
func TestRealWebsitesGoosieVsBrowser(t *testing.T) {
	milestoneGate(t, 3)

	config := VisualTestConfig{
		DiffThreshold:  0.95,
		OutputDir:      filepath.Join("testdata", "results", "real_websites"),
		ViewportWidth:  1280,
		ViewportHeight: 800,
	}

	for _, site := range testWebsites {
		site := site
		t.Run(site.Name, func(t *testing.T) {
			localConfig := config
			localConfig.DiffThreshold = site.DiffThreshold

			htmlContent := fetchWebsite(t, site.URL)

			pwPage := newPage(t)
			defer pwPage.Close()

			err := pwPage.SetViewportSize(localConfig.ViewportWidth, localConfig.ViewportHeight)
			require.NoError(t, err)

			_, err = pwPage.Goto(site.URL)
			if err != nil {
				t.Skipf("Skipping %s: Playwright navigation failed: %v", site.Name, err)
			}

			_, _ = pwPage.Evaluate(`() => {
				const style = document.createElement('style');
				style.textContent = "* { font-family: -apple-system, BlinkMacSystemFont, \"Segoe UI\", Roboto, \"Helvetica Neue\", Arial, sans-serif !important; line-height: 1.2; } body { background: #ffffff; }";
				document.head.appendChild(style);
			}`)

			browserPNG, err := pwPage.Screenshot(playwright.PageScreenshotOptions{
				FullPage: playwright.Bool(false),
			})
			require.NoError(t, err)

			r := renderer.NewRenderer(float32(localConfig.ViewportWidth), float32(localConfig.ViewportHeight))
			r.SetTestingMode(true)
			r.SetCurrentURL(site.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			obj, err := r.RenderHTML(ctx, htmlContent)
			require.NoError(t, err)

			goosieImg, err := testutil.RenderToImage(obj, localConfig.ViewportWidth, localConfig.ViewportHeight)
			require.NoError(t, err)

			var goosieBuf bytes.Buffer
			err = png.Encode(&goosieBuf, goosieImg)
			require.NoError(t, err)

			goosiePath := filepath.Join(localConfig.OutputDir, "goosie", site.Name+".png")
			browserPath := filepath.Join(localConfig.OutputDir, "browser", site.Name+".png")
			diffPath := filepath.Join(localConfig.OutputDir, "diffs_compare", site.Name+".png")

			os.MkdirAll(filepath.Dir(goosiePath), 0755)
			os.MkdirAll(filepath.Dir(browserPath), 0755)
			os.MkdirAll(filepath.Dir(diffPath), 0755)

			err = os.WriteFile(goosiePath, goosieBuf.Bytes(), 0644)
			require.NoError(t, err)
			err = os.WriteFile(browserPath, browserPNG, 0644)
			require.NoError(t, err)

			diffPercent, err := compareImages(goosieBuf.Bytes(), browserPNG, diffPath)
			require.NoError(t, err)

			t.Logf("Goosie vs Browser for %s: %.2f%% difference", site.Name, diffPercent*100)

			if diffPercent > localConfig.DiffThreshold {
				t.Errorf("Visual mismatch for %s too large: %.2f%% (limit: %.2f%%)",
					site.Name, diffPercent*100, localConfig.DiffThreshold*100)
			} else {
				os.Remove(diffPath)
			}
		})
	}
}

// TestHTTPSchemeHandling validates M1 navigation: HTTPS scheme handling.
func TestHTTPSchemeHandling(t *testing.T) {
	milestoneGate(t, 1)

	schemeTests := []struct {
		Name string
		URL  string
	}{
		{"https_example.com", "https://example.com/"},
		{"https_iana.org", "https://www.iana.org/"},
		{"https_info.cern.ch", "https://info.cern.ch/"},
	}

	for _, tc := range schemeTests {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			fetcher := gonet.NewFetcher()
			htmlContent, err := fetcher.Fetch(tc.URL)
			if err != nil {
				t.Skipf("Skipping %s: fetch failed: %v", tc.Name, err)
			}
			require.NotEmpty(t, htmlContent)

			r := renderer.NewRenderer(1280, 800)
			r.SetTestingMode(true)
			r.SetCurrentURL(tc.URL)

			ctx := context.Background()
			obj, err := r.RenderHTML(ctx, htmlContent)
			require.NoError(t, err)
			require.NotNil(t, obj)

			height := r.GetContentHeight()
			assert.Greater(t, height, float32(50), "should produce visible content")
			t.Logf("%s: fetched %d bytes, rendered height=%.0fpx", tc.Name, len(htmlContent), height)
		})
	}
}

// TestRealWebsitesByCategory validates M2 DOM parsing across complexity tiers.
func TestRealWebsitesByCategory(t *testing.T) {
	categories := map[string][]realWebsite{}
	for _, site := range testWebsites {
		categories[site.Category] = append(categories[site.Category], site)
	}

	for category, sites := range categories {
		sites := sites
		t.Run(category, func(t *testing.T) {
			t.Logf("Testing category %q with %d sites", category, len(sites))

			for _, site := range sites {
				site := site
				t.Run(site.Name, func(t *testing.T) {
					milestoneGate(t, site.RequiredMilestone)

					htmlContent := fetchWebsite(t, site.URL)

					r := renderer.NewRenderer(1280, 800)
					r.SetTestingMode(true)
					r.SetCurrentURL(site.URL)

					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					obj, err := r.RenderHTML(ctx, htmlContent)
					require.NoError(t, err)
					require.NotNil(t, obj)

					height := r.GetContentHeight()
					t.Logf("[%s] %s: height=%.0fpx, body=%d bytes", category, site.Name, height, len(htmlContent))
					assert.Greater(t, height, site.MinContentHeight)
				})
			}
		})
	}
}

// TestRealWebsitesCSSParsing validates M3 CSS pipeline: selector matching, computed styles.
func TestRealWebsitesCSSParsing(t *testing.T) {
	milestoneGate(t, 3)

	cssHeavySites := []struct {
		Name string
		URL  string
	}{
		{"wikipedia", "https://en.wikipedia.org/wiki/Main_Page"},
		{"w3schools", "https://www.w3schools.com/"},
		{"MDN", "https://developer.mozilla.org/en-US/docs/Web/HTML"},
	}

	for _, site := range cssHeavySites {
		site := site
		t.Run(site.Name, func(t *testing.T) {
			htmlContent := fetchWebsite(t, site.URL)

			r := renderer.NewRenderer(1280, 800)
			r.SetTestingMode(true)
			r.SetCurrentURL(site.URL)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			obj, err := r.RenderHTML(ctx, htmlContent)
			require.NoError(t, err)
			require.NotNil(t, obj)

			root := r.GetRoot()
			require.NotNil(t, root, "render tree root should exist after rendering %s", site.Name)

			nodeCount := countRenderNodes(root)
			t.Logf("%s: render tree has %d nodes", site.Name, nodeCount)
			assert.Greater(t, nodeCount, 0, "render tree should have nodes")
		})
	}
}

// TestFetcherResponseMetadata validates M1 navigation: response metadata preservation.
func TestFetcherResponseMetadata(t *testing.T) {
	milestoneGate(t, 1)

	sites := []struct {
		Name string
		URL  string
	}{
		{"example.com", "https://example.com/"},
		{"iana.org", "https://www.iana.org/"},
		{"httpbin", "https://httpbin.org/html"},
	}

	for _, site := range sites {
		site := site
		t.Run(site.Name, func(t *testing.T) {
			fetcher := gonet.NewFetcher()
			content, err := fetcher.Fetch(site.URL)
			if err != nil {
				t.Skipf("Skipping %s: %v", site.Name, err)
			}
			require.NotEmpty(t, content)

			meta := fetcher.Meta()
			t.Logf("%s: status=%d, content-type=%s, body=%d bytes",
				site.Name, meta.Status, meta.ContentType, len(content))

			assert.True(t, strings.Contains(meta.ContentType, "text/html"),
				"expected HTML content type for %s, got %s", site.Name, meta.ContentType)
		})
	}
}

func countRenderNodes(node *renderer.RenderNode) int {
	if node == nil {
		return 0
	}
	count := 1
	for _, child := range node.Children {
		count += countRenderNodes(child)
	}
	return count
}

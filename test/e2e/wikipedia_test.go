//go:build e2e && online

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/js"
	gonet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestCSSVariablesEndToEnd tests CSS custom property (variable) resolution in the full renderer pipeline.
// No network required.
func TestCSSVariablesEndToEnd(t *testing.T) {
	htmlWithVars := `<html><head><style>
		:root { --color-base: #202122; --background-color-base: #ffffff; }
		body { color: var(--color-base); background-color: var(--background-color-base); }
		p { font-size: var(--font-size-medium, 14px); }
	</style></head><body><p id="p">text</p></body></html>`

	r := renderer.NewRenderer(1280, 800)
	r.SetTestingMode(true)
	_, err := r.RenderHTML(context.Background(), htmlWithVars)
	assert.NoError(t, err)
}

// TestCalcWidthsEndToEnd tests calc() / clamp() expression evaluation in the full renderer pipeline.
// No network required.
func TestCalcWidthsEndToEnd(t *testing.T) {
	htmlWithCalc := `<html><head><style>
		.content-area { width: calc(100% - 160px); }
		.sidebar { width: clamp(120px, 20%, 200px); }
	</style></head><body>
		<div class="content-area">content</div>
		<div class="sidebar">sidebar</div>
	</body></html>`

	r := renderer.NewRenderer(1280, 800)
	r.SetTestingMode(true)
	_, err := r.RenderHTML(context.Background(), htmlWithCalc)
	assert.NoError(t, err)
}

// TestJSEnvironmentFeatures verifies that the JS runtime exposes all APIs expected by
// Wikipedia-style scripts: ES6 builtins, DOM observers, and browser globals.
// No network required.
func TestJSEnvironmentFeatures(t *testing.T) {
	rt := js.NewRuntime()
	rt.LoadHTML(`<html><body><div id="a"></div></body></html>`)

	checks := []struct {
		name string
		code string
	}{
		{"Promise", "typeof Promise === 'function'"},
		{"Promise.all", "typeof Promise.all === 'function'"},
		{"Map", "typeof Map === 'function'"},
		{"Set", "typeof Set === 'function'"},
		{"Array.from", "typeof Array.from === 'function'"},
		{"Object.entries", "typeof Object.entries === 'function'"},
		{"String.includes", "typeof ''.includes === 'function'"},
		{"MutationObserver", "typeof MutationObserver === 'function'"},
		{"IntersectionObserver", "typeof IntersectionObserver === 'function'"},
		{"requestAnimationFrame", "typeof requestAnimationFrame === 'function'"},
		{"getComputedStyle", "typeof window.getComputedStyle === 'function'"},
	}

	for _, tc := range checks {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			val, err := rt.RunScript(tc.code)
			assert.NoError(t, err)
			assert.Equal(t, "true", val.String(), "%s should be defined", tc.name)
		})
	}
}

// TestWikipediaMainPageLoads fetches the Wikipedia main page over the network and renders it.
// This test requires network access and a real HTTP connection.
func TestWikipediaMainPageLoads(t *testing.T) {
	client := &http.Client{Timeout: 30 * time.Second}
	fetcher := gonet.NewFetcherWithClient(client)

	html, err := fetcher.Fetch("https://en.wikipedia.org/wiki/Main_Page")
	if err != nil {
		t.Skipf("Skipping Wikipedia live test because fetching failed: %v", err)
	}
	require.NotEmpty(t, html)

	r := renderer.NewRenderer(1280, 800)
	r.SetTestingMode(true)
	r.SetCurrentURL("https://en.wikipedia.org/wiki/Main_Page")

	obj, err := r.RenderHTML(context.Background(), html)
	assert.NoError(t, err)
	assert.NotNil(t, obj)

	height := r.GetContentHeight()
	assert.Greater(t, height, float32(200), "Wikipedia should produce substantial content")
	t.Logf("Rendered Wikipedia main page, content height: %.0f px", height)
}

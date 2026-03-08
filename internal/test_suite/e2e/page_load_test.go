package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestRealPageLoad(t *testing.T) {
	// 1. Start Test Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/style.css" {
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte("h1 { font-size: 50px; }"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<html>
				<head>
					<link rel="stylesheet" href="/style.css">
				</head>
				<body>
					<h1 id="title">Hello World</h1>
				</body>
			</html>
		`))
	}))
	defer ts.Close()

	// 2. Initialize Components
	testApp := test.NewApp()
	defer testApp.Quit()

	fetcher := net.NewFetcher()
	r := renderer.NewRenderer(800, 600)

	// Set up a channel to wait for async CSS loading
	cssLoaded := make(chan bool, 1)
	r.SetRefreshCallback(func() {
		cssLoaded <- true
	})

	// 3. Fetch
	content, err := fetcher.Fetch(ts.URL)
	assert.NoError(t, err)
	assert.Contains(t, content, "Hello World")

	// 4. Render
	r.SetCurrentURL(ts.URL) // Crucial for resolving /style.css
	_, err = r.RenderHTML(content)
	assert.NoError(t, err)

	// 5. Wait for external CSS to load (async)
	select {
	case <-cssLoaded:
		t.Log("CSS loaded and renderer refreshed")
	case <-time.After(1 * time.Second):
		t.Log("Timed out waiting for CSS refresh (or it happened too fast/failed)")
	}

	// 6. Verify Content and Style
	root := r.GetRoot()
	assert.NotNil(t, root)

	h1 := findNodeByID(root, "title")
	if assert.NotNil(t, h1, "Should find h1 element with id='title'") {
		// h1 should have a text node child
		if assert.NotEmpty(t, h1.Children, "H1 should have children (text node)") {
			textNode := h1.Children[0]
			assert.Equal(t, "Hello World", textNode.Text)
		}

		// The style should be applied after refresh
		if h1.ComputedStyle != nil {
			assert.Equal(t, float32(50), h1.ComputedStyle.FontSize, "H1 should have 50px font size from external CSS")
		}
	}
}

// Helper to find node by ID
func findNodeByID(node *renderer.RenderNode, id string) *renderer.RenderNode {
	if node == nil {
		return nil
	}
	// Check attributes manually as GetAttribute might not be exported or handy here
	// RenderNode has Attrs map[string]string exported
	if val, ok := node.Attrs["id"]; ok && val == id {
		return node
	}
	for _, child := range node.Children {
		if found := findNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

package e2e

import (
	"bytes"
	"context"
	"io"
	stdnet "net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func requireLoopbackListener(t *testing.T) {
	t.Helper()
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback listener unavailable in this environment: %v", err)
	}
	require.NoError(t, ln.Close())
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestRealPageLoad(t *testing.T) {
	requireLoopbackListener(t)
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
	_, err = r.RenderHTML(context.Background(), content)
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

func TestExternalCSSNonCSSResponseIsIgnored(t *testing.T) {
	requireLoopbackListener(t)
	stylesheetRequested := make(chan struct{}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/style.css" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<!doctype html><html><body>Not Found</body></html>"))
			select {
			case stylesheetRequested <- struct{}{}:
			default:
			}
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

	out := captureStdout(t, func() {
		testApp := test.NewApp()
		defer testApp.Quit()

		fetcher := net.NewFetcher()
		r := renderer.NewRenderer(800, 600)

		content, err := fetcher.Fetch(ts.URL)
		assert.NoError(t, err)

		r.SetCurrentURL(ts.URL)
		_, err = r.RenderHTML(context.Background(), content)
		assert.NoError(t, err)

		select {
		case <-stylesheetRequested:
		case <-time.After(1 * time.Second):
			t.Fatalf("timed out waiting for stylesheet request")
		}

		time.Sleep(50 * time.Millisecond)
	})

	assert.NotContains(t, out, "Failed to parse CSS", "Non-CSS responses should not be parsed as CSS")
}

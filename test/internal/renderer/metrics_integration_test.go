package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/net"
)

func TestMetricsRecordingInPipeline(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)

	// An HTML page with inline style, multiple nodes, a layout box, text (fragments), and an image.
	htmlContent := `
	<html>
		<head>
			<style>
				body { margin: 0; padding: 10px; }
				h1, p { color: #333; }
			</style>
		</head>
		<body>
			<div>
				<h1>Title</h1>
				<p>This is a paragraph with <strong>bold</strong> text.</p>
				<img src="http://example.com/logo.png" />
			</div>
		</body>
	</html>`

	// Create recorder
	recorder := metrics.NewRecorder(1, "http://localhost/test.html")
	ctx := metrics.WithRecorder(context.Background(), recorder)

	_, err := r.RenderHTML(ctx, htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML returned error: %v", err)
	}

	snap := recorder.Snapshot()

	// Assertions
	// 1. Nodes: html, head, style, body, div, h1, text, p, text, strong, text, text, img, etc.
	// There should definitely be > 5 nodes.
	if snap.Counters.NodeCount <= 5 {
		t.Errorf("expected > 5 DOM nodes, got %d", snap.Counters.NodeCount)
	}

	// 2. CSS Rules: body {...} (1 rule), h1, p {...} (1 rule). Total = 2 rules.
	if snap.Counters.RuleCount != 2 {
		t.Errorf("expected 2 CSS rules, got %d", snap.Counters.RuleCount)
	}

	// 3. CSS Selectors: "body" (1 selector), "h1", "p" (2 selectors). Total = 3 selectors.
	if snap.Counters.SelectorCount != 3 {
		t.Errorf("expected 3 CSS selectors, got %d", snap.Counters.SelectorCount)
	}

	// 4. Layout Boxes: div, h1, p, img, body, etc. Should be > 2.
	if snap.Counters.BoxCount <= 2 {
		t.Errorf("expected > 2 layout boxes, got %d", snap.Counters.BoxCount)
	}

	// 5. Fragments: the text fragments wrapped in line boxes.
	if snap.Counters.FragmentCount < 2 {
		t.Errorf("expected at least 2 inline text fragments, got %d", snap.Counters.FragmentCount)
	}

	// 6. Display Items: paint commands created.
	if snap.Counters.DisplayItemCount < 1 {
		t.Errorf("expected at least 1 display item (paint commands), got %d", snap.Counters.DisplayItemCount)
	}

	// 7. Images: exactly 1 img tag.
	if snap.Counters.ImageCount != 1 {
		t.Errorf("expected 1 image count, got %d", snap.Counters.ImageCount)
	}
}

func TestMetricsRecordingWithExternalCSS(t *testing.T) {
	// Start a local test HTTP server to serve external CSS
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte("div { border: 1px solid black; } span.highlight { font-weight: bold; }"))
	}))
	defer server.Close()

	r := renderer.NewRenderer(800, 600)
	r.SetTestingMode(true)
	r.SetFetcher(net.NewFetcher()) // Ensure default client uses standard fetcher

	htmlContent := `
	<html>
		<head>
			<link rel="stylesheet" href="` + server.URL + `/style.css" />
		</head>
		<body>
			<div>
				<span class="highlight">Highlight</span>
			</div>
		</body>
	</html>`

	recorder := metrics.NewRecorder(2, "http://localhost/external.html")
	ctx := metrics.WithRecorder(context.Background(), recorder)

	_, err := r.RenderHTML(ctx, htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML returned error: %v", err)
	}

	// Give a tiny moment for any async/callback processing to finalize (though testingMode is synchronous)
	time.Sleep(10 * time.Millisecond)

	snap := recorder.Snapshot()

	// External CSS has 2 rules:
	// 1. div {...}
	// 2. span.highlight {...}
	// And 2 selectors total.
	if snap.Counters.RuleCount != 2 {
		t.Errorf("expected 2 rules from external CSS, got %d", snap.Counters.RuleCount)
	}

	if snap.Counters.SelectorCount != 2 {
		t.Errorf("expected 2 selectors from external CSS, got %d", snap.Counters.SelectorCount)
	}
}

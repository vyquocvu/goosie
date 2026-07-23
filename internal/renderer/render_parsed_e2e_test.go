package renderer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/documentloader"
	"github.com/vyquocvu/goosie/internal/engine/navigation"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"golang.org/x/net/html"
)

// TestRenderParsed_EndToEndCoordinatorFlow exercises the M3 acceptance
// criteria #3: the first styled frame must include rules from required
// external stylesheets.
//
// Setup:
//   - httptest server serves /page (HTML with link to /theme.css) and
//     /theme.css (a small stylesheet).
//   - Streaming parser feeds discoveries into the coordinator.
//   - The coordinator drains before RenderParsed is called.
//
// Asserts:
//   - The renderer's final stylesheet contains the .from-external
//     selector from /theme.css.
//   - RenderParsed returns a non-nil canvas object.
func TestRenderParsed_EndToEndCoordinatorFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html><head>
  <title>M3 E2E</title>
  <link rel="stylesheet" href="theme.css">
</head><body><p class="from-external">hello</p></body></html>`))
	})
	mux.HandleFunc("/theme.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		_, _ = w.Write([]byte(`.from-external { color: rebeccapurple; }`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/page")
	if err != nil {
		t.Fatalf("fetch /page: %v", err)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	htmlContent := string(bodyBytes)

	fetcher := goosienet.NewFetcherWithClient(srv.Client())

	// First pass: drive discovery only.
	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.URL+"/page")
	coord, err := documentloader.New(documentloader.Options{
		NavigationID:      load.ID,
		NavigationContext: navCtx,
		FinalURL:          srv.URL + "/page",
		Scheduler:         sched,
		Fetcher:           fetcher,
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(htmlContent),
		dom.ParseConfig{OnResource: coord.FromDomOnResource()}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := coord.HandleDocumentEnd(waitCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Second pass: collect external CSS into renderer-shaped results.
	sched2 := navigation.NewScheduler()
	defer sched2.Cancel()
	load2, navCtx2 := sched2.Begin(context.Background(), srv.URL+"/page")
	var collected []ExternalCSS
	coord2, err := documentloader.New(documentloader.Options{
		NavigationID:      load2.ID,
		NavigationContext: navCtx2,
		FinalURL:          srv.URL + "/page",
		Scheduler:         sched2,
		Fetcher:           fetcher,
		Callbacks: documentloader.Callbacks{
			OnStylesheet: func(r documentloader.CSSResult) {
				collected = append(collected, ExternalCSS{URL: r.Resolved, Source: r.Source})
			},
		},
	})
	if err != nil {
		t.Fatalf("coordinator2: %v", err)
	}
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(htmlContent),
		dom.ParseConfig{OnResource: coord2.FromDomOnResource()}); err != nil {
		t.Fatalf("parse2: %v", err)
	}
	if err := coord2.HandleDocumentEnd(waitCtx); err != nil {
		t.Fatalf("drain2: %v", err)
	}

	if len(collected) != 1 {
		t.Fatalf("expected 1 external stylesheet, got %d", len(collected))
	}
	if !strings.Contains(string(collected[0].Source), "from-external") {
		t.Errorf("collected source missing selector: %s", collected[0].Source)
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("render parse: %v", err)
	}

	r := NewRenderer(800, 600)
	r.testingMode = true
	r.SetHeadless(true)
	obj, err := r.RenderParsed(context.Background(), doc, collected)
	if err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
	if obj == nil {
		t.Fatal("nil canvas object")
	}

	r.stylesheetMu.RLock()
	defer r.stylesheetMu.RUnlock()
	if r.stylesheet == nil {
		t.Fatal("stylesheet nil after RenderParsed")
	}
	foundExternal := false
	for _, rule := range r.stylesheet.Rules {
		for _, seq := range rule.Selectors {
			for _, c := range seq.Simple.Classes {
				if c == "from-external" {
					foundExternal = true
				}
			}
		}
	}
	if !foundExternal {
		t.Error("rendered stylesheet missing .from-external rule from external CSS")
	}
}

// TestRenderParsed_NoExternalCSSDoesNotBlock — when the document has no
// <link rel="stylesheet">, the coordinator drains immediately and
// RenderParsed returns the inline-only render.
func TestRenderParsed_NoExternalCSSDoesNotBlock(t *testing.T) {
	htmlContent := `<html><head><style>.a { color: red; }</style></head><body><p class="a">x</p></body></html>`

	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), "https://example.com/")
	coord, err := documentloader.New(documentloader.Options{
		NavigationID:      load.ID,
		NavigationContext: navCtx,
		FinalURL:          "https://example.com/",
		Scheduler:         sched,
		Fetcher:           goosienet.NewFetcher(),
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}

	start := time.Now()
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(htmlContent),
		dom.ParseConfig{OnResource: coord.FromDomOnResource()}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("no-CSS drain took %v, expected fast", elapsed)
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := NewRenderer(800, 600)
	r.testingMode = true
	r.SetHeadless(true)
	if _, err := r.RenderParsed(context.Background(), doc, nil); err != nil {
		t.Fatalf("RenderParsed: %v", err)
	}
}

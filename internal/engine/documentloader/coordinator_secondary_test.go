package documentloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine/navigation"
)

// secondaryResourceServer: spin up a test server with a few CSS resources
// and return the base URL plus counts per path.
type secondaryResourceServer struct {
	srv      *httptest.Server
	fetchCnt map[string]*int32
	mu       sync.Mutex
}

func newSecondaryResourceServer(t *testing.T, paths map[string]string) *secondaryResourceServer {
	t.Helper()
	s := &secondaryResourceServer{fetchCnt: map[string]*int32{}}
	mux := http.NewServeMux()
	for path, body := range paths {
		path, body := path, body
		cnt := int32(0)
		s.fetchCnt[path] = &cnt
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&cnt, 1)
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(body))
		})
	}
	s.srv = httptest.NewServer(mux)
	return s
}

func (s *secondaryResourceServer) base() string { return s.srv.URL }
func (s *secondaryResourceServer) Close()       { s.srv.Close() }

func (s *secondaryResourceServer) fetchCount(path string) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return atomic.LoadInt32(s.fetchCnt[path])
}

func newSecondaryResourceFetcher(s *httptest.Server) Fetcher {
	return realFetcherFromTestServer(s)
}

// TestSecondaryResource_FontFace — @font-face url() discovered
// from a stylesheet is fetched by the coordinator and reported via
// OnFont.
func TestSecondaryResource_FontFace(t *testing.T) {
	srv := newSecondaryResourceServer(t, map[string]string{
		"/theme.css": `@font-face {
  font-family: 'MyFont';
  src: url('myfont.woff2');
}`,
		"/myfont.woff2": "fake-woff2-bytes",
	})
	defer srv.Close()

	// Drive the coordinator directly: enqueue the top-level stylesheet
	// and observe that EnqueueSecondary picks up the font URL.
	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.base()+"/theme.css")

	var (
		mu    sync.Mutex
		fonts []FontResult
		css   []CSSResult
	)
	var coord *Coordinator
	coord, err := New(Options{
		NavigationID: load.ID, NavigationContext: navCtx, FinalURL: srv.base() + "/theme.css",
		Scheduler: sched, Fetcher: newSecondaryResourceFetcher(srv.srv),
		Callbacks: Callbacks{
			OnStylesheet: func(r CSSResult) {
				mu.Lock()
				css = append(css, r)
				mu.Unlock()
				sheet, _ := parseCSS(r.Source)
				for _, sub := range extractFromSheet(sheet) {
					if sub.Kind == resourceFontKind {
						coord.EnqueueSecondary(context.Background(), KindFont, sub.URL, r.Resolved)
					}
				}
			},
			OnFont: func(r FontResult) {
				mu.Lock()
				fonts = append(fonts, r)
				mu.Unlock()
			},
		},
	})
	if err != nil {
		t.Fatalf("coord: %v", err)
	}
	coord.HandleResource(Resource{Kind: KindCSS, URL: srv.base() + "/theme.css"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}

	if len(css) != 1 {
		t.Fatalf("css results = %d, want 1", len(css))
	}
	if len(fonts) != 1 {
		t.Fatalf("font results = %d, want 1", len(fonts))
	}
	if !strings.HasSuffix(fonts[0].Resolved, "/myfont.woff2") {
		t.Errorf("font resolved = %q, want suffix /myfont.woff2", fonts[0].Resolved)
	}
	if srv.fetchCount("/myfont.woff2") != 1 {
		t.Errorf("font fetch count = %d, want 1", srv.fetchCount("/myfont.woff2"))
	}
}

// TestSecondaryResource_ImageInDeclaration — url() in any CSS
// declaration value is fetched and reported via OnImage.
func TestSecondaryResource_ImageInDeclaration(t *testing.T) {
	srv := newSecondaryResourceServer(t, map[string]string{
		"/theme.css": `.x { background-image: url('bg.png'); }`,
		"/bg.png":    "fake-png-bytes",
	})
	defer srv.Close()

	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.base()+"/theme.css")

	var (
		mu   sync.Mutex
		imgs []ImageResult
	)
	var coord *Coordinator
	coord, _ = New(Options{
		NavigationID: load.ID, NavigationContext: navCtx, FinalURL: srv.base() + "/theme.css",
		Scheduler: sched, Fetcher: newSecondaryResourceFetcher(srv.srv),
		Callbacks: Callbacks{
			OnStylesheet: func(r CSSResult) {
				sheet, _ := parseCSS(r.Source)
				for _, sub := range extractFromSheet(sheet) {
					if sub.Kind == resourceImageKind {
						coord.EnqueueSecondary(context.Background(), KindImage, sub.URL, r.Resolved)
					}
				}
			},
			OnImage: func(r ImageResult) {
				mu.Lock()
				imgs = append(imgs, r)
				mu.Unlock()
			},
		},
	})
	coord.HandleResource(Resource{Kind: KindCSS, URL: srv.base() + "/theme.css"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(imgs) != 1 {
		t.Fatalf("image results = %d, want 1", len(imgs))
	}
	if srv.fetchCount("/bg.png") != 1 {
		t.Errorf("image fetch count = %d, want 1", srv.fetchCount("/bg.png"))
	}
}

// TestSecondaryResource_ImportResolvesRelative — @import url()
// uses the parent stylesheet URL as the base for resolution.
func TestSecondaryResource_ImportResolvesRelative(t *testing.T) {
	srv := newSecondaryResourceServer(t, map[string]string{
		"/dir/theme.css":  `@import url('nested.css');`,
		"/dir/nested.css": `.x { color: red; }`,
	})
	defer srv.Close()

	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.base()+"/dir/theme.css")

	var (
		mu  sync.Mutex
		css []CSSResult
	)
	var coord *Coordinator
	coord, _ = New(Options{
		NavigationID: load.ID, NavigationContext: navCtx, FinalURL: srv.base() + "/dir/theme.css",
		Scheduler: sched, Fetcher: newSecondaryResourceFetcher(srv.srv),
		Callbacks: Callbacks{
			OnStylesheet: func(r CSSResult) {
				mu.Lock()
				css = append(css, r)
				mu.Unlock()
				sheet, _ := parseCSS(r.Source)
				for _, sub := range extractFromSheet(sheet) {
					if sub.Kind == resourceStylesheetKind {
						coord.EnqueueSecondary(context.Background(), KindCSS, sub.URL, r.Resolved)
					}
				}
			},
		},
	})
	coord.HandleResource(Resource{Kind: KindCSS, URL: srv.base() + "/dir/theme.css"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(css) != 2 {
		t.Fatalf("css results = %d, want 2 (theme + nested)", len(css))
	}
	if srv.fetchCount("/dir/theme.css") != 1 {
		t.Errorf("theme.css fetch count = %d", srv.fetchCount("/dir/theme.css"))
	}
	if srv.fetchCount("/dir/nested.css") != 1 {
		t.Errorf("nested.css fetch count = %d", srv.fetchCount("/dir/nested.css"))
	}
}

// TestMaxCSSImportDepth — exceeding the configured depth limit
// causes the extra @import to be skipped, not infinite-looped.
func TestMaxCSSImportDepth(t *testing.T) {
	// Build a chain a.css -> b.css -> c.css -> d.css (depth 4).
	srv := newSecondaryResourceServer(t, map[string]string{
		"/a.css": `@import url('b.css');`,
		"/b.css": `@import url('c.css');`,
		"/c.css": `@import url('d.css');`,
		"/d.css": `.x { color: red; }`,
	})
	defer srv.Close()

	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.base()+"/a.css")

	var (
		mu    sync.Mutex
		css   []CSSResult
		skips []string
	)
	var coord *Coordinator
	coord, _ = New(Options{
		NavigationID: load.ID, NavigationContext: navCtx, FinalURL: srv.base() + "/a.css",
		Scheduler: sched, Fetcher: newSecondaryResourceFetcher(srv.srv),
		MaxCSSImportDepth: 2, // limit recursion to depth 2
		Callbacks: Callbacks{
			OnStylesheet: func(r CSSResult) {
				mu.Lock()
				css = append(css, r)
				mu.Unlock()
				sheet, _ := parseCSS(r.Source)
				for _, sub := range extractFromSheet(sheet) {
					if sub.Kind == resourceStylesheetKind {
						coord.EnqueueSecondary(context.Background(), KindCSS, sub.URL, r.Resolved)
					}
				}
			},
			OnError: func(_ Resource, e error) {
				mu.Lock()
				skips = append(skips, e.Error())
				mu.Unlock()
			},
		},
	})
	coord.HandleResource(Resource{Kind: KindCSS, URL: srv.base() + "/a.css"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	// With depth 2: a, b can be processed. c and d would push past.
	// We expect at least the depth-limit skip to be reported.
	if len(skips) == 0 {
		t.Errorf("expected at least one skip, got 0 (css=%d)", len(css))
	}
	hasDepthSkip := false
	for _, s := range skips {
		if strings.Contains(s, "max css import depth") {
			hasDepthSkip = true
			break
		}
	}
	if !hasDepthSkip {
		t.Errorf("expected 'max css import depth' skip; got %v", skips)
	}
}

// TestFontResult_HasSource — OnFont delivers the raw bytes
// fetched from the network.
func TestFontResult_HasSource(t *testing.T) {
	srv := newSecondaryResourceServer(t, map[string]string{
		"/f.woff2": "WOFF2-DATA",
	})
	defer srv.Close()
	sched := navigation.NewScheduler()
	defer sched.Cancel()
	load, navCtx := sched.Begin(context.Background(), srv.base()+"/")

	var (
		mu    sync.Mutex
		fonts []FontResult
	)
	var coord *Coordinator
	coord, _ = New(Options{
		NavigationID: load.ID, NavigationContext: navCtx, FinalURL: srv.base() + "/",
		Scheduler: sched, Fetcher: newSecondaryResourceFetcher(srv.srv),
		Callbacks: Callbacks{
			OnFont: func(r FontResult) {
				mu.Lock()
				fonts = append(fonts, r)
				mu.Unlock()
			},
		},
	})
	coord.HandleResource(Resource{Kind: KindFont, URL: srv.base() + "/f.woff2"})
	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(fonts) != 1 {
		t.Fatalf("fonts = %d, want 1", len(fonts))
	}
	if string(fonts[0].Source) != "WOFF2-DATA" {
		t.Errorf("font source = %q, want WOFF2-DATA", fonts[0].Source)
	}
}

// --------------------------------------------------------------------------
// Helpers live in test_adapters.go:
//   - parseCSSAdapter, extractFromSheet wrap css.NewParser/ExtractResources
//   - newSecondaryResourceFetcher returns a Fetcher over the test server
//   - resourceStylesheetKind / resourceFontKind / resourceImageKind
//     mirror css.ResourceKind constants
// --------------------------------------------------------------------------

// parseCSS / extractFromSheet are convenience aliases used throughout
// the secondary-resource tests below.
var parseCSS = parseCSSAdapter

var _ = strings.Contains // keep imports tidy

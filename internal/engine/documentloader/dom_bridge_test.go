package documentloader

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/net"
)

// TestFromDomResource_Mapping — every dom.Resource field lands in the
// correct coordinator.Resource field.
func TestFromDomResource_Mapping(t *testing.T) {
	cases := []struct {
		name string
		in   dom.Resource
		want Resource
	}{
		{
			name: "css with integrity",
			in: dom.Resource{
				Kind:        dom.ResourceCSS,
				URL:         "a.css",
				Position:    7,
				Integrity:   "sha384-abc",
				CrossOrigin: "anonymous",
			},
			want: Resource{
				Kind:        KindCSS,
				URL:         "a.css",
				Position:    7,
				Integrity:   "sha384-abc",
				CrossOrigin: "anonymous",
			},
		},
		{
			name: "external classic script",
			in: dom.Resource{
				Kind:       dom.ResourceScript,
				URL:        "x.js",
				Position:   3,
				ScriptMode: dom.ScriptModeClassic,
			},
			want: Resource{
				Kind:       KindScript,
				URL:        "x.js",
				Position:   3,
				ScriptMode: ScriptModeClassic,
			},
		},
		{
			name: "external async script",
			in: dom.Resource{
				Kind:       dom.ResourceScript,
				URL:        "x.js",
				Position:   4,
				ScriptMode: dom.ScriptModeAsync,
			},
			want: Resource{
				Kind:       KindScript,
				URL:        "x.js",
				Position:   4,
				ScriptMode: ScriptModeAsync,
			},
		},
		{
			name: "inline module script",
			in: dom.Resource{
				Kind:       dom.ResourceScript,
				Position:   5,
				ScriptMode: dom.ScriptModeModule,
				Inline:     true,
			},
			want: Resource{
				Kind:       KindScript,
				Position:   5,
				ScriptMode: ScriptModeModule,
				Inline:     true,
			},
		},
		{
			name: "image with crossorigin",
			in: dom.Resource{
				Kind:        dom.ResourceImage,
				URL:         "x.png",
				Position:    9,
				CrossOrigin: "use-credentials",
			},
			want: Resource{
				Kind:        KindImage,
				URL:         "x.png",
				Position:    9,
				CrossOrigin: "use-credentials",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromDomResource(tc.in)
			if !resourceEqual(got, tc.want) {
				t.Errorf("FromDomResource = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestFromDomResources_Batch — FromDomResources handles empty input.
func TestFromDomResources_Batch(t *testing.T) {
	if got := FromDomResources(nil); got != nil {
		t.Errorf("FromDomResources(nil) = %v, want nil", got)
	}
	out := FromDomResources([]dom.Resource{
		{Kind: dom.ResourceCSS, URL: "a.css", Position: 0},
		{Kind: dom.ResourceScript, URL: "b.js", Position: 1, ScriptMode: dom.ScriptModeDefer},
	})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].Kind != KindCSS || out[0].Position != 0 {
		t.Errorf("out[0] = %+v", out[0])
	}
	if out[1].ScriptMode != ScriptModeDefer {
		t.Errorf("out[1].ScriptMode = %v, want ScriptModeDefer", out[1].ScriptMode)
	}
}

// TestFromDomResource_UnknownDefaults — unknown parser-side kinds and
// modes map to safe defaults (CSS / Classic) rather than producing
// zero values that the coordinator would reject as malformed.
func TestFromDomResource_UnknownDefaults(t *testing.T) {
	got := FromDomResource(dom.Resource{
		Kind:       dom.ResourceKind(99),
		URL:        "x",
		ScriptMode: dom.ScriptMode(99),
	})
	if got.Kind != KindCSS {
		t.Errorf("unknown kind default = %v, want KindCSS", got.Kind)
	}
	if got.ScriptMode != ScriptModeClassic {
		t.Errorf("unknown mode default = %v, want ScriptModeClassic", got.ScriptMode)
	}
}

// TestCoordinatorFromDomOnResource — the bridge closure, when fed by
// the streaming parser, results in the coordinator emitting the
// expected number of results in document order.
func TestCoordinatorFromDomOnResource(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	html := `<html><head>
		<link rel="stylesheet" href="local.css">
		<script src="ext.js"></script>
		<script>inline()</script>
	</head></html>`

	chCSS := h.fetcher.register("https://example.com/local.css")
	chJS := h.fetcher.register("https://example.com/ext.js")

	cfg := dom.ParseConfig{OnResource: coord.FromDomOnResource()}
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html), cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}

	waitForFetch(t, h.fetcher, time.Second,
		"https://example.com/local.css",
		"https://example.com/ext.js")

	chCSS <- fakeResponse{body: "body{}"}
	chJS <- fakeResponse{body: "console.log(1)"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()

	if len(h.cb.CSS) != 1 {
		t.Errorf("expected 1 CSS, got %d", len(h.cb.CSS))
	}
	if len(h.cb.Scripts) != 2 {
		t.Fatalf("expected 2 scripts (1 inline + 1 external), got %d", len(h.cb.Scripts))
	}
	// Document order: CSS at position 0, external script at position 1,
	// inline script at position 2.
	if h.cb.CSS[0].Position != 0 {
		t.Errorf("CSS position = %d, want 0", h.cb.CSS[0].Position)
	}
	if h.cb.Scripts[0].Position != 1 || h.cb.Scripts[0].Inline {
		t.Errorf("scripts[0] = %+v, want pos=1 inline=false", h.cb.Scripts[0])
	}
	if h.cb.Scripts[1].Position != 2 || !h.cb.Scripts[1].Inline {
		t.Errorf("scripts[1] = %+v, want pos=2 inline=true", h.cb.Scripts[1])
	}
}

// TestCoordinatorFromDomOnResource_CSP — the bridge closure respects
// CSP. Off-origin discoveries from the parser are skipped before fetch.
func TestCoordinatorFromDomOnResource_CSP(t *testing.T) {
	csp := net.ParseCSPHeader("default-src 'self'")
	h := newTestHarness(t, "https://example.com/page", csp)
	defer h.shutdown(t)
	coord := h.newCoord(t, csp)

	html := `<html><head>
		<link rel="stylesheet" href="https://evil.test/x.css">
		<link rel="stylesheet" href="local.css">
	</head></html>`

	ch := h.fetcher.register("https://example.com/local.css")

	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: coord.FromDomOnResource()}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	waitForFetch(t, h.fetcher, time.Second, "https://example.com/local.css")
	ch <- fakeResponse{body: "body{}"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()

	if h.fetcher.fetchCountFor("https://evil.test/x.css") != 0 {
		t.Errorf("CSP-blocked CSS was fetched")
	}
	if len(h.cb.CSS) != 1 {
		t.Errorf("expected 1 CSS, got %d", len(h.cb.CSS))
	}
	cspSkip := 0
	for _, e := range h.cb.Errors {
		var s *SkippedError
		if errors.As(e, &s) && strings.Contains(s.Reason, "csp") {
			cspSkip++
		}
	}
	if cspSkip != 1 {
		t.Errorf("CSP skips = %d, want 1 (errors=%v)", cspSkip, h.cb.Errors)
	}
}

// TestCoordinatorFromDomOnResource_ScriptModePreserved — the bridge
// surfaces async/defer/module mode to the coordinator.
func TestCoordinatorFromDomOnResource_ScriptModePreserved(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	// async and defer are unsupported in M1, so they should be skipped
	// (reported via OnError) but the discovery must still propagate.
	html := `<html><head>
		<script src="a.js" async></script>
		<script src="d.js" defer></script>
	</head></html>`

	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: coord.FromDomOnResource()}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}
	h.cb.snapshot()

	if h.fetcher.fetchCountFor("https://example.com/a.js") != 0 {
		t.Errorf("async script should be skipped in M1")
	}
	if h.fetcher.fetchCountFor("https://example.com/d.js") != 0 {
		t.Errorf("defer script should be skipped in M1")
	}
	if len(h.cb.Errors) != 2 {
		t.Errorf("expected 2 skip errors, got %d (%v)", len(h.cb.Errors), h.cb.Errors)
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func resourceEqual(a, b Resource) bool {
	return a.Kind == b.Kind &&
		a.URL == b.URL &&
		a.Position == b.Position &&
		a.ScriptMode == b.ScriptMode &&
		a.Inline == b.Inline &&
		a.Integrity == b.Integrity &&
		a.CrossOrigin == b.CrossOrigin
}

// waitForFetch polls until all of the given URLs have entered the
// fake fetcher (i.e. fetchCount >= 1) or the deadline elapses.
func waitForFetch(t *testing.T, f *fakeFetcher, max time.Duration, urls ...string) {
	t.Helper()
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		ready := true
		for _, u := range urls {
			if f.fetchCountFor(u) < 1 {
				ready = false
				break
			}
		}
		if ready {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("fetches never started for %v", urls)
}
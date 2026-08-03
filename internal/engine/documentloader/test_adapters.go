package documentloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

// cssResourceAdapter mirrors css.CSSResource so tests can iterate
// without importing the css package directly. We keep the shape
// minimal: Kind, URL, Property.
type cssResourceAdapter struct {
	Kind     uint8
	URL      string
	Property string
}

// Kind constants matching css.ResourceKind. The documentloader test
// package does not import css to avoid a cycle (the css package does
// not depend on documentloader, but we keep them decoupled at the
// test level for clarity). These constants MUST stay in sync with
// css.ResourceKind.
const (
	resourceStylesheetKind uint8 = 0
	resourceFontKind       uint8 = 1
	resourceImageKind      uint8 = 2
)

// parseCSS is the test-side wrapper around css.NewParser.Parse.
func parseCSSAdapter(src []byte) (interface{}, error) {
	return css.NewParser(string(src)).Parse()
}

// extractFromSheet walks a parsed css.StyleSheet and returns the
// extracted resources as a flat slice of adapters. We convert via
// type assertion because the sheet's static type is the css
// package's.
func extractFromSheet(sheet interface{}) []cssResourceAdapter {
	cssSheet, ok := sheet.(*css.StyleSheet)
	if !ok {
		return nil
	}
	raw := css.ExtractResources(cssSheet)
	out := make([]cssResourceAdapter, 0, len(raw))
	for _, r := range raw {
		var k uint8
		switch r.Kind {
		case css.ResourceStylesheet:
			k = resourceStylesheetKind
		case css.ResourceFont:
			k = resourceFontKind
		case css.ResourceImage:
			k = resourceImageKind
		default:
			continue
		}
		out = append(out, cssResourceAdapter{
			Kind: k, URL: r.URL, Property: r.Property,
		})
	}
	return out
}

// realFetcherFromTestServer wraps an httptest.Server's Client into a
// documentloader.Fetcher. It performs a synchronous GET and returns
// the body as a string.
func realFetcherFromTestServer(srv *httptest.Server) Fetcher {
	return &httpFetcher{client: srv.Client(), base: srv.URL}
}

// httpFetcher is a minimal Fetcher used by the secondary-resource tests.
// It uses the provided *http.Client to GET absolute URLs and returns the body.
type httpFetcher struct {
	client *http.Client
	base   string
}

func (h *httpFetcher) FetchWithContext(ctx context.Context, rawURL string, _ goosienet.ProgressCallback) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", &httpStatusError{code: resp.StatusCode}
	}
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf), nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return http.StatusText(e.code) }

// ensure time is referenced (some tests reference time.Duration; keep
// this file compilable even if future edits drop direct time refs).
var _ = time.Second

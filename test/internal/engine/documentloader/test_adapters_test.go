package documentloader_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine/documentloader"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type cssResourceAdapter struct {
	Kind     uint8
	URL      string
	Property string
}

const (
	resourceStylesheetKind uint8 = 0
	resourceFontKind       uint8 = 1
	resourceImageKind      uint8 = 2
)

func parseCSSAdapter(src []byte) (interface{}, error) {
	return css.NewParser(string(src)).Parse()
}

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

func realFetcherFromTestServer(srv *httptest.Server) documentloader.Fetcher {
	return &httpFetcher{client: srv.Client(), base: srv.URL}
}

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

var _ = time.Second

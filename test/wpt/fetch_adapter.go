package wpt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	jsapi "github.com/vyquocvu/goosie/internal/js"
	internalnet "github.com/vyquocvu/goosie/internal/net"
)

// wptFetchAdapter gives WPT fetch() calls a bounded transport to the local
// checkout aliases. Browser-origin and CORS checks remain in Runtime.Fetch;
// this adapter only chooses the local listener and never reaches arbitrary
// network hosts.
type wptFetchAdapter struct {
	ctx       context.Context
	client    *http.Client
	baseURL   string
	requests  *atomic.Int32
	successes *atomic.Int32
	failures  *atomic.Int32
}

func (f wptFetchAdapter) Fetch(rawURL, method string, headers map[string]string, body []byte) (result *jsapi.FetchResponse, err error) {
	if f.requests != nil {
		f.requests.Add(1)
	}
	defer func() {
		if err == nil {
			if f.successes != nil {
				f.successes.Add(1)
			}
		} else if f.failures != nil {
			f.failures.Add(1)
		}
	}()
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil {
		return nil, fmt.Errorf("WPT fetch URL is outside the local WPT server aliases")
	}
	transportURL := parsed
	if !sameOrigin(parsed, f.baseURL) {
		var ok bool
		transportURL, ok = wptLocalTransportURL(parsed, f.baseURL)
		if !ok {
			return nil, fmt.Errorf("WPT fetch URL is outside the local WPT server aliases")
		}
	}
	request, err := http.NewRequestWithContext(f.ctx, method, transportURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Host = parsed.Host
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := f.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.ContentLength > internalnet.MaxResponseBytes {
		return nil, fmt.Errorf("WPT fetch response exceeds %d bytes", internalnet.MaxResponseBytes)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, internalnet.MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > internalnet.MaxResponseBytes {
		return nil, fmt.Errorf("WPT fetch response exceeds %d bytes", internalnet.MaxResponseBytes)
	}
	responseHeaders := make(map[string]string, len(response.Header))
	for name, values := range response.Header {
		responseHeaders[name] = strings.Join(values, ", ")
	}
	statusText := strings.TrimPrefix(response.Status, fmt.Sprintf("%d ", response.StatusCode))
	responseURL := parsed.String()
	if response.Request != nil && response.Request.URL != nil {
		responseURL = response.Request.URL.String()
	}
	return &jsapi.FetchResponse{
		Status: response.StatusCode, StatusText: statusText, Headers: responseHeaders,
		Body: responseBody, URL: responseURL,
		OK: response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices,
	}, nil
}

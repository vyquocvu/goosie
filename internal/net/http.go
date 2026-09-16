package net

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTP is the 4-method interface the engine depends on. Declaring it here
// rather than using *http.Client directly keeps navigation mockable in tests.
type HTTP interface {
	Get(url string) (*Response, error)
	Post(url, contentType string, body io.Reader) (*Response, error)
	SetTimeout(d time.Duration)
	Close() error
}

// Response is a simplified HTTP response.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	URL        string
}

// DefaultClient returns an HTTP client backed by net/http.
func DefaultClient() HTTP {
	return &httpClient{client: &http.Client{Timeout: 30 * time.Second}}
}

type httpClient struct {
	client *http.Client
}

func (c *httpClient) Get(url string) (*Response, error) {
	if strings.HasPrefix(url, "file://") {
		return readFileURL(url)
	}
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readResponse(resp, url)
}

func (c *httpClient) Post(url, contentType string, body io.Reader) (*Response, error) {
	resp, err := c.client.Post(url, contentType, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readResponse(resp, url)
}

func (c *httpClient) SetTimeout(d time.Duration) {
	c.client.Timeout = d
}

func (c *httpClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

func readResponse(resp *http.Response, url string) (*Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("net: reading body: %w", err)
	}
	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
		URL:        url,
	}, nil
}

func readFileURL(url string) (*Response, error) {
	path := strings.TrimPrefix(url, "file://")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("net: reading file %s: %w", path, err)
	}
	return &Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/html"},
		Body:       data,
		URL:        url,
	}, nil
}

package net

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
)

// ProgressCallback is a function that can be used to report download progress.
type ProgressCallback func(progress float64)

// Fetcher handles HTTP requests and preserves response metadata.
type Fetcher struct {
	client  *http.Client
	service *Service

	metaMu sync.RWMutex
	meta   ResponseMeta
}

// NewFetcher creates a new Fetcher instance
func NewFetcher() *Fetcher {
	jar, _ := cookiejar.New(nil)
	return NewFetcherWithClient(&http.Client{Jar: jar})
}

// NewFetcherWithClient creates a new Fetcher instance with a custom HTTP client
func NewFetcherWithClient(client *http.Client) *Fetcher {
	return NewFetcherWithService(NewService(ServiceOptions{Client: client}))
}

func NewFetcherWithService(service *Service) *Fetcher {
	if service == nil {
		service = NewService(ServiceOptions{})
	}
	return &Fetcher{
		client:  service.client,
		service: service,
	}
}

func (f *Fetcher) Service() *Service {
	return f.service
}

// CSP returns the Content-Security-Policy parsed from the most recent fetch
// response, or nil if none was present.
func (f *Fetcher) CSP() *CSPPolicy {
	return f.service.CSP()
}

// Fetch retrieves the content from the given URL and preserves response metadata.
func (f *Fetcher) Fetch(url string) (string, error) {
	return f.FetchWithContext(context.Background(), url, nil)
}

// FetchWithContext retrieves the content with cancellation support and preserves response metadata.
func (f *Fetcher) FetchWithContext(ctx context.Context, url string, onProgress ProgressCallback) (string, error) {
	body, meta, err := f.service.FetchWithMeta(ctx, url, onProgress)
	f.metaMu.Lock()
	f.meta = meta
	f.metaMu.Unlock()
	return body, err
}

// Meta returns the response metadata from the most recent fetch.
// It is safe for concurrent use.
func (f *Fetcher) Meta() ResponseMeta {
	f.metaMu.RLock()
	defer f.metaMu.RUnlock()
	return f.meta
}

// FetchStreamWithContext retrieves the response body as an io.ReadCloser without
// buffering the entire body into memory. The caller must close the returned body.
// Response metadata is preserved for security and developer tools (M1.3).
func (f *Fetcher) FetchStreamWithContext(ctx context.Context, url string) (io.ReadCloser, ResponseMeta, error) {
	body, meta, err := f.service.FetchStream(ctx, url)
	f.metaMu.Lock()
	f.meta = meta
	f.metaMu.Unlock()
	return body, meta, err
}

// progressReader wraps an io.Reader to report progress.
type progressReader struct {
	io.Reader
	downloaded int64
	total      int64
	callback   ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 {
		pr.downloaded += int64(n)
		progress := float64(pr.downloaded) / float64(pr.total)
		pr.callback(progress)
	}
	return n, err
}

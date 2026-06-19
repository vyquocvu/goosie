package net

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
)

// ProgressCallback is a function that can be used to report download progress.
type ProgressCallback func(progress float64)

// Fetcher handles HTTP requests
type Fetcher struct {
	client  *http.Client
	service *Service
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

// Fetch retrieves the content from the given URL
func (f *Fetcher) Fetch(url string) (string, error) {
	return f.FetchWithContext(context.Background(), url, nil)
}

// FetchWithContext retrieves the content from the given URL with cancellation support
func (f *Fetcher) FetchWithContext(ctx context.Context, url string, onProgress ProgressCallback) (string, error) {
	return f.service.FetchWithContext(ctx, url, onProgress)
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

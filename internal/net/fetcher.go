package net

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// ProgressCallback is a function that can be used to report download progress.
type ProgressCallback func(progress float64)

// Fetcher handles HTTP requests
type Fetcher struct {
	client *http.Client
}

// NewFetcher creates a new Fetcher instance
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{},
	}
}

// Fetch retrieves the content from the given URL
func (f *Fetcher) Fetch(url string) (string, error) {
	return f.FetchWithContext(context.Background(), url, nil)
}

// FetchWithContext retrieves the content from the given URL with cancellation support
func (f *Fetcher) FetchWithContext(ctx context.Context, url string, onProgress ProgressCallback) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Try to get content length for progress calculation
	totalSizeStr := resp.Header.Get("Content-Length")
	totalSize, _ := strconv.ParseInt(totalSizeStr, 10, 64)

	var reader io.Reader = resp.Body
	if onProgress != nil && totalSize > 0 {
		reader = &progressReader{
			Reader:   resp.Body,
			total:    totalSize,
			callback: onProgress,
		}
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return buf.String(), nil
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

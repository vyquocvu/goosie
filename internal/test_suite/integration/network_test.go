package integration

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/net"
)

// MockRoundTripper is a mock implementation of http.RoundTripper
type MockRoundTripper struct {
	Response *http.Response
	Err      error
}

// RoundTrip implements the http.RoundTripper interface
func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Response, m.Err
}

func TestFetcher(t *testing.T) {
	// Create a mock response
	mockResponse := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString("<html><body>Mock Content</body></html>")),
		Header:     make(http.Header),
	}
	mockResponse.Header.Set("Content-Type", "text/html")

	// Create a mock client
	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: mockResponse,
		},
	}

	// Inject mock client into fetcher
	fetcher := net.NewFetcherWithClient(mockClient)

	// Test Fetch
	content, err := fetcher.Fetch("https://example.com")
	assert.NoError(t, err)
	assert.Equal(t, "<html><body>Mock Content</body></html>", content)
}

func TestFetcherError(t *testing.T) {
	// Create a mock client that returns 404
	mockResponse := &http.Response{
		StatusCode: 404,
		Body:       io.NopCloser(bytes.NewBufferString("Not Found")),
		Header:     make(http.Header),
	}

	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: mockResponse,
		},
	}

	fetcher := net.NewFetcherWithClient(mockClient)

	_, err := fetcher.Fetch("https://example.com/404")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

package testutil

import (
	"bytes"
	"io"
	"net/http"
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

// NewMockClient creates an http.Client with a MockRoundTripper
func NewMockClient(responseBody string, statusCode int, err error) *http.Client {
	return &http.Client{
		Transport: &MockRoundTripper{
			Response: &http.Response{
				StatusCode: statusCode,
				Body:       io.NopCloser(bytes.NewBufferString(responseBody)),
				Header:     make(http.Header),
			},
			Err: err,
		},
	}
}

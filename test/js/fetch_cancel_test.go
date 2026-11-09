package js_test

import (
	"context"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// cancelTrackingFetcher records whether the context passed to Fetch was
// cancelled before the request completed. This verifies that abort() tears
// down the socket at the transport level, not just ignores the response.
type cancelTrackingFetcher struct {
	cancelled bool
	release   chan struct{}
}

func (f *cancelTrackingFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	// Wait for either the context to be cancelled or the release signal
	select {
	case <-ctx.Done():
		f.cancelled = true
		return nil, ctx.Err()
	case <-f.release:
		return &js.FetchResponse{Status: 200, Body: []byte("ok")}, nil
	}
}

// TestFetchAbortCancelsContext verifies that calling abort() on a fetch
// request cancels the context passed to the HTTP transport, so the socket
// is torn down rather than continuing to download bytes.
// qc:BR-123
func TestFetchAbortCancelsContext(t *testing.T) {
	fetcher := &cancelTrackingFetcher{release: make(chan struct{})}
	r, err := js.New(js.Options{
		URL:        "https://example.test/",
		HTTPClient: fetcher,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	// Start a fetch with an abort signal
	if err := r.Run(`
		const controller = new AbortController();
		window.controller = controller;
		window.promise = fetch('https://example.test/slow', {signal: controller.signal});
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Give the fetch time to start
	time.Sleep(50 * time.Millisecond)

	// Abort the request
	if err := r.Run(`window.controller.abort()`, "inline"); err != nil {
		t.Fatalf("abort: %v", err)
	}

	// Wait for the abort to be processed
	deadline := time.Now().Add(2 * time.Second)
	for r.PendingFetches() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("fetch did not leave the pending set after abort")
		}
		time.Sleep(time.Millisecond)
	}
	r.DrainFetchCallbacks()

	// Release the fetcher (though it should have been cancelled already)
	close(fetcher.release)

	// Give time for the cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	// Verify the context was cancelled
	if !fetcher.cancelled {
		t.Error("fetch context was not cancelled when abort() was called; the transport should tear down the socket, not just ignore the response")
	}
}

// TestXHRAbortCancelsContext verifies that calling abort() on an XHR
// request cancels the context passed to the HTTP transport.
// qc:BR-123
func TestXHRAbortCancelsContext(t *testing.T) {
	fetcher := &cancelTrackingFetcher{release: make(chan struct{})}
	r, err := js.New(js.Options{
		URL:        "https://example.test/",
		HTTPClient: fetcher,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	// Start an XHR request
	if err := r.Run(`
		const xhr = new XMLHttpRequest();
		window.xhr = xhr;
		xhr.open('GET', 'https://example.test/slow');
		xhr.send();
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Give the request time to start
	time.Sleep(50 * time.Millisecond)

	// Abort the request
	if err := r.Run(`window.xhr.abort()`, "inline"); err != nil {
		t.Fatalf("abort: %v", err)
	}

	// Drain callbacks
	r.Tick()

	// Release the fetcher (though it should have been cancelled already)
	close(fetcher.release)

	// Give time for the cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	// Verify the context was cancelled
	if !fetcher.cancelled {
		t.Error("XHR context was not cancelled when abort() was called; the transport should tear down the socket, not just ignore the response")
	}
}

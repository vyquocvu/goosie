package js_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

type cancelTrackingFetcher struct {
	cancelled atomic.Bool
	release   chan struct{}
}

func (f *cancelTrackingFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	select {
	case <-ctx.Done():
		f.cancelled.Store(true)
		return nil, ctx.Err()
	case <-f.release:
		return &js.FetchResponse{Status: 200, Body: []byte("ok")}, nil
	}
}

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

	if err := r.Run(`
		const controller = new AbortController();
		window.controller = controller;
		window.promise = fetch('https://example.test/slow', {signal: controller.signal});
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := r.Run(`window.controller.abort()`, "inline"); err != nil {
		t.Fatalf("abort: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for r.PendingFetches() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("fetch did not leave the pending set after abort")
		}
		time.Sleep(time.Millisecond)
	}
	r.DrainFetchCallbacks()

	close(fetcher.release)

	time.Sleep(50 * time.Millisecond)

	if !fetcher.cancelled.Load() {
		t.Error("fetch context was not cancelled when abort() was called; the transport should tear down the socket, not just ignore the response")
	}
}

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

	if err := r.Run(`
		const xhr = new XMLHttpRequest();
		window.xhr = xhr;
		xhr.open('GET', 'https://example.test/slow');
		xhr.send();
	`, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := r.Run(`window.xhr.abort()`, "inline"); err != nil {
		t.Fatalf("abort: %v", err)
	}

	r.Tick()

	close(fetcher.release)

	time.Sleep(50 * time.Millisecond)

	if !fetcher.cancelled.Load() {
		t.Error("XHR context was not cancelled when abort() was called; the transport should tear down the socket, not just ignore the response")
	}
}

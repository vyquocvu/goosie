package js_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

// gatedFetcher blocks inside Fetch until the test releases it, so an abort can
// be observed while the request is still in flight.
type gatedFetcher struct {
	started chan struct{}
	release chan struct{}
	resp    *js.FetchResponse
	calls   atomic.Int32
}

func (g *gatedFetcher) Fetch(ctx context.Context, url, method string, headers map[string]string, body []byte) (*js.FetchResponse, error) {
	g.calls.Add(1)
	g.started <- struct{}{}
	<-g.release
	return g.resp, nil
}

// waitAbortedDrain waits for the request goroutine to observe the abort and
// queue its rejection, then drains it. The HTTP client stays blocked: an abort
// must not need the response to settle the promise.
func waitAbortedDrain(t *testing.T, r *js.Runtime) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for r.PendingFetches() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("fetch did not leave the pending set after abort")
		}
		time.Sleep(time.Millisecond)
	}
	r.DrainFetchCallbacks()
}

// defaultAbortText is what a rejection handler sees when it concatenates the
// value of a default abort. Chromium's reason is a DOMException, so the text
// carries its message: measured as "AbortError: signal is aborted without
// reason" by scripts/verify-br123-fetch-abort.js, section 1. The value's own
// shape is pinned in test/js/fetch_abort_reason_test.go.
const defaultAbortText = "rejected:AbortError: signal is aborted without reason"

// TestFetchAbortSignalRejectsAndDropsLateResponse pins the abort contract: a
// pending fetch rejects with AbortError as soon as the signal fires, and the
// response that arrives afterwards must never reach the script.
// qc:BR-123
func TestFetchAbortSignalRejectsAndDropsLateResponse(t *testing.T) {
	gf := &gatedFetcher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		resp: &js.FetchResponse{
			Status: 200, StatusText: "OK", Body: []byte("late"), OK: true,
			URL: "https://example.test/slow",
		},
	}
	r := newFetchRuntime(t, gf)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gf.release) }) }
	t.Cleanup(release)

	if err := r.Run(`
		var abortState = 'pending';
		var abortLateBody = '';
		var abortController = new AbortController();
		fetch('/slow', {signal: abortController.signal}).then(
			function(resp) {
				abortState = 'resolved';
				resp.text().then(function(body) { abortLateBody = body; });
			},
			function(err) { abortState = 'rejected:' + err; }
		);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-gf.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never reached the HTTP client")
	}

	if err := r.Run(`abortController.abort();`, "inline"); err != nil {
		t.Fatal(err)
	}
	waitAbortedDrain(t, r)

	if v, ok := r.Global("abortState"); !ok || v.Str != defaultAbortText {
		t.Fatalf("abortState = %+v, want %q", v, defaultAbortText)
	}

	// The response completes only now; it must be dropped, not delivered.
	release()
	drainFetch(t, r)
	r.DrainFetchCallbacks()

	if v, ok := r.Global("abortState"); !ok || v.Str != defaultAbortText {
		t.Errorf("state after late response = %+v, want %q", v, defaultAbortText)
	}
	if v, ok := r.Global("abortLateBody"); !ok || v.Str != "" {
		t.Errorf("abortLateBody = %+v, want empty (late response applied)", v)
	}
}

// TestFetchAbortPreAbortedSignalSkipsRequest pins the already-aborted case:
// fetch must reject without ever calling the HTTP client.
// qc:BR-123
func TestFetchAbortPreAbortedSignalSkipsRequest(t *testing.T) {
	m := &mockFetcher{
		responses: map[string]*js.FetchResponse{
			"https://example.test/never": {Status: 200, OK: true},
		},
	}
	r := newFetchRuntime(t, m)

	if err := r.Run(`
		var preAbortState = 'pending';
		var controller = new AbortController();
		controller.abort();
		fetch('/never', {signal: controller.signal}).then(
			function() { preAbortState = 'resolved'; },
			function(err) { preAbortState = 'rejected:' + err; }
		);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	r.DrainFetchCallbacks()

	if v, ok := r.Global("preAbortState"); !ok || v.Str != defaultAbortText {
		t.Errorf("preAbortState = %+v, want %q", v, defaultAbortText)
	}
	if got := m.callCount(); got != 0 {
		t.Errorf("HTTP client called %d times for a pre-aborted fetch, want 0", got)
	}
}

// TestFetchAbortCustomReasonRejectsWithReason verifies the rejection value is
// the signal's reason rather than a hardcoded string.
// qc:BR-123
func TestFetchAbortCustomReasonRejectsWithReason(t *testing.T) {
	gf := &gatedFetcher{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
		resp:    &js.FetchResponse{Status: 200, OK: true},
	}
	r := newFetchRuntime(t, gf)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gf.release) }) }
	t.Cleanup(release)

	if err := r.Run(`
		var reasonState = 'pending';
		var reasonController = new AbortController();
		fetch('/slow', {signal: reasonController.signal}).then(
			function() { reasonState = 'resolved'; },
			function(err) { reasonState = 'rejected:' + err; }
		);
	`, "inline"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-gf.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch never reached the HTTP client")
	}

	if err := r.Run(`reasonController.abort('stopped');`, "inline"); err != nil {
		t.Fatal(err)
	}
	waitAbortedDrain(t, r)

	if v, ok := r.Global("reasonState"); !ok || v.Str != "rejected:stopped" {
		t.Errorf("reasonState = %+v, want %q", v, "rejected:stopped")
	}
}

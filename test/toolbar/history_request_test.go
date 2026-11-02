package toolbar_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestHistoryCarriesTheRequestThatProducedAnEntry(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://example.test/form")
	post := toolbar.Request{Method: "POST", ContentType: "application/x-www-form-urlencoded", Body: "q=hello"}
	h.PushRequest("https://example.test/result", post)

	url, req, base, ok := h.PeekBackRequest()
	if !ok || url != "https://example.test/form" || base != 1 {
		t.Fatalf("PeekBackRequest = (%q, %+v, %d, %v), want the GET form entry", url, req, base, ok)
	}
	if req.IsPost() {
		t.Fatalf("the GET entry carries a POST request: %+v", req)
	}

	url, req, ok = h.CurrentRequest()
	if !ok || url != "https://example.test/result" || !req.IsPost() || req.Body != "q=hello" {
		t.Fatalf("CurrentRequest = (%q, %+v, %v), want the POST result entry", url, req, ok)
	}

	if got := h.Current(); got != "https://example.test/result" {
		t.Fatalf("Current() = %q, want the result URL", got)
	}
	entries, index := h.Entries()
	if len(entries) != 2 || index != 1 {
		t.Fatalf("Entries() = (%v, %d), want both URLs and index 1", entries, index)
	}
}

func TestHistoryPeekForwardRequest(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://example.test/one")
	post := toolbar.Request{Method: "POST", Body: "n=7"}
	h.PushRequest("https://example.test/two", post)
	h.Back()

	url, req, base, ok := h.PeekForwardRequest()
	if !ok || url != "https://example.test/two" || base != 0 || !req.IsPost() || req.Body != "n=7" {
		t.Fatalf("PeekForwardRequest = (%q, %+v, %d, %v)", url, req, base, ok)
	}
	if got := h.Current(); got != "https://example.test/one" {
		t.Fatalf("peeking moved the cursor to %q", got)
	}

	if url, ok := h.Forward(); !ok || url != "https://example.test/two" {
		t.Fatalf("Forward() = (%q, %v)", url, ok)
	}
	if _, req, ok := h.CurrentRequest(); !ok || req.Body != "n=7" {
		t.Fatalf("stepping forward lost the request: %+v", req)
	}
}

func TestHistoryReplaceKeepsRequestAndClearDropsIt(t *testing.T) {
	h := toolbar.NewHistory()
	h.PushRequest("https://example.test/submit", toolbar.Request{Method: "POST", Body: "a=1"})

	h.ReplaceCurrent("https://example.test/landed")
	url, req, ok := h.CurrentRequest()
	if !ok || url != "https://example.test/landed" || req.Body != "a=1" {
		t.Fatalf("ReplaceCurrent lost the request: (%q, %+v, %v)", url, req, ok)
	}

	h.ClearCurrentRequest()
	url, req, ok = h.CurrentRequest()
	if !ok || url != "https://example.test/landed" || req.IsPost() || req.Body != "" {
		t.Fatalf("ClearCurrentRequest = (%q, %+v, %v), want a GET entry", url, req, ok)
	}
}

func TestHistoryRestoreStartsEveryEntryAsGET(t *testing.T) {
	restored := toolbar.RestoreHistory([]string{"https://example.test/a", "https://example.test/b"}, 1)
	url, req, ok := restored.CurrentRequest()
	if !ok || url != "https://example.test/b" || req.IsPost() {
		t.Fatalf("restored CurrentRequest = (%q, %+v, %v), want a GET entry", url, req, ok)
	}
}

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

func TestHistoryStateSurvivesSnapshotTraversalAndRestore(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://example.test/start")
	h.PushState("https://example.test/one", toolbar.Request{}, []byte(`one`), 120, true)
	h.PushState("https://example.test/two", toolbar.Request{}, []byte(`two`), 240, true)
	url, _, scroll, state, sameDocument, base, ok := h.PeekBackEntry()
	if !ok || url != "https://example.test/one" || string(state) != "one" || scroll != 120 || !sameDocument || base != 2 {
		t.Fatalf("PeekBackEntry = (%q, %d, %q, %t, %d, %t)", url, scroll, state, sameDocument, base, ok)
	}
	urls, states, same, index := h.Snapshot()
	if len(urls) != 3 || string(states[1]) != "one" || !same[2] || index != 2 {
		t.Fatalf("Snapshot = (%v, %q, %v, %d)", urls, states, same, index)
	}
	states[1][0] = 'X'
	restored := toolbar.RestoreHistoryWithStates(urls, states, same, index)
	if _, ok := restored.CurrentState(); !ok {
		t.Fatal("restored history lost its current state")
	}
	if state, ok := restored.CurrentState(); !ok || string(state) != "two" {
		t.Fatalf("restored state = %q, %t", state, ok)
	}
	if !restored.Go(-1, index) {
		t.Fatal("Go(-1) failed")
	}
	if state, ok := restored.CurrentState(); !ok || string(state) != "Xne" {
		t.Fatalf("traversed state = %q, %t", state, ok)
	}
}

func TestHistorySameDocumentTraversalFollowsCrossedEntries(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://example.test/a")
	h.PushState("https://example.test/a2", toolbar.Request{}, []byte(`two`), 0, true)
	h.PushState("https://example.test/a3", toolbar.Request{}, []byte(`three`), 0, true)
	h.Push("https://example.test/b")
	if h.SameDocumentTraversal(-1) || h.SameDocumentTraversal(-3) {
		t.Fatal("leaving a loaded document was classified as same-document traversal")
	}
	if !h.Go(-1, 3) {
		t.Fatal("Go(-1) failed")
	}
	if !h.SameDocumentTraversal(-1) || !h.SameDocumentTraversal(-2) {
		t.Fatal("backward traversal within the shared document was not recognized")
	}
	if h.SameDocumentTraversal(1) {
		t.Fatal("traversal back to the loaded document was classified as same-document")
	}
	if !h.Go(-2, 2) {
		t.Fatal("Go(-2) failed")
	}
	if !h.SameDocumentTraversal(1) || !h.SameDocumentTraversal(2) {
		t.Fatal("forward traversal within the shared document was not recognized")
	}
}

func TestHistoryRestoreStartsEveryEntryAsGET(t *testing.T) {
	restored := toolbar.RestoreHistory([]string{"https://example.test/a", "https://example.test/b"}, 1)
	url, req, ok := restored.CurrentRequest()
	if !ok || url != "https://example.test/b" || req.IsPost() {
		t.Fatalf("restored CurrentRequest = (%q, %+v, %v), want a GET entry", url, req, ok)
	}
}

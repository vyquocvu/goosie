package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func newScriptNavigationSession(t *testing.T, html, base string) *Session {
	t.Helper()
	rt, err := js.New(js.Options{URL: base})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(html, nil, 800, WithScripts(base, nil), WithJS(rt))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	return s
}

// qc:BR-207
func TestSessionQueuesClassicAndModuleLocationNavigation(t *testing.T) {
	s := newScriptNavigationSession(t, `<!doctype html><html><body>
		<h1 id="old">old document</h1>
		<script>location.assign("/classic-next");</script>
		<script type="module">location.replace("/module-next");</script>
	</body></html>`, "https://example.test/start")
	defer s.Close()
	if got, err := s.EvaluateJS(`[location.href, document.getElementById("old").textContent].join("|")`); err != nil || got != "https://example.test/module-next|old document" {
		t.Fatalf("runtime state = %q, %v; want immediate URL with old document", got, err)
	}
	first, ok := s.PopNavigation()
	if !ok || first.Kind != js.NavigationAssign || first.URL != "https://example.test/classic-next" {
		t.Fatalf("first navigation = %+v, %t; want classic assign", first, ok)
	}
	second, ok := s.PopNavigation()
	if !ok || second.Kind != js.NavigationReplace || second.URL != "https://example.test/module-next" {
		t.Fatalf("second navigation = %+v, %t; want module replace", second, ok)
	}
	if _, ok := s.PopNavigation(); ok {
		t.Fatal("navigation queue was not drained")
	}
}

// qc:BR-207
func TestSessionQueuesLocationReload(t *testing.T) {
	s := newScriptNavigationSession(t, `<!doctype html><script>location.reload(true);</script>`, "https://example.test/start")
	defer s.Close()
	request, ok := s.PopNavigation()
	if !ok || request.Kind != js.NavigationReload || request.URL != "https://example.test/start" {
		t.Fatalf("reload navigation = %+v, %t", request, ok)
	}
}

// qc:BR-207
func TestSessionNavigationQueueIsBounded(t *testing.T) {
	source := `<script>`
	for i := 0; i < maxPendingNavigations+17; i++ {
		source += fmt.Sprintf(`location.assign("https://example.test/%d");`, i)
	}
	source += `</script>`
	s := newScriptNavigationSession(t, `<!doctype html>`+source, "https://example.test/start")
	defer s.Close()
	for i := 0; i < maxPendingNavigations; i++ {
		request, ok := s.PopNavigation()
		if !ok || request.URL != fmt.Sprintf("https://example.test/%d", i) {
			t.Fatalf("request %d = %+v, %t", i, request, ok)
		}
	}
	if _, ok := s.PopNavigation(); ok {
		t.Fatalf("queue exceeded bound of %d", maxPendingNavigations)
	}
}

func TestSessionNavigationQueuePreservesHistoryState(t *testing.T) {
	s := newScriptNavigationSession(t, `<!doctype html><script>history.pushState({page: 2}, "", "/next");</script>`, "https://example.test/start")
	defer s.Close()
	request, ok := s.PopNavigation()
	if !ok || request.Kind != js.NavigationHistoryPush || request.URL != "https://example.test/next" {
		t.Fatalf("history navigation = %+v, %t", request, ok)
	}
	if len(request.State) == 0 {
		t.Fatal("history navigation lost its state snapshot")
	}
	state := append([]byte(nil), request.State...)
	request.State[0] = 0
	got, err := s.EvaluateJS(`JSON.stringify(history.state)`)
	if err != nil || !strings.Contains(got, `"page":2`) {
		t.Fatalf("history.state = %q, %v", got, err)
	}
	_ = state
}

// qc:BR-207
func TestSessionCloseClearsNavigationQueueAndSink(t *testing.T) {
	s := newScriptNavigationSession(t, `<!doctype html><script>location.assign("/queued");</script>`, "https://example.test/start")
	s.Close()
	s.queueScriptNavigation(js.NavigationRequest{Kind: js.NavigationAssign, URL: "https://example.test/late"})
	if _, ok := s.PopNavigation(); ok {
		t.Fatal("closed session retained or accepted navigation requests")
	}
}

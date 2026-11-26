package js_test

import (
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func historyRuntimeWithSink(t *testing.T, requests *[]js.NavigationRequest) *js.Runtime {
	t.Helper()
	rt, err := js.New(js.Options{URL: "https://example.test/start", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	rt.SetNavigationSink(func(request js.NavigationRequest) {
		*requests = append(*requests, request)
	})
	return rt
}

func TestHistoryStateIsStructuredCloneSnapshot(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/start", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var source = {page: 2, nested: {values: [1, 2]}};
		history.pushState(source, "", "/two");
		source.page = 99;
		source.nested.values.push(3);
		var result = [history.state.page, history.state.nested.values.join(","), source.page, source.nested.values.join(",")].join("|");
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `result`); got != `2|1,2|99|1,2,3` {
		t.Fatalf("history.state = %q", got)
	}
}

func TestHistoryPushStateRejectsCrossOriginAndUncloneableState(t *testing.T) {
	var requests []js.NavigationRequest
	rt := historyRuntimeWithSink(t, &requests)
	if err := rt.Run(`
		var crossOrigin = "";
		try { history.pushState({}, "", "https://other.test/escape"); } catch (e) { crossOrigin = e.name; }
		var uncloneable = "";
		try { history.pushState({fn: function() {}}, "", "/bad"); } catch (e) { uncloneable = e.name; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `crossOrigin`); got != "SecurityError" {
		t.Fatalf("cross-origin error = %q, want SecurityError", got)
	}
	if got := evalString(t, rt, `uncloneable`); got != "DataCloneError" {
		t.Fatalf("clone error = %q, want DataCloneError", got)
	}
	if len(requests) != 0 {
		t.Fatalf("rejected calls queued %d navigations", len(requests))
	}
}

func TestHistoryTraversalRequestsAndPopStatePayload(t *testing.T) {
	var requests []js.NavigationRequest
	rt := historyRuntimeWithSink(t, &requests)
	if err := rt.Run(`
		history.pushState({page: 2}, "", "/two");
		history.pushState({page: 3}, "", "/three");
		history.back();
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %+v, want two pushes and one traversal", requests)
	}
	if requests[0].Kind != js.NavigationHistoryPush || requests[0].URL != "https://example.test/two" {
		t.Fatalf("first request = %+v", requests[0])
	}
	if requests[2].Kind != js.NavigationHistoryTraverse || requests[2].Delta != -1 {
		t.Fatalf("traversal request = %+v", requests[2])
	}
	if err := rt.Run(`
		window.events = [];
		window.addEventListener("popstate", function (event) {
			window.events.push(JSON.stringify(event.state));
		});
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	rt.ApplyHistoryEntry("https://example.test/two", requests[0].State, 1, 3, -1)
	result, err := rt.Evaluate(`window.events.join("|") + "|" + JSON.stringify(history.state) + "|" + history.length`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result, `{"page":2}|{"page":2}|3`) {
		t.Fatalf("popstate result = %q", result)
	}
}

func TestHistorySnapshotAndApplyFireHashChangeForTraversal(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/start"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	rt.SetHistorySnapshot(
		[]string{"https://example.test/start", "https://example.test/start#one"},
		[][]byte{nil, []byte(`{"kind":"object","props":[{"name":"page","value":{"kind":"number","string":"2"}}]}`)},
		1,
	)
	if err := rt.Run(`
		window.events = [];
		window.addEventListener("popstate", function (event) { window.events.push("pop:" + JSON.stringify(event.state)); });
		window.addEventListener("hashchange", function (event) { window.events.push("hash:" + event.oldURL + ">" + event.newURL); });
		var seed = JSON.stringify(history.state) + "|" + String(history.length);
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `seed`); got != `{"page":2}|2` {
		t.Fatalf("seed = %q", got)
	}
	rt.ApplyHistoryEntry("https://example.test/start", nil, 0, 2, -1)
	result, err := rt.Evaluate(`window.events.join("|")`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hash:https://example.test/start#one>https://example.test/start") {
		t.Fatalf("events = %q", result)
	}
	if !strings.Contains(result, "pop:null") {
		t.Fatalf("events = %q", result)
	}
}

func TestHistorySnapshotRejectsOversizedState(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/start"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if err := rt.Run(`
		var name = "";
		try { history.pushState("`+strings.Repeat("x", js.MaxHistoryStateBytes)+`", "", "/large"); } catch (e) { name = e.name; }
	`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `name`); got != "DataCloneError" {
		t.Fatalf("large state error = %q, want DataCloneError", got)
	}
}
